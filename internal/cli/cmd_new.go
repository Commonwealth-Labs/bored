package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	var (
		parent, slug, repo, status, description string
		priority                                int
		dependsOn, labels, ac                   []string
		fromStdin, edit                         bool
	)
	c := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a ticket (prints its id)",
		Long: `Create a ticket. With no --parent it is a new root (an initiative); give it
a --slug so it can be named in commands. Status defaults to backlog, or todo
when any --ac is given. --repo defaults to the parent's repo.`,
		Args: minArgs(1, "new <title words...>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			title := strings.TrimSpace(strings.Join(args, " "))
			var created *model.Ticket
			var ix *model.Index
			err = s.Mutate("", func(tx *store.Tx) error {
				ix = tx.Index()
				t := &model.Ticket{Title: title, Slug: slug, Priority: priority, Labels: labels, DependsOn: []string{}}
				if parent != "" {
					p, err := tx.Resolve(parent)
					if err != nil {
						return err
					}
					t.Parent = p.ID
					if repo == "" {
						t.Repo = p.Repo
					}
				}
				if repo == "none" {
					t.Repo = ""
				} else if repo != "" {
					if _, err := s.Repo(repo); err != nil {
						return err
					}
					t.Repo = repo
				}
				for _, d := range dependsOn {
					for _, ref := range strings.Split(d, ",") {
						if strings.TrimSpace(ref) == "" {
							continue
						}
						b, err := tx.Resolve(ref)
						if err != nil {
							return err
						}
						t.DependsOn = append(t.DependsOn, b.ID)
					}
				}
				if status != "" {
					st, err := model.ParseStatus(status)
					if err != nil {
						return err
					}
					if st != model.StatusBacklog && st != model.StatusTodo {
						return fmt.Errorf("%w: new tickets start in backlog or todo", model.ErrUsage)
					}
					t.Status = st
				} else if len(ac) > 0 {
					t.Status = model.StatusTodo
				} else {
					t.Status = model.StatusBacklog
				}
				if fromStdin {
					b, err := io.ReadAll(os.Stdin)
					if err != nil {
						return err
					}
					t.Body = string(b)
				} else {
					t.Body = store.NewBody(description, ac)
				}
				if err := tx.Create(t); err != nil {
					return err
				}
				tx.CommitMsg = fmt.Sprintf("bored: new %s %q", t.ID, t.Title)
				created = t
				return nil
			})
			if err != nil {
				return err
			}
			if edit {
				if err := editTicket(s, created.ID); err != nil {
					return err
				}
			}
			ix, err = s.Index()
			if err != nil {
				return err
			}
			t := ix.Get(created.ID)
			return out(view(ix, t, false), t.ID+"\n")
		},
	}
	c.Flags().StringVar(&parent, "parent", "", "parent ticket (id or slug)")
	c.Flags().StringVar(&slug, "slug", "", "short name usable wherever an id is")
	c.Flags().StringVar(&repo, "repo", "", "registered repo this ticket's code work happens in (default: the parent's; 'none' to clear)")
	c.Flags().IntVar(&priority, "priority", 3, "1 (highest) to 4")
	c.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "blockers (ids or slugs, comma-separated or repeated)")
	c.Flags().StringArrayVar(&labels, "label", nil, "label (repeatable)")
	c.Flags().StringVar(&status, "status", "", "backlog or todo (default backlog, or todo when --ac given)")
	c.Flags().StringVar(&description, "description", "", "description text")
	c.Flags().StringArrayVar(&ac, "ac", nil, "acceptance criterion (repeatable)")
	c.Flags().BoolVar(&fromStdin, "body-from-stdin", false, "read the whole markdown body from stdin")
	c.Flags().BoolVar(&edit, "edit", false, "open the new ticket in $EDITOR")
	return c
}

func newSetCmd() *cobra.Command {
	var title, parent, slug, repo, assignee, branch, description, plan string
	var priority int
	c := &cobra.Command{
		Use:   "set <id>",
		Short: "Edit fields or body sections without an editor (use 'none' to clear parent, repo, slug, assignee or branch)",
		Args:  exactArgs(1, "set <id> [--title t] [--parent id|none] [--slug s|none] [--repo r|none] [--priority n] [--assignee a|none] [--branch b|none] [--description text] [--plan text]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			var changed []string
			var id string
			err = s.Mutate("", func(tx *store.Tx) error {
				t, err := tx.Resolve(args[0])
				if err != nil {
					return err
				}
				id = t.ID
				if cmd.Flags().Changed("title") {
					t.Title = strings.TrimSpace(title)
					changed = append(changed, "title")
				}
				if cmd.Flags().Changed("parent") {
					if parent == "none" {
						t.Parent = ""
					} else {
						p, err := tx.Resolve(parent)
						if err != nil {
							return err
						}
						if tx.Index().WouldCycleParent(t.ID, p.ID) {
							return fmt.Errorf("%w: %s is inside %s's subtree", model.ErrConflict, p.ID, t.ID)
						}
						t.Parent = p.ID
					}
					changed = append(changed, "parent")
				}
				if cmd.Flags().Changed("slug") {
					if slug == "none" {
						t.Slug = ""
					} else {
						if err := tx.CheckSlugFree(slug, t.ID); err != nil {
							return err
						}
						t.Slug = slug
					}
					changed = append(changed, "slug")
				}
				if cmd.Flags().Changed("repo") {
					if repo == "none" {
						t.Repo = ""
					} else {
						if _, err := s.Repo(repo); err != nil {
							return err
						}
						t.Repo = repo
					}
					changed = append(changed, "repo")
				}
				if cmd.Flags().Changed("priority") {
					t.Priority = priority
					changed = append(changed, "priority")
				}
				if cmd.Flags().Changed("assignee") {
					if assignee == "none" {
						assignee = ""
					}
					t.Assignee = assignee
					changed = append(changed, "assignee")
				}
				if cmd.Flags().Changed("branch") {
					if branch == "none" {
						branch = ""
					}
					t.Branch = branch
					changed = append(changed, "branch")
				}
				if cmd.Flags().Changed("description") {
					t.Body = store.SetSection(t.Body, store.SecDescription, description)
					changed = append(changed, "description")
				}
				if cmd.Flags().Changed("plan") {
					t.Body = store.SetSection(t.Body, store.SecPlan, plan)
					changed = append(changed, "plan")
				}
				if len(changed) == 0 {
					return fmt.Errorf("%w: nothing to set", model.ErrUsage)
				}
				tx.Put(t)
				tx.CommitMsg = fmt.Sprintf("bored: set %s %s", t.ID, strings.Join(changed, ","))
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
			return out(view(ix, t, false), fmt.Sprintf("%s: set %s\n", t.ID, strings.Join(changed, ", ")))
		},
	}
	c.Flags().StringVar(&title, "title", "", "new title")
	c.Flags().StringVar(&parent, "parent", "", "new parent (id, slug, or none)")
	c.Flags().StringVar(&slug, "slug", "", "new slug (or none)")
	c.Flags().StringVar(&repo, "repo", "", "new repo (or none)")
	c.Flags().IntVar(&priority, "priority", 3, "1 (highest) to 4")
	c.Flags().StringVar(&assignee, "assignee", "", "new assignee (or none)")
	c.Flags().StringVar(&branch, "branch", "", "branch name (or none)")
	c.Flags().StringVar(&description, "description", "", "replace the Description section")
	c.Flags().StringVar(&plan, "plan", "", "replace the Plan section")
	return c
}
