package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/crankshift/githerd/internal/discovery"
	"github.com/crankshift/githerd/internal/status"
)

// readWorkers bounds how many repos are read with git at once.
const readWorkers = 8

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	depth := fs.Int("depth", discovery.DefaultDepth, "directory levels below the root to scan")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(stderr, "githerd:", err)
		return 1
	}

	dirs, err := discovery.Repos(root, *depth)
	if err != nil {
		fmt.Fprintln(stderr, "githerd:", err)
		return 1
	}
	repos, errs := readAll(ctx, dirs)
	for _, err := range errs {
		fmt.Fprintln(stderr, "githerd:", err)
	}

	printTable(stdout, root, repos)
	if len(errs) > 0 {
		return 1
	}
	return 0
}

// readAll reads every repo in parallel and keeps the order of dirs. Repos
// that could not be read are left out and their errors returned.
func readAll(ctx context.Context, dirs []string) ([]status.Repo, []error) {
	repos := make([]status.Repo, len(dirs))
	errs := make([]error, len(dirs))
	sem := make(chan struct{}, readWorkers)
	var wg sync.WaitGroup
	for i, dir := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			repos[i], errs[i] = status.Read(ctx, dir)
		}()
	}
	wg.Wait()

	var ok []status.Repo
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

func printTable(w io.Writer, root string, repos []status.Repo) {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REPO\tBRANCH\tUNCOMMITTED\tAHEAD\tBEHIND\tUPSTREAM\tNOTE")
	attention := 0
	for _, r := range repos {
		if r.NeedsAttention() {
			attention++
		}
		upstream := r.Upstream
		if upstream == "" {
			upstream = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\t%s\n",
			relPath(root, r.Dir), r.Branch, r.Uncommitted, r.Ahead, r.Behind, upstream, "")
	}
	tw.Flush()
	for _, line := range strings.SplitAfter(buf.String(), "\n") {
		if line != "" {
			fmt.Fprintln(w, strings.TrimRight(line, " \n"))
		}
	}
	fmt.Fprintf(w, "%d of %d repos need attention\n", attention, len(repos))
}

// relPath returns dir relative to root, with forward slashes.
func relPath(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return dir
	}
	return filepath.ToSlash(rel)
}
