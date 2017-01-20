package containerd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/runrunc"
	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/opencontainers/runtime-spec/specs-go"
)

//go:generate counterfeiter . PidGetter
type PidGetter interface {
	Pid(pidFilePath string) (int, error)
}

type ExecRunner struct {
	containerdShimPath string
	runcPath           string
	processIDGen       runrunc.UidGenerator
	pidGetter          PidGetter
	commandRunner      command_runner.CommandRunner
}

type processState struct {
	specs.Process
	Exec           bool     `json:"exec"`
	Stdin          string   `json:"containerdStdin"`
	Stdout         string   `json:"containerdStdout"`
	Stderr         string   `json:"containerdStderr"`
	RuntimeArgs    []string `json:"runtimeArgs"`
	NoPivotRoot    bool     `json:"noPivotRoot"`
	CheckpointPath string   `json:"checkpoint"`
	RootUID        int      `json:"rootUID"`
	RootGID        int      `json:"rootGID"`
}

func NewExecRunner(containerdShimPath, runcPath string, processIDGen runrunc.UidGenerator, pidGetter PidGetter, commandRunner command_runner.CommandRunner) *ExecRunner {
	return &ExecRunner{
		containerdShimPath: containerdShimPath,
		runcPath:           runcPath,
		processIDGen:       processIDGen,
		pidGetter:          pidGetter,
		commandRunner:      commandRunner,
	}
}

func (c *ExecRunner) Run(log lager.Logger, spec *runrunc.PreparedSpec, processesPath, handle string, tty *garden.TTYSpec, pio garden.ProcessIO) (p garden.Process, theErr error) {
	processID := c.processIDGen.Generate()

	processPath := filepath.Join(processesPath, processID)
	if err := os.MkdirAll(processPath, 0700); err != nil {
		return nil, err
	}

	exitPipe, controlPipe, err := getControlPipes(processPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			exitPipe.Close()
			controlPipe.Close()
		}
	}()

	stdin, stdout, stderr, err := getIOPipes(processPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			stdin.Close()
			stdout.Close()
			stderr.Close()
		}
	}()

	process := newProcess(processID, processPath, filepath.Join(processPath, "pidfile"), exitPipe, controlPipe)
	log.Info(fmt.Sprintf("ExecRunner: %#v", c))

	// bundle dir is not really needed on exec - we just need an existing dir to run runc from
	cmd := exec.Command(c.containerdShimPath, handle, ".", c.runcPath)
	log.Info(fmt.Sprintf("cmd: %#v", cmd))
	cmd.Dir = processPath
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	state := processState{
		Exec:           true,
		Process:        spec.Process,
		NoPivotRoot:    false,
		CheckpointPath: "",
		RootUID:        spec.HostUID,
		RootGID:        spec.HostGID,
		Stdin:          filepath.Join(processPath, "stdin"),
		Stdout:         filepath.Join(processPath, "stdout"),
		Stderr:         filepath.Join(processPath, "stderr"),
	}

	f, err := os.Create(filepath.Join(processPath, "process.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to create shim's process.json for container %s: %s", handle, err.Error())
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(state); err != nil {
		return nil, fmt.Errorf("failed to create shim's processState for container %s: %s", handle, err.Error())
	}

	if err := c.commandRunner.Start(cmd); err != nil {
		return nil, err
	}
	go c.commandRunner.Wait(cmd) // wait on spawned process to avoid zombies

	return process, nil
}

func (c *ExecRunner) Attach(log lager.Logger, processID string, io garden.ProcessIO, processesPath string) (garden.Process, error) {
	return nil, errors.New("Not implemented")
}

func getIOPipes(root string) (stdin, stdout, stderr *os.File, err error) {
	path := filepath.Join(root, "stdin")
	if err = syscall.Mkfifo(path, 0700); err != nil {
		return stdin, stdout, stderr, fmt.Errorf("failed to create shim exit fifo: %s", err.Error())
	}
	if stdin, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0); err != nil {
		return stdin, stdout, stderr, fmt.Errorf("failed to create shim exit fifo: %s", err.Error())
	}

	path = filepath.Join(root, "stdout")
	if err = syscall.Mkfifo(path, 0700); err != nil {
		return stdin, stdout, stderr, fmt.Errorf("failed to create shim exit fifo: %s", err.Error())
	}
	if stdout, err = os.OpenFile(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0); err != nil {
		return stdin, stdout, stderr, fmt.Errorf("failed to create shim exit fifo: %s", err.Error())
	}

	path = filepath.Join(root, "stderr")
	if err = syscall.Mkfifo(path, 0700); err != nil {
		return stdin, stdout, stderr, fmt.Errorf("failed to create shim exit fifo: %s", err.Error())
	}
	if stderr, err = os.OpenFile(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0); err != nil {
		return stdin, stdout, stderr, fmt.Errorf("failed to create shim exit fifo: %s", err.Error())
	}

	return stdin, stdout, stderr, nil
}

func getControlPipes(root string) (exitPipe *os.File, controlPipe *os.File, err error) {
	path := filepath.Join(root, "exit")
	if err = syscall.Mkfifo(path, 0700); err != nil {
		return exitPipe, controlPipe, fmt.Errorf("failed to create shim exit fifo: %s", err.Error())
	}
	if exitPipe, err = os.OpenFile(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0); err != nil {
		return exitPipe, controlPipe, fmt.Errorf("failed to open shim exit fifo: %s", err.Error())
	}

	path = filepath.Join(root, "control")
	if err = syscall.Mkfifo(path, 0700); err != nil {
		return exitPipe, controlPipe, fmt.Errorf("failed to create shim control fifo: %s", err.Error())
	}
	if controlPipe, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0); err != nil {
		return exitPipe, controlPipe, fmt.Errorf("failed to open shim control fifo: %s", err.Error())
	}

	return exitPipe, controlPipe, nil
}

type process struct {
	id             string
	exitStatusFile string
	exitChan       chan struct{}
	exitPipe       *os.File
	controlPipe    *os.File
}

func newProcess(id, dir string, pidFilePath string, exitPipe *os.File, controlPipe *os.File) *process {
	return &process{
		id:             id,
		exitStatusFile: filepath.Join(dir, "exitStatus"),
		exitChan:       make(chan struct{}),
		exitPipe:       exitPipe,
		controlPipe:    controlPipe,
	}
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Wait() (int, error) {
	for {
		exitStatus, err := ioutil.ReadFile(p.exitStatusFile)
		if err == nil {
			return strconv.Atoi(string(exitStatus))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (p *process) SetTTY(ttyspec garden.TTYSpec) error {
	return errors.New("Not implemented")
}

func (p *process) Signal(signal garden.Signal) error {
	return errors.New("Not implemented")
}