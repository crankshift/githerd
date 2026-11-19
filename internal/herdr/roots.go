package herdr

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// rootsFile is the name of the roots file in the plugin config dir.
const rootsFile = "roots"

// readRoots returns the roots listed in the file at path, one per line.
// A missing file lists none.
func readRoots(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var roots []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			roots = append(roots, line)
		}
	}
	return roots, sc.Err()
}
