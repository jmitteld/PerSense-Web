package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// zzmetafuzz_test.go — R4, docs/harness_policy.md: THE HARNESS IS TESTED.
//
// The instrument that decides whether the engine is right had no tests of its
// own. Seven harness defects have been found by noticing that a "divergence"
// made no sense, which is a detection method that depends on somebody looking.
//
// This is the mechanical version. It drives the SHARED PRODUCT ENTRY POINT
// (SolveBlankCellsPrepared / Amortize — R1's `screen.go`) over screens whose DOS
// answers are pinned with oracle provenance, and asserts the harness reports
// AGREEMENT. It is deliberately the inverse of a fidelity test: a failure here is
// read first as "the harness broke", not "the engine broke", because these
// particular screens have already been adjudicated.
//
// What it would have caught, had it existed:
//
//   - unquantized oracle arguments (400 of 400 payments compared unequal)
//   - `cmd/goamort`'s four date bugs and its silent token-ignoring
//   - `firstPeriodDate`'s integer division at sub-monthly frequencies
//   - §58 — the discarded `converged` flag (see the backward corpus below)
//
// PROVENANCE. Every golden is the first line of
//
//	/tmp/oraclebuild/amort_oracle <args>
//
// run 2026-08-02 against the real DOS engine, with the exact argument list in
// each case's `cmd` field. Dates are given EXPLICITLY on every screen rather than
// relying on the driver's defaults — R2: the test must not re-derive a date the
// oracle computed.

type metaCase struct {
	name              string
	cmd               string // the exact oracle invocation these goldens came from
	in                LoanInput
	wantInt, wantPaid float64
	// Backward screens only:
	solveRate bool
	wantRate  float64
}

func metaLoan(amount, rate float64, n, perYr int, ld, fd types.DateRec) Loan {
	return Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty, // blank: the engine solves the payment
		LoanDateStatus: types.InOutInput, LoanDate: ld,
		FirstStatus: types.InOutInput, FirstDate: fd,
	}
}

func d(y int, m time.Month, day int) types.DateRec { return types.NewDateRec(y, m, day) }

func metaCorpus() []metaCase {
	jan1 := d(2024, time.January, 1)

	forward := []metaCase{
		{
			name: "plain-monthly-360",
			cmd:  "100000 0.0925 360 12 loandmy=1.1.2024 firstdmy=1.2.2024",
			in: LoanInput{
				Loan:     metaLoan(100000, 0.0925, 360, 12, jan1, d(2024, time.February, 1)),
				Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
			},
			wantInt: 196163.15, wantPaid: 296163.15,
		},
		{
			name: "r78-exact-inadvance-semimonthly",
			cmd:  "100000 0.1173 48 24 loandmy=1.1.2024 firstdmy=15.1.2024 r78 exact inadv",
			in: LoanInput{
				Loan: metaLoan(100000, 0.1173, 48, 24, jan1, d(2024, time.January, 15)),
				Settings: Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360, YrInv: 1.0 / 360,
					R78: true, Exact: true, InAdvance: true},
			},
			wantInt: 12400.94, wantPaid: 112400.94,
		},
		{
			name: "prepaid-with-points",
			cmd:  "250000 0.075 240 12 loandmy=1.1.2024 firstdmy=1.2.2024 prepaid pts=0.01",
			in: LoanInput{
				Loan:     metaLoan(250000, 0.075, 240, 12, jan1, d(2024, time.February, 1)),
				Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, Prepaid: true},
			},
			wantInt: 235855.92, wantPaid: 485855.92,
		},
		{
			name: "weekly-365",
			cmd:  "150000 0.0999 520 52 loandmy=1.1.2024 firstdmy=8.1.2024 b365",
			in: LoanInput{
				Loan:     metaLoan(150000, 0.0999, 520, 52, jan1, d(2024, time.January, 8)),
				Settings: Settings{Basis: types.Basis365, PerYr: 52, YrDays: 365.25, YrInv: 1.0 / 365.25},
			},
			// NOTE, and the reason this corpus earns its keep: DOS's 365 basis is
			// 365.25 DAYS, not 365 (cmd/goamort/main.go:273). Written as 365 this
			// case fails by $48.12 while the port is provably correct — goamort
			// reproduces DOS to the cent on the same screen. R4's first run caught
			// a harness construction error, which is precisely its job.
			wantInt: 86948.09, wantPaid: 236948.09,
		},
		{
			name: "usa-rule-40yr",
			cmd:  "200000 0.088 480 12 loandmy=1.1.2024 firstdmy=1.2.2024 usa",
			in: LoanInput{
				Loan:     metaLoan(200000, 0.088, 480, 12, jan1, d(2024, time.February, 1)),
				Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, USARule: true},
			},
			wantInt: 525759.77, wantPaid: 725759.77,
		},
	}

	// The points cell on the prepaid screen.
	forward[2].in.Loan.PointsStatus = types.InOutInput
	forward[2].in.Loan.Points = 0.01

	// BACKWARD corpus — the arm §58 lived in. These exercise the shared gate: the
	// harness must solve the rate AND honour the convergence flag.
	backward := []metaCase{
		{
			name:      "exact-29th-anchor-ratesolve",
			cmd:       "40606.39 0.094051 600 24 loandmy=29.6.2021 firstdmy=29.7.2021 exact payhard=176.65 norate",
			solveRate: true, wantRate: 0.0940493440,
			wantInt: 63871.65, wantPaid: 104478.04,
		},
		{
			name:      "exact-15th-anchor-ratesolve",
			cmd:       "40606.39 0.094051 600 24 loandmy=15.6.2021 firstdmy=15.7.2021 exact payhard=176.65 norate",
			solveRate: true, wantRate: 0.0940493440,
			wantInt: 65383.41, wantPaid: 105989.80,
		},
		{
			name: "near-perpetuity-80yr-ratesolve",
			cmd: "291207.99 0.1209560000 1920 24 exact prepaid loandmy=29.5.2024 " +
				"firstdmy=29.7.2024 pts=0.009110 payhard=1962.94 norate",
			solveRate: true, wantRate: 0.1617759257,
			wantInt: 1278016.78, wantPaid: 1569224.77,
		},
	}

	for i, anchor := range []int{29, 15} {
		backward[i].in = LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 40606.39,
				LoanRateStatus: types.StatusEmpty,
				NStatus:        types.InOutInput, NPeriods: 600,
				PerYrStatus: types.InOutInput, PerYr: 24,
				PayAmtStatus: types.InOutInput, PayAmt: 176.65,
				LoanDateStatus: types.InOutInput, LoanDate: d(2021, time.June, anchor),
				FirstStatus: types.InOutInput, FirstDate: d(2021, time.July, anchor),
			},
			Settings: Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360,
				YrInv: 1.0 / 360, Exact: true},
		}
	}
	backward[2].in = LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 291207.99,
			LoanRateStatus: types.StatusEmpty,
			NStatus:        types.InOutInput, NPeriods: 1920,
			PerYrStatus: types.InOutInput, PerYr: 24,
			PayAmtStatus: types.InOutInput, PayAmt: 1962.94,
			PointsStatus: types.InOutInput, Points: 0.009110,
			LoanDateStatus: types.InOutInput, LoanDate: d(2024, time.May, 29),
			FirstStatus: types.InOutInput, FirstDate: d(2024, time.July, 29),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360,
			YrInv: 1.0 / 360, Exact: true, Prepaid: true},
	}

	return append(forward, backward...)
}

// TestMetaHarnessAgreesOnKnownAgreeingScreens is the R4 tripwire.
//
// A failure here means the HARNESS changed, not that the port regressed — these
// screens are already adjudicated. Read it that way before touching the engine.
func TestMetaHarnessAgreesOnKnownAgreeingScreens(t *testing.T) {
	for _, tc := range metaCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in

			if tc.solveRate {
				// Through the SHARED entry point — the same call dos_fuzzer5_test.go
				// makes. If a future change drops the convergence gate again (§58) or
				// alters the solve/write-back contract, this is where it shows.
				out, err := SolveBlankCellsPrepared(in, in, false, true)
				if err != nil {
					t.Fatalf("harness refused a screen DOS answers: %v\n  oracle: %s", err, tc.cmd)
				}
				if got := out.Loan.LoanRate; math.Abs(got-tc.wantRate) > 5e-10 {
					t.Errorf("solved rate = %.10f, DOS %.10f (delta %.2e)\n  oracle: %s",
						got, tc.wantRate, got-tc.wantRate, tc.cmd)
				}
				if !out.RateWasSolved {
					t.Errorf("RateWasSolved not set — the write-back contract changed "+
						"(Amortize.pas:1377's rate arm)\n  oracle: %s", tc.cmd)
				}
				in = out
			}

			r := Amortize(in)
			if r.Err != nil {
				t.Fatalf("Amortize refused a screen DOS answers: %v\n  oracle: %s", r.Err, tc.cmd)
			}
			if math.Abs(r.TotalInt-tc.wantInt) > 0.005 {
				t.Errorf("interest = %.2f, DOS %.2f (delta %+.2f)\n  oracle: %s",
					r.TotalInt, tc.wantInt, r.TotalInt-tc.wantInt, tc.cmd)
			}
			if math.Abs(r.TotalPaid-tc.wantPaid) > 0.005 {
				t.Errorf("paid = %.2f, DOS %.2f (delta %+.2f)\n  oracle: %s",
					r.TotalPaid, tc.wantPaid, r.TotalPaid-tc.wantPaid, tc.cmd)
			}
		})
	}
}

// TestMetaHarnessHonoursTheConvergenceGate pins §58's contract at the level the
// harness actually consumes it, so a regression in `screen.go` fails here as well
// as in zzsec58_nonconverge_test.go.
//
// n=2160 on the near-perpetuity screen is the first term DOS refuses
// ("ERR Computation of payment amount or interest rate did not converge").
// The shared entry point must refuse it too — via ErrDidNotConverge, not by
// returning the unrefined estimate.
func TestMetaHarnessHonoursTheConvergenceGate(t *testing.T) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 291207.99,
			LoanRateStatus: types.StatusEmpty,
			NStatus:        types.InOutInput, NPeriods: 2160,
			PerYrStatus: types.InOutInput, PerYr: 24,
			PayAmtStatus: types.InOutInput, PayAmt: 1962.94,
			PointsStatus: types.InOutInput, Points: 0.009110,
			LoanDateStatus: types.InOutInput, LoanDate: d(2024, time.May, 29),
			FirstStatus: types.InOutInput, FirstDate: d(2024, time.July, 29),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360,
			YrInv: 1.0 / 360, Exact: true, Prepaid: true},
	}
	if _, err := SolveBlankCellsPrepared(in, in, false, true); err == nil {
		t.Errorf("the shared entry point returned a schedule for a screen DOS refuses " +
			"(n=2160, near-perpetuity rate solve). This is §58 reopening: the harness " +
			"would amortize at a rate the product blocks. See docs/harness_policy.md R1.")
	}
}
