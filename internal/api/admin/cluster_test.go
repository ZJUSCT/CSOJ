package admin

import "testing"

func TestValidateQueueMode(t *testing.T) {
	for input, want := range map[string]string{
		"":        "channel",
		"channel": "channel",
		" KUEUE ": "kueue",
	} {
		got, err := validateQueueMode(input)
		if err != nil || got != want {
			t.Fatalf("validateQueueMode(%q) = %q, %v; want %q, nil", input, got, err, want)
		}
	}
	if _, err := validateQueueMode("other"); err == nil {
		t.Fatal("validateQueueMode accepted an unsupported mode")
	}
}
