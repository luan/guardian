package matchargs

import (
	"fmt"
	"sort"
	"strings"
)

type ArgsMatcher struct {
	expected []string
}

func MatchArgs(expected ...string) *ArgsMatcher {
	return &ArgsMatcher{expected: expected}
}

func (m *ArgsMatcher) Match(actual interface{}) (success bool, err error) {
	actualArgs, ok := actual.([]string)
	if !ok {
		return false, fmt.Errorf("MatchArgs expects a ...string")
	}
	actualGlobalFlags, actualSubcmd, actualSubcmdFlags, err := destructureArgs(actualArgs)
	if err != nil {
		return false, err
	}

	expectedGlobalFlags, expectedSubcmd, expectedSubcmdFlags, err := destructureArgs(m.expected)
	if err != nil {
		return false, err
	}

	return equalsInAnyOrder(actualGlobalFlags, expectedGlobalFlags) &&
			actualSubcmd == expectedSubcmd &&
			equalsInAnyOrder(actualSubcmdFlags, expectedSubcmdFlags),
		nil
}

func destructureArgs(args []string) ([]string, string, []string, error) {
	globalFlags, subcmdIndex := parseFlags(args)
	if subcmdIndex == len(args) {
		return globalFlags, "", nil, nil
	}

	subcmd := args[subcmdIndex]

	postSubcmdArgs := args[subcmdIndex+1:]
	subcmdFlags, endOfArgs := parseFlags(postSubcmdArgs)
	if endOfArgs != len(postSubcmdArgs) {
		return nil, "", nil, fmt.Errorf("invalid arguments: %s", strings.Join(args, " "))
	}

	return globalFlags, subcmd, subcmdFlags, nil
}

func parseFlags(args []string) ([]string, int) {
	var flags []string

	var hangingKey string
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			arg = "-" + arg
		}

		if isKeyAndValue(arg) {
			if hangingKey != "" {
				flags = append(flags, hangingKey)
			}
			flags = append(flags, arg)
			hangingKey = ""
		} else if isKey(arg) {
			if hangingKey != "" {
				flags = append(flags, hangingKey)
			}
			hangingKey = arg
		} else if hangingKey != "" {
			flags = append(flags, fmt.Sprintf("%s=%s", hangingKey, arg))
			hangingKey = ""
		} else {
			sort.Strings(flags)
			return flags, i
		}
	}

	if hangingKey != "" {
		flags = append(flags, hangingKey)
	}

	sort.Strings(flags)
	return flags, len(args)
}

func isKey(s string) bool {
	return strings.HasPrefix(s, "--")
}

func isKeyAndValue(s string) bool {
	return isKey(s) && strings.Contains(s, "=")
}

func (m *ArgsMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected args '%s' to match '%s'", joinArgs(m.expected), joinArgs(actual))
}

func (m *ArgsMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected args '%s' not to match '%s'", joinArgs(m.expected), joinArgs(actual))
}

func joinArgs(args interface{}) string {
	return strings.Join(args.([]string), " ")
}

func equalsInAnyOrder(a1, a2 []string) bool {
	sort.Strings(a1)
	sort.Strings(a2)

	if len(a1) != len(a2) {
		return false
	}

	for i := 0; i < len(a1); i++ {
		if a1[i] != a2[i] {
			return false
		}
	}

	return true
}
