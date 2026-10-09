package cli

import (
	"time"

	"github.com/Commonwealth-Labs/bored/internal/model"
)

type counts struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// ticketView is the JSON shape for a ticket: stored fields plus derived ones.
type ticketView struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Parent    string     `json:"parent,omitempty"`
	Slug      string     `json:"slug,omitempty"`
	Repo      string     `json:"repo,omitempty"`
	Priority  int        `json:"priority"`
	DependsOn []string   `json:"depends_on"`
	Labels    []string   `json:"labels"`
	Assignee  string     `json:"assignee"`
	Created   time.Time  `json:"created"`
	Updated   time.Time  `json:"updated"`
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`
	Branch    string     `json:"branch,omitempty"`

	Root      string   `json:"root"`
	Blocked   bool     `json:"blocked"`
	BlockedBy []string `json:"blocked_by"`
	Blocks    []string `json:"blocks"`
	Workable  bool     `json:"workable"`
	Children  []string `json:"children"`
	Progress  counts   `json:"progress"`
	AC        counts   `json:"ac"`
	Path      string   `json:"path"`
	Body      string   `json:"body,omitempty"`
}

func view(ix *model.Index, t *model.Ticket, full bool) ticketView {
	v := ticketView{
		ID: t.ID, Title: t.Title, Status: string(ix.Status(t.ID)), Parent: t.Parent, Slug: t.Slug, Repo: t.Repo,
		Priority: t.Priority, DependsOn: t.DependsOn, Labels: t.Labels, Assignee: t.Assignee,
		Created: t.Created, Updated: t.Updated, ClaimedAt: t.ClaimedAt, Branch: t.Branch, Path: t.Path,
		BlockedBy: []string{}, Blocks: []string{}, Children: []string{},
	}
	if v.DependsOn == nil {
		v.DependsOn = []string{}
	}
	if v.Labels == nil {
		v.Labels = []string{}
	}
	if r := ix.RootOf(t.ID); r != nil {
		v.Root = r.ID
	}
	for _, b := range ix.BlockedBy(t.ID) {
		v.BlockedBy = append(v.BlockedBy, b.ID)
	}
	v.Blocked = len(v.BlockedBy) > 0
	for _, d := range ix.Dependents(t.ID) {
		v.Blocks = append(v.Blocks, d.ID)
	}
	v.Workable = ix.Workable(t.ID)
	for _, c := range ix.Children(t.ID) {
		v.Children = append(v.Children, c.ID)
	}
	v.Progress.Done, v.Progress.Total = ix.Progress(t.ID)
	v.AC.Done, v.AC.Total = model.CountAC(t.Body)
	if full {
		v.Body = t.Body
	}
	return v
}

func views(ix *model.Index, ts []*model.Ticket, full bool) []ticketView {
	out := make([]ticketView, 0, len(ts))
	for _, t := range ts {
		out = append(out, view(ix, t, full))
	}
	return out
}

type repoView struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Exists bool   `json:"exists"`
}
