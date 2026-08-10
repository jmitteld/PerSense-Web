package api

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ROUND-45 GUARDS — A1-A5, THE ADJUSTMENT ECHO **AT THE CONSUMER**.
//
// Round 39's own postmortem (39D §2, carried in START_HERE as items A1-A5) is
// that the R39 wire fields it added have ZERO API-layer tests, and that this is
// a prerequisite for calling ANY R39 fix closed. Five rounds later the count was
// still zero. R42 is the rule these discharge: A TRANSPORT FIX MUST BE VERIFIED
// AT THE CONSUMER, NOT AT THE PRODUCER — round 39 shipped three fixes verified
// at the producer and broken at the consumer.
//
// Each test below pins the WIRE (the JSON `AmortizationResponse`), and each was
// run against the pre-fix tree in round 45 and SEEN TO FAIL (R38/rule 3).
//
// ⚠️ Oracle lines were produced in round 45 by /tmp/oraclebuild/amort_oracle,
// built by legacy/oracle/build_linux.sh:114 with
//
//	-Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX      (NO -dACTU; R47)

func r45Post(t *testing.T, body string) AmortizationResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	HandleAmortizationCalc(rec, req)
	var resp AmortizationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response: %v\n%s", err, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	return resp
}

// --- A1 -----------------------------------------------------------------------
//
// NF-1 AT THE CONSUMER. The engine-side fix (engine.go, DOS's unconditional
// store at AMORTOP.pas:1591-1592) is worth nothing if the value does not reach
// the JSON. `rule78` routes the screen to the PIECEWISE engine, which is the
// engine that answers Nate's screens and the one NF-1 was open on.
//
//	amort_oracle 100000 0.07 360 12 adj=84:0.08: r78 adjdump
//	  → adjrow 1 date 1/1/2031 rate 0.0800000000 ratestatus 3 \
//	    amount 723.211515 amtstatus 1 amtok FALSE
//
// 🚨 THE WIRE CONTRACT IS A POINTER. `AdjustmentEcho.Amount` is `*float64` with
// `omitempty` so a cell DOS leaves blank is ABSENT rather than 0 — see A4. That
// makes "the field is missing" and "the field is 0" different bugs, and this
// test must distinguish them, so it checks the POINTER before the value.
func TestR45A1AdjustmentAmountReachesTheWireOnPiecewise(t *testing.T) {
	resp := r45Post(t, `{"amount":100000,"loanDate":"2024-01-01","rate":0.07,
		"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
		"rule78":true,
		"adjustments":[{"date":"2031-01-01","rate":0.08}]}`)

	if len(resp.Adjustments) != 1 {
		t.Fatalf("expected 1 adjustment echo on the wire, got %d", len(resp.Adjustments))
	}
	e := resp.Adjustments[0]
	if e.Amount == nil {
		t.Fatalf("NF-1 AT THE CONSUMER: `adjustments[0].amount` is ABSENT from the " +
			"JSON. DOS paints 723.21 into its New Payment cell on this screen. The " +
			"UI deletes its own reconstruction and leaves the cell EMPTY when the " +
			"field is missing (39C), so this is Nate's blank cell on the wire.")
	}
	if math.Abs(*e.Amount-723.21) > 0.005 {
		t.Errorf("wire amount %.2f, DOS 723.211515 (rounded 723.21)", *e.Amount)
	}
	if !e.AmountSolved {
		t.Errorf("`amountSolved` is false; the UI only paints a cell the ENGINE " +
			"produced (index.html: `if (a.amountSolved && a.amount != null)`), so a " +
			"correct amount with this flag false is still a blank cell.")
	}
	if e.Date != "2031-01-01" {
		t.Errorf("echo date %q, want 2031-01-01 — the UI matches DOM rows BY DATE", e.Date)
	}
}

// --- A2 -----------------------------------------------------------------------
//
// 🚨 NF-1c — A NEW DEFECT, FOUND IN ROUND 45 BY THE PRIOR-ART SWEEP THAT
// START_HERE MANDATES, NOT BY A FUZZER.
//
// `docs/discrepancies.md:2885-2887` (written 2026-08-07) closed the adjustment
// kicker audit with an explicit forward warning:
//
//	"On the out-bound direction there is nothing to un-kick today:
//	 AmortizationResponse ... has no adjustment echo field, so a SOLVED
//	 adjustment rate (DOS EstimateAndRefineAdjRate, the AO6 shape) never
//	 reaches the UI. If such an echo is ever added it MUST go through
//	 amzUnkickerRate — noted here so the pairing is not lost."
//
// Round 39 added exactly that echo, two days later, and did NOT wire the
// un-kick. `handlers.go` kicks the adjustment rate INBOUND (`row.LoanRate =
// amzKickerRate(*a.Rate, basis)`) and echoes it back RAW, while the LOAN rate on
// the same response is correctly un-kicked. The comment at handlers.go:983 even
// says "the echo divides it back" — true of the loan rate, false of this one.
//
// DOS: INTSUTIL.pas:1649-1651, the `aratecol,adjratecol,aaprcol` arm —
//
//	if (df.c.basis=x365_360) and (col<>aaprcol) then
//	   PercentValueFromCell := ReportedRate(rp^)/kicker
//
// so DISPLAY = internal / (365/360). `adjratecol` is in the SAME arm as
// `aratecol`; only the APR column is exempt.
//
// THE MEASUREMENT. User types 8% on the 365/360 basis, so the internal loan rate
// is 0.08*365/360 = 0.081111111111111106. AO6 adjustment (payment 900 typed,
// rate blank) at installment 84:
//
//	amort_oracle 100000 0.081111111111111106 360 12 adj=84::900.00 b365_360 adjdump
//	  → adjrow 1 date 1/1/2031 rate 0.1064151870 ratestatus 1 \
//	    amount 900.000000 amtstatus 3 amtok TRUE
//
// DOS's SCREEN therefore shows 0.1064151870/(365/360) = 0.1049574447.
// Un-fixed, the port paints 0.1064151870 — 0.146 percentage points high, a
// 1.3889% relative error, on a cell the user reads as an interest rate.
//
// ⚠️ THIS IS THE ARM THE UI ACTUALLY PAINTS. A user-TYPED adjustment rate echoes
// with `rateSolved:false` and the UI leaves it alone, so the raw echo is
// invisible there. It is only the SOLVED (AO6) rate that is painted, which is
// why this went five rounds unnoticed.
func TestR45A2SolvedAdjustmentRateIsUnkickedOn365360(t *testing.T) {
	const body = `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,
		"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"365/360",
		"adjustments":[{"date":"2031-01-01","amount":900.00}]}`
	resp := r45Post(t, body)

	if len(resp.Adjustments) != 1 {
		t.Fatalf("expected 1 adjustment echo, got %d", len(resp.Adjustments))
	}
	e := resp.Adjustments[0]
	if e.Rate == nil {
		t.Fatalf("`adjustments[0].rate` absent; DOS solves it (ratestatus 1 = outp)")
	}
	if !e.RateSolved {
		t.Fatalf("`rateSolved` false on the AO6 arm; DOS reports ratestatus 1 (outp)")
	}
	const wantDisplay = 0.1049574447
	if math.Abs(*e.Rate-wantDisplay) > 5e-7 {
		internal := wantDisplay * (365.0 / 360.0)
		t.Errorf("NF-1c: wire rate %.10f, DOS SCREEN %.10f (internal %.10f).\n"+
			"The echo is in INTERNAL space. INTSUTIL.pas:1649-1651 divides "+
			"adjratecol by the kicker on the 365/360 basis exactly as it does "+
			"aratecol. Route it through amzUnkickerRate, as "+
			"docs/discrepancies.md:2885-2887 required.",
			*e.Rate, wantDisplay, internal)
	}

	// THE IN-GUARD POSITIVE CONTROL (R24). If the un-kick were applied on EVERY
	// basis instead of only 365/360, the assertion above would still pass and
	// this project would have swapped one defect for a broader one. The 360
	// basis must be IDENTITY. DOS on b360 with the SAME internal loan rate
	// solves the same 0.1064151870 and displays it unchanged:
	//
	//	amort_oracle 100000 0.08 360 12 adj=84::900.00 adjdump
	//	  → adjrow 1 rate 0.1066209026 ratestatus 1
	//
	// (the internal rate differs because on b360 the typed 8% is NOT kicked).
	resp360 := r45Post(t, `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,
		"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
		"adjustments":[{"date":"2031-01-01","amount":900.00}]}`)
	if len(resp360.Adjustments) != 1 || resp360.Adjustments[0].Rate == nil {
		t.Fatalf("POSITIVE CONTROL BROKEN: no rate echo on the 360 arm")
	}
	if got := *resp360.Adjustments[0].Rate; math.Abs(got-0.1066209026) > 5e-7 {
		t.Errorf("POSITIVE CONTROL FAILED: on the 360 basis the un-kick must be "+
			"the IDENTITY. wire %.10f, DOS 0.1066209026. An unconditional "+
			"division would show ~0.1051625.", got)
	}
}

// --- A3 -----------------------------------------------------------------------
//
// THE USER'S OWN VALUES MUST NOT COME BACK MARKED SOLVED. This is the failure
// mode that turns a working screen into an unsubmittable one: the UI paints a
// "solved" cell green, `saveState` records it as an output, and the next
// Calculate re-sends it — or drops it — changing AO5 into AO7. Round 39D fixed
// exactly this shape twice (the green map, and `TestR39BothBlankAdjustment...`).
//
// It is also the NEGATIVE CONTROL for A1/A2 at the wire: the round-45 engine fix
// adds an unconditional store, and a version of it that ignored DOS's `not
// amtok` branch selector would relabel the caller's own number as computed.
func TestR45A3CallerSuppliedAdjustmentValuesEchoAsNotSolved(t *testing.T) {
	resp := r45Post(t, `{"amount":100000,"loanDate":"2024-01-01","rate":0.07,
		"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
		"rule78":true,
		"adjustments":[{"date":"2031-01-01","rate":0.08}]}`)
	if len(resp.Adjustments) != 1 {
		t.Fatalf("expected 1 adjustment echo, got %d", len(resp.Adjustments))
	}
	e := resp.Adjustments[0]
	if e.Rate == nil {
		t.Fatalf("the caller's own rate vanished from the echo")
	}
	if e.RateSolved {
		t.Errorf("`rateSolved` is TRUE on a rate the CALLER supplied. The UI paints " +
			"that cell green and the request builder then reads it back as an " +
			"engine output on the next submit.")
	}
	if math.Abs(*e.Rate-0.08) > 1e-9 {
		t.Errorf("echoed rate %.10f, want the caller's 0.08", *e.Rate)
	}
}

// --- A4 -----------------------------------------------------------------------
//
// A CELL DOS LEAVES BLANK MUST BE **ABSENT** ON THE WIRE, NOT 0.
//
// This is §81's trap one screen over: `omitempty` on a computed numeric output
// makes an exact 0 indistinguishable from "not computed". The adjustment echo
// solves it with POINTERS, and this test pins that the pointer discipline
// survives — a `0` here renders as "0.0000%" in a cell the original leaves
// empty, which is the same class of user-visible defect as a missing value.
//
//	amort_oracle 100000 0.07 360 12 adj=84:: r78 adjdump
//	  → adjrow 1 date 1/1/2031 rate 0.0000000000 ratestatus 0 \
//	    amount 665.302495 amtstatus 1 amtok FALSE
//
// So: the AMOUNT is present (DOS re-amortized and stored it — the round-45 fix)
// and the RATE is absent (ratestatus 0). One row, both directions.
func TestR45A4BlankAdjustmentRateIsAbsentFromTheWire(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewReader([]byte(`{"amount":100000,"loanDate":"2024-01-01","rate":0.07,
			"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
			"rule78":true,
			"adjustments":[{"date":"2031-01-01"}]}`)))
	req.Header.Set("Content-Type", "application/json")
	HandleAmortizationCalc(rec, req)

	// Assert on the RAW JSON, not the decoded struct: `omitempty` is the property
	// under test and a decoded nil cannot distinguish "absent" from "null".
	raw := rec.Body.String()
	var envelope struct {
		Adjustments []map[string]json.RawMessage `json:"adjustments"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("bad response: %v\n%s", err, raw)
	}
	if len(envelope.Adjustments) != 1 {
		t.Fatalf("expected 1 adjustment echo, got %d\n%s", len(envelope.Adjustments), raw)
	}
	row := envelope.Adjustments[0]
	if _, present := row["rate"]; present {
		t.Errorf("`rate` is PRESENT on the wire for a row DOS reports with "+
			"ratestatus 0 (empty). A 0 here paints \"0.0000%%\" into a cell the "+
			"original leaves blank. Raw row: %v", row)
	}
	amtRaw, present := row["amount"]
	if !present {
		t.Fatalf("NF-1: `amount` is ABSENT although DOS stores 665.302495 on this "+
			"row (amtstatus 1). DOS's unconditional store at AMORTOP.pas:1591 runs "+
			"on EVERY crossing, including a row that changes nothing. Raw row: %v", row)
	}
	var amt float64
	if err := json.Unmarshal(amtRaw, &amt); err != nil {
		t.Fatalf("amount is not a number: %s", amtRaw)
	}
	if math.Abs(amt-665.30) > 0.005 {
		t.Errorf("wire amount %.2f, DOS 665.302495 (rounded 665.30)", amt)
	}
}

// --- A5 -----------------------------------------------------------------------
//
// THE R39 REGULAR-PAYMENT WIRE FIELD, ON THE PIECEWISE ENGINE.
//
// 39D recorded surviving mutant M11 — "piecewise `RegularPayment` from the modal
// — UNPINNED" — and START_HERE has carried it ever since. `paired_regression` is
// blind to it, and the fixture file pins the dosport arm well and the piecewise
// arm barely. This is the API-layer half.
//
//	amort_oracle 100000 0.07 360 12 adj=84:0.08: r78 adjdump
//	  → payment 665.3025
//
// `payment` is the INITIAL (pre-adjustment) regular payment DOS computes, and
// the UI paints it into the Payment cell. Reconstructing it from the modal row
// of the schedule — the defect R39 exists to kill — gives the POST-adjustment
// 723.21 on this screen, so the two are cleanly separated here.
func TestR45A5RegularPaymentReachesTheWireOnPiecewise(t *testing.T) {
	resp := r45Post(t, `{"amount":100000,"loanDate":"2024-01-01","rate":0.07,
		"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
		"rule78":true,
		"adjustments":[{"date":"2031-01-01","rate":0.08}]}`)

	if resp.Payment == nil {
		t.Fatalf("R39: `payment` is ABSENT from the wire on a PIECEWISE screen. " +
			"The UI's own reconstruction was DELETED in 39C with no fallback, so a " +
			"missing field is a blank Payment cell.")
	}
	if math.Abs(*resp.Payment-665.30) > 0.005 {
		t.Errorf("wire payment %.2f, DOS 665.3025. If this reads 723.21 the value "+
			"has been reconstructed from the schedule's modal row — the M11 mutant, "+
			"and the exact defect R39 exists to kill.", *resp.Payment)
	}
	if !resp.PaymentSolved {
		t.Errorf("`paymentSolved` false although the caller left the payment blank")
	}
}

// --- NF-2 ---------------------------------------------------------------------
//
// 🚨 NF-2's PUBLISHED DESCRIPTION WAS WRONG, AND ROUND 45 RETRACTS IT (rule 11).
//
// START_HERE and 39D both record NF-2 as "DOS snaps the adjustment onto the
// payment grid and echoes the SNAPPED date; the DOM row keeps the typed date".
// The second clause is right; the FIRST HALF OF THE FIRST is not a defect at
// all — the PORT ALSO SNAPS, and it always did:
//
//	engine.go, in Amortize, BEFORE ValidateInputs and BEFORE the dosport
//	delegation — the port of Amortize.pas:258-271, including the
//	`datestatus := defp` demotion. It mutates input.Adjustments IN PLACE, so
//	both engines' echoes already carried the SNAPPED date.
//
// The break was entirely at the CONSUMER: `index.html` matched the echo to a DOM
// row with `rowISO !== a.date`, where `rowISO` is parsed from the row's own date
// INPUT, which still holds what the user typed. For an off-grid date that test
// failed for EVERY row, the forEach returned for all of them, and NOTHING was
// painted — however correct the echo was. 39D measured 16 of 400.
//
// So NF-2 is a CONSUMER defect, which is R42 exactly: round 39 verified this
// transport at the producer and it was broken at the consumer.
//
// DOS, verified round 45:
//
//	amort_oracle 100000 0.07 360 12 adjdmy=12.1.2029:0.08: r78 adjdump
//	  → adjrow 1 date 1/1/2029 ...   (typed the 12th, DOS reports the 1st)
//
// and the on-grid request `adjdmy=1.1.2029:0.08:` returns the identical row —
// the snap is idempotent, which is the control below.
func TestR45NF2SnappedAdjustmentCarriesTheRequestedDate(t *testing.T) {
	resp := r45Post(t, `{"amount":100000,"loanDate":"2024-01-01","rate":0.07,
		"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
		"rule78":true,
		"adjustments":[{"date":"2029-01-12","rate":0.08}]}`)

	if len(resp.Adjustments) != 1 {
		t.Fatalf("expected 1 adjustment echo, got %d", len(resp.Adjustments))
	}
	e := resp.Adjustments[0]
	// `date` is the SNAPPED date — what DOS displays.
	if e.Date != "2029-01-01" {
		t.Errorf("echo date %q, want the SNAPPED 2029-01-01 (DOS: adjrow 1 date "+
			"1/1/2029 for a typed 12 Jan)", e.Date)
	}
	// `requestedDate` is what the client sent — what it must MATCH on.
	if e.RequestedDate != "2029-01-12" {
		t.Fatalf("NF-2: `requestedDate` is %q, want the client's 2029-01-12. "+
			"Without it the client cannot find its own row: it matches by date "+
			"and its row still holds the typed value, so `rowISO !== a.date` is "+
			"true for EVERY row and nothing is painted.", e.RequestedDate)
	}
	// And the payload must still be there — a findable row with nothing in it
	// would be no better.
	if e.Amount == nil || math.Abs(*e.Amount-726.52) > 0.005 {
		t.Errorf("snapped row lost its amount: %v (DOS 726.522878)", e.Amount)
	}

	// 🚨 M11's SHAPE, AND THE REASON THIS ARM EXISTS. The request above carries
	// `rule78`, which routes it to the PIECEWISE engine. Round 39's whole
	// postmortem is that its fixtures pinned the dosport arm well and the
	// piecewise arm barely; pinning only ONE engine is the same defect with the
	// engines swapped. `dosport_entry.go` copies RequestedDate in its own echo
	// loop and a mutation that drops it must be caught. Drop `rule78` and the
	// identical screen is answered by the faithful port.
	//
	//	amort_oracle 100000 0.07 360 12 adjdmy=12.1.2029:0.08: adjdump
	//	  → adjrow 1 date 1/1/2029 ...
	dos := r45Post(t, `{"amount":100000,"loanDate":"2024-01-01","rate":0.07,
		"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
		"adjustments":[{"date":"2029-01-12","rate":0.08}]}`)
	if len(dos.Adjustments) != 1 {
		t.Fatalf("dosport arm: expected 1 adjustment echo, got %d", len(dos.Adjustments))
	}
	if d := dos.Adjustments[0]; d.RequestedDate != "2029-01-12" || d.Date != "2029-01-01" {
		t.Errorf("NF-2 on the DOSPORT arm: date=%q requestedDate=%q, want "+
			"2029-01-01 / 2029-01-12. Both engines echo adjustments and both must "+
			"carry the pre-snap date (M11's shape).", d.Date, d.RequestedDate)
	}
}

// TestR45NF2OnGridAdjustmentOmitsRequestedDate is NF-2's NEGATIVE CONTROL, and
// it is the one that keeps the field honest: `requestedDate` must appear ONLY
// when a snap actually moved the row. If it were emitted unconditionally the
// assertion above would still pass, the client's match would still work, and the
// wire would have grown a permanently redundant field that says "this row was
// moved" about rows that were not — and the date cell would be repainted as a
// computed output on every ordinary screen.
func TestR45NF2OnGridAdjustmentOmitsRequestedDate(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewReader([]byte(`{"amount":100000,"loanDate":"2024-01-01","rate":0.07,
			"firstDate":"2024-02-01","nPeriods":360,"perYr":12,"basis":"360",
			"rule78":true,
			"adjustments":[{"date":"2029-01-01","rate":0.08}]}`)))
	req.Header.Set("Content-Type", "application/json")
	HandleAmortizationCalc(rec, req)

	var envelope struct {
		Adjustments []map[string]json.RawMessage `json:"adjustments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("bad response: %v\n%s", err, rec.Body.String())
	}
	if len(envelope.Adjustments) != 1 {
		t.Fatalf("expected 1 adjustment echo, got %d", len(envelope.Adjustments))
	}
	if v, present := envelope.Adjustments[0]["requestedDate"]; present {
		t.Errorf("`requestedDate` is PRESENT (%s) on an ON-GRID adjustment that "+
			"was never snapped. It must be omitted so the client can tell a moved "+
			"row from an untouched one — the client repaints the date cell as a "+
			"computed output when it is set.", v)
	}
}
