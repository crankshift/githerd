# Go with the standard library only, built at install

githerd is written in Go using only the standard library, and the Herdr plugin builds it with `go build` in `[[build]]` when the plugin is installed, as Herdr's own example plugins build from source. Go's standard library covers JSON, process spawning, concurrency and, through `syscall`, raw terminal mode, so there is nothing to vendor.

## Considered Options

- **Prebuilt binaries committed to the repo**: rejected. Herdr's manifest has no field for prebuilt binaries, and committing them for every release bloats the history.
- **POSIX shell**: rejected. Nothing to build, but a TUI that browses diffs, and reliable parsing of Herdr's JSON, are impractical in shell.
- **Rust**: rejected. Its standard library has no JSON, so it would pull in third-party crates.

## Consequences

Installing the plugin from the Herdr marketplace needs a Go toolchain on the machine.
