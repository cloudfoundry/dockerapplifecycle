package helpers_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"code.cloudfoundry.org/cfhttp"
	"code.cloudfoundry.org/dockerapplifecycle"
	"code.cloudfoundry.org/dockerapplifecycle/docker/nat"
	"code.cloudfoundry.org/dockerapplifecycle/helpers"
	"code.cloudfoundry.org/dockerapplifecycle/protocol"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("Builder helpers", func() {
	var (
		response string
		server   *ghttp.Server
	)

	BeforeEach(func() {
		bs, err := ioutil.ReadFile("manifest.yml")
		Expect(err).NotTo(HaveOccurred())
		response = string(bs)
	})

	setupPingableRegistry := func() {
		server.AllowUnhandledRequests = true
		server.AppendHandlers(
			ghttp.VerifyRequest("GET", "/v2/"),
		)
	}

	setupSlowRegistry := func(tag ...string) {
		tagString := "latest"
		if len(tag) > 0 {
			tagString = tag[0]
		}

		server.AppendHandlers(
			ghttp.RespondWith(200, "Push failed due to a network error. Please try again. If the problem persists, it may be due to a slow connection."),
			ghttp.RespondWith(200, "Push failed due to a network error. Please try again. If the problem persists, it may be due to a slow connection."),
			ghttp.RespondWith(200, "Push failed due to a network error. Please try again. If the problem persists, it may be due to a slow connection."),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v2/some_user/some_repo/manifests/"+tagString),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Docker-Token", "token-1,token-2")
					w.Write([]byte(response))
				}),
			),
		)
	}

	setupRegistry := func(tag ...string) {
		tagString := "latest"
		if len(tag) > 0 {
			tagString = tag[0]
		}

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v2/some_user/some_repo/manifests/"+tagString),
				ghttp.VerifyHeaderKV("Accept", manifest.DockerV2Schema1SignedMediaType, manifest.DockerV2Schema1MediaType),
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Docker-Token", "token-1,token-2")
					w.Write([]byte(response))
				}),
			),
		)
	}

	resultJSON := func(filename string) []byte {
		resultInfo, err := ioutil.ReadFile(filename)
		Expect(err).NotTo(HaveOccurred())

		return resultInfo
	}

	Describe("ParseDockerRef", func() {
		Context("when the repo image is from dockerhub", func() {
			It("prepends 'library/' to the repo Name if there is no '/' character", func() {
				repositoryURL, repoName, _ := helpers.ParseDockerRef("redis")
				Expect(repositoryURL).To(Equal("registry-1.docker.io"))
				Expect(repoName).To(Equal("library/redis"))
			})

			It("does not prepends 'library/' to the repo Name if there is a '/' ", func() {
				repositoryURL, repoName, _ := helpers.ParseDockerRef("b/c")
				Expect(repositoryURL).To(Equal("registry-1.docker.io"))
				Expect(repoName).To(Equal("b/c"))
			})
		})

		Context("When the registryURL is not dockerhub", func() {
			It("does not add a '/' character to a single repo name", func() {
				repositoryURL, repoName, _ := helpers.ParseDockerRef("foobar:5123/baz")
				Expect(repositoryURL).To(Equal("foobar:5123"))
				Expect(repoName).To(Equal("baz"))
			})
		})

		Context("Parsing tags", func() {
			It("should parse tags based off the last colon", func() {
				_, _, tag := helpers.ParseDockerRef("baz/bot:test")
				Expect(tag).To(Equal("test"))
			})

			It("should default the tag to latest", func() {
				_, _, tag := helpers.ParseDockerRef("redis")
				Expect(tag).To(Equal("latest"))
			})
		})
	})

	Describe("FetchMetadata", func() {
		var registryURL string
		var repoName string
		var tag string
		var insecureRegistries []string
		var ctx *types.SystemContext
		var username, password string

		BeforeEach(func() {
			server = ghttp.NewUnstartedServer()

			setupPingableRegistry()

			registryURL = server.Addr()

			repoName = ""
			tag = "latest"

			insecureRegistries = []string{}
		})

		JustBeforeEach(func() {
			fixturesPath := path.Join(os.Getenv("GOPATH"), "src/code.cloudfoundry.org/dockerapplifecycle/helpers/fixtures")
			tlsCA := path.Join(fixturesPath, "testCA.crt")
			tlsCert := path.Join(fixturesPath, "localhost.cert")
			tlsKey := path.Join(fixturesPath, "localhost.key")

			ctx = &types.SystemContext{
				DockerAuthConfig: &types.DockerAuthConfig{
					Username: username,
					Password: password,
				},
				DockerCertPath: fixturesPath,
			}
			for _, insecure := range insecureRegistries {
				if registryURL == insecure {
					ctx.DockerInsecureSkipTLSVerify = true
				}
			}

			if server.HTTPTestServer.TLS == nil {
				tlsConfig, err := cfhttp.NewTLSConfig(tlsCert, tlsKey, tlsCA)
				Expect(err).NotTo(HaveOccurred())
				tlsConfig.ClientAuth = tls.NoClientCert
				server.HTTPTestServer.TLS = tlsConfig
			}
			server.HTTPTestServer.StartTLS()
			var err error
			Expect(err).NotTo(HaveOccurred())
		})

		Context("with an invalid host", func() {
			BeforeEach(func() {
				registryURL = "qewr:5431"
				repoName = "some_user/some_repo"
			})

			It("should error", func() {
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with an unknown repository", func() {
			BeforeEach(func() {
				server.AllowUnhandledRequests = true
				repoName = "some_user/not_some_repo"
			})

			It("should error", func() {
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with an unknown tag", func() {
			BeforeEach(func() {
				server.AllowUnhandledRequests = true
				repoName = "some_user/some_repo"
				tag = "not_some_tag"
			})

			It("should error", func() {
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with a valid repository reference", func() {
			BeforeEach(func() {
				setupRegistry()

				repoName = "some_user/some_repo"
			})

			It("should not error", func() {
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the top-most image layer metadata", func() {
				img, _ := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(img).NotTo(BeNil())
				Expect(img.Config).NotTo(BeNil())
				Expect(img.Config.Cmd).NotTo(BeNil())
				Expect(img.Config.Cmd).To(Equal([]string{"dockerapp"}))
			})
		})

		Context("when the network connection is slow", func() {
			BeforeEach(func() {
				repoName = "some_user/some_repo"
				tag = "some-tag"

				setupSlowRegistry(tag)
			})

			It("should retry 3 times", func() {
				stderr := gbytes.NewBuffer()
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, stderr)
				Expect(err).NotTo(HaveOccurred())

				Expect(stderr).To(gbytes.Say("retry attempt: 1"))
				Expect(stderr).To(gbytes.Say("retry attempt: 2"))
				Expect(stderr).To(gbytes.Say("retry attempt: 3"))
			})
		})

		Context("with a valid repository:tag reference", func() {
			BeforeEach(func() {
				repoName = "some_user/some_repo"
				tag = "some-tag"

				setupRegistry(tag)
			})

			It("should not error", func() {
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the top-most image layer metadata", func() {
				img, _ := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(img).NotTo(BeNil())
				Expect(img.Config).NotTo(BeNil())
				Expect(img.Config.Cmd).To(Equal([]string{"dockerapp"}))
			})
		})

		Context("when the image exposes custom ports", func() {
			BeforeEach(func() {
				setupRegistry()

				repoName = "some_user/some_repo"
			})

			It("should not error", func() {
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the exposed ports", func() {
				img, _ := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(img.Config).NotTo(BeNil())

				Expect(img.Config.ExposedPorts).To(HaveKeyWithValue(nat.NewPort("tcp", "8080"), struct{}{}))
			})
		})

		Context("with an insecure registry", func() {
			BeforeEach(func() {
				setupRegistry()
				server.HTTPTestServer.TLS = &tls.Config{InsecureSkipVerify: false}
				insecureRegistries = append(insecureRegistries, server.Addr())

				repoName = "some_user/some_repo"
			})

			It("should not error", func() {
				_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the top-most image layer metadata", func() {
				img, _ := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
				Expect(img).NotTo(BeNil())
				Expect(img.Config).NotTo(BeNil())
				Expect(img.Config.Cmd).NotTo(BeNil())
				Expect(img.Config.Cmd).To(Equal([]string{"dockerapp"}))
			})

			Context("that is not in the insecureRegistries list", func() {
				BeforeEach(func() {
					insecureRegistries = []string{}
				})

				It("should error", func() {
					_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("https"))
				})
			})
		})

		Context("with a private registry with token authorization", func() {
			BeforeEach(func() {
				server = ghttp.NewUnstartedServer()
				registryURL = server.Addr()
				username = "username"
				password = "password"

				authenticateHeader := http.Header{}
				authenticateHeader.Add("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token"`, registryURL))
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v2/"),
						ghttp.RespondWith(401, "", authenticateHeader),
					),
				)
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/token"),
						ghttp.VerifyBasicAuth(username, password),
						ghttp.RespondWith(200, `{"token":"tokenstring"}`),
					),
				)
			})

			Context("with a valid repository:tag reference", func() {
				BeforeEach(func() {
					repoName = "some_user/some_repo"
					tag = "some-tag"
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyHeaderKV("Authorization", "Bearer tokenstring"),
							ghttp.VerifyRequest("GET", "/v2/some_user/some_repo/manifests/"+tag),
							http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
								w.Header().Set("X-Docker-Token", "token-1,token-2")
								w.Write([]byte(response))
							}),
						),
					)
				})

				It("should not error", func() {
					_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return the top-most image layer metadata", func() {
					img, _ := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
					Expect(img).NotTo(BeNil())
					Expect(img.Config).NotTo(BeNil())
					Expect(img.Config.Cmd).To(Equal([]string{"dockerapp"}))
				})
			})

			Context("with a valid repository:tag reference", func() {
				BeforeEach(func() {
					repoName = "some_user/some_repo"
					tag = "some-tag"

					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyHeaderKV("Authorization", "Bearer tokenstring"),
							ghttp.VerifyRequest("GET", "/v2/some_user/some_repo/manifests/"+tag),
							http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
								w.Header().Set("X-Docker-Token", "token-1,token-2")
								w.Write([]byte(response))
							}),
						),
					)
				})

				It("should not error", func() {
					_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return the top-most image layer metadata", func() {
					img, _ := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
					Expect(img).NotTo(BeNil())
					Expect(img.Config).NotTo(BeNil())
					Expect(img.Config.Cmd).To(Equal([]string{"dockerapp"}))
				})
			})
		})

		Context("with a private registry with basic authorization", func() {
			BeforeEach(func() {
				server = ghttp.NewUnstartedServer()
				registryURL = server.Addr()

				authenticateHeader := http.Header{}
				authenticateHeader.Add("WWW-Authenticate", fmt.Sprintf(`Basic realm="http://%s/token"`, registryURL))
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v2/"),
						ghttp.RespondWith(401, "", authenticateHeader),
					),
				)
			})

			Context("with a valid repository:tag reference", func() {
				BeforeEach(func() {
					repoName = "some_user/some_repo"
					tag = "some-tag"

					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyBasicAuth(username, password),
							ghttp.VerifyRequest("GET", "/v2/some_user/some_repo/manifests/"+tag),
							http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
								w.Header().Set("X-Docker-Token", "token-1,token-2")
								w.Write([]byte(response))
							}),
						),
					)
				})

				It("should not error", func() {
					_, err := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return the top-most image layer metadata", func() {
					img, _ := helpers.FetchMetadata(registryURL, repoName, tag, ctx, os.Stderr)
					Expect(img).NotTo(BeNil())
					Expect(img.Config).NotTo(BeNil())
					Expect(img.Config.Cmd).To(Equal([]string{"dockerapp"}))
				})
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

					var stagingResult dockerapplifecycle.StagingResult
					err = json.Unmarshal(result, &stagingResult)
					Expect(err).NotTo(HaveOccurred())

					Expect(stagingResult.ExecutionMetadata).NotTo(BeEmpty())
					Expect(stagingResult.ProcessTypes).NotTo(BeEmpty())

					var executionMetadata protocol.ExecutionMetadata
					err = json.Unmarshal([]byte(stagingResult.ExecutionMetadata), &executionMetadata)
					Expect(err).NotTo(HaveOccurred())

					Expect(executionMetadata.Cmd).To(Equal(metadata.ExecutionMetadata.Cmd))
					Expect(executionMetadata.Entrypoint).To(Equal(expectedEntryPoint))
					Expect(executionMetadata.Workdir).To(Equal(metadata.ExecutionMetadata.Workdir))

					Expect(stagingResult.ProcessTypes).To(HaveLen(1))
					Expect(stagingResult.ProcessTypes).To(HaveKeyWithValue("web", expectedStartCmd))

					Expect(stagingResult.LifecycleMetadata.DockerImage).To(Equal(metadata.DockerImage))
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
