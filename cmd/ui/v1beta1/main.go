/*
Copyright 2022 The Kubeflow Authors.

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

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	common_v1beta1 "github.com/kubeflow/katib/pkg/common/v1beta1"
	ui "github.com/kubeflow/katib/pkg/ui/v1beta1"
)

var (
	port, host, buildDir, dbManagerAddr *string
)

func init() {
	port = flag.String("port", "8080", "The port to listen to for incoming HTTP connections")
	host = flag.String("host", "0.0.0.0", "The host to listen to for incoming HTTP connections")
	buildDir = flag.String("build-dir", "/app/build", "The dir of frontend")
	dbManagerAddr = flag.String("db-manager-address", common_v1beta1.GetDBManagerAddr(), "The address of Katib DB manager")
}

func main() {
	flag.Parse()
	kuh := ui.NewKatibUIHandler(*dbManagerAddr)

    baseHref := os.Getenv("KATIB_BASE_HREF")
	if baseHref == "" {
        baseHref = "/"
    } else if !strings.HasSuffix(baseHref, "/") {
		baseHref += "/"
	}
	baseHref = baseHref + "katib/"
	log.Printf("base-href: %s", baseHref)

	log.Printf("Serving the frontend dir %s", *buildDir)
	frontend := http.FileServer(http.Dir(*buildDir))
	http.HandleFunc(baseHref, kuh.ServeIndex(*buildDir, baseHref))
	http.Handle(fmt.Sprintf("%sstatic/", baseHref), http.StripPrefix(baseHref, frontend))
	
	http.HandleFunc(fmt.Sprintf("%sfetch_experiments/", baseHref), kuh.FetchExperiments)	
	http.HandleFunc(fmt.Sprintf("%screate_experiment/", baseHref), kuh.CreateExperiment)
	http.HandleFunc(fmt.Sprintf("%sdelete_experiment/", baseHref), kuh.DeleteExperiment)
	http.HandleFunc(fmt.Sprintf("%sfetch_experiment/", baseHref), kuh.FetchExperiment)
	http.HandleFunc(fmt.Sprintf("%sfetch_trial/", baseHref), kuh.FetchTrial)
	http.HandleFunc(fmt.Sprintf("%sfetch_suggestion/", baseHref), kuh.FetchSuggestion)
	http.HandleFunc(fmt.Sprintf("%sfetch_hp_job_info/", baseHref), kuh.FetchHPJobInfo)
	http.HandleFunc(fmt.Sprintf("%sfetch_hp_job_trial_info/", baseHref), kuh.FetchHPJobTrialInfo)
	http.HandleFunc(fmt.Sprintf("%sfetch_nas_job_info/", baseHref), kuh.FetchNASJobInfo)
	http.HandleFunc(fmt.Sprintf("%sfetch_trial_templates/", baseHref), kuh.FetchTrialTemplates)
	http.HandleFunc(fmt.Sprintf("%sadd_template/", baseHref), kuh.AddTemplate)
	http.HandleFunc(fmt.Sprintf("%sedit_template/", baseHref), kuh.EditTemplate)
	http.HandleFunc(fmt.Sprintf("%sdelete_template/", baseHref), kuh.DeleteTemplate)
	http.HandleFunc(fmt.Sprintf("%sfetch_namespaces/", baseHref), kuh.FetchNamespaces)
	http.HandleFunc(fmt.Sprintf("%sfetch_trial_logs/", baseHref), kuh.FetchTrialLogs)

	log.Printf("Serving at %s:%s", *host, *port)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%s", *host, *port), nil); err != nil {
		panic(err)
	}
}
