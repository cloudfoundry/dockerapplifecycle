package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/cloudfoundry-incubator/docker-circus/protocol"
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

		vcapAppEnv["instance_id"] = os.Getenv("CF_INSTANCE_GUID")

		port, err := strconv.Atoi(os.Getenv("PORT"))
		if err == nil {
			vcapAppEnv["port"] = port
		}

		index, err := strconv.Atoi(os.Getenv("CF_INSTANCE_INDEX"))
		if err == nil {
			vcapAppEnv["instance_index"] = index
		}

		mungedAppEnv, err := json.Marshal(vcapAppEnv)
		if err == nil {
			os.Setenv("VCAP_APPLICATION", string(mungedAppEnv))
		}
	}

	os.Chdir(os.Args[1])

	if startCommand != "" {
		syscall.Exec("/bin/sh", []string{
			"/bin/sh",
			"-c",
			startCommand,
		}, os.Environ())
	} else {
		var executionMetadata protocol.ExecutionMetadata
		err := json.Unmarshal([]byte(metadata), &executionMetadata)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid metadata - %s", err)
			os.Exit(1)
		} else if len(executionMetadata.Entrypoint) > 0 || len(executionMetadata.Cmd) > 0 {
			// https://docs.docker.com/reference/builder/#entrypoint and
			// https://docs.docker.com/reference/builder/#cmd dictate how Entrypoint
			// and Cmd are treated by docker; we follow these rules here
			argv := executionMetadata.Entrypoint
			argv = append(argv, executionMetadata.Cmd...)
			syscall.Exec(argv[0], argv, os.Environ())
		} else {
			fmt.Fprintf(os.Stderr, "No start command found or specified")
			os.Exit(1)
		}
	}
}
