/*
Copyright 2019 The Kubernetes Authors.

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

package experiment

import (
	"context"
	"net/http"

	experimentsv1alpha2 "github.com/kubeflow/katib/pkg/api/operators/apis/experiment/v1alpha2"
	"github.com/kubeflow/katib/pkg/controller/v1alpha2/experiment/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// experimentValidator validates Pods
type experimentValidator struct {
	client  client.Client
	decoder types.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = &experimentValidator{}

func (v *experimentValidator) Handle(ctx context.Context, req types.Request) types.Response {
	inst := &experimentsv1alpha2.Experiment{}
	err := v.decoder.Decode(req, inst)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	err = util.ValidateExperiment(inst)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	return admission.ValidationResponse(true, "")
}

// experimentValidator implements inject.Client.
// A client will be automatically injected.
var _ inject.Client = &experimentValidator{}

// InjectClient injects the client.
func (v *experimentValidator) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

// experimentValidator implements inject.Decoder.
// A decoder will be automatically injected.
var _ inject.Decoder = &experimentValidator{}

// InjectDecoder injects the decoder.
func (v *experimentValidator) InjectDecoder(d types.Decoder) error {
	v.decoder = d
	return nil
}
