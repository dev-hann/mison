// Package gitclient wraps git operations for the environment repository.
// Phase 6 (M2) implements this with real git; tests use t.TempDir() repos.
package gitclient

// Client performs git operations on the environment clone.
type Client interface {
	Commit(message string) error
	Push() error
	PullRebase() error
}
