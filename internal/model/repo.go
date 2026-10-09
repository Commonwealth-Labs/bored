package model

// Repo is a registered path on disk. Which tickets belong to it is stored on
// the tickets, not here.
type Repo struct {
	Name   string `yaml:"-"`
	Path   string `yaml:"path"`
	Branch string `yaml:"branch,omitempty"`
}
