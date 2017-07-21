package rundmc

import "os"

type OwnerChanger struct {
	UID int
	GID int
}

func (c *OwnerChanger) Chown(path string) error {
	return os.Chown(path, c.UID, c.GID)
}
