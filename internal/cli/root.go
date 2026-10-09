// Package cli wires the bored commands. Every command works in text mode for
// humans and --json mode for agents, and maps errors to stable exit codes.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
	"github.com/spf13/cobra"
)

// Version is set by the Makefile via -ldflags.
var Version = "dev"

// Exit codes.
const (
	ExitOK       = 0
	ExitFailure  = 1
	ExitUsage    = 2
	ExitNotFound = 3
	ExitConflict = 4
	ExitNothing  = 5
)

type globals struct {
	json     bool
	home     string
	actor    string
	noCommit bool
}

var g globals

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	root := newRoot()
	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	code := exitCode(err)
	msg := err.Error()
	if g.json {
		b, _ := json.Marshal(map[string]any{"error": msg, "code": code})
		fmt.Fprintln(os.Stderr, string(b))
	} else {
		fmt.Fprintln(os.Stderr, "error: "+msg)
	}
	return code
}

func exitCode(err error) int {
	switch {
	case errors.Is(err, model.ErrUsage):
		return ExitUsage
	case errors.Is(err, model.ErrNotFound):
		return ExitNotFound
	case errors.Is(err, model.ErrConflict):
		return ExitConflict
	case errors.Is(err, model.ErrNothing):
		return ExitNothing
	}
	s := err.Error()
	if strings.HasPrefix(s, "unknown command") || strings.HasPrefix(s, "unknown flag") || strings.HasPrefix(s, "unknown shorthand") || strings.Contains(s, "accepts ") || strings.HasPrefix(s, "required flag") || strings.HasPrefix(s, "invalid argument") {
		return ExitUsage
	}
	return ExitFailure
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "bored",
		Short: "A cross-project kanban board for you and your coding agents",
		Long: `bored keeps one markdown file per ticket in a central git-tracked store
(default ~/.bored, override with BORED_HOME). Tickets form a tree via parent;
a ticket with open children is a container and is hidden from the board
until its children are done. Every command takes --json for agents.

Exit codes: 0 ok, 1 failure, 2 usage, 3 not found, 4 conflict, 5 nothing to do.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// No TUI yet: fall through to the board.
			return runBoard(cmd, "", "", nil, false)
		},
	}
	root.PersistentFlags().BoolVar(&g.json, "json", false, "machine-readable output")
	root.PersistentFlags().StringVar(&g.home, "home", "", "store directory (default $BORED_HOME or ~/.bored)")
	root.PersistentFlags().StringVar(&g.actor, "as", "", "who is acting (default $BORED_ACTOR, then config actor)")
	root.PersistentFlags().BoolVar(&g.noCommit, "no-commit", false, "write tickets but skip the git commit (use bored sync later)")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return fmt.Errorf("%w: %v", model.ErrUsage, err)
	})
	root.AddCommand(
		newInitCmd(), newRepoCmd(), newNewCmd(), newSetCmd(), newListCmd(), newTreeCmd(), newShowCmd(),
		newMoveCmd(), newClaimCmd(), newDoneCmd(), newNextCmd(), newReadyCmd(), newLogCmd(), newACCmd(),
		newEditCmd(), newDepCmd(), newBoardCmd(), newPrimeCmd(), newSyncCmd(), newVersionCmd(),
	)
	return root
}

func exactArgs(n int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return fmt.Errorf("%w: expected %d argument(s): %s", model.ErrUsage, n, usage)
		}
		return nil
	}
}

func minArgs(n int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("%w: expected at least %d argument(s): %s", model.ErrUsage, n, usage)
		}
		return nil
	}
}

func storeRoot() string {
	if g.home != "" {
		return g.home
	}
	return store.DefaultRoot()
}

func openStore() (*store.Store, error) {
	s, err := store.Open(storeRoot())
	if err != nil {
		return nil, err
	}
	s.NoCommit = g.noCommit
	return s, nil
}

// actor resolves who is acting.
func actor(s *store.Store) string {
	if g.actor != "" {
		return g.actor
	}
	if a := os.Getenv("BORED_ACTOR"); a != "" {
		return a
	}
	return s.Cfg.Actor
}

// out prints v as JSON or the text form.
func out(v any, text string) error {
	if g.json {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Print(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		fmt.Println()
	}
	return nil
}

// scopeFromFlags resolves scope references against the index.
func scopeFromFlags(s *store.Store, ix *model.Index, under, repo string, labels []string, status, assignee string) (model.Scope, error) {
	var sc model.Scope
	if under != "" {
		t, err := ix.Resolve(under)
		if err != nil {
			return sc, err
		}
		sc.Under = t.ID
	}
	if repo != "" {
		if _, err := s.Repo(repo); err != nil {
			return sc, err
		}
		sc.Repo = repo
	}
	if status != "" {
		st, err := model.ParseStatus(status)
		if err != nil {
			return sc, err
		}
		sc.Status = st
	}
	sc.Labels = labels
	sc.Assignee = assignee
	return sc, nil
}
