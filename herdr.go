package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/crankshift/githerd/internal/herdr"
)

func runHerdr(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "sync" {
		fmt.Fprintf(stderr, "usage: githerd herdr sync\n")
		return 2
	}
	env, err := herdr.EnvFrom(os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "githerd herdr sync:", err)
		return 2
	}
	res, err := herdr.Sync(ctx, env)
	for _, e := range res.Errs {
		fmt.Fprintln(stderr, "githerd herdr sync:", e)
	}
	if err != nil {
		fmt.Fprintln(stderr, "githerd herdr sync:", err)
		return 1
	}
	if len(res.Errs) > 0 {
		return 1
	}
	return 0
}
