package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/crankshift/githerd/internal/herdr"
)

const herdrUsage = "usage: githerd herdr sync|rail\n"

func runHerdr(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || (args[0] != "sync" && args[0] != "rail") {
		fmt.Fprint(stderr, herdrUsage)
		return 2
	}
	name := "githerd herdr " + args[0]
	env, err := herdr.EnvFrom(os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, name+":", err)
		return 2
	}
	if args[0] == "rail" {
		if err := herdr.OpenRail(ctx, env); err != nil {
			fmt.Fprintln(stderr, name+":", err)
			return 1
		}
		return 0
	}
	res, err := herdr.Sync(ctx, env)
	for _, e := range res.Errs {
		fmt.Fprintln(stderr, name+":", e)
	}
	if err != nil {
		fmt.Fprintln(stderr, name+":", err)
		return 1
	}
	if len(res.Errs) > 0 {
		return 1
	}
	return 0
}
