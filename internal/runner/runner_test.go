package runner_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Commonwealth-Labs/bored/internal/git"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/runner"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

var binDir string

const fakeClaude = `#!/bin/sh
# Fake claude: behaves according to $FAKE_MODE and the prompt.
PROMPT="$2"
case "$PROMPT" in
  "/bored-review "*)
    echo '{"type":"result","is_error":false,"result":"Verdict: recommend done. All criteria met.","total_cost_usd":0.02,"num_turns":2}'
    exit 0;;
esac
ID=$(printf '%s' "$PROMPT" | sed -n 's|^/bored-work \([A-Za-z]*-[0-9]*\).*|\1|p')
bored repo detect --json > ./detect.json 2>/dev/null || echo '{"none":true}' > ./detect.json
case "$FAKE_MODE" in
  success)
    bored claim "$ID" --as "$BORED_ACTOR" >/dev/null || exit 1
    bored log "$ID" "fake work in $PWD" --as "$BORED_ACTOR" >/dev/null
    bored move "$ID" review -m "fake handover" --as "$BORED_ACTOR" >/dev/null ;;
  die)
    bored claim "$ID" --as "$BORED_ACTOR" >/dev/null ;;
  crash)
    echo "boom" >&2; exit 1 ;;
esac
echo '{"type":"result","is_error":false,"result":"fake done","total_cost_usd":0.01,"num_turns":3}'
`

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bored-runner-test-")
	if err != nil {
		panic(err)
	}
	binDir = dir
	if out, err := exec.Command("go", "build", "-o", filepath.Join(dir, "bored"), "../../cmd/bored").CombinedOutput(); err != nil {
		panic(string(out))
	}
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(fakeClaude), 0o755); err != nil {
		panic(err)
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Init(filepath.Join(t.TempDir(), "store"), "T")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTicket(t *testing.T, s *store.Store, title, repo string) string {
	t.Helper()
	var id string
	err := s.Mutate("new", func(tx *store.Tx) error {
		tk := &model.Ticket{Title: title, Priority: 2, Status: model.StatusTodo, Repo: repo, Body: model.NewBody("", []string{"done"})}
		if err := tx.Create(tk); err != nil {
			return err
		}
		id = tk.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func opts(t *testing.T, mode string) runner.Options {
	t.Helper()
	os.Setenv("FAKE_MODE", mode)
	return runner.Options{Actor: "claude", Max: 1, Cwd: t.TempDir(), Out: &bytes.Buffer{}, Worktree: true}
}

func status(t *testing.T, s *store.Store, id string) model.Status {
	t.Helper()
	tk, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	return tk.Status
}

func TestSuccessHandsOverToReview(t *testing.T) {
	s := newStore(t)
	id := newTicket(t, s, "Think about it", "")
	o := opts(t, "success")
	res, err := runner.Run(s, o)
	if err != nil || len(res) != 1 || res[0].Outcome != runner.OutcomeReview {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if status(t, s, id) != model.StatusReview {
		t.Errorf("ticket should be in review")
	}
	if res[0].CostUSD != 0.01 || res[0].Turns != 3 {
		t.Errorf("cost/turns not parsed: %+v", res[0])
	}
	for _, f := range []string{"stdout.json", "stderr.log", "meta.json"} {
		if _, err := os.Stat(filepath.Join(res[0].RunDir, f)); err != nil {
			t.Errorf("missing %s in run dir", f)
		}
	}
	ig, _ := os.ReadFile(filepath.Join(s.Root, ".gitignore"))
	if !strings.Contains(string(ig), "runs/") || !strings.Contains(string(ig), "worktrees/") {
		t.Errorf(".gitignore not updated: %q", ig)
	}
	if ok, _ := git.HasChanges(s.Root); ok {
		t.Error("runs/ should be ignored, store has uncommitted changes")
	}
}

func TestDyingSessionIsReleasedAndLoopStops(t *testing.T) {
	s := newStore(t)
	id := newTicket(t, s, "Hard one", "")
	o := opts(t, "die")
	o.Max = 0
	res, err := runner.Run(s, o)
	if !errors.Is(err, runner.ErrStoppedOnFailures) {
		t.Fatalf("expected stop on failures, got err=%v res=%+v", err, res)
	}
	if len(res) != 2 || res[0].Outcome != runner.OutcomeRecover {
		t.Fatalf("res=%+v", res)
	}
	tk, _ := s.Load(id)
	if tk.Status != model.StatusTodo || tk.Assignee != "" {
		t.Errorf("ticket should be released: %s %q", tk.Status, tk.Assignee)
	}
	if !strings.Contains(tk.Body, "runner: session ended without handover") {
		t.Errorf("release not logged:\n%s", tk.Body)
	}
}

func TestCrashIsAnError(t *testing.T) {
	s := newStore(t)
	newTicket(t, s, "Crashy", "")
	res, err := runner.Run(s, opts(t, "crash"))
	if err != nil || len(res) != 1 || res[0].Outcome != runner.OutcomeError || !strings.Contains(res[0].Note, "boom") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestNothingReady(t *testing.T) {
	s := newStore(t)
	_, err := runner.Run(s, opts(t, "success"))
	if !errors.Is(err, runner.ErrNothingReady) {
		t.Fatalf("expected nothing ready, got %v", err)
	}
}

func TestMaxLimitsTickets(t *testing.T) {
	s := newStore(t)
	newTicket(t, s, "One", "")
	newTicket(t, s, "Two", "")
	o := opts(t, "success")
	o.Max = 1
	res, err := runner.Run(s, o)
	if err != nil || len(res) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	o.Max = 0
	res, err = runner.Run(s, o)
	if err != nil || len(res) != 1 {
		t.Fatalf("second run should take the remaining one: res=%+v err=%v", res, err)
	}
}

func TestWorktreePerRepoTicket(t *testing.T) {
	s := newStore(t)
	repo := filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(repo, 0o755)
	if err := git.Init(repo, "main"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644)
	exec.Command("git", "-C", repo, "add", "-A").Run()
	exec.Command("git", "-C", repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init").Run()
	if _, err := s.AddRepo("proj", repo, "main"); err != nil {
		t.Fatal(err)
	}
	id := newTicket(t, s, "Code thing", "proj")
	res, err := runner.Run(s, opts(t, "success"))
	if err != nil || len(res) != 1 || res[0].Outcome != runner.OutcomeReview {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	want := filepath.Join(s.Root, "worktrees", "proj", strings.ToLower(id))
	if res[0].Dir != want {
		t.Fatalf("dir = %s, want %s", res[0].Dir, want)
	}
	if git.CurrentBranch(want) != strings.ToLower(id) {
		t.Errorf("branch = %q", git.CurrentBranch(want))
	}
	det, _ := os.ReadFile(filepath.Join(want, "detect.json"))
	if !strings.Contains(string(det), `"name":"proj"`) {
		t.Errorf("repo detect inside the worktree should find proj: %s", det)
	}
	// Re-running reuses the worktree (ticket back to todo first).
	s.Mutate("reset", func(tx *store.Tx) error {
		tk, _ := tx.Resolve(id)
		model.ApplyMove(tk, model.StatusTodo)
		tx.Put(tk)
		return nil
	})
	res, err = runner.Run(s, opts(t, "success"))
	if err != nil || res[0].Dir != want {
		t.Fatalf("second run: res=%+v err=%v", res, err)
	}
	// --no-worktree runs in the repo itself.
	s.Mutate("reset", func(tx *store.Tx) error {
		tk, _ := tx.Resolve(id)
		model.ApplyMove(tk, model.StatusTodo)
		tx.Put(tk)
		return nil
	})
	o := opts(t, "success")
	o.Worktree = false
	res, _ = runner.Run(s, o)
	if real, _ := filepath.EvalSymlinks(repo); res[0].Dir != real && res[0].Dir != repo {
		t.Errorf("--no-worktree dir = %s", res[0].Dir)
	}
}

func TestDryRun(t *testing.T) {
	s := newStore(t)
	id := newTicket(t, s, "Peek", "")
	o := opts(t, "success")
	o.DryRun = true
	var buf bytes.Buffer
	o.Out = &buf
	res, err := runner.Run(s, o)
	if err != nil || len(res) != 1 || res[0].Outcome != runner.OutcomeDryRun {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if status(t, s, id) != model.StatusTodo || !strings.Contains(buf.String(), id) {
		t.Errorf("dry run should not touch the ticket; out=%q", buf.String())
	}
}

func TestReviewPassLogsVerdict(t *testing.T) {
	s := newStore(t)
	id := newTicket(t, s, "Reviewed", "")
	o := opts(t, "success")
	o.Review = true
	res, err := runner.Run(s, o)
	if err != nil || len(res) != 1 || !strings.Contains(res[0].Review, "recommend done") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	tk, _ := s.Load(id)
	if !strings.Contains(tk.Body, "review (runner): Verdict: recommend done") {
		t.Errorf("verdict not logged:\n%s", tk.Body)
	}
	if tk.Status != model.StatusReview {
		t.Errorf("review pass must not change status: %s", tk.Status)
	}
	if res[0].CostUSD < 0.03 {
		t.Errorf("review cost not added: %v", res[0].CostUSD)
	}
}
