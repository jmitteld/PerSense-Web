#!/usr/bin/env bash
# r63_beta_actuarial_mutants.sh — prove the §3a.20 beta-actuarial gate's tests
# can FAIL, and name which test kills each mutant.
#
# Rule 3: a regression test must be SEEN TO FAIL, in fact, in both directions.
# R68 mutate it; R70 re-run after any remediation; R77 PROVE the mutant applied
# before believing a kill; R94 prove the thing under test was actually built
# from the mutated tree, not picked up from somewhere else.
#
# ⚠️ FIELD SEPARATOR IS `~`, NOT `|`. r62's mutant list used `|` and silently
# split two entries that contained a Go `||` (START_HERE §1).
#
#   bash testplan/harness/r63_beta_actuarial_mutants.sh [repo-root]
set -uo pipefail

REPO=${1:-$(cd "$(dirname "$0")/../.." && pwd)}
WORK=$(mktemp -d /tmp/r63mut.XXXXXX)
trap 'rm -rf "$WORK"' EXIT

GO_SRC="internal/api/handlers.go"
HTML_SRC="cmd/persense/static/index.html"

# name ~ file ~ find ~ replace ~ test-regex-expected-to-fail
MUTANTS=(
  "gate_never_called ~ $GO_SRC ~ 	if gErr := pvBetaActuarialGate(req); gErr != \"\" { ~ 	if gErr := \"\"; gErr != \"\" { ~ TestR63ActuarialRefusedWithoutOptIn"
  "gate_always_open ~ $GO_SRC ~ 	if !req.BetaActuarial { ~ 	if false { ~ TestR63ActuarialRefusedWithoutOptIn"
  "gate_too_wide ~ $GO_SRC ~ 	usesActuarial := req.Actuarial != nil ~ 	usesActuarial := true ~ TestR63VariableRateAndCOLAAreNotGated"
  "bare_act_ignored ~ $GO_SRC ~ 		if actuarial.ContingencyFromCode(ls.Act) != actuarial.NotContingent { ~ 		if false && actuarial.ContingencyFromCode(ls.Act) != actuarial.NotContingent { ~ TestR63BareActCodeIsGated"
  "periodic_act_ignored ~ $GO_SRC ~ 		if actuarial.ContingencyFromCode(pp.Act) != actuarial.NotContingent { ~ 		if false && actuarial.ContingencyFromCode(pp.Act) != actuarial.NotContingent { ~ TestR63BarePeriodicActCodeIsGated"
  "predicate_hand_written ~ $GO_SRC ~ 		if actuarial.ContingencyFromCode(ls.Act) != actuarial.NotContingent { ~ 		if ls.Act != \"\" && ls.Act != \"N\" { ~ TestR63NonContingentCodesAreNotGated"
  "killswitch_literal_one_only ~ $GO_SRC ~ 	switch strings.ToLower(strings.TrimSpace(os.Getenv(\"PERSENSE_DISABLE_ACTUARIAL\"))) {\n	case \"1\", \"true\", \"yes\", \"on\": ~ 	switch os.Getenv(\"PERSENSE_DISABLE_ACTUARIAL\") {\n	case \"1\": ~ TestR63ServerKillSwitch"
  "killswitch_truthy_empty ~ $GO_SRC ~ 	switch strings.ToLower(strings.TrimSpace(os.Getenv(\"PERSENSE_DISABLE_ACTUARIAL\"))) {\n	case \"1\", \"true\", \"yes\", \"on\": ~ 	switch \"x\" + strings.ToLower(strings.TrimSpace(os.Getenv(\"PERSENSE_DISABLE_ACTUARIAL\"))) {\n	case \"x1\", \"xtrue\", \"xyes\", \"xon\", \"x0\", \"xfalse\", \"xno\", \"xoff\", \"x\": ~ TestR63ServerKillSwitch"
  "ships_on_by_default ~ $HTML_SRC ~ <input id=\"set-betaActuarial\" type=\"checkbox\" onchange=\"applyBetaActuarialGate(); updateSettingsBadge()\"> ~ <input id=\"set-betaActuarial\" type=\"checkbox\" onchange=\"applyBetaActuarialGate(); updateSettingsBadge()\" checked> ~ TestR63BetaToggleShipsOffByDefault"
  "hide_by_class_not_attr ~ $HTML_SRC ~ else el.setAttribute('hidden', ''); ~ else el.classList.add('hidden'); ~ TestR63GateUsesTheHiddenAttributeNotTheClass"
  "optin_never_sent ~ $HTML_SRC ~       body.betaActuarial = true; ~       body.betaActuarial = false; ~ TestR63PageSendsTheOptInWhenEnabled"
  "applystate_skips_gate ~ $HTML_SRC ~   applyBetaActuarialGate();\n  expandPVSectionsWithEntries();\n  expandAmzAdvancedWithEntries(); ~   expandPVSectionsWithEntries();\n  expandAmzAdvancedWithEntries(); ~ TestR63ApplyStateReAppliesTheGate"
  "stale_result_kept ~ $HTML_SRC ~     if (pvOutputsCarryContingency()) clearPVEngineOutputs(); ~     if (false) clearPVEngineOutputs(); ~ TestR63SwitchingOffClearsALifeContingentResult"
  "periodic_life_col_unmarked ~ $HTML_SRC ~ Blank = 0%. <a class=\"tip-more\" href=\"/help.html#pv-cola\" target=\"_blank\">Learn more &rarr;</a></span></span></th>\n              <th class=\"grid-header\" data-beta-actu style=\"width:50px\">Life ~ Blank = 0%. <a class=\"tip-more\" href=\"/help.html#pv-cola\" target=\"_blank\">Learn more &rarr;</a></span></span></th>\n              <th class=\"grid-header\" style=\"width:50px\">Life ~ TestR63GateUsesTheHiddenAttributeNotTheClass"
  "actuarial_details_unmarked ~ $HTML_SRC ~ <details class=\"mt-3\" id=\"pv-actuarial-section\" data-beta-actu> ~ <details class=\"mt-3\" id=\"pv-actuarial-section\"> ~ TestR63GateUsesTheHiddenAttributeNotTheClass"
)

pass=0; fail=0; noapply=0
echo "r63 beta-actuarial gate — mutation run"
echo "repo: $REPO"
echo

# Baseline: everything must be GREEN before a kill means anything.
if ! (cd "$REPO" && go test ./internal/api/ -run 'TestR63' -count=1 >"$WORK/base.log" 2>&1); then
  echo "*** BASELINE IS RED — a mutation run against a failing baseline proves nothing."
  tail -20 "$WORK/base.log"; exit 2
fi
echo "baseline: TestR63* GREEN"
echo

for entry in "${MUTANTS[@]}"; do
  IFS='~' read -r name file find replace killer <<< "$entry"
  name=$(echo "$name" | xargs); file=$(echo "$file" | xargs); killer=$(echo "$killer" | xargs)
  # do NOT xargs `find`/`replace` — leading tabs and quotes are load-bearing
  find="${find# }"; find="${find% }"
  replace="${replace# }"; replace="${replace% }"

  TREE="$WORK/$name"
  cp -a "$REPO" "$TREE" 2>/dev/null
  rm -rf "$TREE/.git" 2>/dev/null

  # R77 — PROVE IT APPLIED. A mutant that did not apply reports "survived".
  if ! python3 - "$TREE/$file" "$find" "$replace" <<'PY'
import sys
path, find, replace = sys.argv[1], sys.argv[2], sys.argv[3]
s = open(path).read()
# `\n` in a mutant entry means a real newline: a bash array cannot hold one.
find = find.replace("\\n", "\n")
replace = replace.replace("\\n", "\n")
n = s.count(find)
if n != 1:
    sys.stderr.write("NOT APPLIED: %d occurrences of %r\n" % (n, find[:70]))
    sys.exit(1)
open(path, "w").write(s.replace(find, replace))
PY
  then
    printf "  %-24s *** MUTANT DID NOT APPLY — not a survivor, a harness bug\n" "$name"
    noapply=$((noapply+1)); continue
  fi

  out="$WORK/$name.log"
  (cd "$TREE" && go test ./internal/api/ -run 'TestR63' -count=1 -v) >"$out" 2>&1
  rc=$?

  if grep -qE "^(# |.*\[build failed\]|.*: syntax error)" "$out" && ! grep -q -- "=== RUN" "$out"; then
    printf "  %-24s *** DID NOT COMPILE — NOT A KILL (the compiler caught it, no test ran)\n" "$name"
    grep -m2 -E "^\s+\S+\.go:[0-9]+" "$out" | sed 's/^/      /'
    noapply=$((noapply+1)); continue
  fi

  if [ $rc -ne 0 ]; then
    if grep -q -- "--- FAIL: $killer" "$out"; then
      printf "  %-24s KILLED by %s\n" "$name" "$killer"
      pass=$((pass+1))
    else
      printf "  %-24s killed, but NOT by %s (check which test moved)\n" "$name" "$killer"
      grep -- "--- FAIL" "$out" | head -3 | sed 's/^/      /'
      pass=$((pass+1))
    fi
  else
    printf "  %-24s *** SURVIVED — the tests do not cover this\n" "$name"
    fail=$((fail+1))
  fi
done

echo
echo "killed: $pass   survived: $fail   did-not-apply: $noapply   of ${#MUTANTS[@]}"
[ $fail -eq 0 ] && [ $noapply -eq 0 ] || exit 1
