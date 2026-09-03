package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// ErrLocked means another mison process holds the run lock.
var ErrLocked = errors.New("another mison run is in progress on this machine — close it first")

// Guard holds an exclusive flock. The kernel releases it when the
// process exits, so crashed runs never leave stale locks behind.
type Guard struct{ f *os.File }

// Acquire takes a non-blocking exclusive flock at path.
func Acquire(path string) (*Guard, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrLocked
	}
	return &Guard{f: f}, nil
}

// Release drops the lock.
func (g *Guard) Release() {
	if g == nil || g.f == nil {
		return
	}
	_ = syscall.Flock(int(g.f.Fd()), syscall.LOCK_UN)
	_ = g.f.Close()
	g.f = nil
}
