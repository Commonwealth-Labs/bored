package tui

import (
	"time"

	"github.com/Commonwealth-Labs/bored/internal/edit"
	"github.com/Commonwealth-Labs/bored/internal/model"
)

type ticketsLoadedMsg struct {
	tickets []*model.Ticket
	ix      *model.Index
	modTime time.Time
	err     error
}

type mutationDoneMsg struct {
	what string
	err  error
}

type tickMsg time.Time

type editDoneMsg struct {
	sess *edit.Session
	err  error
}
