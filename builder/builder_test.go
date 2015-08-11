package main_test

import (
	"fmt"
	"io/ioutil"
	"net"
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
		dockerRegistryIPs          string
		dockerRegistryHost         string
		dockerRegistryPort         string
		dockerDaemonExecutablePath string
		cacheDockerImage           bool
		dockerLoginServer          string
		dockerUser                 string
		dockerPassword             string
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

	setupRegistryResponse := func(response string) {
		fakeDockerRegistry.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/images/id-1/json"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Add("X-Docker-Size", "789")
					w.Write([]byte(response))
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
		dockerRegistryIPs = ""
		dockerRegistryHost = ""
		dockerRegistryPort = ""
		dockerDaemonExecutablePath = ""
		cacheDockerImage = false
		dockerLoginServer = ""
		dockerUser = ""
		dockerPassword = ""
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
		if len(dockerRegistryIPs) > 0 {
			args = append(args, "-dockerRegistryIPs", dockerRegistryIPs)
		}
		if len(dockerRegistryHost) > 0 {
			args = append(args, "-dockerRegistryHost", dockerRegistryHost)
		}
		if len(dockerRegistryPort) > 0 {
			args = append(args, "-dockerRegistryPort", dockerRegistryPort)
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
		if len(dockerUser) > 0 {
			args = append(args, "-dockerUser", dockerUser)
		}
		if len(dockerPassword) > 0 {
			args = append(args, "-dockerPassword", dockerPassword)
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
				dockerRegistryIPs, _, err = net.SplitHostPort(parts.Host)
				Expect(err).NotTo(HaveOccurred())

				dockerRegistryHost = "url"
			})

			It("should exit with an error", func() {
				session := setupBuilder()
				Eventually(session.Err).Should(gbytes.Say(fmt.Sprintf("docker daemon not found in %s", dockerDaemonExecutablePath)))
				Eventually(session).Should(gexec.Exit(1))
			})
		})

		Context("when docker registry host is invalid", func() {
			BeforeEach(func() {
				cacheDockerImage = true
				dockerImageURL = buildDockerImageURL()

				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				dockerRegistryIPs, _, err = net.SplitHostPort(parts.Host)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("host is missing", func() {
				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("missing flag: dockerRegistryHost required"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

			Context("host contains schema", func() {
				BeforeEach(func() {
					dockerRegistryHost = "https://docker-registry"
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("invalid host format https://docker-registry"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

			Context("host contains port", func() {
				BeforeEach(func() {
					dockerRegistryHost = "docker-registry:8080"
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("invalid host format docker-registry:8080"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})
		})

		Context("when docker registry port is invalid", func() {
			BeforeEach(func() {
				cacheDockerImage = true
				dockerImageURL = buildDockerImageURL()

				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				dockerRegistryIPs, _, err = net.SplitHostPort(parts.Host)
				Expect(err).NotTo(HaveOccurred())

				dockerRegistryHost = "host"
			})

			Context("and port is negative", func() {
				BeforeEach(func() {
					dockerRegistryPort = "-1"
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("negative port number"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

			Context("and port is out of range", func() {
				BeforeEach(func() {
					dockerRegistryPort = "65536"
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say("port number too big 65536"))
					Eventually(session).Should(gexec.Exit(1))
				})
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
				dockerRegistryHost = "docker-registry.service.cf.internal"
			})

			Context("with a valid docker url", dockerURLFunc)
			Context("with a valid docker ref", dockerRefFunc)
		})

		Context("with a valid docker registries", func() {
			BeforeEach(func() {
				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				host, _, err := net.SplitHostPort(parts.Host)
				Expect(err).NotTo(HaveOccurred())

				dockerRegistryIPs = host + ",10.244.2.6"
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
					Eventually(session.Err).Should(gbytes.Say("missing flag: dockerRegistryIPs required"))
					Eventually(session).Should(gexec.Exit(1))
				})
			})
		})

		Context("with invalid docker registry credentials", func() {
			BeforeEach(func() {
				parts, err := url.Parse(fakeDockerRegistry.URL())
				Expect(err).NotTo(HaveOccurred())
				host, _, err := net.SplitHostPort(parts.Host)
				Expect(err).NotTo(HaveOccurred())
				dockerRegistryIPs = host + ",10.244.2.6"

				dockerRegistryHost = "docker-registry.service.cf.internal"
				dockerRef = fmt.Sprintf("%s/some-repo", parts.Host)
				cacheDockerImage = true
			})

			whenFlagsMissing := func() {
				session := setupBuilder()
				Eventually(session.Err).Should(gbytes.Say("missing flags: dockerUser, dockerPassword and dockerEmail required simultaneously"))
				Eventually(session).Should(gexec.Exit(1))
			}

			Context("without user", func() {
				BeforeEach(func() {
					dockerLoginServer = "http://loginserver.com"
					dockerPassword = "password"
					dockerEmail = "mail@example.com"
				})

				It("errors", whenFlagsMissing)
			})

			Context("without password", func() {
				BeforeEach(func() {
					dockerLoginServer = "http://loginserver.com"
					dockerUser = "user"
					dockerEmail = "email@example.com"
				})

				It("errors", whenFlagsMissing)
			})

			Context("without email", func() {
				BeforeEach(func() {
					dockerLoginServer = "http://loginserver.com"
					dockerUser = "user"
					dockerPassword = "password"
				})

				It("errors", whenFlagsMissing)
			})

			Context("with invalid email", func() {
				BeforeEach(func() {
					dockerUser = "user"
					dockerPassword = "password"
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
					dockerUser = "user"
					dockerPassword = "password"
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

		Context("with exposed ports in image metadata", func() {
			BeforeEach(func() {
				dockerImageURL = buildDockerImageURL()
				cacheDockerImage = false

				setupFakeDockerRegistry()
			})

			Context("with correct ports", func() {
				BeforeEach(func() {
					setupRegistryResponse(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["-bazbot","-foobar"],"Entrypoint":["/dockerapp","-t"],"WorkingDir":"/workdir", "ExposedPorts": {"8081/udp":{}, "8081/tcp":{}, "8079/tcp":{}, "8078/udp":{}} }}`)
				})

				Describe("the json", func() {
					It("should contain sorted exposed ports", func() {
						session := setupBuilder()
						Eventually(session, 10*time.Second).Should(gexec.Exit(0))

						result := resultJSON()

						Expect(result).To(ContainSubstring(`\"ports\":[{\"Port\":8078,\"Protocol\":\"udp\"},{\"Port\":8079,\"Protocol\":\"tcp\"},{\"Port\":8081,\"Protocol\":\"tcp\"},{\"Port\":8081,\"Protocol\":\"udp\"}]`))
					})
				})
			})

			Context("with incorrect port", func() {
				Context("bigger than allowed uint16 range", func() {
					BeforeEach(func() {
						setupRegistryResponse(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["-bazbot","-foobar"],"Entrypoint":["/dockerapp","-t"],"WorkingDir":"/workdir", "ExposedPorts": {"8081/tcp":{}, "65536/tcp":{}, "8078/udp":{}} }}`)
					})

					It("should error", func() {
						session := setupBuilder()
						Eventually(session.Err).Should(gbytes.Say("value out of range"))
						Eventually(session, 10*time.Second).Should(gexec.Exit(2))
					})
				})

				Context("negative port", func() {
					BeforeEach(func() {
						setupRegistryResponse(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["-bazbot","-foobar"],"Entrypoint":["/dockerapp","-t"],"WorkingDir":"/workdir", "ExposedPorts": {"8081/tcp":{}, "-8078/udp":{}} }}`)
					})

					It("should error", func() {
						session := setupBuilder()
						Eventually(session.Err).Should(gbytes.Say("invalid syntax"))
						Eventually(session, 10*time.Second).Should(gexec.Exit(2))
					})
				})

				Context("not a number", func() {
					BeforeEach(func() {
						setupRegistryResponse(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["-bazbot","-foobar"],"Entrypoint":["/dockerapp","-t"],"WorkingDir":"/workdir", "ExposedPorts": {"8081/tcp":{}, "8a8a/udp":{}} }}`)
					})

					It("should error", func() {
						session := setupBuilder()
						Eventually(session.Err).Should(gbytes.Say("invalid syntax"))
						Eventually(session, 10*time.Second).Should(gexec.Exit(2))
					})
				})
			})
		})

		Context("with specified user in image metadata", func() {
			BeforeEach(func() {
				dockerImageURL = buildDockerImageURL()
				cacheDockerImage = false

				setupFakeDockerRegistry()
				setupRegistryResponse(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["-bazbot","-foobar"],"Entrypoint":["/dockerapp","-t"],"WorkingDir":"/workdir", "User": "custom-user"}}`)
			})

			Describe("the json", func() {
				It("should contain custom user", func() {
					session := setupBuilder()
					Eventually(session, 10*time.Second).Should(gexec.Exit(0))

					result := resultJSON()

					Expect(result).To(ContainSubstring(`\"user\":\"custom-user\"`))
				})
			})

		})
	})
})
