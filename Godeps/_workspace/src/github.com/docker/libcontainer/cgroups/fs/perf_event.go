package fs

import (
	"github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/docker/libcontainer/cgroups"
)

type PerfEventGroup struct {
}

func (s *PerfEventGroup) Set(d *data) error {
	// we just want to join this group even though we don't set anything
	if _, err := d.join("perf_event"); err != nil && !cgroups.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *PerfEventGroup) Remove(d *data) error {
	return removePath(d.path("perf_event"))
}

func (s *PerfEventGroup) GetStats(path string, stats *cgroups.Stats) error {
	return nil
}
