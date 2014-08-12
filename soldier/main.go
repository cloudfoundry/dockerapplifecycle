package main

import (
	"encoding/json"
	"os"
	"strconv"
	"syscall"
)

func main() {
	os.Setenv("HOME", os.Args[1])
	os.Setenv("TMPDIR", os.Args[1]+"/tmp")

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

	argv := []string{
		"/bin/sh",
		"-c",
		os.Args[2],
	}

	os.Chdir(os.Args[1])
	syscall.Exec("/bin/sh", argv, os.Environ())
}
