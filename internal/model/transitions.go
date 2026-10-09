package model

import (
	"fmt"
	"time"
)

// CanClaim checks the rules for todo -> doing. force skips the assignee and
// blocked checks but never the structural ones.
func (ix *Index) CanClaim(t *Ticket, actor string, force bool) error {
	if t.Status == StatusDoing && t.Assignee == actor {
		return fmt.Errorf("%w: %s is already claimed by you", ErrConflict, t.ID)
	}
	if t.Status != StatusTodo && !(force && t.Status == StatusBacklog) {
		return fmt.Errorf("%w: %s is %s, not todo", ErrConflict, t.ID, t.Status)
	}
	if !ix.Workable(t.ID) {
		return fmt.Errorf("%w: %s has open children; work those first", ErrConflict, t.ID)
	}
	if !force {
		if t.Assignee != "" && t.Assignee != actor {
			return fmt.Errorf("%w: %s is assigned to %s", ErrConflict, t.ID, t.Assignee)
		}
		if b := ix.BlockedBy(t.ID); len(b) > 0 {
			return fmt.Errorf("%w: %s is blocked by %s", ErrConflict, t.ID, b[0].ID)
		}
		if t.IsStuck() {
			return fmt.Errorf("%w: %s is stuck (an agent gave up: %s); read the log, then bored label rm %s stuck, or claim --force", ErrConflict, t.ID, LastLogLine(t.Body), t.ID)
		}
	}
	return nil
}

// Claim applies todo -> doing for actor. Claiming clears the stuck label:
// whoever claims has looked.
func Claim(t *Ticket, actor string, now time.Time) {
	t.RemoveLabel(LabelStuck)
	t.Status = StatusDoing
	t.Assignee = actor
	ts := now
	t.ClaimedAt = &ts
}

// CanMove checks a move to any status. A container with open children has a
// derived status and cannot be moved by hand.
func (ix *Index) CanMove(t *Ticket, to Status) error {
	if to.Index() < 0 {
		return fmt.Errorf("%w: unknown status %q", ErrUsage, to)
	}
	if ix.HasOpenChildren(t.ID) {
		done, total := ix.Progress(t.ID)
		return fmt.Errorf("%w: %s has open children (%d/%d done); its status follows them", ErrConflict, t.ID, done, total)
	}
	if t.Status == to {
		return fmt.Errorf("%w: %s is already %s", ErrConflict, t.ID, to)
	}
	return nil
}

// ApplyMove sets the status and clears claim state when leaving doing/review
// for todo or backlog.
func ApplyMove(t *Ticket, to Status) {
	t.Status = to
	if to == StatusTodo || to == StatusBacklog {
		t.Assignee = ""
		t.ClaimedAt = nil
	}
}

// CanDone checks review -> done (the human gate). force allows from doing or todo.
func (ix *Index) CanDone(t *Ticket, force bool) error {
	if t.Status == StatusDone {
		return fmt.Errorf("%w: %s is already done", ErrConflict, t.ID)
	}
	if t.Status != StatusReview && !force {
		return fmt.Errorf("%w: %s is %s, not review; use --force to skip review", ErrConflict, t.ID, t.Status)
	}
	return ix.CanMove(t, StatusDone)
}

// CascadeDone runs after t was marked done. Walking up the parents: a parent
// whose children are now all done is handled by what it says about itself.
// Still in backlog, it was never touched: it closes automatically if it has
// no acceptance criteria (it was only a grouping) or becomes todo if it has.
// In todo, doing or review, a human put it there while it had no open
// children, so it stays where it is and is simply workable again. Returns
// the parents it changed, each with a log line appended.
func CascadeDone(ix *Index, t *Ticket, actor string, now time.Time) []*Ticket {
	var changed []*Ticket
	for p := ix.Get(t.Parent); p != nil; p = ix.Get(p.Parent) {
		if ix.HasOpenChildren(p.ID) {
			break
		}
		if p.Status != StatusBacklog {
			break // done already, or deliberately placed by a human
		}
		if HasOwnWork(p) {
			ApplyMove(p, StatusTodo)
			p.Body = AppendLog(p.Body, now.Local(), actor, "all children done; ready for its own work")
			changed = append(changed, p)
			break
		}
		ApplyMove(p, StatusDone)
		p.Body = AppendLog(p.Body, now.Local(), actor, "-> done: all children done")
		changed = append(changed, p)
	}
	return changed
}
