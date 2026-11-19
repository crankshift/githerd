// Package parallel runs one function on many items at once, a few at a
// time.
package parallel

import "sync"

// Map calls f on each of items, at most n at once, and returns the results
// in the order of items.
func Map[T, R any](items []T, n int, f func(T) R) []R {
	out := make([]R, len(items))
	sem := make(chan struct{}, max(n, 1))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = f(item)
		}()
	}
	wg.Wait()
	return out
}
