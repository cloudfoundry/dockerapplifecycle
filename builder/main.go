package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
)

type registries []string

var insecureDockerRegistries registries

func main() {
	flagSet := flag.NewFlagSet("builder", flag.ExitOnError)

	dockerImageURL := flagSet.String(
		"dockerImageURL",
		"",
		"docker image uri in docker://[registry/][scope/]repository[#tag] format",
	)

	dockerRef := flagSet.String(
		"dockerRef",
		"",
		"docker image reference in standard docker string format",
	)

	flagSet.Var(
		&insecureDockerRegistries,
		"insecureDockerRegistries",
		"Insecure Docker Registry addresses (ip:port)",
	)

	outputFilename := flagSet.String(
		"outputMetadataJSONFilename",
		"/tmp/result/result.json",
		"filename in which to write the app metadata",
	)

	if err := flagSet.Parse(os.Args[1:len(os.Args)]); err != nil {
		println(err.Error())
		os.Exit(1)
	}

	var repoName string
	var tag string
	if len(*dockerImageURL) > 0 {
		parts, err := url.Parse(*dockerImageURL)
		if err != nil {
			println("invalid dockerImageURL: " + *dockerImageURL)
			flagSet.PrintDefaults()
			os.Exit(1)
		}
		repoName, tag = helpers.ParseDockerURL(parts)
	} else if len(*dockerRef) > 0 {
		repoName, tag = helpers.ParseDockerRef(*dockerRef)
	} else {
		println("missing flag: dockerImageURL or dockerRef required")
		flagSet.PrintDefaults()
		os.Exit(1)
	}

	img, err := helpers.FetchMetadata(repoName, tag, insecureDockerRegistries)
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}

	info := protocol.ExecutionMetadata{}
	if img.Config != nil {
		info.Cmd = img.Config.Cmd
		info.Entrypoint = img.Config.Entrypoint
		info.Workdir = img.Config.WorkingDir
	}

	if err := helpers.SaveMetadata(*outputFilename, &info); err != nil {
		println(err.Error())
		os.Exit(1)
	}
}

func (r *registries) String() string {
	return fmt.Sprint(*r)
}

func (r *registries) Set(value string) error {
	if len(*r) > 0 {
		return errors.New("Docker Registries flag already set")
	}
	for _, reg := range strings.Split(value, ",") {
		registry := strings.TrimSpace(reg)
		if strings.Contains(registry, "://") {
			return errors.New("no scheme allowed for insecure Docker Registry [" + registry + "]")
		}
		if !strings.Contains(registry, ":") {
			return errors.New("ip:port expected for insecure Docker Registry [" + registry + "]")
		}
		*r = append(*r, registry)
	}
	return nil
}
