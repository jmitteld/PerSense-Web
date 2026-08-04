#!/usr/bin/env bash
# run_plain_arm.sh — run the PLAIN-LOAN differential over a seed range and keep
# the FULL per-seed output. Round 22.
#
# WHY THIS EXISTS. `zzplain_differential_test.go` landed in round 21 as three
# ad-hoc seeds typed on a command line. fuzzer5 has had standing ranges since
# round 16 for a reason: a rate quoted from whatever seeds someone happened to
# run is not comparable round to round, and the first thing anyone asks of a new
# surface's number is whether it moved. It cannot move if nobody knows what it
# was measured on.
#
# It is also, at ~5.7 ms per generated case, by a wide margin the cheapest
# measurement this project owns — three 40-seed ranges cost about fifteen
# minutes and buy a six-figure in-scope denominator on the population that
# every published amortization figure structurally excludes.
#
# The three STANDING RANGES (fixed here so they are quotable, in the same shape
# as fuzzer5's 50100-50139 / 44000-44039 / 44200-44239):
#
#     PLAIN-A   21000-21039     (seed 21000 is round 21's original probe)
#     PLAIN-B   21200-21239
#     PLAIN-C   33000-33039
#
#   ARMDIR=/tmp/r22/plainA ./run_plain_arm.sh <binary> 21000 21039
#   python3 testplan/harness/analyze_plain_arm.py /tmp/r22/plainA
#
# The analyzer pools the ledgers; see its header for the units caution.
set -uo pipefail
BIN=${1:?test binary}; LO=${2:?first seed}; HI=${3:?last seed}
N=${PLAIN_N:-1200}
DIR=${ARMDIR:?ARMDIR required}
# Same default as run_arm.sh — nproc-1. Do not override it: rounds 13-21 all ran
# on two usable cores and the oracle spawns two processes per case.
JOBS=${JOBS:-$(( $(nproc 2>/dev/null || echo 2) - 1 ))}; [ "$JOBS" -lt 1 ] && JOBS=1
mkdir -p "$DIR"

run_one() {  # $1=seed
  PERSENSE_PLAINDIFF=1 PERSENSE_REQUIRE_ORACLE=1 \
  PERSENSE_PLAINDIFF_SEED="$1" PERSENSE_PLAINDIFF_N="$N" \
    timeout 900 "$BIN" -test.run TestDOSPlainLoanDifferential -test.v \
    > "$DIR/seed_$1.log" 2>&1
  echo "$?" > "$DIR/seed_$1.rc"
}
# EXPORT EVERYTHING THE SUBSHELL READS — round 17 lost a whole run to an
# unexported N and silently measured the default instead.
export -f run_one; export DIR N BIN

seq "$LO" "$HI" | xargs -P "$JOBS" -I{} bash -c 'run_one {}'
echo "PLAIN ARM DONE seeds ${LO}-${HI} N=${N} -> $DIR"

# R8 EXTENSION, THE ROUND-22 ITEM: A KILLED BINARY IS A SILENT BUCKET.
# Go buffers t.Logf until the test returns, so a seed whose binary is killed
# (OOM, timeout, wrapper kill) emits NO ledger line at all — and a missing
# ledger reads exactly like a seed that was never asked for. Round 19 lost five
# seeds this way and the paired gate reported NEW 0. The rc file above and this
# check make it a COUNTED bucket rather than an absence.
missing=0; killed=0
for s in $(seq "$LO" "$HI"); do
  grep -q '^\s*.*ledger:' "$DIR/seed_$s.log" 2>/dev/null || missing=$((missing+1))
  rc=$(cat "$DIR/seed_$s.rc" 2>/dev/null || echo 999)
  [ "$rc" != "0" ] && [ "$rc" != "1" ] && killed=$((killed+1))
done
echo "SEEDS WITHOUT A LEDGER LINE: $missing"
echo "SEEDS WITH A NON-TEST EXIT STATUS (killed/timed out): $killed"
[ "$missing" -eq 0 ] && [ "$killed" -eq 0 ] && echo "SEED ACCOUNTING OK"
