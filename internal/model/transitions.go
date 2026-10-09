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
	}
	return nil
}

// Claim applies todo -> doing for actor.
func Claim(t *Ticket, actor string, now time.Time) {
	t.Status = StatusDoing
	t.Assignee = actor
	ts := now
	t.ClaimedAt = &ts
}

// CanMove checks a move to any status. Only one structural rule exists:
// nothing with open children may be done.
func (ix *Index) CanMove(t *Ticket, to Status, force bool) error {
	if to.Index() < 0 {
		return fmt.Errorf("%w: unknown status %q", ErrUsage, to)
	}
	if t.Status == to {
		return fmt.Errorf("%w: %s is already %s", ErrConflict, t.ID, to)
	}
	if to == StatusDone && !force && ix.HasOpenChildren(t.ID) {
		done, total := ix.Progress(t.ID)
		return fmt.Errorf("%w: %s has open children (%d/%d done); finish them or use --force", ErrConflict, t.ID, done, total)
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
	return ix.CanMove(t, StatusDone, force)
}
