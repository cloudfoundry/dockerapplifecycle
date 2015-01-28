package helpers_test

import (
	"testing"

	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
)

func TestDockerLifecycleHealthcheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker-App-Lifecycle-Helpers Suite")
}
