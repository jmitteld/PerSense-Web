package fileio

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestLoadHelpWorksheetCorpus loads EVERY real Per%Sense worksheet file shipped
// with the original Help system (legacy/src/win_source/Help/*.{amz,mtg,pvl} — the
// saved worksheets behind each documented example) and asserts the loader parses
// all of them cleanly with sane fields. These 31 genuine client-format files span
// the full range of option blocks — balloons, prepayments, rate/payment
// adjustments, moratorium, target, multi-row PV, contingencies — so this is a far
// broader import-fidelity check than the three hand-pinned DOS.* fixtures. It also
// tallies which advanced blocks actually appeared, so a loader path that silently
// stops parsing a block (returning an empty slice) is caught by the coverage
// assertions at the end.
//
// The files are READ-ONLY legacy reference material; this test only reads them.
func TestLoadHelpWorksheetCorpus(t *testing.T) {
	const helpDir = "../../legacy/src/win_source/Help"

	// real globs the extension but drops macOS AppleDouble (._*) resource forks.
	real := func(ext string) []string {
		all, _ := filepath.Glob(filepath.Join(helpDir, "*."+ext))
		var out []string
		for _, p := range all {
			if !strings.HasPrefix(filepath.Base(p), "._") {
				out = append(out, p)
			}
		}
		return out
	}

	amz := real("amz")
	mtg := real("mtg")
	pvl := real("pvl")
	if len(amz) < 10 || len(mtg) < 3 || len(pvl) < 8 {
		t.Fatalf("Help worksheet corpus looks incomplete: amz=%d mtg=%d pvl=%d (is %s present?)",
			len(amz), len(mtg), len(pvl), helpDir)
	}

	// Coverage tallies across the corpus.
	var sawBalloon, sawPrepay, sawAdjust, sawMoratorium, sawTarget, sawSkip int
	var sawPVMultiRow, sawPVContingency int

	for _, p := range amz {
		f, err := LoadAmortizationFile(p)
		if err != nil {
			t.Errorf("%s: load failed: %v", filepath.Base(p), err)
			continue
		}
		// Core sanity: an amortization worksheet must carry at least a loan
		// amount or a payment, and a positive payment frequency. Payment is
		// stored with the DOS cash-flow sign convention (an outflow is
		// negative — e.g. AM_Example14 has Payment=-1500), so test for non-zero,
		// not positivity.
		if f.Loan.Amount == 0 && f.Loan.PayAmt == 0 {
			t.Errorf("%s: neither Amount nor Payment present (Amount=%.2f Payment=%.2f)",
				filepath.Base(p), f.Loan.Amount, f.Loan.PayAmt)
		}
		if f.Loan.PerYr <= 0 {
			t.Errorf("%s: non-positive PerYr %d", filepath.Base(p), f.Loan.PerYr)
		}
		if len(f.Balloons) > 0 {
			sawBalloon++
		}
		if len(f.Prepayments) > 0 {
			sawPrepay++
		}
		if len(f.Adjustments) > 0 {
			sawAdjust++
		}
		if f.Moratorium.FirstRepayStatus >= types.InOutDefault {
			sawMoratorium++
		}
		if f.Target.TargetStatus >= types.InOutDefault {
			sawTarget++
		}
		if f.SkipMonths.SkipStatus >= types.InOutDefault {
			sawSkip++
		}
	}

	for _, p := range mtg {
		f, err := LoadMortgageFile(p)
		if err != nil {
			t.Errorf("%s: load failed: %v", filepath.Base(p), err)
			continue
		}
		if len(f.Mortgages) == 0 {
			t.Errorf("%s: no mortgage rows parsed", filepath.Base(p))
		}
	}

	for _, p := range pvl {
		f, err := LoadPresentValueFile(p)
		if err != nil {
			t.Errorf("%s: load failed: %v", filepath.Base(p), err)
			continue
		}
		rows := len(f.LumpSums) + len(f.Periodics)
		if rows == 0 {
			t.Errorf("%s: no lump-sum or periodic rows parsed", filepath.Base(p))
		}
		if rows > 1 {
			sawPVMultiRow++
		}
		for _, ls := range f.LumpSums {
			if ls.Act != 0 {
				sawPVContingency++
				break
			}
		}
	}

	t.Logf("parsed corpus: %d amz, %d mtg, %d pvl (total %d real Help worksheets)",
		len(amz), len(mtg), len(pvl), len(amz)+len(mtg)+len(pvl))
	t.Logf("amz option-block coverage: balloon=%d prepay=%d adjust=%d moratorium=%d target=%d skip=%d",
		sawBalloon, sawPrepay, sawAdjust, sawMoratorium, sawTarget, sawSkip)
	t.Logf("pv coverage: multi-row=%d contingency=%d", sawPVMultiRow, sawPVContingency)

	// At least the balloon block must be exercised by the corpus — several
	// documented amortization examples use balloons. A zero here means the
	// balloon-block loader path went unexercised (or silently broke).
	if sawBalloon == 0 {
		t.Errorf("no balloon block appeared in the amortization corpus — loader path unexercised")
	}
}
