// Package bored exposes the embedded skill files so `bored install-skills`
// can write them without needing the source tree. The embed directive must
// live at the module root because go:embed cannot reach parent directories.
package bored

import "embed"

// Skills holds skills/<name>/SKILL.md for each shipped skill.
//
//go:embed skills/*/SKILL.md
var Skills embed.FS
