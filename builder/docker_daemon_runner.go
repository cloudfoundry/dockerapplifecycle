package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

var DockerArgs []string = []string{"--daemon=true", "--iptables=false", "--ipv6=false", `--log-level="debug"`}

type DockerDaemon struct {
	DockerDaemonPath         string
	InsecureDockerRegistries []string
}

func (daemon *DockerDaemon) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	daemonProcess, errorChannel := launchDockerDaemon(daemon.DockerDaemonPath, daemon.InsecureDockerRegistries)
	close(ready)

	select {
	case err := <-errorChannel:
		if err != nil {
			return err
		}
	case signal := <-signals:
		err := daemonProcess.Signal(signal)
		if err != nil {
			println("failed to send signal", signal.String(), "to Docker daemon:", err.Error())
		}
	}

	return nil
}

func launchDockerDaemon(daemonPath string, insecureDockerRegistriesList []string) (*os.Process, <-chan error) {
	chanError := make(chan error, 1)

	args := DockerArgs
	if len(insecureDockerRegistriesList) > 0 {
		args = append(args, "--insecure-registry="+strings.Join(insecureDockerRegistriesList, ","))
	}

	cmd := exec.Command(daemonPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	err := cmd.Start()
	if err != nil {
		chanError <- fmt.Errorf(
			"failed to start Docker daemon [%s %s]: %s",
			path.Clean(daemonPath),
			strings.Join(args, " "),
			err,
		)
		return nil, chanError
	}

	go func() {
		defer close(chanError)

		err := cmd.Wait()
		if err != nil {
			chanError <- err
			println("Docker daemon failed with", err.Error())
		}

		chanError <- nil
	}()

	return cmd.Process, chanError
}
