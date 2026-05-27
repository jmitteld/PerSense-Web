#!/usr/bin/env bash
# regen_refdata.sh — regenerate legacy/reference-output/refdata.json
# from legacy/testharness/refdata.pas using Free Pascal.
#
# Run this whenever refdata.pas has been extended with new cases, or
# whenever you want to confirm the checked-in JSON still matches what
# the harness emits today. The recommended cadence is "after every
# DOS-fidelity-impacting port" — see docs/dispatch_gaps.md §0.11.7.
#
# Requirements:
#   - Free Pascal compiler (fpc) on PATH, or set FPC=/path/to/fpc.
#     macOS:   brew install fpc           (provides /opt/homebrew/bin/fpc)
#     Linux:   apt-get install fpc        (provides /usr/bin/fpc)
#
# Usage:
#   scripts/regen_refdata.sh           # regenerate, fail if diff
#   scripts/regen_refdata.sh --apply   # regenerate and overwrite the
#                                      # checked-in JSON if it differs

set -euo pipefail

cd "$(dirname "$0")/.."

FPC="${FPC:-fpc}"
if ! command -v "$FPC" >/dev/null 2>&1; then
  echo "fpc not found on PATH. Install Free Pascal or set FPC=/path/to/fpc." >&2
  exit 1
fi

# Some Linux installs (specifically the bare `ppca64` compiler binary
# from `apt-get download fp-compiler-3.2.2`, unpacked without root)
# don't ship with an fpc.cfg, so the compiler can't find the `system`
# unit on its own. Locate the rtl directory under the fpc tree and
# point the compiler at it via FPC_CFG_EXTRA. On a normal `fpc`
# install this is a no-op — the wrapper picks up /etc/fpc.cfg.
FPC_CFG_EXTRA=()
FPC_DIR="$(dirname "$FPC")"
for guess in \
    "$FPC_DIR/units/aarch64-linux/rtl" \
    "$FPC_DIR/units/x86_64-linux/rtl" \
    "$FPC_DIR/../units/aarch64-linux/rtl" \
    "$FPC_DIR/../units/x86_64-linux/rtl"; do
  if [[ -d "$guess" ]]; then
    FPC_CFG_EXTRA+=("-Fu$guess")
    # rtl-objpas sits alongside rtl when both are unpacked.
    objpas="$(dirname "$guess")/rtl-objpas"
    [[ -d "$objpas" ]] && FPC_CFG_EXTRA+=("-Fu$objpas")
    break
  fi
done

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Compiling legacy/testharness/refdata.pas with $FPC ..."
mkdir -p "$TMP/build"
# -o sets the output executable path; -FU sends unit (.o, .ppu) files
# to the same scratch dir so nothing lands in legacy/. -Cn would
# suppress linker invocation; we want a normal exe.
if ! "$FPC" "${FPC_CFG_EXTRA[@]}" \
     -o"$TMP/build/refdata" -FU"$TMP/build" \
     legacy/testharness/refdata.pas; then
  echo "fpc failed — see output above." >&2
  exit 1
fi

echo "Running harness ..."
"$TMP/build/refdata" > "$TMP/refdata.json"

if diff -q legacy/reference-output/refdata.json "$TMP/refdata.json" >/dev/null 2>&1; then
  echo "refdata.json is current — no changes."
  exit 0
fi

echo
echo "refdata.json DIFFERS from harness output:"
diff -u legacy/reference-output/refdata.json "$TMP/refdata.json" | head -60 || true
echo

if [[ "${1:-}" == "--apply" ]]; then
  cp "$TMP/refdata.json" legacy/reference-output/refdata.json
  echo "Applied. Re-run 'go test ./internal/finance/...' to confirm the cross-check tests still pass."
  exit 0
fi

echo "Run with --apply to overwrite the checked-in JSON, or extend refdata.pas as needed."
exit 1
