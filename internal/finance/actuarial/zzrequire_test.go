package actuarial

// REQUIRE GATE for the third-party actuarial oracle (round 18b).
//
// WHY THIS EXISTS. `PERSENSE_REQUIRE_ORACLE=1` turns "the DOS oracle is missing"
// from a skip into a hard failure for the amort/pv/mtg drivers. The actuarial
// differentials had no equivalent, and the consequence was not hypothetical:
// they shell out to `scripts/actuarial_oracle.py`, which is one of the five
// DEVICE-ONLY files the bootstrap tarball omits, and they need two pip packages
// that are not preinstalled. So in EVERY cloud container this project has ever
// run, both tests skipped and the package printed
//
//	ok  	github.com/persense/persense-port/internal/finance/actuarial	0.005s
//
// while its only two differentials did nothing — for weeks, on the surface the
// backlog had already written off as "no coverage at all". Standing rule 2 says
// a green suite is not validation and a skipped differential still prints ok;
// this is that rule with teeth.
//
// The gate is deliberately keyed to the SAME variable as the DOS oracles rather
// than a new one. An operator who has said "I am measuring, missing oracles are
// failures" has said it about every oracle, and asking them to remember a second
// variable is how the first one gets forgotten.

import (
	"os"
	"testing"
)

// requireActuarialOracle fails when the third-party oracle is unavailable and
// the operator asked for a gated run; otherwise it skips with a message that
// says exactly how to fix it.
func requireActuarialOracle(t *testing.T, reason string) {
	t.Helper()
	const fix = "install it with `pip install actuarialmath ipython " +
		"--break-system-packages` and make sure scripts/ is in the source tree " +
		"(the bootstrap tarball omits it — see " +
		"claude/workflow_bootstrap_correction_2026-08-02.md)"
	if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
		t.Fatalf("%s, and PERSENSE_REQUIRE_ORACLE is set: this run is supposed to be "+
			"measuring, so a missing oracle is a FAILURE and not a skip. %s", reason, fix)
	}
	t.Skipf("%s. %s", reason, fix)
}
