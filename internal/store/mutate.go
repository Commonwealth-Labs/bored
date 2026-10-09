package store

import (
	"fmt"
	"time"

	"github.com/Commonwealth-Labs/bored/internal/model"
)

// Tx is one locked mutation. Get tickets through the index, change them,
// Put them; everything dirty is written on success.
type Tx struct {
	s       *Store
	ix      *model.Index
	now     time.Time
	dirty   map[string]*model.Ticket
	created []string

	// CommitMsg is the git commit message. Mutate's argument seeds it; fn may
	// overwrite it once ids are known. Empty means no commit.
	CommitMsg string
}

// Index is the snapshot taken under the lock.
func (tx *Tx) Index() *model.Index { return tx.ix }

// Now is the transaction timestamp.
func (tx *Tx) Now() time.Time { return tx.now }

// Store exposes config and repos.
func (tx *Tx) Store() *Store { return tx.s }

// Resolve a reference against the snapshot.
func (tx *Tx) Resolve(ref string) (*model.Ticket, error) { return tx.ix.Resolve(ref) }

// Put marks a ticket for writing.
func (tx *Tx) Put(t *model.Ticket) { tx.dirty[t.ID] = t }

// Create allocates an ID, fills timestamps and defaults, and marks the
// ticket for writing. Slug uniqueness is checked here, inside the lock.
func (tx *Tx) Create(t *model.Ticket) error {
	if t.Slug != "" {
		if other, err := tx.ix.Resolve(t.Slug); err == nil {
			return fmt.Errorf("%w: slug %q is already used by %s", model.ErrUsage, t.Slug, other.ID)
		}
	}
	id, err := tx.s.allocate()
	if err != nil {
		return err
	}
	t.ID = id
	t.Created = tx.now
	t.Updated = tx.now
	if t.Priority == 0 {
		t.Priority = 3
	}
	if t.Status == "" {
		t.Status = model.StatusBacklog
	}
	if err := t.Validate(); err != nil {
		tx.s.release(id)
		return err
	}
	tx.created = append(tx.created, id)
	tx.dirty[id] = t
	return nil
}

// CheckSlugFree errors if slug belongs to a ticket other than id.
func (tx *Tx) CheckSlugFree(slug, id string) error {
	if slug == "" {
		return nil
	}
	if !model.ValidSlug(slug) {
		return fmt.Errorf("%w: slug %q must be lowercase letters, digits and hyphens, and not look like an id", model.ErrUsage, slug)
	}
	if other, err := tx.ix.Resolve(slug); err == nil && other.ID != id {
		return fmt.Errorf("%w: slug %q is already used by %s", model.ErrUsage, slug, other.ID)
	}
	return nil
}

// Mutate runs fn under the store lock, writes dirty tickets atomically, and
// commits unless auto-commit is off or --no-commit was given.
func (s *Store) Mutate(commitMsg string, fn func(tx *Tx) error) error {
	lk, err := s.acquire(5 * time.Second)
	if err != nil {
		return err
	}
	defer lk.release()

	ix, err := s.Index()
	if err != nil {
		return err
	}
	tx := &Tx{s: s, ix: ix, now: time.Now().UTC().Truncate(time.Second), dirty: map[string]*model.Ticket{}, CommitMsg: commitMsg}
	if err := fn(tx); err != nil {
		for _, id := range tx.created {
			s.release(id)
		}
		return err
	}
	for _, t := range tx.dirty {
		t.Updated = tx.now
		if err := s.save(t); err != nil {
			return err
		}
	}
	if len(tx.dirty) == 0 {
		return nil
	}
	if s.Cfg.AutoCommit && !s.NoCommit && tx.CommitMsg != "" {
		if _, err := s.Commit(tx.CommitMsg); err != nil {
			return fmt.Errorf("tickets written but git commit failed: %w", err)
		}
	}
	return nil
}
