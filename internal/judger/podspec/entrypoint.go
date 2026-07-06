package podspec

import (
	"fmt"
	"strings"
)

// GenerateEntrypointScript builds a /bin/sh -c script that runs each command
// in `steps` sequentially, echoing "--- Executing Command N ---" and
// "--- Exit Code: N ---" markers around each, and aborts on the first non-zero exit.
func GenerateEntrypointScript(steps [][]string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nset -e\n")
	if len(steps) == 0 {
		b.WriteString("exit 0\n")
		return b.String()
	}
	for i, cmd := range steps {
		joined := strings.Join(cmd, " ")
		escaped := strings.ReplaceAll(joined, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		fmt.Fprintf(&b, "echo '--- Executing Command %d ---'\n", i+1)
		fmt.Fprintf(&b, "eval \"%s\"\n", escaped)
		b.WriteString("ec=$?\n")
		fmt.Fprintf(&b, "echo '--- Exit Code: $ec ---'\n")
		b.WriteString("[ $ec -ne 0 ] && exit $ec\n")
	}
	b.WriteString("exit $ec\n")
	return b.String()
}
