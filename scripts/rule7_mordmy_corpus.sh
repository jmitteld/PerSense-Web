#!/bin/bash
# rule7_mordmy_corpus.sh — ROUND 41.
#
# WHY THIS EXISTS. Round 41 changed amort_oracle.pas (NF-6: the `mordmy=` year
# bound wrapped above 2155) and claimed "rule 7 holds, 0 differing over a
# 378-line corpus". The round-41 audit's fourth finding was that the claim was
# UNREPRODUCIBLE FROM THE TREE — the corpus existed only in a shell history and
# the in-source comment quoted a different number (324) than the round report
# (378). A rule-7 claim that cannot be re-run is a claim about the author, not
# about the binary. Rule 6: goldens carry provenance.
#
# It is also the standing recipe for the NEXT oracle edit. Rule 7 is the single
# most dangerous rule in this project to get wrong — the oracle is the authority
# every published number is measured against, and it has been the obstacle five
# times (rounds 29, 31, 32, 35, and the §65 retraction).
#
# USAGE
#   scripts/rule7_mordmy_corpus.sh <OLD_BINARY> <NEW_BINARY>
#   e.g. scripts/rule7_mordmy_corpus.sh /tmp/pre_amort_oracle /tmp/oraclebuild/amort_oracle
#
# EXIT 0 = default stdout unchanged on every well-formed line. EXIT 1 = a
# difference, printed.
#
# ⚠️ SCOPE, so nobody reads more into a pass than it carries. This sweeps
# WELL-FORMED command lines only. It says NOTHING about malformed input, and the
# round-41 change deliberately DOES alter stdout for 14 malformed / out-of-range
# `mordmy=` forms and for two duplicate-token shapes (see the comment at the
# `mordmy=` parser). "Rule 7 holds" means "no valid command line moved".
#
# ⚠️ AND ONE RUN IS NOT ENOUGH ON A PATH WITH UNINITIALISED STATE. The round-41
# audit found that a malformed `payoff=` value makes the oracle NONDETERMINISTIC
# (ParseDMY exits without touching its out-parameter, so the date is heap
# garbage): 40 identical invocations produced four different answers, all exit 0.
# A single-shot A/B corpus cannot distinguish "unchanged" from "nondeterministic
# and it happened to agree". REPEATS defaults to 3 for that reason.

set -u
OLD=${1:?usage: rule7_mordmy_corpus.sh <old_binary> <new_binary>}
NEW=${2:?usage: rule7_mordmy_corpus.sh <old_binary> <new_binary>}
REPEATS=${REPEATS:-3}

AMOUNTS="100000 250000 37500"
RATES="0.06 0.105 0.13"
TERMS="24 120 360"
# One representative of every option block the generator can stack, plus the two
# well-formed mordmy= shapes the change is actually about.
TOKENSETS=(
  ""
  "b6=5000"
  "mor=3"
  "skip=2:4"
  "pts=0.02"
  "adj=6:0.08"
  "pre=600:1000:3:819.78"
  "targ=1502.57"
  "plusreg"
  "exact b365"
  "r78"
  "usa"
  "mordmy=15.2.2030"
  "mordmy=1.6.2026 b6=5000"
)

n=0; diffs=0; nondet=0
for amt in $AMOUNTS; do
  for rate in $RATES; do
    for np in $TERMS; do
      for extra in "${TOKENSETS[@]}"; do
        n=$((n + 1))
        a=$("$OLD" "$amt" "$rate" "$np" 12 $extra 2>&1)
        b=$("$NEW" "$amt" "$rate" "$np" 12 $extra 2>&1)
        if [ "$a" != "$b" ]; then
          diffs=$((diffs + 1))
          echo "DIFF: $amt $rate $np 12 $extra"
          echo "  OLD: $a"
          echo "  NEW: $b"
          continue
        fi
        # self-determinism: the same binary, the same argv, REPEATS times
        for _ in $(seq 2 "$REPEATS"); do
          c=$("$NEW" "$amt" "$rate" "$np" 12 $extra 2>&1)
          if [ "$c" != "$b" ]; then
            nondet=$((nondet + 1))
            echo "NONDETERMINISTIC: $amt $rate $np 12 $extra"
            break
          fi
        done
      done
    done
  done
done

echo "rule-7 corpus: $n command lines, $diffs differing, $nondet nondeterministic (REPEATS=$REPEATS)"
[ "$diffs" -eq 0 ] && [ "$nondet" -eq 0 ]
