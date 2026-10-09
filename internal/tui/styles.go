package tui

import "charm.land/lipgloss/v2"

type styles struct {
	title, colHead, colHeadSel, card, cardSel, id, dim, badge, blocked, assignee,
	status, errText, help, treeSel, promptLabel, ok lipgloss.Style
	accent, muted lipgloss.Style
}

func newStyles(dark bool) styles {
	ld := lipgloss.LightDark(dark)
	accent := ld(lipgloss.Color("#5A3FC0"), lipgloss.Color("#B39DFF"))
	muted := ld(lipgloss.Color("#6B6B6B"), lipgloss.Color("#8A8A8A"))
	fg := ld(lipgloss.Color("#1A1A1A"), lipgloss.Color("#E6E6E6"))
	warn := ld(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))
	bad := ld(lipgloss.Color("#B91C1C"), lipgloss.Color("#F87171"))
	good := ld(lipgloss.Color("#047857"), lipgloss.Color("#34D399"))
	border := ld(lipgloss.Color("#C9C9C9"), lipgloss.Color("#3F3F3F"))

	return styles{
		title:       lipgloss.NewStyle().Bold(true).Foreground(accent),
		colHead:     lipgloss.NewStyle().Bold(true).Foreground(muted),
		colHeadSel:  lipgloss.NewStyle().Bold(true).Foreground(accent).Underline(true),
		card:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Foreground(fg),
		cardSel:     lipgloss.NewStyle().Reverse(true).Bold(true),
		id:          lipgloss.NewStyle().Bold(true).Foreground(accent),
		dim:         lipgloss.NewStyle().Foreground(muted),
		badge:       lipgloss.NewStyle().Foreground(warn),
		blocked:     lipgloss.NewStyle().Foreground(bad),
		assignee:    lipgloss.NewStyle().Foreground(good),
		status:      lipgloss.NewStyle().Foreground(muted),
		errText:     lipgloss.NewStyle().Foreground(bad),
		ok:          lipgloss.NewStyle().Foreground(good),
		help:        lipgloss.NewStyle().Foreground(muted),
		treeSel:     lipgloss.NewStyle().Reverse(true).Bold(true),
		promptLabel: lipgloss.NewStyle().Bold(true).Foreground(accent),
		accent:      lipgloss.NewStyle().Foreground(accent),
		muted:       lipgloss.NewStyle().Foreground(muted),
	}
}
