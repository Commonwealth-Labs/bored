package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Commonwealth-Labs/bored/internal/model"
	"go.yaml.in/yaml/v3"
)

const fence = "---\n"

// ParseTicket splits frontmatter from body. The body is kept verbatim.
func ParseTicket(data []byte) (*model.Ticket, error) {
	s := string(data)
	if !strings.HasPrefix(s, fence) {
		return nil, fmt.Errorf("%w: file does not start with ---", model.ErrUsage)
	}
	rest := s[len(fence):]
	end := strings.Index(rest, "\n"+fence)
	var front, body string
	if end < 0 {
		// Allow a file that is only frontmatter ending in "---" without trailing newline.
		if strings.HasSuffix(rest, "\n---") {
			front = rest[:len(rest)-len("\n---")]
		} else {
			return nil, fmt.Errorf("%w: frontmatter has no closing ---", model.ErrUsage)
		}
	} else {
		front = rest[:end]
		body = rest[end+1+len(fence):]
	}
	t := &model.Ticket{Priority: 3}
	if err := yaml.Unmarshal([]byte(front), t); err != nil {
		return nil, fmt.Errorf("%w: frontmatter: %v", model.ErrUsage, err)
	}
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	if t.Labels == nil {
		t.Labels = []string{}
	}
	t.Body = strings.TrimPrefix(body, "\n")
	return t, nil
}

// RenderTicket produces the file contents.
func RenderTicket(t *model.Ticket) ([]byte, error) {
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	if t.Labels == nil {
		t.Labels = []string{}
	}
	front, err := yaml.Marshal(t)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(fence)
	buf.Write(front)
	buf.WriteString(fence)
	buf.WriteString("\n")
	body := t.Body
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	buf.WriteString(body)
	return buf.Bytes(), nil
}

// PathFor is the ticket file path for an ID.
func (s *Store) PathFor(id string) string {
	return filepath.Join(s.ticketsDir(), id+".md")
}

func (s *Store) ticketFileRe() *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(s.Cfg.IDPrefix) + `-\d+\.md$`)
}

// List loads every ticket. Files that fail to parse are reported, not skipped silently.
func (s *Store) List() ([]*model.Ticket, error) {
	entries, err := os.ReadDir(s.ticketsDir())
	if err != nil {
		return nil, err
	}
	re := s.ticketFileRe()
	var out []*model.Ticket
	var errs []string
	for _, e := range entries {
		if e.IsDir() || !re.MatchString(e.Name()) {
			continue
		}
		t, err := s.loadPath(filepath.Join(s.ticketsDir(), e.Name()))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return model.IDNum(out[i].ID) < model.IDNum(out[j].ID) })
	if len(errs) > 0 {
		return out, fmt.Errorf("%d ticket file(s) failed to parse:\n  %s", len(errs), strings.Join(errs, "\n  "))
	}
	return out, nil
}

func (s *Store) loadPath(path string) (*model.Ticket, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t, err := ParseTicket(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	t.Path = path
	want := strings.TrimSuffix(filepath.Base(path), ".md")
	if t.ID != want {
		return nil, fmt.Errorf("%s: %w: frontmatter id %q does not match filename", filepath.Base(path), model.ErrUsage, t.ID)
	}
	return t, nil
}

// Load reads one ticket by exact ID.
func (s *Store) Load(id string) (*model.Ticket, error) {
	t, err := s.loadPath(s.PathFor(id))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: no ticket %s", model.ErrNotFound, id)
	}
	return t, err
}

// Index loads everything into a model.Index.
func (s *Store) Index() (*model.Index, error) {
	ts, err := s.List()
	if err != nil {
		return nil, err
	}
	return model.NewIndex(s.Cfg.IDPrefix, ts), nil
}

// save writes atomically: temp file in the same dir, then rename.
func (s *Store) save(t *model.Ticket) error {
	if err := t.Validate(); err != nil {
		return err
	}
	data, err := RenderTicket(t)
	if err != nil {
		return err
	}
	final := s.PathFor(t.ID)
	tmp := filepath.Join(s.ticketsDir(), "."+t.ID+".md.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	t.Path = final
	return nil
}
