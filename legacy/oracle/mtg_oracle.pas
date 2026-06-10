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

procedure AllocMtg;
var i: integer;
begin
  for i := 1 to maxlines do begin New(e[i]); ZeroMortgage(e[i]); end;
  nlines[MTGBlock]    := 1;
  scrollpos[MTGBlock] := 0;
  df.c.basis      := x360;
  df.c.centurydiv := 20;
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
