package render

import (
	"strings"
	"testing"
)

func TestEscapeAngles(t *testing.T) {
	in := "picks with `next --as <actor>` and then <actor> again\n\n```\ncode <actor>\n```\nalready \\<escaped> and a < b\n"
	got := EscapeAngles(in)
	want := "picks with `next --as <actor>` and then \\<actor> again\n\n```\ncode <actor>\n```\nalready \\<escaped> and a \\< b\n"
	if got != want {
		t.Errorf("EscapeAngles:\n got %q\nwant %q", got, want)
	}
}

func TestMarkdownKeepsPlaceholders(t *testing.T) {
	out := MarkdownWith("stops on exit 5 after <actor> picks\n", 80, true)
	if !strings.Contains(out, "<actor>") {
		t.Errorf("placeholder dropped from rendered markdown: %q", out)
	}
}
