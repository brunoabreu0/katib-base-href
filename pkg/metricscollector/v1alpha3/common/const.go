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
package common

import (
	"time"

	v1alpha3common "github.com/kubeflow/katib/pkg/apis/controller/common/v1alpha3"
)

const (
	DefaultPollInterval = time.Second
	DefaultTimeout      = 0
	DefaultWaitAll      = false

	MetricCollectorContainerName       = "metrics-collector"
	MetricLoggerCollectorContainerName = "metrics-logger-and-collector"

	TrainingCompleted = "completed"

	DefaultFilter = `([\w|-]+)\s*=\s*((-?\d+)(\.\d+)?)`
)

var (
	AutoInjectMetricsCollecterList = [...]v1alpha3common.CollectorKind{
		v1alpha3common.StdOutCollector,
		v1alpha3common.TfEventCollector,
		v1alpha3common.FileCollector,
		v1alpha3common.PrometheusMetricCollector,
	}
)
