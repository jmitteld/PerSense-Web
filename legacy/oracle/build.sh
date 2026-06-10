#!/usr/bin/env bash
# build.sh — compile the headless source-oracle.
#
# The legacy sources have inconsistent filename casing (PETYPES.PAS vs
# PEDATA.pas vs VIDEODAT.pas). On a case-sensitive volume FPC's unit
# search misses some of them, so we stage lowercase-named symlinks to
# every unit in legacy/oracle/units and compile from there. The headless
# stub Globals / HelpSystemUnit (in legacy/oracle) override the real,
# GUI-coupled ones; every other unit links from legacy/src/dos_source.
#
# Usage:
#   legacy/oracle/build.sh                       # milestone 1: link probe
#   TARGET=amort_oracle legacy/oracle/build.sh   # milestone 2: the driver
#
# Requires fpc on PATH (brew install fpc), or set FPC=/path/to/fpc.
set -euo pipefail

cd "$(dirname "$0")/../.."          # repo root
FPC="${FPC:-fpc}"
TARGET="${TARGET:-link_probe}"
ROOT="legacy/oracle"
STAGE="$ROOT/units"
OUT="$ROOT/build"

# Per-target unit-output dir so building several targets into the same $OUT
# (e.g. scripts/build_oracles.sh builds amort_oracle, pv_oracle, mtg_oracle in
# sequence) can't collide on compiled .ppu files.
UNITOUT="$OUT/_units_$TARGET"
rm -rf "$STAGE"
mkdir -p "$STAGE" "$OUT" "$UNITOUT"

lc() { printf '%s' "$1" | tr 'A-Z' 'a-z'; }

# Stage lowercase symlinks to every real legacy unit.
shopt -s nullglob
for f in legacy/src/dos_source/*.pas legacy/src/dos_source/*.PAS; do
  ln -sf "$(cd "$(dirname "$f")" && pwd)/$(basename "$f")" "$STAGE/$(lc "$(basename "$f")")"
done
# Headless stubs + the target program override anything with the same name.
for f in "$ROOT"/*.pas; do
  ln -sf "$(cd "$ROOT" && pwd)/$(basename "$f")" "$STAGE/$(lc "$(basename "$f")")"
done
shopt -u nullglob

echo "Compiling $TARGET ..."
# Conditional-compilation flags from the authoritative build config
# legacy/src/win_source/Persense.cfg (-DV_3;SCROLLS;PVLX). These select
# the full-product code paths (e.g. nscr=6) — NOT ACTU, matching the
# shipped build that never compiled life-contingency.
# -Mdelphi: the win_source units are Delphi-flavored.  -Sg: allow goto/label.
# -gl: line-number info in run-time backtraces (so a crash shows file:line).
"$FPC" -Mdelphi -Sg -gl -dV_3 -dSCROLLS -dPVLX \
  -Fu"$STAGE" -FU"$UNITOUT" -o"$OUT/$TARGET" "$STAGE/$(lc "$TARGET").pas"

echo "OK: built $OUT/$TARGET"
