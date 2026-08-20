#!/usr/bin/env bash
# r61_tooltip_mutants.sh — the mutation harness for the CLICK-ONLY tooltip
# change (client UI item #10, round 61). COMMITTED, because an uncommitted
# instrument makes its finding unfalsifiable in both directions (R32).
#
# Run:   bash testplan/harness/r61_tooltip_mutants.sh
# Needs: node with `playwright` resolvable, a chromium at $CHROME, and python3
#        (for the throwaway static server). NO ORACLE and NO GO — this is a
#        display-layer change; the engine is untouched (rule 2 / R37).
#
# 🚨 THE TAILWIND CAVEAT, AND IT IS LOAD-BEARING. The shipped page pulls
#    https://cdn.tailwindcss.com, and Tailwind is what supplies
#    `.hidden { display: none }` — the inline stylesheet has no such rule.
#    Chromium inside the project container does NOT fetch that CDN, so a page
#    served naively renders with EVERY `.modal-overlay.hidden` covering the
#    viewport and swallowing clicks. This harness therefore vendors the CDN
#    script into the MUTANT COPY ONLY and rewrites the tag there. The shipped
#    file is never touched. Measure without this and you are measuring the
#    container, not the product — see the r61 record on §95.
#
# 🚨 AND IT VERIFIES THE SERVED BYTES, NOT ONLY THE FILE ON DISK. See the
# serve-check in run_mutant: a stale server holding the port made an entire
# r61 run measure the wrong tree while every mutant was "proven applied".
#
# Shape from r59_date_mask_mutants.sh: R77 (prove each mutant APPLIED by md5
# before believing "survived"), R68 (place the mutant where the original put
# the statement), R82 (mutate in the CANCELLING direction), a CONTROL that must
# be KILLED to prove the killer-extractor works, and a NO-OP control that must
# SURVIVE to prove the harness is not killing everything.
#
# Each mutant is an EXACT old/new text pair, replaced with an asserted count
# of 1. No regexes, no nested quoting.
#
# MEASURED AT r61 CLOSE against index.html md5 $
# (the file AFTER every r61 edit): 17 mutants — 14 KILLED (CONTROL_extractor
# among them, by a NAMED assertion), and 3 SURVIVED: NOOP_equivalent as
# intended, plus raf_guard_removed and capture_flag_dropped, both reported in
# the list below rather than quietly dropped.
# 🚨 RE-RUN IT; DO NOT QUOTE THIS BLOCK. Any later edit to index.html — even a
# comment — makes the md5 above stale, and a stale anchor is how R77 gets you.
#   M1  hover_reveal_restored     the reveal path item #10 asked us to remove
#   M2  focus_reveal_restored     the same reveal, by keyboard
#   M3  route_always_modal        every tip back to the "black pop-up"
#   M4  route_always_bubble       a "Learn more" link stranded in a bubble
#   M5  route_ignores_links       the link half of the routing rule
#   M6  threshold_collapsed       TIP_BUBBLE_MAX made meaningless
#   M7  toggle_removed            a second click on the same icon
#   M8  outside_dismiss_removed   click-away, the primary dismisser
#   M9  scroll_dismiss_removed    a fixed bubble drifting off its icon
#   M10 capture_flag_dropped      ⚠️ EXPECTED TO SURVIVE. It was listed as a
#                                 kill until an audit showed the assertion that
#                                 killed it had stubbed out `onMtgHeaderClick`
#                                 — deleting the `.tip` guard that has been in
#                                 that function since long before r61. Capture
#                                 phase has NO measured user-visible effect in
#                                 this page: no ancestor of any tip has a click
#                                 handler that would fire, and none calls
#                                 stopPropagation. It is kept because it is the
#                                 correct phase for a document-level dismisser,
#                                 NOT because a test pins it.
#   M11 escape_dismiss_removed    Escape
#   M12 enter_activate_removed    keyboard parity with the mouse
#   M13 pointer_events_none       an open bubble that clicks fall THROUGH
#   M14 raf_guard_removed         ⚠️ SURVIVED at r61 — reported, not hidden.
#                                 No constructible sequence makes the guard
#                                 observable; the element is `hidden` in every
#                                 state it covers. Kept as cheap defence, and
#                                 the source says at the line that nothing
#                                 pins it. Do NOT quietly delete this mutant
#                                 to make the harness look clean.
#   CONTROL_extractor             must be KILLED BY A NAMED ASSERTION.
#                                 ⚠️ On its first r61 run it killed by CRASHING
#                                 and the extractor printed nothing — which
#                                 proves nothing. The test was made null-safe
#                                 and the control re-run. A crash is not a kill.
#   NOOP_equivalent               must SURVIVE — proves we kill on meaning
set -uo pipefail
SRC=${SRC:-/root/pw}
MUT=${MUT:-/root/mut61}
CHROME=${CHROME:-/opt/pw-browsers/chromium-1194/chrome-linux/chrome}
TWJS=${TWJS:-/tmp/tw.js}          # a local copy of cdn.tailwindcss.com
PORT=${PORT:-8899}
CAP=${R61_TIPS_PER_SCREEN:-2}
F=cmd/persense/static/index.html
TEST=testplan/harness/r61_tooltip_click_test.js
OUT=/tmp/r61_mutants.txt
: > "$OUT"

if [ ! -s "$TWJS" ]; then
  echo "fetching cdn.tailwindcss.com -> $TWJS"
  curl -sSL -o "$TWJS" https://cdn.tailwindcss.com || { echo "FATAL: no tailwind copy"; exit 2; }
fi

run_mutant() {  # $1=name  $2=exact old text  $3=exact new text
  local name="$1"
  rm -rf "$MUT"; cp -a "$SRC" "$MUT"
  local before after
  before=$(md5sum "$MUT/$F" | cut -d' ' -f1)
  if [ -n "$2" ]; then
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
  fi
  after=$(md5sum "$MUT/$F" | cut -d' ' -f1)
  if [ -n "$2" ] && [ "$before" = "$after" ]; then
    echo "MUTANT $name: *** DID NOT APPLY *** (file unchanged) — R77" | tee -a "$OUT"
    return
  fi
  # Probe-only: vendor Tailwind into the mutant copy. Never the shipped file.
  cp "$TWJS" "$MUT/cmd/persense/static/tailwind-cdn-local.js"
  python3 - "$MUT/$F" <<'PYEOF'
import sys
p = sys.argv[1]
s = open(p, encoding='utf-8').read()
old = '<script src="https://cdn.tailwindcss.com"></script>'
assert s.count(old) == 1, 'tailwind tag not found exactly once'
open(p, 'w', encoding='utf-8').write(s.replace(old, '<script src="/tailwind-cdn-local.js"></script>', 1))
PYEOF
  # 🚨🚨 SERVE-CHECK. R77 SAYS PROVE THE MUTANT APPLIED; IT IS NOT ENOUGH.
  # An earlier r61 run left a stale `python3 -m http.server` holding this port.
  # Every subsequent bind failed SILENTLY, so all 17 mutants were measured
  # against a tree that was not the mutant — md5-proven applied ON DISK and
  # never reached OVER THE WIRE, and the run produced 17 identical "kills"
  # including the NO-OP control, which is what gave it away.
  # Kill anything already on the port, start the server WITHOUT a subshell so
  # $! is the server's own pid, then verify the SERVED bytes match the mutant.
  local stale
  stale=$(ps -eo pid,args | grep "[h]ttp.server $PORT" | awk '{print $1}')
  [ -n "$stale" ] && { echo "  (killing stale server(s) on $PORT: $stale)"; kill $stale 2>/dev/null; sleep 1; }
  local here; here=$(pwd)
  cd "$MUT/cmd/persense/static" || return
  python3 -m http.server "$PORT" >/dev/null 2>&1 &
  local srvpid=$!
  cd "$here" || return
  sleep 2
  local served ondisk
  served=$(curl -s "http://127.0.0.1:$PORT/index.html" | md5sum | cut -d' ' -f1)
  ondisk=$(md5sum "$MUT/$F" | cut -d' ' -f1)
  if [ "$served" != "$ondisk" ]; then
    echo "MUTANT $name: *** SERVER IS NOT SERVING THIS MUTANT *** (served $served, on disk $ondisk) — R77/R94" | tee -a "$OUT"
    kill "$srvpid" 2>/dev/null; sleep 1
    return
  fi
  R61_TIPS_PER_SCREEN="$CAP" R61_SHOW=6 PW_CHROME="$CHROME" \
    node "$SRC/$TEST" "http://127.0.0.1:$PORT" > "/tmp/r61mut_$name.log" 2>&1
  local rc=$?
  kill "$srvpid" 2>/dev/null; sleep 1
  if [ $rc -eq 0 ]; then
    echo "MUTANT $name: SURVIVED  (applied: $before -> $after)" | tee -a "$OUT"
  else
    local killers
    killers=$(grep -oE '^  ✗ .*' "/tmp/r61mut_$name.log" | sed 's/^  ✗ //' | sed 's/^[a-z]*\/[0-9]*: //' | sort -u | head -3 | tr '\n' ';')
    [ -z "$killers" ] && killers="(CRASH or no ✗ line: $(tail -2 /tmp/r61mut_$name.log | head -1))"
    echo "MUTANT $name: KILLED BY: $killers (applied: $before -> $after)" | tee -a "$OUT"
  fi
}

run_mutant hover_reveal_restored "$(cat <<'X'
  // Enter / Space on the focused icon is the keyboard equivalent of a click.
X
)" "$(cat <<'X'
  document.addEventListener('mouseover', function (e) {
    const tip = e.target && e.target.closest && e.target.closest('.tip');
    if (tip) showBubble(tip);
  });
  // Enter / Space on the focused icon is the keyboard equivalent of a click.
X
)"

run_mutant focus_reveal_restored "$(cat <<'X'
  // Enter / Space on the focused icon is the keyboard equivalent of a click.
X
)" "$(cat <<'X'
  document.addEventListener('focusin', function (e) {
    const tip = e.target && e.target.closest && e.target.closest('.tip');
    if (tip) showBubble(tip);
  });
  // Enter / Space on the focused icon is the keyboard equivalent of a click.
X
)"

run_mutant route_always_modal "$(cat <<'X'
    if (/<a[\s>]/i.test(body) || body.indexOf('tip-more') !== -1) return 'modal';
X
)" "$(cat <<'X'
    if (true) return 'modal';
X
)"

run_mutant route_always_bubble "$(cat <<'X'
    if (/<a[\s>]/i.test(body) || body.indexOf('tip-more') !== -1) return 'modal';
    return plainLen(body) > TIP_BUBBLE_MAX ? 'modal' : 'bubble';
X
)" "$(cat <<'X'
    return 'bubble';
X
)"

run_mutant route_ignores_links "$(cat <<'X'
    if (/<a[\s>]/i.test(body) || body.indexOf('tip-more') !== -1) return 'modal';
X
)" "$(cat <<'X'
    if (false) return 'modal';
X
)"

run_mutant threshold_collapsed "$(cat <<'X'
  const TIP_BUBBLE_MAX = 200;
X
)" "$(cat <<'X'
  const TIP_BUBBLE_MAX = 20;
X
)"

run_mutant toggle_removed "$(cat <<'X'
    if (openTip === tip && bubbleOpen()) { hideBubble(); return; }
X
)" "$(cat <<'X'
    if (false) { hideBubble(); return; }
X
)"

run_mutant outside_dismiss_removed "$(cat <<'X'
          e.target.closest && e.target.closest('a')) return;
      hideBubble();
X
)" "$(cat <<'X'
          e.target.closest && e.target.closest('a')) return;
X
)"

run_mutant scroll_reanchor_removed "$(cat <<'X'
  window.addEventListener('scroll', reanchorBubble, true);
X
)" "$(cat <<'X'
  window.addEventListener('scroll', function () { /* mutant: never re-anchor */ }, true);
X
)"

# 🚨 THIS MUTANT RESTORES r61's OWN WITHDRAWN FIRST CUT, the way r59's harness
# restores its first cut. `scroll -> hideBubble()` passed every test the round
# had at the time and ate the user's click. It must never come back.
run_mutant scroll_dismisses_unconditionally "$(cat <<'X'
      _tipReanchorPending = false;
      positionBubble();
X
)" "$(cat <<'X'
      _tipReanchorPending = false;
      hideBubble();
X
)"

run_mutant capture_flag_dropped "$(cat <<'X'
      hideBubble();
    }
  }, true);
X
)" "$(cat <<'X'
      hideBubble();
    }
  }, false);
X
)"

run_mutant escape_dismiss_removed "$(cat <<'X'
      if (e.key === 'Escape') {
        hideBubble();
X
)" "$(cat <<'X'
      if (e.key === 'Escape') {
X
)"

run_mutant enter_activate_removed "$(cat <<'X'
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      activate(tip);
    }
X
)" "$(cat <<'X'
    if (false) {
      e.preventDefault();
      activate(tip);
    }
X
)"

run_mutant pointer_events_none "$(cat <<'X'
      position: fixed; z-index: 10000; pointer-events: auto;
X
)" "$(cat <<'X'
      position: fixed; z-index: 10000; pointer-events: none;
X
)"

run_mutant raf_guard_removed "$(cat <<'X'
      if (openTip !== tip || bubbleEl.hidden) return;
X
)" "$(cat <<'X'
      if (false) return;
X
)"

run_mutant CONTROL_extractor "$(cat <<'X'
      e.stopPropagation();
      activate(tip);
      return;
X
)" "$(cat <<'X'
      e.stopPropagation();
      return;
X
)"

run_mutant NOOP_equivalent "$(cat <<'X'
    return plainLen(body) > TIP_BUBBLE_MAX ? 'modal' : 'bubble';
X
)" "$(cat <<'X'
    return (plainLen(body) > TIP_BUBBLE_MAX) ? 'modal' : 'bubble';
X
)"

echo
echo "==== SUMMARY ===="
cat "$OUT"
