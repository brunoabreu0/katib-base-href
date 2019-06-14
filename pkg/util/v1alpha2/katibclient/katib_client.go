/*

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

package katibclient

import (
	"context"
	"io/ioutil"
	"strings"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	experimentsv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/experiment/v1alpha2"
	trialsv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/trial/v1alpha2"
	"github.com/kubeflow/katib/pkg/controller/v1alpha2/consts"
)

type Client interface {
	InjectClient(c client.Client)
	GetExperimentList(namespace ...string) (*experimentsv1alpha2.ExperimentList, error)
	CreateExperiment(experiment *experimentsv1alpha2.Experiment, namespace ...string) error
	DeleteExperiment(experiment *experimentsv1alpha2.Experiment, namespace ...string) error
	GetExperiment(name string, namespace ...string) (*experimentsv1alpha2.Experiment, error)
	GetConfigMap(name string, namespace ...string) (map[string]string, error)
	GetTrialList(name string, namespace ...string) (*trialsv1alpha2.TrialList, error)
	GetTrialTemplates(namespace ...string) (map[string]string, error)
	UpdateTrialTemplates(newTrialTemplates map[string]string, namespace ...string) error
	GetMetricsCollectorTemplates(namespace ...string) (map[string]string, error)
	UpdateMetricsCollectorTemplates(newMCTemplates map[string]string, namespace ...string) error
}

type KatibClient struct {
	client client.Client
}

func NewWithGivenClient(c client.Client) Client {
	return &KatibClient{
		client: c,
	}
}

func NewClient(options client.Options) (Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	experimentsv1alpha2.AddToScheme(scheme.Scheme)
	trialsv1alpha2.AddToScheme(scheme.Scheme)
	cl, err := client.New(cfg, options)
	return &KatibClient{
		client: cl,
	}, nil
}

func (k *KatibClient) InjectClient(c client.Client) {
	k.client = c
}

func (k *KatibClient) GetExperimentList(namespace ...string) (*experimentsv1alpha2.ExperimentList, error) {
	ns := getNamespace(namespace...)
	expList := &experimentsv1alpha2.ExperimentList{}
	listOpt := client.InNamespace(ns)

	if err := k.client.List(context.Background(), listOpt, expList); err != nil {
		return expList, err
	}
	return expList, nil

}

func (k *KatibClient) GetTrialList(name string, namespace ...string) (*trialsv1alpha2.TrialList, error) {
	ns := getNamespace(namespace...)
	trialList := &trialsv1alpha2.TrialList{}
	labels := map[string]string{consts.LabelExperimentName: name}
	listOpt := &client.ListOptions{}
	listOpt.MatchingLabels(labels).InNamespace(ns)

	if err := k.client.List(context.Background(), listOpt, trialList); err != nil {
		return trialList, err
	}
	return trialList, nil

}

func (k *KatibClient) CreateExperiment(experiment *experimentsv1alpha2.Experiment, namespace ...string) error {

	if err := k.client.Create(context.Background(), experiment); err != nil {
		return err
	}
	return nil
}

func (k *KatibClient) DeleteExperiment(experiment *experimentsv1alpha2.Experiment, namespace ...string) error {

	if err := k.client.Delete(context.Background(), experiment); err != nil {
		return err
	}
	return nil
}

func (k *KatibClient) GetExperiment(name string, namespace ...string) (*experimentsv1alpha2.Experiment, error) {
	ns := getNamespace(namespace...)
	exp := &experimentsv1alpha2.Experiment{}
	if err := k.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: ns}, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

func (k *KatibClient) GetConfigMap(name string, namespace ...string) (map[string]string, error) {
	ns := getNamespace(namespace...)
	configMap := &apiv1.ConfigMap{}
	if err := k.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: ns}, configMap); err != nil {
		return map[string]string{}, err
	}
	return configMap.Data, nil
}

func (k *KatibClient) GetTrialTemplates(namespace ...string) (map[string]string, error) {

	ns := getNamespace(namespace...)
	trialTemplates := &apiv1.ConfigMap{}

	if err := k.client.Get(context.Background(), types.NamespacedName{Name: experimentsv1alpha2.DefaultTrialConfigMapName, Namespace: ns}, trialTemplates); err != nil {
		return nil, err
	}
	return trialTemplates.Data, nil

}

func (k *KatibClient) UpdateTrialTemplates(newTrialTemplates map[string]string, namespace ...string) error {
	ns := getNamespace(namespace...)
	trialTemplates := &apiv1.ConfigMap{}

	if err := k.client.Get(context.Background(), types.NamespacedName{Name: experimentsv1alpha2.DefaultTrialConfigMapName, Namespace: ns}, trialTemplates); err != nil {
		return err
	}
	trialTemplates.Data = newTrialTemplates

	if err := k.client.Update(context.Background(), trialTemplates); err != nil {
		return err
	}
	return nil
}

func (k *KatibClient) GetMetricsCollectorTemplates(namespace ...string) (map[string]string, error) {
	ns := getNamespace(namespace...)
	return k.GetConfigMap(experimentsv1alpha2.DefaultMetricsCollectorConfigMapName, ns)
}

func (k *KatibClient) UpdateMetricsCollectorTemplates(newMCTemplates map[string]string, namespace ...string) error {

	ns := getNamespace(namespace...)
	metricsCollectorTemplates := &apiv1.ConfigMap{}

	if err := k.client.Get(context.Background(), types.NamespacedName{Name: experimentsv1alpha2.DefaultMetricsCollectorConfigMapName, Namespace: ns}, metricsCollectorTemplates); err != nil {
		return err
	}

	metricsCollectorTemplates.Data = newMCTemplates

	if err := k.client.Update(context.Background(), metricsCollectorTemplates); err != nil {
		return err
	}
	return nil
}

func getNamespace(namespace ...string) string {
	if len(namespace) == 0 {
		data, _ := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		return strings.TrimSpace(string(data))
	}
	return namespace[0]
}
