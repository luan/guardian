package cgroups

import (
	"os"
	"path/filepath"
)

//go:generate counterfeiter . Chowner
type Chowner interface {
	RecursiveChown(path string) error
}

type OSChowner struct {
	UID int
	GID int
}

func (c *OSChowner) RecursiveChown(path string) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err == nil {
			err = os.Chown(name, c.UID, c.GID)
		}
		return err
	})
}
