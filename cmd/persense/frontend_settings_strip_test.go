package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestSettingsStripTokens guards the DOS-style computational-settings status
// strip added to the Amortization and Present Value screens (updateSettingsStrip
// in index.html). The strip reproduces the DOS program's persistent settings bar
// (INTSUTIL.pas: procedure UpdateSettings) so the active basis / timing /
// prepaid / per-year mode is always visible instead of being hidden behind the
// Computational Settings modal.
//
// Provenance of the expected token strings — DOS UpdateSettings, INTSUTIL.pas:
//   374-436, per screen:
//     PV  (iPVL): COLA:<month>  <basis>  [Exact]  <n>perYr
//     AMZ (iAMZ): <basis> [USA|Act|R78] <Arr|Adv> [InclReg|PlusReg] <PrePd|No-PrePd> <n>perYr
// Two deliberate deviations are asserted here so a future edit can't silently
// undo them (both documented at the updateSettingsStrip definition):
//   - the DOS "19"+centurydiv year token (e.g. "1975", INTSUTIL.pas:420) is
//     omitted — this port has no century-divide setting; and
//   - "Exact" is surfaced on BOTH screens but only when it actually changes the
//     result (Exact=YES and basis<>360), whereas DOS printed it on PV only.
//
// Like the sibling frontend tests, this executes the SHIPPED JS extracted from
// index.html (SETTINGS_DEFAULTS + updateSettingsStrip + renderSettingsStrip)
// against a minimal fake DOM under Node, so it tests real code, not a copy.
// Skips when node is not installed.
func TestSettingsStripTokens(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	htmlBytes, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	defaults := regexp.MustCompile(`(?s)const SETTINGS_DEFAULTS = \{.*?\};`).FindString(html)
	if defaults == "" {
		t.Fatal("SETTINGS_DEFAULTS definition not found in index.html")
	}
	// Capture the whole settings-strip block in one shot: from settingsTokens'
	// start through renderSettingsStrip's close, which is immediately followed by
	// the AUTO-CALCULATE section banner. This includes settingsTokens,
	// settingsSummaryText, updateSettingsStrip, and renderSettingsStrip.
	fnRe := regexp.MustCompile(`(?s)(function settingsTokens\(screen\) \{.*\n\})\n\n// ========== AUTO-CALCULATE`)
	fnMatch := fnRe.FindStringSubmatch(html)
	if fnMatch == nil {
		t.Fatal("settingsTokens..renderSettingsStrip block not found in index.html")
	}
	shipped := fnMatch[1]
	for _, want := range []string{"function settingsSummaryText", "function updateSettingsStrip", "function renderSettingsStrip"} {
		if !strings.Contains(shipped, want) {
			t.Fatalf("extracted block did not include %s", want)
		}
	}

	harness := `
'use strict';
` + defaults + `

` + shipped + `

// ----- minimal fake DOM + fancy stub -----
let __fancy = false;
function amzAdvActiveCount() { return __fancy ? 1 : 0; }
const els = {};
function makeStrip() { return { innerHTML: '' }; }
const document = {
  getElementById(id) {
    if (!els[id]) {
      els[id] = (id.endsWith('-settings-strip'))
        ? makeStrip()
        : { value: (id in SETTINGS_DEFAULTS) ? SETTINGS_DEFAULTS[id] : '' };
    }
    return els[id];
  }
};

function tokensOf(id) {
  const html = els[id] ? els[id].innerHTML : '';
  const re = /<span class="ss-tok( ss-nondefault)?">([^<]*)<\/span>/g;
  const out = [];
  let m;
  while ((m = re.exec(html)) !== null) out.push({ text: m[2], nd: !!m[1] });
  return out;
}

function run(overrides, fancy) {
  for (const k of Object.keys(els)) delete els[k];   // reset DOM each scenario
  __fancy = !!fancy;
  for (const [id, v] of Object.entries(overrides)) document.getElementById(id).value = v;
  updateSettingsStrip();
  const join = arr => arr.map(t => t.text).join('|');
  const ndjoin = arr => arr.filter(t => t.nd).map(t => t.text).join('|');
  const amz = tokensOf('amz-settings-strip'), pv = tokensOf('pv-settings-strip');
  return { amz: join(amz), pv: join(pv), amzNd: ndjoin(amz), pvNd: ndjoin(pv),
           amzText: settingsSummaryText('amz'), pvText: settingsSummaryText('pv') };
}

console.log(JSON.stringify({
  A_defaults:      run({}, false),
  B_nonfancy:      run({ 'set-basis':'365','set-rule78':'yes','set-timing':'advance',
                         'set-prepaid':'no','set-exact':'yes','set-perYr':'24',
                         'set-colaMonth':'continuous' }, false),
  C_fancyUSA:      run({ 'set-basis':'365','set-interestRule':'usa','set-timing':'advance',
                         'set-balloonIncl':'no' }, true),
  D_fancyInclReg:  run({ 'set-basis':'365','set-balloonIncl':'yes' }, true),
  E_exactInert360: run({ 'set-exact':'yes','set-basis':'360' }, false),
  F_colaMonthNum:  run({ 'set-colaMonth':'6' }, false),
}));
`

	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}

	type strip struct {
		Amz     string `json:"amz"`
		Pv      string `json:"pv"`
		AmzNd   string `json:"amzNd"`
		PvNd    string `json:"pvNd"`
		AmzText string `json:"amzText"`
		PvText  string `json:"pvText"`
	}
	var res map[string]strip
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}

	eq := func(name, got, want string) {
		if got != want {
			t.Errorf("%s:\n  got:  %q\n  want: %q", name, got, want)
		}
	}

	// A) Defaults render the DOS default mode with nothing highlighted.
	eq("A amz", res["A_defaults"].Amz, "360|Arr|PrePd|12perYr")
	eq("A pv", res["A_defaults"].Pv, "COLA:Ann|360|12perYr")
	eq("A amz highlighted", res["A_defaults"].AmzNd, "")
	eq("A pv highlighted", res["A_defaults"].PvNd, "")

	// B) No advanced options: interest-rule token is R78 (not USA/Act), timing
	// shows Adv, Exact appears (365 basis), all changed tokens highlighted.
	eq("B amz", res["B_nonfancy"].Amz, "365|R78|Adv|No-PrePd|Exact|24perYr")
	eq("B pv", res["B_nonfancy"].Pv, "COLA:Cnt|365|Exact|24perYr")
	eq("B amz highlighted", res["B_nonfancy"].AmzNd, "365|R78|Adv|No-PrePd|Exact|24perYr")

	// C) With advanced options (fancy): interest rule switches to USA/Act, Adv
	// is suppressed to Arr (no in-advance under fancy), and Incl/PlusReg appears.
	eq("C amz", res["C_fancyUSA"].Amz, "365|USA|Arr|PlusReg|PrePd|12perYr")

	// D) Fancy + "includes regular payment" -> InclReg; Actuarial -> Act.
	eq("D amz", res["D_fancyInclReg"].Amz, "365|Act|Arr|InclReg|PrePd|12perYr")

	// E) Exact is inert on the 360 basis, so it must NOT appear on either strip.
	eq("E amz", res["E_exactInert360"].Amz, "360|Arr|PrePd|12perYr")
	eq("E pv", res["E_exactInert360"].Pv, "COLA:Ann|360|12perYr")

	// F) A numeric COLA month is written as the number, matching DOS.
	eq("F pv", res["F_colaMonthNum"].Pv, "COLA:6|360|12perYr")

	// settingsSummaryText is the plain-text form stamped onto the schedule
	// header and CSV exports — space-joined, no highlighting.
	eq("summary amz defaults", res["A_defaults"].AmzText, "360 Arr PrePd 12perYr")
	eq("summary pv defaults", res["A_defaults"].PvText, "COLA:Ann 360 12perYr")
	eq("summary amz fancy", res["C_fancyUSA"].AmzText, "365 USA Arr PlusReg PrePd 12perYr")
}
