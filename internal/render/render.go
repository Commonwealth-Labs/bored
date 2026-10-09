// Package render produces the plain-text views shared by the CLI and prime:
// cards, the stacked board, the tree, the list table, and markdown bodies.
package render

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

// IsTTY reports whether f is a terminal.
func IsTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// Markdown renders md for the terminal, falling back to the raw text.
func Markdown(md string, width int) string {
	opts := []glamour.TermRendererOption{glamour.WithWordWrap(width)}
	if os.Getenv("GLAMOUR_STYLE") != "" {
		opts = append(opts, glamour.WithEnvironmentConfig())
	} else if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		opts = append(opts, glamour.WithStandardStyle("dark"))
	} else {
		opts = append(opts, glamour.WithStandardStyle("light"))
	}
	r, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}

// Badges returns the short derived markers for a ticket: AC count, blocked,
// children progress.
func Badges(ix *model.Index, t *model.Ticket) []string {
	var b []string
	if d, n := store.CountAC(t.Body); n > 0 {
		b = append(b, fmt.Sprintf("ac %d/%d", d, n))
	}
	if d, n := ix.Progress(t.ID); n > 0 {
		b = append(b, fmt.Sprintf("children %d/%d", d, n))
	}
	if bl := ix.BlockedBy(t.ID); len(bl) > 0 {
		ids := make([]string, len(bl))
		for i, x := range bl {
			ids[i] = x.ID
		}
		b = append(b, "blocked by "+strings.Join(ids, ","))
	}
	return b
}

// Where renders "root › repo" for a card.
func Where(ix *model.Index, t *model.Ticket) string {
	root := ix.RootOf(t.ID)
	s := ""
	if root != nil && root.ID != t.ID {
		s = root.Ref()
	}
	if t.Repo != "" && t.Repo != s {
		if s != "" {
			s += "›"
		}
		s += t.Repo
	}
	return s
}

// Card is a one-line summary used by board and prime.
func Card(ix *model.Index, t *model.Ticket, mark string) string {
	var sb strings.Builder
	sb.WriteString(t.ID + mark)
	sb.WriteString(fmt.Sprintf("  P%d", t.Priority))
	if w := Where(ix, t); w != "" {
		sb.WriteString("  " + w)
	}
	if t.Assignee != "" {
		sb.WriteString("  (" + t.Assignee + ")")
	}
	sb.WriteString("  " + t.Title)
	if b := Badges(ix, t); len(b) > 0 {
		sb.WriteString("  [" + strings.Join(b, "; ") + "]")
	}
	return sb.String()
}

// BoardOpts controls which tickets appear.
type BoardOpts struct {
	AllLevels  bool          // include tickets with open children
	DoneWindow time.Duration // hide done tickets older than this (0 = show all)
	Now        time.Time
	MarkRepo   string // append * to tickets naming this repo
}

// Columns groups in-scope tickets by status, sorted like next.
func Columns(ix *model.Index, sc model.Scope, o BoardOpts) map[model.Status][]*model.Ticket {
	cols := map[model.Status][]*model.Ticket{}
	for _, t := range ix.Filter(sc) {
		if !o.AllLevels && ix.HasOpenChildren(t.ID) {
			continue
		}
		if t.Status == model.StatusDone && o.DoneWindow > 0 && o.Now.Sub(t.Updated) > o.DoneWindow {
			continue
		}
		cols[t.Status] = append(cols[t.Status], t)
	}
	for st, ts := range cols {
		sort.SliceStable(ts, func(i, j int) bool {
			a, b := ts[i], ts[j]
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			return model.IDNum(a.ID) < model.IDNum(b.ID)
		})
		cols[st] = ts
	}
	return cols
}

// Board renders the stacked column view.
func Board(ix *model.Index, sc model.Scope, o BoardOpts) string {
	cols := Columns(ix, sc, o)
	var sb strings.Builder
	for _, st := range model.AllStatuses {
		ts := cols[st]
		sb.WriteString(fmt.Sprintf("%s (%d)\n", strings.ToUpper(string(st)), len(ts)))
		for _, t := range ts {
			mark := ""
			if o.MarkRepo != "" && t.Repo == o.MarkRepo {
				mark = "*"
			}
			sb.WriteString("  " + Card(ix, t, mark) + "\n")
		}
	}
	return sb.String()
}

// Tree renders roots and their subtrees with box-drawing guides.
func Tree(ix *model.Index, roots []*model.Ticket) string {
	var sb strings.Builder
	for _, r := range roots {
		writeNode(&sb, ix, r, "", "", true)
	}
	return sb.String()
}

func writeNode(sb *strings.Builder, ix *model.Index, t *model.Ticket, prefix, branch string, root bool) {
	line := branch + t.ID
	if t.Slug != "" {
		line += " (" + t.Slug + ")"
	}
	line += fmt.Sprintf("  %-7s P%d", t.Status, t.Priority)
	if t.Repo != "" {
		line += "  " + t.Repo
	}
	if t.Assignee != "" {
		line += "  (" + t.Assignee + ")"
	}
	line += "  " + t.Title
	if d, n := ix.Progress(t.ID); n > 0 {
		line += fmt.Sprintf("  %d/%d", d, n)
	}
	if ix.IsBlocked(t.ID) {
		line += "  [blocked]"
	}
	sb.WriteString(line + "\n")
	kids := ix.Children(t.ID)
	for i, c := range kids {
		last := i == len(kids)-1
		b, next := "├── ", "│   "
		if last {
			b, next = "└── ", "    "
		}
		writeNode(sb, ix, c, prefix+next, prefix+b, false)
	}
}

// Table renders a list.
func Table(ix *model.Index, ts []*model.Ticket) string {
	if len(ts) == 0 {
		return "(no tickets)\n"
	}
	rows := make([][]string, 0, len(ts)+1)
	rows = append(rows, []string{"ID", "STATUS", "P", "WHERE", "ASSIGNEE", "TITLE", "NOTES"})
	for _, t := range ts {
		rows = append(rows, []string{t.ID, string(t.Status), fmt.Sprint(t.Priority), Where(ix, t), t.Assignee, t.Title, strings.Join(Badges(ix, t), "; ")})
	}
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths)-1 && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	var sb strings.Builder
	for _, r := range rows {
		for i, c := range r {
			if i == len(r)-1 {
				sb.WriteString(c)
			} else {
				sb.WriteString(fmt.Sprintf("%-*s  ", widths[i], c))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// Header renders the metadata block shown above a ticket body.
func Header(ix *model.Index, t *model.Ticket) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s  %s\n", t.ID, t.Title))
	sb.WriteString(fmt.Sprintf("status: %s   priority: P%d", t.Status, t.Priority))
	if t.Slug != "" {
		sb.WriteString("   slug: " + t.Slug)
	}
	if t.Repo != "" {
		sb.WriteString("   repo: " + t.Repo)
	}
	if t.Assignee != "" {
		sb.WriteString("   assignee: " + t.Assignee)
	}
	if t.Branch != "" {
		sb.WriteString("   branch: " + t.Branch)
	}
	sb.WriteString("\n")
	if len(t.Labels) > 0 {
		sb.WriteString("labels: " + strings.Join(t.Labels, ", ") + "\n")
	}
	if anc := ix.Ancestors(t.ID); len(anc) > 0 {
		parts := make([]string, len(anc))
		for i, a := range anc {
			parts[i] = a.ID + " " + a.Title
		}
		sb.WriteString("under: " + strings.Join(parts, " › ") + "\n")
	}
	if kids := ix.Children(t.ID); len(kids) > 0 {
		d, n := ix.Progress(t.ID)
		sb.WriteString(fmt.Sprintf("children (%d/%d done):\n", d, n))
		for _, k := range kids {
			sb.WriteString(fmt.Sprintf("  %s  %-7s %s\n", k.ID, k.Status, k.Title))
		}
	}
	if bl := ix.BlockedBy(t.ID); len(bl) > 0 {
		parts := make([]string, len(bl))
		for i, b := range bl {
			parts[i] = fmt.Sprintf("%s (%s)", b.ID, b.Status)
		}
		sb.WriteString("blocked by: " + strings.Join(parts, ", ") + "\n")
	}
	if deps := ix.Dependents(t.ID); len(deps) > 0 {
		parts := make([]string, len(deps))
		for i, d := range deps {
			parts[i] = d.ID
		}
		sb.WriteString("blocks: " + strings.Join(parts, ", ") + "\n")
	}
	sb.WriteString(fmt.Sprintf("created: %s   updated: %s\n", t.Created.Local().Format("2006-01-02 15:04"), t.Updated.Local().Format("2006-01-02 15:04")))
	return sb.String()
}
