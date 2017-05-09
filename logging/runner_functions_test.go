package logging_test

import (
	"bytes"
	"os/exec"
	"time"

	"code.cloudfoundry.org/guardian/logging"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	. "github.com/onsi/gomega"
)

func assertLogDuration(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	err := runner.Run(exec.Command(argv[0], argv[1:]...))
	Expect(err).ToNot(HaveOccurred())

	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	log := logger.TestSink.Logs()[1]

	took := log.Data["took"].(string)
	Expect(took).ToNot(BeEmpty())

	duration, err := time.ParseDuration(took)
	Expect(err).ToNot(HaveOccurred())
	Expect(duration).To(BeNumerically(">=", 1*time.Second))
}

func assertLogCommandArguments(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	err := runner.Run(exec.Command(argv[0], argv[1:]...))
	Expect(err).ToNot(HaveOccurred())

	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	args := make([]interface{}, len(argv))
	for i := range argv {
		args[i] = argv[i]
	}

	log := logger.TestSink.Logs()[0]
	Expect(log.LogLevel).To(Equal(lager.DEBUG))
	Expect(log.Message).To(Equal("test.command.starting"))
	Expect(log.Data["argv"]).To(Equal(args))

	log = logger.TestSink.Logs()[1]
	Expect(log.LogLevel).To(Equal(lager.DEBUG))
	Expect(log.Message).To(Equal("test.command.succeeded"))
	Expect(log.Data["argv"]).To(Equal(args))

}

func assertLogExistStatusDebug(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	err := runner.Run(exec.Command(argv[0], argv[1:]...))
	Expect(err).ToNot(HaveOccurred())
	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	log := logger.TestSink.Logs()[1]
	Expect(log.LogLevel).To(Equal(lager.DEBUG))
	Expect(log.Message).To(Equal("test.command.succeeded"))
	Expect(log.Data["exit-status"]).To(Equal(float64(0))) // JSOOOOOOOOOOOOOOOOOOON
}

func assertNotLogStdoutErr(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	err := runner.Run(exec.Command(argv[0], argv[1:]...))
	Expect(err).ToNot(HaveOccurred())

	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	log := logger.TestSink.Logs()[1]
	Expect(log.LogLevel).To(Equal(lager.DEBUG))
	Expect(log.Message).To(Equal("test.command.succeeded"))
	Expect(log.Data).ToNot(HaveKey("stdout"))
	Expect(log.Data).ToNot(HaveKey("stderr"))
}

func assertLogErrors(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	err := runner.Run(exec.Command(argv[0], argv[1:]...))
	Expect(err).To(HaveOccurred())

	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	log := logger.TestSink.Logs()[1]
	Expect(log.LogLevel).To(Equal(lager.ERROR))
	Expect(log.Message).To(Equal("test.command.failed"))
	Expect(log.Data["error"]).ToNot(BeEmpty())
	Expect(log.Data).ToNot(HaveKey("exit-status"))
}

func assertLogStatusWithErrorLevel(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	err := runner.Run(exec.Command(argv[0], argv[1:]...))
	Expect(err).To(HaveOccurred())

	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	log := logger.TestSink.Logs()[1]
	Expect(log.LogLevel).To(Equal(lager.ERROR))
	Expect(log.Message).To(Equal("test.command.failed"))
	Expect(log.Data["error"]).To(Equal("exit status 1"))
	Expect(log.Data["exit-status"]).To(Equal(float64(1))) // JSOOOOOOOOOOOOOOOOOOON
}

func assertReportsStdoutErr(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	err := runner.Run(exec.Command(argv[0], argv[1:]...))
	Expect(err).To(HaveOccurred())

	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	log := logger.TestSink.Logs()[1]
	Expect(log.LogLevel).To(Equal(lager.ERROR))
	Expect(log.Message).To(Equal("test.command.failed"))
	Expect(log.Data["stdout"]).To(Equal("hi out\n"))
	Expect(log.Data["stderr"]).To(Equal("hi err\n"))
}

func assertMultiplexesCallerLogs(runner *logging.Runner, logger *lagertest.TestLogger, argv []string) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := runner.Run(cmd)
	Expect(err).To(HaveOccurred())

	Expect(logger.TestSink.Logs()).To(HaveLen(2))

	log := logger.TestSink.Logs()[1]
	Expect(log.LogLevel).To(Equal(lager.ERROR))
	Expect(log.Message).To(Equal("test.command.failed"))
	Expect(log.Data["stdout"]).To(Equal("hi out\n"))
	Expect(log.Data["stderr"]).To(Equal("hi err\n"))

	Expect(stdout.String()).To(Equal("hi out\n"))
	Expect(stderr.String()).To(Equal("hi err\n"))
}
