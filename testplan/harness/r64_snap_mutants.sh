#!/usr/bin/env bash
# r64_snap_mutants.sh — does r64_snap_ui_test.js have any power?
#
# Reverts each round-64 change in turn, rebuilds the server FROM THE MUTATED
# TREE (R98), proves the mutant is what is actually being SERVED (R94/R99), runs
# the browser harness, and records WHICH NAMED ASSERTION killed it (R102).
#
# 🚨 A MUTANT THAT DOES NOT APPLY IS NOT A KILL, AND NEITHER IS ONE THAT DOES NOT
# BUILD. Both are reported as errors and make the run fail. r63 recorded two
# non-compiling mutants as kills before R102 was written.
# 🚨 THE FIELD SEPARATOR IS `~`, NOT `|`. r62's mutant list used `|` and silently
# split two entries containing a Go `||`.
#
#   bash testplan/harness/r64_snap_mutants.sh [--port 8871]
#
# Exit 0 = every mutant killed by a named assertion. Non-zero = a survivor, a
# mutant that did not apply, or a build failure.

set -uo pipefail
cd "$(dirname "$0")/../.." || exit 2
ROOT=$(pwd)
PORT=8871
[ "${1:-}" = "--port" ] && PORT="${2:-8871}"

HTML="cmd/persense/static/index.html"
HANDLERS="internal/api/handlers.go"
WORK=$(mktemp -d /tmp/r64mut.XXXXXX)
BIN="$WORK/persense"
LOG="$WORK/run.log"
URL="http://127.0.0.1:$PORT/"

cp "$HTML" "$WORK/index.html.orig"
cp "$HANDLERS" "$WORK/handlers.go.orig"
restore() { cp "$WORK/index.html.orig" "$HTML"; cp "$WORK/handlers.go.orig" "$HANDLERS"; }
trap 'restore; pkill -f "r64mut.*persense .*-port $PORT" >/dev/null 2>&1; ' EXIT

# name ~ file ~ old ~ new
MUTANTS=(
"cue-css-inert~$HTML~      border-color: var(--border-snapped) !important;~      border-color: var(--border-cell);"
"cue-not-applied~$HTML~  el.classList.add('cell-snapped');~  if (false) el.classList.add('cell-snapped');"
"cue-no-title~$HTML~  if (note) el.title = note;~  if (false && note) el.title = note;"
"cue-not-cleared-on-edit~$HTML~    clearSnappedCell(t);~    if (false) clearSnappedCell(t);"
"pv-no-repaint~$HTML~    r.el.value = fmtDateDisplay(r.iso);\n    r.el.classList.remove('cell-output');\n  });\n  applySnapMarks(document.getElementById('screen-presentvalue')~    r.el.classList.remove('cell-output');\n  });\n  applySnapMarks(document.getElementById('screen-presentvalue')"
"pv-no-notes~$HTML~      out.notes.push(note);~      if (false) out.notes.push(note);"
"pv-blank-guard-dropped~$HTML~      if (blanks[f]) return;                       // engine derived it: not a snap~      if (!blanks[f]) return;"
"prepay-no-repaint~$HTML~  prepaySnap.repaint.forEach(function (r) {\n    r.el.value = fmtDateDisplay(r.iso);~  prepaySnap.repaint.forEach(function (r) {"
"prepay-row-index-ignored~$HTML~    const dom = (typeof amzPrepayRowIndex !== 'undefined' && amzPrepayRowIndex[idx] != null)\n      ? amzPrepayRowIndex[idx] : idx;~    const dom = idx;"
"nn-echo-row-index-ignored~$HTML~      const row = rows[amzPrepayRowIndex[idx] != null ? amzPrepayRowIndex[idx] : idx];~      const row = rows[idx];"
"nperiods-wins-note-reverted~$HTML~    if (key === 'lastDate' && body.nPeriods) {~    if (false) {"
"mutual-exclusion-reverted~$HTML~    if (t.id === 'amz-lastDate' || t.id === 'amz-nPeriods') {~    if (false) {"
"sec95-hidden-fallback-reverted~$HTML~    .hidden { display: none; }~    .hidden-r64-disabled { display: none; }"
"autocalc-decline-silent~$HTML~    setAutoCalcHint(screen, true,\n      'Not enough entered yet to calculate. Press Calculate to see exactly what is missing.');~    setAutoCalcHint(screen, false);"
"screen-title-removed~$HTML~    <h2 class=\"screen-title\">Present Value</h2>\n~"
"pv-error-moved-back-to-the-bottom~$HTML~      <div id=\"pv-error\" class=\"error-msg\" role=\"alert\" aria-live=\"assertive\"></div>\n      <div id=\"pv-autocalc-hint\" class=\"autocalc-hint hidden\" aria-live=\"polite\"></div>\n\n      <!-- Actuarial / Life Contingency -->~\n      <!-- Actuarial / Life Contingency -->@@      <div id=\"pv-table-info\" class=\"text-xs hidden\" style=\"color:var(--text-secondary); padding:4px 2px;\"></div>\n      </div>~      <div id=\"pv-table-info\" class=\"text-xs hidden\" style=\"color:var(--text-secondary); padding:4px 2px;\"></div>\n      </div>\n\n      <div id=\"pv-error\" class=\"error-msg\" role=\"alert\" aria-live=\"assertive\"></div>\n      <div id=\"pv-autocalc-hint\" class=\"autocalc-hint hidden\" aria-live=\"polite\"></div>"
"tip-label-width-reverted~$HTML~<label class=\"grid-header inline-block\" style=\"min-width:80px\">As-of Date ~<label class=\"grid-header inline-block\" style=\"width:80px\">As-of Date "
"api-stop-never-reported~$HANDLERS~				if n > 0 && dateutil.DateOK(last) && dateutil.DateComp(last, row.StopDate) != 0 {~				if false {"
"api-stop-echoes-typed-date~$HANDLERS~					prepayStop[i] = last.Time.Format(\"2006-01-02\")~					prepayStop[i] = row.StopDate.Time.Format(\"2006-01-02\")"
)

killed=0; survived=0; broken=0
declare -a SURVIVORS BROKEN

start_server() {
  pkill -f "$BIN -port $PORT" >/dev/null 2>&1
  sleep 0.5
  cat > "$WORK/serve.sh" <<EOF
#!/bin/bash
exec "$BIN" -port $PORT
EOF
  chmod +x "$WORK/serve.sh"
  setsid "$WORK/serve.sh" > "$WORK/serve.log" 2>&1 &
  sleep 2.5
}

# ---- baseline: the harness must PASS on the clean tree, or nothing below means
# ---- anything (R101: a control that never passes is not a control).
echo "=== BASELINE (clean tree) ==="
go build -o "$BIN" ./cmd/persense || { echo "BASELINE BUILD FAILED"; exit 2; }
start_server
CLEAN_MD5=$(curl -s "$URL" | md5sum | cut -d' ' -f1)
TREE_MD5=$(md5sum "$HTML" | cut -d' ' -f1)
if [ "$CLEAN_MD5" != "$TREE_MD5" ]; then
  echo "SERVED PAGE != TREE ($CLEAN_MD5 vs $TREE_MD5) — refusing to run"; exit 2
fi
if ! node testplan/harness/r64_snap_ui_test.js "$URL" > "$LOG" 2>&1; then
  echo "BASELINE HARNESS FAILED ON THE CLEAN TREE — fix that first:"
  grep -E '^\s+FAIL' "$LOG"
  exit 2
fi
BASE_OK=$(grep -c '^  ok   ' "$LOG")
echo "baseline: $BASE_OK assertions pass, 0 fail"
echo

for m in "${MUTANTS[@]}"; do
  NAME="${m%%~*}"; rest="${m#*~}"
  FILE="${rest%%~*}"; EDITS="${rest#*~}"

  restore
  # apply with python so the pattern is literal and the count is asserted.
  # EDITS is one or more `old~new` pairs separated by the literal `@@`, so a
  # mutant that RELOCATES something (delete here, insert there) can be expressed
  # as one revert instead of two half-reverts that each test nothing.
  if ! python3 - "$FILE" "$EDITS" <<'PY'
import sys
path, edits = sys.argv[1], sys.argv[2]
s = open(path, encoding='utf-8').read()
pairs = edits.split('@@')
for p in pairs:
    if '~' not in p:
        print("MALFORMED-EDIT %r" % p[:60]); sys.exit(4)
    old, new = p.split('~', 1)
    old = old.replace('\\n', '\n'); new = new.replace('\\n', '\n')
    if old == new:
        print("DEGENERATE-EDIT"); sys.exit(4)
    n = s.count(old)
    if n != 1:
        print("DID-NOT-APPLY count=%d" % n); sys.exit(3)
    s = s.replace(old, new)
open(path, 'w', encoding='utf-8').write(s)
PY
  then
    echo "!! $NAME: MUTANT DID NOT APPLY (not a kill — R77)"; broken=$((broken+1)); BROKEN+=("$NAME:did-not-apply"); continue
  fi

  if ! go build -o "$BIN" ./cmd/persense > "$WORK/build.log" 2>&1; then
    echo "!! $NAME: MUTANT DOES NOT BUILD (not a kill — R102)"
    head -3 "$WORK/build.log"; broken=$((broken+1)); BROKEN+=("$NAME:build-failed"); continue
  fi
  start_server
  SERVED=$(curl -s "$URL" | md5sum | cut -d' ' -f1)
  WANT=$(md5sum "$HTML" | cut -d' ' -f1)
  if [ "$SERVED" != "$WANT" ]; then
    echo "!! $NAME: THE MUTANT WAS NOT SERVED (R94) served=$SERVED want=$WANT"
    broken=$((broken+1)); BROKEN+=("$NAME:not-served"); continue
  fi
  if [ "$SERVED" = "$CLEAN_MD5" ] && [ "$FILE" = "$HTML" ]; then
    echo "!! $NAME: served page identical to the clean one — mutant is a no-op"
    broken=$((broken+1)); BROKEN+=("$NAME:no-op"); continue
  fi

  node testplan/harness/r64_snap_ui_test.js "$URL" > "$LOG" 2>&1
  rc=$?
  if [ $rc -eq 0 ]; then
    echo "SURVIVED  $NAME"
    survived=$((survived+1)); SURVIVORS+=("$NAME")
  else
    KILLERS=$(grep -E '^  FAIL ' "$LOG" | sed 's/^  FAIL //; s/  --.*//' | head -3 | paste -sd' ; ' -)
    [ -z "$KILLERS" ] && KILLERS="(harness crashed, rc=$rc — investigate, this is NOT a clean kill)"
    echo "killed    $NAME   <-  $KILLERS"
    killed=$((killed+1))
  fi
done

restore
echo
echo "=== $killed killed, $survived survived, $broken not-a-kill (did-not-apply / build-failed / not-served) ==="
[ ${#SURVIVORS[@]} -gt 0 ] && echo "SURVIVORS: ${SURVIVORS[*]}"
[ ${#BROKEN[@]} -gt 0 ] && echo "NOT-A-KILL: ${BROKEN[*]}"
[ $survived -eq 0 ] && [ $broken -eq 0 ] && exit 0
exit 1
