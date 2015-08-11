package main_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit/ginkgomon"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/tedsuo/ifrit/grouper"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/builder"
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
		ginkgomon.Interrupt(lifecycle)
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
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Timed out waiting for docker daemon to start"))
		})

		Context("and the process is interrupted", func() {
			BeforeEach(func() {
				lifecycle.Signal(os.Interrupt)
			})

			It("exists with error", func() {
				err := <-lifecycle.Wait()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake_docker_daemon exited with error: interrupt"))
				Expect(err.Error()).To(ContainSubstring("builder exited with error: interrupt"))
			})
		})
	})

	Describe("cached tags generation", func() {
		var (
			builder            main.Builder
			dockerRegistryIPs  []string
			dockerRegistryHost string
			dockerRegistryPort int
		)

		generateTag := func() (string, string) {
			image, err := builder.GenerateImageName()
			Expect(err).NotTo(HaveOccurred())

			parts := strings.Split(image, "/")
			Expect(parts).To(HaveLen(2))

			return parts[0], parts[1]
		}

		imageGeneration := func() {
			generatedImageNames := make(map[string]int)

			uniqueImageNames := func() bool {
				_, imageName := generateTag()
				generatedImageNames[imageName]++

				for key := range generatedImageNames {
					if generatedImageNames[key] != 1 {
						return false
					}
				}

				return true
			}

			It("generates different image names", func() {
				Consistently(uniqueImageNames).Should(BeTrue())
			})
		}

		BeforeEach(func() {
			builder = main.Builder{
				DockerRegistryIPs:  dockerRegistryIPs,
				DockerRegistryHost: dockerRegistryHost,
				DockerRegistryPort: dockerRegistryPort,
			}
		})

		Context("when there are several Docker Registry addresses", func() {
			dockerRegistryIPs = []string{"one", "two", "three", "four"}
			dockerRegistryHost = "docker-registry.service.cf.internal"
			dockerRegistryPort = 8080

			Describe("addresses", func() {
				hostOnly := func() string {
					address, _ := generateTag()
					return address
				}

				It("uses docker registry host and port", func() {
					Consistently(hostOnly).Should(Equal(fmt.Sprintf("%s:%d", dockerRegistryHost, dockerRegistryPort)))
				})
			})

			Describe("image names", imageGeneration)
		})

		Context("when there is a single Docker Registry address", func() {
			dockerRegistryIPs = []string{"one"}

			Describe("image names", imageGeneration)
		})
	})

})
