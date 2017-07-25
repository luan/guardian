package bundlerules_test

import (
	"code.cloudfoundry.org/guardian/gardener"
	"code.cloudfoundry.org/guardian/rundmc/bundlerules"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CGroup Path", func() {
	It("sets the correct cgroup path in the bundle", func() {
		newBndl, err := bundlerules.CGroupPath{}.Apply(goci.Bundle(), gardener.DesiredContainerSpec{
			Hostname: "banana",
		}, "not-needed-path")
		Expect(err).NotTo(HaveOccurred())

		Expect(newBndl.CGroupPath()).To(Equal("garden/banana"))
	})

})
