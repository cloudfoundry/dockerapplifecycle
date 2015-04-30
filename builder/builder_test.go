package main_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"time"

	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega/gbytes"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega/gexec"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega/ghttp"
)

var _ = Describe("Building", func() {
	var (
		builderCmd                 *exec.Cmd
		dockerRef                  string
		dockerImageURL             string
		insecureDockerRegistries   string
		dockerRegistryAddresses    string
		dockerDaemonExecutablePath string
		cacheDockerImage           bool
		dockerLoginServer          string
		dockerAuthToken            string
		dockerEmail                string
		outputMetadataDir          string
		outputMetadataJSONFilename string
		fakeDockerRegistry         *ghttp.Server
	)

	setupBuilder := func() *gexec.Session {
		session, err := gexec.Start(
			builderCmd,
			GinkgoWriter,
			GinkgoWriter,
		)
		Expect(err).NotTo(HaveOccurred())

		return session
	}

	setupFakeDockerRegistry := func() {
		fakeDockerRegistry.AppendHandlers(
			ghttp.VerifyRequest("GET", "/v1/_ping"),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/repositories/some-repo/images"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Docker-Token", "token-1,token-2")
					w.Write([]byte(`[
                            {"id": "id-1", "checksum": "sha-1"},
                            {"id": "id-2", "checksum": "sha-2"},
                            {"id": "id-3", "checksum": "sha-3"}
                        ]`))
				}),
			),
		)

		fakeDockerRegistry.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/repositories/library/some-repo/tags"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Write([]byte(`{
                            "latest": "id-1",
                            "some-other-tag": "id-2"
                        }`))
				}),
			),
		)
	}

	buildDockerImageURL := func() string {
		parts, err := url.Parse(fakeDockerRegistry.URL())
		Expect(err).NotTo(HaveOccurred())
		return fmt.Sprintf("docker://%s/some-repo", parts.Host)
	}

	resultJSON := func() []byte {
		resultInfo, err := ioutil.ReadFile(outputMetadataJSONFilename)
		Expect(err).NotTo(HaveOccurred())

		return resultInfo
	}

	BeforeEach(func() {
		var err error

		dockerRef = ""
		dockerImageURL = ""
		insecureDockerRegistries = ""
		dockerRegistryAddresses = ""
		dockerDaemonExecutablePath = ""
		cacheDockerImage = false
		dockerLoginServer = ""
		dockerAuthToken = ""
		dockerEmail = ""

		outputMetadataDir, err = ioutil.TempDir("", "building-result")
		Expect(err).NotTo(HaveOccurred())

		outputMetadataJSONFilename = path.Join(outputMetadataDir, "result.json")

		fakeDockerRegistry = ghttp.NewServer()
	})

	AfterEach(func() {
		os.RemoveAll(outputMetadataDir)
	})

	JustBeforeEach(func() {
		args := []string{"-outputMetadataJSONFilename", outputMetadataJSONFilename}

		if len(dockerImageURL) > 0 {
			args = append(args, "-dockerImageURL", dockerImageURL)
		}
		if len(dockerRef) > 0 {
			args = append(args, "-dockerRef", dockerRef)
		}
		if len(insecureDockerRegistries) > 0 {
			args = append(args, "-insecureDockerRegistries", insecureDockerRegistries)
		}
		if len(dockerRegistryAddresses) > 0 {
			args = append(args, "-dockerRegistryAddresses", dockerRegistryAddresses)
		}
		if cacheDockerImage {
			args = append(args, "-cacheDockerImage")
		}
		if len(dockerDaemonExecutablePath) > 0 {
			args = append(args, "-dockerDaemonExecutablePath", dockerDaemonExecutablePath)
		}
		if len(dockerLoginServer) > 0 {
			args = append(args, "-dockerLoginServer", dockerLoginServer)
		}
		if len(dockerAuthToken) > 0 {
			args = append(args, "-dockerAuthToken", dockerAuthToken)
		}
		if len(dockerEmail) > 0 {
			args = append(args, "-dockerEmail", dockerEmail)
		}

		builderCmd = exec.Command(builderPath, args...)

		builderCmd.Env = os.Environ()
	})

	Context("when running the main", func() {
		Context("with no docker image arg specified", func() {
			It("should exit with an error", func() {
				session := setupBuilder()
				Eventually(session.Err).Should(gbytes.Say("missing flag: dockerImageURL or dockerRef required"))
				Eventually(session).Should(gexec.Exit(1))
			})
		})

		Context("with an invalid output path", func() {
			It("should exit with an error", func() {
				session := setupBuilder()
				Eventually(session).Should(gexec.Exit(1))
			})
		})

		Context("with an invalid docker registry addesses", func() {
			var invalidRegistryAddress string

			Context("when an address has a scheme", func() {
				BeforeEach(func() {
					invalidRegistryAddress = "://10.244.2.6:5050"
					insecureDockerRegistries = "10.244.2.7:8080, " + invalidRegistryAddress

					dockerImageURL = buildDockerImageURL()
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say(fmt.Sprintf("invalid value \"%s\" for flag -insecureDockerRegistries: no scheme allowed for Docker Registry \\[%s\\]", insecureDockerRegistries, invalidRegistryAddress)))
					Eventually(session).Should(gexec.Exit(2))
				})
			})

			Context("when an address has no port", func() {
				BeforeEach(func() {
					invalidRegistryAddress = "10.244.2.6"
					insecureDockerRegistries = invalidRegistryAddress + " , 10.244.2.7:8080"

					dockerImageURL = buildDockerImageURL()
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say(fmt.Sprintf("invalid value \"%s\" for flag -insecureDockerRegistries: ip:port expected for Docker Registry \\[%s\\]", insecureDockerRegistries, invalidRegistryAddress)))
					Eventually(session).Should(gexec.Exit(2))
				})
			})
		})

		Context("when docker daemon dir is invalid", func() {
			BeforeEach(func() {
				cacheDockerImage = true
				dockerImageURL = buildDockerImageURL()
				dockerDaemonExecutablePath = "missing_dir/docker"

				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				dockerRegistryAddresses = parts.Host
			})

			It("should exit with an error", func() {
				session := setupBuilder()
				Eventually(session.Err).Should(gbytes.Say(fmt.Sprintf("docker daemon not found in %s", dockerDaemonExecutablePath)))
				Eventually(session).Should(gexec.Exit(1))
			})
		})

		testValid := func() {
			BeforeEach(func() {
				setupFakeDockerRegistry()
				fakeDockerRegistry.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/id-1/json"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Add("X-Docker-Size", "789")
							w.Write([]byte(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["-bazbot","-foobar"],"Entrypoint":["/dockerapp","-t"],"WorkingDir":"/workdir"}}`))
						}),
					),
				)
			})

			It("should exit successfully", func() {
				session := setupBuilder()
				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
			})

			Describe("the json", func() {
				It("should contain the execution metadata", func() {
					session := setupBuilder()
					Eventually(session, 10*time.Second).Should(gexec.Exit(0))

					result := resultJSON()

					Expect(result).To(ContainSubstring(`\"cmd\":[\"-bazbot\",\"-foobar\"]`))
					Expect(result).To(ContainSubstring(`\"entrypoint\":[\"/dockerapp\",\"-t\"]`))
					Expect(result).To(ContainSubstring(`\"workdir\":\"/workdir\"`))
				})
			})
		}

		dockerURLFunc := func() {
			BeforeEach(func() {
				dockerImageURL = buildDockerImageURL()
			})

			testValid()
		}

		dockerRefFunc := func() {
			BeforeEach(func() {
				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				dockerRef = fmt.Sprintf("%s/some-repo", parts.Host)
			})

			testValid()
		}

		Context("with a valid insecure docker registries", func() {
			BeforeEach(func() {
				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				insecureDockerRegistries = parts.Host + ",10.244.2.6:80"
			})

			Context("with a valid docker url", dockerURLFunc)
			Context("with a valid docker ref", dockerRefFunc)
		})

		Context("with a valid docker registries", func() {
			BeforeEach(func() {
				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				dockerRegistryAddresses = parts.Host + ",10.244.2.6:80"
			})

			Context("with a valid docker url", dockerURLFunc)
			Context("with a valid docker ref", dockerRefFunc)
		})

		Context("without docker registries", func() {
			Context("when there is no caching requested", func() {
				Context("with a valid docker url", dockerURLFunc)
				Context("with a valid docker ref", dockerRefFunc)
			})

			Context("when caching is requested", func() {
				var session *gexec.Session

				BeforeEach(func() {
					dockerImageURL = buildDockerImageURL()
					cacheDockerImage = true
				})

				It("should error", func() {
					session = setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("missing flag: dockerRegistryAddresses required"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})
		})

		Context("with invalid docker registry credentials", func() {
			BeforeEach(func() {
				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				dockerRegistryAddresses = parts.Host + ",10.244.2.6:80"
				dockerRef = fmt.Sprintf("%s/some-repo", parts.Host)
				cacheDockerImage = true
			})

			Context("without auth token", func() {
				BeforeEach(func() {
					dockerLoginServer = "http://loginserver.com"
					dockerEmail = "mail@example.com"
				})

				It("errors", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("missing flags: dockerLoginServer, dockerAuthToken and dockerEmail required simultaneously"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

			Context("without email", func() {
				BeforeEach(func() {
					dockerLoginServer = "http://loginserver.com"
					dockerAuthToken = "token"
				})

				It("errors", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("missing flags: dockerLoginServer, dockerAuthToken and dockerEmail required simultaneously"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

			Context("without login server", func() {
				BeforeEach(func() {
					dockerAuthToken = "token"
					dockerEmail = "mail@example.com"
				})

				It("errors", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("missing flags: dockerLoginServer, dockerAuthToken and dockerEmail required simultaneously"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

			Context("with invalid email", func() {
				BeforeEach(func() {
					dockerAuthToken = "token"
					dockerEmail = "invalid email"
					dockerLoginServer = "http://loginserver.com"
				})

				It("errors", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say(regexp.QuoteMeta(fmt.Sprintf("invalid dockerEmail [%s]", dockerEmail))))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

			Context("with invalid login server URL", func() {
				BeforeEach(func() {
					dockerAuthToken = "token"
					dockerEmail = "mail@example.com"
					dockerLoginServer = "://missingSchema.com"
				})

				It("errors", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say(regexp.QuoteMeta(fmt.Sprintf("invalid dockerLoginServer [%s]", dockerLoginServer))))
					Eventually(session).Should(gexec.Exit(1))
				})
			})
		})

	})

})
