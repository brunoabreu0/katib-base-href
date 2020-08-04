package experiment

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	commonapiv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/common/v1beta1"
	experimentsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/experiments/v1beta1"
	suggestionsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/suggestions/v1beta1"
	trialsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/trials/v1beta1"
	"github.com/kubeflow/katib/pkg/controller.v1beta1/consts"
	experimentUtil "github.com/kubeflow/katib/pkg/controller.v1beta1/experiment/util"
	util "github.com/kubeflow/katib/pkg/controller.v1beta1/util"
	manifestmock "github.com/kubeflow/katib/pkg/mock/v1beta1/experiment/manifest"
	suggestionmock "github.com/kubeflow/katib/pkg/mock/v1beta1/experiment/suggestion"
	kubeflowcommonv1 "github.com/kubeflow/tf-operator/pkg/apis/common/v1"
	tfv1 "github.com/kubeflow/tf-operator/pkg/apis/tensorflow/v1"
)

const (
	experimentName = "test-experiment"
	trialName      = "test-trial"
	namespace      = "default"

	timeout = time.Second * 40
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

type statusMatcher struct {
	x *suggestionsv1beta1.Suggestion
}

func (statusM statusMatcher) Matches(x interface{}) bool {
	// Cast interface to suggestion
	s := x.(*suggestionsv1beta1.Suggestion)

	isMatch := false
	// Verify that status is correct
	// statusM.x contains condition on 0 element that s must have
	for _, cond := range s.Status.Conditions {
		if cond.Type == statusM.x.Status.Conditions[0].Type &&
			cond.Reason == statusM.x.Status.Conditions[0].Reason &&
			cond.Message == statusM.x.Status.Conditions[0].Message {
			isMatch = true
		}
	}

	return isMatch
}

func (statusM statusMatcher) String() string {
	return fmt.Sprintf("status is equal %v", statusM.x.Status.Conditions)
}

func TestAdd(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Test - Try to add experiment controller to the manager
	g.Expect(Add(mgr)).NotTo(gomega.HaveOccurred())
}

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockSuggestion := suggestionmock.NewMockSuggestion(mockCtrl)

	mockCtrl2 := gomock.NewController(t)
	defer mockCtrl2.Finish()
	mockGenerator := manifestmock.NewMockGenerator(mockCtrl)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c := mgr.GetClient()

	r := &ReconcileExperiment{
		Client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		Suggestion: mockSuggestion,
		Generator:  mockGenerator,
		collector:  experimentUtil.NewExpsCollector(mgr.GetCache(), prometheus.NewRegistry()),
		recorder:   mgr.GetRecorder(ControllerName),
	}
	r.updateStatusHandler = func(instance *experimentsv1beta1.Experiment) error {
		var err error = errors.NewBadRequest("fake-error")
		// Try to update status until it be succeeded
		for err != nil {
			updatedInstance := &experimentsv1beta1.Experiment{}
			c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, updatedInstance)
			updatedInstance.Status = instance.Status
			err = r.updateStatus(updatedInstance)
		}
		return err
	}

	recFn := SetupTestReconcile(r)
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	returnedTFJob := newFakeTFJob()

	returnedUnstructured, err := util.ConvertObjectToUnstructured(returnedTFJob)
	if err != nil {
		t.Errorf("ConvertObjectToUnstructured failed: %v", err)
	}

	mockGenerator.EXPECT().GetRunSpecWithHyperParameters(gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(
		returnedUnstructured,
		nil).AnyTimes()

	suggestionRestartNo := newFakeSuggestion()

	mockSuggestion.EXPECT().GetOrCreateSuggestion(gomock.Any(), gomock.Any()).Return(
		suggestionRestartNo, nil).AnyTimes()

	mockSuggestion.EXPECT().UpdateSuggestion(gomock.Any()).Return(nil).AnyTimes()

	reasonRestart := "Experiment is succeeded"
	msgRestartNo := "Suggestion is succeeded, can't be restarted"
	suggestionRestartNo.MarkSuggestionStatusSucceeded(reasonRestart, msgRestartNo)

	suggestionRestartYes := newFakeSuggestion()
	suggestionRestartYes.Spec.ResumePolicy = experimentsv1beta1.FromVolume

	msgRestartYes := "Suggestion is succeeded, suggestion volume is not deleted, can be restarted"
	suggestionRestartYes.MarkSuggestionStatusSucceeded(reasonRestart, msgRestartYes)

	suggestionRestarting := newFakeSuggestion()

	msgRestarting := "Suggestion is not running"
	suggestionRestarting.MarkSuggestionStatusRunning(corev1.ConditionFalse, suggestionsv1beta1.SuggestionRestartReason, msgRestarting)

	// Manually update suggestion status after UpdateSuggestionStatus is called
	// Call when Trials are being deleted
	deleteTrialsCall := mockSuggestion.EXPECT().UpdateSuggestionStatus(gomock.Any()).Return(nil).Do(
		func(arg0 interface{}) {
			var err error = errors.NewBadRequest("fake-error")
			suggestion := &suggestionsv1beta1.Suggestion{}
			// We should Get suggestion because resource version can be modified
			for err != nil {
				c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, suggestion)
				suggestion.Status.Suggestions = suggestion.Status.Suggestions[1:]
				err = c.Status().Update(context.TODO(), suggestion)
			}
		})

	// Call when experiment is completed with ResumePolicy = NeverResume
	restartNoCall := mockSuggestion.EXPECT().UpdateSuggestionStatus(statusMatcher{suggestionRestartNo}).Return(nil).Do(
		func(arg0 interface{}) {
			var err error = errors.NewBadRequest("fake-error")
			suggestion := &suggestionsv1beta1.Suggestion{}
			for err != nil {
				c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, suggestion)
				suggestion.MarkSuggestionStatusSucceeded(reasonRestart, msgRestartNo)
				err = c.Status().Update(context.TODO(), suggestion)
			}
		})

	// Call when experiment is completed with ResumePolicy = FromVolume
	restartYesCall := mockSuggestion.EXPECT().UpdateSuggestionStatus(statusMatcher{suggestionRestartYes}).Return(nil).Do(
		func(arg0 interface{}) {
			var err error = errors.NewBadRequest("fake-error")
			suggestion := &suggestionsv1beta1.Suggestion{}
			for err != nil {
				c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, suggestion)
				suggestion.MarkSuggestionStatusSucceeded(reasonRestart, msgRestartYes)
				err = c.Status().Update(context.TODO(), suggestion)
			}
		})

	// Call when experiment is restarting
	experimentRestartingCall := mockSuggestion.EXPECT().UpdateSuggestionStatus(statusMatcher{suggestionRestarting}).Return(nil).Do(
		func(arg0 interface{}) {
			suggestion := &suggestionsv1beta1.Suggestion{}
			var err error = errors.NewBadRequest("fake-error")
			for err != nil {
				c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, suggestion)
				suggestion.MarkSuggestionStatusRunning(corev1.ConditionFalse, suggestionsv1beta1.SuggestionRestartReason, msgRestarting)
				err = c.Status().Update(context.TODO(), suggestion)
			}
		})

	gomock.InOrder(
		deleteTrialsCall,
		restartNoCall,
		restartYesCall,
		experimentRestartingCall,
	)

	// Test 1 - Regural experiment run
	instance := newFakeInstance()

	// Create the suggestion with NeverResume
	g.Expect(c.Create(context.TODO(), suggestionRestartNo)).NotTo(gomega.HaveOccurred())
	// Manually update suggestion's status with 3 suggestions
	// Ones redundant trial is deleted, suggestion status must be updated
	g.Eventually(func() error {
		suggestion := &suggestionsv1beta1.Suggestion{}
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, suggestion)
		suggestion.Status.Suggestions = newFakeSuggestion().Status.Suggestions
		errStatus := c.Status().Update(context.TODO(), suggestion)
		return errStatus
	}, timeout).ShouldNot(gomega.HaveOccurred())

	// Create the experiment
	g.Expect(c.Create(context.TODO(), instance)).NotTo(gomega.HaveOccurred())

	// Expect that experiment status is running
	experiment := &experimentsv1beta1.Experiment{}
	g.Eventually(func() bool {
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, experiment)
		return experiment.IsRunning()
	}, timeout).Should(gomega.BeTrue())

	// Expect that 2 trials are created, 1 should be deleted because ParallelTrialCount=2
	g.Eventually(func() int {
		trials := &trialsv1beta1.TrialList{}
		label := labels.Set{
			consts.LabelExperimentName: experimentName,
		}
		c.List(context.TODO(), &client.ListOptions{LabelSelector: label.AsSelector()}, trials)
		return len(trials.Items)
	}, timeout).Should(gomega.Equal(2))

	// Expect that suggestion status doesn't have first deleted trial
	// test-trial-1 must be deleted from suggestion status
	// UpdateSuggestionStatus with deleteTrialsCall call
	g.Eventually(func() bool {
		suggestion := &suggestionsv1beta1.Suggestion{}
		isDeleted := true
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, suggestion)
		for _, s := range suggestion.Status.Suggestions {
			if s.Name == trialName+"-1" {
				isDeleted = false
			}
		}
		return isDeleted
	}, timeout).Should(gomega.BeTrue())

	// Manually update experiment status to failed to make experiment completed
	// Expect that experiment is updated
	g.Eventually(func() error {
		experiment = &experimentsv1beta1.Experiment{}
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, experiment)
		experiment.MarkExperimentStatusFailed(experimentUtil.ExperimentMaxTrialsReachedReason, "Experiment is failed")
		return c.Status().Update(context.TODO(), experiment)
	}, timeout).ShouldNot(gomega.HaveOccurred())

	// Expect that suggestion with ResumePolicy = NeverResume is succeeded
	// UpdateSuggestionStatus with restartNoCall call
	g.Eventually(func() bool {
		suggestion := &suggestionsv1beta1.Suggestion{}
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, suggestion)
		return suggestion.IsSucceeded()
	}, timeout).Should(gomega.BeTrue())

	// Expect that suggestion with ResumePolicy = NeverResume is deleted
	g.Eventually(func() bool {
		// Delete the suggestion
		c.Delete(context.TODO(), suggestionRestartNo)
		return errors.IsNotFound(c.Get(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: experimentName}, &suggestionsv1beta1.Suggestion{}))
	}, timeout).Should(gomega.BeTrue())

	// Create the suggestion with ResumePolicy = FromVolume
	g.Expect(c.Create(context.TODO(), suggestionRestartYes)).NotTo(gomega.HaveOccurred())
	// Expect that suggestion is created
	g.Eventually(func() bool {
		return errors.IsNotFound(c.Get(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: experimentName}, &suggestionsv1beta1.Suggestion{}))
	}, timeout).ShouldNot(gomega.BeTrue())

	// Manually update suggestion ResumePolicy to FromVolume and mark experiment succeeded to test resume experiment.
	// Expect that suggestion spec is updated.
	g.Eventually(func() bool {
		experiment := &experimentsv1beta1.Experiment{}
		// Update ResumePolicy and maxTrialCount for resume
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, experiment)
		experiment.Spec.ResumePolicy = experimentsv1beta1.FromVolume
		var max int32 = 5
		experiment.Spec.MaxTrialCount = &max
		errUpdate := c.Update(context.TODO(), experiment)
		return errUpdate == nil
	}, timeout).Should(gomega.BeTrue())

	// Expect that experiment status is updated
	g.Eventually(func() bool {
		experiment := &experimentsv1beta1.Experiment{}
		// Update status to succeeded
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, experiment)
		experiment.MarkExperimentStatusSucceeded(experimentUtil.ExperimentMaxTrialsReachedReason, "Experiment is succeeded")
		errStatus := c.Status().Update(context.TODO(), experiment)
		return errStatus == nil
	}, timeout).Should(gomega.BeTrue())

	// Expect that experiment with FromVolume is restarting.
	// Experiment should be not succeeded and not failed.
	// UpdateSuggestionStatus with restartYesCall call and UpdateSuggestionStatus with experimentRestartingCall call.
	g.Eventually(func() bool {
		experiment := &experimentsv1beta1.Experiment{}
		c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: experimentName}, experiment)
		return experiment.IsRestarting() && !experiment.IsSucceeded() && !experiment.IsFailed()
	}, timeout).Should(gomega.BeTrue())

	// Expect that suggestion with ResumePolicy = FromVolume is deleted
	g.Eventually(func() bool {
		// Delete the suggestion
		c.Delete(context.TODO(), suggestionRestartYes)
		return errors.IsNotFound(c.Get(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: experimentName}, &suggestionsv1beta1.Suggestion{}))
	}, timeout).Should(gomega.BeTrue())

	// Expect that experiment is deleted
	g.Eventually(func() bool {
		// Delete the experiment
		c.Delete(context.TODO(), instance)
		return errors.IsNotFound(c.Get(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: experimentName}, &experimentsv1beta1.Experiment{}))
	}, timeout).Should(gomega.BeTrue())

	// Test 2 - Update status for empty experiment
	g.Expect(r.updateStatus(&experimentsv1beta1.Experiment{})).To(gomega.HaveOccurred())

	// Test 3 - Cleanup suggestion resources without deployed suggestion
	g.Expect(r.cleanupSuggestionResources(instance)).NotTo(gomega.HaveOccurred())
}

func newFakeInstance() *experimentsv1beta1.Experiment {
	var parallelCount int32 = 2
	var goal float64 = 99.9

	trialTemplateJob := &tfv1.TFJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubeflow.org/v1",
			Kind:       "TFJob",
		},
		Spec: tfv1.TFJobSpec{
			TFReplicaSpecs: map[tfv1.TFReplicaType]*kubeflowcommonv1.ReplicaSpec{
				tfv1.TFReplicaTypePS: {
					Replicas:      func() *int32 { i := int32(1); return &i }(),
					RestartPolicy: kubeflowcommonv1.RestartPolicyOnFailure,
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "tensorflow",
									Image: "gcr.io/kubeflow-ci/tf-mnist-with-summaries:1.0",
									Command: []string{
										"python",
										"/var/tf_mnist/mnist_with_summaries.py",
										"--log_dir=/train/metrics",
										"--lr=${trialParameters.learningRate}",
										"--num-layers=${trialParameters.numberLayers}",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	trialSpec, _ := util.ConvertObjectToUnstructured(trialTemplateJob)

	return &experimentsv1beta1.Experiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      experimentName,
			Namespace: namespace,
		},
		Spec: experimentsv1beta1.ExperimentSpec{
			ParallelTrialCount: &parallelCount,
			MaxTrialCount:      &parallelCount,
			Objective: &commonapiv1beta1.ObjectiveSpec{
				Type:                commonapiv1beta1.ObjectiveTypeMaximize,
				Goal:                &goal,
				ObjectiveMetricName: "accuracy",
			},
			Algorithm: &commonapiv1beta1.AlgorithmSpec{
				AlgorithmName: "random",
			},
			MetricsCollectorSpec: &commonapiv1beta1.MetricsCollectorSpec{
				Collector: &commonapiv1beta1.CollectorSpec{
					Kind: commonapiv1beta1.StdOutCollector,
				},
			},
			ResumePolicy: experimentsv1beta1.NeverResume,
			TrialTemplate: &experimentsv1beta1.TrialTemplate{
				TrialParameters: []experimentsv1beta1.TrialParameterSpec{
					{
						Name:        "learningRate",
						Description: "Learning Rate",
						Reference:   "lr",
					},
					{
						Name:        "numberLayers",
						Description: "Number of layers",
						Reference:   "num-layers",
					},
				},
				TrialSource: experimentsv1beta1.TrialSource{
					TrialSpec: trialSpec,
				},
			},
		},
	}
}

func newFakeSuggestion() *suggestionsv1beta1.Suggestion {
	return &suggestionsv1beta1.Suggestion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      experimentName,
			Namespace: namespace,
		},
		Spec: suggestionsv1beta1.SuggestionSpec{
			ResumePolicy: experimentsv1beta1.NeverResume,
		},
		Status: suggestionsv1beta1.SuggestionStatus{
			Suggestions: []suggestionsv1beta1.TrialAssignment{
				{
					Name: trialName + "-1",
					ParameterAssignments: []commonapiv1beta1.ParameterAssignment{
						{
							Name:  "lr",
							Value: "0.01",
						},
						{
							Name:  "num-layers",
							Value: "5",
						},
					},
				},
				{
					Name: trialName + "-2",
					ParameterAssignments: []commonapiv1beta1.ParameterAssignment{
						{
							Name:  "lr",
							Value: "0.01",
						},
						{
							Name:  "num-layers",
							Value: "5",
						},
					},
				},
				{
					Name: trialName + "-3",
					ParameterAssignments: []commonapiv1beta1.ParameterAssignment{
						{
							Name:  "lr",
							Value: "0.01",
						},
						{
							Name:  "num-layers",
							Value: "5",
						},
					},
				},
			},
		},
	}
}

func newFakeTFJob() *tfv1.TFJob {
	return &tfv1.TFJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubeflow.org/v1",
			Kind:       "TFJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trial-name",
			Namespace: "trial-namespace",
		},
		Spec: tfv1.TFJobSpec{
			TFReplicaSpecs: map[tfv1.TFReplicaType]*kubeflowcommonv1.ReplicaSpec{
				tfv1.TFReplicaTypePS: {
					Replicas:      func() *int32 { i := int32(1); return &i }(),
					RestartPolicy: kubeflowcommonv1.RestartPolicyOnFailure,
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "tensorflow",
									Image: "gcr.io/kubeflow-ci/tf-mnist-with-summaries:1.0",
									Command: []string{
										"python",
										"/var/tf_mnist/mnist_with_summaries.py",
										"--log_dir=/train/metrics",
										"--lr=0.01",
										"--num-layers=5",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
