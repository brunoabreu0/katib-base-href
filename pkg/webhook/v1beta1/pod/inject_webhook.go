/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pod

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	common "github.com/kubeflow/katib/pkg/apis/controller/common/v1beta1"
	trialsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/trials/v1beta1"
	katibmanagerv1beta1 "github.com/kubeflow/katib/pkg/common/v1beta1"
	"github.com/kubeflow/katib/pkg/controller.v1beta1/consts"
	jobv1beta1 "github.com/kubeflow/katib/pkg/job/v1beta1"
	mccommon "github.com/kubeflow/katib/pkg/metricscollector/v1beta1/common"
	"github.com/kubeflow/katib/pkg/util/v1beta1/katibconfig"
)

var log = logf.Log.WithName("injector-webhook")

// sidecarInjector that inject metrics collect sidecar into master pod
type sidecarInjector struct {
	client  client.Client
	decoder types.Decoder

	// injectSecurityContext indicates if we should inject the security
	// context into the metrics collector sidecar.
	injectSecurityContext bool
}

var _ admission.Handler = &sidecarInjector{}

func (s *sidecarInjector) Handle(ctx context.Context, req types.Request) types.Response {
	// Get the namespace from req since the namespace in the pod is empty.
	namespace := req.AdmissionRequest.Namespace
	pod := &v1.Pod{}
	err := s.decoder.Decode(req, pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	// Check whether the pod need to be mutated
	needMutate, err := s.MutationRequired(pod, namespace)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	} else {
		if !needMutate {
			return admission.ValidationResponse(true, "")
		}
	}

	// Do mutation
	mutatedPod, err := s.Mutate(pod, namespace)
	if err != nil {
		log.Error(err, "Failed to inject metrics collector")
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	return admission.PatchResponse(pod, mutatedPod)
}

var _ inject.Client = &sidecarInjector{}

func (s *sidecarInjector) InjectClient(c client.Client) error {
	s.client = c
	return nil
}

var _ inject.Decoder = &sidecarInjector{}

func (s *sidecarInjector) InjectDecoder(d types.Decoder) error {
	s.decoder = d
	return nil
}

// NewSidecarInjector returns a new sidecar injector.
func NewSidecarInjector(c client.Client) *sidecarInjector {
	return &sidecarInjector{
		injectSecurityContext: viper.GetBool(consts.ConfigInjectSecurityContext),
		client:                c,
	}
}

func (s *sidecarInjector) MutationRequired(pod *v1.Pod, ns string) (bool, error) {
	jobKind, jobName, err := getKatibJob(pod)
	if err != nil {
		return false, nil
	}

	trial := &trialsv1beta1.Trial{}
	// jobName and Trial name is equal
	if err := s.client.Get(context.TODO(), apitypes.NamespacedName{Name: jobName, Namespace: ns}, trial); err != nil {
		return false, err
	}

	// If PrimaryPodLabel is not set we mutate all pods which are related to Trial job
	// Otherwise mutate pod only with appropriate labels
	if trial.Spec.PrimaryPodLabels != nil {
		if !isPrimaryPod(pod.Labels, trial.Spec.PrimaryPodLabels) {
			return false, nil
		}
	} else {
		// TODO (andreyvelich): This can be deleted after switch to custom CRD
		if !isMasterRole(pod, jobKind) {
			return false, nil
		}
	}

	if trial.Spec.MetricsCollector.Collector.Kind == common.NoneCollector {
		return false, nil
	}
	return true, nil
}

func (s *sidecarInjector) Mutate(pod *v1.Pod, namespace string) (*v1.Pod, error) {
	mutatedPod := pod.DeepCopy()

	kind, trialName, _ := getKatibJob(pod)
	trial := &trialsv1beta1.Trial{}
	if err := s.client.Get(context.TODO(), apitypes.NamespacedName{Name: trialName, Namespace: namespace}, trial); err != nil {
		return nil, err
	}

	injectContainer, err := s.getMetricsCollectorContainer(trial, pod)
	if err != nil {
		return nil, err
	}
	mutatedPod.Spec.Containers = append(mutatedPod.Spec.Containers, *injectContainer)

	mutatedPod.Spec.ShareProcessNamespace = pointer.BoolPtr(true)

	mountPath, pathKind := getMountPath(trial.Spec.MetricsCollector)
	if mountPath != "" {
		if err = mutateVolume(mutatedPod, kind, mountPath, injectContainer.Name, pathKind); err != nil {
			return nil, err
		}
	}
	if needWrapWorkerContainer(trial.Spec.MetricsCollector) {
		if err = wrapWorkerContainer(mutatedPod, namespace, kind, mountPath, pathKind, trial.Spec.MetricsCollector); err != nil {
			return nil, err
		}
	}
	log.Info("Inject metrics collector sidecar container", "Pod Generate Name", mutatedPod.GenerateName, "Trial", trialName)
	return mutatedPod, nil
}

func (s *sidecarInjector) getMetricsCollectorContainer(trial *trialsv1beta1.Trial, originalPod *v1.Pod) (*v1.Container, error) {
	mc := trial.Spec.MetricsCollector
	if mc.Collector.Kind == common.CustomCollector {
		return mc.Collector.CustomCollector, nil
	}
	metricName := trial.Spec.Objective.ObjectiveMetricName
	for _, v := range trial.Spec.Objective.AdditionalMetricNames {
		metricName += ";"
		metricName += v
	}
	metricsCollectorConfigData, err := katibconfig.GetMetricsCollectorConfigData(mc.Collector.Kind, s.client)
	if err != nil {
		return nil, err
	}
	args := getMetricsCollectorArgs(trial.Name, metricName, mc)
	sidecarContainerName := getSidecarContainerName(trial.Spec.MetricsCollector.Collector.Kind)

	injectContainer := v1.Container{
		Name:            sidecarContainerName,
		Image:           metricsCollectorConfigData.Image,
		Args:            args,
		ImagePullPolicy: metricsCollectorConfigData.ImagePullPolicy,
		Resources:       metricsCollectorConfigData.Resource,
	}

	// Inject the security context when the flag is enabled.
	if s.injectSecurityContext {
		if len(originalPod.Spec.Containers) != 0 &&
			originalPod.Spec.Containers[0].SecurityContext != nil {
			injectContainer.SecurityContext = originalPod.Spec.Containers[0].SecurityContext.DeepCopy()
		}
	}

	return &injectContainer, nil
}

func getMetricsCollectorArgs(trialName, metricName string, mc common.MetricsCollectorSpec) []string {
	args := []string{"-t", trialName, "-m", metricName, "-s", katibmanagerv1beta1.GetDBManagerAddr()}
	if mountPath, _ := getMountPath(mc); mountPath != "" {
		args = append(args, "-path", mountPath)
	}
	if mc.Source != nil && mc.Source.Filter != nil && len(mc.Source.Filter.MetricsFormat) > 0 {
		args = append(args, "-f", strings.Join(mc.Source.Filter.MetricsFormat, ";"))
	}
	return args
}

func getMountPath(mc common.MetricsCollectorSpec) (string, common.FileSystemKind) {
	if mc.Collector.Kind == common.StdOutCollector {
		return common.DefaultFilePath, common.FileKind
	} else if mc.Collector.Kind == common.FileCollector {
		return mc.Source.FileSystemPath.Path, common.FileKind
	} else if mc.Collector.Kind == common.TfEventCollector {
		return mc.Source.FileSystemPath.Path, common.DirectoryKind
	} else if mc.Collector.Kind == common.CustomCollector {
		if mc.Source == nil || mc.Source.FileSystemPath == nil {
			return "", common.InvalidKind
		}
		return mc.Source.FileSystemPath.Path, mc.Source.FileSystemPath.Kind
	} else {
		return "", common.InvalidKind
	}
}

func needWrapWorkerContainer(mc common.MetricsCollectorSpec) bool {
	mcKind := mc.Collector.Kind
	for _, kind := range NeedWrapWorkerMetricsCollecterList {
		if mcKind == kind {
			return true
		}
	}
	return false
}

func wrapWorkerContainer(
	pod *v1.Pod, namespace, jobKind, metricsFile string,
	pathKind common.FileSystemKind,
	mc common.MetricsCollectorSpec) error {
	index := -1
	for i, c := range pod.Spec.Containers {
		jobProvider, err := jobv1beta1.New(jobKind)
		if err != nil {
			return err
		}
		if jobProvider.IsTrainingContainer(i, c) {
			index = i
			break
		}
	}
	if index >= 0 {
		command := []string{"sh", "-c"}
		args, err := getContainerCommand(pod, namespace, index)
		if err != nil {
			return err
		}
		// If the first two commands are sh -c, we do not inject command.
		if args[0] == "sh" || args[0] == "bash" {
			if args[1] == "-c" {
				command = args[0:2]
				args = args[2:]
			}
		}
		if mc.Collector.Kind == common.StdOutCollector {
			redirectStr := fmt.Sprintf("1>%s 2>&1", metricsFile)
			args = append(args, redirectStr)
		}
		args = append(args, "&&", getMarkCompletedCommand(metricsFile, pathKind))
		argsStr := strings.Join(args, " ")
		c := &pod.Spec.Containers[index]
		c.Command = command
		c.Args = []string{argsStr}
	}
	return nil
}

func getMarkCompletedCommand(mountPath string, pathKind common.FileSystemKind) string {
	dir := mountPath
	if pathKind == common.FileKind {
		dir = filepath.Dir(mountPath)
	}
	// $$ is process id in shell
	pidFile := filepath.Join(dir, "$$$$.pid")
	return fmt.Sprintf("echo %s > %s", mccommon.TrainingCompleted, pidFile)
}

func mutateVolume(pod *v1.Pod, jobKind, mountPath, sidecarContainerName string, pathKind common.FileSystemKind) error {
	metricsVol := v1.Volume{
		Name: common.MetricsVolume,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	dir := mountPath
	if pathKind == common.FileKind {
		dir = filepath.Dir(mountPath)
	}
	vm := v1.VolumeMount{
		Name:      metricsVol.Name,
		MountPath: dir,
	}
	indexList := []int{}
	for i, c := range pod.Spec.Containers {
		shouldMount := false
		if c.Name == sidecarContainerName {
			shouldMount = true
		} else {
			jobProvider, err := jobv1beta1.New(jobKind)
			if err != nil {
				return err
			}
			shouldMount = jobProvider.IsTrainingContainer(i, c)
		}
		if shouldMount {
			indexList = append(indexList, i)
		}
	}
	for _, i := range indexList {
		c := &pod.Spec.Containers[i]
		if c.VolumeMounts == nil {
			c.VolumeMounts = make([]v1.VolumeMount, 0)
		}
		c.VolumeMounts = append(c.VolumeMounts, vm)
		pod.Spec.Containers[i] = *c
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, metricsVol)

	return nil
}

func getSidecarContainerName(cKind common.CollectorKind) string {
	if cKind == common.StdOutCollector || cKind == common.FileCollector {
		return mccommon.MetricLoggerCollectorContainerName
	} else {
		return mccommon.MetricCollectorContainerName
	}
}
