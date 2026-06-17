package api
import ("fmt";"testing")
func TestZZBalloonProbe(t *testing.T){
 scen:=map[string]string{
  "A pmt-blank balloon-date-only":`{"amount":100000,"loanDate":"2026-06-13","firstDate":"2026-08-01","rate":0.12,"perYr":12,"nPeriods":120,"balloons":[{"date":"2030-01-01"}]}`,
  "B pmt-1434.71 balloon-date-only":`{"amount":100000,"loanDate":"2026-06-13","firstDate":"2026-08-01","rate":0.12,"perYr":12,"nPeriods":120,"payment":1434.71,"balloons":[{"date":"2030-01-01"}]}`,
 }
 for name,body:=range scen{
  r,code:=amortCall(t,body)
  fmt.Printf("\n=== %s (code %d) ===\n err=%q warnings=%v nRows=%d totalPaid=%.2f\n",name,code,r.Error,r.Warnings,len(r.Schedule),r.TotalPaid)
  var firstReg float64
  for _,x:=range r.Schedule{ if x.PayNum>=1 {firstReg=x.Payment;break} }
  fmt.Printf(" firstRegPmt=%.2f\n",firstReg)
  for _,x:=range r.Schedule{ if x.Date=="2030-01-01"{ fmt.Printf(" balloon row 2030-01-01: payNum=%d payment=%.2f interest=%.2f balance=%.2f\n",x.PayNum,x.Payment,x.Interest,x.Principal) } }
  if len(r.Schedule)>0{ last:=r.Schedule[len(r.Schedule)-1]; fmt.Printf(" last: payNum=%d date=%s payment=%.2f balance=%.2f\n",last.PayNum,last.Date,last.Payment,last.Principal) }
 }
}
