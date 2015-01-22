package helpers_test

import (
	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega"
	"testing"
)

func TestDockerCircusSpy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker-Circus-Helpers Suite")
}
