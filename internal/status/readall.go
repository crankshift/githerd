package status

import (
	"context"
	"sync"
)

// readWorkers bounds how many repos ReadAll reads with git at once.
const readWorkers = 8

// ReadAll reads every repo in dirs in parallel and keeps the order of dirs.
// Repos that could not be read are left out and their errors returned.
func ReadAll(ctx context.Context, dirs []string) ([]Repo, []error) {
	repos := make([]Repo, len(dirs))
	errs := make([]error, len(dirs))
	sem := make(chan struct{}, readWorkers)
	var wg sync.WaitGroup
	for i, dir := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			repos[i], errs[i] = Read(ctx, dir)
		}()
	}
	wg.Wait()

	var ok []Repo
	var failed []error
	for i := range dirs {
		if errs[i] != nil {
			failed = append(failed, errs[i])
			continue
		}
		ok = append(ok, repos[i])
	}
	return ok, failed
}
