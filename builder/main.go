package main

import (
	"flag"
	"net/url"
	"os"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
)

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

	dockerRegistryURL := flagSet.String(
		"dockerRegistryURL",
		"",
		"Private Docker Registry URL",
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

	var insecureRegistries []string
	if len(*dockerRegistryURL) > 0 {
		parts, err := url.Parse(*dockerRegistryURL)
		if err != nil {
			println("invalid dockerRegistryURL: " + *dockerRegistryURL)
			flagSet.PrintDefaults()
			os.Exit(1)
		}

		if parts.Scheme == "http" {
			insecureRegistries = []string{*dockerRegistryURL}
		}
	}

	img, err := helpers.FetchMetadata(repoName, tag, insecureRegistries)
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
