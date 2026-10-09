package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Section headings every ticket body carries.
const (
	SecDescription = "## Description"
	SecAC          = "## Acceptance Criteria"
	SecPlan        = "## Plan"
	SecLog         = "## Log"
)

// NewBody builds a body with the standard sections.
func NewBody(description string, ac []string) string {
	var b strings.Builder
	b.WriteString(SecDescription + "\n\n")
	if strings.TrimSpace(description) != "" {
		b.WriteString(strings.TrimSpace(description) + "\n\n")
	}
	b.WriteString(SecAC + "\n\n")
	for _, c := range ac {
		b.WriteString("- [ ] " + strings.TrimSpace(c) + "\n")
	}
	if len(ac) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(SecPlan + "\n\n")
	b.WriteString(SecLog + "\n")
	return b.String()
}

// sectionBounds returns [start,end) line indexes of a section's content
// (excluding the heading), or -1,-1 when the heading is absent.
func sectionBounds(lines []string, heading string) (int, int) {
	start := -1
	for i, l := range lines {
		if start < 0 {
			if strings.TrimSpace(l) == heading {
				start = i + 1
			}
			continue
		}
		if strings.HasPrefix(l, "## ") {
			return start, i
		}
	}
	if start < 0 {
		return -1, -1
	}
	return start, len(lines)
}

// AppendLog adds a timestamped line under ## Log, creating the section at the
// end if it is missing.
func AppendLog(body string, ts time.Time, actor, msg string) string {
	line := fmt.Sprintf("- %s %s: %s", ts.Format("2006-01-02 15:04"), actor, strings.TrimSpace(msg))
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	start, end := sectionBounds(lines, SecLog)
	if start < 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, SecLog, line)
		return strings.Join(lines, "\n") + "\n"
	}
	// Insert before trailing blank lines of the section so the list stays contiguous.
	ins := end
	for ins > start && strings.TrimSpace(lines[ins-1]) == "" {
		ins--
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:ins]...)
	out = append(out, line)
	out = append(out, lines[ins:]...)
	return strings.Join(out, "\n") + "\n"
}

var acRe = regexp.MustCompile(`^\s*[-*] \[( |x|X)\] `)

// ACItem is one acceptance criterion.
type ACItem struct {
	N    int    `json:"n"`
	Done bool   `json:"done"`
	Text string `json:"text"`
}

// ListAC returns the criteria in order.
func ListAC(body string) []ACItem {
	lines := strings.Split(body, "\n")
	start, end := sectionBounds(lines, SecAC)
	if start < 0 {
		return nil
	}
	var out []ACItem
	for _, l := range lines[start:end] {
		m := acRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		out = append(out, ACItem{N: len(out) + 1, Done: m[1] != " ", Text: strings.TrimSpace(l[len(m[0]):])})
	}
	return out
}

// CountAC returns done/total.
func CountAC(body string) (done, total int) {
	for _, it := range ListAC(body) {
		total++
		if it.Done {
			done++
		}
	}
	return
}

// SetAC checks or unchecks the nth criterion (1-based).
func SetAC(body string, n int, done bool) (string, error) {
	lines := strings.Split(body, "\n")
	start, end := sectionBounds(lines, SecAC)
	if start < 0 {
		return "", fmt.Errorf("no %s section", SecAC)
	}
	k := 0
	for i := start; i < end; i++ {
		m := acRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		k++
		if k != n {
			continue
		}
		mark := " "
		if done {
			mark = "x"
		}
		loc := acRe.FindStringSubmatchIndex(lines[i])
		// group 1 is the mark inside the brackets
		lines[i] = lines[i][:loc[2]] + mark + lines[i][loc[3]:]
		return strings.Join(lines, "\n"), nil
	}
	return "", fmt.Errorf("no acceptance criterion #%d (have %d)", n, k)
}

// AddAC appends a criterion to the section, creating it before ## Plan or at
// the end if absent.
func AddAC(body, text string) string {
	item := "- [ ] " + strings.TrimSpace(text)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	start, end := sectionBounds(lines, SecAC)
	if start < 0 {
		return strings.TrimRight(body, "\n") + "\n\n" + SecAC + "\n\n" + item + "\n"
	}
	ins := end
	for ins > start && strings.TrimSpace(lines[ins-1]) == "" {
		ins--
	}
	out := append([]string{}, lines[:ins]...)
	if ins == start {
		out = append(out, "")
	}
	out = append(out, item)
	if ins < len(lines) {
		out = append(out, "")
		out = append(out, lines[ins:]...)
	}
	return strings.Join(out, "\n") + "\n"
}

// Section returns the trimmed content of a section, "" if absent.
func Section(body, heading string) string {
	lines := strings.Split(body, "\n")
	start, end := sectionBounds(lines, heading)
	if start < 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

// SetSection replaces a section's content, creating the section at the end
// if absent.
func SetSection(body, heading, content string) string {
	content = strings.TrimSpace(content)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	start, end := sectionBounds(lines, heading)
	if start < 0 {
		return strings.TrimRight(body, "\n") + "\n\n" + heading + "\n\n" + content + "\n"
	}
	out := append([]string{}, lines[:start]...)
	out = append(out, "")
	if content != "" {
		out = append(out, strings.Split(content, "\n")...)
		out = append(out, "")
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n") + "\n"
}

// LastLogLine returns the text of the newest log entry without its timestamp
// and actor, or "" if there is none.
func LastLogLine(body string) string {
	lines := strings.Split(body, "\n")
	start, end := sectionBounds(lines, SecLog)
	if start < 0 {
		return ""
	}
	for i := end - 1; i >= start; i-- {
		l := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(l, "- ") {
			continue
		}
		l = strings.TrimPrefix(l, "- ")
		if i := strings.Index(l, ": "); i >= 0 && len(l) > 16 && l[4] == '-' {
			return l[i+2:]
		}
		return l
	}
	return ""
}
