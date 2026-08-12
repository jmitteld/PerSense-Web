ITEM 0j — ACCEPTANCE-CLAUSE TRACE. Apply with `patch -p1`, run, then REVERT.
=========================================================================
Round 50 filed item 0j and its code was lost; round 54 redid it. This recipe
is the instrument, committed as a patch rather than left in /tmp — r53's own
audit established that an UNCOMMITTED instrument makes its finding
unfalsifiable in both directions, and r52 lost a whole round to exactly that.

WHAT IT PRINTS. One `ACC=` line per acceptance decision in dosIterateCore:

    ACC= bestp=<residual> halfpenny=<bool> accInit=<signed init> accTol=<threshold>
         relRescued=<bool> verdict=<accept|refuse>

`relRescued` is the whole question: it is true exactly when the half-penny
test FAILED but the relative clause accepted anyway. DOS
(AMORTOP.pas:1487) can only do that when `acc_limit * init` is POSITIVE, i.e.
when init > 0.

WHAT TO DO WITH IT. Run any generator through it and grep:

    go test -c -o /tmp/0j.test ./internal/finance/amortization/
    PERSENSE_FUZZ=1 PERSENSE_REQUIRE_ORACLE=1 PERSENSE_FUZZ_SEED=50100 \
      PERSENSE_FUZZ_N=400 DP0JTRACE=1 /tmp/0j.test \
      -test.run TestDOSFuzzer5AllAdvancedOptions -test.v 2>&1 \
      | grep '^ACC=' > /tmp/acc.txt
    grep -c 'accInit=-'            /tmp/acc.txt   # negative-init decisions
    grep 'accInit=-' /tmp/acc.txt | grep -c 'relRescued=true'

🚨 THE STANDING RESULT THIS EXISTS TO RE-TEST. Round 50 measured the APR class
arm at 562 acceptance decisions with ZERO carrying a negative accInit, which is
why item 0j's behavioural guard uses a SYNTHETIC terminal and why the fix moved
nothing on the round-54 paired regression (FIXED 0 / NEW 0, seeds 50100-50109,
N=400). That zero is a statement about its GENERATOR (R31). The point of this
patch is to re-ask it on a WIDER one. A negative-init decision found anywhere
reachable promotes item 0j from a fidelity fix to a defect closure.

⚠️ It writes to stderr, so it is exactly the shape of emitter that
desensitized `paired_regression.sh` (R74). The gate now refuses
FZ5CASEDUMP / PERSENSE_FUZZ_FLAKEDUMP / FZ5DISCRIMDUMP by name; DP0JTRACE is
NOT on that list because this patch is not committed into the build. NEVER
apply this patch and run paired_regression.sh in the same tree.
