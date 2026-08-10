package amortization

// zzr47_usa_ao6_usap_test.go — round 47.
//
// WHAT THIS PINS, AND WHY IT EXISTS AT ALL.
//
// There is no STATIC golden anywhere in `internal/` for the USA rule ∧ DEEP
// negative amortisation ∧ amount-only ("AO6") adjustment triple, and the
// randomized fuzzer structurally cannot reach it: `dos_fuzzer5_test.go:124`
// draws the payment at 0.85x–1.35x fair, while the case below runs at ~0.05x
// fair (100 against ~1933). The USA ∧ AO6 PAIR is generated routinely
// (`dos_fuzzer5_test.go:1352` draws `usa`; `:1734` draws payment-only) and
// appears statically at `zzsec65_oracle_advisory_test.go:88` — but as a refusal
// case, so with no golden. It is the negative-amortisation limb that is
// uncovered, and that limb is exactly where the statement below bites.
//
// The guard is worth more than coverage: the port is DOS-exact here, and the
// obvious "faithfulness" edit BREAKS it by $3,060.36.
//
// THE PASCAL THAT LOOKS LIKE A MISSING LIMB. DOS's AO6 rate branch is
//
//	AMORTOP.pas:1515   if (adj[next_adj]^.amtok) then
//	AMORTOP.pas:1523     if Iterate(p, usap, payment.date, nextpayment.date,
//	AMORTOP.pas:1525        adj[next_adj]^.loanrate := h^.loanrate;
//	AMORTOP.pas:1526        adj[next_adj]^.loanratestatus := outp;   <- the FREEZE
//	AMORTOP.pas:1534     p := h^.amount;
//	AMORTOP.pas:1535     usap := 0;                                  <- the limb
//
// `usap := 0` occurs EXACTLY THREE times in AMORTOP.pas — `:168` (init), `:660`
// (the `if usap < 0` clamp) and `:1535` — and `:1535` is only in the RATE
// branch, never in the amount (AO5) branch. engine.go's AO6 block cites
// `AMORTOP.pas:1520-1535` but mirrors only `:1520-1531`; neither engine writes
// `usap` at the crossing. That reads like an unported statement, and its
// precondition (a live non-zero accumulator, which needs USA plus payments that
// under-cover interest) is the USA ∧ prepay ∧ adjustment shape of §89 §4.
//
// 🚨 IT IS NOT A MISSING LIMB. MEASURED, ROUND 47, on the case below — which
// reaches the branch with usap = 56,400.12, traced, not assumed:
//
//	                                        solved AO6 rate   total interest
//	DOS (amort_oracle)                      -0.2017100479     -20,800.02
//	port as shipped                         -0.2017100479     -20,800.02   EXACT
//	port + a literal `usap = 0` at the site  -0.2017100479     -23,860.38   OFF BY 3,060.36
//
// WHY IT DOES NOT TRANSFER — corrected by the round-47 adversarial audit, which
// refuted this file's first explanation. `usap` is NOT a by-value parameter of
// Re_Amortize. It is declared in the UNIT-LEVEL var block at `AMORTOP.pas:73`
// (`f, f_1, p, usap, d, … : real;`), and `Re_Amortize` at `:1499` takes only
// `(var p: real)` — so `:1535` writes the unit global. The by-value shadowing
// lives in `Iterate` (`:1415`) and `DetermineLastPaymentDate` (`:1323`), which
// are different routines; that is §68's mechanism, not this one. The running
// accumulator the SCHEDULE walk advances is `RepayFancyLoan`'s separate
// `usapart` var param (`:1101`). See dosport_walk.go:596-599 and
// engine.go:3929/:4000, which read the same way. So `:1535` clears the outer
// global between Re_Amortize passes; it does not reset the schedule's live
// accumulator, and the port — which has no such second variable at this site —
// has nothing to clear. The measurement above is what decides it: transcribing
// the statement moves a value DOS never moves.
//
// STANDING RULE 5: fuzzing locates, only reading explains — and standing rule
// 11: the reading must be MEASURED before it is believed. Both this lead AND
// this file's first explanation of it were refuted that way.
//
// So this file guards TWO things, and the second is the point:
//   1. the DOS golden on a limb nothing else in the tree reaches, and
//   2. that the port's CURRENT usap handling at the AO6 crossing is correct —
//      with the tempting "fix" as the demonstrated killing mutant.
//
// MUTANTS VERIFIED (round 47), all killed:
//   M1  `usap = 0` BEFORE `loan.LoanRate = r`      -> T1 (Δ3060.36) and T2
//   M1b `usap = 0` AFTER  `adj.LoanRateStatus = …` -> T1 (Δ3060.36) and T2
//       (M1b is the DOS-FAITHFUL position — after the `:1526` freeze. The audit
//        found this file's first T2 missed it, because it scanned a fixed
//        character window on the WRONG SIDE of its anchor. T2 now brace-matches
//        the whole block.)
//   M2  USA row accrual perturbed 1%               -> T1 at Δ0.02
//   M3  payment raised above interest              -> T1's POSITIVE CONTROL

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// r47UsaAO6 — USA rule, a payment far below the periodic interest (so the
// US-Rule accumulator genuinely runs), and an amount-only adjustment at
// installment 36 (1 Jan 2028), which makes DOS solve the implied rate.
//
// Provenance: constructed in round 47 to REACH AMORTOP.pas:1535's precondition,
// after the §89 §4 case was found to reach it with usap = 0 — i.e. structurally
// unable to express the difference (R49). Verified against the oracle:
//
//	amort_oracle 100000 0.2000000000 120 12 usa loandmy=1.1.2025 \
//	  firstdmy=1.2.2025 adj=36::900 payhard=100 [adjdump]
//	-> adjrow 1 date 1/1/2028 rate -0.2017100479 ratestatus 1
//	   amount 900.000000 amtstatus 3 amtok TRUE
//	-> payment 100.0000 interest -20800.02 paid 79199.98
//
// The `adjdmy=1.1.2028::900` date form gives byte-identical output.
// Oracle build flags: -Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX.
// -dACTU is ABSENT AND UNBUILDABLE (R47) — this case uses no actuarial path.
const r47UsaAO6 = "100000 0.2000000000 120 12 usa " +
	"loandmy=1.1.2025 firstdmy=1.2.2025 adj=36::900 payhard=100"

const (
	r47DOSAdjRate  = -0.2017100479
	r47DOSInterest = -20800.02
	r47DOSPaid     = 79199.98

	// TOLERANCES, DECLARED WITH PROVENANCE (item 0e is about exactly this).
	//
	// r47RateTol — 5e-6 absolute, the floor dos_fuzzer5_test.go:2815 uses for a
	// solved adjustment rate. ⚠️ That site is a bare inline literal pinned by
	// nothing; round 47 mutated it 5e-6 -> 5e-1 and the suite stayed green.
	// Quoted here as provenance, NOT as authority.
	r47RateTol = 5e-6
	// r47TotalTol — 0.011, DELIBERATELY TIGHTER than the fuzzer's totals floor.
	// The fuzzer judges totals at math.Max(1.0, 5e-4*|v|)
	// (dos_fuzzer5_test.go:2456-2457), which on this case is $10.40. That is a
	// floor for a RANDOMIZED population; this is a STATIC golden read off a
	// 2-decimal oracle print, so a cent-scale floor is the honest one. Stated
	// so the difference is a choice on the record rather than an accident.
	// Both mutants that move this number move it by 3060.36 — they would be
	// caught under either floor. M2 (Δ0.02) would NOT be. That is why it is
	// tight.
	r47TotalTol = 0.011
)

// TestR47USARuleAO6AdjustmentMatchesDOS is T1 — the golden.
func TestR47USARuleAO6AdjustmentMatchesDOS(t *testing.T) {
	in, _ := m5Parse(t, r47UsaAO6)
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v\n  repro: amort_oracle %s adjdump", res.Err, r47UsaAO6)
	}

	// ---- POSITIVE CONTROL (R24 / R49). ---------------------------------
	// The guard is worthless if the sample cannot express the difference. The
	// mutant only bites when the US-Rule accumulator is LIVE at the crossing,
	// which requires rows where interest exceeds the payment. Assert the case
	// really is negative-amortising BEFORE trusting any verdict below — this is
	// the assertion the §89 §4 case would have FAILED (usap = 0 there).
	// Measured round 47: 36 of 120 paid rows. The threshold is 30, a 20% margin;
	// it is deliberately not 1, so that a case which merely grazes the limb
	// cannot silently stand in for one that lives on it.
	negAm := 0
	for _, row := range res.Schedule {
		if row.PayNum > 0 && row.Interest > row.PayAmt {
			negAm++
		}
	}
	if negAm < 30 {
		t.Fatalf("POSITIVE CONTROL FAILED: only %d negative-amortising rows (round 47 "+
			"measured 36); this case no longer reaches AMORTOP.pas:1535's precondition, "+
			"so the mutant below cannot be expressed and the golden proves nothing (R49).\n"+
			"  repro: amort_oracle %s adjdump", negAm, r47UsaAO6)
	}

	// ---- the solved AO6 rate -------------------------------------------
	var got *ResolvedAdjustment
	for i := range res.Adjustments {
		if res.Adjustments[i].RateSolved {
			got = &res.Adjustments[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("no adjustment with a solved rate\n  repro: amort_oracle %s adjdump", r47UsaAO6)
	}
	if d := math.Abs(got.Rate - r47DOSAdjRate); d > r47RateTol {
		t.Errorf("solved AO6 rate = %.10f, DOS says %.10f (delta %.3g, tol %.3g)\n"+
			"  repro: amort_oracle %s adjdump", got.Rate, r47DOSAdjRate, d, r47RateTol, r47UsaAO6)
	}

	// ---- the totals — THIS is what the mutant moves ---------------------
	if d := math.Abs(res.TotalInt - r47DOSInterest); d > r47TotalTol {
		t.Errorf("total interest = %.2f, DOS says %.2f (delta %.2f, tol %.3g)\n"+
			"  🚨 a delta of ~3060.36 here means someone transcribed AMORTOP.pas:1535's\n"+
			"     `usap := 0` into the port's AO6 crossing. DOS's :1535 clears the UNIT\n"+
			"     GLOBAL (AMORTOP.pas:73) between Re_Amortize passes; the schedule walk's\n"+
			"     live accumulator is RepayFancyLoan's separate `usapart` (:1101). The\n"+
			"     port has no second variable to clear here. Measured round 47. Revert it.\n"+
			"  repro: amort_oracle %s", res.TotalInt, r47DOSInterest, d, r47TotalTol, r47UsaAO6)
	}
	if d := math.Abs(res.TotalPaid - r47DOSPaid); d > r47TotalTol {
		t.Errorf("total paid = %.2f, DOS says %.2f (delta %.2f, tol %.3g)\n"+
			"  repro: amort_oracle %s", res.TotalPaid, r47DOSPaid, d, r47TotalTol, r47UsaAO6)
	}

	// ---- assert WHICH engine answered (standing trap) -------------------
	// "There are two engines, and the one you are reading is probably not the
	// one that answered." Recorded so a route change cannot silently retarget
	// this guard onto the other engine. Errorf, not Fatalf: a route move is a
	// re-measure signal, not a defect (R14/R36).
	if res.EngineUsed != "piecewise" {
		t.Errorf("EngineUsed = %q, this guard was measured on \"piecewise\"; if the route "+
			"moved, re-measure the golden before editing it (R14/R36 — a re-measured "+
			"tree is not a regression, but it is also not the same claim)", res.EngineUsed)
	}
}

// TestR47AO6CrossingHasNoUsapWrite is T2 — the SOURCE half, asserted ACROSS
// files (R50: a self-reading guard is unconditionally true, so this test reads
// engine.go, not itself).
//
// 🚨 THIS TEST'S FIRST VERSION WAS A FALSE GUARD. It scanned a fixed 900-char
// window BEFORE an anchor comment; the DOS-faithful insertion point is AFTER
// the freeze (AMORTOP.pas:1526 then :1535), so the audit's M1b killed T1 and
// PASSED T2. It now brace-matches the ENTIRE AO6 freeze block, so position
// inside the block cannot matter.
//
// It is deliberately narrow: it does not forbid `usap` writes in engine.go —
// the three row-accrual sites (:3353, :5170, :5908) and the `usap < 0` clamps
// beside them are correct and required. It forbids a zeroing write inside the
// AO6 freeze block only.
func TestR47AO6CrossingHasNoUsapWrite(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed; cannot locate engine.go")
	}
	enginePath := filepath.Join(filepath.Dir(thisFile), "engine.go")
	src, err := os.ReadFile(enginePath)
	if err != nil {
		t.Fatalf("read %s: %v", enginePath, err)
	}
	text := string(src)

	// COUNT THE NEEDLE FIRST. Use a full phrase, not a token (r46: a needle can
	// miss the only reachable form). The block opens with this exact statement.
	const opener = "{\n\t\t\t\t\t\tloan.LoanRate = r\n"
	n := strings.Count(text, opener)
	if n != 1 {
		t.Fatalf("block opener occurs %d times in engine.go, expected exactly 1 — this "+
			"guard has lost its bearings and must be RE-ANCHORED before it is trusted. "+
			"Do not delete it; a missing anchor here reads as 'unguarded', not 'clean'.", n)
	}
	start := strings.Index(text, opener)

	// Brace-match from the opening `{` to its partner. String and rune literals
	// containing braces would break a naive counter; the block's body is
	// comments plus Go statements, and the assertion above pins the block's
	// identity, so a depth counter over `{`/`}` is sound here — but verify the
	// block closes, and fail loudly if it does not.
	depth, end := 0, -1
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatalf("AO6 freeze block never closes from offset %d — cannot scan it", start)
	}
	block := text[start : end+1]

	// A sanity floor on the extent: the first version of this test effectively
	// examined 35 characters of the block it named. If the matched region is
	// implausibly small the match is wrong, and a passing verdict is worthless.
	if len(block) < 400 {
		t.Fatalf("AO6 freeze block matched only %d chars — implausible; re-anchor "+
			"before trusting a pass (the round-47 audit's finding was exactly this)", len(block))
	}

	// Match every reachable form of a zeroing write. `usap := 0` cannot appear
	// (redeclaration) but is listed because a careless transcription of the
	// Pascal is the exact mutant this guards.
	for _, form := range []string{
		"usap = 0", "usap := 0", "usap *= 0", "usap -= usap",
		"usap = float64(0)", "usap = .0",
	} {
		if strings.Contains(block, form) {
			t.Errorf("found %q inside the AO6 freeze block of engine.go (block is %d chars).\n"+
				"  This is AMORTOP.pas:1535 transcribed literally. MEASURED ROUND 47: it\n"+
				"  moves total interest on the r47UsaAO6 case from -20800.02 (DOS-exact)\n"+
				"  to -23860.38 — a divergence of 3060.36 INTRODUCED by the change, at\n"+
				"  EITHER insertion position. DOS's :1535 clears the unit global\n"+
				"  (AMORTOP.pas:73) between Re_Amortize passes, not the schedule's live\n"+
				"  accumulator (RepayFancyLoan's `usapart`, :1101).",
				form, len(block))
		}
	}
}
