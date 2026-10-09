package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bored-cli-test-")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "bored")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/bored")
	if outb, err := build.CombinedOutput(); err != nil {
		panic(string(outb))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type run struct {
	t    *testing.T
	home string
}

func newRun(t *testing.T) *run {
	t.Helper()
	r := &run{t: t, home: filepath.Join(t.TempDir(), "store")}
	r.ok("init")
	return r
}

func (r *run) exec(args ...string) (string, int) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "BORED_HOME="+r.home, "BORED_ACTOR=tester")
	outb, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		r.t.Fatalf("exec %v: %v", args, err)
	}
	return string(outb), code
}

func (r *run) ok(args ...string) string {
	r.t.Helper()
	outb, code := r.exec(args...)
	if code != 0 {
		r.t.Fatalf("bored %s: exit %d: %s", strings.Join(args, " "), code, outb)
	}
	return outb
}

func (r *run) code(want int, args ...string) {
	r.t.Helper()
	outb, code := r.exec(args...)
	if code != want {
		r.t.Errorf("bored %s: want exit %d got %d: %s", strings.Join(args, " "), want, code, outb)
	}
}

func (r *run) jsonOf(args ...string) map[string]any {
	r.t.Helper()
	outb := r.ok(append(args, "--json")...)
	var m map[string]any
	if err := json.Unmarshal([]byte(outb), &m); err != nil {
		r.t.Fatalf("bad json from %v: %v\n%s", args, err, outb)
	}
	return m
}

func TestLifecycleAndExitCodes(t *testing.T) {
	r := newRun(t)
	if got := strings.TrimSpace(r.ok("new", "Root", "--slug", "root")); got != "BRD-1" {
		t.Fatalf("new printed %q", got)
	}
	r.ok("new", "Leaf A", "--parent", "root", "--priority", "1", "--ac", "it works")
	r.ok("new", "Leaf B", "--parent", "root", "--ac", "also works", "--depends-on", "2")

	r.code(2, "new", "Dup", "--slug", "root")
	r.code(3, "show", "42")
	r.code(2, "move", "2", "sideways")
	r.code(4, "claim", "3")    // blocked
	r.code(4, "claim", "root") // container (and backlog)
	r.code(5, "next", "--under", "3")

	n := r.jsonOf("next")
	if n["ticket"].(map[string]any)["id"] != "BRD-2" {
		t.Errorf("next = %v", n)
	}
	r.ok("claim", "2")
	r.code(4, "claim", "2")
	r.ok("log", "2", "working on it")
	r.ok("ac", "2", "check", "1")
	r.ok("move", "2", "review", "-m", "ready for review")
	r.code(4, "done", "3")
	r.ok("done", "2", "-m", "lgtm")

	v := r.jsonOf("show", "2")
	if v["status"] != "done" || v["assignee"] != "tester" || v["ac"].(map[string]any)["done"].(float64) != 1 {
		t.Errorf("show after done = %v", v)
	}
	if !strings.Contains(v["body"].(string), "tester: -> done: lgtm") {
		t.Errorf("log line missing from body:\n%s", v["body"])
	}

	ready := r.ok("ready", "--json")
	if !strings.Contains(ready, `"id":"BRD-3"`) {
		t.Errorf("BRD-3 should be ready after BRD-2 done: %s", ready)
	}
	board := r.jsonOf("board")
	if len(board["todo"].([]any)) != 1 {
		t.Errorf("board todo = %v", board["todo"])
	}
	r.code(4, "dep", "add", "2", "3")
	r.code(4, "set", "root", "--parent", "3")
	r.code(2, "set", "root")
	r.code(2, "list", "--bogus")
	r.code(3, "repo", "detect")
}

func TestContainerBecomesWorkable(t *testing.T) {
	r := newRun(t)
	r.ok("new", "Chunk", "--status", "todo", "--ac", "all children done")
	r.ok("new", "Kid", "--parent", "1", "--ac", "x")
	if out := r.ok("board"); strings.Contains(out, "\n  BRD-1 ") {
		t.Errorf("container should be hidden from board:\n%s", out)
	}
	r.code(4, "claim", "1")
	r.ok("claim", "2")
	r.ok("move", "2", "review")
	r.ok("done", "2")
	if out := r.ok("board"); !strings.Contains(out, "\n  BRD-1 ") {
		t.Errorf("container should reappear once children are done:\n%s", out)
	}
	r.ok("claim", "1")
	r.ok("move", "1", "review")
	r.ok("done", "1")
	tree := r.ok("tree", "--all")
	if !strings.Contains(tree, "1/1") {
		t.Errorf("tree progress:\n%s", tree)
	}
}

func TestLabelCommands(t *testing.T) {
	r := newRun(t)
	r.ok("new", "Leaf", "--ac", "x")
	r.ok("label", "add", "1", "stuck")
	r.code(4, "label", "add", "1", "stuck")
	r.code(5, "ready")
	r.code(4, "claim", "1")
	r.ok("label", "rm", "1", "stuck")
	r.code(3, "label", "rm", "1", "stuck")
	r.ok("claim", "1")
	v := r.jsonOf("show", "1")
	if v["status"] != "doing" {
		t.Errorf("claim after unstick: %v", v["status"])
	}
}
