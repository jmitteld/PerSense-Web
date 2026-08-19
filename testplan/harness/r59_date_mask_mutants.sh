#!/usr/bin/env bash
# r59_date_mask_mutants.sh — the mutation harness for the date-field correction
# fix (client UI report #8, round 59). COMMITTED, because an uncommitted
# instrument makes its finding unfalsifiable in both directions (R32).
#
# Run:   bash testplan/harness/r59_date_mask_mutants.sh
# Needs: node and go on PATH, and SRC below pointing at the tree. NO ORACLE —
#        this is a display-layer fix; the engine is untouched.
#
# Shape from r58_mutants.sh: R77 (prove each mutant APPLIED by md5 before
# believing "survived" — and this harness caught itself failing to apply on its
# first run), R68 (place the mutant where the original put the statement), R82
# (mutate in the CANCELLING direction), a CONTROL that must be killed to prove
# the killer-extractor works, and a NO-OP control that must SURVIVE to prove the
# harness is not killing everything.
#
# Each mutant is an EXACT old/new text pair, replaced with an asserted count of
# 1. No regexes, no nested quoting.
#
# EXPECTED AT r59 CLOSE — fifteen killed, one surviving. The list grew
# TWICE, each time from an adversarial pass refuting the previous cut:
# EXPECTED — twenty killed, one surviving:
#   M1 segment_branch_disabled KILLED  disabling the segment-preserving branch
#                                     sends every correction back through the
#                                     spill + re-slice, i.e. the original defect
#   M2 firstcut_truncate_year KILLED  this round's FIRST cut: truncating an
#                                     over-long year turned a blocked field into
#                                     a silently ACCEPTED wrong date
#   M3 firstcut_autocomplete  KILLED  this round's FIRST cut: the typing branch
#                                     discarding the tail behind a one-digit
#                                     segment (1/15/ + "2" collapsed to "1")
#   M4 empty_reset_removed    KILLED  a field of nothing but delimiters
#   M6 spill_disabled         KILLED  deleting a delimiter stops re-flowing
#   M7 early_return_always    KILLED  the mask never assigns — proves the sweep
#                                     has power over the CODE and not merely
#                                     over the test harness's own splice
#   M8 caret_off_by_one       KILLED  the caret block is reached
#   M9  truncate_unconditional     KILLED  pass 2: the year cap must not repair
#                                          a field forward typing cannot make
#   M10 truncate_gate_drops_rawseg KILLED  ... judged on the RAW segments
#   M11 truncate_gate_drops_length KILLED  ... and only one digit over
#   M12 fourth_segment_dropped     KILLED  a stray delimiter ate the year
#   M13 split_on_locked_only       KILLED  a "-"-locked field holding slashes
#   M14 autocomplete_year_half     KILLED  the `|| yy` half of the F1 fix
#   M15 autocomplete_day_half      KILLED  the `|| dd` half of the F1 fix
#   M16 trailing_empty_kept        KILLED  pass 4: deleting a year must leave
#                                          "12/15", the shape the smart-year
#                                          inference already reads
#   M16b smartyear_regex_widened   KILLED  pass 4: and the inference must NOT be
#                                          widened to fire on a TYPED "12/15/"
#   M16c caret_counts_from_out     KILLED  pass 4: the caret counts digits in
#                                          the BROWSER's value, not the output
#   M16d derived_write_ungated     ?       pass 4: a field the validator rejects
#                                          must not drive a write into another
#   M17 yearauto_strip_ungated     KILLED  pass 3: the century strip on a
#                                          non-canonical (pasted) field
#   M18 yearauto_never_cleared     KILLED  pass 3: a correction must clear it
#   M19 splitOn_guard_dropped      KILLED  pass 3: only fall back when the
#                                          locked delimiter is ABSENT
#   M20 fold_prepends              KILLED  pass 3: the 4th segment's digits go
#                                          AFTER the year's, not before
#   CONTROL_extractor              KILLED  R77: the killer-extractor works
#   NOOP_kept_rewritten       SURVIVES an equivalent rewrite
#
# M2 and M3 are the mutants that matter most: they restore THIS ROUND'S OWN
# FIRST ATTEMPT, which passed a green suite and was then refuted by an
# adversarial audit. They exist so that regression cannot be made twice.
set -uo pipefail
SRC=${SRC:-/root/pw}
MUT=/root/mut59
F=cmd/persense/static/index.html
TESTS='TestFrontendDateFieldCorrectionJS|TestFrontendDateDelimiterJS|TestExpandDOBYearJS|TestFrontendLoanDateYearJS'
OUT=/tmp/r59_mutants.txt
: > "$OUT"

run_mutant() {  # $1=name  $2=exact old text  $3=exact new text
  local name="$1"
  rm -rf "$MUT"; cp -a "$SRC" "$MUT"
  local before after
  before=$(md5sum "$MUT/$F" | cut -d' ' -f1)
  MUT_OLD="$2" MUT_NEW="$3" python3 - "$MUT/$F" <<'PYEOF'
import os, sys
p = sys.argv[1]
s = open(p, encoding='utf-8').read()
old, new = os.environ['MUT_OLD'], os.environ['MUT_NEW']
n = s.count(old)
if n != 1:
    sys.stderr.write('anchor matched %d times\n' % n)
    sys.exit(1)
open(p, 'w', encoding='utf-8').write(s.replace(old, new))
PYEOF
  after=$(md5sum "$MUT/$F" | cut -d' ' -f1)
  if [ "$before" = "$after" ]; then
    echo "MUTANT $name: *** DID NOT APPLY *** (file unchanged) — R77" | tee -a "$OUT"
    return
  fi
  ( cd "$MUT/cmd/persense" && go test -count=1 -run "$TESTS" . > /tmp/r59mut_$name.log 2>&1 )
  local rc=$?
  if [ $rc -eq 0 ]; then
    echo "MUTANT $name: SURVIVED  (applied: $before -> $after)" | tee -a "$OUT"
  else
    local killers
    killers=$(grep -oE '^\s*--- FAIL: (Test[A-Za-z0-9]*)' /tmp/r59mut_$name.log \
              | sed 's/.*--- FAIL: //' | sort -u | tr '\n' ' ')
    [ -z "$killers" ] && killers="(BUILD FAILURE or no --- FAIL line: $(tail -2 /tmp/r59mut_$name.log | head -1))"
    echo "MUTANT $name: KILLED BY: $killers (applied: $before -> $after)" | tee -a "$OUT"
  fi
}

run_mutant segment_branch_disabled "$(cat <<'X'
    } else if (segs.length >= 3) {
X
)" "$(cat <<'X'
    } else if (false) {
X
)"

run_mutant firstcut_truncate_year "$(cat <<'X'
      out = kept.slice(0, last).join(delim);
X
)" "$(cat <<'X'
      out = kept.slice(0, last).map(function (k, i) { return i === 2 ? k.slice(0, 4) : k; }).join(delim);
X
)"

run_mutant firstcut_autocomplete "$(cat <<'X'
    if (mm.length === 2 || dd || yy) {
      out += delim + dd;
      if (dd.length === 2 || yy) out += delim + yy;
    }
X
)" "$(cat <<'X'
    if (mm.length === 2) {
      out += delim + dd;
      if (dd.length === 2) out += delim + yy;
    }
X
)"

run_mutant empty_reset_removed "$(cat <<'X'
    if (!(kept[0] + kept[1] + kept[2])) {
X
)" "$(cat <<'X'
    if (false) {
X
)"

run_mutant spill_disabled "$(cat <<'X'
  if (mm.length > 2) { dd = mm.slice(2) + dd; mm = mm.slice(0, 2); }
  if (dd.length > 2) { yy = dd.slice(2) + yy; dd = dd.slice(0, 2); }
X
)" "$(cat <<'X'
  if (mm.length > 2) { mm = mm.slice(0, 2); }
  if (dd.length > 2) { dd = dd.slice(0, 2); }
X
)"

run_mutant early_return_always "$(cat <<'X'
  if (out === raw) return;
X
)" "$(cat <<'X'
  if (true) return;
X
)"

run_mutant caret_off_by_one "$(cat <<'X'
    while (pos < out.length && seen < digitsBefore) {
X
)" "$(cat <<'X'
    while (pos < out.length && seen < digitsBefore + 1) {
X
)"

run_mutant truncate_unconditional "$(cat <<'X'
    if (!overlongSegment && mm.length === 2 && dd.length === 2 && yy.length <= 5) {
      yy = yy.slice(0, 4);
    }
X
)" "$(cat <<'X'
    yy = yy.slice(0, 4);
X
)"

run_mutant truncate_gate_drops_rawseg "$(cat <<'X'
    if (!overlongSegment && mm.length === 2 && dd.length === 2 && yy.length <= 5) {
X
)" "$(cat <<'X'
    if (mm.length === 2 && dd.length === 2 && yy.length <= 5) {
X
)"

run_mutant truncate_gate_drops_length "$(cat <<'X'
    if (!overlongSegment && mm.length === 2 && dd.length === 2 && yy.length <= 5) {
X
)" "$(cat <<'X'
    if (!overlongSegment && mm.length === 2 && dd.length === 2) {
X
)"

run_mutant fourth_segment_dropped "$(cat <<'X'
    if (segs.length > 3) {
      kept[2] += segs.slice(3).join('').replace(/\D/g, '');
    }
X
)" "$(cat <<'X'
    if (false) {
      kept[2] += segs.slice(3).join('').replace(/\D/g, '');
    }
X
)"

run_mutant split_on_locked_only "$(cat <<'X'
  const segs = raw.split(splitOn);
X
)" "$(cat <<'X'
  const segs = raw.split(delim);
X
)"

run_mutant autocomplete_year_half "$(cat <<'X'
      if (dd.length === 2 || yy) out += delim + yy;
X
)" "$(cat <<'X'
      if (dd.length === 2) out += delim + yy;
X
)"

run_mutant autocomplete_day_half "$(cat <<'X'
    if (mm.length === 2 || dd || yy) {
X
)" "$(cat <<'X'
    if (mm.length === 2 || yy) {
X
)"

run_mutant trailing_empty_kept "$(cat <<'X'
      let last = kept.length;
      while (last > 1 && kept[last - 1] === '') last--;
      out = kept.slice(0, last).join(delim);
X
)" "$(cat <<'X'
      out = kept[0] + delim + kept[1] + delim + kept[2];
X
)"

run_mutant smartyear_regex_widened "$(cat <<'X'
  const m = String(raw || '').trim().match(/^(\d{1,2})[\/-](\d{1,2})$/);
X
)" "$(cat <<'X'
  const m = String(raw || '').trim().match(/^(\d{1,2})[\/-](\d{1,2})[\/-]?$/);
X
)"

run_mutant caret_counts_from_out "$(cat <<'X'
    const digitsBefore = raw.slice(0, caret).replace(/\D/g, '').length;
X
)" "$(cat <<'X'
    const digitsBefore = out.slice(0, caret).replace(/\D/g, '').length;
X
)"

run_mutant derived_write_ungated "$(cat <<'X'
  if (!dateValidity(loanEl.value).valid) return; // don't derive from a field we reject
X
)" "$(cat <<'X'
  if (false) return; // don't derive from a field we reject
X
)"

run_mutant yearauto_strip_ungated "$(cat <<'X'
    if (el.dataset.yearAuto && yy.length > 4 &&
        mm.length === 2 && dd.length === 2 &&
        yy.slice(0, 2) === el.dataset.yearAuto.slice(0, 2)) {
X
)" "$(cat <<'X'
    if (el.dataset.yearAuto && yy.length > 4 &&
        yy.slice(0, 2) === el.dataset.yearAuto.slice(0, 2)) {
X
)"

run_mutant yearauto_never_cleared "$(cat <<'X'
    el.dataset.yearAuto = '';
    // The user is CORRECTING, not entering.
X
)" "$(cat <<'X'
    // The user is CORRECTING, not entering.
X
)"

run_mutant splitOn_guard_dropped "$(cat <<'X'
  const splitOn = (raw.indexOf(delim) < 0 && raw.indexOf(delim === '/' ? '-' : '/') >= 0)
X
)" "$(cat <<'X'
  const splitOn = (raw.indexOf(delim === '/' ? '-' : '/') >= 0)
X
)"

run_mutant fold_prepends "$(cat <<'X'
      kept[2] += segs.slice(3).join('').replace(/\D/g, '');
X
)" "$(cat <<'X'
      kept[2] = segs.slice(3).join('').replace(/\D/g, '') + kept[2];
X
)"

run_mutant CONTROL_extractor "$(cat <<'X'
      out = kept.slice(0, last).join(delim);
X
)" "$(cat <<'X'
      out = 'XX';
X
)"

run_mutant NOOP_kept_rewritten "$(cat <<'X'
    const kept = [0, 1, 2].map(function (i) {
      return (segs[i] || '').replace(/\D/g, '');
    });
X
)" "$(cat <<'X'
    const kept = [segs[0], segs[1], segs[2]].map(function (v) {
      return (v || '').replace(/[^0-9]/g, '');
    });
X
)"

echo
echo '===== r59 date-mask mutation summary ====='
cat "$OUT"
