package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Commonwealth-Labs/bored/internal/git"
	"github.com/Commonwealth-Labs/bored/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	s, err := Init(root, "TST")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRenderParseRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 9, 10, 30, 0, 0, time.UTC)
	claimed := now.Add(time.Hour)
	in := &model.Ticket{
		ID: "TST-12", Title: "Add OAuth login", Status: model.StatusDoing,
		Parent: "TST-10", Slug: "oauth", Repo: "h1v3", Priority: 2,
		DependsOn: []string{"TST-11"}, Labels: []string{"auth", "backend"},
		Assignee: "claude", Created: now, Updated: now, ClaimedAt: &claimed, Branch: "feat/oauth",
		Body: NewBody("Why and what.", []string{"Login works", "Cookie set"}),
	}
	data, err := RenderTicket(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"id: TST-12\n", "depends_on: [TST-11]\n", "labels: [auth, backend]\n", "created: 2026-10-09T10:30:00Z\n", "claimed_at: 2026-10-09T11:30:00Z\n", "branch: feat/oauth\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered output missing %q:\n%s", want, s)
		}
	}
	out, err := ParseTicket(data)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != in.ID || out.Title != in.Title || out.Status != in.Status || out.Parent != in.Parent ||
		out.Slug != in.Slug || out.Repo != in.Repo || out.Priority != in.Priority || out.Assignee != in.Assignee ||
		out.Branch != in.Branch || !out.Created.Equal(in.Created) || out.ClaimedAt == nil || !out.ClaimedAt.Equal(claimed) {
		t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	if strings.Join(out.DependsOn, ",") != "TST-11" || strings.Join(out.Labels, ",") != "auth,backend" {
		t.Errorf("lists mismatch: %v %v", out.DependsOn, out.Labels)
	}
	if out.Body != in.Body {
		t.Errorf("body not byte-identical:\n%q\n%q", in.Body, out.Body)
	}
	// Render again: must be stable.
	data2, _ := RenderTicket(out)
	if string(data2) != s {
		t.Errorf("second render differs:\n%s\n---\n%s", s, data2)
	}
}

func TestParseMinimal(t *testing.T) {
	src := "---\nid: TST-1\ntitle: Hello\nstatus: backlog\n---\n"
	tk, err := ParseTicket([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Priority != 3 || tk.Body != "" || tk.ClaimedAt != nil || len(tk.Labels) != 0 || tk.DependsOn == nil {
		t.Errorf("defaults wrong: %+v", tk)
	}
	if err := tk.Validate(); err != nil {
		t.Errorf("minimal ticket should validate: %v", err)
	}
}

func TestBodyOps(t *testing.T) {
	body := NewBody("desc", []string{"a", "b"})
	ts := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	body = AppendLog(body, ts, "claude", "claimed")
	body = AppendLog(body, ts.Add(time.Minute), "claude", "did a thing")
	if !strings.HasSuffix(body, "## Log\n- 2026-10-09 12:00 claude: claimed\n- 2026-10-09 12:01 claude: did a thing\n") {
		t.Errorf("log append wrong:\n%s", body)
	}
	done, total := CountAC(body)
	if done != 0 || total != 2 {
		t.Errorf("ac count %d/%d", done, total)
	}
	body, err := SetAC(body, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := CountAC(body); d != 1 {
		t.Errorf("expected 1 done after check:\n%s", body)
	}
	if _, err := SetAC(body, 3, true); err == nil {
		t.Error("expected error for missing criterion")
	}
	body = AddAC(body, "c")
	if _, total := CountAC(body); total != 3 {
		t.Errorf("expected 3 after add:\n%s", body)
	}
	if got := ListAC(body)[2].Text; got != "c" {
		t.Errorf("third ac = %q", got)
	}
	if Section(body, SecDescription) != "desc" {
		t.Errorf("description section = %q", Section(body, SecDescription))
	}
	body = SetSection(body, SecPlan, "step 1\nstep 2")
	if Section(body, SecPlan) != "step 1\nstep 2" {
		t.Errorf("plan section = %q\n%s", Section(body, SecPlan), body)
	}
	// Log must survive plan edit.
	if d, _ := CountAC(body); d != 1 || !strings.Contains(body, "claude: claimed") {
		t.Errorf("earlier edits lost:\n%s", body)
	}
	// Log on a body without the section.
	b2 := AppendLog("just text\n", ts, "x", "y")
	if b2 != "just text\n\n## Log\n- 2026-10-09 12:00 x: y\n" {
		t.Errorf("log on bare body:\n%q", b2)
	}
}

func TestAllocateConcurrent(t *testing.T) {
	s := newTestStore(t)
	const n = 50
	var wg sync.WaitGroup
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := s.allocate()
			if err != nil {
				t.Error(err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate id %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Errorf("got %d ids, want %d", len(seen), n)
	}
}

func TestMutateCreatesSavesCommits(t *testing.T) {
	if !git.Available() {
		t.Skip("git not available")
	}
	s := newTestStore(t)
	before := git.CommitCount(s.Root)
	err := s.Mutate("bored: new", func(tx *Tx) error {
		return tx.Create(&model.Ticket{Title: "First", Slug: "first", Body: NewBody("", nil)})
	})
	if err != nil {
		t.Fatal(err)
	}
	tk, err := s.Load("TST-1")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != model.StatusBacklog || tk.Priority != 3 || tk.Created.IsZero() {
		t.Errorf("defaults not applied: %+v", tk)
	}
	if git.CommitCount(s.Root) != before+1 {
		t.Errorf("expected one commit, got %d -> %d", before, git.CommitCount(s.Root))
	}
	// Duplicate slug rejected and placeholder released.
	err = s.Mutate("bored: new", func(tx *Tx) error {
		return tx.Create(&model.Ticket{Title: "Second", Slug: "first"})
	})
	if err == nil {
		t.Fatal("expected duplicate slug error")
	}
	if _, err := os.Stat(s.PathFor("TST-2")); err == nil {
		t.Error("placeholder TST-2 not released")
	}
	// Failing fn leaves no tmp files.
	entries, _ := os.ReadDir(filepath.Join(s.Root, "tickets"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("stray tmp file %s", e.Name())
		}
	}
	// No-op mutate makes no commit.
	_ = s.Mutate("noop", func(tx *Tx) error { return nil })
	if git.CommitCount(s.Root) != before+1 {
		t.Error("no-op mutate committed")
	}
}

func TestListRejectsBadFiles(t *testing.T) {
	s := newTestStore(t)
	os.WriteFile(s.PathFor("TST-1"), []byte("not a ticket"), 0o644)
	os.WriteFile(filepath.Join(s.ticketsDir(), "notes.md"), []byte("ignored"), 0o644)
	_, err := s.List()
	if err == nil || !strings.Contains(err.Error(), "TST-1.md") {
		t.Errorf("expected parse error naming TST-1.md, got %v", err)
	}
}
