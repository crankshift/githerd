package herdr

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// rootsFile is the name of the roots file in the plugin config dir.
const rootsFile = "roots"

// readRoots returns the roots listed in the file at path: one path per
// line, with a leading ~ standing for the home dir. Blank lines and lines
// starting with # are ignored. A missing file lists no roots. A line that
// is not an absolute path is left out and returned as an error.
func readRoots(path string) ([]string, []error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var roots []string
	var errs []error
	sc := bufio.NewScanner(bytes.NewReader(b))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		root, err := expandHome(line)
		if err == nil && !filepath.IsAbs(root) {
			err = errors.New("not an absolute path")
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s:%d: %s: %v", path, n, line, err))
			continue
		}
		roots = append(roots, filepath.Clean(root))
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, err)
	}
	return roots, errs
}

// expandHome replaces a leading ~ in path, alone or before a separator,
// with the home dir.
func expandHome(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~")
	if !ok || (rest != "" && rest[0] != filepath.Separator) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home + rest, nil
}
