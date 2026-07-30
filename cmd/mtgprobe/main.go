package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/persense/persense-port/internal/finance/mortgage"
	"github.com/persense/persense-port/internal/types"
)

func fv(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
func iv(s string) int     { v, _ := strconv.Atoi(s); return v }

// args: mode then key=value pairs.
// keys: price,pct,cash,financed,monthly,years,rate,points,tax,when,howmuch
// value "-" means blank/empty (status empty). otherwise input.
func main() {
	mode := os.Args[1]
	m := mortgage.MtgLine{}
	set := map[string]string{}
	for _, a := range os.Args[2:] {
		for i := 0; i < len(a); i++ {
			if a[i] == '=' {
				set[a[:i]] = a[i+1:]
				break
			}
		}
	}
	put := func(k string, st *int8, f func(string)) {
		if v, ok := set[k]; ok && v != "-" {
			*st = types.InOutInput
			f(v)
		}
	}
	put("price", &m.PriceStatus, func(s string) { m.Price = fv(s) })
	put("pct", &m.PctStatus, func(s string) { m.Pct = fv(s) })
	put("cash", &m.CashStatus, func(s string) { m.Cash = fv(s) })
	put("financed", &m.FinancedStatus, func(s string) { m.Financed = fv(s) })
	put("monthly", &m.MonthlyStatus, func(s string) { m.Monthly = fv(s) })
	put("years", &m.YearsStatus, func(s string) { m.Years = iv(s) })
	put("rate", &m.RateStatus, func(s string) { m.Rate = fv(s) })
	put("points", &m.PointsStatus, func(s string) { m.Points = fv(s) })
	put("tax", &m.TaxStatus, func(s string) { m.Tax = fv(s) })
	put("when", &m.WhenStatus, func(s string) { m.When = iv(s) })
	put("howmuch", &m.HowMuchStatus, func(s string) { m.HowMuch = fv(s) })

	switch mode {
	case "calc":
		r := mortgage.Calc(m)
		if r.Err != nil {
			fmt.Printf("ERR %s\n", r.Err.Error())
			return
		}
		l := r.Line
		fmt.Printf("monthly %.6f mstat %d price %.6f pstat %d cash %.6f cstat %d financed %.6f fstat %d howmuch %.6f hstat %d\n",
			l.Monthly, l.MonthlyStatus, l.Price, l.PriceStatus, l.Cash, l.CashStatus, l.Financed, l.FinancedStatus, l.HowMuch, l.HowMuchStatus)
		for _, w := range r.Warnings {
			fmt.Printf("WARN %s\n", w)
		}
	case "apr":
		r := mortgage.Calc(m)
		if r.Err != nil {
			fmt.Printf("ERR calc %s\n", r.Err.Error())
			return
		}
		a, conv, err := mortgage.FullTermAPR(r.Line, 360)
		if err != nil {
			fmt.Printf("ERR apr %v\n", err)
			return
		}
		fmt.Printf("apr %.10f conv %v monthly %.6f financed %.6f\n", a, conv, r.Line.Monthly, r.Line.Financed)
	case "aprraw":
		// no Calc: mirror aprfin/taxapr (which DO run Calc but it refuses harmlessly)
		r := mortgage.Calc(m)
		l := m
		if r.Err == nil {
			l = r.Line
		} else {
			// DOS: Calc "refuses" via MessageBox but the record keeps its inputs;
			// FirstPass still set balloonstatus.
			if m.WhenStatus == types.InOutInput {
				if m.HowMuchStatus == types.InOutInput {
					l.BalloonStat = types.BalloonKnown
				} else {
					l.BalloonStat = types.BalloonUnk
				}
			}
		}
		if !mortgage.EnoughDataForAPR(&l) {
			fmt.Printf("ERR insufficient\n")
			return
		}
		a, conv, err := mortgage.FullTermAPR(l, 360)
		if err != nil {
			fmt.Printf("ERR apr %v\n", err)
			return
		}
		fmt.Printf("apr %.10f conv %v monthly %.6f financed %.6f\n", a, conv, l.Monthly, l.Financed)
	}
}
