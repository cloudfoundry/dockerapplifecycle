package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
)

type Builder struct {
	RepoName                 string
	Tag                      string
	InsecureDockerRegistries []string
	OutputFilename           string
}

func (builder *Builder) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
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
