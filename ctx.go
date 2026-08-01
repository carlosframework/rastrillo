package rastrillo

import (
	"database/sql"
	"log/slog"
)

// Actor identifies who is calling an action: a human request or a named
// agent. See the design doc §8 — every action's caller is attributed,
// never anonymous, so audit trails can say who did what honestly.
type Actor struct {
	Human bool
	Name  string // empty for a human; the agent's name otherwise
}

// Ctx is passed to every action. It is the one extension point for
// per-request state a manifest or middleware needs to add — see Scope.
type Ctx struct {
	DB     *sql.DB
	Logger *slog.Logger

	// Locale is the resolved locale for this request (design doc §10).
	// Empty until the localization system lands; defaults are the app's
	// problem for now.
	Locale string

	// Actor records who is calling this action (design doc §8).
	Actor Actor

	// Scope is resolved by app-level middleware (_middleware.go, design
	// doc §4) and type-asserted by the handler. Rastrillo never reads it.
	Scope any
}
