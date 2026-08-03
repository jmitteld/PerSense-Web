#!/usr/bin/env bash
# check_skips.sh — fail if any test outside the allowlist skips in a gated run.
#
# WHY THIS EXISTS (round 18b). Three actuarial differentials skipped in every
# cloud container this project ever ran — their oracle script is outside the
# bootstrap tarball and their pip dependencies are not preinstalled — and the
# package printed `ok` the entire time. The backlog then described that surface
# as having "no coverage at all" while 668 checks sat there not running.
#
# Standing rule 2: a green suite is not validation; a skipped differential still
# prints ok. `PERSENSE_REQUIRE_ORACLE=1` enforces that for the three DOS drivers
# and nothing enforced it anywhere else. This script is the general version.
#
# A skip is a coverage claim nobody is checking. Either it is a deliberate
# opt-in — in which case it is written down in skip_allowlist.txt with a reason —
# or it is a gap, and this exits non-zero.
#
# USAGE
#   testplan/harness/check_skips.sh              # from the repo root
#   ALLOW=path/to/list testplan/harness/check_skips.sh
#
# Run it whenever the gated suite is run; it costs one extra suite run, or none
# if you pipe an existing -v log in via SUITE_LOG.
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ALLOW=${ALLOW:-$ROOT/testplan/harness/skip_allowlist.txt}
[ -f "$ALLOW" ] || { echo "ERROR: allowlist not found at $ALLOW" >&2; exit 2; }

TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

if [ -n "${SUITE_LOG:-}" ]; then
  cp "$SUITE_LOG" "$TMP/suite.log"
else
  echo "running the gated suite (this is the slow part)..."
  (cd "$ROOT" && PERSENSE_REQUIRE_ORACLE=1 go test ./... -v) > "$TMP/suite.log" 2>&1
fi

# Top-level and subtest SKIP lines both matter; key on the leaf test name.
grep -E '^(    )*--- SKIP' "$TMP/suite.log" \
  | grep -oE 'SKIP: [A-Za-z0-9_]+' | sed 's/SKIP: //' | sort -u > "$TMP/actual.txt"

sed 's/#.*//' "$ALLOW" | tr -d ' \t' | grep -v '^$' | sort -u > "$TMP/allowed.txt"

UNEXPECTED=$(comm -23 "$TMP/actual.txt" "$TMP/allowed.txt")
GONE=$(comm -13 "$TMP/actual.txt" "$TMP/allowed.txt")

echo "skipping now: $(wc -l < "$TMP/actual.txt")   allowlisted: $(wc -l < "$TMP/allowed.txt")"

if [ -n "$GONE" ]; then
  echo
  echo "INFO — allowlisted tests that are no longer skipping (good; prune the list):"
  echo "$GONE" | sed 's/^/    /'
fi

if [ -n "$UNEXPECTED" ]; then
  echo
  echo "FAIL — these tests skipped and are NOT on the allowlist:"
  echo "$UNEXPECTED" | sed 's/^/    /'
  echo
  echo "A skip is a coverage claim nobody is checking. Either:"
  echo "  - the test is a deliberate opt-in  -> add it to $(basename "$ALLOW") WITH A REASON, or"
  echo "  - a dependency is missing          -> give it a REQUIRE gate so it FAILS instead"
  echo "    (see internal/finance/actuarial/zzrequire_test.go)"
  exit 1
fi

echo "OK — every skipping test is a documented opt-in."
