package util

import (
	"reflect"
	"testing"

	commonv1 "github.com/kubeflow/tf-operator/pkg/apis/common/v1"
	tfv1 "github.com/kubeflow/tf-operator/pkg/apis/tensorflow/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	trialsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/trials/v1beta1"
	"github.com/kubeflow/katib/pkg/controller.v1beta1/util"
)

const (
	testMessage = "test-message"
	testReason  = "test-reason"
)

func TestGetDeployedJobStatus(t *testing.T) {

	successCondition := "status.conditions.#(type==\"Succeeded\")#|#(status==\"True\")#"
	failureCondition := "status.conditions.#(type==\"Failed\")#|#(status==\"True\")#"

	tcs := []struct {
		trial                  *trialsv1beta1.Trial
		deployedJob            *unstructured.Unstructured
		expectedTrialJobStatus *TrialJobStatus
		err                    bool
		testDescription        string
	}{
		{
			trial: newFakeTrial(successCondition, failureCondition),
			deployedJob: func() *unstructured.Unstructured {
				tfJob := newFakeTFJob()
				tfJob.Status.Conditions[0].Status = corev1.ConditionFalse
				tfJob.Status.Conditions[1].Status = corev1.ConditionFalse
				return newFakeDeployedJob(tfJob)
			}(),
			expectedTrialJobStatus: func() *TrialJobStatus {
				return &TrialJobStatus{
					Condition: JobRunning,
				}
			}(),
			err:             false,
			testDescription: "TFJob status is running",
		},
		{
			trial:       newFakeTrial(successCondition, failureCondition),
			deployedJob: newFakeDeployedJob(newFakeTFJob()),
			expectedTrialJobStatus: func() *TrialJobStatus {
				return &TrialJobStatus{
					Condition: JobSucceeded,
					Message:   testMessage,
					Reason:    testReason,
				}
			}(),
			err:             false,
			testDescription: "TFJob status is succeeded, reason and message must be returned",
		},
		{
			trial: newFakeTrial(successCondition, failureCondition),
			deployedJob: func() *unstructured.Unstructured {
				tfJob := newFakeTFJob()
				tfJob.Status.Conditions[0].Status = corev1.ConditionTrue
				tfJob.Status.Conditions[1].Status = corev1.ConditionFalse
				return newFakeDeployedJob(tfJob)
			}(),
			expectedTrialJobStatus: func() *TrialJobStatus {
				return &TrialJobStatus{
					Condition: JobFailed,
					Message:   testMessage,
					Reason:    testReason,
				}
			}(),
			err:             false,
			testDescription: "TFJob status is failed, reason and message must be returned",
		},
		{
			trial:       newFakeTrial("status.replicaStatuses.master.[@this].#(active==\"1\")", failureCondition),
			deployedJob: newFakeDeployedJob(newFakeTFJob()),
			expectedTrialJobStatus: func() *TrialJobStatus {
				return &TrialJobStatus{
					Condition: JobSucceeded,
				}
			}(),
			err:             false,
			testDescription: "TFJob status is succeeded because active = 1 in replica statuses",
		},
	}

	for _, tc := range tcs {
		actualTrialJobStatus, err := GetDeployedJobStatus(tc.trial, tc.deployedJob)

		if tc.err && err == nil {
			t.Errorf("Case: %v failed. Expected err, got nil", tc.testDescription)
		} else if !tc.err {
			if err != nil {
				t.Errorf("Case: %v failed. Expected nil, got %v", tc.testDescription, err)
			} else if !reflect.DeepEqual(tc.expectedTrialJobStatus, actualTrialJobStatus) {
				t.Errorf("Case: %v failed. Expected %v\n got %v", tc.testDescription, tc.expectedTrialJobStatus, actualTrialJobStatus)
			}
		}
	}
}

func newFakeTrial(successCondition, failureCondition string) *trialsv1beta1.Trial {
	return &trialsv1beta1.Trial{
		Spec: trialsv1beta1.TrialSpec{
			SuccessCondition: successCondition,
			FailureCondition: failureCondition,
		},
	}
}

func newFakeTFJob() *tfv1.TFJob {
	return &tfv1.TFJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-tfjob",
		},
		Status: commonv1.JobStatus{
			Conditions: []commonv1.JobCondition{
				{
					Type:    commonv1.JobFailed,
					Status:  corev1.ConditionFalse,
					Reason:  testReason,
					Message: testMessage,
				},
				{
					Type:    commonv1.JobSucceeded,
					Status:  corev1.ConditionTrue,
					Reason:  testReason,
					Message: testMessage,
				},
			},
			ReplicaStatuses: map[commonv1.ReplicaType]*commonv1.ReplicaStatus{
				"master": {
					Active: int32(1),
				},
			},
		},
	}
}
func newFakeDeployedJob(job interface{}) *unstructured.Unstructured {

	jobUnstructured, _ := util.ConvertObjectToUnstructured(job)
	return jobUnstructured
}
