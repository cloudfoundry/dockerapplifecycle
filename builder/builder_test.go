package main_test

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("Building", func() {
	var (
		builderCmd                 *exec.Cmd
		dockerRef                  string
		dockerImageURL             string
		insecureDockerRegistries   string
		dockerDaemonExecutablePath string
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
		Ω(err).ShouldNot(HaveOccurred())

		return session
	}

	BeforeEach(func() {
		var err error

		dockerRef = ""
		dockerImageURL = ""
		insecureDockerRegistries = ""
		dockerDaemonExecutablePath = "./docker"

		outputMetadataDir, err = ioutil.TempDir("", "building-result")
		Ω(err).ShouldNot(HaveOccurred())

		outputMetadataJSONFilename = path.Join(outputMetadataDir, "result.json")

		fakeDockerRegistry = ghttp.NewServer()
	})

	AfterEach(func() {
		os.RemoveAll(outputMetadataDir)
	})

	JustBeforeEach(func() {
		args := []string{"-dockerImageURL", dockerImageURL,
			"-dockerRef", dockerRef,
			"-dockerDaemonExecutablePath", dockerDaemonExecutablePath,
			"-outputMetadataJSONFilename", outputMetadataJSONFilename}

		if len(insecureDockerRegistries) > 0 {
			args = append(args, "-insecureDockerRegistries", insecureDockerRegistries)
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
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say(fmt.Sprintf("invalid value \"%s\" for flag -insecureDockerRegistries: no scheme allowed for insecure Docker Registry \\[%s\\]", insecureDockerRegistries, invalidRegistryAddress)))
					Eventually(session).Should(gexec.Exit(2))
				})
			})

			Context("when an address has no port", func() {
				BeforeEach(func() {
					invalidRegistryAddress = "10.244.2.6"
					insecureDockerRegistries = invalidRegistryAddress + " , 10.244.2.7:8080"
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say(fmt.Sprintf("invalid value \"%s\" for flag -insecureDockerRegistries: ip:port expected for insecure Docker Registry \\[%s\\]", insecureDockerRegistries, invalidRegistryAddress)))
					Eventually(session).Should(gexec.Exit(2))
				})
			})

			Context("when docker daemon dir is invalid", func() {
				BeforeEach(func() {
					parts, err := url.Parse(fakeDockerRegistry.URL())
					Ω(err).ShouldNot(HaveOccurred())
					dockerImageURL = fmt.Sprintf("docker://%s/some-repo", parts.Host)

					dockerDaemonExecutablePath = "missing_dir/docker"
				})

				It("should exit with an error", func() {
					session := setupBuilder()
					Eventually(session.Err).Should(gbytes.Say(fmt.Sprintf("docker daemon not found in %s", dockerDaemonExecutablePath)))
					Eventually(session).Should(gexec.Exit(1))
				})
			})

		})

	})
})
