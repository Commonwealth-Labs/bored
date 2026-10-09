// Package runner implements `bored run`: hand ready tickets to headless
// Claude Code sessions one at a time, record what happened, and stop when
// nothing is ready.
package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Commonwealth-Labs/bored/internal/git"
	"github.com/Commonwealth-Labs/bored/internal/model"
	"github.com/Commonwealth-Labs/bored/internal/store"
)

// Options controls a run.
type Options struct {
	Actor          string
	Scope          model.Scope
	Max            int     // tickets to attempt; 0 = until nothing is ready
	MaxTurns       int     // per session
	BudgetUSD      float64 // per session; 0 = no cap
	PermissionMode string
	Worktree       bool
	Review         bool
	DryRun         bool
	ClaudeBin      string
	Cwd            string    // where repo-less tickets run
	Out            io.Writer // progress
}

// Outcome of one ticket.
type Outcome string

const (
	OutcomeReview   Outcome = "review"    // agent handed over
	OutcomeReleased Outcome = "released"  // agent gave it back itself
	OutcomeRecover  Outcome = "recovered" // session ended mid-way; runner released it
	OutcomeSkipped  Outcome = "skipped"   // could not prepare a working directory
	OutcomeError    Outcome = "error"     // claude failed to run
	OutcomeDryRun   Outcome = "dry-run"
)

// Result is one ticket's record.
type Result struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Dir      string  `json:"dir"`
	Outcome  Outcome `json:"outcome"`
	CostUSD  float64 `json:"cost_usd"`
	Turns    int     `json:"turns"`
	RunDir   string  `json:"run_dir,omitempty"`
	Note     string  `json:"note,omitempty"`
	Review   string  `json:"review,omitempty"`
	Duration string  `json:"duration,omitempty"`
}

// ErrNothingReady is returned when the first pick finds nothing.
var ErrNothingReady = errors.New("nothing ready")

// ErrStoppedOnFailures is returned after two consecutive failed sessions.
var ErrStoppedOnFailures = errors.New("stopped after two consecutive failures")

// Run executes the loop.
func Run(s *store.Store, o Options) ([]Result, error) {
	if o.ClaudeBin == "" {
		o.ClaudeBin = "claude"
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.MaxTurns == 0 {
		o.MaxTurns = 60
	}
	if o.PermissionMode == "" {
		o.PermissionMode = "acceptEdits"
	}
	if !o.DryRun {
		if _, err := exec.LookPath(o.ClaudeBin); err != nil {
			return nil, fmt.Errorf("%s not found on PATH", o.ClaudeBin)
		}
		if err := s.EnsureIgnored("runs/", "worktrees/"); err != nil {
			return nil, err
		}
	}
	var results []Result
	fails := 0
	for n := 0; o.Max == 0 || n < o.Max; n++ {
		ix, err := s.Index()
		if err != nil {
			return results, err
		}
		t, reason := ix.Next(o.Scope, o.Actor, time.Now())
		if t == nil {
			if n == 0 {
				return results, ErrNothingReady
			}
			fmt.Fprintln(o.Out, "nothing more ready")
			break
		}
		dir, note, err := workDir(s, t, o)
		res := Result{ID: t.ID, Title: t.Title, Dir: dir}
		if err != nil {
			res.Outcome, res.Note = OutcomeSkipped, err.Error()
			fmt.Fprintf(o.Out, "skip %s: %v\n", t.ID, err)
			results = append(results, res)
			// Can't make progress on this one; stop rather than loop on it.
			break
		}
		fmt.Fprintf(o.Out, "→ %s  %s\n  %s\n  in %s%s\n", t.ID, t.Title, reason, dir, note)
		if o.DryRun {
			res.Outcome = OutcomeDryRun
			results = append(results, res)
			break
		}
		start := time.Now()
		sess, serr := session(s, o, dir, t.ID, "/bored-work "+t.ID, "work")
		res.RunDir, res.CostUSD, res.Turns = sess.runDir, sess.cost, sess.turns
		res.Duration = time.Since(start).Round(time.Second).String()
		after, lerr := s.Load(t.ID)
		switch {
		case lerr != nil:
			res.Outcome, res.Note = OutcomeError, lerr.Error()
		case serr != nil && after.Status != model.StatusReview:
			res.Outcome, res.Note = OutcomeError, serr.Error()
			if after.Status == model.StatusDoing && after.Assignee == o.Actor {
				release(s, t.ID, o.Actor, "runner: claude failed to run ("+serr.Error()+"); released")
				res.Outcome = OutcomeRecover
			}
		case after.Status == model.StatusReview:
			res.Outcome = OutcomeReview
		case after.Status == model.StatusDoing && after.Assignee == o.Actor:
			release(s, t.ID, o.Actor, fmt.Sprintf("runner: session ended without handover after %d turns (is_error=%v); released. See %s", sess.turns, sess.isError, sess.runDir))
			res.Outcome, res.Note = OutcomeRecover, "session ended without handover"
		case after.Status == model.StatusTodo:
			res.Outcome = OutcomeReleased
		default:
			res.Outcome, res.Note = OutcomeError, "unexpected status "+string(after.Status)
		}
		if res.Outcome == OutcomeReview && o.Review {
			rev, rerr := session(s, o, dir, t.ID, "/bored-review "+t.ID+" Non-interactive run: give your verdict and reasoning but do not run bored done or bored move; stop after the recommendation.", "review")
			res.CostUSD += rev.cost
			if rerr == nil && rev.result != "" {
				res.Review = rev.result
				s.Mutate("bored: log "+t.ID, func(tx *store.Tx) error {
					cur, err := tx.Resolve(t.ID)
					if err != nil {
						return err
					}
					cur.Body = model.AppendLog(cur.Body, tx.Now().Local(), o.Actor, "review (runner): "+squash(rev.result, 1500))
					tx.Put(cur)
					return nil
				})
			}
		}
		fmt.Fprintf(o.Out, "  %s  $%.2f  %d turns  %s\n", res.Outcome, res.CostUSD, res.Turns, res.Duration)
		results = append(results, res)
		if res.Outcome == OutcomeReview || res.Outcome == OutcomeReleased {
			fails = 0
		} else {
			fails++
			if fails >= 2 {
				return results, ErrStoppedOnFailures
			}
		}
	}
	return results, nil
}

// workDir decides where a ticket's session runs.
func workDir(s *store.Store, t *model.Ticket, o Options) (dir, note string, err error) {
	if t.Repo == "" {
		return o.Cwd, "  (no repo: thinking work)", nil
	}
	r, err := s.Repo(t.Repo)
	if err != nil {
		return "", "", err
	}
	if !git.Exists(r.Path) {
		return "", "", fmt.Errorf("repo %s path %s does not exist yet", r.Name, r.Path)
	}
	if !o.Worktree {
		return r.Path, "", nil
	}
	if !git.IsRepo(r.Path) {
		return "", "", fmt.Errorf("repo %s at %s is not a git repository; use --no-worktree", r.Name, r.Path)
	}
	wt := filepath.Join(s.Root, "worktrees", r.Name, strings.ToLower(t.ID))
	if git.Exists(wt) {
		return wt, "  (existing worktree)", nil
	}
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		return "", "", err
	}
	if err := git.WorktreeAdd(r.Path, wt, strings.ToLower(t.ID), r.Branch); err != nil {
		return "", "", err
	}
	return wt, "  (new worktree, branch " + strings.ToLower(t.ID) + ")", nil
}

type sessionResult struct {
	runDir  string
	result  string
	cost    float64
	turns   int
	isError bool
}

// session spawns one headless claude process and records its output.
func session(s *store.Store, o Options, dir, id, prompt, kind string) (sessionResult, error) {
	var sr sessionResult
	sr.runDir = filepath.Join(s.Root, "runs", time.Now().Format("20060102-150405")+"-"+strings.ToLower(id)+"-"+kind)
	if err := os.MkdirAll(sr.runDir, 0o755); err != nil {
		return sr, err
	}
	args := []string{"-p", prompt, "--output-format", "json", "--permission-mode", o.PermissionMode, "--max-turns", strconv.Itoa(o.MaxTurns)}
	if o.BudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(o.BudgetUSD, 'f', 2, 64))
	}
	cmd := exec.Command(o.ClaudeBin, args...)
	cmd.Dir = dir
	cmd.Env = childEnv(s, o)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	os.WriteFile(filepath.Join(sr.runDir, "stdout.json"), stdout.Bytes(), 0o644)
	os.WriteFile(filepath.Join(sr.runDir, "stderr.log"), stderr.Bytes(), 0o644)
	meta := map[string]any{"id": id, "kind": kind, "dir": dir, "args": args, "started": time.Now().Format(time.RFC3339)}
	var parsed struct {
		IsError bool    `json:"is_error"`
		Result  string  `json:"result"`
		Cost    float64 `json:"total_cost_usd"`
		Turns   int     `json:"num_turns"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err == nil {
		sr.result, sr.cost, sr.turns, sr.isError = parsed.Result, parsed.Cost, parsed.Turns, parsed.IsError
		meta["cost_usd"], meta["turns"], meta["is_error"] = sr.cost, sr.turns, sr.isError
	}
	if runErr != nil {
		meta["error"] = runErr.Error()
	}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(sr.runDir, "meta.json"), mb, 0o644)
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return sr, fmt.Errorf("%s: %s", o.ClaudeBin, squash(msg, 300))
	}
	return sr, nil
}

// childEnv passes the store and actor through and unsets CLAUDECODE so a run
// can be started from inside a Claude Code session.
func childEnv(s *store.Store, o Options) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CLAUDECODE=") || strings.HasPrefix(kv, "BORED_HOME=") || strings.HasPrefix(kv, "BORED_ACTOR=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "BORED_HOME="+s.Root, "BORED_ACTOR="+o.Actor)
}

func release(s *store.Store, id, actor, note string) {
	s.Mutate(fmt.Sprintf("bored: move %s doing->todo (runner)", id), func(tx *store.Tx) error {
		cur, err := tx.Resolve(id)
		if err != nil {
			return err
		}
		model.ApplyMove(cur, model.StatusTodo)
		cur.Body = model.AppendLog(cur.Body, tx.Now().Local(), actor, note)
		tx.Put(cur)
		return nil
	})
}

func squash(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
