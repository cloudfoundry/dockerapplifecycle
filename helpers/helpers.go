package helpers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/docker/docker/image"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/docker/docker/pkg/parsers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/docker/docker/registry"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/docker/docker/utils"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
)

// For docker:// URLs
func ParseDockerURL(parts *url.URL) (string, string) {
	var tag string
	if len(parts.Fragment) > 0 {
		tag = parts.Fragment
	} else {
		tag = "latest"
	}

	var repoName string
	if len(parts.Host) == 0 {
		repoName = strings.TrimPrefix(parts.Path, "/")
	} else {
		repoName = parts.Host + parts.Path
	}

	return repoName, tag
}

// For standard docker image references expressed as a protocol-less string
func ParseDockerRef(dockerRef string) (string, string) {
	repoName, tag := parsers.ParseRepositoryTag(dockerRef)

	if len(tag) == 0 {
		tag = "latest"
	}

	return repoName, tag
}

func FetchMetadata(repoName string, tag string, insecureRegistries []string) (*image.Image, error) {
	hostname, repoName, err := registry.ResolveRepositoryName(repoName)
	if err != nil {
		return nil, err
	}

	endpoint, err := registry.NewEndpoint(hostname, insecureRegistries)
	if err != nil {
		return nil, err
	}

	authConfig := &registry.AuthConfig{}
	session, err := registry.NewSession(authConfig, utils.NewHTTPRequestFactory(), endpoint, true)
	if err != nil {
		return nil, err
	}

	repoData, err := session.GetRepositoryData(repoName)
	if err != nil {
		return nil, err
	}

	tagsList, err := session.GetRemoteTags(repoData.Endpoints, repoName, repoData.Tokens)
	if err != nil {
		return nil, err
	}

	imgID, ok := tagsList[tag]
	if !ok {
		return nil, fmt.Errorf("unknown tag: %s:%s", repoName, tag)
	}

	for _, endpoint := range repoData.Endpoints {
		imgJSON, _, err := session.GetRemoteImageJSON(imgID, endpoint, repoData.Tokens)
		if err == nil {
			img, err := image.NewImgJSON(imgJSON)
			if err != nil {
				return nil, err
			}
			return img, err
		}
	}

	return nil, fmt.Errorf("all endpoints failed: %s", err)
}

func SaveMetadata(filename string, metadata *protocol.DockerImageMetadata) error {
	err := os.MkdirAll(path.Dir(filename), 0755)
	if err != nil {
		return err
	}

	executionMetadataJSON, err := json.Marshal(metadata.ExecutionMetadata)
	if err != nil {
		return err
	}

	resultFile, err := os.Create(filename)
	if err != nil {
		return err
	}

	defer resultFile.Close()

	startCommand := strings.Join(metadata.ExecutionMetadata.Cmd, " ")
	if len(metadata.ExecutionMetadata.Entrypoint) > 0 {
		startCommand = strings.Join([]string{strings.Join(metadata.ExecutionMetadata.Entrypoint, " "), startCommand}, " ")
	}
	err = json.NewEncoder(resultFile).Encode(docker_app_lifecycle.StagingDockerResult{
		ExecutionMetadata: string(executionMetadataJSON),
		DetectedStartCommand: map[string]string{
			"web": startCommand,
		},
		DockerImage: metadata.DockerImage,
	})
	if err != nil {
		return err
	}

	return nil
}
