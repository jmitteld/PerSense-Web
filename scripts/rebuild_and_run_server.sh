#!/bin/sh
# Rebuild and run the Per%Sense web server on :8080.
#
# Frees the port from any previous run first. `go run` cannot rebind a port
# that an earlier server still holds — it dies with "bind: address already in
# use" (main.go log.Fatal) while the OLD server keeps serving, so the browser
# silently keeps hitting stale code. Killing the prior listener avoids that
# "rebuilt but still seeing old numbers" trap.
set -e

# Compile everything first so a build error stops us before we touch the port.
go build ./...

# Free :8080 from a prior run, if anything is listening there.
if command -v lsof >/dev/null 2>&1; then
  lsof -ti tcp:8080 | xargs kill 2>/dev/null || true
fi

go run ./cmd/persense/
