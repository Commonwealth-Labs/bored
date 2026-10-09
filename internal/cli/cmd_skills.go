package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bored "github.com/Commonwealth-Labs/bored"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/spf13/cobra"
)

func newInstallSkillsCmd() *cobra.Command {
	var dir, repoDir string
	var symlink, force bool
	c := &cobra.Command{
		Use:   "install-skills",
		Short: "Install the Claude Code skills (bored-plan, bored-work, bored-review, bored-next)",
		Long: `Copies the embedded skills into ~/.claude/skills/<name>/SKILL.md. With
--symlink the skill directories are linked to a checkout of this repo instead
(--repo-dir, default: the directory containing the current binary's source is
unknown, so pass it) so edits there are live. Existing skills are left alone
unless --force is given.`,
		Args: exactArgs(0, "install-skills [--dir d] [--symlink --repo-dir r] [--force]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dir = filepath.Join(home, ".claude", "skills")
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			names, err := skillNames()
			if err != nil {
				return err
			}
			type result struct {
				Name   string `json:"name"`
				Path   string `json:"path"`
				Action string `json:"action"`
			}
			var results []result
			var sb strings.Builder
			for _, name := range names {
				target := filepath.Join(dir, name)
				action := "installed"
				if st, err := os.Lstat(target); err == nil {
					if !force {
						results = append(results, result{name, target, "skipped (exists; use --force)"})
						sb.WriteString(fmt.Sprintf("%-14s skipped, %s exists (use --force)\n", name, target))
						continue
					}
					if st.Mode()&os.ModeSymlink != 0 || st.IsDir() {
						if err := os.RemoveAll(target); err != nil {
							return err
						}
					}
					action = "replaced"
				}
				if symlink {
					if repoDir == "" {
						return fmt.Errorf("%w: --symlink needs --repo-dir pointing at a bored checkout", model.ErrUsage)
					}
					src := filepath.Join(repoDir, "skills", name)
					if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
						return fmt.Errorf("%w: %s has no SKILL.md", model.ErrNotFound, src)
					}
					if err := os.Symlink(src, target); err != nil {
						return err
					}
					action += " (symlink)"
				} else {
					data, err := bored.Skills.ReadFile(filepath.Join("skills", name, "SKILL.md"))
					if err != nil {
						return err
					}
					if err := os.MkdirAll(target, 0o755); err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(target, "SKILL.md"), data, 0o644); err != nil {
						return err
					}
				}
				results = append(results, result{name, target, action})
				sb.WriteString(fmt.Sprintf("%-14s %s -> %s\n", name, action, target))
			}
			return out(results, sb.String())
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "skills directory (default ~/.claude/skills)")
	c.Flags().BoolVar(&symlink, "symlink", false, "symlink to a repo checkout instead of copying")
	c.Flags().StringVar(&repoDir, "repo-dir", "", "bored checkout to symlink from (with --symlink)")
	c.Flags().BoolVar(&force, "force", false, "replace existing skill directories")
	return c
}

func skillNames() ([]string, error) {
	entries, err := fs.ReadDir(bored.Skills, "skills")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
