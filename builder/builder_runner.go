package main

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/nu7hatch/gouuid"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/unix_transport"
)

type Builder struct {
	RepoName                   string
	Tag                        string
	DockerRegistryAddresses    []string
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

		info := protocol.DockerImageMetadata{}
		if img.Config != nil {
			info.ExecutionMetadata.Cmd = img.Config.Cmd
			info.ExecutionMetadata.Entrypoint = img.Config.Entrypoint
			info.ExecutionMetadata.Workdir = img.Config.WorkingDir
		}

		dockerImageURL := builder.RepoName
		if len(builder.Tag) > 0 {
			dockerImageURL = dockerImageURL + ":" + builder.Tag
		}
		info.DockerImage = dockerImageURL

		if builder.CacheDockerImage {
			info.DockerImage, err = builder.cacheDockerImage(dockerImageURL)
			if err != nil {
				println("failed to cache image", dockerImageURL, err.Error())
				errorChan <- err
				return
			}
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

func (builder *Builder) cacheDockerImage(dockerImage string) (string, error) {
	fmt.Println("Caching docker image ...")

	fmt.Printf("Pulling docker image %s ...\n", dockerImage)
	err := builder.RunDockerCommand("pull", dockerImage)
	if err != nil {
		return "", err
	}
	fmt.Println("Docker image pulled.")

	cachedDockerImage, err := builder.GenerateImageName()
	if err != nil {
		return "", err
	}
	fmt.Printf("Docker image will be cached as %s\n", cachedDockerImage)

	fmt.Printf("Tagging docker image %s as %s ...\n", dockerImage, cachedDockerImage)
	err = builder.RunDockerCommand("tag", dockerImage, cachedDockerImage)
	if err != nil {
		return "", err
	}
	fmt.Println("Docker image tagged.")

	fmt.Printf("Pushing docker image %s\n", cachedDockerImage)
	err = builder.RunDockerCommand("push", cachedDockerImage)
	if err != nil {
		return "", err
	}
	fmt.Println("Docker image pushed.")
	fmt.Println("Docker image caching completed.")

	return cachedDockerImage, nil
}

func (builder *Builder) RunDockerCommand(args ...string) error {
	cmd := exec.Command(builder.DockerDaemonExecutablePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func (builder *Builder) GenerateImageName() (string, error) {
	uuid, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", getRegistryAddress(builder.DockerRegistryAddresses), uuid), nil
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

func getRegistryAddress(registryAddresses []string) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return registryAddresses[r.Intn(len(registryAddresses))]
}
