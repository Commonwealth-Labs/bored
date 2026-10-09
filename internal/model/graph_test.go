package model

import (
	"strings"
	"testing"
	"time"
)

func mk(id, parent string, st Status, pri int, deps ...string) *Ticket {
	return &Ticket{ID: id, Title: id, Status: st, Parent: parent, Priority: pri, DependsOn: deps,
		Created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(IDNum(id)) * time.Hour)}
}

func fixture() *Index {
	// T-1 root (slug iam)
	//   T-2 chunk (doing)
	//     T-4 leaf todo P2
	//     T-5 leaf todo P1 depends on T-4
	//     T-6 leaf done
	//   T-3 chunk, no children, todo P3
	// T-7 root todo P1 assigned to andrew
	ts := []*Ticket{
		mk("T-1", "", StatusDoing, 3), mk("T-2", "T-1", StatusDoing, 3), mk("T-3", "T-1", StatusTodo, 3),
		mk("T-4", "T-2", StatusTodo, 2), mk("T-5", "T-2", StatusTodo, 1, "T-4"), mk("T-6", "T-2", StatusDone, 3),
		mk("T-7", "", StatusTodo, 1),
	}
	ts[0].Slug = "iam"
	ts[3].Repo = "h1v3"
	ts[6].Assignee = "andrew"
	return NewIndex("T", ts)
}

func ids(ts []*Ticket) string {
	s := ""
	for _, t := range ts {
		s += t.ID + " "
	}
	return s
}

func TestResolve(t *testing.T) {
	ix := fixture()
	for _, ref := range []string{"1", "t-1", "T-1", "iam", "IAM", " 01 "} {
		tk, err := ix.Resolve(ref)
		if err != nil || tk.ID != "T-1" {
			t.Errorf("Resolve(%q) = %v, %v", ref, tk, err)
		}
	}
	if _, err := ix.Resolve("99"); err == nil {
		t.Error("expected not found")
	}
	if _, err := ix.Resolve("nope"); err == nil {
		t.Error("expected not found for slug")
	}
	if _, err := ix.Resolve("X-1"); err == nil {
		t.Error("wrong prefix should not resolve")
	}
}

func TestStructure(t *testing.T) {
	ix := fixture()
	if got := ids(ix.Children("T-2")); got != "T-4 T-5 T-6 " {
		t.Errorf("children = %q", got)
	}
	if !ix.HasOpenChildren("T-1") || !ix.HasOpenChildren("T-2") || ix.HasOpenChildren("T-3") {
		t.Error("open children wrong")
	}
	if ix.Workable("T-2") || !ix.Workable("T-3") || !ix.Workable("T-4") {
		t.Error("workable wrong")
	}
	if d, n := ix.Progress("T-2"); d != 1 || n != 3 {
		t.Errorf("progress = %d/%d", d, n)
	}
	if ix.RootOf("T-5").ID != "T-1" || ix.RootOf("T-7").ID != "T-7" {
		t.Error("root wrong")
	}
	if got := ids(ix.Subtree("T-1")); got != "T-1 T-2 T-4 T-5 T-6 T-3 " {
		t.Errorf("subtree = %q", got)
	}
	if got := ids(ix.Roots()); got != "T-1 T-7 " {
		t.Errorf("roots = %q", got)
	}
	if !ix.InSubtree("T-5", "T-1") || ix.InSubtree("T-7", "T-1") || !ix.InSubtree("T-1", "T-1") {
		t.Error("InSubtree wrong")
	}
}

func TestBlockingAndReady(t *testing.T) {
	ix := fixture()
	if !ix.IsBlocked("T-5") || ix.IsBlocked("T-4") {
		t.Error("blocked wrong")
	}
	if ix.OpenDependents("T-4") != 1 {
		t.Error("open dependents wrong")
	}
	// Ready for claude: T-4 (P2), T-3 (P3). T-5 blocked, T-7 assigned to andrew, T-2 container.
	if got := ids(ix.Ready(Scope{}, "claude")); got != "T-4 T-3 " {
		t.Errorf("ready(claude) = %q", got)
	}
	// Ready for andrew includes T-7 at P1 first.
	if got := ids(ix.Ready(Scope{}, "andrew")); got != "T-7 T-4 T-3 " {
		t.Errorf("ready(andrew) = %q", got)
	}
	// Under iam excludes T-7.
	if got := ids(ix.Ready(Scope{Under: "T-1"}, "andrew")); got != "T-4 T-3 " {
		t.Errorf("ready under iam = %q", got)
	}
	// Repo scope: T-4 names h1v3; T-3 is repo-less under the same root; T-7 is another root.
	if got := ids(ix.Filter(Scope{Repo: "h1v3", Status: StatusTodo})); got != "T-3 T-4 T-5 " {
		t.Errorf("repo scope = %q", got)
	}
}

func TestNext(t *testing.T) {
	ix := fixture()
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	tk, reason := ix.Next(Scope{}, "claude", now)
	if tk == nil || tk.ID != "T-4" || reason == "" {
		t.Errorf("next(claude) = %v %q", tk, reason)
	}
	// Mark T-4 doing by claude: next returns it as in progress.
	ix.Get("T-4").Status = StatusDoing
	ix.Get("T-4").Assignee = "claude"
	tk, reason = ix.Next(Scope{}, "claude", now)
	if tk.ID != "T-4" || reason[:19] != "already in progress" {
		t.Errorf("next with doing = %v %q", tk, reason)
	}
	// Nothing in scope.
	if tk, _ := ix.Next(Scope{Under: "T-6"}, "x", now); tk != nil {
		t.Error("expected nil next")
	}
}

func TestCycles(t *testing.T) {
	ix := fixture()
	if !ix.WouldCycleDep("T-4", "T-5") { // T-5 depends on T-4; making T-4 depend on T-5 cycles
		t.Error("dep cycle not detected")
	}
	if ix.WouldCycleDep("T-3", "T-5") {
		t.Error("false dep cycle")
	}
	if !ix.WouldCycleParent("T-1", "T-5") || !ix.WouldCycleParent("T-2", "T-2") {
		t.Error("parent cycle not detected")
	}
	if ix.WouldCycleParent("T-7", "T-3") {
		t.Error("false parent cycle")
	}
}

func TestTransitions(t *testing.T) {
	ix := fixture()
	if err := ix.CanClaim(ix.Get("T-2"), "claude", false); err == nil {
		t.Error("claiming a container should fail")
	}
	if err := ix.CanClaim(ix.Get("T-5"), "claude", false); err == nil {
		t.Error("claiming a blocked ticket should fail")
	}
	if err := ix.CanClaim(ix.Get("T-5"), "claude", true); err != nil {
		t.Errorf("force claim of blocked should pass: %v", err)
	}
	if err := ix.CanClaim(ix.Get("T-7"), "claude", false); err == nil {
		t.Error("claiming someone else's ticket should fail")
	}
	if err := ix.CanClaim(ix.Get("T-7"), "andrew", false); err != nil {
		t.Errorf("owner claim should pass: %v", err)
	}
	if err := ix.CanMove(ix.Get("T-2"), StatusDone); err == nil {
		t.Error("done with open children should fail")
	}
	if err := ix.CanMove(ix.Get("T-2"), StatusBacklog); err == nil {
		t.Error("any move on a container with open children should fail")
	}
	if err := ix.CanMove(ix.Get("T-4"), StatusReview); err != nil {
		t.Errorf("leaf move should pass: %v", err)
	}
	if err := ix.CanDone(ix.Get("T-4"), false); err == nil {
		t.Error("done from todo without force should fail")
	}
	tk := ix.Get("T-4")
	Claim(tk, "claude", time.Now())
	if tk.Status != StatusDoing || tk.Assignee != "claude" || tk.ClaimedAt == nil {
		t.Error("claim not applied")
	}
	ApplyMove(tk, StatusTodo)
	if tk.Assignee != "" || tk.ClaimedAt != nil {
		t.Error("move to todo should clear claim")
	}
}

func TestDerivedStatus(t *testing.T) {
	ix := fixture()
	// T-2 has children todo, todo, done: started -> doing. T-1 follows T-2 -> doing.
	if got := ix.Status("T-2"); got != StatusDoing {
		t.Errorf("T-2 effective = %s, want doing", got)
	}
	if got := ix.Status("T-1"); got != StatusDoing {
		t.Errorf("T-1 effective = %s, want doing", got)
	}
	// Leaves report their stored status.
	if got := ix.Status("T-4"); got != StatusTodo {
		t.Errorf("T-4 effective = %s", got)
	}
	// A container whose children are all backlog is backlog; once one is todo it's todo.
	ts := []*Ticket{mk("R-1", "", StatusDoing, 3), mk("R-2", "R-1", StatusBacklog, 3), mk("R-3", "R-1", StatusBacklog, 3)}
	ix2 := NewIndex("R", ts)
	if got := ix2.Status("R-1"); got != StatusBacklog {
		t.Errorf("all-backlog children: %s, want backlog (stored doing ignored)", got)
	}
	ts[1].Status = StatusTodo
	if got := ix2.Status("R-1"); got != StatusTodo {
		t.Errorf("one ready child: %s, want todo", got)
	}
	ts[1].Status = StatusDone
	ts[2].Status = StatusDone
	if got := ix2.Status("R-1"); got != StatusDoing {
		t.Errorf("all children done: stored status (%s) should show through, got %s", ts[0].Status, got)
	}
	// Filter and Ready use the effective status.
	if got := ids(ix.Filter(Scope{Status: StatusDoing})); got != "T-1 T-2 " {
		t.Errorf("filter doing = %q", got)
	}
}

func TestCascadeDone(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	// grouping chain: G-1 (no AC) > G-2 (no AC) > G-3 leaf, plus W-4 under G-1 with its own AC
	ts := []*Ticket{
		mk("G-1", "", StatusBacklog, 3), mk("G-2", "G-1", StatusBacklog, 3), mk("G-3", "G-2", StatusReview, 2),
		mk("G-4", "G-1", StatusBacklog, 3), mk("G-5", "G-4", StatusReview, 2),
	}
	ts[3].Body = NewBody("integrate", []string{"integration verified"})
	ix := NewIndex("G", ts)
	// Finish G-3: G-2 has no AC -> auto done; G-1 still has G-4 open -> stops.
	ApplyMove(ix.Get("G-3"), StatusDone)
	changed := CascadeDone(ix, ix.Get("G-3"), "tester", now)
	if len(changed) != 1 || changed[0].ID != "G-2" || ix.Get("G-2").Status != StatusDone {
		t.Fatalf("expected G-2 auto-done, changed=%v", ids(changed))
	}
	if ix.Get("G-1").Status != StatusBacklog {
		t.Error("G-1 should be untouched while G-4 is open")
	}
	if !strings.Contains(ix.Get("G-2").Body, "all children done") {
		t.Error("log line missing on G-2")
	}
	// Finish G-5: G-4 has AC -> becomes todo, not done; G-1 therefore still open.
	ApplyMove(ix.Get("G-5"), StatusDone)
	changed = CascadeDone(ix, ix.Get("G-5"), "tester", now)
	if len(changed) != 1 || changed[0].ID != "G-4" || ix.Get("G-4").Status != StatusTodo {
		t.Fatalf("expected G-4 -> todo, changed=%v status=%s", ids(changed), ix.Get("G-4").Status)
	}
	if ix.Status("G-1") != StatusDoing {
		t.Errorf("G-1 effective should be doing (G-2 done, G-4 ready), got %s", ix.Status("G-1"))
	}
	// Now G-4 is a workable todo with its own AC; finishing it closes G-1.
	if !ix.Workable("G-4") {
		t.Fatal("G-4 should be workable")
	}
	ApplyMove(ix.Get("G-4"), StatusDone)
	changed = CascadeDone(ix, ix.Get("G-4"), "tester", now)
	if len(changed) != 1 || changed[0].ID != "G-1" || ix.Get("G-1").Status != StatusDone {
		t.Fatalf("expected G-1 auto-done, changed=%v", ids(changed))
	}
}
