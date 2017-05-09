package logging_test

import (
	"code.cloudfoundry.org/guardian/logging"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("Logging Runner", func() {
	var innerRunner command_runner.CommandRunner
	var logger *lagertest.TestLogger

	var runner *logging.Runner

	BeforeEach(func() {
		innerRunner = linux_command_runner.New()
		logger = lagertest.NewTestLogger("test")
	})

	JustBeforeEach(func() {
		runner = &logging.Runner{
			CommandRunner: innerRunner,
			Logger:        logger,
		}
	})

	It("logs the duration it took to run the command", func() {
		args := []string{"sleep", "1"}
		assertLogDuration(runner, logger, args)
	})

	It("logs the command's argv", func() {
		args := []string{"bash", "-c", "echo sup"}
		assertLogCommandArguments(runner, logger, args)
	})

	Describe("running a command that exits normally", func() {
		It("logs its exit status with 'debug' level", func() {
			args := []string{"true"}
			assertLogExistStatusDebug(runner, logger, args)
		})

		Context("when the command has output to stdout/stderr", func() {
			It("does not log stdout/stderr", func() {
				args := []string{"sh", "-c", "echo hi out; echo hi err >&2"}
				assertNotLogStdoutErr(runner, logger, args)
			})
		})
	})

	Describe("running a bogus command", func() {
		It("logs the error", func() {
			args := []string{"morgan-freeman"}
			assertLogErrors(runner, logger, args)
		})
	})

	Describe("running a command that exits nonzero", func() {
		It("logs its status with 'error' level", func() {
			args := []string{"false"}
			assertLogStatusWithErrorLevel(runner, logger, args)
		})

		Context("when the command has output to stdout/stderr", func() {
			It("reports the stdout/stderr in the log data", func() {
				args := []string{"sh", "-c", "echo hi out; echo hi err >&2; exit 1"}
				assertReportsStdoutErr(runner, logger, args)
			})

			Context("and it is being collected by the caller", func() {
				It("multiplexes to the caller and the logs", func() {
					args := []string{"sh", "-c", "echo hi out; echo hi err >&2; exit 1"}
					assertMultiplexesCallerLogs(runner, logger, args)
				})
			})
		})
	})
})
