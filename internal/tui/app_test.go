package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func seed(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Init(t.TempDir(), "T")
	if err != nil {
		t.Fatal(err)
	}
	s.Cfg.AutoCommit = false
	mk := func(title, parent string, pri int, ac []string) string {
		var id string
		err := s.Mutate("", func(tx *store.Tx) error {
			tk := &model.Ticket{Title: title, Priority: pri, Body: store.NewBody("desc of "+title, ac)}
			if parent != "" {
				p, err := tx.Resolve(parent)
				if err != nil {
					return err
				}
				tk.Parent = p.ID
			}
			if len(ac) > 0 {
				tk.Status = model.StatusTodo
			}
			if err := tx.Create(tk); err != nil {
				return err
			}
			id = tk.ID
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	root := mk("The IAM build", "", 3, nil)
	chunk := mk("Authentication", root, 3, nil)
	mk("Decide IdP", chunk, 1, []string{"decision recorded"})
	mk("Add OAuth login", chunk, 2, []string{"redirects", "cookie"})
	mk("Loose idea", "", 4, nil)
	return s
}

// drive applies a message and any resulting commands synchronously.
func drive(t *testing.T, a *App, msg tea.Msg) {
	t.Helper()
	m, cmd := a.Update(msg)
	if m != a {
		t.Fatalf("Update returned a different model")
	}
	for i := 0; cmd != nil && i < 10; i++ {
		out := cmd()
		cmd = nil
		switch out := out.(type) {
		case nil:
		case tea.BatchMsg:
			for _, c := range out {
				if c != nil {
					if m := c(); m != nil {
						if _, isTick := m.(tickMsg); !isTick {
							_, cmd = a.Update(m)
						}
					}
				}
			}
		case tickMsg:
			// don't loop on the poll
		default:
			_, cmd = a.Update(out)
		}
	}
}

func newTestApp(t *testing.T, w, h int) *App {
	s := seed(t)
	a := newApp(s, "tester")
	drive(t, a, tea.WindowSizeMsg{Width: w, Height: h})
	m := a.load()()
	drive(t, a, m)
	return a
}

func plain(a *App) string { return stripANSI(a.View().Content) }

func TestBoardRendersAndHidesContainers(t *testing.T) {
	a := newTestApp(t, 120, 30)
	v := plain(a)
	for _, want := range []string{"BACKLOG 1", "TODO 2", "T-3  Decide IdP", "T-4  Add OAuth login", "T-5  Loose idea"} {
		if !strings.Contains(v, want) {
			t.Errorf("board missing %q:\n%s", want, v)
		}
	}
	cardLine := regexp.MustCompile(`(^|\s)(T-1|T-2)  [A-Za-z]`)
	if cardLine.MatchString(v) {
		t.Errorf("containers should not appear as cards:\n%s", v)
	}
	// Details line describes the selected card (T-5 in backlog).
	if !strings.Contains(v, "T-5  ·  P4  ·  backlog") {
		t.Errorf("details line missing:\n%s", v)
	}
	// Move to T-4 and the details line should show its AC count and root.
	drive(t, a, key("l"))
	drive(t, a, key("j"))
	if v := plain(a); !strings.Contains(v, "ac 0/2") || !strings.Contains(v, "·  T-1  ·") {
		t.Errorf("details for T-4 missing:\n%s", v)
	}
}

func TestViewToggleCarriesCursor(t *testing.T) {
	a := newTestApp(t, 120, 30)
	drive(t, a, key("l")) // board: T-3
	drive(t, a, key("t"))
	if a.current() == nil || a.current().ID != "T-3" {
		t.Fatalf("tree cursor should land on T-3, got %v", a.current())
	}
	drive(t, a, key("j")) // tree: next row is T-4
	drive(t, a, key("t"))
	if a.current() == nil || a.current().ID != "T-4" {
		t.Fatalf("board cursor should land on T-4, got %v", a.current())
	}
}

func TestNavigationDetailAndTree(t *testing.T) {
	a := newTestApp(t, 120, 30)
	// cursor starts backlog col; move to todo column and down
	drive(t, a, key("l"))
	if a.col != 1 || a.current() == nil || a.current().ID != "T-3" {
		t.Fatalf("after l: col=%d current=%v", a.col, a.current())
	}
	drive(t, a, key("j"))
	if a.current().ID != "T-4" {
		t.Fatalf("after j: %v", a.current().ID)
	}
	drive(t, a, key("enter"))
	if a.mode != modeDetail {
		t.Fatal("enter should open detail")
	}
	if v := plain(a); !strings.Contains(v, "Add OAuth login") || !strings.Contains(v, "under: T-2") {
		t.Errorf("detail view:\n%s", v)
	}
	drive(t, a, key("esc"))
	if a.mode != modeNav {
		t.Fatal("esc should close detail")
	}
	drive(t, a, key("t"))
	if a.view != viewTree {
		t.Fatal("t should switch to tree")
	}
	v := plain(a)
	for _, want := range []string{"T-1", "└── T-2", "    ├── T-3", "0/2", "T-5"} {
		if !strings.Contains(v, want) {
			t.Errorf("tree missing %q:\n%s", want, v)
		}
	}
	if a.current().ID != "T-4" {
		t.Errorf("tree cursor should follow the board selection, got %s", a.current().ID)
	}
	drive(t, a, key("h"))
	if a.current().ID != "T-2" {
		t.Errorf("h should jump to parent, got %s", a.current().ID)
	}
}

func TestClaimMoveAndDoneViaKeys(t *testing.T) {
	a := newTestApp(t, 120, 30)
	drive(t, a, key("l")) // todo column, T-3
	drive(t, a, key("c")) // claim
	tk := a.ix.Get("T-3")
	if tk.Status != model.StatusDoing || tk.Assignee != "tester" {
		t.Fatalf("claim failed: %+v status=%s", a.status, tk.Status)
	}
	// cursor stays in the todo column, now on the card that was below T-3
	if a.col != 1 || a.current() == nil || a.current().ID != "T-4" {
		t.Fatalf("cursor should stay in todo on T-4: col=%d row=%d cur=%v", a.col, a.row, a.current())
	}
	drive(t, a, key("l")) // doing column: T-3
	if a.current().ID != "T-3" {
		t.Fatalf("expected T-3 in doing, got %v", a.current())
	}
	drive(t, a, key("]")) // doing -> review
	if a.ix.Get("T-3").Status != model.StatusReview {
		t.Fatalf("move right failed: %s", a.status)
	}
	if a.col != 2 {
		t.Fatalf("cursor should stay in the doing column, got col=%d", a.col)
	}
	drive(t, a, key("l")) // review column: T-3
	drive(t, a, key("]")) // review -> done asks
	if a.mode != modeConfirm {
		t.Fatal("moving to done should ask for confirmation")
	}
	drive(t, a, key("n"))
	if a.ix.Get("T-3").Status != model.StatusReview || a.mode != modeNav {
		t.Fatal("n should cancel")
	}
	drive(t, a, key("d"))
	drive(t, a, key("y"))
	if a.ix.Get("T-3").Status != model.StatusDone {
		t.Fatalf("done failed: %s", a.status)
	}
	if a.col != 3 {
		t.Fatalf("cursor should stay in the review column after done, got col=%d", a.col)
	}
	// log prompt
	drive(t, a, key("l")) // move to... whichever; pick T-4 explicitly
	a.syncBoardCursorTo(a.ix.Get("T-4"))
	drive(t, a, key("m"))
	if a.mode != modePrompt {
		t.Fatal("m should open the log prompt")
	}
	for _, ch := range "hello" {
		drive(t, a, key(string(ch)))
	}
	drive(t, a, key("enter"))
	if !strings.Contains(a.ix.Get("T-4").Body, "tester: hello") {
		t.Errorf("log line missing:\n%s", a.ix.Get("T-4").Body)
	}
}

func TestFilterAndRootCycle(t *testing.T) {
	a := newTestApp(t, 120, 30)
	drive(t, a, key("/"))
	for _, ch := range "oauth" {
		drive(t, a, key(string(ch)))
	}
	drive(t, a, key("enter"))
	v := plain(a)
	if !strings.Contains(v, "T-4  Add OAuth login") || strings.Contains(v, "T-3  Decide") {
		t.Errorf("filter should keep only T-4:\n%s", v)
	}
	drive(t, a, key("esc"))
	if a.text != "" {
		t.Error("esc should clear the filter")
	}
	drive(t, a, key("p")) // first root: T-1
	if a.scope.Under != "T-1" {
		t.Errorf("p should scope under T-1, got %q", a.scope.Under)
	}
	if v := plain(a); strings.Contains(v, "T-5") {
		t.Errorf("T-5 is another root and should be hidden:\n%s", v)
	}
	drive(t, a, key("p"))
	drive(t, a, key("p"))
	if a.scope.Under != "" {
		t.Errorf("cycling past the last root should return to all, got %q", a.scope.Under)
	}
}

func TestFormCreatesTicket(t *testing.T) {
	a := newTestApp(t, 120, 40)
	drive(t, a, key("t"))
	a.syncTreeCursorTo(a.ix.Get("T-2"))
	_, cmd := a.startForm()
	if a.mode != modeForm || a.draft.parent != "T-2" {
		t.Fatalf("form should default parent to the selected tree node, got %q", a.draft.parent)
	}
	if cmd != nil {
		if m := cmd(); m != nil {
			drive(t, a, m)
		}
	}
	if v := plain(a); !strings.Contains(v, "New ticket") || !strings.Contains(v, "Title") {
		t.Errorf("form view:\n%s", v)
	}
	// Bypass the interactive form: complete a draft directly.
	a.form = nil
	a.mode = modeNav
	drive(t, a, a.createFromDraft(draft{title: "Child from form", parent: "T-2", priority: "2", description: "made in the TUI"})())
	var found *model.Ticket
	for _, tk := range a.ix.All() {
		if tk.Title == "Child from form" {
			found = tk
		}
	}
	if found == nil || found.Parent != "T-2" || found.Priority != 2 || !strings.Contains(found.Body, "made in the TUI") {
		t.Fatalf("ticket not created correctly: %+v", found)
	}
}

func TestSmallTerminalDoesNotPanic(t *testing.T) {
	for _, sz := range [][2]int{{40, 10}, {60, 15}, {200, 60}} {
		a := newTestApp(t, sz[0], sz[1])
		for _, k := range []string{"l", "j", "enter", "esc", "t", "j", "?", "esc", "G", "g"} {
			drive(t, a, key(k))
		}
		if plain(a) == "" {
			t.Errorf("empty view at %v", sz)
		}
	}
}

func TestPollReloadsWhenFilesChange(t *testing.T) {
	a := newTestApp(t, 120, 30)
	before := len(a.ix.All())
	// Another process adds a ticket.
	err := a.store.Mutate("", func(tx *store.Tx) error {
		return tx.Create(&model.Ticket{Title: "From elsewhere", Priority: 3, Body: store.NewBody("", nil)})
	})
	if err != nil {
		t.Fatal(err)
	}
	// Ensure the mtime moves past what the app saw (filesystems have coarse clocks).
	a.lastMod = a.lastMod.Add(-2 * 1e9)
	drive(t, a, tickMsg{})
	if len(a.ix.All()) != before+1 {
		t.Errorf("poll did not reload: %d -> %d", before, len(a.ix.All()))
	}
}

func TestConfirmDialogIsVisible(t *testing.T) {
	a := newTestApp(t, 120, 30)
	drive(t, a, key("l"))
	drive(t, a, key("c")) // T-3 -> doing; cursor stays in todo
	drive(t, a, key("l")) // doing: T-3
	drive(t, a, key("]")) // -> review; cursor stays in doing
	drive(t, a, key("l")) // review: T-3
	drive(t, a, key("d"))
	if a.mode != modeConfirm {
		t.Fatal("d from review should ask")
	}
	v := plain(a)
	if !strings.Contains(v, "Mark T-3 done?") || !strings.Contains(v, "y / enter  yes") {
		t.Errorf("confirm dialog not visible:\n%s", v)
	}
	drive(t, a, key("enter"))
	if a.ix.Get("T-3").Status != model.StatusDone {
		t.Fatalf("enter should confirm: %s", a.status)
	}
}

func TestConfirmFromDetailKeepsDetailVisible(t *testing.T) {
	a := newTestApp(t, 120, 30)
	drive(t, a, key("l"))
	drive(t, a, key("c"))
	drive(t, a, key("l"))
	drive(t, a, key("]"))
	drive(t, a, key("l"))
	drive(t, a, key("enter"))
	if a.mode != modeDetail {
		t.Fatal("expected detail")
	}
	drive(t, a, key("d"))
	v := plain(a)
	if !strings.Contains(v, "Mark T-3 done?") || !strings.Contains(v, "under: T-2") {
		t.Errorf("dialog should overlay the detail view:\n%s", v)
	}
	drive(t, a, key("y"))
	if a.ix.Get("T-3").Status != model.StatusDone || a.mode != modeDetail {
		t.Errorf("after confirm: status=%s mode=%d", a.ix.Get("T-3").Status, a.mode)
	}
}

func TestSelectedRowHighlightCoversWholeLine(t *testing.T) {
	a := newTestApp(t, 120, 30)
	drive(t, a, key("l"))
	raw := a.View().Content
	// The selected row must be one styled run: bold+reverse immediately
	// followed by id and title, with no reset in between.
	if !strings.Contains(raw, "\x1b[1;7mT-3  Decide IdP") {
		for _, line := range strings.Split(raw, "\n") {
			if strings.Contains(line, "Decide IdP") {
				t.Fatalf("selected row not a single highlighted run: %q", line)
			}
		}
		t.Fatal("selected row not found")
	}
	// Tree view: same rule for the selected row.
	drive(t, a, key("t"))
	raw = a.View().Content
	if !strings.Contains(raw, "\x1b[1;7m") {
		t.Fatal("tree selection not highlighted")
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, "\x1b[1;7m") {
			seg := line[strings.Index(line, "\x1b[1;7m"):]
			end := strings.Index(seg, "\x1b[m")
			if end < 0 || !strings.Contains(seg[:end], "Decide IdP") {
				t.Errorf("tree selected row is not one highlighted run: %q", line)
			}
		}
	}
}
