package managerclient

import (
	"database/sql"

	commonapiv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/common/v1alpha2"
	experimentsv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/experiment/v1alpha2"
	api_pb "github.com/kubeflow/katib/pkg/api/v1alpha2"
	commonv1alpha2 "github.com/kubeflow/katib/pkg/common/v1alpha2"
)

// ManagerClient is the interface for katib manager client
// in experiment controller.
type ManagerClient interface {
	CreateExperimentInDB(instance *experimentsv1alpha2.Experiment) error
	DeleteExperimentInDB(instance *experimentsv1alpha2.Experiment) error
	UpdateExperimentStatusInDB(instance *experimentsv1alpha2.Experiment) error
	GetExperimentFromDB(instance *experimentsv1alpha2.Experiment) (
		*api_pb.GetExperimentReply, error)
}

// DefaultClient implements the Client interface.
type DefaultClient struct {
}

// New creates a new ManagerClient.
func New() ManagerClient {
	return &DefaultClient{}
}

func (d *DefaultClient) CreateExperimentInDB(instance *experimentsv1alpha2.Experiment) error {
	experiment := getExperimentConf(instance)
	request := &api_pb.RegisterExperimentRequest{
		Experiment: experiment,
	}
	if _, err := commonv1alpha2.RegisterExperiment(request); err != nil {
		return err
	}
	return nil
}

func (d *DefaultClient) DeleteExperimentInDB(instance *experimentsv1alpha2.Experiment) error {
	request := &api_pb.DeleteExperimentRequest{
		ExperimentName: instance.Name,
	}
	if _, err := commonv1alpha2.DeleteExperiment(request); err != nil {
		return err
	}
	return nil
}

func (d *DefaultClient) UpdateExperimentStatusInDB(instance *experimentsv1alpha2.Experiment) error {
	newStatus := &api_pb.ExperimentStatus{
		StartTime:      commonv1alpha2.ConvertTime2RFC3339(instance.Status.StartTime),
		CompletionTime: commonv1alpha2.ConvertTime2RFC3339(instance.Status.CompletionTime),
		Condition:      getCondition(instance),
	}
	request := &api_pb.UpdateExperimentStatusRequest{
		NewStatus:      newStatus,
		ExperimentName: instance.Name,
	}
	if _, err := commonv1alpha2.UpdateExperimentStatus(request); err != nil {
		return err
	}
	return nil
}

func (d *DefaultClient) GetExperimentFromDB(instance *experimentsv1alpha2.Experiment) (*api_pb.GetExperimentReply, error) {
	return nil, sql.ErrNoRows
}

func getExperimentConf(instance *experimentsv1alpha2.Experiment) *api_pb.Experiment {
	experiment := &api_pb.Experiment{
		Spec: &api_pb.ExperimentSpec{
			Objective: &api_pb.ObjectiveSpec{
				AdditionalMetricNames: []string{},
			},
			Algorithm: &api_pb.AlgorithmSpec{
				AlgorithmSetting: []*api_pb.AlgorithmSetting{},
			},
		},
		Status: &api_pb.ExperimentStatus{
			StartTime:      commonv1alpha2.ConvertTime2RFC3339(instance.Status.StartTime),
			CompletionTime: commonv1alpha2.ConvertTime2RFC3339(instance.Status.CompletionTime),
			Condition:      getCondition(instance),
		},
	}

	experiment.Name = instance.Name

	//Populate Objective
	switch instance.Spec.Objective.Type {
	case commonapiv1alpha2.ObjectiveTypeMaximize:
		experiment.Spec.Objective.Type = api_pb.ObjectiveType_MAXIMIZE
	case commonapiv1alpha2.ObjectiveTypeMinimize:
		experiment.Spec.Objective.Type = api_pb.ObjectiveType_MINIMIZE
	default:
		experiment.Spec.Objective.Type = api_pb.ObjectiveType_UNKNOWN

	}
	experiment.Spec.Objective.Goal = float32(*instance.Spec.Objective.Goal)
	experiment.Spec.Objective.ObjectiveMetricName = instance.Spec.Objective.ObjectiveMetricName
	for _, m := range instance.Spec.Objective.AdditionalMetricNames {
		experiment.Spec.Objective.AdditionalMetricNames = append(experiment.Spec.Objective.AdditionalMetricNames, m)
	}

	//Populate Algorithm Spec
	experiment.Spec.Algorithm.AlgorithmName = instance.Spec.Algorithm.AlgorithmName

	for _, as := range instance.Spec.Algorithm.AlgorithmSettings {
		experiment.Spec.Algorithm.AlgorithmSetting = append(
			experiment.Spec.Algorithm.AlgorithmSetting,
			&api_pb.AlgorithmSetting{
				Name:  as.Name,
				Value: as.Value,
			})
	}

	//Populate HP Experiment
	if instance.Spec.Parameters != nil {
		parameterSpecs := &api_pb.ExperimentSpec_ParameterSpecs{
			Parameters: []*api_pb.ParameterSpec{},
		}
		for _, p := range instance.Spec.Parameters {
			parameter := &api_pb.ParameterSpec{
				FeasibleSpace: &api_pb.FeasibleSpace{},
			}
			parameter.Name = p.Name
			parameter.FeasibleSpace.Min = p.FeasibleSpace.Min
			parameter.FeasibleSpace.Max = p.FeasibleSpace.Max
			parameter.FeasibleSpace.List = p.FeasibleSpace.List
			parameter.FeasibleSpace.Step = p.FeasibleSpace.Step

			switch p.ParameterType {
			case experimentsv1alpha2.ParameterTypeCategorical:
				parameter.ParameterType = api_pb.ParameterType_CATEGORICAL
			case experimentsv1alpha2.ParameterTypeDiscrete:
				parameter.ParameterType = api_pb.ParameterType_DISCRETE
			case experimentsv1alpha2.ParameterTypeDouble:
				parameter.ParameterType = api_pb.ParameterType_DOUBLE
			case experimentsv1alpha2.ParameterTypeInt:
				parameter.ParameterType = api_pb.ParameterType_INT
			case experimentsv1alpha2.ParameterTypeUnknown:
				parameter.ParameterType = api_pb.ParameterType_UNKNOWN_TYPE
			}
			parameterSpecs.Parameters = append(parameterSpecs.Parameters, parameter)
		}
		experiment.Spec.ParameterSpecs = parameterSpecs
	}

	//Populate NAS Experiment
	if instance.Spec.NasConfig != nil {

		nasConfig := &api_pb.NasConfig{
			GraphConfig: &api_pb.GraphConfig{},
			Operations: &api_pb.NasConfig_Operations{
				Operation: []*api_pb.Operation{},
			},
		}

		nasConfig.GraphConfig.NumLayers = *instance.Spec.NasConfig.GraphConfig.NumLayers

		for _, i := range instance.Spec.NasConfig.GraphConfig.InputSizes {
			nasConfig.GraphConfig.InputSizes = append(nasConfig.GraphConfig.InputSizes, i)
		}

		for _, o := range instance.Spec.NasConfig.GraphConfig.OutputSizes {
			nasConfig.GraphConfig.OutputSizes = append(nasConfig.GraphConfig.OutputSizes, o)
		}

		for _, op := range instance.Spec.NasConfig.Operations {
			operation := &api_pb.Operation{
				ParameterSpecs: &api_pb.Operation_ParameterSpecs{
					Parameters: []*api_pb.ParameterSpec{},
				},
			}

			operation.OperationType = op.OperationType

			for _, p := range op.Parameters {
				parameter := &api_pb.ParameterSpec{
					FeasibleSpace: &api_pb.FeasibleSpace{},
				}
				parameter.Name = p.Name
				parameter.FeasibleSpace.Min = p.FeasibleSpace.Min
				parameter.FeasibleSpace.Max = p.FeasibleSpace.Max
				parameter.FeasibleSpace.List = p.FeasibleSpace.List
				parameter.FeasibleSpace.Step = p.FeasibleSpace.Step

				switch p.ParameterType {
				case experimentsv1alpha2.ParameterTypeCategorical:
					parameter.ParameterType = api_pb.ParameterType_CATEGORICAL
				case experimentsv1alpha2.ParameterTypeDiscrete:
					parameter.ParameterType = api_pb.ParameterType_DISCRETE
				case experimentsv1alpha2.ParameterTypeDouble:
					parameter.ParameterType = api_pb.ParameterType_DOUBLE
				case experimentsv1alpha2.ParameterTypeInt:
					parameter.ParameterType = api_pb.ParameterType_INT
				case experimentsv1alpha2.ParameterTypeUnknown:
					parameter.ParameterType = api_pb.ParameterType_UNKNOWN_TYPE
				}
				operation.ParameterSpecs.Parameters = append(operation.ParameterSpecs.Parameters, parameter)
			}
			nasConfig.Operations.Operation = append(nasConfig.Operations.Operation, operation)
		}

		experiment.Spec.NasConfig = nasConfig
	}

	return experiment

}

func getCondition(inst *experimentsv1alpha2.Experiment) api_pb.ExperimentStatus_ExperimentConditionType {
	condition, _ := inst.GetLastConditionType()
	switch condition {
	case experimentsv1alpha2.ExperimentCreated:
		return api_pb.ExperimentStatus_CREATED
	case experimentsv1alpha2.ExperimentRunning:
		return api_pb.ExperimentStatus_RUNNING
	case experimentsv1alpha2.ExperimentRestarting:
		return api_pb.ExperimentStatus_RESTARTING
	case experimentsv1alpha2.ExperimentSucceeded:
		return api_pb.ExperimentStatus_SUCCEEDED
	case experimentsv1alpha2.ExperimentFailed:
		return api_pb.ExperimentStatus_FAILED
	default:
		return api_pb.ExperimentStatus_UNKNOWN
	}
}
