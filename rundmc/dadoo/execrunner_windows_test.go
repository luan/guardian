package dadoo_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/dadoo"
	dadoofakes "code.cloudfoundry.org/guardian/rundmc/dadoo/dadoofakes"
	"code.cloudfoundry.org/guardian/rundmc/runrunc"
	fakes "code.cloudfoundry.org/guardian/rundmc/runrunc/runruncfakes"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	specs "github.com/opencontainers/runtime-spec/specs-go"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("WindowsExecRunner", func() {
	var (
		runner                 *dadoo.WindowsExecRunner
		log                    *lagertest.TestLogger
		fakeCommandRunner      *fake_command_runner.FakeCommandRunner
		fakeProcessIDGenerator *fakes.FakeUidGenerator
		fakePidGetter          *dadoofakes.FakePidGetter
		bundlePath             string
		processesPath          string
		processID              string
	)

	BeforeEach(func() {
		log = lagertest.NewTestLogger("WindowsExecRunner")
		fakeCommandRunner = fake_command_runner.New()
		fakeProcessIDGenerator = new(fakes.FakeUidGenerator)
		fakePidGetter = new(dadoofakes.FakePidGetter)
		processID = fmt.Sprintf("pid-%d", GinkgoParallelNode())

		var err error
		bundlePath, err = ioutil.TempDir("", "dadooexecrunnerbundle")
		Expect(err).NotTo(HaveOccurred())
		processesPath = filepath.Join(bundlePath, "processes")

		runner = dadoo.NewExecRunner(
			"dadooPathNotUsed",
			"fakeRunW",
			fakeProcessIDGenerator,
			fakePidGetter,
			fakeCommandRunner,
			false,
		)
	})

	Describe("Run", func() {
		It("calls runc with correct parameters", func() {
			_, err := runner.Run(log, processID, &runrunc.PreparedSpec{}, bundlePath, processesPath, "some-handle", nil, garden.ProcessIO{})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCommandRunner.StartedCommands()[0].Args).To(
				ConsistOf(
					"fakeRunW",
					"-debug",
					"-log", filepath.Join(bundlePath, fmt.Sprintf("windows.%s.log", processID)),
					"exec",
					"-p", filepath.Join(bundlePath, "process.json"),
					"-d",
					"-pid-file", filepath.Join(processesPath, processID, "pidfile"),
					"some-handle",
				),
			)
		})

		Context("when a processID is reused concurrently", func() {
			var processID string

			BeforeEach(func() {
				processID = "same-id"
				_, err := runner.Run(log, processID, &runrunc.PreparedSpec{}, bundlePath, processesPath, "some-handle", nil, garden.ProcessIO{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a sensible error", func() {
				_, err := runner.Run(log, processID, &runrunc.PreparedSpec{}, bundlePath, processesPath, "some-handle", nil, garden.ProcessIO{})
				Expect(err).To(MatchError(fmt.Sprintf("process ID '%s' already in use", processID)))
			})
		})

		Context("when the commandRunner fails", func() {
			It("returns a nice error", func() {
				fakeCommandRunner.WhenRunning(fake_command_runner.CommandSpec{}, func(cmd *exec.Cmd) error {
					return errors.New("boom")
				})

				_, err := runner.Run(log, processID, &runrunc.PreparedSpec{Process: specs.Process{}}, bundlePath, processesPath, "some-handle", nil, garden.ProcessIO{})
				Expect(err).To(MatchError(ContainSubstring("boom")))
			})
		})

		Describe("the returned garden.Process", func() {
			Context("when an empty process ID is passed", func() {
				BeforeEach(func() {
					fakeProcessIDGenerator.GenerateReturns("some-generated-id")
				})

				It("has a generated ID", func() {
					process, err := runner.Run(log, "", &runrunc.PreparedSpec{}, bundlePath, processesPath, "some-handle", nil, garden.ProcessIO{})
					Expect(err).NotTo(HaveOccurred())

					Expect(process.ID()).To(Equal("some-generated-id"))
				})
			})

			Context("when a non-empty process ID is passed", func() {
				It("has the passed ID", func() {
					process, err := runner.Run(log, processID, &runrunc.PreparedSpec{}, bundlePath, processesPath, "some-handle", nil, garden.ProcessIO{})
					Expect(err).NotTo(HaveOccurred())

					Expect(process.ID()).To(Equal(processID))
				})
			})

			Describe("Wait", func() {
				It("returns the exit code of the process", func() {
					fakeCommandRunner.WhenRunning(fake_command_runner.CommandSpec{}, func(cmd *exec.Cmd) error {
						Expect(ioutil.WriteFile(filepath.Join(processesPath, processID, "exitcode"), []byte("42"), 0600)).To(Succeed())
						return nil
					})

					process, err := runner.Run(log, processID, &runrunc.PreparedSpec{}, bundlePath, processesPath, "some-handle", nil, garden.ProcessIO{})
					Expect(err).NotTo(HaveOccurred())

					Expect(process.Wait()).To(Equal(42))
				})
			})
		})
	})
})
