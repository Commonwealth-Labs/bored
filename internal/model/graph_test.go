package model

import (
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
	if err := ix.CanMove(ix.Get("T-2"), StatusDone, false); err == nil {
		t.Error("done with open children should fail")
	}
	if err := ix.CanMove(ix.Get("T-2"), StatusDone, true); err != nil {
		t.Error("forced done should pass")
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
