package main

import (
	"flag"
	"fmt"
	"net/http"

	ui "github.com/kubeflow/katib/pkg/ui/v1alpha2"
)

var (
	port, host, buildDir *string
)

func init() {
	port = flag.String("port", "80", "the port to listen to for incoming HTTP connections")
	host = flag.String("host", "0.0.0.0", "the host to listen to for incoming HTTP connections")
	buildDir = flag.String("build-dir", "/app/build", "the dir of frontend")
}
func main() {
	flag.Parse()
	kuh := ui.NewKatibUIHandler()

	frontend := http.FileServer(http.Dir(*buildDir))
	http.Handle("/katib/", http.StripPrefix("/katib/", frontend))

	http.HandleFunc("/katib/fetch_hp_jobs/", kuh.FetchHPJobs)
	http.HandleFunc("/katib/fetch_nas_jobs/", kuh.FetchNASJobs)
	http.HandleFunc("/katib/submit_yaml/", kuh.SubmitYamlJob)
	http.HandleFunc("/katib/submit_hp_job/", kuh.SubmitParamsJob)
	http.HandleFunc("/katib/submit_nas_job/", kuh.SubmitParamsJob)

	//TODO: Add it in Katib client
	http.HandleFunc("/katib/delete_experiment/", kuh.DeleteExperiment)

	http.HandleFunc("/katib/fetch_hp_job_info/", kuh.FetchHPJobInfo)
	http.HandleFunc("/katib/fetch_hp_job_trial_info/", kuh.FetchHPJobTrialInfo)
	http.HandleFunc("/katib/fetch_nas_job_info/", kuh.FetchNASJobInfo)

	http.HandleFunc("/katib/fetch_trial_templates/", kuh.FetchTrialTemplates)
	http.HandleFunc("/katib/fetch_collector_templates/", kuh.FetchMetricsCollectorTemplates)
	http.HandleFunc("/katib/update_template/", kuh.AddEditDeleteTemplate)

	if err := http.ListenAndServe(fmt.Sprintf("%s:%s", *host, *port), nil); err != nil {
		panic(err)
	}
}
