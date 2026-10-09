package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/render"
)

func (a *App) rebuildColumns() {
	prev := a.current()
	opts := render.BoardOpts{Now: time.Now(), DoneWindow: time.Duration(a.store.Cfg.DoneWindowDays) * 24 * time.Hour}
	cols := render.Columns(a.ix, a.scope, opts)
	for i, st := range model.AllStatuses {
		a.cols[i] = a.cols[i][:0]
		for _, t := range cols[st] {
			if a.matchesText(t) {
				a.cols[i] = append(a.cols[i], t)
			}
		}
	}
	// After [ or ] the cursor travels with the ticket to its new column.
	if a.followID != "" {
		if t := a.ix.Get(a.followID); t != nil {
			a.syncBoardCursorTo(t)
		}
		a.followID = ""
		a.clampBoard()
		return
	}
	// Otherwise stay in the current column. If the previously selected ticket
	// is still here, keep it under the cursor; if it left (claimed, done), the
	// row index now points at the card that was below it, clamped to the end.
	if a.view == viewBoard && prev != nil {
		for ri, t := range a.cols[a.col] {
			if t.ID == prev.ID {
				a.row = ri
				break
			}
		}
	}
	a.clampBoard()
}

func (a *App) clampBoard() {
	if a.col < 0 {
		a.col = 0
	}
	if a.col >= len(a.cols) {
		a.col = len(a.cols) - 1
	}
	n := len(a.cols[a.col])
	if a.row >= n {
		a.row = n - 1
	}
	if a.row < 0 {
		a.row = 0
	}
}

// syncBoardCursorTo points the cursor at t if it is on the board.
func (a *App) syncBoardCursorTo(t *model.Ticket) {
	if t == nil {
		return
	}
	for ci, col := range a.cols {
		for ri, x := range col {
			if x.ID == t.ID {
				a.col, a.row = ci, ri
				return
			}
		}
	}
}

func (a *App) boardNav(k string) {
	switch k {
	case "j", "down":
		a.row++
	case "k", "up":
		a.row--
	case "h", "left":
		a.col--
		a.row = min(a.row, max(0, len(a.cols[max(0, a.col)])-1))
	case "l", "right":
		a.col++
		if a.col < len(a.cols) {
			a.row = min(a.row, max(0, len(a.cols[a.col])-1))
		}
	case "g":
		a.row = 0
	case "G":
		a.row = len(a.cols[a.col]) - 1
	}
	a.clampBoard()
}

func (a *App) renderBoard() string {
	n := len(model.AllStatuses)
	gap := 2
	colW := (a.width - gap*(n-1)) / n
	if colW < 12 {
		colW = 12
	}
	h := a.bodyHeight()
	avail := h - 1 // minus header
	var rendered []string
	for ci, st := range model.AllStatuses {
		cards := a.cols[ci]
		head := fmt.Sprintf("%s %d", strings.ToUpper(string(st)), len(cards))
		if n := render.StuckCount(cards); n > 0 {
			head += fmt.Sprintf(" (%d stuck)", n)
		}
		if ci == a.col {
			head = a.st.colHeadSel.Render(head)
		} else {
			head = a.st.colHead.Render(head)
		}
		lines := []string{truncate(head, colW)}
		off := a.colOff[ci]
		if ci == a.col {
			if a.row < off {
				off = a.row
			}
			if a.row >= off+avail {
				off = a.row - avail + 1
			}
		}
		if off > max(0, len(cards)-avail) {
			off = max(0, len(cards)-avail)
		}
		if off < 0 {
			off = 0
		}
		a.colOff[ci] = off
		for i := off; i < len(cards) && i < off+avail; i++ {
			if i == off+avail-1 && len(cards) > off+avail {
				lines = append(lines, a.st.dim.Render(fmt.Sprintf("… %d more", len(cards)-i)))
				break
			}
			lines = append(lines, a.renderCard(cards[i], colW, ci == a.col && i == a.row))
		}
		if len(cards) == 0 {
			lines = append(lines, a.st.dim.Render("—"))
		}
		col := lipgloss.NewStyle().Width(colW).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n"))
		rendered = append(rendered, col)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, joinWithGap(rendered, gap)...)
}

func joinWithGap(cols []string, gap int) []string {
	out := make([]string, 0, len(cols)*2)
	for i, c := range cols {
		if i > 0 {
			out = append(out, strings.Repeat(" ", gap))
		}
		out = append(out, c)
	}
	return out
}
