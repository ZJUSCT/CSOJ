package podspec

import (
	"strings"
	"testing"
)

func TestGenerateEntrypointScript_SingleCommand(t *testing.T) {
	got := generateEntrypointScript([][]string{{"echo", "hello"}})
	if !strings.Contains(got, `eval "echo hello"`) {
		t.Errorf("expected script to eval the command; got:\n%s", got)
	}
	if !strings.Contains(got, "set -e") {
		t.Errorf("expected `set -e`; got:\n%s", got)
	}
}

func TestGenerateEntrypointScript_MultipleCommands(t *testing.T) {
	got := generateEntrypointScript([][]string{{"gcc", "main.c"}, {"./a.out"}})
	if !strings.Contains(got, `eval "gcc main.c"`) {
		t.Errorf("missing first command; got:\n%s", got)
	}
	if !strings.Contains(got, `eval "./a.out"`) {
		t.Errorf("missing second command; got:\n%s", got)
	}
	if strings.Index(got, "gcc main.c") > strings.Index(got, "./a.out") {
		t.Errorf("commands out of order; got:\n%s", got)
	}
}

func TestGenerateEntrypointScript_Empty(t *testing.T) {
	got := generateEntrypointScript(nil)
	if !strings.Contains(got, "exit 0") {
		t.Errorf("expected an exit 0 no-op; got:\n%s", got)
	}
}

func TestGenerateEntrypointScript_EscapesQuotes(t *testing.T) {
	got := generateEntrypointScript([][]string{{"echo", `he said "hi"`}})
	if !strings.Contains(got, `he said \"hi\"`) {
		t.Errorf("expected escaped quotes; got:\n%s", got)
	}
}
