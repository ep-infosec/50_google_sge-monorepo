// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sge-monorepo/libs/go/zip_utils"
	"sge-monorepo/tools/p4_benchmark/protos/benchmarkpb"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	gouuid "github.com/nu7hatch/gouuid"
)

// Loads the benchmark suite definition from a textpb file.
func loadBenchmarkDefinition(benchmarkSuitePath string) (*benchmarkpb.BenchmarkSuite, error) {

	definitionBytes, err := ioutil.ReadFile(benchmarkSuitePath)
	if err != nil {
		return nil, fmt.Errorf("could not find benchmark definition: %v", err)
	}

	benchmarkProto := benchmarkpb.BenchmarkSuite{}
	if err := proto.UnmarshalText(string(definitionBytes), &benchmarkProto); err != nil {
		return nil, fmt.Errorf("could not unmarshal benchmark definition: %v", err)
	}

	return &benchmarkProto, nil
}

// Prints the usage.
func printUsage() {
	fmt.Println(`Usage:
p4benchmark [-workspace_root=path] [-session_id=id] [-csv_output=path] [-verbose] <benchmark suite textproto> <command>`)
	fmt.Println("  -workspace_root: Perforce workspace root containing benchmark packages, runs and results")
	fmt.Println("  -session_id: The session id generated by the setup command")
	fmt.Println("  -csv_output: The path to the CSV output")
	fmt.Println("  -verbose: Verbose output")
	fmt.Println("  command: One of setup|teardown|run")
}

// Ensures path separators match the platform
func normalizePathSeparators(path string) string {
	separator := string(os.PathSeparator)
	return strings.TrimSuffix(strings.ReplaceAll(strings.ReplaceAll(path, "//", separator), "\\", separator), separator)
}

// Checks if a specified path exists and is a directory.
func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}

	if fileInfo.Mode().IsRegular() {
		return false
	}

	return true
}

// Checks if a specified path exists and is a file.
func isFile(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}

	if fileInfo.Mode().IsRegular() {
		return true
	}

	return false
}

// Executes the setup step.
// Optionally creates a new session and extracts all archive files specified in benchmarkSuite.
func setup(benchmarkSuite *benchmarkpb.BenchmarkSuite, workspaceRoot string, existingSessionId string, csvLogger *CsvLogger, verbose bool) error {
	sessionId := existingSessionId

	sessionsRoot := filepath.Join(workspaceRoot, "sessions")

	if sessionId == "" {
		uuid, err := gouuid.NewV4()
		if err != nil {
			return fmt.Errorf("cannot generate UUID: %v", err)
		}
		sessionId = fmt.Sprintf("%v", uuid)
	}

	sessionRoot := filepath.Join(sessionsRoot, sessionId)
	fmt.Printf("Setting up session %v in %v\n", sessionId, sessionRoot)

	if isDirectory(sessionRoot) {
		glog.Warning("Session directory already exists")
	} else {
		err := os.Mkdir(sessionRoot, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s : %v", sessionRoot, err)
		}
	}

	startTime := time.Now()

	archiveRoot := filepath.Join(workspaceRoot, "data")

	for _, archive := range benchmarkSuite.Setup.Archive {
		archivePath := filepath.Join(archiveRoot, normalizePathSeparators(archive.ArchivePath))
		glog.Infof("Extracting archive: %v\n", archivePath)
		if !isFile(archivePath) {
			return fmt.Errorf("file %v does not exist", archivePath)
		}
		if len(archive.Subdirectory) == 0 {
			if err := zip_utils.UnzipFile(archivePath, sessionRoot); err != nil {
				return fmt.Errorf("failed to extract archive %s : %v", archivePath, err)
			}
		} else {
			for _, subdirectory := range archive.Subdirectory {
				if err := zip_utils.UnzipSubdirectoriesFromFile(archivePath, subdirectory, sessionRoot); err != nil {
					return fmt.Errorf("failed to extract subdirectory %s from archive %s  : %v", subdirectory, archivePath, err)
				}
			}
		}
	}

	endTime := time.Now()
	csvLogger.Log("setup", "extraction", "", 1, startTime, endTime, nil)

	return nil
}

// Executes the teardown step.
func teardown(benchmarkSuite *benchmarkpb.BenchmarkSuite, csvLogger *CsvLogger, verbose bool) error {
	startTime := time.Now()
	// TODO: Implement the teardown logic
	endTime := time.Now()
	csvLogger.Log("teardown", "TBD", "", 1, startTime, endTime, nil)
	return nil
}

func runBefore(fullCommand string) (map[string]string, error) {
	beforeCommandOutput := map[string]string{}
	parts := strings.Split(fullCommand, " ")
	command := exec.Command(parts[0], parts[1:]...)
	output, err := command.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("%s -%v", output, err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		lineParts := strings.Split(line, ":")
		if len(lineParts) < 2 {
			continue
		}
		beforeCommandOutput[strings.TrimSpace(lineParts[0])] = strings.TrimSpace(strings.Join(lineParts[1:], ":"))
	}

	return beforeCommandOutput, nil
}

func runCommand(commandName string, set string, repetition int, fullCommand string, csvLogger *CsvLogger, beforeCommandOutput *map[string]string) error {
	startTime := time.Now()
	parts := strings.Split(fullCommand, " ")
	command := exec.Command(parts[0], parts[1:]...)
	output, err := command.CombinedOutput()
	endTime := time.Now()
	elapsedTime := endTime.Sub(startTime)
	log.Printf("Time to execute %s: %s", commandName, elapsedTime)
	csvLogger.Log("run", commandName, set, repetition, startTime, endTime, beforeCommandOutput)

	if err != nil {
		return fmt.Errorf("%s -%v", output, err)
	}

	return nil
}

func expandCommand(command string, env map[string]string) string {
	expandedCommand := command
	for envKey, envValue := range env {
		expandedCommand = strings.ReplaceAll(expandedCommand, "$"+envKey, envValue)
	}
	return normalizePathSeparators(expandedCommand)
}

// Executes the run step.
func run(benchmarkSuite *benchmarkpb.BenchmarkSuite, workspaceRoot string, sessionId string, sets string, csvLogger *CsvLogger, verbose bool, repetitions int) error {

	sessionRoot := filepath.Join(workspaceRoot, "sessions", sessionId)

	glog.Infof("Running benchmarks for session %v in %v\n", sessionId, sessionRoot)

	if !isDirectory(sessionRoot) {
		return fmt.Errorf("Session directory %v does not exist", sessionRoot)
	}

	// Add the sessionRoot to the list of substitution variables.
	env := map[string]string{}
	env["sessionRoot"] = normalizePathSeparators(sessionRoot)
	env["workspaceRoot"] = normalizePathSeparators(workspaceRoot)
	env["sessionId"] = sessionId

	benchmarkSets := strings.Split(sets, ",")
	setLookup := make(map[string]*benchmarkpb.BenchmarkSet)
	for _, set := range benchmarkSuite.Set {
		setLookup[set.Name] = set
	}

	// Iterate over all requested test sets and run all benchmark steps for them.
	for _, requestedSet := range benchmarkSets {
		if matchedSet, ok := setLookup[requestedSet]; ok {
			glog.Infof("Running benchmark set %v", matchedSet.Name)
			// Add the source and target properties of the current set to the list of substitution variables.
			env["source"] = normalizePathSeparators(matchedSet.Source)
			env["target"] = normalizePathSeparators(matchedSet.Target)

			for i := 1; i <= repetitions; i++ {
				if verbose {
					glog.Infof("Repeating %d out of %d", i, repetitions)
				}

				for _, step := range benchmarkSuite.Step {
					var loggingExtras *map[string]string = nil
					if len(step.Before) > 0 {
						command := step.Before
						glog.Infof("Processing before command '%v'", command)
						command = expandCommand(command, env)
						if verbose {
							glog.Infof("Executing before command '%v'", command)
						}
						beforeCommandOutput, err := runBefore(command)
						if err != nil {
							return fmt.Errorf("Failed to execute before command '%v': %v", command, err)
						}
						loggingExtras = &beforeCommandOutput

						if verbose {
							glog.Infof("Before command output: %v", beforeCommandOutput)
						}
					}
					command := step.Command
					glog.Infof("Processing command '%v'", command)
					command = expandCommand(command, env)
					if verbose {
						glog.Infof("Executing command '%v'", command)
					}

					err := runCommand(step.Name, matchedSet.Name, i, command, csvLogger, loggingExtras)
					if err != nil {
						return fmt.Errorf("Failed to execute command '%v': %v", command, err)
					}
				}
			}
		} else {
			glog.Warningf("Benchmark set %v not found", requestedSet)
		}
	}

	return nil
}

// Runs the p4_benchmark tool.
func runBenchmark() error {
	flags := struct {
		workspaceRoot string
		sessionId     string
		csvOutput     string
		verbose       bool
		repetitions   int
	}{}
	flag.StringVar(&flags.workspaceRoot, "workspace_root", "", "Perforce workspace root path.")
	flag.StringVar(&flags.sessionId, "session_id", "", "Benchmarking session id.")
	flag.StringVar(&flags.csvOutput, "csv_output", "benchmark.csv", "CSV output path.")
	flag.BoolVar(&flags.verbose, "verbose", false, "Verbose output.")
	flag.IntVar(&flags.repetitions, "repetitions", 1, "Number of times to repeat each step.")
	flag.Parse()

	if flag.NArg() < 2 {
		printUsage()
		return fmt.Errorf("insufficient number or arguments specified")
	}

	if flags.workspaceRoot == "" {
		printUsage()
		return fmt.Errorf("workspace root needs to be specified")
	}

	if !isDirectory(flags.workspaceRoot) {
		return fmt.Errorf("workspace root does not exist or is not a directory")
	}

	benchmarkProto, err := loadBenchmarkDefinition(flag.Arg(0))
	if err != nil {
		return err
	}

	csvLogger := NewCsvLogger(flags.csvOutput)

	switch flag.Arg(1) {
	case "setup":
		return setup(benchmarkProto, flags.workspaceRoot, flags.sessionId, csvLogger, flags.verbose)
	case "teardown":
		return teardown(benchmarkProto, csvLogger, flags.verbose)
	case "run":
		if flag.NArg() != 3 {
			printUsage()
			return fmt.Errorf("insufficient number or arguments specified for the run command")
		}
		return run(benchmarkProto, flags.workspaceRoot, flags.sessionId, flag.Arg(2), csvLogger, flags.verbose, flags.repetitions)
	default:
		return fmt.Errorf("unknown command: %q", flag.Arg(1))
	}
}

func main() {
	// glog to both stderr and to file
	flag.Set("alsologtostderr", "true")

	if err := runBenchmark(); err == nil {
		glog.Info("Benchmark run successfull")
	} else {
		glog.Errorf("Benchmark run failed: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			glog.Errorf("exitErr.Stderr: %v", exitErr.Stderr)
		}
		os.Exit(1)
	}
}
