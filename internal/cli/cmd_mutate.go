package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/render"
	"github.com/Commonwealth-Labs/bored/internal/store"
	"github.com/spf13/cobra"
)

// mutateOne runs fn on a resolved ticket and prints the result.
func mutateOne(ref string, fn func(tx *store.Tx, t *model.Ticket) (string, error), text func(t *model.Ticket) string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	var id string
	err = s.Mutate("", func(tx *store.Tx) error {
		t, err := tx.Resolve(ref)
		if err != nil {
			return err
		}
		id = t.ID
		msg, err := fn(tx, t)
		if err != nil {
			return err
		}
		tx.Put(t)
		tx.CommitMsg = msg
		return nil
	})
	if err != nil {
		return err
	}
	ix, err := s.Index()
	if err != nil {
		return err
	}
	t := ix.Get(id)
	return out(view(ix, t, false), text(t))
}

func newMoveCmd() *cobra.Command {
	var note string
	var force bool
	c := &cobra.Command{
		Use:   "move <id> <status>",
		Short: "Move a ticket to a status (todo clears the assignee)",
		Args:  exactArgs(2, "move <id> <status> [-m note] [--force]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			to, err := model.ParseStatus(args[1])
			if err != nil {
				return err
			}
			return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
				if err := tx.Index().CanMove(t, to, force); err != nil {
					return "", err
				}
				from := t.Status
				model.ApplyMove(t, to)
				who := actor(tx.Store())
				line := "-> " + string(to)
				if note != "" {
					line += ": " + note
				}
				t.Body = store.AppendLog(t.Body, tx.Now().Local(), who, line)
				return fmt.Sprintf("bored: move %s %s->%s (%s)", t.ID, from, to, who), nil
			}, func(t *model.Ticket) string { return fmt.Sprintf("%s -> %s\n", t.ID, t.Status) })
		},
	}
	c.Flags().StringVarP(&note, "message", "m", "", "note appended to the log")
	c.Flags().BoolVar(&force, "force", false, "allow done with open children")
	return c
}

func newClaimCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "claim <id>",
		Short: "Take a todo ticket: sets doing and assignee",
		Args:  exactArgs(1, "claim <id> [--as actor] [--force]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
				who := actor(tx.Store())
				if err := tx.Index().CanClaim(t, who, force); err != nil {
					return "", err
				}
				model.Claim(t, who, tx.Now())
				t.Body = store.AppendLog(t.Body, tx.Now().Local(), who, "claimed")
				return fmt.Sprintf("bored: claim %s (%s)", t.ID, who), nil
			}, func(t *model.Ticket) string { return fmt.Sprintf("%s claimed by %s\n", t.ID, t.Assignee) })
		},
	}
	c.Flags().BoolVar(&force, "force", false, "claim even if blocked, assigned to someone else, or in backlog")
	return c
}

func newDoneCmd() *cobra.Command {
	var note string
	var force bool
	c := &cobra.Command{
		Use:   "done <id>",
		Short: "Mark a reviewed ticket done (the human gate)",
		Args:  exactArgs(1, "done <id> [-m note] [--force]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
				if err := tx.Index().CanDone(t, force); err != nil {
					return "", err
				}
				who := actor(tx.Store())
				model.ApplyMove(t, model.StatusDone)
				line := "-> done"
				if note != "" {
					line += ": " + note
				}
				t.Body = store.AppendLog(t.Body, tx.Now().Local(), who, line)
				return fmt.Sprintf("bored: done %s (%s)", t.ID, who), nil
			}, func(t *model.Ticket) string { return t.ID + " done\n" })
		},
	}
	c.Flags().StringVarP(&note, "message", "m", "", "note appended to the log")
	c.Flags().BoolVar(&force, "force", false, "allow from doing/todo and with open children")
	return c
}

func newLogCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "log <id> <message>",
		Short: "Append a timestamped line to the ticket's log",
		Args:  minArgs(2, "log <id> <message words...>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := strings.TrimSpace(strings.Join(args[1:], " "))
			if msg == "" {
				return fmt.Errorf("%w: empty log message", model.ErrUsage)
			}
			return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
				t.Body = store.AppendLog(t.Body, tx.Now().Local(), actor(tx.Store()), msg)
				return "bored: log " + t.ID, nil
			}, func(t *model.Ticket) string { return t.ID + ": logged\n" })
		},
	}
	return c
}

func newACCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "ac <id> [list|check N|uncheck N|add TEXT]",
		Short: "List, check, uncheck or add acceptance criteria",
		Args:  minArgs(1, "ac <id> [list|check N|uncheck N|add TEXT]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			op := "list"
			if len(args) > 1 {
				op = args[1]
			}
			switch op {
			case "list":
				s, err := openStore()
				if err != nil {
					return err
				}
				ix, err := s.Index()
				if err != nil {
					return err
				}
				t, err := ix.Resolve(args[0])
				if err != nil {
					return err
				}
				items := store.ListAC(t.Body)
				if items == nil {
					items = []store.ACItem{}
				}
				var sb strings.Builder
				for _, it := range items {
					mark := " "
					if it.Done {
						mark = "x"
					}
					sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", it.N, mark, it.Text))
				}
				if len(items) == 0 {
					sb.WriteString("(no acceptance criteria)\n")
				}
				return out(items, sb.String())
			case "check", "uncheck":
				if len(args) != 3 {
					return fmt.Errorf("%w: ac <id> %s N", model.ErrUsage, op)
				}
				n, err := strconv.Atoi(args[2])
				if err != nil || n < 1 {
					return fmt.Errorf("%w: N must be a positive number", model.ErrUsage)
				}
				done := op == "check"
				return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
					body, err := store.SetAC(t.Body, n, done)
					if err != nil {
						return "", fmt.Errorf("%w: %v", model.ErrNotFound, err)
					}
					t.Body = body
					return fmt.Sprintf("bored: ac %s %s %d", t.ID, op, n), nil
				}, func(t *model.Ticket) string {
					d, tot := store.CountAC(t.Body)
					return fmt.Sprintf("%s: ac %d/%d done\n", t.ID, d, tot)
				})
			case "add":
				if len(args) < 3 {
					return fmt.Errorf("%w: ac <id> add TEXT", model.ErrUsage)
				}
				text := strings.Join(args[2:], " ")
				return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
					t.Body = store.AddAC(t.Body, text)
					return "bored: ac " + t.ID + " add", nil
				}, func(t *model.Ticket) string {
					_, tot := store.CountAC(t.Body)
					return fmt.Sprintf("%s: %d acceptance criteria\n", t.ID, tot)
				})
			}
			return fmt.Errorf("%w: unknown ac operation %q", model.ErrUsage, op)
		},
	}
	return c
}

func newDepCmd() *cobra.Command {
	c := &cobra.Command{Use: "dep", Short: "Manage dependencies (blockers)"}
	add := &cobra.Command{
		Use:   "add <id> <blocker-id>",
		Short: "Make <id> wait on <blocker-id>",
		Args:  exactArgs(2, "dep add <id> <blocker-id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
				b, err := tx.Resolve(args[1])
				if err != nil {
					return "", err
				}
				if b.ID == t.ID {
					return "", fmt.Errorf("%w: a ticket cannot block itself", model.ErrUsage)
				}
				for _, d := range t.DependsOn {
					if d == b.ID {
						return "", fmt.Errorf("%w: %s already depends on %s", model.ErrConflict, t.ID, b.ID)
					}
				}
				if tx.Index().WouldCycleDep(t.ID, b.ID) {
					return "", fmt.Errorf("%w: %s already depends on %s (cycle)", model.ErrConflict, b.ID, t.ID)
				}
				t.DependsOn = append(t.DependsOn, b.ID)
				return fmt.Sprintf("bored: dep %s blocked-by %s", t.ID, b.ID), nil
			}, func(t *model.Ticket) string {
				return fmt.Sprintf("%s depends on %s\n", t.ID, strings.Join(t.DependsOn, ", "))
			})
		},
	}
	rm := &cobra.Command{
		Use:   "rm <id> <blocker-id>",
		Short: "Remove a dependency",
		Args:  exactArgs(2, "dep rm <id> <blocker-id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateOne(args[0], func(tx *store.Tx, t *model.Ticket) (string, error) {
				b, err := tx.Resolve(args[1])
				if err != nil {
					return "", err
				}
				kept := t.DependsOn[:0]
				found := false
				for _, d := range t.DependsOn {
					if d == b.ID {
						found = true
						continue
					}
					kept = append(kept, d)
				}
				if !found {
					return "", fmt.Errorf("%w: %s does not depend on %s", model.ErrNotFound, t.ID, b.ID)
				}
				t.DependsOn = kept
				return fmt.Sprintf("bored: dep %s unblocked-by %s", t.ID, b.ID), nil
			}, func(t *model.Ticket) string {
				return fmt.Sprintf("%s depends on [%s]\n", t.ID, strings.Join(t.DependsOn, ", "))
			})
		},
	}
	list := &cobra.Command{
		Use:   "list <id>",
		Short: "Show blockers and dependents",
		Args:  exactArgs(1, "dep list <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			t, err := ix.Resolve(args[0])
			if err != nil {
				return err
			}
			type depView struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Title  string `json:"title"`
			}
			var blockers, dependents []depView
			var sb strings.Builder
			sb.WriteString(t.ID + " depends on:\n")
			for _, d := range t.DependsOn {
				b := ix.Get(d)
				if b == nil {
					blockers = append(blockers, depView{ID: d, Status: "missing"})
					sb.WriteString("  " + d + "  (missing)\n")
					continue
				}
				blockers = append(blockers, depView{b.ID, string(b.Status), b.Title})
				sb.WriteString(fmt.Sprintf("  %s  %-7s %s\n", b.ID, b.Status, b.Title))
			}
			if len(t.DependsOn) == 0 {
				sb.WriteString("  (nothing)\n")
			}
			sb.WriteString(t.ID + " blocks:\n")
			for _, d := range ix.Dependents(t.ID) {
				dependents = append(dependents, depView{d.ID, string(d.Status), d.Title})
				sb.WriteString(fmt.Sprintf("  %s  %-7s %s\n", d.ID, d.Status, d.Title))
			}
			if len(dependents) == 0 {
				sb.WriteString("  (nothing)\n")
			}
			if blockers == nil {
				blockers = []depView{}
			}
			if dependents == nil {
				dependents = []depView{}
			}
			return out(map[string]any{"id": t.ID, "depends_on": blockers, "blocks": dependents}, sb.String())
		},
	}
	c.AddCommand(add, rm, list)
	return c
}

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <id>",
		Short: "Open a ticket in $EDITOR and validate on save",
		Args:  exactArgs(1, "edit <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			t, err := ix.Resolve(args[0])
			if err != nil {
				return err
			}
			if err := editTicket(s, t.ID); err != nil {
				return err
			}
			ix, err = s.Index()
			if err != nil {
				return err
			}
			t = ix.Get(t.ID)
			return out(view(ix, t, false), t.ID+": saved\n")
		},
	}
}

// editTicket copies the ticket to a temp file, runs the editor, validates the
// result, then writes it back under the lock. The lock is not held while the
// editor is open.
func editTicket(s *store.Store, id string) error {
	orig, err := s.Load(id)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(orig.Path)
	if err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "bored-edit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	tmp := filepath.Join(tmpDir, id+".md")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	editor := s.Cfg.Editor
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], tmp)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %s: %w", editor, err)
	}
	edited, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	if string(edited) == string(data) {
		fmt.Fprintln(os.Stderr, "no changes")
		return nil
	}
	nt, err := store.ParseTicket(edited)
	if err != nil {
		return fmt.Errorf("edited file rejected: %w (original left untouched)", err)
	}
	if nt.ID != id {
		return fmt.Errorf("%w: id changed from %s to %s; ids are fixed", model.ErrUsage, id, nt.ID)
	}
	return s.Mutate("bored: edit "+id, func(tx *store.Tx) error {
		cur, err := tx.Resolve(id)
		if err != nil {
			return err
		}
		if nt.Slug != cur.Slug {
			if err := tx.CheckSlugFree(nt.Slug, id); err != nil {
				return err
			}
		}
		if nt.Parent != "" {
			if _, err := tx.Resolve(nt.Parent); err != nil {
				return err
			}
			if tx.Index().WouldCycleParent(id, nt.Parent) {
				return fmt.Errorf("%w: parent %s is inside %s's subtree", model.ErrConflict, nt.Parent, id)
			}
		}
		if nt.Repo != "" {
			if _, err := s.Repo(nt.Repo); err != nil {
				return err
			}
		}
		if err := nt.Validate(); err != nil {
			return err
		}
		nt.Created = cur.Created
		nt.Path = cur.Path
		tx.Put(nt)
		return nil
	})
}

var _ = render.IsTTY
