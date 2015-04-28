package helpers_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/docker_app_lifecycle"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/docker/docker/nat"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega/ghttp"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/helpers"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/protocol"
)

var _ = Describe("Builder helpers", func() {
	var (
		server    *ghttp.Server
		endpoint1 *ghttp.Server
		endpoint2 *ghttp.Server
	)

	setupPingableRegistry := func() {
		server.AllowUnhandledRequests = true
		server.AppendHandlers(
			ghttp.VerifyRequest("GET", "/v1/_ping"),
		)
	}

	setupRegistry := func() {
		server.AppendHandlers(
			ghttp.VerifyRequest("GET", "/v1/_ping"),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/repositories/some_user/some_repo/images"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Docker-Token", "token-1,token-2")
					w.Header().Add("X-Docker-Endpoints", endpoint1.HTTPTestServer.Listener.Addr().String())
					w.Header().Add("X-Docker-Endpoints", endpoint2.HTTPTestServer.Listener.Addr().String())
					w.Write([]byte(`[
	                           {"id": "id-1", "checksum": "sha-1"},
	                           {"id": "id-2", "checksum": "sha-2"},
	                           {"id": "id-3", "checksum": "sha-3"}
	                       ]`))
				}),
			),
		)

		endpoint1.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/repositories/some_user/some_repo/tags"),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Write([]byte(`{
	                           "latest": "id-1",
	                           "some-tag": "id-2"
	                       }`))
				}),
			),
		)
	}

	resultJSON := func(filename string) []byte {
		resultInfo, err := ioutil.ReadFile(filename)
		Expect(err).NotTo(HaveOccurred())

		return resultInfo
	}

	Describe("ParseDockerURL", func() {
		It("should return a repo and tag", func() {
			parts, _ := url.Parse("docker://foobar:5123/baz/bot#test")
			repoName, tag := helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("foobar:5123/baz/bot"))
			Expect(tag).To(Equal("test"))

			parts, _ = url.Parse("docker:///baz/bot#test")
			repoName, tag = helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("baz/bot"))
			Expect(tag).To(Equal("test"))

			parts, _ = url.Parse("docker:///bot#test")
			repoName, tag = helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("bot"))
			Expect(tag).To(Equal("test"))

			parts, _ = url.Parse("docker:///xyz#123")
			repoName, tag = helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("xyz"))
			Expect(tag).To(Equal("123"))

			parts, _ = url.Parse("docker:///a:123/b/c#456")
			repoName, tag = helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("a:123/b/c"))
			Expect(tag).To(Equal("456"))
		})

		It("should default to the latest tag", func() {
			parts, _ := url.Parse("docker://a/b/c#latest")
			repoName, tag := helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("a/b/c"))
			Expect(tag).To(Equal("latest"))

			parts, _ = url.Parse("docker://foobar:5123/baz/bot")
			repoName, tag = helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("foobar:5123/baz/bot"))
			Expect(tag).To(Equal("latest"))

			parts, _ = url.Parse("docker:///baz/bot")
			repoName, tag = helpers.ParseDockerURL(parts)
			Expect(repoName).To(Equal("baz/bot"))
			Expect(tag).To(Equal("latest"))
		})
	})

	Describe("ParseDockerRef", func() {
		It("should return a repo and tag", func() {
			repoName, tag := helpers.ParseDockerRef("foobar:5123/baz/bot:test")
			Expect(repoName).To(Equal("foobar:5123/baz/bot"))
			Expect(tag).To(Equal("test"))

			repoName, tag = helpers.ParseDockerRef("baz/bot:test")
			Expect(repoName).To(Equal("baz/bot"))
			Expect(tag).To(Equal("test"))

			repoName, tag = helpers.ParseDockerRef("bot:test")
			Expect(repoName).To(Equal("bot"))
			Expect(tag).To(Equal("test"))

			repoName, tag = helpers.ParseDockerRef("xyz:123")
			Expect(repoName).To(Equal("xyz"))
			Expect(tag).To(Equal("123"))

			repoName, tag = helpers.ParseDockerRef("a:123/b/c:456")
			Expect(repoName).To(Equal("a:123/b/c"))
			Expect(tag).To(Equal("456"))
		})

		It("should default to the latest tag", func() {
			repoName, tag := helpers.ParseDockerRef("a/b/c")
			Expect(repoName).To(Equal("a/b/c"))
			Expect(tag).To(Equal("latest"))

			repoName, tag = helpers.ParseDockerRef("foobar:5123/baz/bot")
			Expect(repoName).To(Equal("foobar:5123/baz/bot"))
			Expect(tag).To(Equal("latest"))

			repoName, tag = helpers.ParseDockerRef("baz/bot")
			Expect(repoName).To(Equal("baz/bot"))
			Expect(tag).To(Equal("latest"))
		})
	})

	Describe("FetchMetadata", func() {
		var registryHost string
		var repoName string
		var tag string
		var insecureRegistries []string

		BeforeEach(func() {
			server = ghttp.NewServer()
			endpoint1 = ghttp.NewServer()
			endpoint2 = ghttp.NewServer()

			parts, _ := url.Parse(server.URL())
			registryHost = parts.Host

			repoName = ""
			tag = "latest"

			insecureRegistries = []string{}
		})

		Context("with an invalid host", func() {
			BeforeEach(func() {
				setupPingableRegistry()
				repoName = "qwer:5123/some_user/some_repo"
			})

			It("should error", func() {
				_, err := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with an unknown repository", func() {
			BeforeEach(func() {
				setupPingableRegistry()
				repoName = registryHost + "/some_user/not_some_repo"
			})
			It("should error", func() {
				_, err := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with an unknown tag", func() {
			BeforeEach(func() {
				setupPingableRegistry()
				repoName = registryHost + "/some_user/some_repo"
				tag = "not_some_tag"
			})

			It("should error", func() {
				_, err := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with a valid repository reference", func() {
			BeforeEach(func() {
				setupRegistry()

				repoName = registryHost + "/some_user/some_repo"

				endpoint1.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/id-1/json"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Add("X-Docker-Size", "789")
							w.Write([]byte(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["/dockerapp", "-foobar", "bazbot"]}}`))
						}),
					),
				)
			})

			It("should not error", func() {
				_, err := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the top-most image layer metadata", func() {
				img, _ := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(img).NotTo(BeNil())
				Expect(img.Config).NotTo(BeNil())
				Expect(img.Config.Cmd).NotTo(BeNil())
				Expect(img.Config.Cmd).To(Equal([]string{"/dockerapp", "-foobar", "bazbot"}))
			})
		})

		Context("with a valid repository:tag reference", func() {
			BeforeEach(func() {
				setupRegistry()

				repoName = registryHost + "/some_user/some_repo"
				tag = "some-tag"

				endpoint1.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/id-2/json"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Add("X-Docker-Size", "123")
							w.Write([]byte(`{"id":"layer-2","parent":"parent-2","Config":{"Cmd":["/dockerapp", "arg1", "arg2"]}}`))
						}),
					),
				)
			})

			It("should not error", func() {
				_, err := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the top-most image layer metadata", func() {
				img, _ := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(img).NotTo(BeNil())
				Expect(img.Config).NotTo(BeNil())
				Expect(img.Config.Cmd).To(Equal([]string{"/dockerapp", "arg1", "arg2"}))
			})
		})

		Context("when the image exposes custom ports", func() {
			BeforeEach(func() {
				setupRegistry()

				repoName = registryHost + "/some_user/some_repo"

				endpoint1.AppendHandlers(
					ghttp.CombineHandlers(
						// Docker Image config: https://github.com/docker/docker/blob/master/runconfig/fixtures/container_config_1_19.json
						ghttp.VerifyRequest("GET", "/v1/images/id-1/json"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Add("X-Docker-Size", "789")
							w.Write([]byte(`{"id": "layer-1",
							                 "parent": "parent-1",
															 "Config": {
																 "Cmd": ["/dockerapp", "-foobar", "bazbot"],
																 "ExposedPorts": {
                                   "80/tcp": {},
																	 "53/udp": {}
                                  }
															  }
														  }`))
						}),
					),
				)
			})

			It("should not error", func() {
				_, err := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the exposed ports", func() {
				img, _ := helpers.FetchMetadata(repoName, tag, insecureRegistries)
				Expect(img.Config).NotTo(BeNil())

				Expect(img.Config.ExposedPorts).To(HaveKeyWithValue(nat.NewPort("udp", "53"), struct{}{}))
				Expect(img.Config.ExposedPorts).To(HaveKeyWithValue(nat.NewPort("tcp", "80"), struct{}{}))
			})
		})

	})

	Context("SaveMetadata", func() {
		var metadata protocol.DockerImageMetadata
		var outputDir string

		BeforeEach(func() {
			metadata = protocol.DockerImageMetadata{
				ExecutionMetadata: protocol.ExecutionMetadata{
					Cmd:        []string{"fake-arg1", "fake-arg2"},
					Entrypoint: []string{"fake-cmd", "fake-arg0"},
					Workdir:    "/fake-workdir",
				},
				DockerImage: "cloudfoundry/diego-docker-app",
			}
		})

		Context("to an unwritable path on disk", func() {
			It("should error", func() {
				err := helpers.SaveMetadata("////tmp/", &metadata)
				Expect(err).To(HaveOccurred())
			})
		})
		Context("with a writable path on disk", func() {

			BeforeEach(func() {
				var err error
				outputDir, err = ioutil.TempDir(os.TempDir(), "metadata")
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				os.RemoveAll(outputDir)
			})

			It("should output a json file", func() {
				filename := path.Join(outputDir, "result.json")
				err := helpers.SaveMetadata(filename, &metadata)
				Expect(err).NotTo(HaveOccurred())
				_, err = os.Stat(filename)
				Expect(err).NotTo(HaveOccurred())
			})

			Describe("the json", func() {
				verifyMetadata := func(expectedEntryPoint []string, expectedStartCmd string) {
					err := helpers.SaveMetadata(path.Join(outputDir, "result.json"), &metadata)
					Expect(err).NotTo(HaveOccurred())
					result := resultJSON(path.Join(outputDir, "result.json"))

					var stagingResult docker_app_lifecycle.StagingDockerResult
					err = json.Unmarshal(result, &stagingResult)
					Expect(err).NotTo(HaveOccurred())

					Expect(stagingResult.ExecutionMetadata).NotTo(BeEmpty())
					Expect(stagingResult.DetectedStartCommand).NotTo(BeEmpty())

					var executionMetadata protocol.ExecutionMetadata
					err = json.Unmarshal([]byte(stagingResult.ExecutionMetadata), &executionMetadata)
					Expect(err).NotTo(HaveOccurred())

					Expect(executionMetadata.Cmd).To(Equal(metadata.ExecutionMetadata.Cmd))
					Expect(executionMetadata.Entrypoint).To(Equal(expectedEntryPoint))
					Expect(executionMetadata.Workdir).To(Equal(metadata.ExecutionMetadata.Workdir))

					Expect(stagingResult.DetectedStartCommand).To(HaveLen(1))
					Expect(stagingResult.DetectedStartCommand).To(HaveKeyWithValue("web", expectedStartCmd))

					Expect(stagingResult.DockerImage).To(Equal(metadata.DockerImage))
				}

				It("should contain the metadata", func() {
					verifyMetadata(metadata.ExecutionMetadata.Entrypoint, "fake-cmd fake-arg0 fake-arg1 fake-arg2")
				})

				Context("when the EntryPoint is empty", func() {
					BeforeEach(func() {
						metadata.ExecutionMetadata.Entrypoint = []string{}
					})

					It("contains all but the EntryPoint", func() {
						verifyMetadata(nil, "fake-arg1 fake-arg2")
					})
				})

				Context("when the EntryPoint is nil", func() {
					BeforeEach(func() {
						metadata.ExecutionMetadata.Entrypoint = nil
					})

					It("contains all but the EntryPoint", func() {
						verifyMetadata(metadata.ExecutionMetadata.Entrypoint, "fake-arg1 fake-arg2")
					})
				})
			})
		})
	})
})
