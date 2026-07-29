program mtg_oracle;
{ Headless source-oracle for the MORTGAGE engine. Drives the REAL DOS
  Mortgage.Calc against the headless Globals/HelpSystemUnit stubs, for
  differential testing vs the Go mortgage package.

  The mortgage line stores `rate` as the TRUE (continuously compounded) rate —
  the GUI converts the user's entered APR before Calc sees it. To avoid the
  conversion as a confound, this driver takes the true rate directly and the Go
  comparison feeds the same true rate.

  Usage:
      mtg_oracle monthly PRICE PCT YEARS TRUERATE [POINTS] [BWHEN BHOWMUCH]
        Solve the MONTHLY payment. PCT is the fraction down (0.20 = 20% down).
        POINTS optional (fraction, default 0). A balloon is added with
        BWHEN years / BHOWMUCH dollars when both are given.
      mtg_oracle price PCT YEARS TRUERATE MONTHLY [POINTS]
        Solve the PRICE from a known monthly payment.

  Prints one machine-readable line:
      monthly <m> price <p> cash <c> financed <f>      (or:  ERR <message>) }

uses
  SysUtils, Classes,
  Globals, peTypes, peData, INTSUTIL, MORTGAGE;

var
  e1, e2, e3, e4, e5, e6: real;
  iarg: integer;
  mode: string;
  crStr1, crStr2, crFinal: string;
  cmpApr1, cmpApr2: real;

procedure AllocMtg;
var i: integer;
begin
  for i := 1 to maxlines do begin New(e[i]); ZeroMortgage(e[i]); end;
  nlines[MTGBlock]    := 1;
  scrollpos[MTGBlock] := 0;
  df.c.basis      := x360;
  { 50 = shipped DOS default (PEDATA.pas:67). Was 20, which put
    RepayFancyLoan's no-valid-very_last horizon (AMORTOP.pas:1143-1147,
    `stopdate.y := 100 + pred(df.c.centurydiv)`) at year 2019 and made every
    fancy term solve refuse. See amort_oracle.pas for the full finding. }
  df.c.centurydiv := 50;
  SetYrDays;
end;

procedure Report;
begin
  if errorflag or OracleErrorFired then
  begin
    if OracleErrorFired then Writeln('ERR ', OracleLastError)
    else Writeln('ERR errorflag');
    Halt(0);
  end;
  Writeln('monthly ', e[1]^.monthly:0:6,
          ' price ',   e[1]^.price:0:6,
          ' cash ',    e[1]^.cash:0:6,
          ' financed ', e[1]^.financed:0:6);
end;

{ Parse the first float that appears after `lbl` in s. }
function FloatAfter(const s, lbl: string): real;
var p, q: integer; t: string; e: integer; v: double;
begin
  FloatAfter := -1; p := Pos(lbl, s); if p = 0 then exit;
  q := p + Length(lbl);
  while (q <= Length(s)) and not (s[q] in ['0'..'9', '-', '.']) do inc(q);
  t := '';
  while (q <= Length(s)) and (s[q] in ['0'..'9', '.', '-']) do begin t := t + s[q]; inc(q); end;
  Val(t, v, e); if e = 0 then FloatAfter := v;
end;

{ Compute and print the full-term APR via the real ReportAPR. ReportAPR routes
  its result string through MessageBox, which the headless Globals stub captures
  into OracleLastError — so we read the APR back from there. Prints a fraction
  (DOS reports 100*apr). }
procedure ReportAprResult;
var aprPct: real;
begin
  OracleErrorFired := false; OracleLastError := '';
  ReportAPR(e[1]^);
  if Pos('did not converge', OracleLastError) > 0 then
  begin Writeln('ERR apr did not converge'); Halt(0); end;
  aprPct := FloatAfter(OracleLastError, 'term');
  if aprPct < 0 then begin Writeln('ERR apr parse: ', OracleLastError); Halt(0); end;
  Writeln('apr ', (aprPct / 100):0:10,
          ' monthly ', e[1]^.monthly:0:6, ' financed ', e[1]^.financed:0:6);
end;

begin
  if ParamCount >= 1 then mode := ParamStr(1) else mode := 'monthly';
  AllocMtg;

  { eval Pr Pc Ca Fi Mo Ye Ra : run the REAL Mortgage FirstPass+Calc dispatch
    over a field-presence pattern (each arg '1' present or '0' blank) for
    Price / Pct / Cash / Financed / Monthly / Years / Rate, with Points=0 and no
    balloon. Present fields take a self-consistent tuple (200000, 20% down, cash
    40000, financed 160000, 30yr, 7% true rate, monthly 1066.683053). Reports the
    solve consequence: which of monthly/price/cash/financed became an OUTPUT
    (status=outp=1) and their values, or ERR on a refusal/over-determined screen.
    The Go mortgage.Calc must agree. }
  if mode = 'eval' then
  begin
    with e[1]^ do
    begin
      if ParamStr(2) = '1' then begin pricestatus := inp; price := 200000; end else begin pricestatus := empty; price := 0; end;
      if ParamStr(3) = '1' then begin pctstatus := inp; pct := 0.20; end else begin pctstatus := empty; pct := 0; end;
      if ParamStr(4) = '1' then begin cashstatus := inp; cash := 40000; end else begin cashstatus := empty; cash := 0; end;
      if ParamStr(5) = '1' then begin financedstatus := inp; financed := 160000; end else begin financedstatus := empty; financed := 0; end;
      if ParamStr(6) = '1' then begin monthlystatus := inp; monthly := 1066.683053; end else begin monthlystatus := empty; monthly := 0; end;
      if ParamStr(7) = '1' then begin yearsstatus := inp; years := 30; end else begin yearsstatus := empty; years := 0; end;
      if ParamStr(8) = '1' then begin ratestatus := inp; rate := 0.07; end else begin ratestatus := empty; rate := 0; end;
      { Optional extra axes (backward-compatible: absent args parse as blank):
        ParamStr(9)  balloon WHEN present (year 7)
        ParamStr(10) balloon HOWMUCH present (50000)
        ParamStr(11) points VALUE (default 0) }
      if ParamStr(9) = '1' then begin whenstatus := inp; when := 7; end else begin whenstatus := empty; when := 0; end;
      if ParamStr(10) = '1' then begin howmuchstatus := inp; howmuch := 50000; end else begin howmuchstatus := empty; howmuch := 0; end;
      pointsstatus := inp;
      if ParamStr(11) <> '' then Val(ParamStr(11), points, iarg) else points := 0;
      taxstatus := inp; tax := 0;
    end;
    OracleErrorFired := false; OracleLastError := '';
    CalculateRows(1, 1);
    if errorflag or OracleErrorFired then
      Writeln('ERR ', OracleLastError)
    else
      Writeln('ok monthly ', e[1]^.monthly:0:4, ' mstat ', e[1]^.monthlystatus,
              ' price ', e[1]^.price:0:4, ' pstat ', e[1]^.pricestatus,
              ' cash ', e[1]^.cash:0:4, ' cstat ', e[1]^.cashstatus,
              ' financed ', e[1]^.financed:0:4, ' fstat ', e[1]^.financedstatus,
              ' howmuch ', e[1]^.howmuch:0:4, ' hstat ', e[1]^.howmuchstatus);
    Halt(0);
  end;

  { compare PRICE1 PCT1 YEARS1 RATE1 POINTS1 PRICE2 PCT2 YEARS2 RATE2 POINTS2
    -> drive the real ReportComparisonOfAPRs (its screen code is commented out,
    so it runs headlessly). It internally FirstPass+Calc's both rows, computes
    each full-term APR, and either declares one always-better or finds the
    crossover. The crossover APR is in the FinalResult string ("cross at X");
    the crossover TIME (years) is left in the PEDATA global `apr_crossover`. }
  if mode = 'compare' then
  begin
    nlines[MTGBlock] := 2;
    Val(ParamStr(2), e1, iarg); Val(ParamStr(3), e2, iarg);
    e3 := StrToIntDef(ParamStr(4), 30); Val(ParamStr(5), e4, iarg); Val(ParamStr(6), e5, iarg);
    with e[1]^ do
    begin
      pricestatus := inp; price := e1; pctstatus := inp; pct := e2;
      yearsstatus := inp; years := Round(e3); ratestatus := inp; rate := e4;
      pointsstatus := inp; points := e5; taxstatus := inp; tax := 0; monthlystatus := empty;
    end;
    Val(ParamStr(7), e1, iarg); Val(ParamStr(8), e2, iarg);
    e3 := StrToIntDef(ParamStr(9), 30); Val(ParamStr(10), e4, iarg); Val(ParamStr(11), e5, iarg);
    with e[2]^ do
    begin
      pricestatus := inp; price := e1; pctstatus := inp; pct := e2;
      yearsstatus := inp; years := Round(e3); ratestatus := inp; rate := e4;
      pointsstatus := inp; points := e5; taxstatus := inp; tax := 0; monthlystatus := empty;
    end;
    { Optional balloon axes (backward-compatible: absent args parse as blank):
      ParamStr(12) balloon WHEN (years) for mortgage 1
      ParamStr(13) balloon HOWMUCH for mortgage 1
      ParamStr(14) balloon WHEN (years) for mortgage 2
      ParamStr(15) balloon HOWMUCH for mortgage 2
      Both when+howmuch set => balloon_known (FirstPass derives status). This
      drives the crossover TryBalloonDates fallback (Mortgage.pas:462-508). }
    if (ParamStr(12) <> '') and (ParamStr(13) <> '') then
      with e[1]^ do
      begin
        e6 := StrToIntDef(ParamStr(12), 0); Val(ParamStr(13), e4, iarg);
        whenstatus := inp; when := Round(e6); howmuchstatus := inp; howmuch := e4;
      end;
    if (ParamStr(14) <> '') and (ParamStr(15) <> '') then
      with e[2]^ do
      begin
        e6 := StrToIntDef(ParamStr(14), 0); Val(ParamStr(15), e4, iarg);
        whenstatus := inp; when := Round(e6); howmuchstatus := inp; howmuch := e4;
      end;
    crStr1 := ''; crStr2 := ''; crFinal := '';
    ReportComparisonOfAPRs(1, 2, crStr1, crStr2, crFinal);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    cmpApr1 := FloatAfter(crStr1, 'APR =');
    cmpApr2 := FloatAfter(crStr2, 'APR =');
    if Pos('cross at', crFinal) > 0 then
      Writeln('cross ', (FloatAfter(crFinal, 'cross at') / 100):0:10,
              ' time ', apr_crossover:0:10,
              ' apr1 ', (cmpApr1 / 100):0:10, ' apr2 ', (cmpApr2 / 100):0:10)
    else if Pos('always', crFinal) > 0 then
      Writeln('always apr1 ', (cmpApr1 / 100):0:10, ' apr2 ', (cmpApr2 / 100):0:10)
    else
      Writeln('ERR cmp ', crFinal);
    Halt(0);
  end;

  if mode = 'aprfin' then
  begin
    { aprfin FINANCED MONTHLY YEARS TRUERATE [POINTS] — APR on a row given as
      amount-financed + monthly with NO price funding. DOS's compare/APR path
      Calc's the row (which "refuses" to solve price and pops a MessageBox) then
      gates on EnoughDataForAPR and reports the APR regardless. Mirror that:
      set the fields, run Calc, IGNORE the refusal, gate on EnoughDataForAPR. }
    Val(ParamStr(2), e1, iarg);   { financed }
    Val(ParamStr(3), e2, iarg);   { monthly }
    e3 := StrToIntDef(ParamStr(4), 30);
    Val(ParamStr(5), e4, iarg);   { true rate }
    e5 := 0;
    if ParamCount >= 6 then Val(ParamStr(6), e5, iarg);
    with e[1]^ do
    begin
      financedstatus := inp; financed := e1;
      monthlystatus := inp;  monthly := e2;
      yearsstatus := inp;    years := Round(e3);
      ratestatus := inp;     rate := e4;
      pointsstatus := inp;   points := e5;
      taxstatus := inp;      tax := 0;
      pricestatus := empty;  pctstatus := empty; cashstatus := empty;
    end;
    CalculateRows(1, 1);   { FirstPass+Calc; Calc refuses price but that's fine }
    if not EnoughDataForAPR(e[1]^) then
    begin Writeln('ERR insufficient'); Halt(0); end;
    ReportAprResult;
    Halt(0);
  end;

  { --- tax-bearing modes: the other modes hardcode tax:=0, leaving the
    (monthly - tax) threading through the monthly/price solves and the APR
    cash flows unverified. These set a real Monthly Tax+Ins. --- }
  if mode = 'taxmonthly' then
  begin
    { taxmonthly PRICE PCT YEARS TRUERATE TAX [POINTS] — solve monthly with tax }
    Val(ParamStr(2), e1, iarg); Val(ParamStr(3), e2, iarg);
    e3 := StrToIntDef(ParamStr(4), 30); Val(ParamStr(5), e4, iarg); Val(ParamStr(6), e5, iarg);
    e6 := 0; if ParamCount >= 7 then Val(ParamStr(7), e6, iarg);
    with e[1]^ do
    begin
      pricestatus := inp; price := e1; pctstatus := inp; pct := e2;
      yearsstatus := inp; years := Round(e3); ratestatus := inp; rate := e4;
      taxstatus := inp; tax := e5; pointsstatus := inp; points := e6; monthlystatus := empty;
    end;
    CalculateRows(1, 1); Report; Halt(0);
  end;

  if mode = 'taxprice' then
  begin
    { taxprice PCT YEARS TRUERATE MONTHLY TAX [POINTS] — solve price from monthly with tax }
    Val(ParamStr(2), e1, iarg);
    e2 := StrToIntDef(ParamStr(3), 30); Val(ParamStr(4), e3, iarg); Val(ParamStr(5), e4, iarg);
    Val(ParamStr(6), e5, iarg);
    e6 := 0; if ParamCount >= 7 then Val(ParamStr(7), e6, iarg);
    with e[1]^ do
    begin
      pctstatus := inp; pct := e1; yearsstatus := inp; years := Round(e2);
      ratestatus := inp; rate := e3; monthlystatus := inp; monthly := e4;
      taxstatus := inp; tax := e5; pointsstatus := inp; points := e6; pricestatus := empty;
    end;
    CalculateRows(1, 1); Report; Halt(0);
  end;

  if mode = 'solvehowmuch' then
  begin
    { solvehowmuch PRICE PCT YEARS TRUERATE MONTHLY WHEN [POINTS] [TAX] — solve the
      unknown balloon AMOUNT (When given, HowMuch blank => balloon_unk =>
      BalloonCalc, Mortgage.pas:247-257) across the input space. }
    Val(ParamStr(2), e1, iarg); Val(ParamStr(3), e2, iarg);
    e3 := StrToIntDef(ParamStr(4), 30); Val(ParamStr(5), e4, iarg); Val(ParamStr(6), e5, iarg); { monthly }
    e6 := StrToIntDef(ParamStr(7), 0); { when }
    with e[1]^ do
    begin
      pricestatus := inp; price := e1; pctstatus := inp; pct := e2;
      yearsstatus := inp; years := Round(e3); ratestatus := inp; rate := e4;
      monthlystatus := inp; monthly := e5; whenstatus := inp; when := Round(e6);
      pointsstatus := inp; if ParamStr(8) <> '' then Val(ParamStr(8), points, iarg) else points := 0;
      taxstatus := inp; if ParamStr(9) <> '' then Val(ParamStr(9), tax, iarg) else tax := 0;
      howmuchstatus := empty;
    end;
    CalculateRows(1, 1);
    if errorflag or OracleErrorFired then Writeln('ERR ', OracleLastError)
    else Writeln('howmuch ', e[1]^.howmuch:0:6, ' hstat ', e[1]^.howmuchstatus);
    Halt(0);
  end;

  if mode = 'taxpricecash' then
  begin
    { taxpricecash CASH YEARS TRUERATE MONTHLY TAX [POINTS] — solve price from
      monthly with CASH REQUIRED given (exercises Calc's price := cash +
      (1-points)*(...) branch, Mortgage.pas:296, then ComputeCashPctAndFinanced
      deriving pct from cash) }
    Val(ParamStr(2), e1, iarg);
    e2 := StrToIntDef(ParamStr(3), 30); Val(ParamStr(4), e3, iarg); Val(ParamStr(5), e4, iarg);
    Val(ParamStr(6), e5, iarg);
    e6 := 0; if ParamCount >= 7 then Val(ParamStr(7), e6, iarg);
    with e[1]^ do
    begin
      cashstatus := inp; cash := e1; yearsstatus := inp; years := Round(e2);
      ratestatus := inp; rate := e3; monthlystatus := inp; monthly := e4;
      taxstatus := inp; tax := e5; pointsstatus := inp; points := e6; pricestatus := empty;
    end;
    CalculateRows(1, 1); Report; Halt(0);
  end;

  if mode = 'taxapr' then
  begin
    { taxapr FINANCED MONTHLY YEARS TRUERATE TAX [POINTS] — APR with tax set (mirror aprfin) }
    Val(ParamStr(2), e1, iarg); Val(ParamStr(3), e2, iarg);
    e3 := StrToIntDef(ParamStr(4), 30); Val(ParamStr(5), e4, iarg); Val(ParamStr(6), e5, iarg);
    e6 := 0; if ParamCount >= 7 then Val(ParamStr(7), e6, iarg);
    with e[1]^ do
    begin
      financedstatus := inp; financed := e1; monthlystatus := inp; monthly := e2;
      yearsstatus := inp; years := Round(e3); ratestatus := inp; rate := e4;
      taxstatus := inp; tax := e5; pointsstatus := inp; points := e6;
      pricestatus := empty; pctstatus := empty; cashstatus := empty;
    end;
    CalculateRows(1, 1);
    if not EnoughDataForAPR(e[1]^) then begin Writeln('ERR insufficient'); Halt(0); end;
    ReportAprResult; Halt(0);
  end;

  if mode = 'price' then
  begin
    { price PCT YEARS TRUERATE MONTHLY [POINTS] }
    Val(ParamStr(2), e1, iarg);   { pct }
    e2 := StrToIntDef(ParamStr(3), 30);  { years }
    Val(ParamStr(4), e3, iarg);   { true rate }
    Val(ParamStr(5), e4, iarg);   { monthly }
    e5 := 0;
    if ParamCount >= 6 then Val(ParamStr(6), e5, iarg); { points }
    with e[1]^ do
    begin
      pctstatus := inp;     pct := e1;
      yearsstatus := inp;   years := Round(e2);
      ratestatus := inp;    rate := e3;
      monthlystatus := inp; monthly := e4;
      pointsstatus := inp;  points := e5;
      taxstatus := inp;     tax := 0;
      pricestatus := empty;
      { optional balloon (ParamStr 7=when, 8=howmuch) exercises price-from-monthly
        with a known balloon: price := ((monthly-tax)*Summation + balloonval)/(1-pct) }
      if (ParamStr(7) <> '') and (ParamStr(8) <> '') then
      begin
        e6 := StrToIntDef(ParamStr(7), 0); Val(ParamStr(8), e4, iarg);
        whenstatus := inp; when := Round(e6); howmuchstatus := inp; howmuch := e4;
      end;
    end;
  end
  else if (mode = 'mcash') or (mode = 'mfin') then
  begin
    { mcash|mfin PRICE DOWNVALUE YEARS TRUERATE [POINTS] — solve monthly with
      the down payment given as cash required (mcash) or amount financed (mfin)
      instead of percent down, exercising the cash<->pct<->financed dispatch
      (Mortgage.pas ComputeCashPctAndFinanced). }
    Val(ParamStr(2), e1, iarg);   { price }
    Val(ParamStr(3), e2, iarg);   { down value }
    e3 := StrToIntDef(ParamStr(4), 30);
    Val(ParamStr(5), e4, iarg);   { true rate }
    e5 := 0;
    if ParamCount >= 6 then Val(ParamStr(6), e5, iarg);
    with e[1]^ do
    begin
      pricestatus := inp;   price := e1;
      if mode = 'mcash' then begin cashstatus := inp; cash := e2; end
      else begin financedstatus := inp; financed := e2; end;
      yearsstatus := inp;   years := Round(e3);
      ratestatus := inp;    rate := e4;
      pointsstatus := inp;  points := e5;
      taxstatus := inp;     tax := 0;
      monthlystatus := empty;
    end;
  end
  else
  begin
    { monthly|apr PRICE PCT YEARS TRUERATE [POINTS] [BWHEN BHOWMUCH] }
    Val(ParamStr(2), e1, iarg);   { price }
    Val(ParamStr(3), e2, iarg);   { pct }
    e3 := StrToIntDef(ParamStr(4), 30);  { years }
    Val(ParamStr(5), e4, iarg);   { true rate }
    e5 := 0;
    if ParamCount >= 6 then Val(ParamStr(6), e5, iarg); { points }
    with e[1]^ do
    begin
      pricestatus := inp;   price := e1;
      pctstatus := inp;     pct := e2;
      yearsstatus := inp;   years := Round(e3);
      ratestatus := inp;    rate := e4;
      pointsstatus := inp;  points := e5;
      taxstatus := inp;     tax := 0;
      monthlystatus := empty;
      if ParamCount >= 8 then
      begin
        e6 := StrToIntDef(ParamStr(7), 0);    { balloon when (years) }
        Val(ParamStr(8), e4, iarg);            { balloon howmuch }
        whenstatus := inp;    when := Round(e6);
        howmuchstatus := inp; howmuch := e4;
      end;
    end;
  end;

  CalculateRows(1, 1);   { runs FirstPass then Calc for the row }
  if mode = 'apr' then
  begin
    if errorflag or OracleErrorFired then
    begin Writeln('ERR setup'); Halt(0); end;
    ReportAprResult;
    Halt(0);
  end;
  Report;
end.
