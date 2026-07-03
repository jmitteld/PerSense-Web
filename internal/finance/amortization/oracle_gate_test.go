package amortization

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// oracleGateEnv is the environment variable that flips the DOS-oracle differential
// tests from "skip when the oracle is absent" to "FAIL when the oracle is absent or
// not runnable". CI sets it (see the Makefile `ci` target) so a missing/wrong oracle
// can never be silently skipped and mistaken for a green build — the exact trap that
// let a session's engine regressions hide behind `go test ./...` reporting "ok".
const oracleGateEnv = "PERSENSE_REQUIRE_ORACLE"

// gateOracle is the single decision point every DOS-oracle differential test uses
// instead of an inline os.Stat + t.Skip. When the oracle binary is present it returns
// and the test proceeds. When it is absent it SKIPS — unless PERSENSE_REQUIRE_ORACLE
// is set, in which case it FAILS. This makes the differential suite fail-closed under
// the CI gate while staying convenient locally.
func gateOracle(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(oracleBin); err == nil {
		return
	}
	msg := "DOS oracle binary not present (" + oracleBin + "); build via legacy/oracle/build_linux.sh"
	if os.Getenv(oracleGateEnv) != "" {
		t.Fatalf("%s set but %s", oracleGateEnv, msg)
	}
	t.Skip(msg)
}

// TestOracleGate is the belt-and-suspenders guard: it always runs, and when
// PERSENSE_REQUIRE_ORACLE is set it fails unless the oracle is present AND actually
// RUNNABLE (a mere os.Stat is not enough — the checked-in binary is macOS-only and
// exists-but-won't-exec on Linux, which is how a stale/wrong oracle could otherwise
// slip through). It execs a known smoke case and checks the payment. This single test
// makes the whole suite fail-closed in CI regardless of how the individual sweeps gate.
func TestOracleGate(t *testing.T) {
	required := os.Getenv(oracleGateEnv) != ""
	if _, err := os.Stat(oracleBin); err != nil {
		if required {
			t.Fatalf("%s set but oracle binary absent (%s); run legacy/oracle/build_linux.sh", oracleGateEnv, oracleBin)
		}
		t.Skipf("oracle binary absent (%s)", oracleBin)
	}
	// Smoke: amort_oracle 10000 0.12 12 12 -> "payment 888.4879 interest 661.85 paid ...".
	out, err := exec.Command(oracleBin, "10000", "0.12", "12", "12").Output()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(out)), "payment ") {
		msg := "oracle present but not runnable (macOS binary on Linux? rebuild with legacy/oracle/build_linux.sh): err=" +
			errString(err) + " out=" + strings.TrimSpace(string(out))
		if required {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
