/*
Copyright 2018 The Kubeflow Authors

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

/*
MetricsCollector is a default metricscollector for worker.
It will collect metrics from pod log.
You should print metrics in {{MetricsName}}={{MetricsValue}} format.
For example, the objective value name is F1 and the metrics are loss, your training code should print like below.
     ---
     epoch 1:
     batch1 loss=0.8
     batch2 loss=0.6

     F1=0.4

     epoch 2:
     batch1 loss=0.4
     batch2 loss=0.2

     F1=0.7
     ---
The metrics collector will collect all logs of metrics.
*/

package main

import (
	"context"
	"flag"

	api "github.com/kubeflow/katib/pkg/api/v1alpha1"
	"github.com/kubeflow/katib/pkg/manager/v1alpha1/metricscollector"
	"k8s.io/klog"

	"google.golang.org/grpc"
)

var studyID = flag.String("s", "", "Study ID")
var trialID = flag.String("t", "", "Trial ID")
var workerID = flag.String("w", "", "Worker ID")
var workerKind = flag.String("k", "", "Worker Kind")
var namespace = flag.String("n", "", "NameSpace")
var managerSerivce = flag.String("m", "", "Vizier Manager service")

func main() {
	flag.Parse()
	klog.Infof("Study ID: %s, Trial ID: %s, Worker ID: %s, Worker Kind: %s", *studyID, *trialID, *workerID, *workerKind)
	conn, err := grpc.Dial(*managerSerivce, grpc.WithInsecure())
	if err != nil {
		klog.Fatalf("could not connect: %v", err)
	}
	defer conn.Close()
	c := api.NewManagerClient(conn)
	mc, err := metricscollector.NewMetricsCollector()
	if err != nil {
		klog.Fatalf("Failed to create MetricsCollector: %v", err)
	}
	ctx := context.Background()
	screq := &api.GetStudyRequest{
		StudyId: *studyID,
	}
	screp, err := c.GetStudy(ctx, screq)
	if err != nil {
		klog.Fatalf("Failed to GetStudyConf: %v", err)
	}
	mls, err := mc.CollectWorkerLog(*workerID, *workerKind, screp.StudyConfig.ObjectiveValueName, screp.StudyConfig.Metrics, *namespace)
	if err != nil {
		klog.Fatalf("Failed to collect logs: %v", err)
	}
	rmreq := &api.ReportMetricsLogsRequest{
		StudyId:        *studyID,
		MetricsLogSets: []*api.MetricsLogSet{mls},
	}
	_, err = c.ReportMetricsLogs(ctx, rmreq)
	if err != nil {
		klog.Fatalf("Failed to Report logs: %v", err)
	}
	klog.Infof("Metrics reported. :\n%v", mls)
	return
}
