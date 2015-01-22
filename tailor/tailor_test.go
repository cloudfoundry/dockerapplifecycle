package main_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"

	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega/gbytes"
	"github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega/gexec"
	"github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega/ghttp"
)

var _ = Describe("Tailoring", func() {
	var (
		tailorCmd                  *exec.Cmd
		dockerRef                  string
		dockerImageURL             string
		outputMetadataDir          string
		outputMetadataJSONFilename string
		server                     *ghttp.Server
		endpoint1                  *ghttp.Server
		endpoint2                  *ghttp.Server
	)

	tailor := func() *gexec.Session {
		session, err := gexec.Start(
			tailorCmd,
			GinkgoWriter,
			GinkgoWriter,
		)
		Ω(err).ShouldNot(HaveOccurred())

		return session
	}

	setupRegistry := func() {
		server.AppendHandlers(
			ghttp.VerifyRequest("GET", "/v1/_ping"),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/v1/repositories/some-repo/images"),
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

	BeforeEach(func() {
		var err error

		dockerRef = ""
		dockerImageURL = ""

		outputMetadataDir, err = ioutil.TempDir("", "tailoring-result")
		Ω(err).ShouldNot(HaveOccurred())

		outputMetadataJSONFilename = path.Join(outputMetadataDir, "result.json")

		server = ghttp.NewServer()
		endpoint1 = ghttp.NewServer()
		endpoint2 = ghttp.NewServer()
	})

	AfterEach(func() {
		os.RemoveAll(outputMetadataDir)
	})

	JustBeforeEach(func() {
		tailorCmd = exec.Command(tailorPath,
			"-dockerImageUrl", dockerImageURL,
			"-dockerRef", dockerRef,
			"-outputMetadataJSONFilename", outputMetadataJSONFilename,
		)

		tailorCmd.Env = os.Environ()
	})

	resultJSON := func() []byte {
		resultInfo, err := ioutil.ReadFile(outputMetadataJSONFilename)
		Ω(err).ShouldNot(HaveOccurred())

		return resultInfo
	}

	Context("when running the main", func() {
		Context("with no docker image arg specified", func() {
			It("should exit with an error", func() {
				session := tailor()
				Eventually(session.Err).Should(gbytes.Say("missing flag: dockerImageUrl or dockerRef required"))
				Eventually(session).Should(gexec.Exit(1))
			})
		})

		Context("with an invalid output path", func() {
			It("should exit with an error", func() {
				session := tailor()
				Eventually(session).Should(gexec.Exit(1))
			})
		})

		testValid := func() {
			BeforeEach(func() {
				setupRegistry()
				endpoint1.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/id-1/json"),
						http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Add("X-Docker-Size", "789")
							w.Write([]byte(`{"id":"layer-1","parent":"parent-1","Config":{"Cmd":["-bazbot", "-foobar"],"Entrypoint":["/dockerapp", "-t"]}}`))
						}),
					),
				)
			})

			It("should exit successfully", func() {
				session := tailor()
				Eventually(session).Should(gexec.Exit(0))
			})

			Describe("the json", func() {
				It("should contain the execution metadata", func() {
					session := tailor()
					Eventually(session).Should(gexec.Exit(0))

					result := resultJSON()

					Ω(result).Should(ContainSubstring(`\"cmd\":[\"-bazbot\",\"-foobar\"]`))
					Ω(result).Should(ContainSubstring(`\"entrypoint\":[\"/dockerapp\",\"-t\"]`))
				})
			})
		}

		Context("with a valid docker url", func() {
			BeforeEach(func() {
				parts, _ := url.Parse(server.URL())
				dockerImageURL = fmt.Sprintf("docker://%s/some-repo", parts.Host)
			})

			testValid()
		})

		Context("with a valid docker ref", func() {
			BeforeEach(func() {
				parts, _ := url.Parse(server.URL())
				dockerRef = fmt.Sprintf("%s/some-repo", parts.Host)
			})

			testValid()
		})
	})
})
