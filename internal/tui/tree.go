package tui

import (
	"fmt"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/model"
)

type treeRow struct {
	t      *model.Ticket
	prefix string
}

func (a *App) rebuildTree() {
	prev := a.current()
	a.rows = a.rows[:0]
	var roots []*model.Ticket
	if a.scope.Under != "" {
		if t := a.ix.Get(a.scope.Under); t != nil {
			roots = []*model.Ticket{t}
		}
	} else {
		roots = a.ix.Roots()
	}
	for _, r := range roots {
		a.addTreeRows(r, "", "")
	}
	if a.view == viewTree && prev != nil {
		a.syncTreeCursorTo(prev)
	}
	if a.trow >= len(a.rows) {
		a.trow = len(a.rows) - 1
	}
	if a.trow < 0 {
		a.trow = 0
	}
}

func (a *App) addTreeRows(t *model.Ticket, prefix, branch string) {
	// Text filter: keep a node if it or any descendant matches.
	if a.text != "" && !a.subtreeMatches(t) {
		return
	}
	a.rows = append(a.rows, treeRow{t: t, prefix: branch})
	kids := a.ix.Children(t.ID)
	for i, c := range kids {
		last := i == len(kids)-1
		b, next := "├── ", "│   "
		if last {
			b, next = "└── ", "    "
		}
		a.addTreeRows(c, prefix+next, prefix+b)
	}
}

func (a *App) subtreeMatches(t *model.Ticket) bool {
	for _, x := range a.ix.Subtree(t.ID) {
		if a.matchesText(x) {
			return true
		}
	}
	return false
}

func (a *App) syncTreeCursorTo(t *model.Ticket) {
	if t == nil {
		return
	}
	for i, r := range a.rows {
		if r.t.ID == t.ID {
			a.trow = i
			return
		}
	}
}

func (a *App) treeNav(k string) {
	switch k {
	case "j", "down":
		a.trow++
	case "k", "up":
		a.trow--
	case "g":
		a.trow = 0
	case "G":
		a.trow = len(a.rows) - 1
	case "h", "left":
		// jump to parent
		if a.trow < len(a.rows) {
			if p := a.rows[a.trow].t.Parent; p != "" {
				a.syncTreeCursorTo(a.ix.Get(p))
			}
		}
	}
	if a.trow >= len(a.rows) {
		a.trow = len(a.rows) - 1
	}
	if a.trow < 0 {
		a.trow = 0
	}
}

func (a *App) renderTree() string {
	h := a.bodyHeight()
	if len(a.rows) == 0 {
		return a.st.dim.Render("(no tickets)")
	}
	if a.trow < a.treeOff {
		a.treeOff = a.trow
	}
	if a.trow >= a.treeOff+h {
		a.treeOff = a.trow - h + 1
	}
	var sb strings.Builder
	end := min(len(a.rows), a.treeOff+h)
	for i := a.treeOff; i < end; i++ {
		r := a.rows[i]
		t := r.t
		line := a.st.dim.Render(r.prefix) + a.st.id.Render(t.ID)
		if t.Slug != "" {
			line += a.st.dim.Render(" (" + t.Slug + ")")
		}
		line += fmt.Sprintf("  %-7s ", t.Status) + a.st.dim.Render(fmt.Sprintf("P%d", t.Priority))
		if t.Repo != "" {
			line += a.st.dim.Render("  " + t.Repo)
		}
		if t.Assignee != "" {
			line += a.st.assignee.Render("  " + t.Assignee)
		}
		line += "  " + t.Title
		if d, n := a.ix.Progress(t.ID); n > 0 {
			line += a.st.badge.Render(fmt.Sprintf("  %d/%d", d, n))
		}
		if a.ix.IsBlocked(t.ID) {
			line += a.st.blocked.Render("  blocked")
		}
		line = truncate(line, a.width)
		if i == a.trow {
			line = a.st.treeSel.Width(a.width).Render(line)
		}
		sb.WriteString(line)
		if i < end-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
