package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/model"
)

// maxID scans the tickets dir for the highest number in use.
func (s *Store) maxID() (int, error) {
	entries, err := os.ReadDir(s.ticketsDir())
	if err != nil {
		return 0, err
	}
	prefix := s.Cfg.IDPrefix + "-"
	max := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".md") {
			continue
		}
		n := model.IDNum(strings.TrimSuffix(name, ".md"))
		if n < 1<<30 && n > max {
			max = n
		}
	}
	return max, nil
}

// allocate claims the next free ID by creating its file with O_EXCL. The
// caller then overwrites it atomically. Runs under the store lock, so the
// O_EXCL retry is belt and braces.
func (s *Store) allocate() (string, error) {
	n, err := s.maxID()
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 1000; attempt++ {
		n++
		id := fmt.Sprintf("%s-%d", s.Cfg.IDPrefix, n)
		f, err := os.OpenFile(s.PathFor(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return id, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("could not allocate a ticket id")
}

// release removes a placeholder created by allocate when the transaction fails.
func (s *Store) release(id string) {
	p := s.PathFor(id)
	if st, err := os.Stat(p); err == nil && st.Size() == 0 {
		os.Remove(p)
	}
	os.Remove(filepath.Join(s.ticketsDir(), "."+id+".md.tmp"))
}
