package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/Commonwealth-Labs/bored/internal/render"
)

func (a *App) openDetail(id string) {
	t := a.ix.Get(id)
	if t == nil {
		return
	}
	a.detailID = id
	w := min(a.width-2, 100)
	body := render.Markdown(t.Body, w)
	a.vp.SetContent(render.Header(a.ix, t) + "\n" + body)
	a.layoutDetail()
}

func (a *App) layoutDetail() {
	a.vp.SetWidth(max(10, a.width))
	a.vp.SetHeight(a.bodyHeight())
}

func (a *App) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "esc", "q", "backspace":
		a.mode = modeNav
		a.syncBoardCursorTo(a.ix.Get(a.detailID))
		a.syncTreeCursorTo(a.ix.Get(a.detailID))
		return a, nil
	case "?":
		a.mode = modeHelp
		return a, nil
	case "r":
		return a, a.load()
	case "n":
		return a.startForm()
	}
	if cmd, handled := a.ticketAction(k, a.ix.Get(a.detailID)); handled {
		return a, cmd
	}
	var cmd tea.Cmd
	a.vp, cmd = a.vp.Update(msg)
	return a, cmd
}

func (a *App) renderDetail() string {
	return a.vp.View()
}
