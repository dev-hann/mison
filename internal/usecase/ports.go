// Package usecase composes repo services into mison's business flows.
// Every policy — sync decisions, conflict resolution, ownership
// filtering, error classification — lives here, above the atomic
// service layer and behind explicit interaction ports.
package usecase

import "github.com/dev-hann/mison/internal/env"

// Reporter is the one-way notification port: business flows report
// what happened through it and never read anything back.
type Reporter interface {
	Step(msg string)   // ✓ completed local action
	Synced(msg string) // ↻ remote merge notice (always shown)
	Warn(msg string)   // ⚠ non-fatal issue
	Fail(msg string)   // ✗ fatal issue
	Line(msg string)   // plain output
	ToolLine(mark, name, detail string)
}

// Prompter is the two-way confirmation port: blocking questions whose
// answers gate destructive or ambiguous steps.
type Prompter interface {
	Confirm(question string) bool                     // y/N gate
	ResolveConflict(c env.Conflict) (env.Tool, error) // [1/2] gate
}

// ConflictPolicy decides how same-tool conflicts resolve
// non-interactively.
type ConflictPolicy int

// Conflict resolution policies.
const (
	PolicyAsk ConflictPolicy = iota
	PolicyOurs
	PolicyTheirs
)
