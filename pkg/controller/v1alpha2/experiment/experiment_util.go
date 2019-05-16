package experiment

import (
	"bytes"
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	experimentsv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/experiment/v1alpha2"
	trialsv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/trial/v1alpha2"
	apiv1alpha2 "github.com/kubeflow/katib/pkg/api/v1alpha2"
	"github.com/kubeflow/katib/pkg/controller/v1alpha2/experiment/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

func (r *ReconcileExperiment) createTrialInstance(expInstance *experimentsv1alpha2.Experiment, trialInstance *apiv1alpha2.Trial) error {
	BUFSIZE := 1024
	logger := log.WithValues("Experiment", types.NamespacedName{Name: expInstance.GetName(), Namespace: expInstance.GetNamespace()})

	trial := &trialsv1alpha2.Trial{}
	trial.Name = fmt.Sprintf("%s-%s", expInstance.GetName(), utilrand.String(8))
	trial.Namespace = expInstance.GetNamespace()
	trial.Labels = map[string]string{"experiment": expInstance.GetName()}

	if err := controllerutil.SetControllerReference(expInstance, trial, r.scheme); err != nil {
		logger.Error(err, "Set controller reference error")
		return err
	}

	trialParams := util.TrialTemplateParams{
		Experiment: expInstance.GetName(),
		Trial:      trial.Name,
		NameSpace:  trial.Namespace,
	}
	if trialInstance.Spec != nil && trialInstance.Spec.ParameterAssignments != nil {
		for _, p := range trialInstance.Spec.ParameterAssignments.Assignments {
			trialParams.HyperParameters = append(trialParams.HyperParameters, p)
		}
	}

	runSpec, err := util.GetRunSpec(expInstance, trialParams)
	if err != nil {
		logger.Error(err, "Fail to get RunSpec from experiment", expInstance.Name)
		return err
	}

	trial.Spec.RunSpec = runSpec

	buf := bytes.NewBufferString(runSpec)
	job := &unstructured.Unstructured{}
	if err := k8syaml.NewYAMLOrJSONDecoder(buf, BUFSIZE).Decode(job); err != nil {
		return fmt.Errorf("Invalid spec.trialTemplate: %v.", err)
	}

	var metricNames []string
	metricNames = append(metricNames, expInstance.Spec.Objective.ObjectiveMetricName)
	for _, mn := range expInstance.Spec.Objective.AdditionalMetricNames {
		metricNames = append(metricNames, mn)
	}

	mcSpec, err := util.GetMetricsCollectorManifest(expInstance.GetName(), trial.Name, job.GetKind(), trial.Namespace, metricNames, expInstance.Spec.MetricsCollectorSpec)
	if err != nil {
		logger.Error(err, "Error getting metrics collector manifest")
		return err
	}
	trial.Spec.MetricsCollectorSpec = mcSpec.String()

	if err := r.Create(context.TODO(), trial); err != nil {
		logger.Error(err, "Trial create error", "Trial name", trial.Name)
		return err
	}
	return nil

}

func (r *ReconcileExperiment) updateFinalizers(instance *experimentsv1alpha2.Experiment, finalizers []string) (reconcile.Result, error) {
	logger := log.WithValues("Experiment", types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace})
	if instance.GetDeletionTimestamp() != nil {
		if err := util.DeleteExperimentInDB(instance); err != nil {
			logger.Error(err, "Fail to delete data in DB")
			return reconcile.Result{}, err
		}
	}
	instance.SetFinalizers(finalizers)
	if err := r.Update(context.TODO(), instance); err != nil {
		logger.Error(err, "Fail to update finalizers")
		return reconcile.Result{}, err
	} else {
		// Need to requeue because finalizer update does not change metadata.generation
		return reconcile.Result{Requeue: true}, err
	}
}
