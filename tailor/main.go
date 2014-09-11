package main

import (
	"flag"
	"net/url"
	"os"

	"github.com/cloudfoundry-incubator/docker-circus/helpers"
	"github.com/cloudfoundry-incubator/docker-circus/protocol"
)

func main() {
	flagSet := flag.NewFlagSet("tailor", flag.ExitOnError)

	dockerImageUrl := flagSet.String(
		"dockerImageUrl",
		"",
		"docker image uri in docker://[registry/][scope/]repository[#tag] format",
	)

	dockerRef := flagSet.String(
		"dockerRef",
		"",
		"docker image reference in standard docker string format",
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
	if len(*dockerImageUrl) > 0 {
		parts, err := url.Parse(*dockerImageUrl)
		if err != nil {
			println("invalid dockerImageUrl: " + *dockerImageUrl)
			flagSet.PrintDefaults()
			os.Exit(1)
		}
		repoName, tag = helpers.ParseDockerURL(parts)
	} else if len(*dockerRef) > 0 {
		repoName, tag = helpers.ParseDockerRef(*dockerRef)
	} else {
		println("missing flag: dockerImageUrl or dockerRef required")
		flagSet.PrintDefaults()
		os.Exit(1)
	}

	img, err := helpers.FetchMetadata(repoName, tag)
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}

	info := protocol.ExecutionMetadata{}
	if img.Config != nil {
		info.Cmd = img.Config.Cmd
		info.Entrypoint = img.Config.Entrypoint
	}

	if err := helpers.SaveMetadata(*outputFilename, &info); err != nil {
		println(err.Error())
		os.Exit(1)
	}
}