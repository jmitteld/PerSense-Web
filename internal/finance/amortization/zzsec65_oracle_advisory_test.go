package amortization

// zzsec65_oracle_advisory_test.go — regression gate for §65's ADVISORY subclass
// (round 32). THIS TEST PINS THE ORACLE, NOT THE ENGINE.
//
// WHAT WAS WRONG. For nine rounds the harness recorded
//
//	Internal error - last payment not found.  Please contact Ones & Zeros.
//
// as a DOS REFUSAL — a screen the original engine declines to answer. It is not
// one. `RepayFancyLoan` raises it at AMORTOP.pas:1226-1233:
//
//	if ((not h^.lastok) and (WhenToStop^.principal = 0)) then
//	  begin if (not entire) then h^.lastdate := WhenToStop^.date; end
//	else if (DateComp(WhenToStop^.date, very_last) > 0) and (not balance_Calc) then
//	  MessageBox('Internal error - last payment not found. …', DA_InternalError );
//	h^.loanrate := saverate;
//	ComputeTrueRate;
//	DisposeOfOld_Pre;
//
// A BARE STATEMENT — no `exit`, no `errorflag := true`. Control falls straight
// through the epilogue and RepayFancyLoan returns normally. Real DOS's
// MessageBox (dos_source/Globals.pas:107-116) calls MessageDialog.ShowMessage,
// and MessageDialogUnit.pas:63 is a Delphi TForm: it sets a caption, shows an OK
// button, and latches nothing. In the real product the user dismisses a dialog
// and THE SCHEDULE IS DRAWN.
//
// The refusal was the ORACLE DRIVER, not the engine:
//
//	amort_oracle.pas:1101-1109
//	  MakeTable(Output, false);        <-- the table IS BUILT, into Output
//	  if OracleErrorFired then
//	  begin Writeln('ERR ', OracleFirstError); Halt(0); end;   <-- and DISCARDED
//
// legacy/oracle/Globals.pas now swallows DA_InternalError ($02010017), which is
// the FOURTH bare-statement MessageBox corrected there for exactly this reason
// (the other three: DA_ChangeTo365, DA_APRNoConverge, DA_TerminatingBalloonChanged).
//
// WHY THIS IS A TEST AND NOT A NOTE. The harness's authority is the oracle
// binary. A change to it is invisible to every Go assertion in this package
// except one that reads the binary's own output, so without this file the
// correction can be silently reverted by a rebuild from an older tree and
// nothing goes red. Standing rule 12 — the harness is a suspect before the
// engine is — cuts both ways: a corrected harness needs a gate too.
//
// SEEN TO FAIL, BOTH DIRECTIONS (rule 3), 2026-08-04. Two amort_oracle binaries
// built from trees differing in the single swallow line, md5s 430952b1… (PRE)
// and b1301ec3… (POST):
//
//	rung                          PRE                        POST
//	advisory (b364/b480/adj756)   ERR Internal error …       payment 2754.1856 …
//	still-refuses (adj568 …)      ERR Internal error …       ERR … did not converge
//	control (plain 4-arg)         payment 888.4879 …         payment 888.4879 …  (identical)
//
// The middle rung is the POSITIVE CONTROL required by R24: a change that
// swallows one help code and not the error path must still let a real refusal
// through. Without it, "the swallow works" and "the oracle stopped reporting
// errors" look identical. The third is the NEGATIVE CONTROL (R19); it is one row
// of the 145-of-145 byte-identical corpus measured by
// testplan/harness/audit_sec65_messagebox_probe.py.
//
// WHAT THIS TEST DELIBERATELY DOES NOT ASSERT: that the port AGREES with the
// table DOS now emits. It does not — on the 95 harvested in-scope repros DOS's
// table diverges from the port's on 54 of the 68 that could be measured, and the
// standing arms score those as HARD (in-scope HARD 0 → 123 on seeds
// 50100-50139). That is the open engine work, and pinning it here would encode
// today's wrong numbers as an expectation.

import (
	"os/exec"
	"strings"
	"testing"
)

// The three rungs' commands. Provenance: fuzzer5 seed 50100, FZ5CASEDUMP=1 /
// FLAKEDUMP=1 harvest, 2026-08-04. The full command is printed on every failure
// so a reader never has to reconstruct it from this file.
const (
	// An in-scope §65 advisory screen: DOS's walk runs past very_last with
	// principal outstanding, raises DA_InternalError, and goes on to build a
	// table.
	sec65AdvisoryCase = "75102.49 0.1139540000 324 3 b365_360 prepaid plusreg " +
		"loandmy=7.1.2023 firstdmy=7.5.2023 mor=172 b364=13676.12 b480=6824.70 " +
		"pre=668:227:12:35.63 adj=756::2998.23 pts=0.007500"

	// Same subclass, but once the advisory is swallowed a REAL engine error
	// surfaces underneath it. The positive control.
	sec65StillRefusesCase = "195172.77 0.0780800000 348 3 b365_360 prepaid plusreg r78 usa " +
		"loandmy=11.5.2025 firstdmy=11.9.2025 mor=572 b588=46921.30 b756=5262.26 " +
		"pre=256:248:12:77.83 adj=92:0.0930990000:6562.06 adj=424::3990.03 " +
		"adj=568:0.0677100000:5349.42 targ=449.09 pts=0.003887 payhard=6509.76"

	sec65AdvisoryMsg = "last payment not found"
)

func sec65Oracle(t *testing.T, line string) string {
	t.Helper()
	cmd := exec.Command(oracleBin, strings.Fields(line)...)
	out, _ := cmd.Output() // a refusing run still exits 0 and prints ERR on stdout
	return string(out)
}

// TestSec65AdvisoryIsNotARefusal is the gate. It fails against any amort_oracle
// built from a tree that still records DA_InternalError as an oracle error.
func TestSec65AdvisoryIsNotARefusal(t *testing.T) {
	gateOracle(t)

	t.Run("advisory screen yields a table", func(t *testing.T) {
		out := sec65Oracle(t, sec65AdvisoryCase)
		if strings.Contains(out, sec65AdvisoryMsg) {
			t.Fatalf("the oracle still reports DOS's ADVISORY dialog as a refusal.\n"+
				"AMORTOP.pas:1233 is a bare MessageBox — no exit, no errorflag — and\n"+
				"MakeTable has already filled Output by the time it fires. Rebuild the\n"+
				"oracle from a tree whose legacy/oracle/Globals.pas swallows $02010017.\n"+
				"  got: %q\n  repro: amort_oracle %s", strings.TrimSpace(out), sec65AdvisoryCase)
		}
		if !strings.Contains(out, "interest ") || !strings.Contains(out, "paid ") {
			t.Fatalf("no advisory message, but no result line either — the oracle is\n"+
				"broken in a third way and this test must not read that as a pass (R12).\n"+
				"  got: %q\n  repro: amort_oracle %s", strings.TrimSpace(out), sec65AdvisoryCase)
		}
	})

	// POSITIVE CONTROL (R24). Swallowing one help code must not swallow the
	// error path. This screen is the same subclass and still refuses, for a
	// stated reason that is not the advisory.
	t.Run("positive control: a real refusal still refuses", func(t *testing.T) {
		out := sec65Oracle(t, sec65StillRefusesCase)
		if !strings.HasPrefix(strings.TrimSpace(out), "ERR ") {
			t.Fatalf("the positive control STOPPED refusing. Either the swallow is wider\n"+
				"than one help code, or this screen's behaviour moved for another reason —\n"+
				"and 'the fix works' and 'the oracle stopped reporting errors' look the\n"+
				"same from here (R24).\n  got: %q\n  repro: amort_oracle %s",
				strings.TrimSpace(out), sec65StillRefusesCase)
		}
		if strings.Contains(out, sec65AdvisoryMsg) {
			t.Fatalf("the positive control refuses with the ADVISORY message, so it is not\n"+
				"controlling anything — it is the subject.\n  got: %q\n  repro: amort_oracle %s",
				strings.TrimSpace(out), sec65StillRefusesCase)
		}
	})

	// NEGATIVE CONTROL (R19). The swallow must be inert on a screen DOS answers.
	// This is the build's own smoke case, so a failure here means the oracle
	// changed in a way that has nothing to do with §65.
	t.Run("negative control: an answered screen is unchanged", func(t *testing.T) {
		out := strings.TrimSpace(sec65Oracle(t, "10000 0.12 12 12"))
		const want = "payment 888.4879 interest 661.85 paid 10661.85"
		if out != want {
			t.Fatalf("the swallow moved a screen DOS already answered.\n  want %q\n  got  %q", want, out)
		}
	})
}
