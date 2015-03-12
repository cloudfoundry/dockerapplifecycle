package unix_transport

import (
	"testing"

	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
)

func TestUnixTransport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UnixTransport Suite")
}
