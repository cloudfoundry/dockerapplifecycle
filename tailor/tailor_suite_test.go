package main_test

import (
	"testing"

	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega/gexec"
)

var tailorPath string

func TestDockerCircusTailor(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeSuite(func() {
		var err error

		tailorPath, err = gexec.Build("github.com/cloudfoundry-incubator/docker-circus/tailor")
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterSuite(func() {
		gexec.CleanupBuildArtifacts()
	})

	RunSpecs(t, "Docker-Circus-Tailor Suite")
}
