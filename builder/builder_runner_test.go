package main_test

import (
	"errors"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle/builder"
	"github.com/cloudfoundry-incubator/inigo/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
)

var _ = Describe("Builder runner", func() {
	var (
		lifecycle        ifrit.Process
		fakeDeamonRunner func(signals <-chan os.Signal, ready chan<- struct{}) error
	)

	BeforeEach(func() {
		builder := main.Builder{
			RepoName:            "ubuntu",
			Tag:                 "latest",
			OutputFilename:      "/tmp/result/result.json",
			DockerDaemonTimeout: 300 * time.Millisecond,
			CacheDockerImage:    true,
		}

		lifecycle = ifrit.Background(grouper.NewParallel(os.Interrupt, grouper.Members{
			{"builder", ifrit.RunFunc(builder.Run)},
			{"fake_docker_daemon", ifrit.RunFunc(fakeDeamonRunner)},
		}))
	})

	AfterEach(func() {
		helpers.StopProcesses(lifecycle)
	})

	Context("when the daemon won't start", func() {
		fakeDeamonRunner = func(signals <-chan os.Signal, ready chan<- struct{}) error {
			close(ready)
			select {
			case signal := <-signals:
				return errors.New(signal.String())
			case <-time.After(1 * time.Second):
				// Daemon "crashes" after a while
			}
			return nil
		}

		It("times out", func() {
			err := <-lifecycle.Wait()
			Ω(err).Should(HaveOccurred())
			Ω(err.Error()).Should(ContainSubstring("Timed out waiting for docker daemon to start"))
		})

		Context("and the process is interrupted", func() {
			BeforeEach(func() {
				lifecycle.Signal(os.Interrupt)
			})

			It("exists with error", func() {
				err := <-lifecycle.Wait()
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(ContainSubstring("fake_docker_daemon exited with error: interrupt"))
				Ω(err.Error()).Should(ContainSubstring("builder exited with error: interrupt"))
			})
		})

	})
})
