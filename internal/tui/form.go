package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

type draft struct {
	title, parent, repo, priority, description string
}

// startForm opens the new-ticket form. In tree view the selected ticket is
// the default parent; on the board the selected ticket's parent is.
func (a *App) startForm() (tea.Model, tea.Cmd) {
	a.draft = draft{priority: "3"}
	if t := a.current(); t != nil {
		if a.view == viewTree || a.mode == modeDetail {
			a.draft.parent = t.ID
		} else {
			a.draft.parent = t.Parent
		}
	} else if a.scope.Under != "" {
		a.draft.parent = a.scope.Under
	}
	parentOpts := []huh.Option[string]{huh.NewOption("(none: a new root)", "")}
	for _, t := range a.ix.All() {
		if t.Status == model.StatusDone {
			continue
		}
		label := t.ID + "  " + t.Title
		if t.Slug != "" {
			label = t.ID + " (" + t.Slug + ")  " + t.Title
		}
		parentOpts = append(parentOpts, huh.NewOption(label, t.ID))
	}
	repoOpts := []huh.Option[string]{huh.NewOption("(inherit from parent)", ""), huh.NewOption("(none)", "none")}
	for _, r := range a.store.RepoList() {
		repoOpts = append(repoOpts, huh.NewOption(r.Name, r.Name))
	}
	a.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Title").Value(&a.draft.title).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("title is required")
			}
			return nil
		}),
		huh.NewSelect[string]().Title("Parent").Options(parentOpts...).Value(&a.draft.parent).Filtering(true).Height(8),
		huh.NewSelect[string]().Title("Repo").Options(repoOpts...).Value(&a.draft.repo),
		huh.NewSelect[string]().Title("Priority").Options(huh.NewOptions("1", "2", "3", "4")...).Value(&a.draft.priority),
		huh.NewText().Title("Description").Lines(4).Value(&a.draft.description),
	)).WithWidth(min(a.width-4, 90)).WithShowHelp(true)
	a.form = a.form.WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	a.prevMode = a.mode
	a.mode = modeForm
	return a, a.form.Init()
}

func (a *App) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		a.form = nil
		a.mode = a.returnMode()
		return a, nil
	}
	m, cmd := a.form.Update(msg)
	if f, ok := m.(*huh.Form); ok {
		a.form = f
	}
	switch a.form.State {
	case huh.StateAborted:
		a.form = nil
		a.mode = a.returnMode()
		return a, nil
	case huh.StateCompleted:
		d := a.draft
		a.form = nil
		a.mode = a.returnMode()
		return a, a.createFromDraft(d)
	}
	return a, cmd
}

func (a *App) createFromDraft(d draft) tea.Cmd {
	pri, _ := strconv.Atoi(d.priority)
	if pri == 0 {
		pri = 3
	}
	var created string
	s := a.store
	return func() tea.Msg {
		err := s.Mutate("", func(tx *store.Tx) error {
			t := &model.Ticket{Title: strings.TrimSpace(d.title), Priority: pri, Status: model.StatusBacklog, DependsOn: []string{}, Labels: []string{}}
			if d.parent != "" {
				p, err := tx.Resolve(d.parent)
				if err != nil {
					return err
				}
				t.Parent = p.ID
				t.Repo = p.Repo
			}
			switch d.repo {
			case "":
			case "none":
				t.Repo = ""
			default:
				t.Repo = d.repo
			}
			t.Body = store.NewBody(d.description, nil)
			if err := tx.Create(t); err != nil {
				return err
			}
			created = t.ID
			tx.CommitMsg = fmt.Sprintf("bored: new %s %q", t.ID, t.Title)
			return nil
		})
		if err != nil {
			return mutationDoneMsg{what: "new", err: err}
		}
		return mutationDoneMsg{what: "bored: new " + created}
	}
}

func (a *App) renderForm() string {
	if a.form == nil {
		return ""
	}
	return a.st.title.Render("New ticket") + "\n\n" + a.form.View()
}
