package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Index is an in-memory view of all tickets with the derived relationships.
// Build one from a full List(); it is cheap and the store is small.
type Index struct {
	prefix     string
	tickets    []*Ticket
	byID       map[string]*Ticket
	bySlug     map[string]*Ticket
	children   map[string][]*Ticket
	dependents map[string][]*Ticket
}

// NewIndex builds the index. Tickets are sorted by numeric ID.
func NewIndex(prefix string, tickets []*Ticket) *Index {
	ix := &Index{
		prefix:     prefix,
		tickets:    append([]*Ticket(nil), tickets...),
		byID:       map[string]*Ticket{},
		bySlug:     map[string]*Ticket{},
		children:   map[string][]*Ticket{},
		dependents: map[string][]*Ticket{},
	}
	sort.Slice(ix.tickets, func(i, j int) bool { return IDNum(ix.tickets[i].ID) < IDNum(ix.tickets[j].ID) })
	for _, t := range ix.tickets {
		ix.byID[t.ID] = t
		if t.Slug != "" {
			ix.bySlug[strings.ToLower(t.Slug)] = t
		}
	}
	for _, t := range ix.tickets {
		if t.Parent != "" {
			ix.children[t.Parent] = append(ix.children[t.Parent], t)
		}
		for _, d := range t.DependsOn {
			ix.dependents[d] = append(ix.dependents[d], t)
		}
	}
	return ix
}

// IDNum extracts the numeric part of an ID like BRD-12. Unknown shapes sort last.
func IDNum(id string) int {
	i := strings.LastIndexByte(id, '-')
	n := 0
	for _, c := range id[i+1:] {
		if c < '0' || c > '9' {
			return 1 << 30
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// All returns every ticket in ID order.
func (ix *Index) All() []*Ticket { return ix.tickets }

// Get returns a ticket by exact ID, or nil.
func (ix *Index) Get(id string) *Ticket { return ix.byID[id] }

// Resolve accepts an ID in any accepted spelling, or a slug.
func (ix *Index) Resolve(ref string) (*Ticket, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("%w: empty ticket reference", ErrUsage)
	}
	if id, ok := NormalizeID(ix.prefix, ref); ok {
		if t := ix.byID[id]; t != nil {
			return t, nil
		}
		return nil, fmt.Errorf("%w: no ticket %s", ErrNotFound, id)
	}
	if t := ix.bySlug[strings.ToLower(ref)]; t != nil {
		return t, nil
	}
	return nil, fmt.Errorf("%w: no ticket with id or slug %q", ErrNotFound, ref)
}

// Children returns direct children in ID order.
func (ix *Index) Children(id string) []*Ticket { return ix.children[id] }

// HasOpenChildren reports whether any direct child is not done, judged by
// effective status so a grandchild in progress counts.
func (ix *Index) HasOpenChildren(id string) bool {
	for _, c := range ix.children[id] {
		if ix.Status(c.ID) != StatusDone {
			return true
		}
	}
	return false
}

// Workable: a ticket with no open children. Containers become workable again
// when their last child is done.
func (ix *Index) Workable(id string) bool { return !ix.HasOpenChildren(id) }

// Status is the effective status. For a ticket with open children it is
// derived from them and the stored value is ignored: backlog while every
// child is in backlog, todo once any child is ready, doing once any child has
// started or finished. Otherwise it is the stored status.
func (ix *Index) Status(id string) Status {
	return ix.statusOf(id, map[string]bool{})
}

func (ix *Index) statusOf(id string, seen map[string]bool) Status {
	t := ix.byID[id]
	if t == nil {
		return StatusBacklog
	}
	if seen[id] {
		return t.Status
	}
	seen[id] = true
	kids := ix.children[id]
	if len(kids) == 0 {
		return t.Status
	}
	open, started, ready := false, false, false
	for _, c := range kids {
		switch ix.statusOf(c.ID, seen) {
		case StatusDone:
			started = true
		case StatusDoing, StatusReview:
			open, started = true, true
		case StatusTodo:
			open, ready = true, true
		default:
			open = true
		}
	}
	if !open {
		return t.Status
	}
	switch {
	case started:
		return StatusDoing
	case ready:
		return StatusTodo
	default:
		return StatusBacklog
	}
}

// HasOwnWork reports whether a ticket carries acceptance criteria of its own,
// which is what makes a container more than a grouping.
func HasOwnWork(t *Ticket) bool {
	_, n := CountAC(t.Body)
	return n > 0
}

// Progress counts direct children done/total.
func (ix *Index) Progress(id string) (done, total int) {
	for _, c := range ix.children[id] {
		total++
		if ix.Status(c.ID) == StatusDone {
			done++
		}
	}
	return done, total
}

// Ancestors returns parent, grandparent, ... up to the root. Broken parent
// links (pointing at a missing ticket) end the chain.
func (ix *Index) Ancestors(id string) []*Ticket {
	var out []*Ticket
	seen := map[string]bool{id: true}
	t := ix.byID[id]
	for t != nil && t.Parent != "" && !seen[t.Parent] {
		p := ix.byID[t.Parent]
		if p == nil {
			break
		}
		seen[p.ID] = true
		out = append(out, p)
		t = p
	}
	return out
}

// RootOf returns the topmost ancestor, or the ticket itself.
func (ix *Index) RootOf(id string) *Ticket {
	anc := ix.Ancestors(id)
	if len(anc) == 0 {
		return ix.byID[id]
	}
	return anc[len(anc)-1]
}

// Roots returns tickets without a parent (or whose parent is missing).
func (ix *Index) Roots() []*Ticket {
	var out []*Ticket
	for _, t := range ix.tickets {
		if t.Parent == "" || ix.byID[t.Parent] == nil {
			out = append(out, t)
		}
	}
	return out
}

// Subtree returns id and all descendants, depth-first in ID order.
func (ix *Index) Subtree(id string) []*Ticket {
	root := ix.byID[id]
	if root == nil {
		return nil
	}
	var out []*Ticket
	seen := map[string]bool{}
	var walk func(t *Ticket)
	walk = func(t *Ticket) {
		if seen[t.ID] {
			return
		}
		seen[t.ID] = true
		out = append(out, t)
		for _, c := range ix.children[t.ID] {
			walk(c)
		}
	}
	walk(root)
	return out
}

// InSubtree reports whether id is under (or is) ancestor.
func (ix *Index) InSubtree(id, ancestor string) bool {
	if id == ancestor {
		return true
	}
	for _, a := range ix.Ancestors(id) {
		if a.ID == ancestor {
			return true
		}
	}
	return false
}

// BlockedBy returns blockers that are not done. Missing blockers count as
// blocking so a dangling reference is visible rather than silently ignored.
func (ix *Index) BlockedBy(id string) []*Ticket {
	t := ix.byID[id]
	if t == nil {
		return nil
	}
	var out []*Ticket
	for _, d := range t.DependsOn {
		b := ix.byID[d]
		if b == nil {
			out = append(out, &Ticket{ID: d, Title: "(missing)", Status: StatusBacklog})
			continue
		}
		if ix.Status(b.ID) != StatusDone {
			out = append(out, b)
		}
	}
	return out
}

// IsBlocked reports whether any blocker is open.
func (ix *Index) IsBlocked(id string) bool { return len(ix.BlockedBy(id)) > 0 }

// Dependents returns tickets that depend on id.
func (ix *Index) Dependents(id string) []*Ticket { return ix.dependents[id] }

// OpenDependents counts dependents that are not done: what finishing id would unblock.
func (ix *Index) OpenDependents(id string) int {
	n := 0
	for _, d := range ix.dependents[id] {
		if ix.Status(d.ID) != StatusDone {
			n++
		}
	}
	return n
}

// WouldCycleDep reports whether making id depend on blocker creates a cycle,
// i.e. blocker (transitively) already depends on id.
func (ix *Index) WouldCycleDep(id, blocker string) bool {
	if id == blocker {
		return true
	}
	seen := map[string]bool{}
	var reaches func(from string) bool
	reaches = func(from string) bool {
		if from == id {
			return true
		}
		if seen[from] {
			return false
		}
		seen[from] = true
		if t := ix.byID[from]; t != nil {
			for _, d := range t.DependsOn {
				if reaches(d) {
					return true
				}
			}
		}
		return false
	}
	return reaches(blocker)
}

// WouldCycleParent reports whether setting id's parent to parent creates a
// cycle, i.e. parent is id or is inside id's subtree.
func (ix *Index) WouldCycleParent(id, parent string) bool {
	if parent == "" {
		return false
	}
	return ix.InSubtree(parent, id)
}

// Scope narrows a listing. Zero values mean "no filter".
type Scope struct {
	Under    string // ticket ID (already resolved) whose subtree to include
	Repo     string // repo name; also includes repo-less tickets under the same roots
	Labels   []string
	Status   Status
	Assignee string
}

// InScope applies the scope to one ticket.
func (ix *Index) InScope(t *Ticket, sc Scope) bool {
	if sc.Under != "" && !ix.InSubtree(t.ID, sc.Under) {
		return false
	}
	if sc.Status != "" && ix.Status(t.ID) != sc.Status {
		return false
	}
	if sc.Assignee != "" && t.Assignee != sc.Assignee {
		return false
	}
	for _, l := range sc.Labels {
		if !t.HasLabel(l) {
			return false
		}
	}
	if sc.Repo != "" {
		if t.Repo == sc.Repo {
			return true
		}
		if t.Repo != "" {
			return false
		}
		root := ix.RootOf(t.ID)
		for _, r := range ix.repoRoots(sc.Repo) {
			if r.ID == root.ID {
				return true
			}
		}
		return false
	}
	return true
}

// repoRoots returns the roots of every ticket that names repo.
func (ix *Index) repoRoots(repo string) []*Ticket {
	seen := map[string]bool{}
	var out []*Ticket
	for _, t := range ix.tickets {
		if t.Repo != repo {
			continue
		}
		r := ix.RootOf(t.ID)
		if !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}

// Filter returns tickets in scope, in ID order.
func (ix *Index) Filter(sc Scope) []*Ticket {
	var out []*Ticket
	for _, t := range ix.tickets {
		if ix.InScope(t, sc) {
			out = append(out, t)
		}
	}
	return out
}

// Ready returns todo tickets that are workable, unblocked, and either
// unassigned or assigned to actor, sorted the way Next picks.
func (ix *Index) Ready(sc Scope, actor string) []*Ticket {
	var out []*Ticket
	for _, t := range ix.tickets {
		if ix.Status(t.ID) != StatusTodo || !ix.InScope(t, sc) {
			continue
		}
		if !ix.Workable(t.ID) || ix.IsBlocked(t.ID) {
			continue
		}
		if t.Assignee != "" && t.Assignee != actor {
			continue
		}
		out = append(out, t)
	}
	ix.sortForNext(out)
	return out
}

func (ix *Index) sortForNext(ts []*Ticket) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		da, db := ix.OpenDependents(a.ID), ix.OpenDependents(b.ID)
		if da != db {
			return da > db
		}
		return a.Created.Before(b.Created)
	})
}

// Next picks the single ticket actor should work on, with a one-line reason.
// If actor already has something in doing, that wins.
func (ix *Index) Next(sc Scope, actor string, now time.Time) (*Ticket, string) {
	if actor != "" {
		for _, t := range ix.tickets {
			if t.Status == StatusDoing && ix.Workable(t.ID) && t.Assignee == actor && ix.InScope(t, sc) {
				since := ""
				if t.ClaimedAt != nil {
					since = " since " + humanAge(now.Sub(*t.ClaimedAt)) + " ago"
				}
				return t, "already in progress" + since
			}
		}
	}
	ready := ix.Ready(sc, actor)
	if len(ready) == 0 {
		return nil, ""
	}
	t := ready[0]
	parts := []string{fmt.Sprintf("P%d", t.Priority), "unblocked"}
	if n := ix.OpenDependents(t.ID); n > 0 {
		parts = append(parts, fmt.Sprintf("unblocks %d", n))
	}
	parts = append(parts, "in todo "+humanAge(now.Sub(t.Updated)))
	if sc.Repo != "" && t.Repo == sc.Repo {
		parts = append(parts, "this repo")
	}
	if t.Assignee == actor && actor != "" {
		parts = append(parts, "assigned to you")
	}
	return t, strings.Join(parts, ", ")
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "moments"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
