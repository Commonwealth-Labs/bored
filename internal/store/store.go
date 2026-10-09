// Package store reads and writes the ticket files under $BORED_HOME and
// wraps every mutation in a lock + atomic write + git commit.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Commonwealth-Labs/bored/internal/git"
	"github.com/Commonwealth-Labs/bored/internal/model"
)

// Store is an opened $BORED_HOME.
type Store struct {
	Root     string
	Cfg      Config
	Repos    map[string]*model.Repo
	NoCommit bool // set by --no-commit; sync commits later
}

// DefaultRoot is $BORED_HOME or ~/.bored.
func DefaultRoot() string {
	if h := os.Getenv("BORED_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".bored"
	}
	return filepath.Join(home, ".bored")
}

func (s *Store) ticketsDir() string { return filepath.Join(s.Root, "tickets") }
func (s *Store) configPath() string { return filepath.Join(s.Root, "config.yaml") }
func (s *Store) reposPath() string  { return filepath.Join(s.Root, "repos.yaml") }
func (s *Store) lockPath() string   { return filepath.Join(s.Root, ".lock") }

// ErrNotInitialised is returned by Open when the root has no config.
var ErrNotInitialised = errors.New("store not initialised")

// Open loads config and repos from root.
func Open(root string) (*Store, error) {
	s := &Store{Root: root}
	cfg, err := loadConfig(s.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s has no config.yaml (run: bored init)", ErrNotInitialised, root)
		}
		return nil, err
	}
	s.Cfg = cfg
	repos, err := loadRepos(s.reposPath())
	if err != nil {
		return nil, err
	}
	s.Repos = repos
	if err := os.MkdirAll(s.ticketsDir(), 0o755); err != nil {
		return nil, err
	}
	return s, nil
}

// Init creates a store at root: config, repos, tickets dir, git repo with a
// first commit. It refuses to overwrite an existing config.
func Init(root, prefix string) (*Store, error) {
	s := &Store{Root: root}
	if _, err := os.Stat(s.configPath()); err == nil {
		return nil, fmt.Errorf("%w: %s is already initialised", model.ErrConflict, root)
	}
	if err := os.MkdirAll(s.ticketsDir(), 0o755); err != nil {
		return nil, err
	}
	s.Cfg = DefaultConfig(prefix)
	if err := saveConfig(s.configPath(), s.Cfg); err != nil {
		return nil, err
	}
	s.Repos = map[string]*model.Repo{}
	if err := saveRepos(s.reposPath(), s.Repos); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(s.Root, ".gitignore"), []byte(".lock\n*.tmp\n"), 0o644); err != nil {
		return nil, err
	}
	// Keep the tickets dir in git even while empty.
	if err := os.WriteFile(filepath.Join(s.ticketsDir(), ".gitkeep"), nil, 0o644); err != nil {
		return nil, err
	}
	if git.Available() && !git.IsRepo(root) {
		if err := git.Init(root, "main"); err != nil {
			return nil, err
		}
		if _, err := git.CommitAll(root, "bored: init"); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// SaveConfig persists Cfg.
func (s *Store) SaveConfig() error { return saveConfig(s.configPath(), s.Cfg) }

// Commit commits any outstanding changes in the store (used by sync and by
// Mutate). Returns whether a commit was made.
func (s *Store) Commit(msg string) (bool, error) {
	if !git.Available() || !git.IsRepo(s.Root) {
		return false, nil
	}
	return git.CommitAll(s.Root, msg)
}

// Push pushes the store repo.
func (s *Store) Push() error { return git.Push(s.Root) }

// HasUncommitted reports uncommitted changes in the store repo.
func (s *Store) HasUncommitted() bool {
	if !git.Available() || !git.IsRepo(s.Root) {
		return false
	}
	ok, _ := git.HasChanges(s.Root)
	return ok
}
