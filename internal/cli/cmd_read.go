package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/render"
	"github.com/Commonwealth-Labs/bored/internal/store"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var status, under, repo, assignee string
	var labels []string
	var all, full bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List tickets (done hidden unless --all or --status done)",
		Args:  exactArgs(0, "list [--status s] [--under x] [--repo r] [--label l] [--assignee a] [--all] [--full]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			sc, err := scopeFromFlags(s, ix, under, repo, labels, status, assignee)
			if err != nil {
				return err
			}
			ts := ix.Filter(sc)
			if !all && sc.Status == "" {
				kept := ts[:0]
				for _, t := range ts {
					if t.Status != model.StatusDone {
						kept = append(kept, t)
					}
				}
				ts = kept
			}
			return out(views(ix, ts, full), render.Table(ix, ts))
		},
	}
	c.Flags().StringVar(&status, "status", "", "filter by status")
	c.Flags().StringVar(&under, "under", "", "only this ticket's subtree (id or slug)")
	c.Flags().StringVar(&repo, "repo", "", "only this repo, plus repo-less tickets under the same roots")
	c.Flags().StringArrayVar(&labels, "label", nil, "require label (repeatable)")
	c.Flags().StringVar(&assignee, "assignee", "", "filter by assignee")
	c.Flags().BoolVar(&all, "all", false, "include done tickets")
	c.Flags().BoolVar(&full, "full", false, "include bodies in JSON")
	return c
}

type treeNode struct {
	ticketView
	Kids []treeNode `json:"children_tree"`
}

func buildTree(ix *model.Index, t *model.Ticket) treeNode {
	n := treeNode{ticketView: view(ix, t, false), Kids: []treeNode{}}
	for _, c := range ix.Children(t.ID) {
		n.Kids = append(n.Kids, buildTree(ix, c))
	}
	return n
}

func newTreeCmd() *cobra.Command {
	var all, long bool
	c := &cobra.Command{
		Use:   "tree [id|slug]",
		Short: "Show the hierarchy with progress per container",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("%w: tree [id|slug]", model.ErrUsage)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			var roots []*model.Ticket
			if len(args) == 1 {
				t, err := ix.Resolve(args[0])
				if err != nil {
					return err
				}
				roots = []*model.Ticket{t}
			} else {
				for _, r := range ix.Roots() {
					if all || r.Status != model.StatusDone {
						roots = append(roots, r)
					}
				}
			}
			nodes := make([]treeNode, 0, len(roots))
			for _, r := range roots {
				nodes = append(nodes, buildTree(ix, r))
			}
			text := render.Tree(ix, roots, long)
			if text == "" {
				text = "(no tickets)\n"
			}
			return out(nodes, text)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "include done roots")
	c.Flags().BoolVarP(&long, "long", "l", false, "show each ticket's description and AC count")
	return c
}

func newShowCmd() *cobra.Command {
	var plain bool
	c := &cobra.Command{
		Use:   "show <id|slug>",
		Short: "Show a ticket",
		Args:  exactArgs(1, "show <id|slug> [--plain]"),
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
			if g.json {
				return out(view(ix, t, true), "")
			}
			header := render.Header(ix, t)
			body := t.Body
			if !plain && render.IsTTY(os.Stdout) {
				body = render.Markdown(t.Body, 100)
			}
			return out(nil, header+"\n"+body)
		},
	}
	c.Flags().BoolVar(&plain, "plain", false, "print raw markdown even on a terminal")
	return c
}

func runBoard(cmd *cobra.Command, under, repo string, labels []string, allLevels bool) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	ix, err := s.Index()
	if err != nil {
		return err
	}
	sc, err := scopeFromFlags(s, ix, under, repo, labels, "", "")
	if err != nil {
		return err
	}
	o := render.BoardOpts{AllLevels: allLevels, Now: time.Now(), DoneWindow: time.Duration(s.Cfg.DoneWindowDays) * 24 * time.Hour, MarkRepo: repo}
	if g.json {
		cols := render.Columns(ix, sc, o)
		j := map[string][]ticketView{}
		for _, st := range model.AllStatuses {
			j[string(st)] = views(ix, cols[st], false)
		}
		return out(j, "")
	}
	return out(nil, render.Board(ix, sc, o))
}

func newBoardCmd() *cobra.Command {
	var under, repo string
	var labels []string
	var allLevels bool
	c := &cobra.Command{
		Use:   "board",
		Short: "Print the board: workable tickets by status",
		Args:  exactArgs(0, "board [--under x] [--repo r] [--label l] [--all-levels]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoard(cmd, under, repo, labels, allLevels)
		},
	}
	c.Flags().StringVar(&under, "under", "", "only this ticket's subtree (id or slug)")
	c.Flags().StringVar(&repo, "repo", "", "only this repo (marks its tickets with *)")
	c.Flags().StringArrayVar(&labels, "label", nil, "require label (repeatable)")
	c.Flags().BoolVar(&allLevels, "all-levels", false, "include tickets that have open children")
	return c
}

func newReadyCmd() *cobra.Command {
	var under, repo string
	c := &cobra.Command{
		Use:   "ready",
		Short: "Tickets that can be claimed now, best first",
		Args:  exactArgs(0, "ready [--under x] [--repo r]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			sc, err := scopeFromFlags(s, ix, under, repo, nil, "", "")
			if err != nil {
				return err
			}
			ts := ix.Ready(sc, actor(s))
			if len(ts) == 0 {
				return fmt.Errorf("%w: no ready tickets", model.ErrNothing)
			}
			return out(views(ix, ts, false), render.Table(ix, ts))
		},
	}
	c.Flags().StringVar(&under, "under", "", "only this ticket's subtree (id or slug)")
	c.Flags().StringVar(&repo, "repo", "", "only this repo, plus repo-less tickets under the same roots")
	return c
}

func newNextCmd() *cobra.Command {
	var under, repo string
	c := &cobra.Command{
		Use:   "next",
		Short: "The one ticket to work on now, with a reason",
		Args:  exactArgs(0, "next [--under x] [--repo r] [--as actor]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			sc, err := scopeFromFlags(s, ix, under, repo, nil, "", "")
			if err != nil {
				return err
			}
			t, reason := ix.Next(sc, actor(s), time.Now())
			if t == nil {
				return fmt.Errorf("%w: nothing ready", model.ErrNothing)
			}
			return out(map[string]any{"ticket": view(ix, t, false), "reason": reason},
				render.Card(ix, t, "")+"\n  "+reason+"\n")
		},
	}
	c.Flags().StringVar(&under, "under", "", "only this ticket's subtree (id or slug)")
	c.Flags().StringVar(&repo, "repo", "", "only this repo, plus repo-less tickets under the same roots")
	return c
}

func newPrimeCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "prime",
		Short: "Print workflow instructions and the board for an agent's context",
		Args:  exactArgs(0, "prime"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			who := g.actor
			if who == "" {
				who = os.Getenv("BORED_ACTOR")
			}
			if who == "" {
				who = "claude"
			}
			cwd, _ := os.Getwd()
			var sc model.Scope
			var sb strings.Builder
			sb.WriteString("# bored — cross-project kanban (store: " + s.Root + ")\n")
			repo, derr := s.DetectRepo(cwd)
			if derr == nil {
				sc.Repo = repo.Name
				roots := map[string]bool{}
				var names []string
				for _, t := range ix.All() {
					if t.Repo == repo.Name && t.Status != model.StatusDone {
						r := ix.RootOf(t.ID)
						if !roots[r.ID] {
							roots[r.ID] = true
							names = append(names, fmt.Sprintf("%s (%s)", r.Ref(), r.ID))
						}
					}
				}
				sb.WriteString(fmt.Sprintf("This directory: repo %s (%s).", repo.Name, repo.Path))
				if len(names) > 0 {
					sb.WriteString(" Initiatives with work here: " + strings.Join(names, ", "))
				} else {
					sb.WriteString(" No open tickets name this repo yet.")
				}
				sb.WriteString("\n")
			} else {
				sb.WriteString("This directory is not a registered repo; showing everything.\n")
			}
			sb.WriteString("Workflow: backlog -> todo (has AC, agent-ready) -> doing (claim) -> review (human gate) -> done\n")
			sb.WriteString("A ticket with open children is a container: work the children, not it.\n")
			sb.WriteString("Use the CLI, never edit ticket files directly:\n")
			sb.WriteString(fmt.Sprintf("  bored next --as %s --json | bored show ID --json | bored claim ID --as %s | bored log ID \"..\" --as %s\n", who, who, who))
			sb.WriteString("  bored ac ID check N | bored move ID review -m \"summary; branch X\" | bored new \"title\" --parent ID --status backlog\n")
			sb.WriteString("Never move a ticket to done; that is the human's call.\n")
			sb.WriteString("Skills: /bored-next /bored-work /bored-review /bored-plan\n\n")
			title := "## Board"
			if sc.Repo != "" {
				title += " (repo " + sc.Repo + " and its initiatives; * = this repo)"
			}
			sb.WriteString(title + "\n")
			o := render.BoardOpts{Now: time.Now(), DoneWindow: 24 * time.Hour, MarkRepo: sc.Repo}
			cols := render.Columns(ix, sc, o)
			for _, st := range []model.Status{model.StatusDoing, model.StatusReview, model.StatusTodo} {
				ts := cols[st]
				if len(ts) == 0 {
					continue
				}
				sb.WriteString(string(st) + ":\n")
				for _, t := range ts {
					mark := ""
					if sc.Repo != "" && t.Repo == sc.Repo {
						mark = "*"
					}
					sb.WriteString("  " + render.Card(ix, t, mark) + "\n")
				}
			}
			if n := len(cols[model.StatusBacklog]); n > 0 {
				sb.WriteString(fmt.Sprintf("backlog: %d unrefined ticket(s); see bored list --status backlog\n", n))
			}
			if t, reason := ix.Next(sc, who, time.Now()); t != nil {
				sb.WriteString(fmt.Sprintf("Next for %s: %s — %s\n", who, t.ID, reason))
			} else {
				sb.WriteString(fmt.Sprintf("Next for %s: nothing ready\n", who))
			}
			if g.json {
				j := map[string]any{"root": s.Root, "actor": who, "text": sb.String()}
				if derr == nil {
					j["repo"] = repoJSON(repo)
				}
				return out(j, "")
			}
			return out(nil, sb.String())
		},
	}
	return c
}

// unused guard to keep store import if views change
var _ = store.CountAC
