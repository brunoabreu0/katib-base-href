/*
Copyright 2022 The Kubeflow Authors.

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
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configv1beta1 "github.com/kubeflow/katib/pkg/apis/config/v1beta1"
	apis "github.com/kubeflow/katib/pkg/apis/controller"
	common "github.com/kubeflow/katib/pkg/apis/controller/common/v1beta1"
	experimentsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/experiments/v1beta1"
	suggestionsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/suggestions/v1beta1"
	trialsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/trials/v1beta1"
	"github.com/kubeflow/katib/pkg/controller.v1beta1/consts"
	"github.com/kubeflow/katib/pkg/controller.v1beta1/util"
	mccommon "github.com/kubeflow/katib/pkg/metricscollector/v1beta1/common"
)

var (
	timeout = time.Second * 5
)

func TestWrapWorkerContainer(t *testing.T) {
	primaryContainer := "tensorflow"
	trial := &trialsv1beta1.Trial{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trial-name",
			Namespace: "trial-namespace",
		},
		Spec: trialsv1beta1.TrialSpec{
			MetricsCollector: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.StdOutCollector,
				},
			},
			PrimaryContainerName: primaryContainer,
			SuccessCondition:     experimentsv1beta1.DefaultJobSuccessCondition,
			FailureCondition:     experimentsv1beta1.DefaultJobFailureCondition,
		},
	}

	metricsFile := "metric.log"

	testCases := []struct {
		trial           *trialsv1beta1.Trial
		pod             *v1.Pod
		metricsFile     string
		pathKind        common.FileSystemKind
		expectedPod     *v1.Pod
		err             bool
		testDescription string
	}{
		{
			trial: trial,
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: primaryContainer,
							Command: []string{
								"python main.py",
							},
						},
					},
				},
			},
			metricsFile: metricsFile,
			pathKind:    common.FileKind,
			expectedPod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: primaryContainer,
							Command: []string{
								"sh", "-c",
							},
							Args: []string{
								fmt.Sprintf("python main.py 1>%v 2>&1 && echo completed > $$$$.pid", metricsFile),
							},
						},
					},
				},
			},
			err:             false,
			testDescription: "Tensorflow container without sh -c",
		},
		{
			trial: trial,
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: primaryContainer,
							Command: []string{
								"sh", "-c",
								"python main.py",
							},
						},
					},
				},
			},
			metricsFile: metricsFile,
			pathKind:    common.FileKind,
			expectedPod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: primaryContainer,
							Command: []string{
								"sh", "-c",
							},
							Args: []string{
								fmt.Sprintf("python main.py 1>%v 2>&1 && echo completed > $$$$.pid", metricsFile),
							},
						},
					},
				},
			},
			err:             false,
			testDescription: "Tensorflow container with sh -c",
		},
		{
			trial: trial,
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "not-primary-container",
						},
					},
				},
			},
			pathKind:        common.FileKind,
			err:             true,
			testDescription: "Training pod doesn't have primary container",
		},
		{
			trial: func() *trialsv1beta1.Trial {
				t := trial.DeepCopy()
				t.Spec.EarlyStoppingRules = []common.EarlyStoppingRule{
					{
						Name:       "accuracy",
						Value:      "0.6",
						Comparison: common.ComparisonTypeLess,
					},
				}
				return t
			}(),
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: primaryContainer,
							Command: []string{
								"python main.py",
							},
						},
					},
				},
			},
			metricsFile: metricsFile,
			pathKind:    common.FileKind,
			expectedPod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: primaryContainer,
							Command: []string{
								"sh", "-c",
							},
							Args: []string{
								fmt.Sprintf("python main.py 1>%v 2>&1 || "+
									"if test -f $$$$.pid && [ $(head -n 1 $$.pid) = early-stopped ]; then "+
									"echo Training Container was Early Stopped; "+
									"else echo Training Container was Failed; exit 1; fi "+
									"&& echo completed > $$$$.pid", metricsFile),
							},
						},
					},
				},
			},
			err:             false,
			testDescription: "Container with early stopping command",
		},
	}

	for _, c := range testCases {
		err := wrapWorkerContainer(c.trial, c.pod, c.trial.Namespace, c.metricsFile, c.pathKind)
		if c.err && err == nil {
			t.Errorf("Case %s failed. Expected error, got nil", c.testDescription)
		} else if !c.err {
			if err != nil {
				t.Errorf("Case %s failed. Expected nil, got error: %v", c.testDescription, err)
			} else if !equality.Semantic.DeepEqual(c.pod.Spec.Containers, c.expectedPod.Spec.Containers) {
				t.Errorf("Case %s failed. Expected pod: %v, got: %v",
					c.testDescription, c.expectedPod.Spec.Containers, c.pod.Spec.Containers)
			}
		}
	}
}

func TestGetMetricsCollectorArgs(t *testing.T) {

	// Start test k8s server
	envTest := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "manifests", "v1beta1", "components", "crd"),
		},
	}
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		t.Error(err)
	}

	cfg, err := envTest.Start()
	if err != nil {
		t.Error(err)
	}

	g := gomega.NewGomegaWithT(t)

	mgr, err := manager.New(cfg, manager.Options{Metrics: metricsserver.Options{BindAddress: "0"}})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Start test manager.
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(context.TODO())).NotTo(gomega.HaveOccurred())
	}()

	c := mgr.GetClient()
	si := NewSidecarInjector(c, admission.NewDecoder(mgr.GetScheme()))

	testTrialName := "test-trial"
	testSuggestionName := "test-suggestion"
	testNamespace := "kubeflow"
	testAlgorithm := "random"
	testObjective := common.ObjectiveTypeMaximize
	testMetricName := "accuracy"
	katibDBAddress := fmt.Sprintf("katib-db-manager.%v:%v", testNamespace, consts.DefaultSuggestionPort)
	katibEarlyStopAddress := fmt.Sprintf("%v-%v.%v:%v", testSuggestionName, testAlgorithm, testNamespace, consts.DefaultEarlyStoppingPort)
	waitAllProcessesValue := false
	testPath := "/test/path"

	// Create kubeflow namespace.
	kubeflowNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}
	g.Expect(c.Create(context.TODO(), kubeflowNS)).NotTo(gomega.HaveOccurred())

	earlyStoppingRules := []string{
		"accuracy;0.6;less;5",
		"loss;2;greater",
	}

	testSuggestion := &suggestionsv1beta1.Suggestion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSuggestionName,
			Namespace: testNamespace,
		},
		Spec: suggestionsv1beta1.SuggestionSpec{
			Algorithm: &common.AlgorithmSpec{
				AlgorithmName: testAlgorithm,
			},
		},
	}

	testTrial := &trialsv1beta1.Trial{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTrialName,
			Namespace: testNamespace,
			Labels: map[string]string{
				consts.LabelExperimentName: testSuggestionName,
			},
		},
		Spec: trialsv1beta1.TrialSpec{
			Objective: &common.ObjectiveSpec{
				Type: testObjective,
			},
		},
	}

	testCases := []struct {
		trial              *trialsv1beta1.Trial
		metricNames        string
		mCSpec             common.MetricsCollectorSpec
		earlyStoppingRules []string
		katibConfig        configv1beta1.MetricsCollectorConfig
		expectedArgs       []string
		name               string
		err                bool
	}{
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.StdOutCollector,
				},
			},
			katibConfig: configv1beta1.MetricsCollectorConfig{
				WaitAllProcesses: &waitAllProcessesValue,
			},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
				"-path", common.DefaultFilePath,
				"-format", string(common.TextFormat),
				"-w", "false",
			},
			name: "StdOut MC",
		},
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.FileCollector,
				},
				Source: &common.SourceSpec{
					FileSystemPath: &common.FileSystemPath{
						Path:   testPath,
						Format: common.TextFormat,
					},
					Filter: &common.FilterSpec{
						MetricsFormat: []string{
							"{mn1: ([a-b]), mv1: [0-9]}",
							"{mn2: ([a-b]), mv2: ([0-9])}",
						},
					},
				},
			},
			katibConfig: configv1beta1.MetricsCollectorConfig{},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
				"-path", testPath,
				"-f", "{mn1: ([a-b]), mv1: [0-9]};{mn2: ([a-b]), mv2: ([0-9])}",
				"-format", string(common.TextFormat),
			},
			name: "File MC with Filter",
		},
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.FileCollector,
				},
				Source: &common.SourceSpec{
					FileSystemPath: &common.FileSystemPath{
						Path:   testPath,
						Format: common.JsonFormat,
					},
				},
			},
			katibConfig: configv1beta1.MetricsCollectorConfig{},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
				"-path", testPath,
				"-format", string(common.JsonFormat),
			},
			name: "File MC with Json Format",
		},
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.TfEventCollector,
				},
				Source: &common.SourceSpec{
					FileSystemPath: &common.FileSystemPath{
						Path: testPath,
					},
				},
			},
			katibConfig: configv1beta1.MetricsCollectorConfig{},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
				"-path", testPath,
			},
			name: "Tf Event MC",
		},
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.CustomCollector,
				},
			},
			katibConfig: configv1beta1.MetricsCollectorConfig{},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
			},
			name: "Custom MC without Path",
		},
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.CustomCollector,
				},
				Source: &common.SourceSpec{
					FileSystemPath: &common.FileSystemPath{
						Path: testPath,
					},
				},
			},
			katibConfig: configv1beta1.MetricsCollectorConfig{},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
				"-path", testPath,
			},
			name: "Custom MC with Path",
		},
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.PrometheusMetricCollector,
				},
			},
			katibConfig: configv1beta1.MetricsCollectorConfig{},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
			},
			name: "Prometheus MC without Path",
		},
		{
			trial:       testTrial,
			metricNames: testMetricName,
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.StdOutCollector,
				},
			},
			earlyStoppingRules: earlyStoppingRules,
			katibConfig:        configv1beta1.MetricsCollectorConfig{},
			expectedArgs: []string{
				"-t", testTrialName,
				"-m", testMetricName,
				"-o-type", string(testObjective),
				"-s-db", katibDBAddress,
				"-path", common.DefaultFilePath,
				"-format", string(common.TextFormat),
				"-stop-rule", earlyStoppingRules[0],
				"-stop-rule", earlyStoppingRules[1],
				"-s-earlystop", katibEarlyStopAddress,
			},
			name: "Trial with EarlyStopping rules",
		},
		{
			trial: func() *trialsv1beta1.Trial {
				trial := testTrial.DeepCopy()
				trial.ObjectMeta.Labels[consts.LabelExperimentName] = "invalid-name"
				return trial
			}(),
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.StdOutCollector,
				},
			},
			earlyStoppingRules: earlyStoppingRules,
			katibConfig:        configv1beta1.MetricsCollectorConfig{},
			name:               "Trial with invalid Experiment label name. Suggestion is not created",
			err:                true,
		},
	}

	g.Expect(c.Create(context.TODO(), testSuggestion)).NotTo(gomega.HaveOccurred())

	// Wait that Suggestion is created
	g.Eventually(func() error {
		return c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testSuggestionName}, testSuggestion)
	}, timeout).ShouldNot(gomega.HaveOccurred())

	for _, tc := range testCases {
		args, err := si.getMetricsCollectorArgs(tc.trial, tc.metricNames, tc.mCSpec, tc.katibConfig, tc.earlyStoppingRules)

		if !tc.err && err != nil {
			t.Errorf("Case: %v failed. Expected nil, got %v", tc.name, err)
		} else if tc.err && err == nil {
			t.Errorf("Case: %v failed. Expected err, got nil", tc.name)
		} else if !tc.err && !reflect.DeepEqual(tc.expectedArgs, args) {
			t.Errorf("Case %v failed. ExpectedArgs: %v, got %v", tc.name, tc.expectedArgs, args)
		}
	}
}

func TestNeedWrapWorkerContainer(t *testing.T) {
	testCases := []struct {
		mCSpec   common.MetricsCollectorSpec
		needWrap bool
	}{
		{
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.StdOutCollector,
				},
			},
			needWrap: true,
		},
		{
			mCSpec: common.MetricsCollectorSpec{
				Collector: &common.CollectorSpec{
					Kind: common.CustomCollector,
				},
			},
			needWrap: false,
		},
	}

	for _, tc := range testCases {
		needWrap := needWrapWorkerContainer(tc.mCSpec)
		if needWrap != tc.needWrap {
			t.Errorf("Expected needWrap %v, got %v", tc.needWrap, needWrap)
		}
	}
}

func TestMutateMetricsCollectorVolume(t *testing.T) {
	tc := struct {
		pod                  v1.Pod
		expectedPod          v1.Pod
		JobKind              string
		MountPath            string
		SidecarContainerName string
		PrimaryContainerName string
		pathKind             common.FileSystemKind
		err                  bool
	}{
		pod: v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: "train-job",
					},
					{
						Name: "init-container",
					},
					{
						Name: "metrics-collector",
					},
				},
			},
		},
		expectedPod: v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: "train-job",
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      common.MetricsVolume,
								MountPath: filepath.Dir(common.DefaultFilePath),
							},
						},
					},
					{
						Name: "init-container",
					},
					{
						Name: "metrics-collector",
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      common.MetricsVolume,
								MountPath: filepath.Dir(common.DefaultFilePath),
							},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: common.MetricsVolume,
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
		MountPath:            common.DefaultFilePath,
		SidecarContainerName: "metrics-collector",
		PrimaryContainerName: "train-job",
		pathKind:             common.FileKind,
	}

	err := mutateMetricsCollectorVolume(
		&tc.pod,
		tc.MountPath,
		tc.SidecarContainerName,
		tc.PrimaryContainerName,
		tc.pathKind)
	if err != nil {
		t.Errorf("mutateMetricsCollectorVolume failed: %v", err)
	} else if !equality.Semantic.DeepEqual(tc.pod, tc.expectedPod) {
		t.Errorf("Expected pod %v, got %v", tc.expectedPod, tc.pod)
	}
}

func TestGetSidecarContainerName(t *testing.T) {
	testCases := []struct {
		collectorKind         common.CollectorKind
		expectedCollectorKind string
	}{
		{
			collectorKind:         common.StdOutCollector,
			expectedCollectorKind: mccommon.MetricLoggerCollectorContainerName,
		},
		{
			collectorKind:         common.TfEventCollector,
			expectedCollectorKind: mccommon.MetricCollectorContainerName,
		},
	}

	for _, tc := range testCases {
		collectorKind := getSidecarContainerName(tc.collectorKind)
		if collectorKind != tc.expectedCollectorKind {
			t.Errorf("Expected Collector Kind: %v, got %v", tc.expectedCollectorKind, collectorKind)
		}
	}
}

func TestGetKatibJob(t *testing.T) {
	// Start test k8s server
	envTest := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "manifests", "v1beta1", "components", "crd"),
		},
	}
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		t.Error(err)
	}

	cfg, err := envTest.Start()
	if err != nil {
		t.Error(err)
	}

	g := gomega.NewGomegaWithT(t)

	mgr, err := manager.New(cfg, manager.Options{Metrics: metricsserver.Options{BindAddress: "0"}})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Start test manager.
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(context.TODO())).NotTo(gomega.HaveOccurred())
	}()

	c := mgr.GetClient()
	si := NewSidecarInjector(c, admission.NewDecoder(mgr.GetScheme()))

	namespace := "default"
	trialName := "trial-name"
	podName := "pod-name"
	deployName := "deploy-name"
	jobName := "job-name"

	testCases := []struct {
		pod             *v1.Pod
		job             *batchv1.Job
		deployment      *appsv1.Deployment
		expectedJobKind string
		expectedJobName string
		err             bool
		testDescription string
	}{
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1",
							Kind:       "Job",
							Name:       jobName + "-1",
						},
					},
				},
			},
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName + "-1",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "kubeflow.org/v1beta1",
							Kind:       "Trial",
							Name:       trialName + "-1",
							UID:        "test-uid",
						},
					},
				},
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							RestartPolicy: v1.RestartPolicyNever,
							Containers: []v1.Container{
								{
									Name:  "test",
									Image: "test",
								},
							},
						},
					},
				},
			},
			expectedJobKind: "Job",
			expectedJobName: jobName + "-1",
			err:             false,
			testDescription: "Valid run with ownership sequence: Trial -> Job -> Pod",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1",
							Kind:       "Job",
							Name:       jobName + "-2",
						},
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       deployName + "-2",
						},
					},
				},
			},
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName + "-2",
					Namespace: namespace,
				},
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							RestartPolicy: v1.RestartPolicyNever,
							Containers: []v1.Container{
								{
									Name:  "test",
									Image: "test",
								},
							},
						},
					},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployName + "-2",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "kubeflow.org/v1beta1",
							Kind:       "Trial",
							Name:       trialName + "-2",
							UID:        "test-uid",
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test-key": "test-value",
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"test-key": "test-value",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "test",
									Image: "test",
								},
							},
						},
					},
				},
			},
			expectedJobKind: "Deployment",
			expectedJobName: deployName + "-2",
			err:             false,
			testDescription: "Valid run with ownership sequence: Trial -> Deployment -> Pod, Job -> Pod",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1",
							Kind:       "Job",
							Name:       jobName + "-3",
						},
					},
				},
			},
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName + "-3",
					Namespace: namespace,
				},
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							RestartPolicy: v1.RestartPolicyNever,
							Containers: []v1.Container{
								{
									Name:  "test",
									Image: "test",
								},
							},
						},
					},
				},
			},
			err:             true,
			testDescription: "Run for not Trial's pod with ownership sequence: Job -> Pod",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1",
							Kind:       "Job",
							Name:       jobName + "-4",
						},
					},
				},
			},
			err:             true,
			testDescription: "Run when Pod owns Job that doesn't exists",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "invalid/api/version",
							Kind:       "Job",
							Name:       jobName + "-4",
						},
					},
				},
			},
			err:             true,
			testDescription: "Run when Pod owns Job with invalid API version",
		},
	}

	for _, tc := range testCases {
		// Create Job if it is needed
		if tc.job != nil {
			jobUnstr, err := util.ConvertObjectToUnstructured(tc.job)
			gvk := schema.GroupVersionKind{
				Group:   "batch",
				Version: "v1",
				Kind:    "Job",
			}
			jobUnstr.SetGroupVersionKind(gvk)
			if err != nil {
				t.Errorf("ConvertObjectToUnstructured error %v", err)
			}

			g.Expect(c.Create(context.TODO(), jobUnstr)).NotTo(gomega.HaveOccurred())

			// Wait that Job is created
			g.Eventually(func() error {
				return c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tc.job.Name}, jobUnstr)
			}, timeout).ShouldNot(gomega.HaveOccurred())
		}

		// Create Deployment if it is needed
		if tc.deployment != nil {
			g.Expect(c.Create(context.TODO(), tc.deployment)).NotTo(gomega.HaveOccurred())

			// Wait that Deployment is created
			g.Eventually(func() error {
				return c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tc.deployment.Name}, tc.deployment)
			}, timeout).ShouldNot(gomega.HaveOccurred())
		}

		object, _ := util.ConvertObjectToUnstructured(tc.pod)
		jobKind, jobName, err := si.getKatibJob(object, namespace)
		if !tc.err && err != nil {
			t.Errorf("Case %v failed. Error %v", tc.testDescription, err)
		} else if !tc.err && (tc.expectedJobKind != jobKind || tc.expectedJobName != jobName) {
			t.Errorf("Case %v failed. Expected jobKind %v, got %v, Expected jobName %v, got %v",
				tc.testDescription, tc.expectedJobKind, jobKind, tc.expectedJobName, jobName)
		} else if tc.err && err == nil {
			t.Errorf("Expected error got nil")
		}
	}
}

func TestIsPrimaryPod(t *testing.T) {
	testCases := []struct {
		podLabels        map[string]string
		primaryPodLabels map[string]string
		isPrimary        bool
		testDescription  string
	}{
		{
			podLabels: map[string]string{
				"test-key-1": "test-value-1",
				"test-key-2": "test-value-2",
				"test-key-3": "test-value-3",
			},
			primaryPodLabels: map[string]string{
				"test-key-1": "test-value-1",
				"test-key-2": "test-value-2",
			},
			isPrimary:       true,
			testDescription: "Pod contains all labels from primary pod labels",
		},
		{
			podLabels: map[string]string{
				"test-key-1": "test-value-1",
			},
			primaryPodLabels: map[string]string{
				"test-key-1": "test-value-1",
				"test-key-2": "test-value-2",
			},
			isPrimary:       false,
			testDescription: "Pod doesn't contain primary label",
		},
		{
			podLabels: map[string]string{
				"test-key-1": "invalid",
			},
			primaryPodLabels: map[string]string{
				"test-key-1": "test-value-1",
			},
			isPrimary:       false,
			testDescription: "Pod contains label with incorrect value",
		},
	}

	for _, tc := range testCases {
		isPrimary := isPrimaryPod(tc.podLabels, tc.primaryPodLabels)
		if isPrimary != tc.isPrimary {
			t.Errorf("Case %v. Expected isPrimary %v, got %v", tc.testDescription, tc.isPrimary, isPrimary)
		}
	}
}

func TestMutatePodMetadata(t *testing.T) {
	mutatedPodLabels := map[string]string{
		"custom-pod-label":    "custom-value",
		"katib-experiment":    "katib-value",
		consts.LabelTrialName: "test-trial",
	}

	testCases := []struct {
		pod             *v1.Pod
		trial           *trialsv1beta1.Trial
		mutatedPod      *v1.Pod
		testDescription string
	}{
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"custom-pod-label": "custom-value",
					},
				},
			},
			trial: &trialsv1beta1.Trial{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trial",
					Labels: map[string]string{
						"katib-experiment": "katib-value",
					},
				},
			},
			mutatedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: mutatedPodLabels,
				},
			},
			testDescription: "Mutated Pod should contain label from the origin Pod and Trial",
		},
	}

	for _, tc := range testCases {
		mutatePodMetadata(tc.pod, tc.trial)
		if !reflect.DeepEqual(tc.mutatedPod, tc.pod) {
			t.Errorf("Case %v. Expected Pod %v, got %v", tc.testDescription, tc.mutatedPod, tc.pod)
		}
	}
}

func TestMutatePodEnv(t *testing.T) {
	testcases := map[string]struct {
		pod        *v1.Pod
		trial      *trialsv1beta1.Trial
		mutatedPod *v1.Pod
		wantError  error
	}{
		"Valid case for mutating Pod's env variable": {
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "training-container",
						},
					},
				},
			},
			trial: &trialsv1beta1.Trial{
				Spec: trialsv1beta1.TrialSpec{
					PrimaryContainerName: "training-container",
				},
			},
			mutatedPod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "training-container",
							Env: []v1.EnvVar{
								{
									Name: consts.EnvTrialName,
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: fmt.Sprintf("metadata.labels['%s']", consts.LabelTrialName),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"Mismatch for Pod name and primaryContainerName in Trial": {
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "training-container",
						},
					},
				},
			},
			trial: &trialsv1beta1.Trial{
				Spec: trialsv1beta1.TrialSpec{
					PrimaryContainerName: "training-containers",
				},
			},
			mutatedPod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "training-container",
						},
					},
				},
			},
			wantError: fmt.Errorf(
				"Unable to find primary container %v in mutated pod containers %v",
				"training-containers",
				[]v1.Container{
					{
						Name: "training-container",
					},
				},
			),
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			err := mutatePodEnv(testcase.pod, testcase.trial)
			// Compare error with expected error
			if testcase.wantError != nil && err != nil {
				if diff := cmp.Diff(testcase.wantError.Error(), err.Error()); len(diff) != 0 {
					t.Errorf("Unexpected error (-want,+got):\n%s", diff)
				}
			} else if testcase.wantError != nil || err != nil {
				t.Errorf(
					"Unexpected error (-want,+got):\n%s",
					cmp.Diff(testcase.wantError, err, cmpopts.EquateErrors()),
				)
			}
			// Compare Pod with expected pod after mutation
			if diff := cmp.Diff(testcase.mutatedPod, testcase.pod); len(diff) != 0 {
				t.Errorf("Unexpected mutated result (-want,+got):\n%s", diff)
			}
		})
	}
}
