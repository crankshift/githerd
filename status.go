package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/crankshift/githerd/internal/discovery"
	"github.com/crankshift/githerd/internal/fetch"
	"github.com/crankshift/githerd/internal/status"
)

// readWorkers bounds how many repos are read with git at once.
const readWorkers = 8

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	depth := fs.Int("depth", discovery.DefaultDepth, "directory levels below the root to scan")
	doFetch := fs.Bool("fetch", false, "fetch each repo's upstream remote before reporting")
	ttl := fs.Duration("ttl", fetch.DefaultTTL, "with --fetch, skip repos fetched less than this long ago")
	force := fs.Bool("force", false, "with --fetch, fetch every repo, whatever the TTL")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if !*doFetch {
		if name := setFlag(fs, "ttl", "force"); name != "" {
			fmt.Fprintf(stderr, "githerd status: --%s needs --fetch\n", name)
			return 2
		}
	}
	if *ttl < 0 {
		fmt.Fprintln(stderr, "githerd status: --ttl must not be negative")
		return 2
	}
	if len(positional) > 1 {
		fmt.Fprintf(stderr, "githerd status: one root at most, got %d\n", len(positional))
		return 2
	}
	if *depth < 0 {
		fmt.Fprintln(stderr, "githerd status: --depth must not be negative")
		return 2
	}
	root := "."
	if len(positional) == 1 {
		root = positional[0]
	}
	root, err = filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(stderr, "githerd:", err)
		return 1
	}

	dirs, err := discovery.Repos(root, *depth)
	if err != nil {
		fmt.Fprintln(stderr, "githerd:", err)
		return 1
	}
	stale := map[string]fetch.Outcome{} // failed fetches by repo dir
	if *doFetch {
		for _, o := range fetch.Run(ctx, dirs, fetch.Options{TTL: *ttl, Force: *force}) {
			if o.Err != nil {
				stale[o.Dir] = o
			}
		}
	}
	repos, errs := readAll(ctx, dirs)
	for _, err := range errs {
		fmt.Fprintln(stderr, "githerd:", err)
	}

	if *asJSON {
		if err := printJSON(stdout, root, repos, stale); err != nil {
			fmt.Fprintln(stderr, "githerd:", err)
			return 1
		}
	} else {
		printTable(stdout, root, repos, stale)
		for _, dir := range dirs {
			if o, ok := stale[dir]; ok {
				fmt.Fprintf(stderr, "githerd: %s: %s: %v\n", relPath(root, dir), strings.TrimSpace("fetch "+o.Remote), o.Err)
			}
		}
	}
	if len(errs) > 0 {
		return 1
	}
	return 0
}

// parseInterspersed parses args with fs, allowing flags after positional
// arguments, and returns the positional arguments.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			return append(positional, rest...), nil // everything after -- is positional
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// setFlag returns the first of names that was set on the command line, or "".
func setFlag(fs *flag.FlagSet, names ...string) string {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for _, name := range names {
		if set[name] {
			return name
		}
	}
	return ""
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

func printTable(w io.Writer, root string, repos []status.Repo, stale map[string]fetch.Outcome) {
	if len(repos) == 0 {
		fmt.Fprintln(w, "0 of 0 repos need attention")
		return
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REPO\tBRANCH\tUNCOMMITTED\tAHEAD\tBEHIND\tUPSTREAM\tNOTE")
	attention, stales := 0, 0
	for _, r := range repos {
		if r.NeedsAttention() {
			attention++
		}
		_, isStale := stale[r.Dir]
		if isStale {
			stales++
		}
		upstream := r.Upstream
		if upstream == "" {
			upstream = "-"
		}
		branch := r.Branch
		if r.Detached != "" {
			branch = r.Detached
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\t%s\n",
			relPath(root, r.Dir), branch, r.Uncommitted, r.Ahead, r.Behind, upstream, note(r, isStale))
	}
	tw.Flush()
	for _, line := range strings.SplitAfter(buf.String(), "\n") {
		if line != "" {
			fmt.Fprintln(w, strings.TrimRight(line, " \n"))
		}
	}
	fmt.Fprintf(w, "%d of %d repos need attention", attention, len(repos))
	if stales > 0 {
		fmt.Fprintf(w, ", %d stale", stales)
	}
	fmt.Fprintln(w)
}

// jsonReport is the shape of `githerd status --json`.
type jsonReport struct {
	Root  string     `json:"root"`
	Repos []jsonRepo `json:"repos"`
}

// jsonRepo is one repo in a jsonReport. Pointer fields are null when the
// state does not apply.
type jsonRepo struct {
	Path           string  `json:"path"`
	Branch         *string `json:"branch"`
	Detached       *string `json:"detached"`
	Upstream       *string `json:"upstream"`
	Uncommitted    int     `json:"uncommitted"`
	Ahead          int     `json:"ahead"`
	Behind         int     `json:"behind"`
	NeedsAttention bool    `json:"needsAttention"`
	Stale          *string `json:"stale"`     // why the fetch failed; null when it didn't fail or didn't run
	FetchedAt      *string `json:"fetchedAt"` // RFC 3339, UTC
}

func printJSON(w io.Writer, root string, repos []status.Repo, stale map[string]fetch.Outcome) error {
	report := jsonReport{Root: root, Repos: make([]jsonRepo, 0, len(repos))}
	for _, r := range repos {
		j := jsonRepo{
			Path:           relPath(root, r.Dir),
			Branch:         nullable(r.Branch),
			Detached:       nullable(r.Detached),
			Upstream:       nullable(r.Upstream),
			Uncommitted:    r.Uncommitted,
			Ahead:          r.Ahead,
			Behind:         r.Behind,
			NeedsAttention: r.NeedsAttention(),
		}
		if !r.FetchedAt.IsZero() {
			j.FetchedAt = nullable(r.FetchedAt.UTC().Format(time.RFC3339))
		}
		if o, ok := stale[r.Dir]; ok {
			j.Stale = nullable(o.Err.Error())
		}
		report.Repos = append(report.Repos, j)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// nullable returns nil for an empty s, so it encodes as JSON null.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// note names the states of r that the number columns don't show.
func note(r status.Repo, stale bool) string {
	var notes []string
	switch {
	case r.Detached != "":
		notes = append(notes, "detached")
	case r.NoUpstream():
		notes = append(notes, "no upstream")
	}
	if stale {
		notes = append(notes, "stale")
	}
	return strings.Join(notes, ", ")
}

// relPath returns dir relative to root, with forward slashes.
func relPath(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return dir
	}
	return filepath.ToSlash(rel)
}
