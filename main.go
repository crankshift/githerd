// Command githerd reports the state of every git repo under a root.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const usage = `usage: githerd <command> [arguments]

Commands:
  status [root] [--json] [--depth N] [--fetch [--ttl D] [--force] [--fetch-timeout D]]
      print the state of every repo under root (default: .); with --fetch,
      fetch each repo's upstream remote first, skipping repos fetched within
      the TTL (default 15m) unless --force is given
  herdr sync
      from a Herdr hook: refresh the repos under the roots listed in
      $HERDR_PLUGIN_CONFIG_DIR/roots and report them to Herdr's sidebar
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "status":
		return runStatus(ctx, args[1:], stdout, stderr)
	case "herdr":
		return runHerdr(ctx, args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "githerd: unknown command %q\n%s", args[0], usage)
		return 2
	}
}
