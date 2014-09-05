package helpers_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/ghttp"

	"github.com/cloudfoundry-incubator/docker-circus/helpers"
	"github.com/cloudfoundry-incubator/docker-circus/protocol"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
)

var _ = Describe("Tailor helpers", func() {
	var (
		dockerImageURL string
		server         *ghttp.Server
		endpoint1      *ghttp.Server
		endpoint2      *ghttp.Server
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
		Ω(err).ShouldNot(HaveOccurred())

		return resultInfo
	}

	Describe("FetchMetadata", func() {
		var registryHost string
		var parts *url.URL

		BeforeEach(func() {
			server = ghttp.NewServer()
			endpoint1 = ghttp.NewServer()
			endpoint2 = ghttp.NewServer()

			parts, _ := url.Parse(server.URL())
			registryHost = parts.Host
		})

		JustBeforeEach(func() {
			parts, _ = url.Parse(dockerImageURL)
		})

		Context("with an invalid host", func() {
			BeforeEach(func() {
				setupPingableRegistry()
				dockerImageURL = "docker://qwer:5123/some_user/some_repo"
			})
			It("should error", func() {
				_, err := helpers.FetchMetadata(parts)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("with an unknown repository", func() {
			BeforeEach(func() {
				setupPingableRegistry()
				dockerImageURL = "docker://" + registryHost + "/some_user/not_some_repo"
			})
			It("should error", func() {
				_, err := helpers.FetchMetadata(parts)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("with an unknown tag", func() {
			BeforeEach(func() {
				setupPingableRegistry()
				dockerImageURL = "docker://" + registryHost + "/some_user/some_repo#not_some_tag"
			})
			It("should error", func() {
				_, err := helpers.FetchMetadata(parts)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("with a valid repository reference", func() {
			BeforeEach(func() {
				setupRegistry()

				dockerImageURL = "docker://" + registryHost + "/some_user/some_repo"

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
				_, err := helpers.FetchMetadata(parts)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should return the top-most image layer metadata", func() {
				img, _ := helpers.FetchMetadata(parts)
				Ω(img).ShouldNot(BeNil())
				Ω(img.Config).ShouldNot(BeNil())
				Ω(img.Config.Cmd).Should(Equal([]string{"/dockerapp", "-foobar", "bazbot"}))
			})
		})

		Context("with a valid repository:tag reference", func() {
			BeforeEach(func() {
				setupRegistry()

				dockerImageURL = "docker://" + registryHost + "/some_user/some_repo#some-tag"

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
				_, err := helpers.FetchMetadata(parts)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should return the top-most image layer metadata", func() {
				img, _ := helpers.FetchMetadata(parts)
				Ω(img).ShouldNot(BeNil())
				Ω(img.Config).ShouldNot(BeNil())
				Ω(img.Config.Cmd).Should(Equal([]string{"/dockerapp", "arg1", "arg2"}))
			})
		})
	})

	Context("SaveMetadata", func() {
		metadata := protocol.ExecutionMetadata{
			Cmd:        []string{"fake-arg1", "fake-arg2"},
			Entrypoint: []string{"fake-cmd", "fake-arg0"},
		}

		var outputDir string
		Context("to an unwritable path on disk", func() {
			It("should error", func() {
				err := helpers.SaveMetadata("////tmp/", &metadata)
				Ω(err).Should(HaveOccurred())
			})
		})
		Context("with a writable path on disk", func() {

			BeforeEach(func() {
				var err error
				outputDir, err = ioutil.TempDir(os.TempDir(), "metadata")
				Ω(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				os.RemoveAll(outputDir)
			})

			It("should output a json file", func() {
				filename := path.Join(outputDir, "result.json")
				err := helpers.SaveMetadata(filename, &metadata)
				Ω(err).ShouldNot(HaveOccurred())
				_, err = os.Stat(filename)
				Ω(err).ShouldNot(HaveOccurred())

			})

			Describe("the json", func() {
				It("should contain the metadata", func() {
					err := helpers.SaveMetadata(path.Join(outputDir, "result.json"), &metadata)
					Ω(err).ShouldNot(HaveOccurred())
					result := resultJSON(path.Join(outputDir, "result.json"))

					var stagingResult models.StagingDockerResult
					err = json.Unmarshal(result, &stagingResult)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(stagingResult.ExecutionMetadata).ShouldNot(BeEmpty())

					var executionMetadata protocol.ExecutionMetadata
					err = json.Unmarshal([]byte(stagingResult.ExecutionMetadata), &executionMetadata)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(executionMetadata.Cmd).Should(Equal(metadata.Cmd))
					Ω(executionMetadata.Entrypoint).Should(Equal(metadata.Entrypoint))
				})
			})
		})
	})
})
