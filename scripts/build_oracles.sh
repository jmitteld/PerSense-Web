#!/usr/bin/env bash
# build_oracles.sh — build all three DOS source-oracles and smoke-test them.
#
# Compiles the headless source-oracles (the real DOS computational units driven
# against GUI stubs) used by the differential sweep tests:
#   - amort_oracle : amortization (ordinary + balloons)
#   - pv_oracle    : present value (lump, periodic, COLA, variable-rate, backward)
#   - mtg_oracle   : mortgage
#
# Requires Free Pascal (fpc) on PATH, or set FPC=/path/to/fpc.
#   Linux:  sudo apt-get install -y fpc
#   macOS:  brew install fpc
#
# Output binaries land in legacy/oracle/build/. Point the sweep tests at them:
#   export PERSENSE_ORACLE="$PWD/legacy/oracle/build/amort_oracle"
#   export PERSENSE_PV_ORACLE="$PWD/legacy/oracle/build/pv_oracle"
#   export PERSENSE_MTG_ORACLE="$PWD/legacy/oracle/build/mtg_oracle"
#   go test ./internal/finance/... -run TestDOS
#
# (The sweep tests skip automatically when these binaries are absent, so this
# script is only needed to *enable* them — ordinary `go test ./...` is fine
# without it.)
set -euo pipefail

cd "$(dirname "$0")/.."          # repo root
OUT="legacy/oracle/build"

for t in amort_oracle pv_oracle mtg_oracle; do
  echo ">> building $t"
  TARGET="$t" legacy/oracle/build.sh >/dev/null
done

echo ">> smoke-testing"
fail=0
check() { # check <name> <expected-substr> <args...>
  local name="$1" want="$2"; shift 2
  local got; got="$("$OUT/$name" "$@" 2>&1 || true)"
  if printf '%s' "$got" | grep -qF "$want"; then
    echo "   ok: $name"
  else
    echo "   FAIL: $name — expected to contain '$want', got: $got" >&2
    fail=1
  fi
}
check amort_oracle "payment 888.4879"  10000 0.12 12 12
check pv_oracle    "pv 9231.163464"    lump 10000 0.08 12
check mtg_oracle   "monthly 1066.683053" monthly 200000 0.20 30 0.07

[ "$fail" -eq 0 ] || { echo "one or more oracle smoke tests failed" >&2; exit 1; }

echo "OK: all three oracles built and smoke-tested in $OUT"
echo
echo "To run the differential sweeps:"
echo "  export PERSENSE_ORACLE=\"$PWD/$OUT/amort_oracle\""
echo "  export PERSENSE_PV_ORACLE=\"$PWD/$OUT/pv_oracle\""
echo "  export PERSENSE_MTG_ORACLE=\"$PWD/$OUT/mtg_oracle\""
echo "  go test ./internal/finance/... -run TestDOS -v"
