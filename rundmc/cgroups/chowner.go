package cgroups

import "os"

//go:generate counterfeiter . OwnerChanger
type OwnerChanger interface {
	Chown(path string) error
}

type OSChowner struct {
	UID int
	GID int
}

func (c *OSChowner) Chown(path string) error {
	return os.Chown(path, c.UID, c.GID)
}
