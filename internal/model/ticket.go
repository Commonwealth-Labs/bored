// Package model holds the ticket type and the pure rules derived from the
// ticket graph: hierarchy, blocking, readiness and transitions.
package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Status is a ticket's column on the board.
type Status string

const (
	StatusBacklog Status = "backlog"
	StatusTodo    Status = "todo"
	StatusDoing   Status = "doing"
	StatusReview  Status = "review"
	StatusDone    Status = "done"
)

// AllStatuses lists statuses in board order.
var AllStatuses = []Status{StatusBacklog, StatusTodo, StatusDoing, StatusReview, StatusDone}

// ParseStatus accepts a status name case-insensitively.
func ParseStatus(s string) (Status, error) {
	st := Status(strings.ToLower(strings.TrimSpace(s)))
	for _, known := range AllStatuses {
		if st == known {
			return st, nil
		}
	}
	return "", fmt.Errorf("%w: unknown status %q (want one of %s)", ErrUsage, s, JoinStatuses())
}

// JoinStatuses renders the status list for help text.
func JoinStatuses() string {
	parts := make([]string, len(AllStatuses))
	for i, s := range AllStatuses {
		parts[i] = string(s)
	}
	return strings.Join(parts, ", ")
}

// Index of a status in board order, or -1.
func (s Status) Index() int {
	for i, known := range AllStatuses {
		if s == known {
			return i
		}
	}
	return -1
}

// Sentinel errors. The CLI maps these to exit codes.
var (
	ErrUsage    = errors.New("usage")     // exit 2
	ErrNotFound = errors.New("not found") // exit 3
	ErrConflict = errors.New("conflict")  // exit 4
	ErrNothing  = errors.New("nothing")   // exit 5
)

// Ticket is the single construct. Hierarchy comes from Parent; everything
// else about structure (children, workable, blocked) is derived by Index.
type Ticket struct {
	ID        string     `yaml:"id"`
	Title     string     `yaml:"title"`
	Status    Status     `yaml:"status"`
	Parent    string     `yaml:"parent,omitempty"`
	Slug      string     `yaml:"slug,omitempty"`
	Repo      string     `yaml:"repo,omitempty"`
	Priority  int        `yaml:"priority"`
	DependsOn []string   `yaml:"depends_on,flow"`
	Labels    []string   `yaml:"labels,flow"`
	Assignee  string     `yaml:"assignee"`
	Created   time.Time  `yaml:"created"`
	Updated   time.Time  `yaml:"updated"`
	ClaimedAt *time.Time `yaml:"claimed_at,omitempty"`
	Branch    string     `yaml:"branch,omitempty"`

	Body string `yaml:"-"` // markdown after the frontmatter, stored verbatim
	Path string `yaml:"-"` // absolute path on disk, set by the store
}

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidSlug reports whether s can be used as a slug. Slugs must not look
// like ticket IDs or bare numbers so that references stay unambiguous.
func ValidSlug(s string) bool {
	if !slugRe.MatchString(s) {
		return false
	}
	if LooksLikeID(s) {
		return false
	}
	return true
}

var idRe = regexp.MustCompile(`^(?i)([a-z]+-)?\d+$`)

// LooksLikeID reports whether ref is a bare number or PREFIX-number.
func LooksLikeID(ref string) bool {
	return idRe.MatchString(strings.TrimSpace(ref))
}

// NormalizeID turns "12", "brd-12" or "BRD-12" into "BRD-12" for the given
// prefix. It returns ok=false when ref is not ID-shaped.
func NormalizeID(prefix, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if !LooksLikeID(ref) {
		return "", false
	}
	if i := strings.IndexByte(ref, '-'); i >= 0 {
		if !strings.EqualFold(ref[:i], prefix) {
			return "", false
		}
		ref = ref[i+1:]
	}
	ref = strings.TrimLeft(ref, "0")
	if ref == "" {
		ref = "0"
	}
	return prefix + "-" + ref, true
}

// Validate checks the fields that must always hold.
func (t *Ticket) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("%w: ticket has no id", ErrUsage)
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("%w: %s has an empty title", ErrUsage, t.ID)
	}
	if t.Status.Index() < 0 {
		return fmt.Errorf("%w: %s has unknown status %q", ErrUsage, t.ID, t.Status)
	}
	if t.Priority < 1 || t.Priority > 4 {
		return fmt.Errorf("%w: %s priority must be 1-4, got %d", ErrUsage, t.ID, t.Priority)
	}
	if t.Slug != "" && !ValidSlug(t.Slug) {
		return fmt.Errorf("%w: %s slug %q must be lowercase letters, digits and hyphens, and not look like an id", ErrUsage, t.ID, t.Slug)
	}
	if t.Parent == t.ID {
		return fmt.Errorf("%w: %s cannot be its own parent", ErrUsage, t.ID)
	}
	for _, d := range t.DependsOn {
		if d == t.ID {
			return fmt.Errorf("%w: %s cannot depend on itself", ErrUsage, t.ID)
		}
	}
	return nil
}

// LabelStuck marks a ticket an agent gave up on; agents skip it until a
// human removes the label.
const LabelStuck = "stuck"

// IsStuck reports whether the ticket carries the stuck label.
func (t *Ticket) IsStuck() bool { return t.HasLabel(LabelStuck) }

// AddLabel adds l if missing; reports whether it changed anything.
func (t *Ticket) AddLabel(l string) bool {
	if t.HasLabel(l) {
		return false
	}
	t.Labels = append(t.Labels, l)
	return true
}

// RemoveLabel removes l; reports whether it was present.
func (t *Ticket) RemoveLabel(l string) bool {
	kept := t.Labels[:0]
	found := false
	for _, x := range t.Labels {
		if x == l {
			found = true
			continue
		}
		kept = append(kept, x)
	}
	t.Labels = kept
	return found
}

// HasLabel reports whether the ticket carries label l.
func (t *Ticket) HasLabel(l string) bool {
	for _, x := range t.Labels {
		if x == l {
			return true
		}
	}
	return false
}

// Ref is the ticket's slug if it has one, else its ID.
func (t *Ticket) Ref() string {
	if t.Slug != "" {
		return t.Slug
	}
	return t.ID
}
