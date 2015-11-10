package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: %s <app directory> <start command> <metadata>", os.Args[0])
		os.Exit(1)
	}

	dir := os.Args[1]
	startCommand := os.Args[2]
	metadata := os.Args[3]

	os.Setenv("HOME", dir)
	os.Setenv("TMPDIR", filepath.Join(dir, "tmp"))

	vcapAppEnv := map[string]interface{}{}

	err := json.Unmarshal([]byte(os.Getenv("VCAP_APPLICATION")), &vcapAppEnv)
	if err == nil {
		vcapAppEnv["host"] = "0.0.0.0"

		vcapAppEnv["instance_id"] = os.Getenv("INSTANCE_GUID")

		port, err := strconv.Atoi(os.Getenv("PORT"))
		if err == nil {
			vcapAppEnv["port"] = port
		}

		index, err := strconv.Atoi(os.Getenv("INSTANCE_INDEX"))
		if err == nil {
			vcapAppEnv["instance_index"] = index
		}

		mungedAppEnv, err := json.Marshal(vcapAppEnv)
		if err == nil {
			os.Setenv("VCAP_APPLICATION", string(mungedAppEnv))
		}
	}

	var executionMetadata protocol.ExecutionMetadata
	err = json.Unmarshal([]byte(metadata), &executionMetadata)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid metadata - %s", err)
		os.Exit(1)
	}

	workdir := "/"
	if executionMetadata.Workdir != "" {
		workdir = executionMetadata.Workdir
	}
	os.Chdir(workdir)

	if startCommand != "" {
		err = syscall.Exec("/bin/sh", []string{
			"/bin/sh",
			"-c",
			startCommand,
		}, os.Environ())
	} else {
		if len(executionMetadata.Entrypoint) == 0 && len(executionMetadata.Cmd) == 0 {
			fmt.Fprintf(os.Stderr, "No start command found or specified")
			os.Exit(1)
		}

		// https://docs.docker.com/reference/builder/#entrypoint and
		// https://docs.docker.com/reference/builder/#cmd dictate how Entrypoint
		// and Cmd are treated by docker; we follow these rules here
		argv := executionMetadata.Entrypoint
		argv = append(argv, executionMetadata.Cmd...)
		ecmd := exec.Command(argv[0], argv[1:]...)
		ecmd.Stdout = os.Stdout
		ecmd.Stderr = os.Stderr
		err = ecmd.Run()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run: %s", err)
		os.Exit(1)
	}
}
