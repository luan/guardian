package dadoo

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/runrunc"
	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/gunk/command_runner"
)

type WindowsExecRunner struct {
	runwPath      string
	processIDGen  runrunc.UidGenerator
	pidGetter     PidGetter
	commandRunner command_runner.CommandRunner
}

func NewExecRunner(dadooPath, runwPath string, processIDGen runrunc.UidGenerator, pidGetter PidGetter, commandRunner command_runner.CommandRunner, shouldCleanup bool) *WindowsExecRunner {
	return &WindowsExecRunner{
		runwPath:      runwPath,
		processIDGen:  processIDGen,
		pidGetter:     pidGetter,
		commandRunner: commandRunner,
	}
}

func (w *WindowsExecRunner) Run(log lager.Logger, processID string, spec *runrunc.PreparedSpec, bundlePath, processesPath, handle string, tty *garden.TTYSpec, io garden.ProcessIO) (garden.Process, error) {
	if processID == "" {
		processID = w.processIDGen.Generate()
	}

	processPath := filepath.Join(processesPath, processID)

	_, err := os.Stat(processPath)
	if err == nil {
		return nil, errors.New(fmt.Sprintf("process ID '%s' already in use", processID))
	}

	if err := os.MkdirAll(processPath, 0700); err != nil {
		return nil, err
	}

	runwCmd := exec.Command(
		w.runwPath,
		"-debug",
		"-log", filepath.Join(bundlePath, fmt.Sprintf("windows.%s.log", processID)),
		"exec",
		"-p", filepath.Join(bundlePath, "process.json"),
		"-d",
		"-pid-file", filepath.Join(processPath, "pidfile"),
		handle,
	)

	if err := w.commandRunner.Start(runwCmd); err != nil {
		return nil, err
	}

	return &process{
		id:       processID,
		exitcode: filepath.Join(processPath, "exitcode"),
	}, nil
}

type process struct {
	id       string
	exitcode string
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Wait() (int, error) {
	exitcode, err := ioutil.ReadFile(p.exitcode)
	if err != nil {
		panic(err)
	}

	code, err := strconv.Atoi(string(exitcode))
	if err != nil {
		panic(err)
	}

	return code, nil
}
func (p *process) SetTTY(garden.TTYSpec) error {
	return nil
}
func (p *process) Signal(garden.Signal) error {
	return nil
}

func (w *WindowsExecRunner) Attach(log lager.Logger, processID string, io garden.ProcessIO, processesPath string) (garden.Process, error) {
	return nil, nil
}
