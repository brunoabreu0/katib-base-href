package validator

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	commonv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/common/v1beta1"
	experimentsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/experiments/v1beta1"
	util "github.com/kubeflow/katib/pkg/controller.v1beta1/util"
	manifestmock "github.com/kubeflow/katib/pkg/mock/v1beta1/experiment/manifest"
	"github.com/kubeflow/katib/pkg/util/v1beta1/katibconfig"
)

func init() {
	logf.SetLogger(logf.ZapLogger(false))
}

func TestValidateExperiment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	p := manifestmock.NewMockGenerator(mockCtrl)
	g := New(p)

	suggestionConfigData := katibconfig.SuggestionConfig{}
	suggestionConfigData.Image = "algorithmImage"
	fakeNegativeInt := int32(-1)

	p.EXPECT().GetSuggestionConfigData(gomock.Any()).Return(suggestionConfigData, nil).AnyTimes()
	p.EXPECT().GetMetricsCollectorImage(gomock.Any()).Return("metricsCollectorImage", nil).AnyTimes()

	batchJobStr := convertBatchJobToString(newFakeBatchJob())
	p.EXPECT().GetTrialTemplate(gomock.Any()).Return(batchJobStr, nil).AnyTimes()

	tcs := []struct {
		Instance        *experimentsv1beta1.Experiment
		Err             bool
		oldInstance     *experimentsv1beta1.Experiment
		testDescription string
	}{
		//Objective
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Objective = nil
				return i
			}(),
			Err:             true,
			testDescription: "Objective is nil",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Objective.Type = commonv1beta1.ObjectiveTypeUnknown
				return i
			}(),
			Err:             true,
			testDescription: "Objective type is unknown",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Objective.ObjectiveMetricName = ""
				return i
			}(),
			Err:             true,
			testDescription: "Objective metric name is empty",
		},
		//Algorithm
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Algorithm = nil
				return i
			}(),
			Err:             true,
			testDescription: "Algorithm is nil",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Algorithm.AlgorithmName = ""
				return i
			}(),
			Err:             true,
			testDescription: "Algorithm name is empty",
		},
		// Valid Experiment
		{
			Instance:        newFakeInstance(),
			Err:             false,
			testDescription: "Run validator for correct experiment",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MaxFailedTrialCount = &fakeNegativeInt
				return i
			}(),
			Err:             true,
			testDescription: "Max failed trial count is negative",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MaxTrialCount = &fakeNegativeInt
				return i
			}(),
			Err:             true,
			testDescription: "Max trial count is negative",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.ParallelTrialCount = &fakeNegativeInt
				return i
			}(),
			Err:             true,
			testDescription: "Parallel trial count is negative",
		},
		// Valid Resume Experiment
		{
			Instance:        newFakeInstance(),
			Err:             false,
			oldInstance:     newFakeInstance(),
			testDescription: "Run validator to correct resume experiment",
		},
		{
			Instance: newFakeInstance(),
			Err:      true,
			oldInstance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Algorithm.AlgorithmName = "not-test"
				return i
			}(),
			testDescription: "Change algorithm name when resuming experiment",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.ResumePolicy = "invalid-policy"
				return i
			}(),
			Err:             true,
			testDescription: "Invalid resume policy",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Parameters = []experimentsv1beta1.ParameterSpec{}
				i.Spec.NasConfig = nil
				return i
			}(),
			Err:             true,
			testDescription: "Parameters and NAS config is nil",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.NasConfig = &experimentsv1beta1.NasConfig{
					Operations: []experimentsv1beta1.Operation{
						{
							OperationType: "op1",
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Parameters and NAS config is not nil",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialParameters[0].Name = "Invalid"
				return i
			}(),
			Err:             true,
			testDescription: "Invalid Trial parameter name",
		},
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.Parameters[1].FeasibleSpace.Max = "5"
				return i
			}(),
			Err:             true,
			testDescription: "Invalid feasible space in parameters",
		},
	}

	for _, tc := range tcs {
		err := g.ValidateExperiment(tc.Instance, tc.oldInstance)
		if !tc.Err && err != nil {
			t.Errorf("Case: %v failed. Expected nil, got %v", tc.testDescription, err)
		} else if tc.Err && err == nil {
			t.Errorf("Case: %v failed. Expected err, got nil", tc.testDescription)
		}
	}
}

func TestValidateParameters(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	p := manifestmock.NewMockGenerator(mockCtrl)
	g := New(p)

	tcs := []struct {
		parameters      []experimentsv1beta1.ParameterSpec
		err             bool
		testDescription string
	}{
		{
			parameters: func() []experimentsv1beta1.ParameterSpec {
				ps := newFakeInstance().Spec.Parameters
				ps[0].ParameterType = "invalid-type"
				return ps
			}(),
			err:             true,
			testDescription: "Invalid parameter type",
		},
		{
			parameters: func() []experimentsv1beta1.ParameterSpec {
				ps := newFakeInstance().Spec.Parameters
				ps[0].FeasibleSpace = experimentsv1beta1.FeasibleSpace{}
				return ps
			}(),
			err:             true,
			testDescription: "Feasible space is nil",
		},
		{
			parameters: func() []experimentsv1beta1.ParameterSpec {
				ps := newFakeInstance().Spec.Parameters
				ps[0].FeasibleSpace.List = []string{"invalid-list"}
				return ps
			}(),
			err:             true,
			testDescription: "Not empty list for int parameter type",
		},
		{
			parameters: func() []experimentsv1beta1.ParameterSpec {
				ps := newFakeInstance().Spec.Parameters
				ps[0].FeasibleSpace = experimentsv1beta1.FeasibleSpace{
					Max:  "",
					Min:  "",
					Step: "1",
				}
				return ps
			}(),
			err:             true,
			testDescription: "Empty max and min for int parameter type",
		},
		{
			parameters: func() []experimentsv1beta1.ParameterSpec {
				ps := newFakeInstance().Spec.Parameters
				ps[1].FeasibleSpace.Max = "1"
				return ps
			}(),
			err:             true,
			testDescription: "Not empty max for categorical parameter type",
		},
	}

	for _, tc := range tcs {
		err := g.(*DefaultValidator).validateParameters(tc.parameters)
		if !tc.err && err != nil {
			t.Errorf("Case: %v failed. Expected nil, got %v", tc.testDescription, err)
		} else if tc.err && err == nil {
			t.Errorf("Case: %v failed. Expected err, got nil", tc.testDescription)
		}
	}
}

func TestValidateTrialTemplate(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	p := manifestmock.NewMockGenerator(mockCtrl)
	g := New(p)

	validJobStr := convertBatchJobToString(newFakeBatchJob())

	missedParameterJob := newFakeBatchJob()
	missedParameterJob.Spec.Template.Spec.Containers[0].Command[2] = "--lr=${trialParameters.invalidParameter}"
	missedParameterJobStr := convertBatchJobToString(missedParameterJob)

	oddParameterJob := newFakeBatchJob()
	oddParameterJob.Spec.Template.Spec.Containers[0].Command = append(
		oddParameterJob.Spec.Template.Spec.Containers[0].Command,
		"--extra-parameter=${trialParameters.extraParameter}")
	oddParameterJobStr := convertBatchJobToString(oddParameterJob)

	invalidParameterJobStr := `apiVersion: batch/v1
kind: Job
spec:
  template:
    spec:
      containers:
        - name: fake-trial
          image: test-image
          command:
            - --invalidParameter={'num_layers': 2, 'input_sizes': [32, 32, 3]}
            - --lr=${trialParameters.learningRate}"
            - --num-layers=${trialParameters.numberLayers}`

	notEmptyMetadataJob := newFakeBatchJob()
	notEmptyMetadataJob.ObjectMeta = metav1.ObjectMeta{
		Name:      "trial-name",
		Namespace: "trial-namespace",
	}
	notEmptyMetadataStr := convertBatchJobToString(notEmptyMetadataJob)

	emptyAPIVersionJob := newFakeBatchJob()
	emptyAPIVersionJob.TypeMeta.APIVersion = ""
	emptyAPIVersionStr := convertBatchJobToString(emptyAPIVersionJob)

	invalidJobType := newFakeBatchJob()
	invalidJobType.TypeMeta.Kind = "InvalidKind"
	invalidJobTypeStr := convertBatchJobToString(invalidJobType)

	emptyConfigMap := p.EXPECT().GetTrialTemplate(gomock.Any()).Return("", errors.New(string(metav1.StatusReasonNotFound)))

	validTemplate1 := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(validJobStr, nil)
	validTemplate2 := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(validJobStr, nil)
	validTemplate3 := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(validJobStr, nil)
	validTemplate4 := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(validJobStr, nil)

	missedParameterTemplate := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(missedParameterJobStr, nil)
	oddParameterTemplate := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(oddParameterJobStr, nil)
	invalidParameterTemplate := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(invalidParameterJobStr, nil)
	notEmptyMetadataTemplate := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(notEmptyMetadataStr, nil)
	emptyAPIVersionTemplate := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(emptyAPIVersionStr, nil)
	invalidJobTypeTemplate := p.EXPECT().GetTrialTemplate(gomock.Any()).Return(invalidJobTypeStr, nil)

	gomock.InOrder(
		emptyConfigMap,
		validTemplate1,
		validTemplate2,
		validTemplate3,
		validTemplate4,
		missedParameterTemplate,
		oddParameterTemplate,
		invalidParameterTemplate,
		notEmptyMetadataTemplate,
		emptyAPIVersionTemplate,
		invalidJobTypeTemplate,
	)

	tcs := []struct {
		Instance        *experimentsv1beta1.Experiment
		Err             bool
		testDescription string
	}{
		// TrialParamters is nil
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialParameters = nil
				return i
			}(),
			Err:             true,
			testDescription: "Trial parameters is nil",
		},
		// TrialSpec and ConfigMap is nil
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialSpec = nil
				return i
			}(),
			Err:             true,
			testDescription: "Trial spec nil",
		},
		// TrialSpec and ConfigMap is not nil
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialSource.ConfigMap = &experimentsv1beta1.ConfigMapSource{
					ConfigMapName: "config-map-name",
				}
				return i
			}(),
			Err:             true,
			testDescription: "Trial spec and ConfigMap is not nil",
		},
		// ConfigMap missed template path
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialSource = experimentsv1beta1.TrialSource{
					ConfigMap: &experimentsv1beta1.ConfigMapSource{
						ConfigMapName:      "config-map-name",
						ConfigMapNamespace: "config-map-namespace",
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Missed template path in ConfigMap",
		},
		// Wrong path in configMap
		// emptyConfigMap case
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialSpec = nil
				i.Spec.TrialTemplate.TrialSource = experimentsv1beta1.TrialSource{
					ConfigMap: &experimentsv1beta1.ConfigMapSource{
						ConfigMapName:      "config-map-name",
						ConfigMapNamespace: "config-map-namespace",
						TemplatePath:       "wrong-path",
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Wrong template path in ConfigMap",
		},
		// Empty Reference or Name in trialParameters
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialParameters[0].Reference = ""
				return i
			}(),
			Err:             true,
			testDescription: "Empty reference or name in Trial parameters",
		},
		// Wrong Name in trialParameters
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialParameters[0].Name = "{invalid-name}"
				return i
			}(),
			Err:             true,
			testDescription: "Wrong name in Trial parameters",
		},
		// Duplicate Name in trialParameters
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialParameters[1].Name = i.Spec.TrialTemplate.TrialParameters[0].Name
				return i
			}(),
			Err:             true,
			testDescription: "Duplicate name in Trial parameters",
		},
		// Duplicate Reference in trialParameters
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialParameters[1].Reference = i.Spec.TrialTemplate.TrialParameters[0].Reference
				return i
			}(),
			Err:             true,
			testDescription: "Duplicate reference in Trial parameters",
		},
		// Trial Template doesn't contain parameter from trialParameters
		// missedParameterTemplate case
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				return i
			}(),
			Err:             true,
			testDescription: "Trial template doesn't contain parameter from Trial parameters",
		},
		// Trial Template contains extra parameter
		// oddParameterTemplate case
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				return i
			}(),
			Err:             true,
			testDescription: "Trial template contains extra parameter",
		},
		// Trial Template parameter is invalid after substitution
		// Unable convert string to unstructured
		// invalidParameterTemplate case
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				return i
			}(),
			Err:             true,
			testDescription: "Trial template is unable to convert to unstructured after substitution",
		},
		// Trial Template contains Name and Namespace
		// notEmptyMetadataTemplate case
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.TrialTemplate.TrialSpec.SetName("trial-name")
				i.Spec.TrialTemplate.TrialSpec.SetNamespace("trial-namespace")
				return i
			}(),
			Err:             true,
			testDescription: "Trial template contains metadata.name or metadata.namespace",
		},
		// Trial Template doesn't contain APIVersion or Kind
		// emptyAPIVersionTemplate case
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				return i
			}(),
			Err:             true,
			testDescription: "Trial template doesn't contain APIVersion or Kind",
		},
		// Trial Template has invalid Kind
		// invalidJobTypeTemplate case
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				return i
			}(),
			Err:             true,
			testDescription: "Trial template has invalid Kind",
		},
	}
	for _, tc := range tcs {
		err := g.(*DefaultValidator).validateTrialTemplate(tc.Instance)
		if !tc.Err && err != nil {
			t.Errorf("Case: %v failed. Expected nil, got %v", tc.testDescription, err)
		} else if tc.Err && err == nil {
			t.Errorf("Case: %v failed. Expected err, got nil", tc.testDescription)
		}
	}
}

func TestValidateSupportedJob(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	p := manifestmock.NewMockGenerator(mockCtrl)
	g := New(p)

	invalidFieldBatchJob := `apiVersion: batch/v1
kind: Job
spec:
  template:
    spec:
      containers:
        name: container-must-be-list`

	invalidFieldBatchJobUnstr, err := util.ConvertStringToUnstructured(invalidFieldBatchJob)
	if err != nil {
		t.Errorf("ConvertStringToUnstructured failed: %v", err)
	}

	invalidStructureBatchJob := `apiVersion: batch/v1
kind: Job
spec:
  template:
    invalidSpec: not-job-format
    spec:
      containers:
        - name: invalid-list`

	invalidStructureBatchJobUnstr, err := util.ConvertStringToUnstructured(invalidStructureBatchJob)
	if err != nil {
		t.Errorf("ConvertStringToUnstructured failed: %v", err)
	}

	invalidFieldTFJob := `apiVersion: kubeflow.org/v1
kind: TFJob
spec:
  tfReplicaSpecs:
    Worker: InvalidWorker`

	invalidFieldTFJobUnstr, err := util.ConvertStringToUnstructured(invalidFieldTFJob)
	if err != nil {
		t.Errorf("ConvertStringToUnstructured failed: %v", err)
	}

	invalidStructureTFJob := `apiVersion: kubeflow.org/v1
kind: TFJob
spec:
  tfReplicaSpecs:
    Worker:
      replicas: 2
    InvalidWorker:
      InvalidContainer:
        - Name: invalidName1`

	invalidStructureTFJobUnstr, err := util.ConvertStringToUnstructured(invalidStructureTFJob)
	if err != nil {
		t.Errorf("ConvertStringToUnstructured failed: %v", err)
	}

	invalidFieldPyTorchJob := `apiVersion: kubeflow.org/v1
kind: PyTorchJob
spec:
  pytorchReplicaSpecs:
    Master: InvalidMaster`

	invalidFieldPyTorchJobUnstr, err := util.ConvertStringToUnstructured(invalidFieldPyTorchJob)
	if err != nil {
		t.Errorf("ConvertStringToUnstructured failed: %v", err)
	}

	invalidStructurePyTorchJob := `apiVersion: kubeflow.org/v1
kind: PyTorchJob
spec:
  pytorchReplicaSpecs:
    Master:
      template:
        spec:
          containers:
            - name: pytorch
            - invalidName: invalidName`

	invalidStructurePyTorchJobUnstr, err := util.ConvertStringToUnstructured(invalidStructurePyTorchJob)
	if err != nil {
		t.Errorf("ConvertStringToUnstructured failed: %v", err)
	}

	notDefaultResourceBatchJob := `apiVersion: batch/v1
kind: Job
spec:
  template:
    spec:
      containers:
        - resources:
            limits:
              nvidia.com/gpu: 1
            requests:
              nvidia.com/gpu: 1`

	notDefaultResourceBatchUnstr, err := util.ConvertStringToUnstructured(notDefaultResourceBatchJob)
	if err != nil {
		t.Errorf("ConvertStringToUnstructured failed: %v", err)
	}

	tcs := []struct {
		RunSpec         *unstructured.Unstructured
		Err             bool
		testDescription string
	}{
		// Invalid Field Batch Job
		{
			RunSpec:         invalidFieldBatchJobUnstr,
			Err:             true,
			testDescription: "Trial template has invalid Batch Job parameter",
		},
		// Invalid Structure Batch Job
		// Try to patch new runSpec with old Trial template
		// Patch must have only "remove" operations
		// Then all parameters from trial Template were correctly merged
		{
			RunSpec:         invalidStructureBatchJobUnstr,
			Err:             true,
			testDescription: "Trial template has invalid Batch Job structure",
		},
		// Invalid Field TF Job
		{
			RunSpec:         invalidFieldTFJobUnstr,
			Err:             true,
			testDescription: "Trial template has invalid TF Job parameter",
		},
		// Invalid Structure TF Job
		{
			RunSpec:         invalidStructureTFJobUnstr,
			Err:             true,
			testDescription: "Trial template has invalid TF Job structure",
		},
		// Invalid Field PyTorch Job
		{
			RunSpec:         invalidFieldPyTorchJobUnstr,
			Err:             true,
			testDescription: "Trial template has invalid PyTorch Job parameter",
		},
		// Invalid Structure PyTorch Job
		{
			RunSpec:         invalidStructurePyTorchJobUnstr,
			Err:             true,
			testDescription: "Trial template has invalid PyTorch Job structure",
		},
		// Valid case with not default Kubernetes resource (nvidia.com/gpu: 1)
		{
			RunSpec:         notDefaultResourceBatchUnstr,
			Err:             false,
			testDescription: "Valid case with nvidia.com/gpu resource in Trial template",
		},
	}

	for _, tc := range tcs {
		err := g.(*DefaultValidator).validateSupportedJob(tc.RunSpec)
		if !tc.Err && err != nil {
			t.Errorf("Case: %v failed. Expected nil, got %v", tc.testDescription, err)
		} else if tc.Err && err == nil {
			t.Errorf("Case: %v failed. Expected err, got nil", tc.testDescription)
		}
	}

}

func TestValidateMetricsCollector(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	p := manifestmock.NewMockGenerator(mockCtrl)
	g := New(p)

	p.EXPECT().GetMetricsCollectorImage(gomock.Any()).Return("metricsCollectorImage", nil).AnyTimes()

	tcs := []struct {
		Instance        *experimentsv1beta1.Experiment
		Err             bool
		testDescription string
	}{
		// Invalid Metrics Collector Kind
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.CollectorKind("invalid-kind"),
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Invalid metrics collector Kind",
		},
		// FileCollector invalid Path
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.FileCollector,
					},
					Source: &commonv1beta1.SourceSpec{
						FileSystemPath: &commonv1beta1.FileSystemPath{
							Path: "not/absolute/path",
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Invalid path for File metrics collector",
		},
		// TfEventCollector invalid Path
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.TfEventCollector,
					},
					Source: &commonv1beta1.SourceSpec{
						FileSystemPath: &commonv1beta1.FileSystemPath{
							Path: "not/absolute/path",
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Invalid path for TF event metrics collector",
		},
		// PrometheusMetricCollector invalid Port
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.PrometheusMetricCollector,
					},
					Source: &commonv1beta1.SourceSpec{
						HttpGet: &v1.HTTPGetAction{
							Port: intstr.IntOrString{
								StrVal: "Port",
							},
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Invalid port for Prometheus metrics collector",
		},
		// PrometheusMetricCollector invalid Path
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.PrometheusMetricCollector,
					},
					Source: &commonv1beta1.SourceSpec{
						HttpGet: &v1.HTTPGetAction{
							Port: intstr.IntOrString{
								IntVal: 8888,
							},
							Path: "not/valid/path",
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Invalid path for Prometheus metrics collector",
		},
		//  CustomCollector empty CustomCollector
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.CustomCollector,
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Empty container for Custom metrics collector",
		},
		//  CustomCollector invalid Path
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.CustomCollector,
						CustomCollector: &v1.Container{
							Name: "my-collector",
						},
					},
					Source: &commonv1beta1.SourceSpec{
						FileSystemPath: &commonv1beta1.FileSystemPath{
							Path: "not/absolute/path",
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Invalid path for Custom metrics collector",
		},
		// FileMetricCollector invalid regexp in metrics format
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.FileCollector,
					},
					Source: &commonv1beta1.SourceSpec{
						Filter: &commonv1beta1.FilterSpec{
							MetricsFormat: []string{
								"[",
							},
						},
						FileSystemPath: &commonv1beta1.FileSystemPath{
							Path: "/absolute/path",
							Kind: commonv1beta1.FileKind,
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "Invalid metrics format regex for File metrics collector",
		},
		// FileMetricCollector one subexpression in metrics format
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.FileCollector,
					},
					Source: &commonv1beta1.SourceSpec{
						Filter: &commonv1beta1.FilterSpec{
							MetricsFormat: []string{
								"{metricName: ([\\w|-]+)}",
							},
						},
						FileSystemPath: &commonv1beta1.FileSystemPath{
							Path: "/absolute/path",
							Kind: commonv1beta1.FileKind,
						},
					},
				}
				return i
			}(),
			Err:             true,
			testDescription: "One subexpression in metrics format",
		},
		// Valid FileMetricCollector
		{
			Instance: func() *experimentsv1beta1.Experiment {
				i := newFakeInstance()
				i.Spec.MetricsCollectorSpec = &commonv1beta1.MetricsCollectorSpec{
					Collector: &commonv1beta1.CollectorSpec{
						Kind: commonv1beta1.FileCollector,
					},
					Source: &commonv1beta1.SourceSpec{
						FileSystemPath: &commonv1beta1.FileSystemPath{
							Path: "/absolute/path",
							Kind: commonv1beta1.FileKind,
						},
					},
				}
				return i
			}(),
			Err:             false,
			testDescription: "Run validator for correct File metrics collector",
		},
	}

	for _, tc := range tcs {
		err := g.(*DefaultValidator).validateMetricsCollector(tc.Instance)
		if !tc.Err && err != nil {
			t.Errorf("Case: %v failed. Expected nil, got %v", tc.testDescription, err)
		} else if tc.Err && err == nil {
			t.Errorf("Case: %v failed. Expected err, got nil", tc.testDescription)
		}
	}

}

func newFakeInstance() *experimentsv1beta1.Experiment {
	goal := 0.11
	return &experimentsv1beta1.Experiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake",
			Namespace: "fakens",
		},
		Spec: experimentsv1beta1.ExperimentSpec{
			MetricsCollectorSpec: &commonv1beta1.MetricsCollectorSpec{
				Collector: &commonv1beta1.CollectorSpec{
					Kind: commonv1beta1.StdOutCollector,
				},
			},
			Objective: &commonv1beta1.ObjectiveSpec{
				Type:                commonv1beta1.ObjectiveTypeMaximize,
				Goal:                &goal,
				ObjectiveMetricName: "testme",
			},
			Algorithm: &commonv1beta1.AlgorithmSpec{
				AlgorithmName: "test",
				AlgorithmSettings: []commonv1beta1.AlgorithmSetting{
					{
						Name:  "test1",
						Value: "value1",
					},
				},
			},
			Parameters: []experimentsv1beta1.ParameterSpec{
				{
					ParameterType: experimentsv1beta1.ParameterTypeInt,
					FeasibleSpace: experimentsv1beta1.FeasibleSpace{
						Max: "5",
						Min: "1",
					},
				},
				{
					ParameterType: experimentsv1beta1.ParameterTypeCategorical,
					FeasibleSpace: experimentsv1beta1.FeasibleSpace{
						List: []string{"1", "2", "3"},
					},
				},
			},
			TrialTemplate: newFakeTrialTemplate(newFakeBatchJob(), newFakeTrialParamters()),
		},
	}
}

func newFakeBatchJob() *batchv1.Job {

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "training-container",
							Image: "docker.io/kubeflowkatib/mxnet-mnist",
							Command: []string{
								"python3",
								"/opt/mxnet-mnist/mnist.py",
								"--lr=${trialParameters.learningRate}",
								"--num-layers=${trialParameters.numberLayers}",
							},
						},
					},
				},
			},
		},
	}
}

func newFakeTrialParamters() []experimentsv1beta1.TrialParameterSpec {
	return []experimentsv1beta1.TrialParameterSpec{
		{
			Name:        "learningRate",
			Description: "Learning rate",
			Reference:   "lr",
		},
		{
			Name:        "numberLayers",
			Description: "Number of layers",
			Reference:   "num-layers",
		},
	}
}

func newFakeTrialTemplate(trialJob interface{}, trialParameters []experimentsv1beta1.TrialParameterSpec) *experimentsv1beta1.TrialTemplate {

	trialSpec, err := util.ConvertObjectToUnstructured(trialJob)
	if err != nil {
		log.Error(err, "ConvertObjectToUnstructured error")
	}

	return &experimentsv1beta1.TrialTemplate{
		TrialSource: experimentsv1beta1.TrialSource{
			TrialSpec: trialSpec,
		},
		TrialParameters: trialParameters,
	}
}

func convertBatchJobToString(batchJob *batchv1.Job) string {

	batchJobUnstr, err := util.ConvertObjectToUnstructured(batchJob)
	if err != nil {
		log.Error(err, "ConvertObjectToUnstructured error")
	}

	batchJobStr, err := util.ConvertUnstructuredToString(batchJobUnstr)
	if err != nil {
		log.Error(err, "ConvertUnstructuredToString error")
	}

	return batchJobStr
}
