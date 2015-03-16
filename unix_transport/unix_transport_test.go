package unix_transport

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/nu7hatch/gouuid"

	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry-incubator/docker_app_lifecycle/Godeps/_workspace/src/github.com/onsi/gomega/ghttp"
)

var _ = Describe("Unix transport", func() {

	var (
		socket string
		client http.Client
	)

	Context("with server listening", func() {

		var (
			unixSocketListener net.Listener
			unixSocketServer   *ghttp.Server
			resp               *http.Response
			err                error
		)

		BeforeEach(func() {
			uuid, err := uuid.NewV4()
			Ω(err).ShouldNot(HaveOccurred())

			socket = fmt.Sprintf("/tmp/%s.sock", uuid)
			unixSocketListener, err = net.Listen("unix", socket)
			Ω(err).ShouldNot(HaveOccurred())

			unixSocketServer = ghttp.NewUnstartedServer()

			unixSocketServer.HTTPTestServer = &httptest.Server{
				Listener: unixSocketListener,
				Config:   &http.Server{Handler: unixSocketServer},
			}
			unixSocketServer.Start()

			client = http.Client{Transport: New(socket)}
		})

		Context("when a simple GET request is sent", func() {
			BeforeEach(func() {
				unixSocketServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/_ping"),
						ghttp.RespondWith(http.StatusOK, "true"),
					),
				)

				resp, err = client.Get("unix://" + socket + "/_ping")
			})

			It("responds with correct status", func() {
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resp.StatusCode).Should(Equal(http.StatusOK))

			})

			It("responds with correct body", func() {
				bytes, err := ioutil.ReadAll(resp.Body)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(string(bytes)).Should(Equal("true"))
			})
		})

		Context("when a POST request is sent", func() {
			const (
				ReqBody  = `"id":"some-id"`
				RespBody = `{"Image" : "ubuntu"}`
			)

			assertBodyEquals := func(body io.ReadCloser, expectedContent string) {
				bytes, err := ioutil.ReadAll(body)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(string(bytes)).Should(Equal(expectedContent))

			}

			asserHeaderContains := func(header http.Header, key, value string) {
				Ω(header[key]).Should(ConsistOf(value))
			}

			BeforeEach(func() {
				validateBody := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					assertBodyEquals(req.Body, ReqBody)
				})

				validateQueryParams := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					Ω(req.URL.RawQuery).Should(Equal("fromImage=ubunut&tag=latest"))
				})

				handleRequest := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(RespBody))
				})

				unixSocketServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/containers/create"),
						ghttp.VerifyContentType("application/json"),
						validateBody,
						validateQueryParams,
						handleRequest,
					),
				)
				body := strings.NewReader(ReqBody)
				req, err := http.NewRequest("POST", "unix://"+socket+"/containers/create?fromImage=ubunut&tag=latest", body)
				req.Header.Add("Content-Type", "application/json")
				Ω(err).ShouldNot(HaveOccurred())

				resp, err = client.Do(req)
				Ω(err).ShouldNot(HaveOccurred())

			})

			It("responds with correct status", func() {
				Ω(resp.StatusCode).Should(Equal(http.StatusOK))
			})

			It("responds with correct headers", func() {
				asserHeaderContains(resp.Header, "Content-Type", "application/json")
			})

			It("responds with correct body", func() {
				assertBodyEquals(resp.Body, RespBody)
			})

		})

		Context("when socket in reques URI is incorrect", func() {
			It("errors", func() {
				resp, err = client.Get("unix:///fake/socket.sock/_ping")
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(ContainSubstring("Wrong unix socket"))
			})
		})

		AfterEach(func() {
			unixSocketServer.Close()
		})
	})

	Context("with no server listening", func() {
		BeforeEach(func() {
			socket = "/not/existing.sock"
			client = http.Client{Transport: New(socket)}
		})

		It("errors", func() {
			_, err := client.Get("unix:///not/existing.sock/_ping")
			Ω(err).Should(HaveOccurred())
			Ω(err.Error()).Should(ContainSubstring(fmt.Sprintf("dial unix %s: no such file or directory", socket)))
		})
	})
})
