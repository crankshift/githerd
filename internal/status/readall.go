package status

import (
	"context"

	"github.com/crankshift/githerd/internal/parallel"
)

// readWorkers bounds how many repos ReadAll reads with git at once.
const readWorkers = 8

// ReadAll reads every repo in dirs in parallel and keeps the order of dirs.
// Repos that could not be read are left out and their errors returned.
func ReadAll(ctx context.Context, dirs []string) ([]Repo, []error) {
	type read struct {
		repo Repo
		err  error
	}
	reads := parallel.Map(dirs, readWorkers, func(dir string) read {
		r, err := Read(ctx, dir)
		return read{r, err}
	})
	var ok []Repo
	var failed []error
	for _, r := range reads {
		if r.err != nil {
			failed = append(failed, r.err)
			continue
		}
		ok = append(ok, r.repo)
	}
	return ok, failed
}
