// Package tui is the interactive board: a Bubble Tea v2 program over the
// same store and render helpers the CLI uses. All mutations go through
// store.Mutate and the board is always rebuilt from disk afterwards.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/Commonwealth-Labs/bored/internal/edit"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/render"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

type viewKind int

const (
	viewBoard viewKind = iota
	viewTree
)

type mode int

const (
	modeNav mode = iota
	modeDetail
	modeForm
	modePrompt
	modeConfirm
	modeHelp
)

const pollEvery = 3 * time.Second

// App is the Bubble Tea model.
type App struct {
	store   *store.Store
	actor   string
	st      styles
	dark    bool
	width   int
	height  int
	tickets []*model.Ticket
	ix      *model.Index
	lastMod time.Time

	view  viewKind
	mode  mode
	scope model.Scope
	roots []*model.Ticket
	root  int // index into roots for the p filter, -1 = all
	text  string

	// board
	cols   [5][]*model.Ticket
	col    int
	row    int
	colOff [5]int

	// tree
	rows    []treeRow
	trow    int
	treeOff int

	// detail
	detailID string
	vp       viewport.Model

	// form
	form  *huh.Form
	draft draft

	// prompt / confirm
	input      textinput.Model
	promptKind string
	promptID   string
	confirmMsg string
	confirmCmd tea.Cmd

	status   string
	statErr  bool
	prevMode mode
}

// Run starts the program.
func Run(s *store.Store, actor string) error {
	a := newApp(s, actor)
	_, err := tea.NewProgram(a).Run()
	return err
}

func newApp(s *store.Store, actor string) *App {
	a := &App{store: s, actor: actor, dark: true, root: -1, width: 80, height: 24}
	a.st = newStyles(true)
	a.input = textinput.New()
	a.vp = viewport.New()
	return a
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(a.load(), func() tea.Msg { return tea.RequestBackgroundColor() }, a.tick())
}

// load reads the store and builds the index.
func (a *App) load() tea.Cmd {
	s := a.store
	return func() tea.Msg {
		ts, err := s.List()
		if err != nil && ts == nil {
			return ticketsLoadedMsg{err: err}
		}
		return ticketsLoadedMsg{tickets: ts, ix: model.NewIndex(s.Cfg.IDPrefix, ts), modTime: newestMod(s), err: err}
	}
}

func newestMod(s *store.Store) time.Time {
	dir := filepath.Join(s.Root, "tickets")
	var newest time.Time
	if st, err := os.Stat(dir); err == nil {
		newest = st.ModTime()
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

func (a *App) tick() tea.Cmd {
	return tea.Tick(pollEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// mutate runs fn under the store lock off the UI goroutine.
func (a *App) mutate(what string, fn func(tx *store.Tx) error) tea.Cmd {
	s := a.store
	return func() tea.Msg {
		return mutationDoneMsg{what: what, err: s.Mutate(what, fn)}
	}
}

func (a *App) setStatus(msg string, isErr bool) {
	a.status = msg
	a.statErr = isErr
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.layoutDetail()
		if a.form != nil {
			a.form = a.form.WithWidth(min(a.width-4, 90))
		}
		return a, nil
	case tea.BackgroundColorMsg:
		a.dark = msg.IsDark()
		a.st = newStyles(a.dark)
		if a.mode == modeDetail {
			a.openDetail(a.detailID)
		}
		return a, nil
	case ticketsLoadedMsg:
		if msg.err != nil && msg.ix == nil {
			a.setStatus("load: "+msg.err.Error(), true)
			return a, nil
		}
		a.tickets, a.ix, a.lastMod = msg.tickets, msg.ix, msg.modTime
		if msg.err != nil {
			a.setStatus(msg.err.Error(), true)
		}
		a.rebuild()
		if a.mode == modeDetail {
			if a.ix.Get(a.detailID) == nil {
				a.mode = modeNav
			} else {
				a.openDetail(a.detailID)
			}
		}
		return a, nil
	case mutationDoneMsg:
		if msg.err != nil {
			a.setStatus(msg.err.Error(), true)
			return a, nil
		}
		a.setStatus(strings.TrimPrefix(msg.what, "bored: "), false)
		return a, a.load()
	case tickMsg:
		if a.mode == modeForm || a.mode == modePrompt {
			return a, a.tick()
		}
		if newestMod(a.store).After(a.lastMod) {
			return a, tea.Batch(a.load(), a.tick())
		}
		return a, a.tick()
	case editDoneMsg:
		defer msg.sess.Cleanup()
		if msg.err != nil {
			a.setStatus("editor: "+msg.err.Error(), true)
			return a, a.load()
		}
		changed, err := edit.Finish(a.store, msg.sess)
		switch {
		case err != nil:
			a.setStatus(err.Error(), true)
		case changed:
			a.setStatus("edited "+msg.sess.ID, false)
		default:
			a.setStatus("no changes", false)
		}
		return a, a.load()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		switch a.mode {
		case modeForm:
			return a.updateForm(msg)
		case modePrompt:
			return a.updatePrompt(msg)
		case modeConfirm:
			return a.updateConfirm(msg)
		case modeHelp:
			a.mode = modeNav
			return a, nil
		case modeDetail:
			return a.updateDetail(msg)
		default:
			return a.updateNav(msg)
		}
	}
	if a.mode == modeForm && a.form != nil {
		m, cmd := a.form.Update(msg)
		if f, ok := m.(*huh.Form); ok {
			a.form = f
		}
		return a, cmd
	}
	return a, nil
}

// rebuild recomputes columns, tree rows and the root list from the index.
func (a *App) rebuild() {
	if a.ix == nil {
		return
	}
	a.roots = nil
	for _, r := range a.ix.Roots() {
		if r.Status != model.StatusDone {
			a.roots = append(a.roots, r)
		}
	}
	if a.root >= len(a.roots) {
		a.root = -1
	}
	a.scope.Under = ""
	if a.root >= 0 {
		a.scope.Under = a.roots[a.root].ID
	}
	a.rebuildColumns()
	a.rebuildTree()
}

func (a *App) matchesText(t *model.Ticket) bool {
	if a.text == "" {
		return true
	}
	q := strings.ToLower(a.text)
	if strings.Contains(strings.ToLower(t.Title), q) || strings.Contains(strings.ToLower(t.ID), q) || strings.Contains(strings.ToLower(t.Slug), q) {
		return true
	}
	for _, l := range t.Labels {
		if strings.Contains(strings.ToLower(l), q) {
			return true
		}
	}
	return false
}

// current returns the ticket under the cursor for the active view.
func (a *App) current() *model.Ticket {
	switch {
	case a.mode == modeDetail:
		return a.ix.Get(a.detailID)
	case a.view == viewTree:
		if a.trow >= 0 && a.trow < len(a.rows) {
			return a.rows[a.trow].t
		}
	default:
		c := a.cols[a.col]
		if a.row >= 0 && a.row < len(c) {
			return c[a.row]
		}
	}
	return nil
}

func (a *App) updateNav(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "q":
		return a, tea.Quit
	case "?":
		a.mode = modeHelp
		return a, nil
	case "t":
		if a.view == viewBoard {
			a.view = viewTree
			a.syncTreeCursorTo(a.current())
		} else {
			a.view = viewBoard
			a.syncBoardCursorTo(a.current())
		}
		return a, nil
	case "r":
		return a, a.load()
	case "/":
		a.startPrompt("filter", "")
		return a, a.input.Focus()
	case "esc":
		if a.text != "" {
			a.text = ""
			a.rebuild()
		}
		return a, nil
	case "p":
		a.root++
		if a.root >= len(a.roots) {
			a.root = -1
		}
		a.rebuild()
		return a, nil
	case "n":
		return a.startForm()
	case "enter":
		if t := a.current(); t != nil {
			a.openDetail(t.ID)
			a.mode = modeDetail
		}
		return a, nil
	}
	if cmd, handled := a.ticketAction(k, a.current()); handled {
		return a, cmd
	}
	if a.view == viewTree {
		a.treeNav(k)
	} else {
		a.boardNav(k)
	}
	return a, nil
}

// ticketAction handles keys that act on a ticket in any view.
func (a *App) ticketAction(k string, t *model.Ticket) (tea.Cmd, bool) {
	if t == nil {
		return nil, false
	}
	switch k {
	case "]", "L":
		return a.moveBy(t, +1), true
	case "[", "H":
		return a.moveBy(t, -1), true
	case "c":
		return a.claim(t), true
	case "d":
		return a.done(t), true
	case "m":
		a.startPrompt("log", t.ID)
		return a.input.Focus(), true
	case "e":
		return a.editTicket(t), true
	}
	return nil, false
}

func (a *App) moveBy(t *model.Ticket, delta int) tea.Cmd {
	i := t.Status.Index() + delta
	if i < 0 || i >= len(model.AllStatuses) {
		return nil
	}
	to := model.AllStatuses[i]
	id, actor := t.ID, a.actor
	do := a.mutate(fmt.Sprintf("bored: move %s %s->%s (%s)", id, t.Status, to, actor), func(tx *store.Tx) error {
		cur, err := tx.Resolve(id)
		if err != nil {
			return err
		}
		if err := tx.Index().CanMove(cur, to, false); err != nil {
			return err
		}
		model.ApplyMove(cur, to)
		cur.Body = store.AppendLog(cur.Body, tx.Now().Local(), actor, "-> "+string(to))
		tx.Put(cur)
		return nil
	})
	if to == model.StatusDone {
		a.confirm(fmt.Sprintf("Mark %s done? (y/n)", id), do)
		return nil
	}
	return do
}

func (a *App) claim(t *model.Ticket) tea.Cmd {
	id, actor := t.ID, a.actor
	return a.mutate(fmt.Sprintf("bored: claim %s (%s)", id, actor), func(tx *store.Tx) error {
		cur, err := tx.Resolve(id)
		if err != nil {
			return err
		}
		if err := tx.Index().CanClaim(cur, actor, false); err != nil {
			return err
		}
		model.Claim(cur, actor, tx.Now())
		cur.Body = store.AppendLog(cur.Body, tx.Now().Local(), actor, "claimed")
		tx.Put(cur)
		return nil
	})
}

func (a *App) done(t *model.Ticket) tea.Cmd {
	id, actor := t.ID, a.actor
	do := a.mutate(fmt.Sprintf("bored: done %s (%s)", id, actor), func(tx *store.Tx) error {
		cur, err := tx.Resolve(id)
		if err != nil {
			return err
		}
		if err := tx.Index().CanDone(cur, false); err != nil {
			return err
		}
		model.ApplyMove(cur, model.StatusDone)
		cur.Body = store.AppendLog(cur.Body, tx.Now().Local(), actor, "-> done")
		tx.Put(cur)
		return nil
	})
	a.confirm(fmt.Sprintf("Mark %s done? (y/n)", id), do)
	return nil
}

func (a *App) editTicket(t *model.Ticket) tea.Cmd {
	sess, err := edit.Prepare(a.store, t.ID)
	if err != nil {
		a.setStatus(err.Error(), true)
		return nil
	}
	return tea.ExecProcess(sess.Command(a.store.Cfg), func(err error) tea.Msg {
		return editDoneMsg{sess: sess, err: err}
	})
}

func (a *App) confirm(msg string, cmd tea.Cmd) {
	a.confirmMsg, a.confirmCmd = msg, cmd
	a.prevMode = a.mode
	a.mode = modeConfirm
}

func (a *App) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		cmd := a.confirmCmd
		a.confirmMsg, a.confirmCmd = "", nil
		a.mode = a.returnMode()
		return a, cmd
	default:
		a.confirmMsg, a.confirmCmd = "", nil
		a.mode = a.returnMode()
		return a, nil
	}
}

// returnMode is where a prompt or confirm goes back to.
func (a *App) returnMode() mode {
	if a.prevMode == modeDetail {
		return modeDetail
	}
	return modeNav
}

func (a *App) startPrompt(kind, id string) {
	a.promptKind, a.promptID = kind, id
	a.input.Reset()
	a.input.SetWidth(max(20, a.width-20))
	if kind == "filter" {
		a.input.SetValue(a.text)
	}
	a.prevMode = a.mode
	a.mode = modePrompt
}

func (a *App) updatePrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.input.Blur()
		a.mode = a.returnMode()
		return a, nil
	case "enter":
		val := strings.TrimSpace(a.input.Value())
		a.input.Blur()
		a.mode = a.returnMode()
		switch a.promptKind {
		case "filter":
			a.text = val
			a.rebuild()
			return a, nil
		case "log":
			if val == "" {
				return a, nil
			}
			id, actor := a.promptID, a.actor
			return a, a.mutate("bored: log "+id, func(tx *store.Tx) error {
				cur, err := tx.Resolve(id)
				if err != nil {
					return err
				}
				cur.Body = store.AppendLog(cur.Body, tx.Now().Local(), actor, val)
				tx.Put(cur)
				return nil
			})
		}
		return a, nil
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	if a.promptKind == "filter" {
		a.text = a.input.Value()
		a.rebuild()
	}
	return a, cmd
}

func (a *App) View() tea.View {
	var body string
	bodyMode := a.mode
	if a.mode == modeConfirm || a.mode == modePrompt {
		bodyMode = a.prevMode // keep showing what the user was looking at
	}
	switch bodyMode {
	case modeHelp:
		body = a.renderHelp()
	case modeForm:
		body = a.renderForm()
	case modeDetail:
		body = a.renderDetail()
	default:
		if a.view == viewTree {
			body = a.renderTree()
		} else {
			body = a.renderBoard()
		}
	}
	if a.mode == modeConfirm {
		body = a.overlay(body, a.renderConfirmBox())
	}
	v := tea.NewView(a.renderTitle() + "\n" + body + "\n" + a.renderStatus())
	v.AltScreen = true
	v.WindowTitle = "bored"
	return v
}

func (a *App) bodyHeight() int { return max(3, a.height-4) }

func (a *App) renderTitle() string {
	scope := "all"
	if a.root >= 0 && a.root < len(a.roots) {
		scope = a.roots[a.root].Ref()
	}
	view := "board"
	if a.view == viewTree {
		view = "tree"
	}
	if a.mode == modeDetail {
		view = "ticket"
	}
	left := a.st.title.Render("bored") + a.st.dim.Render("  "+view+"  under: "+scope)
	if a.text != "" {
		left += a.st.dim.Render("  filter: ") + a.st.accent.Render(a.text)
	}
	right := a.st.dim.Render(a.store.Root)
	gap := a.width - lipglossWidth(left) - lipglossWidth(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (a *App) renderStatus() string {
	line1 := ""
	switch a.mode {
	case modePrompt:
		label := "filter: "
		if a.promptKind == "log" {
			label = "log " + a.promptID + ": "
		}
		line1 = a.st.promptLabel.Render(label) + a.input.View()
	case modeConfirm:
		line1 = a.st.promptLabel.Render(a.confirmMsg) + a.st.dim.Render("   y = yes, any other key = no")
	default:
		if a.status != "" {
			if a.statErr {
				line1 = a.st.errText.Render(a.status)
			} else {
				line1 = a.st.ok.Render(a.status)
			}
		}
	}
	var hints string
	switch a.mode {
	case modeDetail:
		hints = "j/k scroll  [ ] move  c claim  d done  m log  e edit  esc back"
	case modeForm:
		hints = "tab/shift+tab fields  enter next  esc cancel"
	case modePrompt:
		hints = "enter apply  esc cancel"
	case modeConfirm:
		hints = "y / enter  confirm      n / esc  cancel"
	default:
		hints = "j/k/h/l move  enter open  t tree/board  [ ] move  c claim  d done  n new  m log  e edit  / filter  p root  r reload  ? help  q quit"
	}
	return truncate(line1, a.width) + "\n" + a.st.help.Render(truncate(hints, a.width))
}

// renderConfirmBox draws the yes/no dialog.
func (a *App) renderConfirmBox() string {
	q := a.confirmMsg
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.st.accent.GetForeground()).
		Padding(1, 3).
		Render(a.st.promptLabel.Render(q) + "\n\n" + a.st.dim.Render("y / enter  yes        n / esc  no"))
	return box
}

// overlay centres box on top of body using the lipgloss compositor, so the
// body shows through on either side of the dialog.
func (a *App) overlay(body, box string) string {
	h := a.bodyHeight()
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	top := max(0, (h-bh)/2)
	left := max(0, (a.width-bw)/2)
	c := lipgloss.NewCanvas(a.width, h)
	c.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(body).X(0).Y(0).Z(0),
		lipgloss.NewLayer(box).X(left).Y(top).Z(1),
	))
	return c.Render()
}

func (a *App) renderHelp() string {
	h := `Keys

  j / k, up / down     move within a column or the tree
  h / l, left / right  move between columns
  g / G                top / bottom
  enter                open the ticket
  t                    toggle board / tree
  ] or L               move ticket one status right (asks before done)
  [ or H               move ticket one status left (todo clears the assignee)
  c                    claim (todo -> doing, assigned to you)
  d                    done (from review; asks first)
  n                    new ticket (child of the selected ticket in tree view,
                       sibling of it on the board)
  m                    append a log line
  e                    open in $EDITOR, validated on save
  /                    filter by text (title, id, slug, labels); esc clears
  p                    cycle the root filter (all -> each initiative)
  r                    reload from disk (also happens every 3s if files change)
  q                    quit

The board shows workable tickets only: anything with open children is a
container and lives in the tree view until its children are done.

Press any key to go back.`
	return h
}

func lipglossWidth(s string) int { return ansi.StringWidth(s) }

func stripANSI(s string) string { return ansi.Strip(s) }

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// rendered line helpers used by board and tree
func (a *App) renderCard(t *model.Ticket, width int, selected bool) string {
	where := render.Where(a.ix, t)
	head := a.st.id.Render(t.ID) + a.st.dim.Render(fmt.Sprintf("  P%d", t.Priority))
	if where != "" {
		head += a.st.dim.Render("  " + where)
	}
	if t.Assignee != "" {
		head += a.st.assignee.Render("  " + t.Assignee)
	}
	lines := []string{head, wrap(t.Title, width-4)}
	var badges []string
	if d, n := store.CountAC(t.Body); n > 0 {
		badges = append(badges, a.st.badge.Render(fmt.Sprintf("ac %d/%d", d, n)))
	}
	if bl := a.ix.BlockedBy(t.ID); len(bl) > 0 {
		badges = append(badges, a.st.blocked.Render("blocked by "+bl[0].ID))
	}
	if len(badges) > 0 {
		lines = append(lines, strings.Join(badges, " "))
	}
	style := a.st.card
	if selected {
		style = a.st.cardSel
	}
	return style.Width(width - 2).Render(strings.Join(lines, "\n"))
}

func wrap(s string, w int) string {
	if w < 4 {
		return s
	}
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line == "" {
			line = word
		} else if len([]rune(line))+1+len([]rune(word)) <= w {
			line += " " + word
		} else {
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
