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

	outputFilename := flagSet.String(
		"outputMetadataJSONFilename",
		"/tmp/result/result.json",
		"filename in which to write the app metadata",
	)

	if err := flagSet.Parse(os.Args[1:len(os.Args)]); err != nil {
		println(err.Error())
		os.Exit(1)
	}

	if *dockerImageUrl == "" {
		println("missing flag: dockerImageUrl")
		usage()
	}

	parts, err := url.Parse(*dockerImageUrl)
	if err != nil {
		println("invalid dockerImageUrl: " + *dockerImageUrl)
		usage()
	}

	img, err := helpers.FetchMetadata(parts)
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}

	info := protocol.ExecutionMetadata{}
	if img.Config != nil {
		info.Cmd = img.Config.Cmd
		info.Entrypoint = img.Config.Entrypoint
	}

	if helpers.SaveMetadata(*outputFilename, &info) != nil {
		println(err.Error())
		os.Exit(1)
	}
}

func usage() {
	flag.PrintDefaults()
	os.Exit(1)
}
