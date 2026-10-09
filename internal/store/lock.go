package store

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

type lock struct{ f *os.File }

// acquire takes an exclusive flock on .lock, retrying for up to wait.
func (s *Store) acquire(wait time.Duration) (*lock, error) {
	f, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &lock{f: f}, nil
		}
		if err != syscall.EWOULDBLOCK {
			f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("store is busy (another bored process holds %s)", s.lockPath())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *lock) release() {
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	l.f.Close()
}
