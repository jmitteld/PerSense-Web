#!/usr/bin/env bash
# build_linux.sh — build the headless DOS source-oracle on Linux (no root).
#
# This compiles the REAL DOS computational units (peTypes/peData/INTSUTIL/
# AMORTOP/AMORTIZE from legacy/src/dos_source) against the headless Globals /
# HelpSystemUnit stubs in this directory, producing `amort_oracle`: a
# command-line program that drives the genuine DOS amortization engine and
# prints one machine-readable result line. It is the authority the Go port is
# differentially tested against (see internal/finance/amortization/zz_sweep_test.go).
#
# Why a separate Linux script (vs build.sh): the dev Macs run FPC from
# Homebrew, but the agent sandbox is aarch64 Linux with no root. This script
# fetches the Free Pascal compiler + RTL/FCL units via `apt-get download`
# (which needs no privileges) and extracts them locally with `dpkg-deb -x`,
# then compiles. It is idempotent: if the compiler is already staged it skips
# straight to the build.
#
# Usage:
#   legacy/oracle/build_linux.sh            # set up FPC if needed, then build
#   FPCROOT=/path OUT=/path legacy/oracle/build_linux.sh
#
# Output binary: $OUT/amort_oracle   (default /tmp/oraclebuild/amort_oracle)
#
# Run it:  amort_oracle AMOUNT RATE NPER PERYR
#   e.g.   amort_oracle 10000 0.12 12 12  ->  payment 888.4879 interest 661.85 paid 10661.85
#
# Builds amort_oracle by default. Set TARGET to build a different driver
# (e.g. TARGET=pv_oracle for the present-value oracle):
#   TARGET=pv_oracle legacy/oracle/build_linux.sh   # -> $OUT/pv_oracle
set -euo pipefail

# Repo root = two levels up from this script.
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
FPCROOT="${FPCROOT:-/tmp/fpcroot}"
STAGE="${STAGE:-/tmp/oraclestage}"
OUT="${OUT:-/tmp/oraclebuild}"
TARGET="${TARGET:-amort_oracle}"

ARCH="$(uname -m)"                       # aarch64 / x86_64
case "$ARCH" in
  aarch64) FPCARCH="aarch64-linux"; PPC="ppca64" ;;
  x86_64)  FPCARCH="x86_64-linux";  PPC="ppcx64" ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

LIBDIR="$FPCROOT/usr/lib/$(uname -m | sed 's/x86_64/x86_64/;s/aarch64/aarch64/')-linux-gnu/fpc/3.2.2"
# Debian multiarch dir name differs from uname; resolve it from what's present.
PPCBIN="$(find "$FPCROOT" -name "$PPC" -type f 2>/dev/null | head -1 || true)"

if [ -z "$PPCBIN" ]; then
  echo "Free Pascal compiler not found under $FPCROOT — fetching (no root needed)..."
  mkdir -p "$FPCROOT"
  tmpdeb="$(mktemp -d)"
  ( cd "$tmpdeb"
    apt-get download \
      fp-compiler-3.2.2 \
      fp-units-rtl-3.2.2 \
      fp-units-base-3.2.2 \
      fp-units-fcl-3.2.2 2>/dev/null || {
        echo "apt-get download failed; trying generic package names..." >&2
        apt-get download fp-compiler fp-units-rtl fp-units-base fp-units-fcl
      }
    for d in *.deb; do dpkg-deb -x "$d" "$FPCROOT"; done
  )
  rm -rf "$tmpdeb"
  PPCBIN="$(find "$FPCROOT" -name "$PPC" -type f 2>/dev/null | head -1 || true)"
fi
[ -n "$PPCBIN" ] || { echo "ERROR: could not obtain the $PPC compiler" >&2; exit 1; }

# Unit search roots (RTL/objpas/fcl-base/rtl-extra) sit next to the compiler.
UROOT="$(dirname "$PPCBIN")/units/$FPCARCH"
[ -d "$UROOT/rtl" ] || UROOT="$(find "$FPCROOT" -type d -name "$FPCARCH" -path '*/units/*' | head -1)"
[ -d "$UROOT/rtl" ] || { echo "ERROR: RTL units not found under $FPCROOT" >&2; exit 1; }

echo "compiler: $PPCBIN"
echo "units:    $UROOT"

# Clean only this target's staging + compiled-unit dir; keep other targets'
# binaries already in $OUT (e.g. building pv_oracle must not wipe amort_oracle).
UNITOUT="$STAGE/_units_$TARGET"
rm -rf "$STAGE"; mkdir -p "$STAGE" "$OUT" "$UNITOUT"
lc(){ printf '%s' "$1" | tr 'A-Z' 'a-z'; }

# The legacy sources have inconsistent filename casing; stage lowercase
# symlinks so FPC's unit search resolves every one on a case-sensitive FS.
shopt -s nullglob
for f in "$REPO"/legacy/src/dos_source/*.pas "$REPO"/legacy/src/dos_source/*.PAS; do
  ln -sf "$f" "$STAGE/$(lc "$(basename "$f")")"
done
# Headless stubs + the driver override any real unit of the same name.
for f in "$REPO"/legacy/oracle/*.pas; do
  ln -sf "$f" "$STAGE/$(lc "$(basename "$f")")"
done
shopt -u nullglob

# 64-bit pointer fix for AdvancePointer (VIDEODAT.pas). The legacy helper does
# `var px: longint absolute p` — it overlays a 32-bit longint on the pointer and
# computes `resultx := px + x`, truncating the high 32 bits of any 64-bit
# pointer and leaving them as stack garbage. That corrupts every offset-based
# record access (bf.FixPointers, dataoffset[] walks), faulting nondeterministically.
# Stage a patched COPY (widen the pointer overlays to ptrint) so the read-only
# legacy source is untouched; the patched copy overrides the symlink.
rm -f "$STAGE/videodat.pas"
sed -E 's/:[[:space:]]*longint absolute (p|theresult|result|oldresult);/: ptrint absolute \1;/g' \
  "$REPO/legacy/src/dos_source/VIDEODAT.pas" > "$STAGE/videodat.pas"

# Conditional flags from the authoritative build config Persense.cfg
# (-DV_3;SCROLLS;PVLX) — the full-product code paths, not ACTU.
# -CPPACKRECORD=1: byte-pack records to match the original Turbo Pascal layout.
# The DOS engine uses offset-based record access (bf.FixPointers, dataoffset[],
# disk I/O) that assumes TP's default 1-byte packing; FPC's Delphi-mode default
# aligns reals to 8 bytes, which mis-aligns those offset tables. Name-based field
# access (all the computational paths) is unaffected either way.
"$PPCBIN" -Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX \
  -Fu"$UROOT/rtl" -Fu"$UROOT/rtl-objpas" -Fu"$UROOT/fcl-base" -Fu"$UROOT/rtl-extra" \
  -Fu"$STAGE" -FU"$UNITOUT" -o"$OUT/$TARGET" "$STAGE/$TARGET.pas" \
  > "$OUT/build.log" 2>&1 || { echo "BUILD FAILED:"; tail -20 "$OUT/build.log"; exit 1; }

echo "built: $OUT/$TARGET"
if [ "$TARGET" = "amort_oracle" ]; then
  "$OUT/amort_oracle" 10000 0.12 12 12 && echo "(smoke test ok — expected: payment 888.4879 interest 661.85 paid 10661.85)"
elif [ "$TARGET" = "pv_oracle" ]; then
  "$OUT/pv_oracle" lump 10000 0.08 12 && echo "(smoke test ok — expected: pv 9231.163464 ...)"
elif [ "$TARGET" = "mtg_oracle" ]; then
  "$OUT/mtg_oracle" monthly 200000 0.20 30 0.07 && echo "(smoke test: solved monthly payment for a 200k/20%down/30yr loan)"
fi
