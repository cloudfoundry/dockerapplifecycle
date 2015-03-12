package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/unix_transport"
)

type Builder struct {
	RepoName                 string
	Tag                      string
	InsecureDockerRegistries []string
	OutputFilename           string
	DockerDaemonTimeout      time.Duration
}

func (builder *Builder) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := waitForDocker(signals, builder.DockerDaemonTimeout)
	if err != nil {
		return err
	}
	close(ready)

	select {
	case err := <-builder.fetchMetadata():
		if err != nil {
			return err
		}
	case signal := <-signals:
		return errors.New(signal.String())
	}

	return nil
}

func (builder *Builder) fetchMetadata() <-chan error {
	errorChan := make(chan error, 1)

	go func() {
		defer close(errorChan)

		img, err := helpers.FetchMetadata(builder.RepoName, builder.Tag, builder.InsecureDockerRegistries)
		if err != nil {
			errorChan <- fmt.Errorf(
				"failed to fetch metadata from [%s] with tag [%s] and insecure registries %s due to %s",
				builder.RepoName,
				builder.Tag,
				builder.InsecureDockerRegistries,
				err.Error(),
			)
			return
		}

		info := protocol.ExecutionMetadata{}
		if img.Config != nil {
			info.Cmd = img.Config.Cmd
			info.Entrypoint = img.Config.Entrypoint
			info.Workdir = img.Config.WorkingDir
		}

		if err := helpers.SaveMetadata(builder.OutputFilename, &info); err != nil {
			errorChan <- fmt.Errorf(
				"failed to save metadata to [%s] due to %s",
				builder.OutputFilename,
				err.Error(),
			)
			return
		}

		errorChan <- nil
	}()

	return errorChan
}

func waitForDocker(signals <-chan os.Signal, timeout time.Duration) error {
	giveUp := make(chan struct{})
	defer close(giveUp)

	select {
	case err := <-waitForDockerDaemon(giveUp):
		if err != nil {
			return err
		}
	case <-time.After(timeout):
		return errors.New("Timed out waiting for docker daemon to start")
	case signal := <-signals:
		return errors.New(signal.String())
	}

	return nil
}

func waitForDockerDaemon(giveUp <-chan struct{}) <-chan error {
	errChan := make(chan error, 1)
	client := http.Client{Transport: unix_transport.New("/var/run/docker.sock")}

	go pingDaemonPeriodically(client, errChan, giveUp)

	return errChan
}

func pingDaemonPeriodically(client http.Client, errChan chan<- error, giveUp <-chan struct{}) {
	for {
		resp, err := client.Get("unix:///var/run/docker.sock/_ping")
		if err != nil {
			println("Docker not ready yet. Ping returned ", err.Error())
			select {
			case <-giveUp:
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		} else {
			if resp.StatusCode == http.StatusOK {
				fmt.Println("Docker daemon running")
			} else {
				errChan <- fmt.Errorf("Docker daemon failed to start. Ping returned %s", resp.Status)
				return
			}
			break
		}

	}
	errChan <- nil
	return
}
