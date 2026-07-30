package amortization

// zzm5term_test.go — INVESTIGATION HARNESS for solved-TERM divergences (the
// `lastdate` / `nperiods` pair). Scratch, like zzm5_test.go: not a gate.
//
// fuzzer5 reports a term divergence as one line, but the interesting question is
// almost always "which single option token has to be present for the dates to
// part company". So this harness takes the M5 line and an ABLATION/PROBE list
// and prints DOS vs Go for each variant, marking each SAME or DIFF.
//
//	M5="371448.27 0.0997880000 204 12 b365_360 ... non lastdmy=28.9.2042" \
//	M5PROBE="<none>|b365_360|prepaid|adj=161:0.0863990000:" \
//	  go test ./internal/finance/amortization/ -run TestM5Term -v
//
// M5PROBE is a `|`-separated list of variants. Each entry is a space-separated
// set of tokens applied to the BASE line (the M5 line stripped of every option
// token, keeping only AMOUNT RATE N PERYR). `<none>` means the bare base line.
// With M5PROBE unset the full M5 line is run once, unmodified.

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// m5TermOracle asks the oracle for DOS's post-MakeTable last-date / period-count
// cells, which the `bdump` block emits as `lastdate M/D/YYYY nperiods N`
// (amort_oracle.pas:1013). Retried for the oracle's ~5% flake rate.
func m5TermOracle(args []string) (last string, n int, errMsg string) {
	clean := make([]string, 0, len(args)+1)
	for _, a := range args {
		if a == "rows" || a == "quiet" || a == "apr" || a == "bdump" {
			continue
		}
		clean = append(clean, a)
	}
	clean = append(clean, "bdump")
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, clean...).Output()
		if err != nil || len(out) == 0 {
			continue
		}
		text := strings.TrimSpace(string(out))
		if strings.HasPrefix(text, "ERR") {
			return "", 0, strings.SplitN(text, "\n", 2)[0]
		}
		for _, ln := range strings.Split(text, "\n") {
			f := strings.Fields(ln)
			if len(f) >= 4 && f[0] == "lastdate" && f[2] == "nperiods" {
				n, _ = strconv.Atoi(f[3])
				return f[1], n, ""
			}
		}
	}
	return "", 0, "oracle produced no lastdate line"
}

func TestM5Term(t *testing.T) {
	gateOracle(t)

	line := os.Getenv("M5")
	if line == "" {
		t.Skip("set M5 to the oracle argument line")
	}
	fields := strings.Fields(line)
	if len(fields) < 4 {
		t.Fatalf("M5 needs at least AMOUNT RATE N PERYR")
	}
	// M5BASE holds tokens present in EVERY probe variant — typically the minimal
	// set that triggers the divergence (the loan/first dates and the adjustment),
	// so each variant isolates one further option token.
	base := strings.Join(fields[:4], " ")
	if b := strings.TrimSpace(os.Getenv("M5BASE")); b != "" {
		base += " " + b
	}

	variants := []string{line}
	labels := []string{"<full M5 line>"}
	if pr := os.Getenv("M5PROBE"); pr != "" {
		variants, labels = nil, nil
		for _, v := range strings.Split(pr, "|") {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			labels = append(labels, v)
			if v == "<none>" {
				variants = append(variants, base)
			} else {
				variants = append(variants, base+" "+v)
			}
		}
	}

	same, diff := 0, 0
	for i, v := range variants {
		in, args := m5Parse(t, v)
		dosLast, dosN, errMsg := m5TermOracle(args)
		if errMsg != "" {
			fmt.Printf("%-40s => DOS refused: %s\n", labels[i], errMsg)
			continue
		}
		gr := Amortize(in)
		if gr.Err != nil {
			fmt.Printf("%-40s => DOS %s n=%d | Go ERROR %v\n", labels[i], dosLast, dosN, gr.Err)
			diff++
			continue
		}
		goLast := fmt.Sprintf("%d/%d/%d", int(gr.LastDate.Time.Month()),
			gr.LastDate.Time.Day(), gr.LastDate.Time.Year())
		verdict := "SAME"
		if goLast != dosLast || gr.NPeriods != dosN {
			verdict = "**DIFF**"
			diff++
		} else {
			same++
		}
		fmt.Printf("%-40s => DOS %-11s n=%-4d | Go %-11s n=%-4d %s\n",
			labels[i], dosLast, dosN, goLast, gr.NPeriods, verdict)
	}
	fmt.Printf("probe summary: %d SAME, %d DIFF\n", same, diff)
}
