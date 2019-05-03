package main

import (
	"net/http"

	ui "github.com/kubeflow/katib/pkg/ui/v1alpha2"
)

var (
	port = "80"
)

func main() {
	kuh := ui.NewKatibUIHandler()

	frontend := http.FileServer(http.Dir("/app/build/"))
	http.Handle("/katib/", http.StripPrefix("/katib/", frontend))

	http.HandleFunc("/katib/fetch_hp_jobs/", kuh.FetchHPJobs)
	http.HandleFunc("/katib/fetch_nas_jobs/", kuh.FetchNASJobs)
	http.HandleFunc("/katib/submit_yaml/", kuh.SubmitYamlJob)
	http.HandleFunc("/katib/submit_hp_job/", kuh.SubmitHPJob)
	http.HandleFunc("/katib/submit_nas_job/", kuh.SubmitNASJob)
	http.HandleFunc("/katib/delete_job/", kuh.DeleteJob)

	http.HandleFunc("/katib/fetch_hp_job_info/", kuh.FetchHPJobInfo)
	http.HandleFunc("/katib/fetch_nas_job_info/", kuh.FetchNASJobInfo)
	http.HandleFunc("/katib/fetch_worker_info/", kuh.FetchWorkerInfo)
	http.HandleFunc("/katib/fetch_worker_templates/", kuh.FetchWorkerTemplates)
	http.HandleFunc("/katib/fetch_collector_templates/", kuh.FetchCollectorTemplates)
	http.HandleFunc("/katib/update_template/", kuh.AddEditTemplate)
	http.HandleFunc("/katib/delete_template/", kuh.DeleteTemplate)

	http.ListenAndServe(":"+port, nil)
}
