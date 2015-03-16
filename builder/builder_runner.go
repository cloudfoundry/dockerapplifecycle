package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/unix_transport"
)

type Builder struct {
	RepoName                   string
	Tag                        string
	InsecureDockerRegistries   []string
	OutputFilename             string
	DockerDaemonExecutablePath string
	DockerDaemonTimeout        time.Duration
	CacheDockerImage           bool
}

func (builder *Builder) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	if builder.CacheDockerImage {
		err := waitForDocker(signals, builder.DockerDaemonTimeout)
		if err != nil {
			return err
		}
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

		if !builder.CacheDockerImage {
			return
		}

		fmt.Println("Starting docker image caching ...")

		dockerImageURL := builder.RepoName
		if len(builder.Tag) > 0 {
			dockerImageURL = dockerImageURL + ":" + builder.Tag
		}

		fmt.Sprintf("Pulling docker image %s\n", dockerImageURL)
		err = builder.RunDockerCommand("pull", dockerImageURL)
		if err != nil {
			errorChan <- err
			return
		}

		fmt.Sprintf("Tagging docker image %s as %s\n", dockerImageURL, "10.244.2.6:8080/newtag")
		err = builder.RunDockerCommand("tag", dockerImageURL, "10.244.2.6:8080/newtag")
		if err != nil {
			errorChan <- err
			return
		}

		fmt.Sprintf("Pushing docker image %s\n", "10.244.2.6:8080/newtag")
		err = builder.RunDockerCommand("push", "10.244.2.6:8080/newtag")
		if err != nil {
			errorChan <- err
			return
		}

		fmt.Println("Docker image caching completed.")
		errorChan <- nil
	}()

	return errorChan
}

func (builder *Builder) RunDockerCommand(args ...string) error {
	cmd := exec.Command(builder.DockerDaemonExecutablePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
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
