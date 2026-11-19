package herdr

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// seqFile, in the plugin state dir, holds the --seq of the last sync.
const seqFile = "seq"

// nextSeq returns the --seq for a sync and keeps it in stateDir: the Unix
// time in milliseconds, or the last --seq plus one when the clock is not
// past it, because Herdr ignores a report whose --seq is not above the last
// one from the same source. The caller holds the sync lock.
func nextSeq(stateDir string) (uint64, error) {
	path := filepath.Join(stateDir, seqFile)
	var last uint64
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		// An unreadable value counts as none: the clock takes over.
		last, _ = strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	case !errors.Is(err, fs.ErrNotExist):
		return 0, err
	}
	seq := max(uint64(time.Now().UnixMilli()), last+1)
	return seq, writeAtomic(path, strconv.FormatUint(seq, 10)+"\n")
}

// writeAtomic replaces the file at path with content, so a reader never
// sees it half written.
func writeAtomic(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
