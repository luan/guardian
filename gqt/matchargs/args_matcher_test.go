package matchargs_test

import (
	"strings"

	. "code.cloudfoundry.org/guardian/gqt/matchargs"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("ArgsMatcher", func() {
	var _ = DescribeTable(
		"matching",
		func(expected, actual []string, shouldMatch bool) {
			matcher := MatchArgs(expected...)
			matches, err := matcher.Match(actual)
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(Equal(shouldMatch))
		},
		Entry("empty args", args(""), args(""), true),
		Entry("expected empty", args(""), args("-someBool"), false),
		Entry("actual empty", args("--k1=v1"), args(""), false),

		Entry("global flags match", args("-b"), args("-b"), true),
		Entry("global flags match", args("--b"), args("-b"), true),
		Entry("global flags match", args("-k1=v1"), args("-k1=v1"), true),
		Entry("global flags match", args("--k1=v1"), args("-k1=v1"), true),
		Entry("global flags match", args("--k1 v1"), args("--k1 v1"), true),
		Entry("global flags match", args("--k1 v1"), args("--k1=v1"), true),
		Entry("global flags match", args("-b --k1 v1 -c"), args("-k1=v1 --b --c"), true),
		Entry("global flags don't match", args("--k1 v1"), args("--k1=v2"), false),
		Entry("global flags don't match", args("--k1 v1"), args("--k1 v1 -k2=v2"), false),

		Entry("subcmd matches", args("sub"), args("sub"), true),
		Entry("subcmd matches", args("-k1=v1 sub"), args("--k1=v1 sub"), true),
		Entry("subcmd doesn't match", args("sub"), args("nosub"), false),

		Entry("post-subcmd args match", args("sub -k1=v1"), args("sub --k1 v1"), true),
		Entry("post-subcmd args don't match", args("sub -k1=v1"), args("sub --k2 v1"), false),
		Entry("post-subcmd args match", args("-b sub -k1=v1"), args("-b sub --k1 v1"), true),
		Entry("post-subcmd args don't match", args("-b sub -k1=v1"), args("-b sub --k2 v1"), false),

		Entry("global flags and subcmd match", args("-k1 v1 sub"), args("-k1 v1 sub"), true),
		Entry("global flags match, subcmd doesn't match", args("-k1 v1 sub"), args("-k1 v1 sub2"), false),
	)

	Describe("as a gomega matcher", func() {
		It("can be used to match varags", func() {
			Expect(args("-k1 v1")).To(MatchArgs("-k1", "v1"))
		})

		It("can be used to match a single string", func() {
			Expect(args("a")).To(MatchArgs("a"))
		})

		It("returns meaningful failure messages", func() {
			matcher := MatchArgs(args("1 2")...)
			Expect(matcher.FailureMessage(args("3 4"))).To(Equal("expected args '1 2' to match '3 4'"))
		})

		It("returns meaningful messages for when a failure should have occurred, but didn't", func() {
			matcher := MatchArgs(args("1 2")...)
			Expect(matcher.NegatedFailureMessage(args("3 4"))).To(Equal("expected args '1 2' not to match '3 4'"))
		})
	})

	Context("when actual is not a slice of strings", func() {
		It("returns an error", func() {
			matcher := MatchArgs(args("subcmd")...)
			_, err := matcher.Match(2)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when expected is not a valid argument list", func() {
		It("returns an error", func() {
			matcher := MatchArgs(args("a b")...)
			_, err := matcher.Match([]string{"some-subcommand"})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when actual is not a valid argument list", func() {
		It("returns an error", func() {
			matcher := MatchArgs(args("a")...)
			_, err := matcher.Match(args("a b"))
			Expect(err).To(HaveOccurred())
		})
	})
})

func args(s string) []string {
	return strings.Split(s, " ")
}
