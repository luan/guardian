package imageplugin

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os/exec"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden-shed/rootfs_provider"
	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/gunk/command_runner"
	errorwrapper "github.com/pkg/errors"
	"github.com/tscolari/lagregator"
)

//go:generate counterfeiter . CommandCreator
type CommandCreator interface {
	CreateCommand(log lager.Logger, handle string, spec rootfs_provider.Spec) (*exec.Cmd, error)
	DestroyCommand(log lager.Logger, handle string) *exec.Cmd
	MetricsCommand(log lager.Logger, handle string) *exec.Cmd
}

type ImagePlugin struct {
	UnprivilegedCommandCreator CommandCreator
	PrivilegedCommandCreator   CommandCreator
	CommandRunner              command_runner.CommandRunner
	DefaultRootfs              string
}

type Image struct {
	Config ImageConfig `json:"config,omitempty"`
}

type ImageConfig struct {
	Env []string `json:"Env,omitempty"`
}

type CreateOutputs struct {
	Rootfs string  `json:"rootfs,omitempty"`
	Image  Image   `json:"image,omitempty"`
	Mounts []Mount `json:"mounts,omitempty"`
}

type Mount struct {
	Options []string `json:"options,omitempty"`
	Source  string   `json:"source"`
	Dest    string   `json:"destination"`
	Type    string   `json:"type"`
}

func (p *ImagePlugin) Create(log lager.Logger, handle string, spec rootfs_provider.Spec) (string, []string, error) {
	log = log.Session("image-plugin-create", lager.Data{"handle": handle, "spec": spec})
	log.Debug("start")
	defer log.Debug("end")

	if spec.RootFS.String() == "" {
		var err error
		spec.RootFS, err = url.Parse(p.DefaultRootfs)

		if err != nil {
			log.Error("parsing-default-rootfs-failed", err)
			return "", nil, errorwrapper.Wrap(err, "parsing default rootfs")
		}
	}

	var (
		createCmd *exec.Cmd
		err       error
	)
	if spec.Namespaced {
		createCmd, err = p.UnprivilegedCommandCreator.CreateCommand(log, handle, spec)
	} else {
		createCmd, err = p.PrivilegedCommandCreator.CreateCommand(log, handle, spec)
	}
	if err != nil {
		return "", nil, errorwrapper.Wrap(err, "creating create command")
	}

	stdoutBuffer := bytes.NewBuffer([]byte{})
	createCmd.Stdout = stdoutBuffer
	createCmd.Stderr = lagregator.NewRelogger(log)

	if err := p.CommandRunner.Run(createCmd); err != nil {
		logData := lager.Data{"action": "create", "stdout": stdoutBuffer.String()}
		log.Error("image-plugin-result", err, logData)
		return "", nil, errorwrapper.Wrapf(err, "running image plugin create: %s", stdoutBuffer.String())
	}

	createOutputs := &CreateOutputs{}
	err = json.Unmarshal(stdoutBuffer.Bytes(), createOutputs)
	if err != nil {
		logData := lager.Data{"action": "create", "stdout": stdoutBuffer.String()}
		log.Error("image-plugin-parsing", err, logData)
		return "", nil, errorwrapper.Wrapf(err, "parsing image plugin create: %s", stdoutBuffer.String())
	}

	return createOutputs.Rootfs, createOutputs.Image.Config.Env, nil
}

func (p *ImagePlugin) Destroy(log lager.Logger, handle string) error {
	log = log.Session("image-plugin-destroy", lager.Data{"handle": handle})
	log.Debug("start")
	defer log.Debug("end")

	var destroyCmds []*exec.Cmd
	destroyCmds = append(destroyCmds, p.UnprivilegedCommandCreator.DestroyCommand(log, handle))
	destroyCmds = append(destroyCmds, p.PrivilegedCommandCreator.DestroyCommand(log, handle))

	for _, destroyCmd := range destroyCmds {
		if destroyCmd == nil {
			continue
		}
		stdoutBuffer := bytes.NewBuffer([]byte{})
		destroyCmd.Stdout = stdoutBuffer
		destroyCmd.Stderr = lagregator.NewRelogger(log)

		if err := p.CommandRunner.Run(destroyCmd); err != nil {
			logData := lager.Data{"action": "destroy", "stdout": stdoutBuffer.String()}
			log.Error("image-plugin-result", err, logData)
			return errorwrapper.Wrapf(err, "running image plugin destroy: %s", stdoutBuffer.String())
		}
	}

	return nil
}

func (p *ImagePlugin) Metrics(log lager.Logger, handle string, namespaced bool) (garden.ContainerDiskStat, error) {
	log = log.Session("image-plugin-metrics", lager.Data{"handle": handle, "namespaced": namespaced})
	log.Debug("start")
	defer log.Debug("end")

	var metricsCmd *exec.Cmd
	if namespaced {
		metricsCmd = p.UnprivilegedCommandCreator.MetricsCommand(log, handle)
	} else {
		metricsCmd = p.PrivilegedCommandCreator.MetricsCommand(log, handle)
	}

	stdoutBuffer := bytes.NewBuffer([]byte{})
	metricsCmd.Stdout = stdoutBuffer
	metricsCmd.Stderr = lagregator.NewRelogger(log)

	if err := p.CommandRunner.Run(metricsCmd); err != nil {
		logData := lager.Data{"action": "metrics", "stdout": stdoutBuffer.String()}
		log.Error("image-plugin-result", err, logData)
		return garden.ContainerDiskStat{}, errorwrapper.Wrapf(err, "running image plugin metrics: %s", stdoutBuffer.String())
	}

	var diskStat map[string]map[string]uint64
	var consumableBuffer = bytes.NewBuffer(stdoutBuffer.Bytes())
	if err := json.NewDecoder(consumableBuffer).Decode(&diskStat); err != nil {
		return garden.ContainerDiskStat{}, errorwrapper.Wrapf(err, "parsing stats: %s", stdoutBuffer.String())
	}

	return garden.ContainerDiskStat{
		TotalBytesUsed:     diskStat["disk_usage"]["total_bytes_used"],
		ExclusiveBytesUsed: diskStat["disk_usage"]["exclusive_bytes_used"],
	}, nil
}

func (p *ImagePlugin) GC(log lager.Logger) error {
	return nil
}
