package lockfile

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireSecondFailsUntilReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".run.lock")

	g1, err := Acquire(path)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}

	// flock is per open-file-description: a second acquire from the
	// SAME process must still conflict (mirrors a second terminal)
	if _, err := Acquire(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Acquire() = %v, want ErrLocked", err)
	}

	g1.Release()

	g2, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() after Release() error = %v", err)
	}
	g2.Release()
}

func TestReleaseIdempotent(t *testing.T) {
	g, err := Acquire(filepath.Join(t.TempDir(), ".run.lock"))
	if err != nil {
		t.Fatal(err)
	}
	g.Release()
	g.Release() // must not panic
}
