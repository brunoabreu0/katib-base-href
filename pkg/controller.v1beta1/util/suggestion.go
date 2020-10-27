package util

import (
	"fmt"

	suggestionsv1beta1 "github.com/kubeflow/katib/pkg/apis/controller/suggestions/v1beta1"
	"github.com/kubeflow/katib/pkg/controller.v1beta1/consts"
)

// GetSuggestionDeploymentName returns name for the suggestion's deployment
func GetSuggestionDeploymentName(s *suggestionsv1beta1.Suggestion) string {
	return s.Name + "-" + s.Spec.Algorithm.AlgorithmName
}

// GetSuggestionServiceName returns name for the suggestion's service
func GetSuggestionServiceName(s *suggestionsv1beta1.Suggestion) string {
	return s.Name + "-" + s.Spec.Algorithm.AlgorithmName
}

// GetSuggestionPersistentVolumeName returns name for the suggestion's PV
func GetSuggestionPersistentVolumeName(s *suggestionsv1beta1.Suggestion) string {
	return s.Name + "-" + s.Spec.Algorithm.AlgorithmName + "-" + s.Namespace
}

// GetSuggestionPersistentVolumeClaimName returns name for the suggestion's PVC
func GetSuggestionPersistentVolumeClaimName(s *suggestionsv1beta1.Suggestion) string {
	return s.Name + "-" + s.Spec.Algorithm.AlgorithmName
}

// GetSuggestionRBACName returns name for the suggestion's ServiceAccount, Role and RoleBinding
func GetSuggestionRBACName(s *suggestionsv1beta1.Suggestion) string {
	return s.Name + "-" + s.Spec.Algorithm.AlgorithmName
}

// GetAlgorithmEndpoint returns the endpoint of the Suggestion service with HP or NAS algorithm
func GetAlgorithmEndpoint(s *suggestionsv1beta1.Suggestion) string {
	serviceName := GetSuggestionServiceName(s)
	return fmt.Sprintf("%s.%s:%d",
		serviceName,
		s.Namespace,
		consts.DefaultSuggestionPort)
}

// GetEarlyStoppingEndpoint returns the endpoint of the EarlyStopping service
func GetEarlyStoppingEndpoint(s *suggestionsv1beta1.Suggestion) string {
	serviceName := GetSuggestionServiceName(s)
	return fmt.Sprintf("%s.%s:%d",
		serviceName,
		s.Namespace,
		consts.DefaultEarlyStoppingPort)
}
