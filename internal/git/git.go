// Package git is a thin wrapper over the git binary for the store repo and
// for detecting which registered repo a directory belongs to.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// Available reports whether a git binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// Init creates a repository with the given initial branch.
func Init(dir, branch string) error {
	_, err := run(dir, "init", "-q", "-b", branch)
	return err
}

// HasChanges reports whether the work tree has uncommitted changes.
func HasChanges(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// CommitAll stages everything and commits. Returns false if there was
// nothing to commit.
func CommitAll(dir, msg string) (bool, error) {
	changed, err := HasChanges(dir)
	if err != nil || !changed {
		return false, err
	}
	if _, err := run(dir, "add", "-A"); err != nil {
		return false, err
	}
	if _, err := run(dir, "commit", "-q", "-m", msg); err != nil {
		return false, err
	}
	return true, nil
}

// Push pushes the current branch if a remote named origin exists.
func Push(dir string) error {
	if _, err := run(dir, "remote", "get-url", "origin"); err != nil {
		return errors.New("no remote named origin")
	}
	_, err := run(dir, "push", "-q")
	return err
}

// CommitCount returns the number of commits on HEAD, 0 for an empty repo.
func CommitCount(dir string) int {
	out, err := run(dir, "rev-list", "--count", "HEAD")
	if err != nil {
		return 0
	}
	n := 0
	fmt.Sscanf(out, "%d", &n)
	return n
}

// Toplevels returns the work tree root for dir and, for a linked worktree,
// the main work tree root as well. Both are symlink-resolved absolute paths.
func Toplevels(dir string) ([]string, error) {
	top, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	var out []string
	if p, err := canonical(top); err == nil {
		out = append(out, p)
	}
	common, err := run(dir, "rev-parse", "--git-common-dir")
	if err == nil {
		if !filepath.IsAbs(common) {
			common = filepath.Join(dir, common)
		}
		// common dir is <main>/.git for worktrees; its parent is the main work tree.
		if filepath.Base(common) == ".git" {
			if p, err := canonical(filepath.Dir(common)); err == nil && (len(out) == 0 || p != out[0]) {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// Canonical resolves symlinks and returns an absolute path.
func canonical(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r, nil
	}
	return abs, nil
}

// Canonical is exported for the store's repo matching.
func Canonical(p string) (string, error) { return canonical(p) }

// Exists reports whether path exists.
func Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
