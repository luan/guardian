package gqt_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/gqt/runner"
	"code.cloudfoundry.org/guardian/imageplugin"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Image Plugin", func() {

	var (
		args   []string
		client *runner.RunningGarden
	)

	BeforeEach(func() {
		args = []string{}
	})

	JustBeforeEach(func() {
		client = startGarden(args...)
	})

	AfterEach(func() {
		Expect(client.DestroyAndStop()).To(Succeed())
	})

	Context("when only an unprivileged image plugin is provided", func() {
		var (
			tmpDir string
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			Expect(os.Chmod(tmpDir, 0777)).To(Succeed())

			args = append(args,
				"--image-plugin", testImagePluginBin,
				"--image-plugin-extra-arg", "\"--rootfs-path\"",
				"--image-plugin-extra-arg", tmpDir,
				"--image-plugin-extra-arg", "\"--args-path\"",
				"--image-plugin-extra-arg", filepath.Join(tmpDir, "args"))
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		Context("and an unprivileged container is successfully created", func() {
			var (
				containerSpec garden.ContainerSpec
				container     garden.Container
				handle        string

				destroyContainer bool
			)

			BeforeEach(func() {
				containerSpec = garden.ContainerSpec{}
				destroyContainer = true

				args = append(args,
					"--image-plugin-extra-arg", "\"--create-whoami-path\"",
					"--image-plugin-extra-arg", filepath.Join(tmpDir, "create-whoami"))
			})

			JustBeforeEach(func() {
				var err error
				container, err = client.Create(containerSpec)
				Expect(err).NotTo(HaveOccurred())
				handle = container.Handle()
			})

			AfterEach(func() {
				if destroyContainer {
					Expect(client.Destroy(container.Handle())).To(Succeed())
				}
			})

			It("executes the plugin, passing the correct args", func() {
				pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
				Expect(err).ToNot(HaveOccurred())

				pluginArgs := strings.Split(string(pluginArgsBytes), " ")
				Expect(pluginArgs).To(Equal([]string{
					testImagePluginBin,
					"--rootfs-path", tmpDir,
					"--args-path", filepath.Join(tmpDir, "args"),
					"--create-whoami-path", filepath.Join(tmpDir, "create-whoami"),
					"create",
					os.Getenv("GARDEN_TEST_ROOTFS"),
					handle,
				}))
			})

			It("executes the plugin as root", func() {
				whoamiBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "create-whoami"))
				Expect(err).NotTo(HaveOccurred())

				Expect(string(whoamiBytes)).To(ContainSubstring("0 - 0"))
			})

			Context("when there are env vars", func() {
				BeforeEach(func() {
					image := imageplugin.Image{
						Config: imageplugin.ImageConfig{
							Env: []string{
								"MY_VAR=set",
								"MY_SECOND_VAR=also_set",
							},
						},
					}
					imageJson, err := json.Marshal(image)
					Expect(err).NotTo(HaveOccurred())

					args = append(args,
						"--image-plugin-extra-arg", "\"--image-json\"",
						"--image-plugin-extra-arg", string(imageJson),
					)

					gardenDefaultRootfs := os.Getenv("GARDEN_TEST_ROOTFS")
					Expect(copyFile(filepath.Join(gardenDefaultRootfs, "bin", "env"),
						filepath.Join(tmpDir, "env"))).To(Succeed())
				})

				It("loads the image.json env variables", func() {
					buffer := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						Path: "/env",
						Dir:  "/",
					}, garden.ProcessIO{Stdout: buffer, Stderr: buffer})
					Expect(err).NotTo(HaveOccurred())
					exitCode, err := process.Wait()
					Expect(err).NotTo(HaveOccurred())
					Expect(exitCode).To(BeZero())

					Eventually(buffer).Should(gbytes.Say("MY_VAR=set"))
					Eventually(buffer).Should(gbytes.Say("MY_SECOND_VAR=also_set"))
				})
			})

			Context("when there are mounts", func() {
				var currTempDirName string
				currFileName := "/sample-string-file"
				currFileContent := "sample-string-file-content"

				BeforeEach(func() {
					var err error
					currTempDirName, err = ioutil.TempDir("", "bind-mount")
					Expect(err).NotTo(HaveOccurred())
					Expect(ioutil.WriteFile(currTempDirName+currFileName, []byte(currFileContent), 0644)).To(Succeed())

					mounts := []imageplugin.Mount{
						{
							Type:    "bind",
							Options: []string{"bind"},
							Source:  currTempDirName,
							Dest:    "/bind-mount",
						},
					}
					mountsJson, err := json.Marshal(mounts)
					Expect(err).NotTo(HaveOccurred())

					args = append(args,
						"--image-plugin-extra-arg", "\"--mounts-json\"",
						"--image-plugin-extra-arg", string(mountsJson),
					)

					gardenDefaultRootfs := os.Getenv("GARDEN_TEST_ROOTFS")
					Expect(copyFile(filepath.Join(gardenDefaultRootfs, "bin", "cat"),
						filepath.Join(tmpDir, "cat"))).To(Succeed())
				})

				AfterEach(func() {
					err := os.RemoveAll(currTempDirName)
					Expect(err).NotTo(HaveOccurred())
				})

				FIt("mounts the directories from the image plugin response", func() {
					var stdout bytes.Buffer
					process, err := container.Run(garden.ProcessSpec{
						Path: "/cat",
						Args: []string{"/bind-mount" + currFileName},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(&stdout, GinkgoWriter),
						Stderr: GinkgoWriter,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(process.Wait()).To(Equal(0))
					Expect(stdout.String()).To(Equal(currFileContent))
				})
			})

			Context("when rootfs is not specified", func() {
				BeforeEach(func() {
					containerSpec.RootFSPath = ""
				})

				It("uses the default rootfs", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring(os.Getenv("GARDEN_TEST_ROOTFS")))
				})
			})

			Context("when passing a tagged docker image as the RootFSPath", func() {
				BeforeEach(func() {
					containerSpec.RootFSPath = "docker:///busybox#1.26.1"
				})

				It("replaces the # with :", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring("docker:///busybox:1.26.1"))
				})
			})

			Context("when passing an Image URI", func() {
				BeforeEach(func() {
					containerSpec.Image = garden.ImageRef{URI: "/some/fake/rootfs"}
				})

				It("executes the plugin, passing the Image URI", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring("/some/fake/rootfs"))
				})
			})

			Context("when specifying a quota", func() {
				BeforeEach(func() {
					containerSpec.Limits.Disk.ByteHard = 100000
				})

				It("calls the image plugin setting the quota", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring("--disk-limit-size-bytes 100000"))
				})

				Context("when the quota is total", func() {
					BeforeEach(func() {
						containerSpec.Limits.Disk.Scope = garden.DiskLimitScopeTotal
					})

					It("calls the image plugin without the exclusive flag", func() {
						pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
						Expect(err).ToNot(HaveOccurred())

						Expect(string(pluginArgsBytes)).NotTo(ContainSubstring("--exclude-image-from-quota"))
					})
				})

				Context("when the quota is exclusive", func() {
					BeforeEach(func() {
						containerSpec.Limits.Disk.Scope = garden.DiskLimitScopeExclusive
					})

					It("calls the image plugin setting the exclusive flag", func() {
						pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
						Expect(err).ToNot(HaveOccurred())

						Expect(string(pluginArgsBytes)).To(ContainSubstring("--exclude-image-from-quota"))
					})
				})

				Context("when the plugin logs to stderr", func() {
					BeforeEach(func() {
						args = append(args,
							"--image-plugin-extra-arg", "\"--create-log-content\"",
							"--image-plugin-extra-arg", "CREATE-FAKE-LOG-LINE")
					})

					It("relogs the plugin's stderr to the garden logs", func() {
						Eventually(client).Should(gbytes.Say("CREATE-FAKE-LOG-LINE"))
					})
				})
			})

			Context("and metrics are collected on that container", func() {
				var metrics garden.Metrics
				BeforeEach(func() {
					args = append(args,
						"--image-plugin-extra-arg", "\"--metrics-whoami-path\"",
						"--image-plugin-extra-arg", filepath.Join(tmpDir, "metrics-whoami"),
						"--image-plugin-extra-arg", "\"--metrics-output\"",
						"--image-plugin-extra-arg", `{"disk_usage": {"total_bytes_used": 1000, "exclusive_bytes_used": 2000}}`)
				})

				JustBeforeEach(func() {
					var err error
					metrics, err = container.Metrics()
					Expect(err).NotTo(HaveOccurred())
				})

				It("executes the plugin, passing the correct args", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					pluginArgs := strings.Split(string(pluginArgsBytes), " ")
					Expect(pluginArgs).To(Equal([]string{
						testImagePluginBin,
						"--rootfs-path", tmpDir,
						"--args-path", filepath.Join(tmpDir, "args"),
						"--create-whoami-path", filepath.Join(tmpDir, "create-whoami"),
						"--metrics-whoami-path", filepath.Join(tmpDir, "metrics-whoami"),
						"--metrics-output",
						"{\"disk_usage\":",
						"{\"total_bytes_used\":",
						"1000,",
						"\"exclusive_bytes_used\":",
						"2000}}",
						"stats",
						handle,
					}))
				})

				It("executes the plugin as root", func() {
					whoamiBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "metrics-whoami"))
					Expect(err).NotTo(HaveOccurred())

					Expect(string(whoamiBytes)).To(ContainSubstring("0 - 0"))
				})

				It("returns the plugin stdout as disk stats", func() {
					Expect(metrics.DiskStat.TotalBytesUsed).To(BeEquivalentTo(1000))
					Expect(metrics.DiskStat.ExclusiveBytesUsed).To(BeEquivalentTo(2000))
				})

				Context("when the plugin logs to stderr", func() {
					BeforeEach(func() {
						args = append(args,
							"--image-plugin-extra-arg", "\"--metrics-log-content\"",
							"--image-plugin-extra-arg", "METRICS-FAKE-LOG-LINE")
					})

					It("relogs the plugin's stderr to the garden logs", func() {
						Eventually(client).Should(gbytes.Say("METRICS-FAKE-LOG-LINE"))
					})
				})
			})

			Context("but the plugin returns nonsense stats", func() {
				BeforeEach(func() {
					args = append(args,
						"--image-plugin-extra-arg", "\"--metrics-output\"",
						"--image-plugin-extra-arg", "NONSENSE_JSON")
				})

				It("returns a sensible error containing the json", func() {
					_, err := container.Metrics()
					Expect(err).To(MatchError(ContainSubstring("parsing stats: NONSENSE_JSON")))
				})
			})

			Context("but the plugin fails when collecting metrics", func() {
				BeforeEach(func() {
					args = append(args,
						"--image-plugin-extra-arg", "\"--fail-on\"",
						"--image-plugin-extra-arg", "metrics")
				})

				It("returns the plugin's stdout in a useful error", func() {
					_, err := container.Metrics()
					Expect(err).To(MatchError(ContainSubstring("running image plugin metrics: metrics failed")))
				})
			})

			Context("and that container is destroyed", func() {
				BeforeEach(func() {
					destroyContainer = false
					args = append(args,
						"--image-plugin-extra-arg", "\"--destroy-whoami-path\"",
						"--image-plugin-extra-arg", filepath.Join(tmpDir, "destroy-whoami"))
				})

				JustBeforeEach(func() {
					Expect(client.Destroy(container.Handle())).Should(Succeed())
				})

				It("executes the plugin, passing the correct args", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					pluginArgs := strings.Split(string(pluginArgsBytes), " ")
					Expect(pluginArgs).To(Equal([]string{
						testImagePluginBin,
						"--rootfs-path", tmpDir,
						"--args-path", filepath.Join(tmpDir, "args"),
						"--create-whoami-path", filepath.Join(tmpDir, "create-whoami"),
						"--destroy-whoami-path", filepath.Join(tmpDir, "destroy-whoami"),
						"delete",
						handle,
					}))
				})

				It("executes the plugin as root", func() {
					whoamiBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "destroy-whoami"))
					Expect(err).NotTo(HaveOccurred())

					Expect(string(whoamiBytes)).To(ContainSubstring("0 - 0"))
				})

				Context("when the plugin logs to stderr", func() {
					BeforeEach(func() {
						args = append(args,
							"--image-plugin-extra-arg", "\"--destroy-log-content\"",
							"--image-plugin-extra-arg", "DESTROY-FAKE-LOG-LINE")
					})

					It("relogs the plugin's stderr to the garden logs", func() {
						Eventually(client).Should(gbytes.Say("DESTROY-FAKE-LOG-LINE"))
					})
				})
			})

			Context("but the plugin fails on destruction", func() {
				BeforeEach(func() {
					destroyContainer = false
					args = append(args,
						"--image-plugin-extra-arg", "\"--fail-on\"",
						"--image-plugin-extra-arg", "destroy")
				})

				It("returns the plugin's stdout in a useful error", func() {
					err := client.Destroy(container.Handle())
					Expect(err).To(MatchError(ContainSubstring("running image plugin destroy: destroy failed")))
				})
			})
		})

		Context("but the plugin fails on creation", func() {
			BeforeEach(func() {
				args = append(args,
					"--image-plugin-extra-arg", "\"--fail-on\"",
					"--image-plugin-extra-arg", "create")
			})

			It("returns the plugin's stdout in a useful error", func() {
				_, err := client.Create(garden.ContainerSpec{})
				Expect(err).To(MatchError(ContainSubstring("running image plugin create: create failed")))
			})
		})

		Context("but we attempt to create a privileged container", func() {
			It("returns an informative error", func() {
				_, err := client.Create(garden.ContainerSpec{Privileged: true})
				Expect(err).To(MatchError(ContainSubstring("no privileged_image_plugin provided")))
			})
		})
	})

	Context("when only a privileged image plugin is provided", func() {
		var (
			tmpDir string
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			Expect(os.Chmod(tmpDir, 0777)).To(Succeed())

			args = append(args,
				"--privileged-image-plugin", testImagePluginBin,
				"--privileged-image-plugin-extra-arg", "\"--rootfs-path\"",
				"--privileged-image-plugin-extra-arg", tmpDir,
				"--privileged-image-plugin-extra-arg", "\"--args-path\"",
				"--privileged-image-plugin-extra-arg", filepath.Join(tmpDir, "args"))
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		Context("and a container is created", func() {
			var (
				containerSpec garden.ContainerSpec
				container     garden.Container
				handle        string

				destroyContainer bool
			)

			BeforeEach(func() {
				containerSpec = garden.ContainerSpec{Privileged: true}
				destroyContainer = false

				args = append(args,
					"--privileged-image-plugin-extra-arg", "\"--create-whoami-path\"",
					"--privileged-image-plugin-extra-arg", filepath.Join(tmpDir, "create-whoami"))
			})

			JustBeforeEach(func() {
				var err error
				container, err = client.Create(containerSpec)
				Expect(err).NotTo(HaveOccurred())
				handle = container.Handle()
			})

			AfterEach(func() {
				if destroyContainer {
					Expect(client.Destroy(container.Handle())).To(Succeed())
				}
			})

			It("executes the plugin, passing the correct args", func() {
				pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
				Expect(err).ToNot(HaveOccurred())

				pluginArgs := strings.Split(string(pluginArgsBytes), " ")
				Expect(pluginArgs).To(Equal([]string{
					testImagePluginBin,
					"--rootfs-path", tmpDir,
					"--args-path", filepath.Join(tmpDir, "args"),
					"--create-whoami-path", filepath.Join(tmpDir, "create-whoami"),
					"create",
					os.Getenv("GARDEN_TEST_ROOTFS"),
					handle,
				}))
			})

			It("executes the plugin as root", func() {
				whoamiBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "create-whoami"))
				Expect(err).NotTo(HaveOccurred())

				Expect(string(whoamiBytes)).To(ContainSubstring("0 - 0"))
			})

			Context("when there are env vars", func() {
				BeforeEach(func() {
					image := imageplugin.Image{
						Config: imageplugin.ImageConfig{
							Env: []string{
								"MY_VAR=set",
								"MY_SECOND_VAR=also_set",
							},
						},
					}
					imageJson, err := json.Marshal(image)
					Expect(err).NotTo(HaveOccurred())

					args = append(args,
						"--privileged-image-plugin-extra-arg", "\"--image-json\"",
						"--privileged-image-plugin-extra-arg", string(imageJson),
					)

					gardenDefaultRootfs := os.Getenv("GARDEN_TEST_ROOTFS")
					Expect(copyFile(filepath.Join(gardenDefaultRootfs, "bin", "env"),
						filepath.Join(tmpDir, "env"))).To(Succeed())
				})

				It("loads the image.json env variables", func() {
					buffer := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						Path: "/env",
						Dir:  "/",
					}, garden.ProcessIO{Stdout: buffer, Stderr: buffer})
					Expect(err).NotTo(HaveOccurred())
					exitCode, err := process.Wait()
					Expect(err).NotTo(HaveOccurred())
					Expect(exitCode).To(BeZero())

					Eventually(buffer).Should(gbytes.Say("MY_VAR=set"))
					Eventually(buffer).Should(gbytes.Say("MY_SECOND_VAR=also_set"))
				})
			})

			Context("when rootfs is not specified", func() {
				BeforeEach(func() {
					containerSpec.RootFSPath = ""
				})

				It("uses the default rootfs", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring(os.Getenv("GARDEN_TEST_ROOTFS")))
				})
			})

			Context("when passing a tagged docker image as the RootFSPath", func() {
				BeforeEach(func() {
					containerSpec.RootFSPath = "docker:///busybox#1.26.1"
				})

				It("replaces the # with :", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring("docker:///busybox:1.26.1"))
				})
			})

			Context("when passing an Image URI", func() {
				BeforeEach(func() {
					containerSpec.Image = garden.ImageRef{URI: "/some/fake/rootfs"}
				})

				It("executes the plugin, passing the Image URI", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring("/some/fake/rootfs"))
				})
			})

			Context("when specifying a quota", func() {
				BeforeEach(func() {
					containerSpec.Limits.Disk.ByteHard = 100000
				})

				It("calls the image plugin setting the quota", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginArgsBytes)).To(ContainSubstring("--disk-limit-size-bytes 100000"))
				})

				Context("when the quota is total", func() {
					BeforeEach(func() {
						containerSpec.Limits.Disk.Scope = garden.DiskLimitScopeTotal
					})

					It("calls the image plugin without the exclusive flag", func() {
						pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
						Expect(err).ToNot(HaveOccurred())

						Expect(string(pluginArgsBytes)).NotTo(ContainSubstring("--exclude-image-from-quota"))
					})
				})

				Context("when the quota is exclusive", func() {
					BeforeEach(func() {
						containerSpec.Limits.Disk.Scope = garden.DiskLimitScopeExclusive
					})

					It("calls the image plugin setting the exclusive flag", func() {
						pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
						Expect(err).ToNot(HaveOccurred())

						Expect(string(pluginArgsBytes)).To(ContainSubstring("--exclude-image-from-quota"))
					})
				})
			})

			Context("and metrics are collected on that container", func() {
				BeforeEach(func() {
					args = append(args,
						"--privileged-image-plugin-extra-arg", "\"--metrics-whoami-path\"",
						"--privileged-image-plugin-extra-arg", filepath.Join(tmpDir, "metrics-whoami"),
						"--privileged-image-plugin-extra-arg", "\"--metrics-output\"",
						"--privileged-image-plugin-extra-arg", `{"disk_usage": {"total_bytes_used": 1000, "exclusive_bytes_used": 2000}}`)
				})

				JustBeforeEach(func() {
					_, err := container.Metrics()
					Expect(err).NotTo(HaveOccurred())
				})

				It("executes the plugin, passing the correct args", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					pluginArgs := strings.Split(string(pluginArgsBytes), " ")
					Expect(pluginArgs).To(Equal([]string{
						testImagePluginBin,
						"--rootfs-path", tmpDir,
						"--args-path", filepath.Join(tmpDir, "args"),
						"--create-whoami-path", filepath.Join(tmpDir, "create-whoami"),
						"--metrics-whoami-path", filepath.Join(tmpDir, "metrics-whoami"),
						"--metrics-output",
						"{\"disk_usage\":",
						"{\"total_bytes_used\":",
						"1000,",
						"\"exclusive_bytes_used\":",
						"2000}}",
						"stats",
						handle,
					}))
				})

				It("executes the plugin as root", func() {
					whoamiBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "metrics-whoami"))
					Expect(err).NotTo(HaveOccurred())

					Expect(string(whoamiBytes)).To(ContainSubstring("0 - 0"))
				})

				Context("when the plugin logs to stderr", func() {
					BeforeEach(func() {
						args = append(args,
							"--privileged-image-plugin-extra-arg", "\"--metrics-log-content\"",
							"--privileged-image-plugin-extra-arg", "METRICS-FAKE-LOG-LINE")
					})

					It("relogs the plugin's stderr to the garden logs", func() {
						Eventually(client).Should(gbytes.Say("METRICS-FAKE-LOG-LINE"))
					})
				})
			})

			Context("but the plugin fails when collecting metrics", func() {
				BeforeEach(func() {
					args = append(args,
						"--privileged-image-plugin-extra-arg", "\"--fail-on\"",
						"--privileged-image-plugin-extra-arg", "metrics")
				})

				It("returns the plugin's stdout in a useful error", func() {
					_, err := container.Metrics()
					Expect(err).To(MatchError(ContainSubstring("running image plugin metrics: metrics failed")))
				})
			})

			Context("and that container is destroyed", func() {
				BeforeEach(func() {
					destroyContainer = false

					args = append(args,
						"--privileged-image-plugin-extra-arg", "\"--destroy-whoami-path\"",
						"--privileged-image-plugin-extra-arg", filepath.Join(tmpDir, "destroy-whoami"))
				})

				JustBeforeEach(func() {
					Expect(client.Destroy(container.Handle())).Should(Succeed())
				})

				It("executes the plugin, passing the correct args", func() {
					pluginArgsBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
					Expect(err).ToNot(HaveOccurred())

					pluginArgs := strings.Split(string(pluginArgsBytes), " ")
					Expect(pluginArgs).To(Equal([]string{
						testImagePluginBin,
						"--rootfs-path", tmpDir,
						"--args-path", filepath.Join(tmpDir, "args"),
						"--create-whoami-path", filepath.Join(tmpDir, "create-whoami"),
						"--destroy-whoami-path", filepath.Join(tmpDir, "destroy-whoami"),
						"delete",
						handle,
					}))
				})

				It("executes the plugin as root", func() {
					whoamiBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "destroy-whoami"))
					Expect(err).NotTo(HaveOccurred())

					Expect(string(whoamiBytes)).To(ContainSubstring("0 - 0"))
				})

				Context("when the plugin logs to stderr", func() {
					BeforeEach(func() {
						args = append(args,
							"--privileged-image-plugin-extra-arg", "\"--destroy-log-content\"",
							"--privileged-image-plugin-extra-arg", "DESTROY-FAKE-LOG-LINE")
					})

					It("relogs the plugin's stderr to the garden logs", func() {
						Eventually(client).Should(gbytes.Say("DESTROY-FAKE-LOG-LINE"))
					})
				})
			})

			Context("but the plugin fails on destruction", func() {
				BeforeEach(func() {
					destroyContainer = false
					args = append(args,
						"--privileged-image-plugin-extra-arg", "\"--fail-on\"",
						"--privileged-image-plugin-extra-arg", "destroy")
				})

				It("returns the plugin's stdout in a useful error", func() {
					err := client.Destroy(container.Handle())
					Expect(err).To(MatchError(ContainSubstring("running image plugin destroy: destroy failed")))
				})
			})
		})

		Context("but the plugin fails on creation", func() {
			BeforeEach(func() {
				args = append(args,
					"--privileged-image-plugin-extra-arg", "\"--fail-on\"",
					"--privileged-image-plugin-extra-arg", "create")
			})

			It("returns the plugin's stdout in a useful error", func() {
				_, err := client.Create(garden.ContainerSpec{Privileged: true})
				Expect(err).To(MatchError(ContainSubstring("running image plugin create: create failed")))
			})
		})

		Context("but we attempt to create an unprivileged container", func() {
			It("returns an informative error", func() {
				_, err := client.Create(garden.ContainerSpec{Privileged: false})
				Expect(err).To(MatchError(ContainSubstring("no image_plugin provided")))
			})
		})
	})

	Context("when both image_plugin and privileged_image_plugin are provided", func() {
		var (
			tmpDir string
		)

		BeforeEach(func() {
			// make a a copy of the fake image plugin so we can check location of file called
			Expect(copyFile(testImagePluginBin, fmt.Sprintf("%s-priv", testImagePluginBin))).To(Succeed())

			var err error
			tmpDir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			Expect(os.Chmod(tmpDir, 0777)).To(Succeed())

			args = append(args,
				"--image-plugin", testImagePluginBin,
				"--image-plugin-extra-arg", "\"--rootfs-path\"",
				"--image-plugin-extra-arg", tmpDir,
				"--image-plugin-extra-arg", "\"--create-bin-location-path\"",
				"--image-plugin-extra-arg", filepath.Join(tmpDir, "create-bin-location"),
				"--image-plugin-extra-arg", "\"--destroy-bin-location-path\"",
				"--image-plugin-extra-arg", filepath.Join(tmpDir, "destroy-bin-location"),
				"--image-plugin-extra-arg", "\"--metrics-bin-location-path\"",
				"--image-plugin-extra-arg", filepath.Join(tmpDir, "metrics-bin-location"),
				"--privileged-image-plugin", fmt.Sprintf("%s-priv", testImagePluginBin),
				"--privileged-image-plugin-extra-arg", "\"--rootfs-path\"",
				"--privileged-image-plugin-extra-arg", tmpDir,
				"--privileged-image-plugin-extra-arg", "\"--create-bin-location-path\"",
				"--privileged-image-plugin-extra-arg", filepath.Join(tmpDir, "create-bin-location"),
				"--privileged-image-plugin-extra-arg", "\"--destroy-bin-location-path\"",
				"--privileged-image-plugin-extra-arg", filepath.Join(tmpDir, "destroy-bin-location"),
				"--privileged-image-plugin-extra-arg", "\"--metrics-bin-location-path\"",
				"--privileged-image-plugin-extra-arg", filepath.Join(tmpDir, "metrics-bin-location"))
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		Context("when an unprivileged container is created", func() {
			var container garden.Container

			JustBeforeEach(func() {
				var err error
				container, err = client.Create(garden.ContainerSpec{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("calls only the unprivileged plugin", func() {
				pluginLocationBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "create-bin-location"))
				Expect(err).ToNot(HaveOccurred())

				Expect(string(pluginLocationBytes)).To(Equal(testImagePluginBin))
			})

			Context("and metrics are collected on that container", func() {
				JustBeforeEach(func() {
					_, err := container.Metrics()
					Expect(err).NotTo(HaveOccurred())
				})

				It("calls only the unprivileged plugin", func() {
					pluginLocationBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "metrics-bin-location"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginLocationBytes)).To(Equal(testImagePluginBin))
				})
			})

			Context("and that container is destroyed", func() {
				JustBeforeEach(func() {
					Expect(client.Destroy(container.Handle())).To(Succeed())
				})

				It("calls the unprivileged plugin", func() {
					pluginLocationBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "destroy-bin-location"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginLocationBytes)).To(ContainSubstring(testImagePluginBin))
				})
			})
		})

		Context("when a privileged container is created", func() {
			var container garden.Container

			JustBeforeEach(func() {
				var err error
				container, err = client.Create(garden.ContainerSpec{Privileged: true})
				Expect(err).NotTo(HaveOccurred())
			})

			It("calls only the privileged plugin", func() {
				pluginLocationBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "create-bin-location"))
				Expect(err).ToNot(HaveOccurred())

				Expect(string(pluginLocationBytes)).To(Equal(fmt.Sprintf("%s-priv", testImagePluginBin)))
			})

			Context("and metrics are collected on that container", func() {
				JustBeforeEach(func() {
					_, err := container.Metrics()
					Expect(err).NotTo(HaveOccurred())
				})

				It("calls only the privileged plugin", func() {
					pluginLocationBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "metrics-bin-location"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginLocationBytes)).To(Equal(fmt.Sprintf("%s-priv", testImagePluginBin)))
				})
			})

			Context("and that container is destroyed", func() {
				JustBeforeEach(func() {
					Expect(client.Destroy(container.Handle())).To(Succeed())
				})

				It("calls the privileged plugin", func() {
					pluginLocationBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "destroy-bin-location"))
					Expect(err).ToNot(HaveOccurred())

					Expect(string(pluginLocationBytes)).To(ContainSubstring(fmt.Sprintf("%s-priv", testImagePluginBin)))
				})
			})
		})
	})

	Context("when images are located in a private registry", func() {
		var (
			tmpDir    string
			imageSpec garden.ImageRef
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			Expect(os.Chmod(tmpDir, 0777)).To(Succeed())

			args = append(args,
				"--log-level", "debug",
				"--image-plugin", testImagePluginBin,
				"--image-plugin-extra-arg", "\"--rootfs-path\"",
				"--image-plugin-extra-arg", tmpDir,
				"--image-plugin-extra-arg", "\"--args-path\"",
				"--image-plugin-extra-arg", filepath.Join(tmpDir, "args"))

			imageSpec = garden.ImageRef{
				URI:      "",
				Username: "imagepluginuser",
				Password: "secretpassword",
			}
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		It("calls the image plugin with username and password", func() {
			_, err := client.Create(garden.ContainerSpec{
				Image: imageSpec,
			})
			Expect(err).NotTo(HaveOccurred())

			args, err := ioutil.ReadFile(filepath.Join(tmpDir, "args"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(args)).To(ContainSubstring("imagepluginuser"))
			Expect(string(args)).To(ContainSubstring("secretpassword"))
		})

		It("does not log username and password", func() {
			_, err := client.Create(garden.ContainerSpec{
				Image: imageSpec,
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(client, "1s").ShouldNot(gbytes.Say("imagepluginuser"))
			Eventually(client, "1s").ShouldNot(gbytes.Say("secretpassword"))
		})

	})
})

func copyFile(srcPath, dstPath string) error {
	dirPath := filepath.Dir(dstPath)
	if err := os.MkdirAll(dirPath, 0777); err != nil {
		return err
	}

	reader, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	writer, err := os.Create(dstPath)
	if err != nil {
		reader.Close()
		return err
	}

	if _, err := io.Copy(writer, reader); err != nil {
		writer.Close()
		reader.Close()
		return err
	}

	writer.Close()
	reader.Close()

	return os.Chmod(writer.Name(), 0777)
}
