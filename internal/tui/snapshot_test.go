package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

// TestConfirmOverlayKeepsSurroundingCards puts cards on the rows the dialog
// covers and checks they are still drawn either side of it.
func TestConfirmOverlayKeepsSurroundingCards(t *testing.T) {
	a := newTestApp(t, 140, 30)
	for i := 0; i < 20; i++ {
		err := a.store.Mutate("", func(tx *store.Tx) error {
			return tx.Create(&model.Ticket{Title: fmt.Sprintf("Filler %d", i), Priority: 3, Status: model.StatusTodo, Body: store.NewBody("", []string{"x"})})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	drive(t, a, a.load()())
	drive(t, a, tea.WindowSizeMsg{Width: 140, Height: 30})
	drive(t, a, key("l"))
	drive(t, a, key("c"))
	drive(t, a, key("]"))
	drive(t, a, key("d"))
	v := plain(a)
	t.Logf("\n%s", v)
	found := false
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "Mark T-3 done?") && !strings.Contains(line, "y = yes") {
			found = true
			// A TODO-column card sits left of the dialog on this row.
			if !strings.Contains(line[:strings.Index(line, "Mark")], "Filler") {
				t.Errorf("card to the left of the dialog was erased: %q", line)
			}
		}
	}
	if !found {
		t.Fatal("dialog row not found")
	}
}
