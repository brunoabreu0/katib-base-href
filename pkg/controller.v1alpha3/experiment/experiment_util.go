package experiment

import (
	"bytes"
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	experimentsv1alpha3 "github.com/kubeflow/katib/pkg/apis/controller/experiments/v1alpha3"
	suggestionsv1alpha3 "github.com/kubeflow/katib/pkg/apis/controller/suggestions/v1alpha3"
	trialsv1alpha3 "github.com/kubeflow/katib/pkg/apis/controller/trials/v1alpha3"
	suggestionController "github.com/kubeflow/katib/pkg/controller.v1alpha3/suggestion"
	"github.com/kubeflow/katib/pkg/controller.v1alpha3/util"
)

const (
	updatePrometheusMetrics = "update-prometheus-metrics"
)

func (r *ReconcileExperiment) createTrialInstance(expInstance *experimentsv1alpha3.Experiment, trialAssignment *suggestionsv1alpha3.TrialAssignment) error {
	BUFSIZE := 1024
	logger := log.WithValues("Experiment", types.NamespacedName{Name: expInstance.GetName(), Namespace: expInstance.GetNamespace()})

	trial := &trialsv1alpha3.Trial{}
	trial.Name = trialAssignment.Name
	trial.Namespace = expInstance.GetNamespace()
	trial.Labels = util.TrialLabels(expInstance)

	if err := controllerutil.SetControllerReference(expInstance, trial, r.scheme); err != nil {
		logger.Error(err, "Set controller reference error")
		return err
	}

	trial.Spec.Objective = expInstance.Spec.Objective

	hps := trialAssignment.ParameterAssignments
	trial.Spec.ParameterAssignments = trialAssignment.ParameterAssignments
	runSpec, err := r.GetRunSpecWithHyperParameters(expInstance, expInstance.GetName(), trial.Name, trial.Namespace, hps)
	if err != nil {
		logger.Error(err, "Fail to get RunSpec from experiment", expInstance.Name)
		return err
	}

	trial.Spec.RunSpec = runSpec
	if expInstance.Spec.TrialTemplate != nil {
		trial.Spec.RetainRun = expInstance.Spec.TrialTemplate.Retain
	}

	buf := bytes.NewBufferString(runSpec)
	job := &unstructured.Unstructured{}
	if err := k8syaml.NewYAMLOrJSONDecoder(buf, BUFSIZE).Decode(job); err != nil {
		return fmt.Errorf("Invalid spec.trialTemplate: %v.", err)
	}

	if expInstance.Spec.MetricsCollectorSpec != nil {
		trial.Spec.MetricsCollector = *expInstance.Spec.MetricsCollectorSpec
	}

	if err := r.Create(context.TODO(), trial); err != nil {
		logger.Error(err, "Trial create error", "Trial name", trial.Name)
		return err
	}
	return nil

}

func needUpdateFinalizers(exp *experimentsv1alpha3.Experiment) (bool, []string) {
	deleted := !exp.ObjectMeta.DeletionTimestamp.IsZero()
	pendingFinalizers := exp.GetFinalizers()
	contained := false
	for _, elem := range pendingFinalizers {
		if elem == updatePrometheusMetrics {
			contained = true
			break
		}
	}

	if !deleted && !contained {
		finalizers := append(pendingFinalizers, updatePrometheusMetrics)
		return true, finalizers
	}
	if deleted && contained {
		finalizers := []string{}
		for _, pendingFinalizer := range pendingFinalizers {
			if pendingFinalizer != updatePrometheusMetrics {
				finalizers = append(finalizers, pendingFinalizer)
			}
		}
		return true, finalizers
	}
	return false, []string{}
}

func (r *ReconcileExperiment) updateFinalizers(instance *experimentsv1alpha3.Experiment, finalizers []string) (reconcile.Result, error) {
	instance.SetFinalizers(finalizers)
	if err := r.Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	} else {
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			r.collector.IncreaseExperimentsDeletedCount(instance.Namespace)
		} else {
			r.collector.IncreaseExperimentsCreatedCount(instance.Namespace)
		}
		// Need to requeue because finalizer update does not change metadata.generation
		return reconcile.Result{Requeue: true}, err
	}
}

func (r *ReconcileExperiment) terminateSuggestion(instance *experimentsv1alpha3.Experiment) error {
	original := &suggestionsv1alpha3.Suggestion{}
	err := r.Get(context.TODO(),
		types.NamespacedName{Namespace: instance.GetNamespace(), Name: instance.GetName()}, original)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	// If Suggestion is failed or Suggestion is Succeeded, not needed to terminate Suggestion
	if original.IsFailed() || original.IsSucceeded() {
		return nil
	}
	log.Info("Start terminating suggestion")
	suggestion := original.DeepCopy()
	msg := "Suggestion is succeeded"
	suggestion.MarkSuggestionStatusSucceeded(suggestionController.SuggestionSucceededReason, msg)
	log.Info("Mark suggestion succeeded")

	if err := r.UpdateSuggestionStatus(suggestion); err != nil {
		return err
	}
	return nil
}
