// Package discovery finds the repos under a root.
package discovery

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDepth is how many directory levels below the root are scanned
// when the caller has no preference.
const DefaultDepth = 3

// Repos returns the repos under root, at most depth directory levels below
// it, as absolute paths in path order. The root comes first when it is a repo.
func Repos(root string, depth int) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", root)
	}
	var repos []string
	walk(root, depth, &repos)
	return repos, nil
}

func walk(dir string, depth int, repos *[]string) {
	if isRepo(dir) {
		*repos = append(*repos, dir)
	}
	if depth <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // an unreadable directory hides no repos we could report
	}
	for _, e := range entries {
		if e.IsDir() {
			walk(filepath.Join(dir, e.Name()), depth-1, repos)
		}
	}
}

// isRepo reports whether dir contains .git, as a directory or as a file.
func isRepo(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}
