package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/runner"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var o runner.Options
	var under, repo string
	var noWorktree bool
	c := &cobra.Command{
		Use:   "run",
		Short: "Hand ready tickets to headless Claude Code sessions, one at a time",
		Long: `For each ready ticket (bored next --as <actor>) spawn
  claude -p "/bored-work <id>" --output-format json --permission-mode <mode> ...
in the right place: a per-ticket git worktree under the store for tickets
that name a repo, or the current directory for thinking work. Afterwards the
ticket is reloaded: review means the agent handed over; a ticket left in
doing is released back to todo with a log line. Output and results for every
session are kept under <store>/runs/. Stops when nothing is ready, after
--max tickets, or after two consecutive failures.`,
		Args: exactArgs(0, "run [--under x] [--repo r] [--max N] [--review] [--dry-run] ..."),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ix, err := s.Index()
			if err != nil {
				return err
			}
			o.Scope, err = scopeFromFlags(s, ix, under, repo, nil, "", "")
			if err != nil {
				return err
			}
			if o.Actor == "" {
				o.Actor = "claude"
				if g.actor != "" {
					o.Actor = g.actor
				}
			}
			o.Worktree = !noWorktree
			o.Cwd, _ = os.Getwd()
			if !g.json {
				o.Out = os.Stdout
			}
			results, rerr := runner.Run(s, o)
			if g.json {
				if results == nil {
					results = []runner.Result{}
				}
				out(map[string]any{"results": results, "error": errString(rerr)}, "")
			} else if len(results) > 0 {
				fmt.Printf("%d ticket(s): ", len(results))
				total := 0.0
				for i, r := range results {
					if i > 0 {
						fmt.Print(", ")
					}
					fmt.Printf("%s %s", r.ID, r.Outcome)
					total += r.CostUSD
				}
				fmt.Printf("  ($%.2f)\n", total)
			}
			switch {
			case errors.Is(rerr, runner.ErrNothingReady):
				return fmt.Errorf("%w: nothing ready", model.ErrNothing)
			case rerr != nil:
				return rerr
			}
			return nil
		},
	}
	c.Flags().StringVar(&under, "under", "", "only this ticket's subtree (id or slug)")
	c.Flags().StringVar(&repo, "repo", "", "only this repo, plus repo-less tickets under the same roots")
	c.Flags().IntVar(&o.Max, "max", 1, "tickets to attempt (0 = until nothing is ready)")
	c.Flags().IntVar(&o.MaxTurns, "max-turns", 60, "claude --max-turns per session")
	c.Flags().Float64Var(&o.BudgetUSD, "budget-usd", 0, "claude --max-budget-usd per session (0 = none)")
	c.Flags().StringVar(&o.PermissionMode, "permission-mode", "acceptEdits", "claude --permission-mode")
	c.Flags().BoolVar(&noWorktree, "no-worktree", false, "run in the repo path instead of a per-ticket worktree")
	c.Flags().BoolVar(&o.Review, "review", false, "after a successful run, have a second session review it and log the verdict")
	c.Flags().BoolVar(&o.DryRun, "dry-run", false, "print the pick and where it would run, then stop")
	c.Flags().StringVar(&o.ClaudeBin, "claude", "claude", "claude binary")
	return c
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
