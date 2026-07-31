// Command pvprobe drives the Go present-value engine from the command line, so a
// case can be compared against the real DOS oracle (/tmp/oraclebuild/pv_oracle)
// without writing a throwaway Go test each time.
//
// It is the PV counterpart of cmd/mtgprobe. Verifying the Go side of a PV finding
// was the single slowest step of the 2026-07-30 audit purely because no such tool
// existed.
//
// Usage mirrors pv_oracle's `table` mode:
//
//	pvprobe table RATE BASIS asof=D.M.Y [colamonth=ann|cnt|1..12]
//	        [per=Df.Mf.Yf:Dt.Mt.Yt:PERYR:AMT:COLA]... [lump=D.M.Y:AMT]...
//
// BASIS is 360 | 365 | 365360. Example — the Julian-ceiling case:
//
//	pvprobe table 0.10 360 asof=1.1.2028 per=1.6.2035:1.12.2149:26:1000.00:0.03
//	  -> DOS (pv_oracle) answers 170805.731733
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	pv "github.com/persense/persense-port/internal/finance/presentvalue"
	"github.com/persense/persense-port/internal/types"
)

func fatalf(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "pvprobe: "+f+"\n", a...)
	os.Exit(2)
}

func parseDMY(s string) types.DateRec {
	f := strings.Split(s, ".")
	if len(f) != 3 {
		fatalf("bad D.M.Y %q", s)
	}
	d, _ := strconv.Atoi(f[0])
	m, _ := strconv.Atoi(f[1])
	y, _ := strconv.Atoi(f[2])
	return types.NewDateRec(y, time.Month(m), d)
}

func fv(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }

func main() {
	if len(os.Args) < 4 {
		fatalf("usage: pvprobe table RATE BASIS asof=D.M.Y [per=...] [lump=...]")
	}
	if os.Args[1] != "table" {
		fatalf("only `table` mode is implemented; got %q", os.Args[1])
	}
	rate := fv(os.Args[2])

	basis := types.Basis360
	switch os.Args[3] {
	case "360":
		basis = types.Basis360
	case "365":
		basis = types.Basis365
	case "365360":
		basis = types.Basis365360
	default:
		fatalf("basis must be 360|365|365360, got %q", os.Args[3])
	}

	var asOf types.DateRec
	var lumps []pv.LumpSumPayment
	var pers []pv.PeriodicPayment
	// COLA escalation month, DOS df.c.colamonth: ANN (anniversary of the from
	// date, the default), CNT (continuous), or a calendar month 1-12. It selects
	// between three genuinely different summation shapes, so a probe that cannot
	// set it cannot reach the month-specific path at all.
	colaMonth := types.COLAAnnual

	for _, a := range os.Args[4:] {
		switch {
		case strings.HasPrefix(a, "colamonth="):
			v := strings.TrimPrefix(a, "colamonth=")
			switch v {
			case "ann":
				colaMonth = types.COLAAnnual
			case "cnt":
				colaMonth = types.COLAContinuous
			default:
				m, err := strconv.Atoi(v)
				if err != nil || m < 1 || m > 12 {
					fatalf("colamonth must be ann|cnt|1..12, got %q", v)
				}
				colaMonth = byte(m)
			}
		case strings.HasPrefix(a, "asof="):
			asOf = parseDMY(strings.TrimPrefix(a, "asof="))
		case strings.HasPrefix(a, "lump="):
			f := strings.Split(strings.TrimPrefix(a, "lump="), ":")
			if len(f) != 2 {
				fatalf("bad lump= %q (want D.M.Y:AMT)", a)
			}
			lumps = append(lumps, pv.LumpSumPayment{
				DateStatus: types.InOutInput, Date: parseDMY(f[0]),
				AmtStatus: types.InOutInput, Amt: fv(f[1]),
			})
		case strings.HasPrefix(a, "per="):
			f := strings.Split(strings.TrimPrefix(a, "per="), ":")
			if len(f) != 5 {
				fatalf("bad per= %q (want FROM:TO:PERYR:AMT:COLA)", a)
			}
			py, _ := strconv.Atoi(f[2])
			p := pv.PeriodicPayment{
				FromDateStatus: types.InOutInput, FromDate: parseDMY(f[0]),
				ToDateStatus: types.InOutInput, ToDate: parseDMY(f[1]),
				PerYrStatus: types.InOutInput, PerYr: py,
				AmtStatus: types.InOutInput, Amt: fv(f[3]),
			}
			if c := fv(f[4]); c != 0 {
				p.COLAStatus, p.COLA = types.InOutInput, c
			}
			pers = append(pers, p)
		default:
			fatalf("unrecognised token %q", a)
		}
	}

	// Mirrors tblInput in dos_pv_table_test.go, which mirrors DOS SetYrDays
	// (INTSUTIL.pas:333): 365.25 for x365, 360 otherwise — including x365_360,
	// which is an actual-day count over a 360 denominator.
	s := pv.PVSettings{Basis: basis, PerYr: 12, COLAMonth: colaMonth}
	if basis == types.Basis365 {
		s.YrDays, s.YrInv = 365.25, 1/365.25
	} else {
		s.YrDays, s.YrInv = 360, 1.0/360
	}

	res := pv.Calculate(pv.PVInput{
		LumpSums:  lumps,
		Periodics: pers,
		PresVal: pv.PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: pv.RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
		},
		Settings: s,
	})
	if res.Err != nil {
		fmt.Printf("ERR %v\n", res.Err)
		return
	}
	fmt.Printf("pv %.6f\n", res.SumValue)
	for i, p := range res.Periodics {
		fmt.Printf("  per[%d] val %.6f ninst %d\n", i, p.Val, p.NInstallments)
	}
	for i, l := range res.LumpSums {
		fmt.Printf("  lump[%d] val %.6f\n", i, l.Val)
	}
}
