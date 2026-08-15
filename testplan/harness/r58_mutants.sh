#!/usr/bin/env bash
# r58_mutants.sh — the mutation harness for §99 (round 58), COMMITTED because an
# uncommitted instrument makes its finding unfalsifiable in both directions (R32).
#
# Run:  bash testplan/harness/r58_mutants.sh
# Needs: /tmp/oraclebuild/amort_oracle, and SRC below pointing at the tree.
#
# EXPECTED AT r58 CLOSE — six killed, four surviving, each survival explained
# in fancybisect.go §99 or zzr58_segment_horizon_overshoot_test.go:
#   M1  no_overshoot            KILLED by TestR58SegmentHorizonOvershootMatchesDOS
#   M2  guard_removed           KILLED by TestR58GuardIsLoadBearing
#   M3  guard_inverted          KILLED by BOTH
#   M4  bound_never_moves       KILLED by TestR58SegmentHorizonOvershootMatchesDOS
#   M6  extended_counter_unset  KILLED by TestR58SegmentHorizonOvershootMatchesDOS
#   CONTROL_extractor         KILLED  <- R77: proves the killer-extractor works
#   M7  derived_cap_removed     SURVIVES — the cap is provably redundant
#   M5  nperiods_raise_readded  SURVIVES — the loop is dead; that is why it is gone
#   M10 guard_widened_to_le     SURVIVES — pin 1 is `eq` (admitted), pin 2 is
#                               `gt` (rejected); NEITHER is `lt` or `notok`, so
#                               neither widening moves either verdict (R84).
#                               (An earlier header said "both pins are eq" —
#                               false, and refuted by the guard pin's own
#                               `eligible != 0` assertion.)
#   M11 notok_disjunct_readded  SURVIVES — same reason. A THIRD PIN IS OWED (r59),
#                               over seed 51045 case 275, measured `lt`.
#
# r58 mutation harness for §99.
# R77: PROVE each mutant applied (assert the file changed) before believing
# "survived". R68: place the mutant where the original put the statement.
# R70: re-run after any remediation. R82: include the CANCELLING direction.
set -uo pipefail
SRC=${SRC:-/root/pw}
MUT=/root/mut
F=internal/finance/amortization/fancybisect.go
OUT=/tmp/mutants.txt
: > "$OUT"

run_mutant() {  # $1=name  $2=python-edit-script
  rm -rf "$MUT"; cp -a "$SRC" "$MUT"
  local before after
  before=$(md5sum "$MUT/$F" | cut -d' ' -f1)
  python3 - "$MUT/$F" <<PYEOF
import sys
p = sys.argv[1]
s = open(p).read()
$2
open(p, 'w').write(s)
PYEOF
  after=$(md5sum "$MUT/$F" | cut -d' ' -f1)
  if [ "$before" = "$after" ]; then
    echo "MUTANT $1: *** DID NOT APPLY *** (file unchanged) — R77" | tee -a "$OUT"
    return
  fi
  ( cd "$MUT" && PERSENSE_REQUIRE_ORACLE=1 PERSENSE_ORACLE=/tmp/oraclebuild/amort_oracle \
      go test -count=1 -run 'TestR58' ./internal/finance/amortization \
      > /tmp/mut_$1.log 2>&1 )
  local rc=$?
  if [ $rc -eq 0 ]; then
    echo "MUTANT $1: SURVIVED  (applied: $before -> $after)  *** GAP ***" | tee -a "$OUT"
  else
    local killers
    killers=$(grep -oE '^\s*--- FAIL: (TestR58[A-Za-z0-9]*)' /tmp/mut_$1.log \
              | sed 's/.*--- FAIL: //' | sort -u | tr '\n' ' ')
    if [ -z "$killers" ]; then
      killers="(BUILD FAILURE or no --- FAIL line: $(tail -2 /tmp/mut_$1.log | head -1))"
    fi
    echo "MUTANT $1: KILLED BY: $killers (applied: $before -> $after)" | tee -a "$OUT"
  fi
}


# M1 — REMOVE THE §99 OVERSHOOT ENTIRELY (back to round 25's bare clamp).
# This is the mutant that PRODUCES the effect the gate watches (R84).
run_mutant M1_no_overshoot \
"old = 'if dateutil.DateOK(veryLast) &&\n\t\t\t\t\tdateutil.DateComp(veryLast, hLastDate) == 0 {'
new = 'if false {'
assert s.count(old) == 1, 'M1 anchor count %d' % s.count(old)
s = s.replace(old, new)"

# M2 — R82's CANCELLING DIRECTION: delete the very_last guard, keep the
# overshoot. This is the r52 shape that booked NEW = 1.
run_mutant M2_guard_removed \
"old = 'if dateutil.DateOK(veryLast) &&\n\t\t\t\t\tdateutil.DateComp(veryLast, hLastDate) == 0 {'
new = 'if true {'
assert s.count(old) == 1, 'M2 anchor count %d' % s.count(old)
s = s.replace(old, new)"

# M3 — INVERT the guard. Admits exactly the screens it should reject and
# rejects exactly the ones it should admit.
run_mutant M3_guard_inverted \
"old = 'dateutil.DateComp(veryLast, hLastDate) == 0 {'
new = 'dateutil.DateComp(veryLast, hLastDate) != 0 {'
assert s.count(old) == 1, 'M3 anchor count %d' % s.count(old)
s = s.replace(old, new)"

# M4 — CONTENT, NOT PRESENCE: keep the branch and the guard, but never let the
# overshoot move the bound (the 'computed and never used' fail-open shape, R84).
run_mutant M4_bound_never_moves \
"old = '\t\t\t\t\tif dateutil.DateComp(dt, bound) > 0 {\n\t\t\t\t\t\tbound = dt\n\t\t\t\t\t}'
assert s.count(old) == 1, 'M4 anchor count %d' % s.count(old)
s = s.replace(old, '\t\t\t\t\t_ = dt')"


# M6 — the counters lie (a positive control that cannot fail is not a control).
run_mutant M6_extended_counter_never_set \
"old = 'settings.segHorizonStats[\"extended\"]++'
assert s.count(old) == 1, 'M6 anchor count %d' % s.count(old)
s = s.replace(old, '_ = settings.segHorizonStats')"

# CONTROL — R77's second half: PROVE THE KILLER-EXTRACTOR WORKS. Guaranteed to
# break the headline value; an empty "KILLED BY:" here means the extractor is
# broken and every line above is untrustworthy.
run_mutant CONTROL_extractor \
"old = 'sub.Loan.LastDate = bound\n\t\t\t\t\t\tif settings.segHorizonStats != nil {'
assert s.count(old) == 1, 'CONTROL anchor count %d' % s.count(old)
s = s.replace(old, 'sub.Loan.LastDate = hLastDate\n\t\t\t\t\t\tif settings.segHorizonStats != nil {')"

# M7 — remove the 'never past derived' cap. Reported honestly whichever way it
# goes: on both pinned screens bound == derived, so a survival is a statement
# about the PINS' reach, not about the cap being unnecessary.
run_mutant M7_derived_cap_removed \
"old = '\t\t\t\t\tif dateutil.DateComp(bound, derived) > 0 {\n\t\t\t\t\t\tbound = derived\n\t\t\t\t\t}'
assert s.count(old) == 1, 'M7 anchor count %d' % s.count(old)
s = s.replace(old, '')"

# M5 — (RESTORED so its cited result is reproducible from the tree, r58 pass 2
# finding 3). An NPeriods raise loop copied from the short arm. It is DEAD on
# both pins, so it SURVIVES — which is exactly why the loop was removed from the
# fix rather than kept and "tested".
run_mutant M5_nperiods_raise_readded \
"old = '\t\t\t\t\t\tsub.Loan.LastDate = bound'
assert s.count(old) == 1, 'M5 anchor count %d' % s.count(old)
s = s.replace(old, '\t\t\t\t\t\tn := sub.Loan.NPeriods\n\t\t\t\t\t\td2 := sub.Loan.LastDate\n\t\t\t\t\t\tfor i := 0; i < maxSegmentPeriods; i++ {\n\t\t\t\t\t\t\tnd, e := dateutil.AddPeriod(d2, sub.Loan.PerYr, day, false)\n\t\t\t\t\t\t\tif e != nil || dateutil.DateComp(nd, bound) > 0 || dateutil.DateComp(nd, d2) <= 0 { break }\n\t\t\t\t\t\t\td2 = nd\n\t\t\t\t\t\t\tn++\n\t\t\t\t\t\t}\n\t\t\t\t\t\tsub.Loan.NPeriods = n\n\t\t\t\t\t\tsub.Loan.LastDate = bound')"

# M10 — WIDEN the guard back to the r58 first-edition `<= 0`. Both pins are `eq`
# screens, so this SURVIVES: the pins have no power over the lt arm (R84).
run_mutant M10_guard_widened_to_le \
"old = 'dateutil.DateComp(veryLast, hLastDate) == 0 {'
assert s.count(old) == 1, 'M10 anchor count %d' % s.count(old)
s = s.replace(old, 'dateutil.DateComp(veryLast, hLastDate) <= 0 {')"

# M11 — re-add the Pascal-wrong `!DateOK` disjunct. SURVIVES, same reason.
run_mutant M11_notok_disjunct_readded \
"old = 'if dateutil.DateOK(veryLast) &&\n\t\t\t\t\tdateutil.DateComp(veryLast, hLastDate) == 0 {'
assert s.count(old) == 1, 'M11 anchor count %d' % s.count(old)
s = s.replace(old, 'if !dateutil.DateOK(veryLast) ||\n\t\t\t\t\tdateutil.DateComp(veryLast, hLastDate) == 0 {')"

echo; echo '=== SUMMARY ==='; cat "$OUT"
