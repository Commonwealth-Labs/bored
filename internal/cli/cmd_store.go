package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/git"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var prefix string
	c := &cobra.Command{
		Use:   "init",
		Short: "Create the store (config, repos.yaml, tickets/, git repo)",
		Args:  exactArgs(0, "init takes no arguments"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Init(storeRoot(), prefix)
			if err != nil {
				return err
			}
			return out(map[string]any{"root": s.Root, "prefix": s.Cfg.IDPrefix, "actor": s.Cfg.Actor},
				fmt.Sprintf("Initialised store at %s (ids %s-n, actor %s)\n", s.Root, s.Cfg.IDPrefix, s.Cfg.Actor))
		},
	}
	c.Flags().StringVar(&prefix, "prefix", "BRD", "ticket id prefix")
	return c
}

func newRepoCmd() *cobra.Command {
	c := &cobra.Command{Use: "repo", Short: "Register and inspect repos (named paths on disk)"}

	var branch, path string
	add := &cobra.Command{
		Use:   "add <name> <path>",
		Short: "Register a repo; the path need not exist yet",
		Args:  exactArgs(2, "repo add <name> <path>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			r, err := s.AddRepo(args[0], args[1], branch)
			if err != nil {
				return err
			}
			if s.Cfg.AutoCommit && !s.NoCommit {
				s.Commit("bored: repo add " + r.Name)
			}
			return out(repoJSON(r), fmt.Sprintf("Registered repo %s -> %s\n", r.Name, r.Path))
		},
	}
	add.Flags().StringVar(&branch, "branch", "main", "default branch")

	set := &cobra.Command{
		Use:   "set <name>",
		Short: "Change a repo's path or branch",
		Args:  exactArgs(1, "repo set <name> [--path p] [--branch b]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			r, err := s.SetRepo(args[0], path, branch)
			if err != nil {
				return err
			}
			if s.Cfg.AutoCommit && !s.NoCommit {
				s.Commit("bored: repo set " + r.Name)
			}
			return out(repoJSON(r), fmt.Sprintf("Updated repo %s -> %s (%s)\n", r.Name, r.Path, r.Branch))
		},
	}
	set.Flags().StringVar(&path, "path", "", "new path")
	set.Flags().StringVar(&branch, "branch", "", "new default branch")

	list := &cobra.Command{
		Use:   "list",
		Short: "List registered repos",
		Args:  exactArgs(0, "repo list"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			rs := s.RepoList()
			vs := make([]repoView, 0, len(rs))
			var sb strings.Builder
			for _, r := range rs {
				v := repoJSON(r)
				vs = append(vs, v)
				note := ""
				if !v.Exists {
					note = "  (path does not exist yet)"
				}
				sb.WriteString(fmt.Sprintf("%-16s %s  [%s]%s\n", r.Name, r.Path, r.Branch, note))
			}
			if len(rs) == 0 {
				sb.WriteString("(no repos registered; see: bored repo add)\n")
			}
			return out(vs, sb.String())
		},
	}

	detect := &cobra.Command{
		Use:   "detect",
		Short: "Which registered repo contains the current directory",
		Args:  exactArgs(0, "repo detect"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			cwd, _ := os.Getwd()
			r, err := s.DetectRepo(cwd)
			if err != nil {
				return err
			}
			return out(repoJSON(r), fmt.Sprintf("%s  %s\n", r.Name, r.Path))
		},
	}
	c.AddCommand(add, set, list, detect)
	return c
}

func repoJSON(r *model.Repo) repoView {
	return repoView{Name: r.Name, Path: r.Path, Branch: r.Branch, Exists: git.Exists(r.Path)}
}

func newSyncCmd() *cobra.Command {
	var msg string
	var push bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "Commit outstanding changes in the store (and optionally push)",
		Args:  exactArgs(0, "sync [-m msg] [--push]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			if msg == "" {
				msg = "bored: sync"
			}
			committed, err := s.Commit(msg)
			if err != nil {
				return err
			}
			pushed := false
			if push {
				if err := s.Push(); err != nil {
					return err
				}
				pushed = true
			}
			text := "nothing to commit\n"
			if committed {
				text = "committed: " + msg + "\n"
			}
			if pushed {
				text += "pushed\n"
			}
			return out(map[string]any{"committed": committed, "pushed": pushed}, text)
		},
	}
	c.Flags().StringVarP(&msg, "message", "m", "", "commit message")
	c.Flags().BoolVar(&push, "push", false, "push to origin after committing")
	return c
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  exactArgs(0, "version"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return out(map[string]string{"version": Version}, "bored "+Version+"\n")
		},
	}
}
