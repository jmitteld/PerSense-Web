package amortization

// LONG-HORIZON BACKWARD-SOLVE DIFFERENTIAL (round 20).
//
// WHY THIS FILE EXISTS
// --------------------
// The backlog's number-one item since round 18b was "post-2100 backward solves
// are still the most likely home for an undetected systematic defect." Two
// instruments were supposed to cover that ground and neither did:
//
//   - `zzbits_backward_test.go` (R11) draws `n = (3 + rand(27)) * peryr` off a
//     2024 loan date, so its longest schedule ends in 2053. It has never
//     evaluated a single post-2100 backward solve. It also covers only two of
//     the four backward cells: `norate` and `noamt`.
//   - `testplan/harness/long_horizon_sweep.py` reaches past 2100 but is a
//     FORWARD differential — it is structurally blind to a solve.
//
// So the term-solve cells (`noterm`, `non`) had no bit- or exact-level
// differential at any horizon, and the value cells had none past 2053. This
// file closes both gaps on one axis: three horizon strata x four backward cells.
//
// WHAT IT ASSERTS, AND WHY EACH ASSERTION IS SHAPED THE WAY IT IS
// ---------------------------------------------------------------
//  1. COVERAGE IS PROVEN, NOT LABELLED. A stratification label is a coverage
//     claim (round 17, and it cost four rounds of mis-priced §54). This test
//     therefore asserts against DOS'S OWN REPORTED last dates that the
//     post-2100 stratum actually produced post-2100 schedules. A stratum that
//     silently degenerates to short terms would otherwise pass while measuring
//     nothing.
//  2. A REFUSAL IS PAIRED, NEVER DROPPED. DOS declines roughly a third of
//     long-horizon `noterm` screens ("Payment amount is too small to compute
//     number of periods"). The obvious harness drops those cases. That is a
//     silent bucket in exactly the sense R8 and round 19's OOM are about, and
//     it is where a refusal asymmetry would hide — the port answering a screen
//     DOS refuses is the worst direction of divergence there is. Every DOS
//     no-solve here is run through the port and counted.
//  3. THE TERM CELLS ARE EXACT, NOT TOLERANCED. `nperiods` is an integer and a
//     last date is three integers. There is no rounding to forgive, so the
//     assertion is equality and the count of differences must be zero.
//  4. THE VALUE CELLS REUSE R11 + R14. Same ULP tail bound, same acceptance-band
//     ratio, same reasoning as `zzbits_backward_test.go` — see the long note on
//     assertion 3 there for why a ULP count is not the unit that decides
//     severity for a solver.
//
// WHAT ROUND 20 MEASURED WITH IT — PERSENSE_BITS_N=1500, 3 strata, 17,031
// paired backward solves. These are this test's own printed numbers (R13):
//
//	noamt   4500 compared, 4500 bit-identical, 0 non-exact, all three strata
//	norate  4447 compared, 64 non-exact — 63 of them in the <=2053 CONTROL
//	        band (max 227 ULP), 1 in the straddle (32 ULP), and the post-2100
//	        stratum is bit-clean at 1447 of 1447. Worst acceptance-band ratio
//	        9.1e-09, i.e. eight orders of magnitude inside DOS's own tolerance.
//	noterm  3584 compared, 0 nperiods differences, 0 last-date differences.
//	        DOS's own reported last years span 1902-2155 — the §55 year byte
//	        wrapping below 1900 at one end and sitting on its ceiling at the
//	        other — and the port reproduces every one of them exactly.
//	non     4500 compared, 0 nperiods differences, 0 last-date differences,
//	        DOS last years 2027-2148.
//	        916 DOS `noterm` refusals, 916 also refused by the port, 0 answered.
//	        All 916 carry the same reason ("Payment amount is too small to
//	        compute number of periods"), so the pairing currently exercises ONE
//	        refusal class — the bucket is no longer silent, but do not read it
//	        as coverage of DOS's refusal behaviour generally.
//
// The conclusion is a negative one and it is worth stating plainly: on this
// axis, post-2100 backward solves are NOT a home for an undetected systematic
// defect. The one lean that does show up lives in the SHORT-term band and is
// DOS's own stopping rule (see zzbits_backward_test.go).

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// backwardHorizonStratum draws whole-year terms in [loYears, hiYears] off the
// shared 2024 loan date. The year bounds are chosen so the SCHEDULES land where
// the name says: control ends by 2053, straddle spans DOS's Feb-2100 century
// boundary (§54), and post2100 sits entirely beyond it while staying inside
// DOS's 2155 year byte (§55).
type backwardHorizonStratum struct {
	name             string
	loYears, hiYears int
	// wantMaxYearAtLeast is the coverage assertion: DOS's own reported last
	// dates in this stratum must reach at least this year. 0 disables it.
	wantMaxYearAtLeast int
}

// termCellStat is the exact-comparison counterpart of backwardBitStat, for the
// two cells whose answer is integers rather than a double.
type termCellStat struct {
	name             string
	compared         int
	dosNoSolve       int
	goNoSolve        int
	nDiff, dateDiff  int
	worst            string
	dosMinY, dosMaxY int
	// refusal pairing
	dosRefusedGoAnswered int
	dosRefusedGoRefused  int
	refusalReasons       map[string]int
	goOnlyExample        string
}

func (s *termCellStat) noteDOSYear(y int) {
	if s.dosMinY == 0 || y < s.dosMinY {
		s.dosMinY = y
	}
	if y > s.dosMaxY {
		s.dosMaxY = y
	}
}

// parseSolvedTermLine reads `solvedterm <n> last <y>-<m>-<d>`
// (amort_oracle.pas:1216), the line the `noterm` driver already emitted long
// before anything read it.
func parseSolvedTermLine(out string) (n, y, m, d int, ok bool) {
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) >= 4 && f[0] == "solvedterm" && f[2] == "last" {
			p := strings.Split(f[3], "-")
			if len(p) != 3 {
				return 0, 0, 0, 0, false
			}
			n, _ = strconv.Atoi(f[1])
			y, _ = strconv.Atoi(p[0])
			m, _ = strconv.Atoi(p[1])
			d, _ = strconv.Atoi(p[2])
			return n, y, m, d, true
		}
	}
	return 0, 0, 0, 0, false
}

// parseBdumpLastLine reads `lastdate M/D/YYYY nperiods N` from a `bdump`
// (amort_oracle.pas:1051). `non` does not emit `solvedterm` — the term it
// derives from the typed last date shows up here instead.
func parseBdumpLastLine(out string) (n, y, m, d int, ok bool) {
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) >= 4 && f[0] == "lastdate" && f[2] == "nperiods" {
			p := strings.Split(f[1], "/")
			if len(p) != 3 {
				continue
			}
			m, _ = strconv.Atoi(p[0])
			d, _ = strconv.Atoi(p[1])
			y, _ = strconv.Atoi(p[2])
			n, _ = strconv.Atoi(f[3])
			ok = true
		}
	}
	return
}

func oracleErrLine(out string) string {
	reason := "(no ERR line)"
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "ERR ") {
			reason = strings.TrimSpace(ln)
		}
	}
	return reason
}

func TestDOSBackwardSolveLongHorizon(t *testing.T) {
	bin := oracleBin
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but not present (%s)", bin)
		}
		t.Skipf("amort oracle not present (%s); build via legacy/oracle/build_linux.sh", bin)
	}
	env := append(os.Environ(), "PERSENSE_ORACLE_RAWBITS=1")

	// 250 per stratum is ~6s on two cores and is what the standing suite runs.
	// PERSENSE_BITS_N raises it for a measurement pass; round 20 used 1500.
	cases := 250
	if v := os.Getenv("PERSENSE_BITS_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cases = n
		}
	}

	strata := []backwardHorizonStratum{
		// The R11 band, kept as the matched control: any effect that shows up
		// only in the long strata has to be read against this one.
		{"control_to_2053", 3, 29, 0},
		// Straddles Feb 2100 — §54's two-calendar boundary.
		{"straddle_2084_2103", 60, 79, 2100},
		// Entirely past it, and short of §55's 2155 year byte.
		{"post2100_2104_2148", 80, 124, 2120},
	}

	for _, st := range strata {
		st := st
		t.Run(st.name, func(t *testing.T) {
			// Seeded per stratum so a failure reproduces standalone.
			rng := rand.New(rand.NewSource(int64(20200 + st.loYears)))
			rateStat := &backwardBitStat{name: "solvedrate (norate)"}
			amtStat := &backwardBitStat{name: "solvedamount (noamt)"}
			termStat := &termCellStat{name: "noterm", refusalReasons: map[string]int{}}
			nStat := &termCellStat{name: "non", refusalReasons: map[string]int{}}

			for i := 0; i < cases; i++ {
				amount := quantize(float64(25000+rng.Intn(475000)), 2)
				rate := quantize(0.03+rng.Float64()*0.11, 6)
				perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
				years := st.loYears + rng.Intn(st.hiYears-st.loYears+1)
				n := years * perYr

				// Same construction as R11: plain loans, payment drawn OFF the
				// fair value so the solve is non-degenerate, every argument
				// quantized to the decimals the oracle will parse back.
				fair, _, ok := goSolve(amount, rate, n, perYr)
				if !ok {
					continue
				}
				pay := quantize(fair*(0.85+rng.Float64()*0.5), 2)
				if pay <= 0 {
					continue
				}
				args := []string{
					strconv.FormatFloat(amount, 'f', 2, 64),
					strconv.FormatFloat(rate, 'f', 6, 64),
					strconv.Itoa(n), strconv.Itoa(perYr),
					"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64),
				}
				base := gzLoanInput(amount, rate, n, perYr, Settings{
					Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360})
				base.Loan.PayAmtStatus = types.InOutInput
				base.Loan.PayAmt = pay

				run := func(extra ...string) (string, bool) {
					cmd := exec.Command(bin, append(append([]string{}, args...), extra...)...)
					cmd.Env = env
					out, err := cmd.Output()
					if err != nil {
						return "", false
					}
					return string(out), true
				}
				repro := func(mode string) string {
					return "amort_oracle " + joinArgs(args) + " " + mode
				}

				// ---------------- norate ----------------
				if out, ok := run("norate"); ok {
					if bits, okb := parseRawBits(out)["solvedrate"]; !okb {
						rateStat.dosNoSolve++
					} else {
						in := base
						in.Loan.LoanRateStatus = types.StatusEmpty
						solved, serr := SolveBlankCells(in, false, true)
						if serr != nil {
							rateStat.nonConv++ // the product's own §58 gate
						} else {
							dosRate := math.Float64frombits(bits)
							rateStat.note(t, solved.Loan.LoanRate, dosRate, repro("norate"))
							if gp, _, okg := goSolve(amount, solved.Loan.LoanRate, n, perYr); okg {
								if dp, _, okd := goSolve(amount, dosRate, n, perYr); okd {
									band := 0.005
									if r := 2e-8 * math.Abs(amount); r > band {
										band = r
									}
									rateStat.noteBand(gp, dp, pay, band, repro("norate"))
								}
							}
						}
					}
				}

				// ---------------- noamt ----------------
				if out, ok := run("noamt"); ok {
					if bits, okb := parseRawBits(out)["solvedamount"]; !okb {
						amtStat.dosNoSolve++
					} else {
						in := base
						in.Loan.AmountStatus = types.StatusEmpty
						solved, serr := SolveBlankCells(in, true, false)
						if serr != nil {
							amtStat.nonConv++
						} else {
							amtStat.note(t, solved.Loan.Amount,
								math.Float64frombits(bits), repro("noamt"))
						}
					}
				}

				// ---------------- noterm: BOTH n and last date blank ----------------
				// LastOK is left false rather than forced, for the same reason
				// fuzzer5 leaves it false: the oracle leaves h^.lastok false too
				// and both sides' FirstPass equivalents derive it. Forcing it
				// would hide a divergence in that derivation.
				if out, ok := run("noterm", "bdump"); ok {
					in := base
					in.Loan.NStatus, in.Loan.NPeriods = types.StatusEmpty, 0
					in.Loan.LastStatus, in.Loan.LastOK = types.StatusEmpty, false
					dn, dy, dm, dd, okp := parseSolvedTermLine(out)
					gr := Amortize(in)
					goAnswered := gr.Err == nil && len(gr.Schedule) > 0
					if !okp {
						// THE PAIRED REFUSAL. Never dropped.
						termStat.dosNoSolve++
						reason := oracleErrLine(out)
						termStat.refusalReasons[reason]++
						if goAnswered {
							termStat.dosRefusedGoAnswered++
							if termStat.goOnlyExample == "" {
								termStat.goOnlyExample = fmt.Sprintf(
									"port answered n=%d last=%s where DOS said %q | %s",
									gr.NPeriods, gr.LastDate.Time.Format("2006-01-02"),
									reason, repro("noterm"))
							}
						} else {
							termStat.dosRefusedGoRefused++
						}
					} else if !goAnswered {
						termStat.goNoSolve++
					} else {
						termStat.compared++
						termStat.noteDOSYear(dy)
						gy, gm, gd := gr.LastDate.Time.Year(),
							int(gr.LastDate.Time.Month()), gr.LastDate.Time.Day()
						if gr.NPeriods != dn {
							termStat.nDiff++
							if termStat.worst == "" {
								termStat.worst = fmt.Sprintf("nperiods DOS %d Go %d | %s",
									dn, gr.NPeriods, repro("noterm"))
							}
						}
						if gy != dy || gm != dm || gd != dd {
							termStat.dateDiff++
							if termStat.worst == "" {
								termStat.worst = fmt.Sprintf(
									"last date DOS %d-%d-%d Go %d-%d-%d | %s",
									dy, dm, dd, gy, gm, gd, repro("noterm"))
							}
						}
					}
				}

				// ---------------- non: n blank, last date TYPED ----------------
				// The last date is placed ON the schedule's own grid through
				// fz5AddMonths, which carries and clamps the day exactly as
				// repeated AddPeriod calls would and truncates the year to DOS's
				// byte. An off-grid date would exercise FirstPass's SNAP instead
				// of its term derivation and make a first divergence ambiguous.
				lastDate := fz5AddMonths(base.Loan.FirstDate, (n-1)*(12/perYr))
				lastTok := fmt.Sprintf("lastdmy=%d.%d.%d", lastDate.Time.Day(),
					int(lastDate.Time.Month()), lastDate.Time.Year())
				if out, ok := run("non", lastTok, "bdump"); ok {
					in := base
					in.Loan.NStatus, in.Loan.NPeriods = types.StatusEmpty, 0
					in.Loan.LastStatus, in.Loan.LastDate = types.InOutInput, lastDate
					dn, dy, dm, dd, okp := parseBdumpLastLine(out)
					gr := Amortize(in)
					goAnswered := gr.Err == nil && len(gr.Schedule) > 0
					reproNon := repro("non " + lastTok)
					if !okp || dn == 0 {
						nStat.dosNoSolve++
						reason := oracleErrLine(out)
						nStat.refusalReasons[reason]++
						if goAnswered {
							nStat.dosRefusedGoAnswered++
							if nStat.goOnlyExample == "" {
								nStat.goOnlyExample = fmt.Sprintf(
									"port answered n=%d last=%s where DOS said %q | %s",
									gr.NPeriods, gr.LastDate.Time.Format("2006-01-02"),
									reason, reproNon)
							}
						} else {
							nStat.dosRefusedGoRefused++
						}
					} else if !goAnswered {
						nStat.goNoSolve++
					} else {
						nStat.compared++
						nStat.noteDOSYear(dy)
						gy, gm, gd := gr.LastDate.Time.Year(),
							int(gr.LastDate.Time.Month()), gr.LastDate.Time.Day()
						if gr.NPeriods != dn {
							nStat.nDiff++
							if nStat.worst == "" {
								nStat.worst = fmt.Sprintf("nperiods DOS %d Go %d | %s",
									dn, gr.NPeriods, reproNon)
							}
						}
						if gy != dy || gm != dm || gd != dd {
							nStat.dateDiff++
							if nStat.worst == "" {
								nStat.worst = fmt.Sprintf(
									"last date DOS %d-%d-%d Go %d-%d-%d | %s",
									dy, dm, dd, gy, gm, gd, reproNon)
							}
						}
					}
				}
			}

			// ---- the two value cells: R11 tail + R14 acceptance band ----
			for _, s := range []*backwardBitStat{rateStat, amtStat} {
				if s.checked == 0 {
					t.Errorf("%s: NOTHING was compared in stratum %s (DOS no-solve %d, "+
						"product non-converged %d). A differential that compares nothing "+
						"reports green — R5.", s.name, st.name, s.dosNoSolve, s.nonConv)
					continue
				}
				t.Logf("%s: compared %d (DOS no-solve %d, non-converged %d) | exact %d, "+
					"above %d, below %d | max %d ULP", s.name, s.checked, s.dosNoSolve,
					s.nonConv, s.exact, s.above, s.below, s.maxAbs)
				const maxULP = 1 << 20
				if s.maxAbs > maxULP {
					t.Errorf("%s [%s]: worst case is %d ULP (limit %d) — DIFFERENT ROOTS, "+
						"not different rounding.\n  %s", s.name, st.name, s.maxAbs, maxULP, s.worst)
				}
				if s.bandChecked > 0 {
					t.Logf("   acceptance band: worst payment gap %.3g of DOS's own Iterate "+
						"tolerance over %d cases", s.maxBandRatio, s.bandChecked)
					if s.maxBandRatio >= 1 {
						t.Errorf("%s [%s]: solved values are DISTINGUISHABLE by DOS's own "+
							"convergence test — repriced payment gap %.3g of "+
							"max(halfpenny, 2e-8 x amount).\n  %s",
							s.name, st.name, s.maxBandRatio, s.worstBand)
					}
					if nb := s.goNearer + s.dosNearer; nb >= 20 && s.dosNearer > s.goNearer {
						if pn := binomTwoTailed(s.dosNearer, nb); pn < 0.01 {
							t.Errorf("%s [%s]: DOS's early-stopped value reprices CLOSER than "+
								"the port's in %d of %d cases (p=%.2g).",
								s.name, st.name, s.dosNearer, nb, pn)
						}
					}
				}
			}

			// ---- the two term cells: exact, plus the paired refusal ----
			for _, s := range []*termCellStat{termStat, nStat} {
				if s.compared == 0 {
					t.Errorf("%s [%s]: NOTHING was compared (DOS no-solve %d, port no-solve "+
						"%d). R5.", s.name, st.name, s.dosNoSolve, s.goNoSolve)
					continue
				}
				t.Logf("%s: compared %d (DOS no-solve %d, port no-solve %d) | nperiods "+
					"differences %d, last-date differences %d | DOS last years %d-%d",
					s.name, s.compared, s.dosNoSolve, s.goNoSolve, s.nDiff, s.dateDiff,
					s.dosMinY, s.dosMaxY)
				reasons := make([]string, 0, len(s.refusalReasons))
				for k := range s.refusalReasons {
					reasons = append(reasons, k)
				}
				sort.Strings(reasons)
				for _, r := range reasons {
					t.Logf("   DOS refusal x%d: %s", s.refusalReasons[r], r)
				}
				if s.dosNoSolve > 0 {
					t.Logf("   refusal pairing: port also refused %d, port ANSWERED %d",
						s.dosRefusedGoRefused, s.dosRefusedGoAnswered)
				}
				if s.nDiff > 0 || s.dateDiff > 0 {
					t.Errorf("%s [%s]: %d nperiods and %d last-date differences against DOS. "+
						"These are integers — there is no rounding to forgive.\n  %s",
						s.name, st.name, s.nDiff, s.dateDiff, s.worst)
				}
				// THE ASYMMETRY THAT MATTERS. A port that answers a screen DOS
				// refuses shows the user a schedule that does not exist.
				if s.dosRefusedGoAnswered > 0 {
					t.Errorf("%s [%s]: REFUSAL ASYMMETRY — the port answered %d of %d screens "+
						"DOS declined. A schedule DOS will not draw is the worst direction "+
						"of divergence.\n  %s", s.name, st.name, s.dosRefusedGoAnswered,
						s.dosNoSolve, s.goOnlyExample)
				}
			}

			// ---- coverage is proven, not labelled (round 17's lesson) ----
			if st.wantMaxYearAtLeast > 0 {
				reach := termStat.dosMaxY
				if nStat.dosMaxY > reach {
					reach = nStat.dosMaxY
				}
				if reach < st.wantMaxYearAtLeast {
					t.Errorf("stratum %s never reached its own label: DOS's furthest reported "+
						"last date is %d, and the stratum claims to sample past %d. A "+
						"stratification label is a coverage claim; this one would have been "+
						"false while the test passed.", st.name, reach, st.wantMaxYearAtLeast)
				}
			}
		})
	}
}
