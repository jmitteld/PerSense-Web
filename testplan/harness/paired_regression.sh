#!/usr/bin/env bash
# paired_regression.sh — prove a fix did not introduce NEW divergences.
#
# WHY THIS EXISTS
# A green test suite does not mean a fix was safe. A fix can close 18 divergences
# and open 5, and every pass/fail count in the project will still read "clean":
#   - the fuzzer FAILS on Go-solved-DOS-refused but only LOGS the mirror
#     (DOS-solved-Go-refused), so an over-refusal regression is invisible;
#   - the committed suite's corpus is fixed, so a fix is only tested against
#     cases someone already thought of;
#   - fresh-seed sweeps report a COUNT, and a count cannot distinguish
#     "fixed 18, broke 5" from "fixed 13".
# On 2026-07-30 both failure modes happened for real: a documented fix, applied
# as written, moved a schedule by $6838.28; and a correct-looking refusal port
# imported an adjustment-free loan's refusal onto ~0.9% of adjustment-carrying
# screens. Neither was caught by the suite.
#
# WHAT IT DOES
# Runs the SAME seeds through a pre-fix and a post-fix build, keys every failure
# by its reproducing oracle command line, and set-diffs the two failure sets:
#     FIXED        present in pre, absent in post   -> the intended effect
#     STILL BROKEN present in both                  -> not closed yet
#     NEW          absent in pre, present in post   -> A REGRESSION. Blocks the fix.
# Exit status is non-zero if the NEW set is non-empty, so it can gate a sync.
#
# USAGE
#   go test -c -o /tmp/pre.test  ./internal/finance/amortization/   # from the PRE tree
#   go test -c -o /tmp/post.test ./internal/finance/amortization/   # from the POST tree
#   testplan/harness/paired_regression.sh /tmp/pre.test /tmp/post.test 40000 40199
#
# Env passthrough: PERSENSE_FUZZ_MODES (aim at a frontier), FUZZ_N (default 400),
# JOBS (default = cores-1). Requires the oracle at /tmp/oraclebuild/amort_oracle
# (build with legacy/oracle/build_linux.sh).
set -uo pipefail

PRE=${1:?pre-fix test binary}; POST=${2:?post-fix test binary}
LO=${3:?first seed}; HI=${4:?last seed}
N=${FUZZ_N:-400}
JOBS=${JOBS:-$(( $(nproc 2>/dev/null || echo 2) - 1 ))}; [ "$JOBS" -lt 1 ] && JOBS=1
WORK=$(mktemp -d); trap 'rm -rf "$WORK"' EXIT

if [ ! -x /tmp/oraclebuild/amort_oracle ]; then
  echo "ERROR: oracle missing. Run legacy/oracle/build_linux.sh first." >&2; exit 2
fi

run_one() {  # $1=binary $2=tag $3=seed
  PERSENSE_FUZZ=1 PERSENSE_REQUIRE_ORACLE=1 \
  PERSENSE_FUZZ_SEED="$3" PERSENSE_FUZZ_N="$N" \
    timeout 900 "$1" -test.run TestDOSFuzzer5AllAdvancedOptions -test.v 2>&1 \
    | grep -oE "amort_oracle .*" >> "$WORK/$2.raw"
}
export -f run_one; export WORK N

for pair in "$PRE pre" "$POST post"; do
  set -- $pair
  : > "$WORK/$2.raw"
  seq "$LO" "$HI" | xargs -P "$JOBS" -I{} bash -c "run_one $1 $2 {}"
  sort -u "$WORK/$2.raw" > "$WORK/$2.set"
done

nf=$(comm -13 "$WORK/pre.set" "$WORK/post.set" | wc -l)   # in post only
nb=$(comm -23 "$WORK/pre.set" "$WORK/post.set" | wc -l)   # in pre only
ns=$(comm -12 "$WORK/pre.set" "$WORK/post.set" | wc -l)

echo "seeds $LO-$HI  N=$N  modes=${PERSENSE_FUZZ_MODES:-<all>}"
echo "  FIXED        : $nb"
echo "  STILL BROKEN : $ns"
echo "  NEW (regress): $nf"
if [ "$nf" -gt 0 ]; then
  echo
  echo "REGRESSIONS — these cases agreed with DOS before the fix and do not now:"
  comm -13 "$WORK/pre.set" "$WORK/post.set" | sed 's/^/  /'
  echo
  echo "VERDICT: BLOCKED. Do not sync until the NEW set is empty."
  exit 1
fi
echo "VERDICT: no new divergences."
