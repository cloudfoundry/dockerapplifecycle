package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var soldier string

func TestDockerCircusSoldier(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker-Circus-Soldier Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	soldierPath, err := gexec.Build("github.com/cloudfoundry-incubator/docker-circus/soldier")
	Ω(err).ShouldNot(HaveOccurred())
	return []byte(soldierPath)
}, func(soldierPath []byte) {
	soldier = string(soldierPath)
})

var _ = SynchronizedAfterSuite(func() {
	//noop
}, func() {
	gexec.CleanupBuildArtifacts()
})
