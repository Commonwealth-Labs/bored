package store

import (
	"fmt"
	"os"
	"sort"

	"github.com/Commonwealth-Labs/bored/internal/git"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"go.yaml.in/yaml/v3"
)

type reposFile struct {
	Repos map[string]*model.Repo `yaml:"repos"`
}

func loadRepos(path string) (map[string]*model.Repo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*model.Repo{}, nil
		}
		return nil, err
	}
	var f reposFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Repos == nil {
		f.Repos = map[string]*model.Repo{}
	}
	for name, r := range f.Repos {
		r.Name = name
	}
	return f.Repos, nil
}

func saveRepos(path string, repos map[string]*model.Repo) error {
	b, err := yaml.Marshal(reposFile{Repos: repos})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// RepoList returns repos sorted by name.
func (s *Store) RepoList() []*model.Repo {
	out := make([]*model.Repo, 0, len(s.Repos))
	for _, r := range s.Repos {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Repo returns a registered repo by name.
func (s *Store) Repo(name string) (*model.Repo, error) {
	if r, ok := s.Repos[name]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("%w: no repo named %q (see: bored repo list)", model.ErrNotFound, name)
}

// AddRepo registers a path. The path must be absolute but need not exist yet.
func (s *Store) AddRepo(name, path, branch string) (*model.Repo, error) {
	if _, exists := s.Repos[name]; exists {
		return nil, fmt.Errorf("%w: repo %q already registered", model.ErrConflict, name)
	}
	if !model.ValidSlug(name) {
		return nil, fmt.Errorf("%w: repo name %q must be lowercase letters, digits and hyphens", model.ErrUsage, name)
	}
	abs, err := git.Canonical(path)
	if err != nil {
		return nil, err
	}
	r := &model.Repo{Name: name, Path: abs, Branch: branch}
	s.Repos[name] = r
	return r, saveRepos(s.reposPath(), s.Repos)
}

// SetRepo updates fields; empty strings leave a field alone.
func (s *Store) SetRepo(name, path, branch string) (*model.Repo, error) {
	r, err := s.Repo(name)
	if err != nil {
		return nil, err
	}
	if path != "" {
		abs, err := git.Canonical(path)
		if err != nil {
			return nil, err
		}
		r.Path = abs
	}
	if branch != "" {
		r.Branch = branch
	}
	return r, saveRepos(s.reposPath(), s.Repos)
}

// DetectRepo finds the registered repo whose path is the work tree containing
// dir. Linked git worktrees resolve to the main work tree's repo.
func (s *Store) DetectRepo(dir string) (*model.Repo, error) {
	tops, err := git.Toplevels(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %s is not inside a git work tree", model.ErrNotFound, dir)
	}
	for _, top := range tops {
		for _, r := range s.RepoList() {
			rp, err := git.Canonical(r.Path)
			if err != nil {
				continue
			}
			if rp == top {
				return r, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: no registered repo for %s (see: bored repo add)", model.ErrNotFound, tops[0])
}
