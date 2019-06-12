package trial

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	commonv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/common/v1alpha2"
	trialsv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/trial/v1alpha2"
	api_pb "github.com/kubeflow/katib/pkg/api/v1alpha2"
	managerclientmock "github.com/kubeflow/katib/pkg/mock/v1alpha2/trial/managerclient"
)

const (
	trialName = "foo"
	namespace = "default"

	timeout = time.Second * 40
)

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: trialName, Namespace: namespace}}
var expectedResult = reconcile.Result{Requeue: true}
var tfJobKey = types.NamespacedName{Name: "test", Namespace: namespace}

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestCreateTFJobTrial(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := newFakeTrialWithTFJob()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mc := managerclientmock.NewMockManagerClient(mockCtrl)
	mc.EXPECT().CreateTrialInDB(gomock.Any()).Return(nil).AnyTimes()
	mc.EXPECT().UpdateTrialStatusInDB(gomock.Any()).Return(nil).AnyTimes()

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c := mgr.GetClient()

	recFn := SetupTestReconcile(&ReconcileTrial{
		Client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		ManagerClient: mc,
		updateStatusHandler: func(instance *trialsv1alpha2.Trial) error {
			if !instance.IsCreated() {
				t.Errorf("Expected got condition created")
			}
			return nil
		},
	})
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Trial object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(c.Delete(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
}

func TestReconcileTFJobTrial(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := newFakeTrialWithTFJob()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mc := managerclientmock.NewMockManagerClient(mockCtrl)
	mc.EXPECT().CreateTrialInDB(gomock.Any()).Return(nil).AnyTimes()
	mc.EXPECT().UpdateTrialStatusInDB(gomock.Any()).Return(nil).AnyTimes()
	mc.EXPECT().GetTrialObservationLog(gomock.Any()).Return(&api_pb.GetObservationLogReply{
		ObservationLog: nil,
	}, nil).AnyTimes()

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c := mgr.GetClient()

	r := &ReconcileTrial{
		Client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		ManagerClient: mc,
	}

	r.updateStatusHandler = func(instance *trialsv1alpha2.Trial) error {
		if !instance.IsCreated() {
			t.Errorf("Expected got condition created")
		}
		return r.updateStatus(instance)
	}

	recFn := SetupTestReconcile(r)
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Trial object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())

	tfJob := &unstructured.Unstructured{}
	bufSize := 1024
	buf := bytes.NewBufferString(instance.Spec.RunSpec)
	if err := k8syaml.NewYAMLOrJSONDecoder(buf, bufSize).Decode(tfJob); err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
	g.Eventually(func() error { return c.Get(context.TODO(), tfJobKey, tfJob) }, timeout).
		Should(gomega.Succeed())

	// Delete the TFJob and expect Reconcile to be called for TFJob deletion
	g.Expect(c.Delete(context.TODO(), tfJob)).NotTo(gomega.HaveOccurred())
	g.Eventually(func() error { return c.Get(context.TODO(), tfJobKey, tfJob) }, timeout).
		Should(gomega.Succeed())

	// Manually delete TFJob since GC isn't enabled in the test control plane
	g.Eventually(func() error { return c.Delete(context.TODO(), tfJob) }, timeout).
		Should(gomega.MatchError("tfjobs.kubeflow.org \"test\" not found"))
	g.Expect(c.Delete(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
}

func TestReconcileCompletedTFJobTrial(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := newFakeTrialWithTFJob()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mc := managerclientmock.NewMockManagerClient(mockCtrl)
	mc.EXPECT().CreateTrialInDB(gomock.Any()).Return(nil).AnyTimes()
	mc.EXPECT().UpdateTrialStatusInDB(gomock.Any()).Return(nil).AnyTimes()
	mc.EXPECT().GetTrialObservationLog(gomock.Any()).Return(&api_pb.GetObservationLogReply{
		ObservationLog: nil,
	}, nil).AnyTimes()

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c := mgr.GetClient()

	r := &ReconcileTrial{
		Client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		ManagerClient: mc,
	}

	r.updateStatusHandler = func(instance *trialsv1alpha2.Trial) error {
		if !instance.IsCreated() {
			t.Errorf("Expected got condition created")
		}
		return r.updateStatus(instance)
	}

	recFn := SetupTestReconcile(r)
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Trial object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)

	g.Eventually(func() error {
		return c.Get(context.TODO(), expectedRequest.NamespacedName, instance)
	}, timeout).
		Should(gomega.Succeed())
	instance.MarkTrialStatusSucceeded("", "")
	g.Expect(c.Status().Update(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
	g.Eventually(func() bool {
		err := c.Get(context.TODO(), expectedRequest.NamespacedName, instance)
		if err == nil && instance.IsCompleted() {
			return true
		}
		return false
	}, timeout).
		Should(gomega.BeTrue())
}

func TestFailedToCreateTrialInDB(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := newFakeTrialWithTFJob()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mc := managerclientmock.NewMockManagerClient(mockCtrl)
	expectedErr := fmt.Errorf("test")
	mc.EXPECT().CreateTrialInDB(gomock.Any()).Return(expectedErr).AnyTimes()

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c := mgr.GetClient()

	r := &ReconcileTrial{
		Client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		ManagerClient: mc,
	}

	r.updateStatusHandler = r.updateStatus

	recFn := SetupTestReconcile(r)
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Trial object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(func() error {
		return c.Get(context.TODO(), expectedRequest.NamespacedName, instance)
	}, timeout).
		Should(gomega.Succeed())
	if instance.IsCreated() {
		t.Errorf("Expected not to got condition created")
	}
	g.Expect(c.Delete(context.TODO(), instance)).NotTo(gomega.HaveOccurred())
}

func newFakeTrialWithTFJob() *trialsv1alpha2.Trial {
	objectiveSpec := commonv1alpha2.ObjectiveSpec{ObjectiveMetricName: "test"}
	t := &trialsv1alpha2.Trial{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trialName,
			Namespace: namespace,
		},
		Spec: trialsv1alpha2.TrialSpec{
			Objective: &objectiveSpec,
			MetricsCollectorSpec: `apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: test
  namespace: default
spec:
  schedule: "*/1 * * * *"
  successfulJobsHistoryLimit: 0
  failedJobsHistoryLimit: 1
  jobTemplate:
    spec:
      backoffLimit: 0
      template:
        spec:
          serviceAccountName: metrics-collector
          containers:
          - name: test
            image: katib/metrics-collector
            args:
            - "./metricscollector.v1alpha2"
            - "-e"
            - "teste"
            - "-t"
            - "test"
            - "-k"
            - "TFJob"
            - "-n"
            - "default"
            - "-m"
            - "test"
            - "-mn"
            - "test"
          restartPolicy: Never`,
			RunSpec: `apiVersion: "kubeflow.org/v1"
kind: "TFJob"
metadata:
  name: "test"
  namespace: "default"
spec:
  tfReplicaSpecs:
    PS:
      replicas: 2
      restartPolicy: Never
      template:
        spec:
          containers:
            - name: tensorflow
              image: kubeflow/tf-dist-mnist-test:1.0
    Worker:
      replicas: 4
      restartPolicy: Never
      template:
        spec:
          containers:
            - name: tensorflow
              image: kubeflow/tf-dist-mnist-test:1.0
`,
		},
	}
	return t
}
