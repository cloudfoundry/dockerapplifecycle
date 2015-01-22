package image

import (
	"github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/docker/docker/daemon/graphdriver"
)

type Graph interface {
	Get(id string) (*Image, error)
	ImageRoot(id string) string
	Driver() graphdriver.Driver
}
