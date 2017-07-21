package guardiancmd

import (
	"fmt"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/guardian/rundmc"
	"code.cloudfoundry.org/lager"
)

type SetupCommand struct {
	LogLevel LagerFlag
	Logger   lager.Logger

	Tag         string `hidden:"true" long:"tag" description:"Optional 2-character identifier used for namespacing global configuration."`
	RootlessUID int    `hidden:"true" long:"rootless-uid" description:"UID that guardian will run as"`
	RootlessGID int    `hidden:"true" long:"rootless-gid" description:"GID that guardian will run as"`
}

func (cmd *SetupCommand) Execute(args []string) error {
	cmd.Logger, _ = cmd.LogLevel.Logger("guardian-setup")

	cgroupsMountpoint := "/sys/fs/cgroup"
	if cmd.Tag != "" {
		cgroupsMountpoint = filepath.Join(os.TempDir(), fmt.Sprintf("cgroups-%s", cmd.Tag))
	}
	chowner := &rundmc.OwnerChanger{UID: cmd.RootlessUID, GID: cmd.RootlessUID}
	cgroupStarter := rundmc.NewStarter(cmd.Logger, mustOpen("/proc/cgroups"), mustOpen("/proc/self/cgroup"), cgroupsMountpoint, commandRunner(), chowner)
	return cgroupStarter.Start()
}
