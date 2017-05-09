package logging_test

import (
	"os/exec"

	"code.cloudfoundry.org/guardian/logging"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Logging Runner", func() {
	var innerRunner command_runner.CommandRunner
	var logger *lagertest.TestLogger
	var runner *logging.Runner

	BeforeEach(func() {
		innerRunner = fake_command_runner.New()
		logger = lagertest.NewTestLogger("test")
	})

	JustBeforeEach(func() {
		runner = &logging.Runner{
			CommandRunner: innerRunner,
			Logger:        logger,
		}
	})

	Describe("delegation", func() {
		It("runs using the provided runner", func() {
			err := runner.Run(exec.Command("morgan-freeman"))
			Expect(err).ToNot(HaveOccurred())

			Expect(innerRunner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "morgan-freeman",
			}))
		})
	})

})
