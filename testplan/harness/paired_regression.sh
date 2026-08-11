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

# 🚨🚨 THIS GATE COULD BE SILENTLY DESENSITIZED BY A LOG LINE — FOUND BY r53's
# OWN ADVERSARIAL AUDIT, IN TWO PASSES. PASS 1 FOUND ONE VARIABLE; PASS 2 FOUND
# THE CLASS, AND THAT GUARDING ONE NAME WAS THE WRONG REMEDIATION.
#
# THE MECHANISM. The harvester was `grep -oE "amort_oracle .*"` over the WHOLE
# raw -test.v stream. It cannot tell a FAILURE SIGNAL from a LOG LINE that
# happens to quote a reproducing command — so any env-gated diagnostic that
# prints one inflates the "failure" set, a regression whose command already
# appears there drops out of NEW, and the gate prints "no new divergences" on a
# live regression. Three emitters have this shape:
#   FZ5CASEDUMP           dos_fuzzer5_test.go:2108   (measured: FIXED 8/NEW 2
#                                                      -> FIXED 4/NEW 1; one
#                                                      real regression MASKED,
#                                                      pristine vs bolder, seed
#                                                      50100, FUZZ_N=400)
#   PERSENSE_FUZZ_FLAKEDUMP  :2166,:2182,:2263,:2279,:2329  (measured: unique
#                                                      harvested lines 21 -> 28
#                                                      over seeds 50100-50104,
#                                                      IDENTICAL ENGINE; and the
#                                                      source's own comment at
#                                                      :2282 records this line
#                                                      ungated once producing
#                                                      NEW 195 on an identical
#                                                      engine)
#   FZ5DISCRIMDUMP        :2885                      (same shape; did not fire
#                                                      on the seeds measured)
#
# TWO CHANGES, AND THE SECOND IS THE ONE THAT CLOSES THE CLASS:
#
#  1. REFUSE on any known dump variable (below). A name list can only ever be a
#     mitigation — the next diagnostic someone adds is not on it.
#  2. HARVEST ON `SIG=`, NOT ON `amort_oracle`. Every HARD and ADVISORY line the
#     fuzzer emits carries `SIG=<LEVEL>:<class>` on the SAME LINE as the
#     command, and no diagnostic log line does. The verdict is now computed from
#     that discriminating harvest. MEASURED BEFORE SWITCHING: on a clean run the
#     two harvests are IDENTICAL (4 == 4, symmetric difference empty), so this
#     loses no signal — and the RAW harvest is still taken and CROSS-CHECKED
#     below, so a future emitter shows up as a DISAGREEMENT instead of a silent
#     miss. That is the class-level detector; the name list is the doorstop.
#
# 🚨 AND IT FAILS CLOSED NOW. Before r53 this script printed
# "FIXED 0 / STILL 0 / NEW 0 / VERDICT: no new divergences" and EXITED 0 when
# handed two non-existent binaries. The project's MANDATORY rule-4 gate was
# trivially greenable by a typo'd path. R64: a run that produced no output is
# not a zero.
for v in FZ5CASEDUMP PERSENSE_FUZZ_FLAKEDUMP FZ5DISCRIMDUMP; do
  if [ -n "${!v-}" ]; then   # ${!v} indirection: no word-split, no glob (audit LOW-5)
    echo "REFUSING TO RUN: $v is set." >&2
    echo "  It makes the fuzzer print reproducing commands from LOG lines, which" >&2
    echo "  inflates this script's failure set and can MASK a regression." >&2
    echo "  Re-run with: env -u $v $0 ..." >&2
    exit 3
  fi
done

PRE=${1:?pre-fix test binary}; POST=${2:?post-fix test binary}
LO=${3:?first seed}; HI=${4:?last seed}
N=${FUZZ_N:-400}
JOBS=${JOBS:-$(( $(nproc 2>/dev/null || echo 2) - 1 ))}; [ "$JOBS" -lt 1 ] && JOBS=1
WORK=$(mktemp -d); trap 'rm -rf "$WORK"' EXIT

if [ ! -x /tmp/oraclebuild/amort_oracle ]; then
  echo "ERROR: oracle missing. Run legacy/oracle/build_linux.sh first." >&2; exit 2
fi
for b in "$PRE" "$POST"; do
  if [ ! -x "$b" ]; then
    echo "ERROR: test binary not executable: $b" >&2
    echo "  Build it with: go test -c -o $b ./internal/finance/amortization/" >&2
    exit 2
  fi
done

run_one() {  # $1=binary $2=tag $3=seed
  PERSENSE_FUZZ=1 PERSENSE_REQUIRE_ORACLE=1 \
  PERSENSE_FUZZ_SEED="$3" PERSENSE_FUZZ_N="$N" \
    timeout 900 "$1" -test.run TestDOSFuzzer5AllAdvancedOptions -test.v \
    > "$WORK/$2.$3.out" 2>&1
  # THE VERDICT HARVEST: signal lines only.
  grep -oE "SIG=[A-Za-z]+:[a-z_0-9]+ amort_oracle .*" "$WORK/$2.$3.out" \
    | sed -E 's/^SIG=[A-Za-z]+:[a-z_0-9]+ //' >> "$WORK/$2.sig"
  # THE CROSS-CHECK HARVEST: the old, undiscriminating one. Kept ONLY so a new
  # log-line emitter surfaces as a disagreement instead of a silent miss.
  grep -oE "amort_oracle .*" "$WORK/$2.$3.out" >> "$WORK/$2.raw"
  rm -f "$WORK/$2.$3.out"
}
export -f run_one; export WORK N

for pair in "$PRE pre" "$POST post"; do
  set -- $pair
  : > "$WORK/$2.raw"; : > "$WORK/$2.sig"
  seq "$LO" "$HI" | xargs -P "$JOBS" -I{} bash -c "run_one $1 $2 {}"
  sort -u "$WORK/$2.sig" > "$WORK/$2.set"
  sort -u "$WORK/$2.raw" > "$WORK/$2.rawset"
  # 🚨 R64 — A RUN THAT PRODUCED NO OUTPUT IS NOT A ZERO, AND THE GUARD MUST
  # TEST THE HARVEST THE VERDICT IS COMPUTED FROM. r53 audit pass 3 (N1-A)
  # demonstrated the first version of this guard testing `.rawset` while the
  # verdict came from `.set`: one character of drift in the SIG format gave
  # "FIXED 0 / STILL 0 / NEW 0 / VERDICT: no new divergences", exit 0, on a run
  # containing TWO REAL REGRESSIONS. Both are checked now.
  if [ ! -s "$WORK/$2.rawset" ]; then
    echo "ERROR: the $2 tree harvested NOTHING AT ALL over seeds $LO-$HI." >&2
    echo "  That is not 'no divergences' — it is a run that produced no output" >&2
    echo "  (bad binary, wrong -test.run, oracle refusing everything). R64." >&2
    exit 2
  fi
  if [ ! -s "$WORK/$2.set" ]; then
    echo "ERROR: the $2 tree produced output but the SIG= harvest is EMPTY." >&2
    echo "  The verdict is computed from that harvest, so this would read as" >&2
    echo "  'no new divergences' while comparing nothing at all. The emitted" >&2
    echo "  signal format has almost certainly drifted from this script's" >&2
    echo "  SIG=[A-Za-z]+:[a-z_0-9]+ pattern. R64 + r53 audit N1-A." >&2
    grep -oE "amort_oracle .*" "$WORK/$2.rawset" | head -2 | sed 's/^/    /' >&2
    exit 2
  fi
done

# 🚨 THE CLASS-LEVEL DETECTOR. If the undiscriminating harvest sees commands the
# SIG harvest does not, some log line is printing reproducing commands into the
# stream and the OLD verdict would have been computed over a contaminated set.
for t in pre post; do
  extra=$(comm -13 "$WORK/$t.set" "$WORK/$t.rawset" | wc -l | tr -d " ")
  if [ "$extra" -gt 0 ]; then
    echo "🚨 WARNING: the $t tree printed $extra reproducing command(s) OUTSIDE a" >&2
    echo "   SIG= line. Some diagnostic is emitting into the harvest stream." >&2
    echo "   The verdict below is computed from SIG lines, so a DIAGNOSTIC" >&2
    echo "   emitter cannot contaminate it — but if the SIG harvest has itself" >&2
    echo "   drifted, the run above already failed closed. Add the responsible" >&2
    echo "   env var to the refusal list at the top." >&2
    comm -13 "$WORK/$t.set" "$WORK/$t.rawset" | head -3 | sed 's/^/     /' >&2
  fi
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
