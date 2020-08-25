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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	psutil "github.com/shirou/gopsutil/process"
)

// WaitPidsOpts is the input options for metrics collector
type WaitPidsOpts struct {
	PollInterval           time.Duration
	Timeout                time.Duration
	WaitAll                bool
	CompletedMarkedDirPath string
}

// WaitMainProcesses holds metrics collector parser until required pids are finished.
func WaitMainProcesses(opts WaitPidsOpts) error {

	if runtime.GOOS != "linux" {
		return fmt.Errorf("Platform '%s' unsupported", runtime.GOOS)
	}

	pids, mainPid, err := GetMainProcesses(opts.CompletedMarkedDirPath)
	if err != nil {
		return err
	}

	return WaitPIDs(pids, mainPid, opts)
}

// GetMainProcesses returns array with all running processes pids
// and main process pid which metrics collector is waiting.
func GetMainProcesses(completedMarkedDirPath string) (map[int]bool, int, error) {
	pids := make(map[int]bool)
	allPids, err := psutil.Pids()
	mainPid := 0

	if err != nil {
		return nil, 0, fmt.Errorf("Failed to list processes: %v", err)
	}

	thisPID := os.Getpid()
	for _, pid := range allPids {
		// Create process object from pid
		proc, err := psutil.NewProcess(pid)
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to create new Process from pid %v, error: %v", pid, err)
		}

		// Get parent process
		ppid, err := proc.Ppid()
		if err != nil {
			return nil, 0, fmt.Errorf("Unable to get parent pid for pid: %v, error: %v", ppid, err)
		}

		// Ignore the pause container, our own pid, and non-root processes (parent pid != 0)
		if pid == 1 || pid == int32(thisPID) || ppid != 0 {
			continue
		}

		// Read the process command line
		cmdline, err := proc.Cmdline()
		if err != nil {
			return nil, 0, fmt.Errorf("Unable to get cmdline from pid %v, error: %v", pid, err)
		}

		// By default mainPid is the first process.
		// Command line contains completed marker for the main pid
		// For example: echo completed > /var/log/katib/$$$$.pid
		// completedMarkedDirPath is the directory for completed marker, e.g. /var/log/katib
		if mainPid == 0 ||
			strings.Contains(cmdline, fmt.Sprintf("echo %s > %s", TrainingCompleted, completedMarkedDirPath)) {
			mainPid = int(pid)
		}

		pids[int(pid)] = true
	}

	return pids, mainPid, nil
}

// WaitPIDs waits until all pids are finished.
// If waitAll == false WaitPIDs waits until main process is finished.
func WaitPIDs(pids map[int]bool, mainPid int, opts WaitPidsOpts) error {

	// notFinishedPids contains pids that are not finished yet
	notFinishedPids := pids

	// Get info from options
	waitAll := opts.WaitAll
	timeout := opts.Timeout
	endTime := time.Now().Add(timeout)
	pollInterval := opts.PollInterval

	// Start main wait loop
	// We should exit when timeout is out or notFinishedPids is empty
	for (timeout == 0 || time.Now().Before(endTime)) && len(notFinishedPids) > 0 {
		// Start loop over not finished pids
		for pid := range notFinishedPids {
			// If pid is completed /proc/<pid> dir doesn't exist
			path := fmt.Sprintf("/proc/%d", pid)
			_, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					if pid == mainPid {
						// For mainPid we check if file with "completed" marker exists if CompletedMarkedDirPath is set
						if opts.CompletedMarkedDirPath != "" {
							markFile := filepath.Join(opts.CompletedMarkedDirPath, fmt.Sprintf("%d.pid", pid))
							// Read file with "completed" marker
							if contents, err := ioutil.ReadFile(markFile); err != nil {
								return fmt.Errorf("Unable to read file %v for pid %v, error: %v", markFile, pid, err)
								// Check if file contains "completed" marker
							} else if strings.TrimSpace(string(contents)) != TrainingCompleted {
								return fmt.Errorf("Unable to find marker: %v in file: %v with contents: %v for pid: %v",
									TrainingCompleted, markFile, string(contents), pid)
							}
						}
						// Delete main pid from map with pids
						delete(notFinishedPids, pid)
						// Exit loop if wait all is false because main pid is finished
						if !waitAll {
							return nil
						}
						// Delete not main pid from map with pids
					} else {
						delete(notFinishedPids, pid)
					}
					// We should receive only not exist error when we check /proc/<pid> dir
				} else {
					return fmt.Errorf("Fail to check process info: %v", err)
				}
			}
		}
		// Sleep for pollInterval seconds before next attempt
		time.Sleep(pollInterval)
	}

	// After main loop notFinishedPids map should be empty
	if len(notFinishedPids) != 0 {
		return fmt.Errorf("Timed out waiting for pids to complete")
	}
	return nil
}
