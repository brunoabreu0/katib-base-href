# V1beta1ObjectiveSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**additional_metric_names** | **list[str]** | This can be empty if we only care about the objective metric. Note: If we adopt a push instead of pull mechanism, this can be omitted completely. | [optional] 
**goal** | **float** |  | [optional] 
**metric_strategies** | [**list[V1beta1MetricStrategy]**](V1beta1MetricStrategy.md) | This field is allowed to missing, experiment defaulter (webhook) will fill it. | [optional] 
**objective_metric_name** | **str** |  | [optional] 
**type** | **str** |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


