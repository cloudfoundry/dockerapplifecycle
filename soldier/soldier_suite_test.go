package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDockerCircusSoldier(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker-Circus-Soldier Suite")
}
