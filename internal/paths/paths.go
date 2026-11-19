// Package paths tells which dir holds a path, for absolute, clean paths
// such as repo dirs and pane cwds.
package paths

import "path/filepath"

// Within reports whether path is dir or lies under it. Both are absolute
// and clean; a relative or empty one lies within nothing.
func Within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

// Deepest returns the index of the item of items whose dir, as dir says,
// holds path and is the longest: path is that dir or lies under it. The
// first such item wins a tie. It returns -1 when no dir holds path.
func Deepest[T any](path string, items []T, dir func(T) string) int {
	best := -1
	for i, it := range items {
		if d := dir(it); Within(path, d) && (best < 0 || len(d) > len(dir(items[best]))) {
			best = i
		}
	}
	return best
}
