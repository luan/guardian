package matchargs_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMatchargs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Args Matcher Suite")
}
