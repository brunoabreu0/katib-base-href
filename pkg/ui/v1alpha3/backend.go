package ui

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"

	experimentv1alpha3 "github.com/kubeflow/katib/pkg/apis/controller/experiments/v1alpha3"
	trialsv1alpha3 "github.com/kubeflow/katib/pkg/apis/controller/trials/v1alpha3"
	api_pb_v1alpha3 "github.com/kubeflow/katib/pkg/apis/manager/v1alpha3"
	common_v1alpha3 "github.com/kubeflow/katib/pkg/common/v1alpha3"
	"github.com/kubeflow/katib/pkg/controller.v1alpha3/consts"
	"github.com/kubeflow/katib/pkg/util/v1alpha3/katibclient"
)

func NewKatibUIHandler() *KatibUIHandler {
	kclient, err := katibclient.NewClient(client.Options{})
	if err != nil {
		log.Printf("NewClient for Katib failed: %v", err)
		panic(err)
	}
	return &KatibUIHandler{
		katibClient: kclient,
	}
}

func (k *KatibUIHandler) connectManager() (*grpc.ClientConn, api_pb_v1alpha3.ManagerClient) {
	conn, err := grpc.Dial(common_v1alpha3.ManagerAddr, grpc.WithInsecure())
	if err != nil {
		log.Printf("Dial to GRPC failed: %v", err)
		return nil, nil
	}
	c := api_pb_v1alpha3.NewManagerClient(conn)
	return conn, c
}

func (k *KatibUIHandler) FetchHPJobs(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)

	jobs := make([]JobView, 0)

	el, err := k.katibClient.GetExperimentList()
	if err != nil {
		log.Printf("GetExperimentList for HP failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, experiment := range el.Items {
		if experiment.Spec.Parameters != nil {
			experimentLastCondition, err := experiment.GetLastConditionType()
			if err != nil {
				log.Printf("GetLastConditionType for HP failed: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jobs = append(jobs, JobView{
				Name:   experiment.Name,
				Status: string(experimentLastCondition),
			})
		}
	}

	response, err := json.Marshal(jobs)
	if err != nil {
		log.Printf("Marshal HP jobs failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(response)

}

func (k *KatibUIHandler) FetchNASJobs(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)

	jobs := make([]JobView, 0)

	el, err := k.katibClient.GetExperimentList()
	if err != nil {
		log.Printf("GetExperimentList for NAS failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, experiment := range el.Items {
		if experiment.Spec.NasConfig != nil {
			experimentLastCondition, err := experiment.GetLastConditionType()
			if err != nil {
				log.Printf("GetLastConditionType for HP failed: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jobs = append(jobs, JobView{
				Name:   experiment.Name,
				Status: string(experimentLastCondition),
			})
		}
	}

	response, err := json.Marshal(jobs)
	if err != nil {
		log.Printf("Marshal NAS jobs failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(response)

}

func (k *KatibUIHandler) SubmitYamlJob(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)
	var data map[string]interface{}

	json.NewDecoder(r.Body).Decode(&data)

	job := experimentv1alpha3.Experiment{}
	if yamlContent, ok := data["yaml"].(string); ok {
		err := yaml.Unmarshal([]byte(yamlContent), &job)
		if err != nil {
			log.Printf("Unmarshal YAML content failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = k.katibClient.CreateExperiment(&job)
		if err != nil {
			log.Printf("CreateExperiment from YAML failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

}

func (k *KatibUIHandler) SubmitParamsJob(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)
	var data map[string]interface{}

	json.NewDecoder(r.Body).Decode(&data)
	if data, ok := data["postData"]; ok {
		jsonbody, err := json.Marshal(data)
		if err != nil {
			log.Printf("Marshal data for HP job failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		job := experimentv1alpha3.Experiment{}
		if err := json.Unmarshal(jsonbody, &job); err != nil {
			log.Printf("Unmarshal HP job failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dataMap := data.(map[string]interface{})
		job.TypeMeta = metav1.TypeMeta{
			APIVersion: "kubeflow.org/v1alpha3",
			Kind:       "Experiment",
		}
		job.ObjectMeta = metav1.ObjectMeta{
			Name:      dataMap["metadata"].(map[string]interface{})["name"].(string),
			Namespace: dataMap["metadata"].(map[string]interface{})["namespace"].(string),
		}
		err = k.katibClient.CreateExperiment(&job)
		if err != nil {
			log.Printf("CreateExperiment for HP failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (k *KatibUIHandler) DeleteExperiment(w http.ResponseWriter, r *http.Request) {
	experimentName := r.URL.Query()["experimentName"][0]
	experiment, err := k.katibClient.GetExperiment(experimentName)
	if err != nil {
		log.Printf("GetExperiment failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = k.katibClient.DeleteExperiment(experiment)
	if err != nil {
		log.Printf("DeleteExperiment failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (k *KatibUIHandler) FetchHPJobInfo(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)
	experimentName := r.URL.Query()["experimentName"][0]

	conn, c := k.connectManager()
	defer conn.Close()

	resultText := "trialName"
	experiment, err := k.katibClient.GetExperiment(experimentName)
	if err != nil {
		log.Printf("GetExperiment from HP job failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Got Experiment")
	metricsList := map[string]int{}
	metricsName := experiment.Spec.Objective.ObjectiveMetricName
	resultText += "," + metricsName
	metricsList[metricsName] = 0
	for i, m := range experiment.Spec.Objective.AdditionalMetricNames {
		resultText += "," + m
		metricsList[m] = i + 1
	}
	log.Printf("Got metrics names")
	paramList := map[string]int{}
	for i, p := range experiment.Spec.Parameters {
		resultText += "," + p.Name
		paramList[p.Name] = i + len(metricsList)
	}
	log.Printf("Got Parameters names")

	trialList, err := k.katibClient.GetTrialList(experimentName)
	if err != nil {
		log.Printf("GetTrialList from HP job failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Got Trial List")

	for _, t := range trialList.Items {
		succeeded := false
		for _, condition := range t.Status.Conditions {
			if condition.Type == trialsv1alpha3.TrialSucceeded {
				succeeded = true
			}
		}
		if succeeded {
			obsLogResp, err := c.GetObservationLog(
				context.Background(),
				&api_pb_v1alpha3.GetObservationLogRequest{
					TrialName: t.Name,
					StartTime: "",
					EndTime:   "",
				},
			)
			if err != nil {
				log.Printf("GetObservationLog from HP job failed: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			trialResText := make([]string, len(metricsList)+len(paramList))
			for _, m := range obsLogResp.ObservationLog.MetricLogs {
				trialResText[metricsList[m.Metric.Name]] = m.Metric.Value

			}
			for _, trialParam := range t.Spec.ParameterAssignments {
				trialResText[paramList[trialParam.Name]] = trialParam.Value
			}
			resultText += "\n" + t.Name + "," + strings.Join(trialResText, ",")
		}
	}
	log.Printf("Logs parsed, results:\n %v", resultText)
	response, err := json.Marshal(resultText)
	if err != nil {
		log.Printf("Marshal result text for HP job failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(response)

}

func (k *KatibUIHandler) FetchHPJobTrialInfo(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)
	trialName := r.URL.Query()["trialName"][0]
	conn, c := k.connectManager()
	defer conn.Close()

	resultText := "metricName,time,value\n"
	obsLogResp, err := c.GetObservationLog(
		context.Background(),
		&api_pb_v1alpha3.GetObservationLogRequest{
			TrialName: trialName,
			StartTime: "",
			EndTime:   "",
		},
	)
	if err != nil {
		log.Printf("GetObservationLog failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	prevTime := ""
	for _, m := range obsLogResp.ObservationLog.MetricLogs {
		parsedTime, _ := time.Parse(time.RFC3339Nano, m.TimeStamp)
		formatTime := parsedTime.Format("2006-01-02T15:4:5")
		if formatTime != prevTime {
			resultText += m.Metric.Name + "," + formatTime + "," + m.Metric.Value + "\n"
			prevTime = formatTime
		}
	}

	response, err := json.Marshal(resultText)
	if err != nil {
		log.Printf("Marshal result text in Trial info failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(response)
}

func (k *KatibUIHandler) FetchNASJobInfo(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)
	experimentName := r.URL.Query()["experimentName"][0]

	responseRaw := make([]NNView, 0)
	var architecture string
	var decoder string

	conn, c := k.connectManager()

	defer conn.Close()

	trials, err := k.katibClient.GetTrialList(experimentName)
	if err != nil {
		log.Printf("GetTrialList from NAS job failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Got Trial List")

	for i, t := range trials.Items {
		succeeded := false
		for _, condition := range t.Status.Conditions {
			if condition.Type == trialsv1alpha3.TrialSucceeded {
				succeeded = true
			}
		}
		if succeeded {
			obsLogResp, err := c.GetObservationLog(
				context.Background(),
				&api_pb_v1alpha3.GetObservationLogRequest{
					TrialName: t.Name,
					StartTime: "",
					EndTime:   "",
				},
			)
			if err != nil {
				log.Printf("GetObservationLog from NAS job failed: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			metricsName := make([]string, 0)
			metricsValue := make([]string, 0)
			for _, m := range obsLogResp.ObservationLog.MetricLogs {
				metricsName = append(metricsName, m.Metric.Name)
				metricsValue = append(metricsValue, m.Metric.Value)

			}
			for _, trialParam := range t.Spec.ParameterAssignments {
				if trialParam.Name == "architecture" {
					architecture = trialParam.Value
				}
				if trialParam.Name == "nn_config" {
					decoder = trialParam.Value
				}
			}
			responseRaw = append(responseRaw, NNView{
				Name:         "Generation " + strconv.Itoa(i),
				TrialName:    t.Name,
				Architecture: generateNNImage(architecture, decoder),
				MetricsName:  metricsName,
				MetricsValue: metricsValue,
			})
		}
	}
	log.Printf("Logs parsed, result: %v", responseRaw)

	response, err := json.Marshal(responseRaw)
	if err != nil {
		log.Printf("Marshal result in NAS job failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(response)
}

// FetchTrialTemplates gets the trial templates for the given namespace.
func (k *KatibUIHandler) FetchTrialTemplates(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)
	namespace := r.URL.Query()["namespace"][0]
	if namespace == "" {
		namespace = consts.DefaultKatibNamespace
	}
	trialTemplates, err := k.katibClient.GetTrialTemplates(namespace)
	if err != nil {
		log.Printf("GetTrialTemplate failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(getTemplatesView(trialTemplates))
	if err != nil {
		log.Printf("Marshal templates failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(response)
}

func (k *KatibUIHandler) AddEditDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	//enableCors(&w)
	//TODO: need to delete?
	if r.Method == "OPTIONS" {
		return
	}
	var data map[string]interface{}
	var err error
	var templateResponse TemplateResponse

	json.NewDecoder(r.Body).Decode(&data)
	if data["action"].(string) == "delete" {
		templateResponse, err = k.updateTemplates(data, true)
	} else {
		templateResponse, err = k.updateTemplates(data, false)
	}
	if err != nil {
		log.Printf("updateTemplates failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(templateResponse)
	if err != nil {
		log.Printf("Marhal failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(response)
}
