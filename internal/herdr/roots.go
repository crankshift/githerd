package herdr

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/crankshift/githerd/internal/discovery"
)

// rootsFile is the name of the roots file in the plugin config dir.
const rootsFile = "roots"

// RootsFile returns the path of the roots file in the plugin config dir.
func RootsFile(configDir string) string { return filepath.Join(configDir, rootsFile) }

// readRoots returns the roots listed in the file at path, with their Path
// and Depth set: one root per line, a leading ~ standing for the home dir,
// and optionally after it, apart, depth=N, how many directory levels below
// the root to scan instead of discovery.DefaultDepth. Blank lines and lines
// starting with # are ignored. A missing file lists no roots. A line that
// names no absolute path, or no whole number of levels, is left out and
// returned as an error.
func readRoots(path string) ([]Root, []error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var roots []Root
	var errs []error
	sc := bufio.NewScanner(bytes.NewReader(b))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		root, err := parseRoot(line)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s:%d: %s: %v", path, n, line, err))
			continue
		}
		roots = append(roots, root)
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, err)
	}
	return roots, errs
}

// parseRoot returns the root a line of the roots file lists.
func parseRoot(line string) (Root, error) {
	root := Root{Path: line, Depth: discovery.DefaultDepth}
	if i := strings.LastIndexAny(line, " \t"); i >= 0 {
		if depth, ok := strings.CutPrefix(line[i+1:], "depth="); ok {
			n, err := strconv.Atoi(depth)
			if err != nil || n < 0 {
				return Root{}, fmt.Errorf("depth %q is not a whole number", depth)
			}
			root.Path, root.Depth = strings.TrimSpace(line[:i]), n
		}
	}
	path, err := expandHome(root.Path)
	if err != nil {
		return Root{}, err
	}
	if !filepath.IsAbs(path) {
		return Root{}, errors.New("not an absolute path")
	}
	root.Path = filepath.Clean(path)
	return root, nil
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
