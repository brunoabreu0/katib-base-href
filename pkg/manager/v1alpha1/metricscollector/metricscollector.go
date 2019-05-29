package metricscollector

import (
	"errors"
	"fmt"
	"strings"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog"

	api "github.com/kubeflow/katib/pkg/api/v1alpha1"
	"github.com/kubeflow/katib/pkg/controller/v1alpha1/studyjob"
)

type MetricsCollector struct {
	clientset *kubernetes.Clientset
}

func NewMetricsCollector() (*MetricsCollector, error) {
	config, err := restclient.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &MetricsCollector{
		clientset: clientset,
	}, nil

}

func (d *MetricsCollector) CollectWorkerLog(wID string, wkind string, objectiveValueName string, metrics []string, namespace string) (*api.MetricsLogSet, error) {
	labelMap := make(map[string]string)
	if wkind == studyjob.TFJobWorker {
		labelMap["tf-job-name"] = wID
		labelMap["tf-job-role"] = "master"
	} else if wkind == studyjob.PyTorchJobWorker {
		labelMap["pytorch-job-name"] = wID
		labelMap["pytorch-job-role"] = "master"
	} else {
		labelMap["job-name"] = wID
	}
	pl, err := d.clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: labels.Set(labelMap).String(), IncludeUninitialized: true})
	if err != nil {
		return nil, err
	}
	if len(pl.Items) == 0 {
		return nil, errors.New(fmt.Sprintf("No Pods are found in Job %v", wID))
	}
	logopt := apiv1.PodLogOptions{Timestamps: true}
	logs, err := d.clientset.CoreV1().Pods(namespace).GetLogs(pl.Items[0].ObjectMeta.Name, &logopt).Do().Raw()
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return &api.MetricsLogSet{}, nil
	}
	mls, err := d.parseLogs(wID, strings.Split(string(logs), "\n"), objectiveValueName, metrics)
	return mls, err
}

func (d *MetricsCollector) parseLogs(wID string, logs []string, objectiveValueName string, metrics []string) (*api.MetricsLogSet, error) {
	var lasterr error
	ret := &api.MetricsLogSet{
		WorkerId: wID,
	}
	mlogs := make(map[string]*api.MetricsLog)
	mlogs[objectiveValueName] = &api.MetricsLog{
		Name: objectiveValueName,
	}
	for _, m := range metrics {
		if m != objectiveValueName {
			mlogs[m] = &api.MetricsLog{
				Name: m,
			}
		}
	}
	for _, logline := range logs {
		if logline == "" {
			continue
		}
		ls := strings.SplitN(logline, " ", 2)
		if len(ls) != 2 {
			klog.Errorf("Error parsing log: %s", logline)
			lasterr = errors.New("Error parsing log")
			continue
		}
		_, err := time.Parse(time.RFC3339Nano, ls[0])
		if err != nil {
			klog.Errorf("Error parsing time %s: %v", ls[0], err)
			lasterr = err
			continue
		}
		kvpairs := strings.Fields(ls[1])
		for _, kv := range kvpairs {
			v := strings.Split(kv, "=")
			if len(v) > 2 {
				klog.Infof("Ignoring trailing garbage: %s", kv)
			}
			if len(v) == 1 {
				continue
			}
			metricsName := ""
			if v[0] == objectiveValueName {
				metricsName = v[0]
			} else {
				for _, m := range metrics {
					if v[0] == m {
						metricsName = v[0]
					}
				}
			}
			if metricsName == "" {
				continue
			}
			mlogs[metricsName].Values = append(mlogs[metricsName].Values, &api.MetricsValueTime{
				Time:  ls[0],
				Value: v[1],
			})
		}
	}
	for _, ml := range mlogs {
		ret.MetricsLogs = append(ret.MetricsLogs, ml)
	}
	if lasterr != nil {
		return ret, lasterr
	}
	return ret, nil
}
