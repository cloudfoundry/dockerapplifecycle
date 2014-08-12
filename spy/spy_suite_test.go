package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDockerCircusSpy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker-Circus-Spy Suite")
}
