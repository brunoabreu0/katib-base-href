package dlk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kubeflow/katib/pkg/api"
	"github.com/kubeflow/katib/pkg/db"
	dlkapi "github.com/kubeflow/katib/dlk/dlkmanager/api"
	"github.com/kubeflow/katib/dlk/dlkmanager/datastore"
	wIF "github.com/kubeflow/katib/pkg/manager/worker_interface"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type DlkWorkerInterface struct {
	RunningTrialList   map[string][]*api.Trial
	CompletedTrialList map[string][]*api.Trial
	dlkmanager         string
	namespace          string
	mux                *sync.Mutex
	dbIf               db.VizierDBInterface
}

func NewDlkWorkerInterface(s string, n string) *DlkWorkerInterface {
	return &DlkWorkerInterface{
		RunningTrialList:   make(map[string][]*api.Trial),
		CompletedTrialList: make(map[string][]*api.Trial),
		dlkmanager:         s,
		namespace:          n,
		mux:                new(sync.Mutex),
		dbIf:               db.New(),
	}
}

func (d *DlkWorkerInterface) getLt(tID string) (*datastore.LearningTaskInfo, error) {
	url := d.dlkmanager + "/learningTask/" + d.namespace + "/" + tID
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rs := &datastore.LearningTaskInfo{}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, rs)
	if err != nil {
		return nil, err
	}
	return rs, nil
}
func (d *DlkWorkerInterface) getLtLogs(tID string, stime string) (*datastore.LtLogInfo, error) {
	str := d.dlkmanager + "/learningTasks/logs/" + d.namespace + "/" + tID + "/worker"
	reqURL, err := url.Parse(str)
	if stime != "" {
		parameters := url.Values{}
		parameters.Add("sinceTime", stime)
		reqURL.RawQuery = parameters.Encode()

	}
	resp, err := http.Get(reqURL.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// get and decode response(json)
	rs := &datastore.LtLogInfo{}
	body, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, rs)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (d *DlkWorkerInterface) IsTrialComplete(studyId string, tID string) (bool, error) {
	lt, err := d.getLt(tID)
	if err != nil {
		return false, err
	} else if lt == nil {
		return false, nil
	} else if lt.State == "completed" {
		return true, nil
	}
	return false, nil
}

func (d *DlkWorkerInterface) storeTrialLog(tID string) error {
	var unformatted_logs []string
	ltlogs, _ := d.getLtLogs(tID, "")
	for _, pl := range ltlogs.PodLogs {
		for _, l := range pl.Logs {
			unformatted_logs = append(unformatted_logs,
				l.Time+" "+l.Value)
		}
	}
	err := d.dbIf.StoreTrialLogs(tID, unformatted_logs)
	return err
}

func (d *DlkWorkerInterface) GetTrialObjValue(studyId string, tID string, objname string) (string, error) {
	return wIF.GetTrialObjValue(d.dbIf, studyId, tID, objname)
}

func (d *DlkWorkerInterface) GetTrialEvLogs(studyId string, tID string, metrics []string, sinceTime string) ([]*api.EvaluationLog, error) {
	return wIF.GetTrialEvLogs(d.dbIf, studyId, tID, metrics, sinceTime)
}

func (d *DlkWorkerInterface) CheckRunningTrials(studyId string, objname string) error {
	d.mux.Lock()
	defer d.mux.Unlock()
	if len(d.RunningTrialList[studyId]) == 0 {
		return nil
	}
	sc, _ := d.dbIf.GetStudyConfig(studyId)
	metrics := sc.Metrics
	for _, t := range d.RunningTrialList[studyId] {
		status, err := d.dbIf.GetTrialStatus(t.TrialId)
		if err != nil {
			log.Printf("Error getting status of %s: %v", t.TrialId, err)
			continue
		}
		if status == api.TrialState_RUNNING {
			c, _ := d.IsTrialComplete(studyId, t.TrialId)
			err = d.storeTrialLog(t.TrialId)
			if err != nil {
				log.Printf("Error storing trial log of %s: %v", t.TrialId, err)
			}
			var es []*api.EvaluationLog
			if len(t.EvalLogs) == 0 {
				es, err = d.GetTrialEvLogs(studyId, t.TrialId, metrics, "")
			} else {
				es, err = d.GetTrialEvLogs(studyId, t.TrialId, metrics, t.EvalLogs[len(t.EvalLogs)-1].Time)
			}
			if err != nil {
				log.Printf("GetTrialEvLogs Err %v", err)
				return err
			}
			if len(es) > 0 {
				t.EvalLogs = append(t.EvalLogs, es...)
			}
			if c {
				o, _ := d.GetTrialObjValue(studyId, t.TrialId, objname)
				t.ObjectiveValue = o
				t.Status = api.TrialState_COMPLETED
				d.dbIf.UpdateTrial(t.TrialId, api.TrialState_COMPLETED)
				log.Printf("Trial %v is completed.", t.TrialId)
				log.Printf("Objective Value: %v", t.ObjectiveValue)
				d.CompletedTrialList[studyId] = append(d.CompletedTrialList[studyId], t)
				if len(d.RunningTrialList[studyId]) <= 1 {
					d.RunningTrialList[studyId] = []*api.Trial{}
				} else {
					tn := t.TrialId
					for j, tt := range d.RunningTrialList[studyId] {
						if tt.TrialId == tn {
							d.RunningTrialList[studyId] = append(d.RunningTrialList[studyId][:j], d.RunningTrialList[studyId][j+1:]...)
							break
						}
					}
				}
			}
		}
	}
	return nil
}
func (d *DlkWorkerInterface) convertTrialToManifest(trials []*api.Trial, studyId string) []*dlkapi.LTConfig {
	sc, _ := d.dbIf.GetStudyConfig(studyId)
	ret := make([]*dlkapi.LTConfig, len(trials))
	command := strings.Join(sc.Command, " ")
	d.mux.Lock()
	defer d.mux.Unlock()
	for i, t := range trials {
		d.RunningTrialList[studyId] = append(d.RunningTrialList[studyId], t)
		var param string
		for _, v := range t.ParameterSet {
			param += " " + v.Name + "=" + v.Value
		}
		e := []dlkapi.EnvConf{
			dlkapi.EnvConf{Name: "STUDY_ID", Value: studyId},
			dlkapi.EnvConf{Name: "TRIAL_ID", Value: t.TrialId},
		}
		c := strings.Replace(strings.Replace(command, "{{STUDY_ID}}", studyId, -1), "{{TRIAL_ID}}", t.TrialId, -1)
		var sched = "default-scheduler"
		if sc.Scheduler != "" {
			sched = sc.Scheduler
		}
		j := &dlkapi.LTConfig{
			Ns:          d.namespace,
			Scheduler:   sched,
			Name:        t.TrialId,
			NrPS:        0,
			NrWorker:    1,
			PsImage:     sc.Image,
			WorkerImage: sc.Image,
			Gpu:         int(sc.Gpu),
			DryRun:      false,
			EntryPoint:  c + param,
			Parameters:  "",
			Timeout:     0,
			Pvc:         sc.Mount.Pvc,
			MountPath:   sc.Mount.Path,
			Priority:    0,
			User:        sc.Owner,
			Envs:        e,
			PullSecret:  sc.PullSecret,
		}
		ret[i] = j
	}
	return ret
}
func (d *DlkWorkerInterface) SpawnWorkers(trials []*api.Trial, studyId string) error {
	runp := d.convertTrialToManifest(trials, studyId)
	url := fmt.Sprintf("%s/learningTask", d.dlkmanager)
	for _, j := range runp {
		//encode json
		b, err := json.Marshal(*j)
		if err != nil {
			return err
		}
		//send REST API Request
		resp, err := http.Post(url, "application/json", bytes.NewReader(b))
		if err != nil {
			return err
		}
		d.dbIf.UpdateTrial(j.Name, api.TrialState_RUNNING)
		resp.Body.Close()
		log.Printf("Created Lt %v.", j.Name)
	}
	return nil
}
func (d *DlkWorkerInterface) GetRunningTrials(studyId string) []*api.Trial {
	d.mux.Lock()
	defer d.mux.Unlock()
	return d.RunningTrialList[studyId]
}
func (d *DlkWorkerInterface) GetCompletedTrials(studyId string) []*api.Trial {
	d.mux.Lock()
	defer d.mux.Unlock()
	return d.CompletedTrialList[studyId]
}
func (d *DlkWorkerInterface) CleanWorkers(studyId string) error {
	url := fmt.Sprintf("%s/learningTasks/%s/", d.dlkmanager, d.namespace)
	d.mux.Lock()
	defer d.mux.Unlock()
	for _, t := range d.RunningTrialList[studyId] {
		req, err := http.NewRequest("DELETE", url+t.TrialId, nil)
		if err != nil {
			log.Printf("failed to create DELETE request: %s\n", err)
			return err
		}
		//send REST API Request
		_, err = http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
	}
	return nil
}
func (d *DlkWorkerInterface) CompleteTrial(studyId string, tID string, iscomplete bool) error {
	url := fmt.Sprintf("%s/learningTasks/%s/", d.dlkmanager, d.namespace)
	d.mux.Lock()
	defer d.mux.Unlock()
	if iscomplete {
		log.Printf("Trial %s completed", tID)
	} else {
		log.Printf("Trial %s has been killed", tID)
	}
	req, err := http.NewRequest("DELETE", url+tID, nil)
	if err != nil {
		log.Printf("failed to create DELETE request: %s\n", err)
		return err
	}
	//send REST API Request
	_, err = http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	for i, t := range d.RunningTrialList[studyId] {
		if t.TrialId == tID {
			d.RunningTrialList[studyId] = append(d.RunningTrialList[studyId][:i], d.RunningTrialList[studyId][i+1:]...)
			if iscomplete {
				t.Status = api.TrialState_COMPLETED
				d.dbIf.UpdateTrial(t.TrialId, api.TrialState_COMPLETED)
			} else {
				t.Status = api.TrialState_KILLED
				d.dbIf.UpdateTrial(t.TrialId, api.TrialState_KILLED)
			}
			d.CompletedTrialList[studyId] = append(d.CompletedTrialList[studyId], t)
		}
	}
	return nil
}
