package amortization

// zzm5tack_test.go — SCRATCH probe for §46 TackOnFinalBalloon divergences.
// Driven by M5, same as zzm5_test.go. Prints the gate decision and each arm of
// Amortize.pas:1040-1088 so the Pascal can be walked against the port.

import (
	"os"
	"testing"

	"github.com/persense/persense-port/internal/dateutil"
)

func TestM5Tack(t *testing.T) {
	gateOracle(t)
	line := os.Getenv("M5")
	if line == "" {
		t.Skip("set M5=<oracle arg line>")
	}
	in, _ := m5Parse(t, line)
	m5PreSolveRate(t, &in)

	l := in.Loan
	if err := FirstPass(&l); err != nil {
		t.Fatalf("FirstPass: %v", err)
	}
	settings := in.Settings
	tackIn := in
	tackIn.Loan = l

	t.Logf("gate: Fancy=%v AmountStatus=%d LoanRateStatus=%d PayAmtStatus=%d "+
		"nadj=%d nballoons=%d LastOK=%v LastDate=%v NPeriods=%d PerYr=%d",
		in.Fancy, l.AmountStatus, l.LoanRateStatus, l.PayAmtStatus,
		len(in.Adjustments), len(in.Balloons), l.LastOK,
		l.LastDate.Time.Format("2006-01-02"), l.NPeriods, l.PerYr)
	t.Logf("gate open? %v", tackOnGateOpen(&tackIn))

	vl := determineVeryLast(&l, in.Balloons, in.Prepayments)
	t.Logf("determineVeryLast = %v (ok=%v)", vl.Time.Format("2006-01-02"), dateutil.DateOK(vl))
	for i, b := range in.Balloons {
		t.Logf("  balloon[%d] date=%v (st %d) amount=%.4f (st %d)", i,
			b.Date.Time.Format("2006-01-02"), b.DateStatus, b.Amount, b.AmountStatus)
	}
	for i, pp := range in.Prepayments {
		t.Logf("  prepay[%d] start=%v stop=%v (st %d) nn=%d (st %d) peryr=%d amt=%.4f", i,
			pp.StartDate.Time.Format("2006-01-02"), pp.StopDate.Time.Format("2006-01-02"),
			pp.StopDateStatus, pp.NN, pp.NNStatus, pp.PerYr, pp.Payment)
	}

	res := tackOnFinalBalloon(tackIn, &settings)
	t.Logf("tack: Fired=%v Live=%v MergeIdx=%d Date=%v Amount=%.4f Adjusted=%v",
		res.Fired, res.Live, res.MergeIdx, res.Date.Time.Format("2006-01-02"),
		res.Amount, res.Adjusted)

	gr := Amortize(in)
	if gr.Err != nil {
		t.Logf("Amortize err: %v", gr.Err)
	}
	t.Logf("result: rows=%d totalInt=%.2f totalPaid=%.2f finalPrinc=%.2f", len(gr.Schedule),
		gr.TotalInt, gr.TotalPaid, gr.FinalPrinc)
	for i, b := range gr.Balloons {
		t.Logf("  result balloon[%d] %v %.4f tacked=%v", i,
			b.Date.Time.Format("2006-01-02"), b.Amount, b.TackedOn)
	}
}
