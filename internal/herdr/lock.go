package herdr

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Files in the plugin state dir that keep syncs to one at a time.
const (
	lockFile = "sync.lock"    // locked by the running sync
	markFile = "sync.pending" // left by a sync that found the lock held
)

// syncLock is the held sync lock. The OS releases it when the process
// exits, so a crashed sync never leaves it held.
type syncLock struct {
	f    *os.File
	mark string
}

// lock takes the sync lock in dir and clears the mark, or returns nil when
// another sync holds the lock, after leaving the mark for it.
func lock(dir string) (*syncLock, error) {
	mark := filepath.Join(dir, markFile)
	f, err := tryLock(filepath.Join(dir, lockFile))
	if err != nil {
		return nil, err
	}
	if f == nil {
		if err := os.WriteFile(mark, nil, 0o644); err != nil {
			return nil, err
		}
		// The holder may have looked for the mark and let go before it
		// was left; then nobody would see it, so try the lock once more.
		if f, err = tryLock(filepath.Join(dir, lockFile)); f == nil || err != nil {
			return nil, err
		}
	}
	return held(f, mark)
}

// waitLock takes the sync lock in dir, waiting while another sync holds
// it, and clears the mark. It leaves no mark: the sync that waits runs
// after the one that holds the lock, which covers what a mark asks for.
func waitLock(ctx context.Context, dir string) (*syncLock, error) {
	for {
		f, err := tryLock(filepath.Join(dir, lockFile))
		if err != nil {
			return nil, err
		}
		if f != nil {
			return held(f, filepath.Join(dir, markFile))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(lockPoll):
		}
	}
}

// lockPoll is how often waitLock tries the lock.
const lockPoll = 50 * time.Millisecond

// held returns the lock held on f, once it has cleared the mark at path:
// the sync about to run covers what the mark asked for.
func held(f *os.File, mark string) (*syncLock, error) {
	if err := os.Remove(mark); err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.Close()
		return nil, err
	}
	return &syncLock{f: f, mark: mark}, nil
}

// unlock releases the lock and reports whether another sync left the mark
// meanwhile, so this one should run once more.
func (l *syncLock) unlock() (marked bool) {
	l.f.Close()
	_, err := os.Stat(l.mark)
	return err == nil
}

// tryLock locks the file at path without waiting, creating it if needed.
// It returns nil when another process holds the lock.
func tryLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, nil
		}
		return nil, err
	}
	return f, nil
}
