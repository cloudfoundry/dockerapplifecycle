package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"code.cloudfoundry.org/dockerapplifecycle/docker/nat"
	"code.cloudfoundry.org/dockerapplifecycle/helpers"
	"code.cloudfoundry.org/dockerapplifecycle/protocol"
	ecr "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	ecrapi "github.com/awslabs/amazon-ecr-credential-helper/ecr-login/api"
	"github.com/containers/image/types"
)

const ECR_REPO_REGEX = `[a-zA-Z0-9][a-zA-Z0-9_-]*\.dkr\.ecr\.[a-zA-Z0-9][a-zA-Z0-9_-]*\.amazonaws\.com(\.cn)?[^ ]*`

type Builder struct {
	RegistryURL                string
	RepoName                   string
	Tag                        string
	InsecureDockerRegistries   []string
	OutputFilename             string
	DockerDaemonExecutablePath string
	DockerDaemonUnixSocket     string
	DockerDaemonTimeout        time.Duration
	CacheDockerImage           bool
	DockerRegistryIPs          []string
	DockerRegistryHost         string
	DockerRegistryPort         int
	DockerRegistryRequireTLS   bool
	DockerLoginServer          string
	DockerUser                 string
	DockerPassword             string
	DockerEmail                string
}

func (builder *Builder) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	select {
	case err := <-builder.build():
		if err != nil {
			return err
		}
	case signal := <-signals:
		return errors.New(signal.String())
	}

	return nil
}

func (builder Builder) build() <-chan error {
	errorChan := make(chan error, 1)

	go func() {
		defer close(errorChan)

		username, password, err := builder.getCredentials()
		if err != nil {
			errorChan <- err
			return
		}

		ctx := &types.SystemContext{
			DockerAuthConfig: &types.DockerAuthConfig{
				Username: username,
				Password: password,
			},
		}
		for _, insecure := range builder.InsecureDockerRegistries {
			if builder.RegistryURL == insecure {
				ctx.DockerInsecureSkipTLSVerify = true
			}
		}

		imgConfig, err := helpers.FetchMetadata(builder.RegistryURL, builder.RepoName, builder.Tag, ctx, os.Stderr)
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
		if imgConfig != nil {
			info.ExecutionMetadata.Cmd = imgConfig.Cmd
			info.ExecutionMetadata.Entrypoint = imgConfig.Entrypoint
			info.ExecutionMetadata.Workdir = imgConfig.WorkingDir
			info.ExecutionMetadata.User = imgConfig.User
			info.ExecutionMetadata.ExposedPorts, err = extractPorts(convertPortsToNatPorts(imgConfig.ExposedPorts))
			if err != nil {
				portDetails := fmt.Sprintf("%v", imgConfig.ExposedPorts)
				println("failed to parse image ports", portDetails, err.Error())
				errorChan <- err
				return
			}
		}

		dockerImageURL := builder.RepoName
		if builder.RegistryURL != helpers.DockerHubHostname {
			dockerImageURL = builder.RegistryURL + "/" + dockerImageURL
		}
		if len(builder.Tag) > 0 {
			dockerImageURL = dockerImageURL + ":" + builder.Tag
		}
		info.DockerImage = dockerImageURL

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

func (builder Builder) getCredentials() (string, string, error) {
	username := builder.DockerUser
	password := builder.DockerPassword

	rECRRepo, err := regexp.Compile(ECR_REPO_REGEX)
	if err != nil {
		return "", "", fmt.Errorf(
			"failed to compile ECR repo regex: %s",
			err.Error(),
		)
	}

	if rECRRepo.MatchString(builder.RegistryURL) {
		os.Setenv("AWS_ACCESS_KEY_ID", builder.DockerUser)
		os.Setenv("AWS_SECRET_ACCESS_KEY", builder.DockerPassword)

		defer os.Unsetenv("AWS_ACCESS_KEY_ID")
		defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")

		username, password, err = ecr.ECRHelper{
			ClientFactory: ecrapi.DefaultClientFactory{},
		}.Get(builder.RegistryURL)
		if err != nil {
			return "", "", fmt.Errorf(
				"failed to get ECR credentials from [%s] due to %s",
				builder.RegistryURL,
				err.Error(),
			)
		}
	}

	return username, password, nil
}

func convertPortsToNatPorts(ports map[string]struct{}) map[nat.Port]struct{} {
	natPorts := map[nat.Port]struct{}{}
	for portProto, v := range ports {
		proto, port := nat.SplitProtoPort(portProto)
		p := nat.NewPort(proto, port)
		natPorts[p] = v
	}
	return natPorts
}

func extractPorts(dockerPorts map[nat.Port]struct{}) (exposedPorts []protocol.Port, err error) {
	sortedPorts := sortPorts(dockerPorts)
	for _, port := range sortedPorts {
		exposedPort, err := strconv.ParseUint(port.Port(), 10, 16)
		if err != nil {
			return []protocol.Port{}, err
		}
		exposedPorts = append(exposedPorts, protocol.Port{Port: uint16(exposedPort), Protocol: port.Proto()})
	}
	return exposedPorts, nil
}

func sortPorts(dockerPorts map[nat.Port]struct{}) []nat.Port {
	var dockerPortsSlice []nat.Port
	for port := range dockerPorts {
		dockerPortsSlice = append(dockerPortsSlice, port)
	}
	nat.Sort(dockerPortsSlice, func(ip, jp nat.Port) bool {
		return ip.Int() < jp.Int() || (ip.Int() == jp.Int() && ip.Proto() == "tcp")
	})
	return dockerPortsSlice
}
