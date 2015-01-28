package main_test

import (
	"testing"

	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega/gexec"
)

var healthcheck string

func TestDockerLifecycleHealthCheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker-App-Lifecycle-HealthCheck Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	healthcheckPath, err := gexec.Build("github.com/cloudfoundry-incubator/docker_app_lifecycle/healthcheck")
	Ω(err).ShouldNot(HaveOccurred())
	return []byte(healthcheckPath)
}, func(healthcheckPath []byte) {
	healthcheck = string(healthcheckPath)
})

var _ = SynchronizedAfterSuite(func() {
	//noop
}, func() {
	gexec.CleanupBuildArtifacts()
})
