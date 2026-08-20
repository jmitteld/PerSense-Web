package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Round 63 — decision 3a.20. The life-contingency surface is gated behind an
// explicit opt-in and the server FAILS CLOSED without it.
//
// WHY THESE ASSERTIONS AND NOT A HIDDEN-PANEL TEST.
// Hiding the controls is presentation. The boundary is only real if the ENGINE
// refuses, because `internal/api` accepts an `actuarial` block from any caller,
// localStorage restores values into hidden controls, and a stale tab keeps
// posting what it was holding. Every case below drives the HTTP handler.
//
// ⚠️ THE NEGATIVE CASES ARE THE POINT. A gate that only proves "off refuses"
// is half a gate: it passes just as well if the feature is broken outright.
// Each refusal is paired with an acceptance, and TestR63GateHasPower asserts
// the two are actually distinguishable.

func r63PostPV(t *testing.T, body string) (int, PVResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc",
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	HandlePVCalc(rec, req)
	var out PVResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	return rec.Code, out
}

// A minimal life-contingent screen: one lump sum, one contingency code, and
// the actuarial block the port needs to evaluate it.
const r63ContingentBody = `{
  "asOfDate":"2026-01-01","rate":0.06,
  "lumpSums":[{"date":"2036-01-01","amount":100000,"act":"L"}],
  "actuarial":{"table1":[[60,0.01],[70,0.02],[80,0.05],[120,1.0]],
               "dob1":"1966-01-01","asOfNow":"2026-01-01"}%s}`

func r63Body(withOptIn bool) string {
	if withOptIn {
		return strings.Replace(r63ContingentBody, "%s", `,"betaActuarial":true`, 1)
	}
	return strings.Replace(r63ContingentBody, "%s", "", 1)
}

func TestR63ActuarialRefusedWithoutOptIn(t *testing.T) {
	code, out := r63PostPV(t, r63Body(false))
	if code != http.StatusBadRequest {
		t.Errorf("contingency without opt-in: status %d, want 400", code)
	}
	if !strings.Contains(out.Error, "Life contingency (beta) is turned off") {
		t.Errorf("expected the beta-off refusal, got %q", out.Error)
	}
}

func TestR63ActuarialAcceptedWithOptIn(t *testing.T) {
	code, out := r63PostPV(t, r63Body(true))
	if code != http.StatusOK {
		t.Fatalf("contingency WITH opt-in: status %d, error %q — the gate must "+
			"admit the feature, not just refuse it", code, out.Error)
	}
	if out.Error != "" {
		t.Fatalf("unexpected error with opt-in: %q", out.Error)
	}
}

// TestR63GateHasPower is the control (R76/R84): it proves the two arms produce
// DIFFERENT outcomes, so neither assertion above can be vacuously green.
func TestR63GateHasPower(t *testing.T) {
	offCode, _ := r63PostPV(t, r63Body(false))
	onCode, _ := r63PostPV(t, r63Body(true))
	if offCode == onCode {
		t.Fatalf("gate has NO POWER: both arms returned %d — the opt-in does "+
			"not change the outcome, so these tests assert nothing", offCode)
	}
}

// A bare `act` with no actuarial block must ALSO be refused. Without this the
// gate leaks: validateContingencyConfig would reject it with a "configure the
// tables" message, which reads as a form error rather than a disabled feature.
func TestR63BareActCodeIsGated(t *testing.T) {
	body := `{"asOfDate":"2026-01-01","rate":0.06,
	          "lumpSums":[{"date":"2036-01-01","amount":100000,"act":"D"}]}`
	_, out := r63PostPV(t, body)
	if !strings.Contains(out.Error, "Life contingency (beta) is turned off") {
		t.Errorf("bare act code: want the beta refusal, got %q", out.Error)
	}
}

// 🚨 THE REGRESSION THIS GATE MUST NOT CAUSE.
// Variable rate is {$ifdef PVLX} — the oracle BUILDS it, five DOS sweeps pass,
// and it shipped in the Windows product. All 14 COLA options are
// oracle-exercised. Neither is actuarial and neither may be gated. If someone
// later widens pvBetaActuarialGate, this fails.
func TestR63VariableRateAndCOLAAreNotGated(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"variable rate schedule", `{"asOfDate":"2026-01-01","rate":0.06,
			"lumpSums":[{"date":"2036-01-01","amount":100000}],
			"rateSchedule":[{"date":"2026-01-01","trueRate":0.05},
			                {"date":"2030-01-01","trueRate":0.08}]}`},
		{"per-row COLA", `{"asOfDate":"2026-01-01","rate":0.06,
			"periodics":[{"fromDate":"2026-01-01","toDate":"2036-01-01",
			              "perYr":12,"amount":1000,"cola":0.03}]}`},
		{"COLA month = continuous", `{"asOfDate":"2026-01-01","rate":0.06,"colaMonth":98,
			"periodics":[{"fromDate":"2026-01-01","toDate":"2036-01-01",
			              "perYr":12,"amount":1000,"cola":0.03}]}`},
		{"COLA month = July", `{"asOfDate":"2026-01-01","rate":0.06,"colaMonth":7,
			"periodics":[{"fromDate":"2026-01-01","toDate":"2036-01-01",
			              "perYr":12,"amount":1000,"cola":0.03}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := r63PostPV(t, tc.body)
			if code != http.StatusOK {
				t.Fatalf("%s was REFUSED (status %d, %q) — it is DOS-validated "+
					"and must not be behind the actuarial gate", tc.name, code, out.Error)
			}
			if strings.Contains(out.Error, "Life contingency") {
				t.Fatalf("%s hit the actuarial gate: %q", tc.name, out.Error)
			}
		})
	}
}

// The deployment kill switch. 🚨 Written as an explicit "1" comparison, never
// `os.Getenv(x) != ""` — that form is TRUTHY ON "0" (START_HERE §1), so the
// "0" case below is a real assertion and not decoration.
func TestR63ServerKillSwitch(t *testing.T) {
	// 🚨 THE VALUES AN OPERATOR ACTUALLY TYPES. The first cut matched the
	// literal "1" only, so `true`, `yes` and `1 ` were SILENT NO-OPS — measured,
	// round 63 audit pass C. A kill switch that fails open on three of the four
	// obvious spellings is not a control.
	for _, v := range []string{"1", "true", "TRUE", "yes", "on", " 1 "} {
		t.Run("disables on "+v, func(t *testing.T) {
			t.Setenv("PERSENSE_DISABLE_ACTUARIAL", v)
			_, out := r63PostPV(t, r63Body(true))
			if !strings.Contains(out.Error, "disabled on this server") {
				t.Errorf("PERSENSE_DISABLE_ACTUARIAL=%q must disable; got %q", v, out.Error)
			}
		})
	}
	// ⚠️ And the values that must NOT disable it. "0" is the load-bearing one:
	// the `os.Getenv(x) != ""` idiom is TRUTHY on "0" and is a documented trap.
	for _, v := range []string{"0", "false", "no", "off", ""} {
		t.Run("leaves enabled on "+v, func(t *testing.T) {
			t.Setenv("PERSENSE_DISABLE_ACTUARIAL", v)
			code, out := r63PostPV(t, r63Body(true))
			if code != http.StatusOK {
				t.Errorf("PERSENSE_DISABLE_ACTUARIAL=%q must NOT disable the "+
					"feature; status %d, %q", v, code, out.Error)
			}
		})
	}
	os.Unsetenv("PERSENSE_DISABLE_ACTUARIAL")
}

// PASS-D F3: the PERIODIC limb of the gate's predicate was covered by nothing —
// mutating it to `false` left the whole suite green. TestR63BareActCodeIsGated
// exercises a LUMP SUM only.
func TestR63BarePeriodicActCodeIsGated(t *testing.T) {
	body := `{"asOfDate":"2026-01-01","rate":0.06,
	          "periodics":[{"fromDate":"2026-01-01","toDate":"2036-01-01",
	                        "perYr":12,"amount":1000,"act":"L"}]}`
	_, out := r63PostPV(t, body)
	if !strings.Contains(out.Error, "Life contingency (beta) is turned off") {
		t.Errorf("bare periodic act code: want the beta refusal, got %q", out.Error)
	}
}

// PASS-C F4: the gate's predicate must be the ENGINE's, so a code the engine
// treats as non-contingent is never refused. `ContingencyFromCode` is an
// exact-match switch; seven codes used to be refused for rows that reach no
// ACTU code at all.
func TestR63NonContingentCodesAreNotGated(t *testing.T) {
	for _, code := range []string{"", "N", "n", "l", "d", "X", " N", "N ", "None"} {
		t.Run("act="+code, func(t *testing.T) {
			body := `{"asOfDate":"2026-01-01","rate":0.06,
			          "lumpSums":[{"date":"2036-01-01","amount":100000,"act":"` + code + `"}]}`
			code2, out := r63PostPV(t, body)
			if code2 != http.StatusOK {
				t.Errorf("act=%q reaches no ACTU code (ContingencyFromCode gives "+
					"NotContingent) and must not be gated; status %d, %q",
					code, code2, out.Error)
			}
		})
	}
	// ...and every code the engine DOES treat as contingent must be refused.
	for _, code := range []string{"L", "D", "1", "2", "E", "B"} {
		t.Run("gated act="+code, func(t *testing.T) {
			body := `{"asOfDate":"2026-01-01","rate":0.06,
			          "lumpSums":[{"date":"2036-01-01","amount":100000,"act":"` + code + `"}]}`
			_, out := r63PostPV(t, body)
			if !strings.Contains(out.Error, "Life contingency (beta) is turned off") {
				t.Errorf("act=%q IS contingent and must be gated; got %q", code, out.Error)
			}
		})
	}
}

// ---- the shipped page ----
//
// These read cmd/persense/static/index.html as TEXT. They cannot prove the
// gate WORKS — only a browser can, and the browser harness must state whether
// Tailwind loaded (§95). What they pin is the part a text test can own: that
// the mechanism is the `hidden` ATTRIBUTE and not the `.hidden` CLASS, which
// is inert with the CDN unreachable.

func r63IndexHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../cmd/persense/static/index.html")
	if err != nil {
		// NOT t.Skip. A SKIP IS NOT A PASS (R12), and the shipped page is not an
		// optional dependency — four gate assertions would go quiet.
		t.Fatalf("index.html not readable, so the shipped-page assertions below "+
			"would silently assert nothing: %v", err)
	}
	return string(b)
}

func TestR63BetaToggleShipsOffByDefault(t *testing.T) {
	h := r63IndexHTML(t)
	re := regexp.MustCompile(`<input id="set-betaActuarial"[^>]*>`)
	m := re.FindString(h)
	if m == "" {
		t.Fatal("no #set-betaActuarial checkbox in the shipped page")
	}
	if strings.Contains(m, " checked") {
		t.Errorf("the beta toggle ships CHECKED; decision 3a.20 is off by default: %s", m)
	}
	if !strings.Contains(m, "applyBetaActuarialGate()") {
		t.Errorf("the toggle is not wired to applyBetaActuarialGate: %s", m)
	}
}

func TestR63GateUsesTheHiddenAttributeNotTheClass(t *testing.T) {
	h := r63IndexHTML(t)
	if !strings.Contains(h, `el.setAttribute('hidden', '')`) {
		t.Error("applyBetaActuarialGate must hide with the `hidden` ATTRIBUTE: " +
			"the `.hidden` CLASS comes from cdn.tailwindcss.com and the inline " +
			"stylesheet has no such rule, so a class hide is INERT offline (§95)")
	}
	if !strings.Contains(h, "[hidden] { display: none !important; }") {
		t.Error("the inline stylesheet must carry an explicit [hidden] rule so " +
			"no later utility class can outrank the UA default")
	}
	// Every gated region must be findable by the one selector the gate uses.
	//
	// 🚨 THIS WAS `n < 6` AND THAT TOLERATED LOSING A REGION. The file holds
	// SEVEN occurrences — six markers plus the querySelectorAll in
	// applyBetaActuarialGate — so `< 6` still passed with the periodic Life
	// column's marker deleted, leaving that column visible with the feature off
	// and the whole suite green (demonstrated, round 63 audit pass D).
	// The markers are now named and counted individually.
	for _, m := range []struct{ what, frag string }{
		{"the actuarial <details>", `id="pv-actuarial-section" data-beta-actu`},
		{"the lump-sum act <td>", `<td style="padding:0" data-beta-actu><select class="grid-cell" data-ls=`},
		{"the periodic act <td>", `<td style="padding:0" data-beta-actu><select class="grid-cell" data-per=`},
		{"the Life Contingency journey group", `<div class="journey-group" data-beta-actu>`},
	} {
		if !strings.Contains(h, m.frag) {
			t.Errorf("gated region missing its data-beta-actu marker: %s", m.what)
		}
	}
	if n := strings.Count(h, `<th class="grid-header" data-beta-actu`); n != 2 {
		t.Errorf("%d marked Life column headers, want exactly 2 (lump sums and "+
			"periodics) — an unmarked one stays visible with the feature off", n)
	}
	if n := strings.Count(h, "data-beta-actu"); n != 7 {
		t.Errorf("%d data-beta-actu occurrences, want exactly 7 (6 markers + the "+
			"selector in applyBetaActuarialGate); a change here needs this count "+
			"updated deliberately, not silently", n)
	}
}

// 🚨 THE LINE THAT MAKES THE FEATURE WORK AT ALL.
//
// `body.betaActuarial = true` in getPVInput is what turns the user's toggle
// into the server's opt-in. Delete it and the feature is DEAD FOR EVERY USER —
// the page hides nothing, the server refuses everything, and (measured, round
// 63 audit pass D) the entire Go suite AND the round's own browser harness both
// stay GREEN. The harness computed the value and forgot to assert it.
func TestR63PageSendsTheOptInWhenEnabled(t *testing.T) {
	h := r63IndexHTML(t)
	if !strings.Contains(h, "body.betaActuarial = true;") {
		t.Fatal("getPVInput no longer sets body.betaActuarial — with this line " +
			"gone the server refuses every request the UI makes and the feature " +
			"is unusable, with no other test in the repository noticing")
	}
	// And it must be set only inside the branch that actually built an
	// actuarial block, never unconditionally.
	i := strings.Index(h, "body.betaActuarial = true;")
	start := i - 400
	if start < 0 {
		start = 0
	}
	if !strings.Contains(h[start:i], "body.actuarial = acfg;") {
		t.Error("body.betaActuarial is not set alongside body.actuarial — the " +
			"opt-in must travel with the actuarial block, not be sent blanket")
	}
}

// PASS-C F1: applyState() restores both the checkbox and the per-row `act`
// values, so it is a gate-restoring path. It is reached by UNDO as well as by
// restoreState, and one Undo click after switching the feature off used to
// re-post a contingency with every control still hidden.
func TestR63ApplyStateReAppliesTheGate(t *testing.T) {
	h := r63IndexHTML(t)
	i := strings.Index(h, "function applyState(s) {")
	if i < 0 {
		t.Fatal("applyState not found")
	}
	j := strings.Index(h[i:], "\n}")
	if j < 0 {
		t.Fatal("could not bound applyState")
	}
	if !strings.Contains(h[i:i+j], "applyBetaActuarialGate();") {
		t.Error("applyState does not re-apply the beta gate. It restores " +
			"set-betaActuarial AND the per-row act values, so Undo and restore " +
			"both desync the gate from the data without this call")
	}
}

// PASS-C F2: hiding the inputs does not un-paint the answer.
func TestR63SwitchingOffClearsALifeContingentResult(t *testing.T) {
	h := r63IndexHTML(t)
	for _, frag := range []string{
		"function pvOutputsCarryContingency()",
		"function clearPVEngineOutputs()",
		"if (pvOutputsCarryContingency()) clearPVEngineOutputs();",
	} {
		if !strings.Contains(h, frag) {
			t.Errorf("missing %q — with the feature off, a survival-weighted "+
				"total stays on screen, styled as engine output and annotated "+
				"with a survival probability, and survives a reload", frag)
		}
	}
}

// The gate must not have swept up the COLA column or the variable-rate panel.
func TestR63GateDidNotMarkCOLAOrVariableRate(t *testing.T) {
	h := r63IndexHTML(t)
	for _, frag := range []string{
		`data-f="cola"`,
		`<details class="mt-3" id="pv-vr-section">`,
	} {
		i := strings.Index(h, frag)
		if i < 0 {
			t.Errorf("%s not found — did the gate remove DOS-validated markup?", frag)
			continue
		}
		// look back a little: the marker would sit on the same element
		start := i - 200
		if start < 0 {
			start = 0
		}
		if strings.Contains(h[start:i], "data-beta-actu") {
			t.Errorf("%s appears to be inside a data-beta-actu region; variable "+
				"rate and COLA are DOS-validated and must stay visible", frag)
		}
	}
}
