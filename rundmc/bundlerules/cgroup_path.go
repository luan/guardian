package bundlerules

import (
	"code.cloudfoundry.org/guardian/gardener"
	"code.cloudfoundry.org/guardian/rundmc/goci"
)

type CGroupPath struct {
}

func (l CGroupPath) Apply(bndl goci.Bndl, spec gardener.DesiredContainerSpec, _ string) (goci.Bndl, error) {
	subFolder := spec.Hostname
	return bndl.WithCGroupPath("garden/" + subFolder), nil
}
