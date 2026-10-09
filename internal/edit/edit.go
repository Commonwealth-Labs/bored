// Package edit implements the $EDITOR round trip for a ticket: copy to a
// temp file, let something run the editor, then validate and write back under
// the store lock. The lock is never held while the editor is open.
package edit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

// Session is one in-progress edit.
type Session struct {
	ID   string
	Tmp  string
	dir  string
	orig []byte
}

// Prepare copies the ticket to a temp file for editing.
func Prepare(s *store.Store, id string) (*Session, error) {
	t, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(t.Path)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "bored-edit-")
	if err != nil {
		return nil, err
	}
	tmp := filepath.Join(dir, id+".md")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &Session{ID: id, Tmp: tmp, dir: dir, orig: data}, nil
}

// Cleanup removes the temp file.
func (e *Session) Cleanup() { os.RemoveAll(e.dir) }

// Editor resolves the editor command from config, $VISUAL, $EDITOR, then vi.
func Editor(cfg store.Config) (string, []string) {
	editor := cfg.Editor
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	return parts[0], parts[1:]
}

// Command builds the editor process for this session, attached to the
// current terminal.
func (e *Session) Command(cfg store.Config) *exec.Cmd {
	bin, args := Editor(cfg)
	cmd := exec.Command(bin, append(args, e.Tmp)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// Finish validates the edited file and writes it back. It returns false when
// nothing changed.
func Finish(s *store.Store, e *Session) (bool, error) {
	edited, err := os.ReadFile(e.Tmp)
	if err != nil {
		return false, err
	}
	if string(edited) == string(e.orig) {
		return false, nil
	}
	nt, err := store.ParseTicket(edited)
	if err != nil {
		return false, fmt.Errorf("edited file rejected: %w (original left untouched)", err)
	}
	if nt.ID != e.ID {
		return false, fmt.Errorf("%w: id changed from %s to %s; ids are fixed", model.ErrUsage, e.ID, nt.ID)
	}
	err = s.Mutate("bored: edit "+e.ID, func(tx *store.Tx) error {
		cur, err := tx.Resolve(e.ID)
		if err != nil {
			return err
		}
		if nt.Slug != cur.Slug {
			if err := tx.CheckSlugFree(nt.Slug, e.ID); err != nil {
				return err
			}
		}
		if nt.Parent != "" {
			if _, err := tx.Resolve(nt.Parent); err != nil {
				return err
			}
			if tx.Index().WouldCycleParent(e.ID, nt.Parent) {
				return fmt.Errorf("%w: parent %s is inside %s's subtree", model.ErrConflict, nt.Parent, e.ID)
			}
		}
		if nt.Repo != "" {
			if _, err := s.Repo(nt.Repo); err != nil {
				return err
			}
		}
		if err := nt.Validate(); err != nil {
			return err
		}
		nt.Created = cur.Created
		nt.Path = cur.Path
		tx.Put(nt)
		return nil
	})
	return err == nil, err
}
