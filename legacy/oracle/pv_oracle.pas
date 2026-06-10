program pv_oracle;
{ Headless source-oracle for the PRESENT VALUE engine. Drives the REAL DOS PV
  units (PRESVALU/PVLUTIL/PVLXSCRN/pvltable, on the peData/INTSUTIL base)
  against the headless Globals/HelpSystemUnit stubs, for differential testing
  vs the Go presentvalue package.

  Milestone A: forward PV of a single LUMP SUM. Given a future amount on a
  date, a valuation (as-of) date, and a rate, compute the present value
  (c[1]^.sumvalue) the way the genuine engine does and print it:

      pv <value>          (or:  ERR <message>)

  Usage:
      pv_oracle lump AMOUNT RATE ASOF_MONTHS
        AMOUNT       future cash amount
        RATE         discount rate as a fraction (0.08 = 8%), continuous via exxp
        ASOF_MONTHS  months from the as-of date to the payment date (payment is
                     ASOF_MONTHS months AFTER the as-of date)

      pv_oracle periodic AMTN RATE PERYR NPERIODS [COLA] [COLAMODE]
        AMTN         per-payment amount
        RATE         discount rate as a fraction
        PERYR        payments per year (1,2,4,12)
        NPERIODS     number of payments (todate = fromdate + NPERIODS periods)
        COLA         optional cost-of-living escalation rate (fraction, default 0)
        COLAMODE     'ann' (annual-stepped, default) or 'cnt' (continuous)
      The stream runs from the as-of date (fromdate) for NPERIODS payments.

  As-of date is fixed at 2024-01-01. 30/360 basis. Prints:  pv <value> ... }

uses
  SysUtils, Classes,
  Globals, peTypes, peData, INTSUTIL, PVLUTIL, PVLXSCRN, pvltable, PRESVALU;

var
  i, e: integer;
  argAmount, argRate, argCola: real;
  argMonths, argPerYr, argN: integer;
  tot: integer;
  mode: string;

{ Allocate + zero every line record the engine may read, wire the array
  pointers, and set the common config. Shared by both modes. }
procedure AllocAll;
begin
  thisrun  := ipvl;
  pvlfancy := false;
  scripting := true;   { suppress RecordError screen I/O on the backward paths }
{$ifdef ACTU}
  fold_in_life := false;
{$endif}
  for i := 1 to maxlines do begin New(a[i]); ZeroLumpSum(a[i]); end;
  for i := 1 to maxlines do begin New(b[i]); ZeroPeriodic(b[i]); end;
  for i := 1 to presvallines do begin New(c[i]); ZeroPresVal(c[i]); end;
  for i := 1 to maxlines do begin New(cc[i]); ZeroRateLine(cc[i]); end;
  New(d); ZeroXPresVal(d);
  a_ := @a; b_ := @b; c_ := @c;

  nlines[PVLPresValBlock]  := 1;
  nlines[PVLLumpSumBlock]  := 0;
  nlines[PVLPeriodicBlock] := 0;

  with c[1]^ do
  begin
    asofstatus := inp;
    asof.d := 1; asof.m := 1; asof.y := 124;       { 2024-01-01 }
    r.status := inp;
    r.peryr  := 1;
    sumvaluestatus := empty;
    sumvalue := 0;
    durationstatus := empty;
  end;

  df.c.basis      := x360;
  df.c.centurydiv := 20;
  df.c.colamonth  := ANN;   { default; periodic mode may override to CNT }
  SetYrDays;
end;

{ Lump sum: date + amount present, value blank. Payment pMonths after as-of. }
procedure SetupLumpPV(pAmount, pRate: real; pMonths: integer);
begin
  AllocAll;
  c[1]^.r.rate := pRate;
  nlines[PVLLumpSumBlock] := 1;
  tot := (1 - 1) + pMonths;
  with a[1]^ do
  begin
    datestatus := inp;
    date.d := 1;
    date.m := (tot mod 12) + 1;
    date.y := 124 + (tot div 12);
    amt0status := inp;
    amt0 := pAmount;
    val0status := empty;
    val0 := 0;
  end;
end;

{ Periodic stream: NPERIODS payments of pAmt at pPerYr/yr, from the as-of date,
  optional COLA. fromdate = as-of (2024-01-01); todate = fromdate + N periods. }
procedure SetupPeriodicPV(pAmt, pRate: real; pPerYr, pN: integer; pCola: real; pColaCnt: boolean);
var mPer, totMonths: integer;
begin
  AllocAll;
  if pColaCnt then df.c.colamonth := CNT;   { continuous COLA }
  c[1]^.r.rate := pRate;
  nlines[PVLPeriodicBlock] := 1;
  mPer := 12 div pPerYr;
  totMonths := pN * mPer;
  with b[1]^ do
  begin
    fromdatestatus := inp;
    fromdate.d := 1; fromdate.m := 1; fromdate.y := 124;         { 2024-01-01 }
    todatestatus := inp;
    todate.d := 1;
    todate.m := ((0 + totMonths) mod 12) + 1;
    todate.y := 124 + ((0 + totMonths) div 12);
    peryrstatus := inp;
    peryr := pPerYr;
    amtnstatus := inp;
    amtn := pAmt;
    { The DOS GUI stores COLA in CONTINUOUS form: the user types a yield and
      the screen converts it via ln(1+yield) before the engine sees it (PV_COLA
      help: "interpreted as yields, not rates"). Replicate that here so the
      headless oracle matches what the shipped program would compute. }
    if pCola <> 0 then begin colastatus := inp; cola := Ln(1 + pCola); end
    else begin colastatus := empty; cola := 0; end;
    valnstatus := empty;
    valn := 0;
  end;
end;

{ Multi-row forward PV: several lump and/or periodic lines, one fixed rate,
  discounted to the as-of date. Tokens from ParamStr(tokenBase):
    lMONTHS=AMT          a lump of AMT, MONTHS after the as-of date
    pAMTN:PERYR:N        a level periodic of AMTN, PERYR/yr, N payments from as-of
  Validates the multi-line classification + summation across rows. }
procedure SetupMultiPV(pRate: real; tokenBase: integer);
var
  ai, la, lb, eqpos, p1, p2, tot, e: integer;
  tok, body: string;
  mv, py, nn: integer; amtv: double;
begin
  AllocAll;
  c[1]^.r.rate := pRate;
  la := 0; lb := 0;
  for ai := tokenBase to ParamCount do
  begin
    tok := ParamStr(ai);
    if (Length(tok) < 2) then continue;
    if (tok[1] = 'l') then
    begin
      { lMONTHS=AMT }
      eqpos := Pos('=', tok); if eqpos = 0 then continue;
      mv := StrToIntDef(Copy(tok, 2, eqpos - 2), -1);
      Val(Copy(tok, eqpos + 1, Length(tok)), amtv, e);
      if (mv < 0) or (e <> 0) then continue;
      inc(la);
      tot := (1 - 1) + mv;
      with a[la]^ do
      begin
        datestatus := inp;
        date.d := 1; date.m := (tot mod 12) + 1; date.y := 124 + (tot div 12);
        amt0status := inp; amt0 := amtv; val0status := empty; val0 := 0;
      end;
    end
    else if (tok[1] = 'p') then
    begin
      { pAMTN:PERYR:N }
      body := Copy(tok, 2, Length(tok));
      p1 := Pos(':', body); if p1 = 0 then continue;
      Val(Copy(body, 1, p1 - 1), amtv, e); if e <> 0 then continue;
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos(':', body); if p2 = 0 then continue;
      py := StrToIntDef(Copy(body, 1, p2 - 1), 0);
      nn := StrToIntDef(Copy(body, p2 + 1, Length(body)), 0);
      if (py < 1) or (nn < 1) then continue;
      inc(lb);
      tot := nn * (12 div py);
      with b[lb]^ do
      begin
        fromdatestatus := inp; fromdate.d := 1; fromdate.m := 1; fromdate.y := 124;
        todatestatus := inp; todate.d := 1;
        todate.m := (tot mod 12) + 1; todate.y := 124 + (tot div 12);
        peryrstatus := inp; peryr := py;
        amtnstatus := inp; amtn := amtv;
        colastatus := empty; cola := 0;
        valnstatus := empty; valn := 0;
      end;
    end;
  end;
  nlines[PVLLumpSumBlock]  := la;
  nlines[PVLPeriodicBlock] := lb;
end;

{ Variable-rate (PVLfancy) lump sum: discount a single future amount through a
  multi-step rate schedule to the as-of date, the way the real fancy engine
  does (ValueOfOnePayment over cc[]). Args after 'vr':
    LUMP_AMOUNT PAY_MONTHS NRATES  year0 rate0  year1 rate1  ...
  where PAY_MONTHS is months from the as-of date (2024-01-01) to the payment,
  and each (yearK, rateK) makes rateK effective from yearK-01-01. rateK is the
  continuous (true) rate. Continuous discounting (d^.simple=false). }
procedure SetupVRLump(pAmount: real; pMonths, pNRates: integer);
var i, tot, base: integer; yr: integer; rt: real; ecode: integer;
begin
  AllocAll;
  pvlfancy := true;

  nlines[PVLRatesBlock]    := pNRates;   { = nlines[3], the rate-line count }
  nlines[PVLXBlock]        := 1;
  nlines[PVLLumpSumBlock]  := 1;
  nlines[PVLPeriodicBlock] := 0;

  base := 6;  { ParamStr(1)='vr',2=amt,3=months,4=nrates; pairs start at 5 }
  for i := 1 to pNRates do
  begin
    yr := StrToIntDef(ParamStr(5 + (i - 1) * 2), 2024);
    Val(ParamStr(6 + (i - 1) * 2), rt, ecode);
    cc[i]^.datestatus := inp;
    cc[i]^.date.d := 1; cc[i]^.date.m := 1; cc[i]^.date.y := yr - 1900;
    cc[i]^.r.status := inp;
    cc[i]^.r.rate   := rt;
    cc[i]^.r.peryr  := 1;
  end;

  { d^ : the extra block holds the fancy as-of date and the (blank) value. }
  with d^ do
  begin
    xasofstatus := inp;
    xasof.d := 1; xasof.m := 1; xasof.y := 124;   { 2024-01-01 }
    simplestatus := inp;
    simple := false;                               { continuous discounting }
    xvaluestatus := empty;
    xvalue := 0;
    status := contains_unknown;
  end;

  tot := (1 - 1) + pMonths;
  with a[1]^ do
  begin
    datestatus := inp;
    date.d := 1;
    date.m := (tot mod 12) + 1;
    date.y := 124 + (tot div 12);
    amt0status := inp;
    amt0 := pAmount;
    val0status := empty;
    val0 := 0;
  end;
  if base = 0 then ;  { silence unused }
end;

{ Variable-rate PERIODIC stream: a level (optionally COLA-escalating) periodic
  payment discounted through a multi-step rate schedule (the fancy FancySummation
  path). Rate pairs (year rate) start at ParamStr(rateBase). }
procedure SetupVRPeriodic(pAmtn: real; pPerYr, pN: integer; pCola: real;
                          pNRates, rateBase: integer);
var i, mPer, totMonths, yr: integer; rt: real; ecode: integer;
begin
  AllocAll;
  pvlfancy := true;
  nlines[PVLRatesBlock]    := pNRates;
  nlines[PVLXBlock]        := 1;
  nlines[PVLLumpSumBlock]  := 0;
  nlines[PVLPeriodicBlock] := 1;

  for i := 1 to pNRates do
  begin
    yr := StrToIntDef(ParamStr(rateBase + (i - 1) * 2), 2024);
    Val(ParamStr(rateBase + 1 + (i - 1) * 2), rt, ecode);
    cc[i]^.datestatus := inp;
    cc[i]^.date.d := 1; cc[i]^.date.m := 1; cc[i]^.date.y := yr - 1900;
    cc[i]^.r.status := inp; cc[i]^.r.rate := rt; cc[i]^.r.peryr := 1;
  end;

  with d^ do
  begin
    xasofstatus := inp;
    xasof.d := 1; xasof.m := 1; xasof.y := 124;
    simplestatus := inp; simple := false;
    xvaluestatus := empty; xvalue := 0;
    status := contains_unknown;
  end;

  mPer := 12 div pPerYr;
  totMonths := pN * mPer;
  with b[1]^ do
  begin
    fromdatestatus := inp;
    fromdate.d := 1; fromdate.m := 1; fromdate.y := 124;   { 2024-01-01 = as-of }
    todatestatus := inp;
    todate.d := 1;
    todate.m := (totMonths mod 12) + 1;
    todate.y := 124 + (totMonths div 12);
    peryrstatus := inp; peryr := pPerYr;
    amtnstatus := inp;  amtn := pAmtn;
    if pCola <> 0 then begin colastatus := inp; cola := Ln(1 + pCola); end
    else begin colastatus := empty; cola := 0; end;
    valnstatus := empty; valn := 0;
  end;
end;

{ Backward solves: supply the target sumvalue and blank one field; the real
  engine's BackwardCalc (amounts) or FrontwardCalc Newton branch (rate/as-of)
  solves it. A single lump line at pMonths after the as-of date. }
procedure SetupLumpFrame(pMonths: integer);
begin
  AllocAll;
  nlines[PVLLumpSumBlock] := 1;
  tot := (1 - 1) + pMonths;
  with a[1]^ do
  begin
    datestatus := inp;
    date.d := 1; date.m := (tot mod 12) + 1; date.y := 124 + (tot div 12);
    val0status := empty; val0 := 0;
  end;
end;

{ A single periodic line from the as-of date for pN payments at pPerYr/yr, value
  blank — the frame for backward periodic-amount/date solves. }
procedure SetupPeriodicFrame(pPerYr, pN: integer);
var mPer, totMonths: integer;
begin
  AllocAll;
  nlines[PVLPeriodicBlock] := 1;
  mPer := 12 div pPerYr;
  totMonths := pN * mPer;
  with b[1]^ do
  begin
    fromdatestatus := inp;  fromdate.d := 1; fromdate.m := 1; fromdate.y := 124;
    todatestatus := inp;    todate.d := 1;
    todate.m := (totMonths mod 12) + 1; todate.y := 124 + (totMonths div 12);
    peryrstatus := inp;     peryr := pPerYr;
    colastatus := empty;    cola := 0;
    valnstatus := empty;    valn := 0;
  end;
end;

begin
  if ParamCount >= 1 then mode := ParamStr(1) else mode := 'lump';

  if mode = 'vr' then
  begin
    Val(ParamStr(2), argAmount, e);
    argMonths := StrToIntDef(ParamStr(3), 12);
    argN      := StrToIntDef(ParamStr(4), 1);
    SetupVRLump(argAmount, argMonths, argN);
    Enter(no_tab);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    Writeln('pv ', d^.xvalue:0:6, ' status ', d^.status, ' frontward ', frontward);
    Halt(0);
  end;

  { multi RATE  l<months>=<amt> ...  p<amtn>:<peryr>:<n> ... }
  if mode = 'multi' then
  begin
    Val(ParamStr(2), argRate, e);
    SetupMultiPV(argRate, 3);
    Enter(no_tab);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    Writeln('pv ', c[1]^.sumvalue:0:6, ' status ', c[1]^.status);
    Halt(0);
  end;

  { vrp AMTN PERYR NPERIODS COLA NRATES  year0 rate0  year1 rate1 ... }
  if mode = 'vrp' then
  begin
    Val(ParamStr(2), argAmount, e);
    argPerYr := StrToIntDef(ParamStr(3), 12);
    argN     := StrToIntDef(ParamStr(4), 12);
    Val(ParamStr(5), argCola, e);
    argMonths := StrToIntDef(ParamStr(6), 1);   { reuse argMonths as NRATES }
    SetupVRPeriodic(argAmount, argPerYr, argN, argCola, argMonths, 7);
    Enter(no_tab);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    Writeln('pv ', d^.xvalue:0:6, ' status ', d^.status, ' frontward ', frontward);
    Halt(0);
  end;

  { bk_rate SUMVALUE AMOUNT ASOF_MONTHS  -> solve the RATE (FrontwardCalc's
    Newton branch; no screen/backup machinery needed, so this runs headlessly).
    The lump/periodic AMOUNT backward solves go through BackwardCalc's bf
    backup-frame, which depends on the full screen-column layout and is not
    driven here — those are validated instead by round-tripping through the
    bit-identical forward oracle (see presentvalue/dos_pv_oracle_test.go). }
  if mode = 'bk_rate' then
  begin
    Val(ParamStr(2), argAmount, e);   { sumvalue target }
    Val(ParamStr(3), argRate,   e);   { the (known) lump amount }
    argMonths := StrToIntDef(ParamStr(4), 12);
    SetupLumpFrame(argMonths);
    c[1]^.r.status := empty; c[1]^.r.rate := 0;
    c[1]^.sumvaluestatus := inp; c[1]^.sumvalue := argAmount;
    a[1]^.amt0status := inp; a[1]^.amt0 := argRate;
    Enter(no_tab);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    Writeln('rate ', c[1]^.r.rate:0:10);
    Halt(0);
  end;

  { bk_asof SUMVALUE AMOUNT RATE LUMP_MONTHS -> solve the AS-OF date (the other
    FrontwardCalc Newton branch, like bk_rate). A single lump of AMOUNT at
    LUMP_MONTHS after 2024-01-01, discounted at RATE; given the target SUMVALUE,
    solve the valuation (as-of) date. Output the solved date as y m d (Pascal
    year, e.g. 124 = 2024). }
  if mode = 'bk_asof' then
  begin
    Val(ParamStr(2), argAmount, e);   { sumvalue target }
    Val(ParamStr(3), argRate,   e);   { the known lump amount }
    Val(ParamStr(4), argCola,   e);   { the known rate (reusing argCola) }
    argMonths := StrToIntDef(ParamStr(5), 12);
    SetupLumpFrame(argMonths);
    c[1]^.asofstatus := empty;                 { solve the as-of date }
    c[1]^.r.status := inp; c[1]^.r.rate := argCola;
    c[1]^.sumvaluestatus := inp; c[1]^.sumvalue := argAmount;
    a[1]^.amt0status := inp; a[1]^.amt0 := argRate;
    Enter(no_tab);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    Writeln('asof ', c[1]^.asof.y, ' ', c[1]^.asof.m, ' ', c[1]^.asof.d);
    Halt(0);
  end;

  { Direct BackwardCalc drives. Now that records are byte-packed (-CPPACKRECORD=1)
    the bf.FixPointers offset machinery is aligned, so the PERIODIC backward
    solves run headlessly and are direct-diffed below.

    NOTE: the LUMP-block backward solves (lump amount/date) still fault inside
    Enter's ComputeLumpsumLineValues path even with packing fixed — a residual
    in the lump-block setup we could not localize without a runtime debugger. The
    lump AMOUNT solve (PV-1) is validated instead by round-tripping through the
    bit-identical forward oracle; the lump DATE solve (PV-2) remains the one PV
    backward path not yet directly diffed. See docs/mortgage_pv_oracle_extension.md. }

  { bk_per_amt SUMVALUE RATE PERYR NPERIODS -> solve the unknown PERIODIC AMOUNT.
    The stream runs from the as-of date for NPERIODS payments. }
  if mode = 'bk_per_amt' then
  begin
    Val(ParamStr(2), argAmount, e); Val(ParamStr(3), argRate, e);
    argPerYr := StrToIntDef(ParamStr(4), 12);
    argN := StrToIntDef(ParamStr(5), 12);
    SetupPeriodicFrame(argPerYr, argN);
    b[1]^.amtnstatus := empty; b[1]^.amtn := 0;
    c[1]^.r.status := inp; c[1]^.r.rate := argRate;
    c[1]^.sumvaluestatus := inp; c[1]^.sumvalue := argAmount;
    Enter(no_tab);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    Writeln('amt ', b[1]^.amtn:0:6);
    Halt(0);
  end;

  { bk_per_todate SUMVALUE AMTN RATE PERYR NSEED -> solve the unknown TO-DATE of
    a periodic stream (PV-5): from-date = as-of, amount given, sumvalue given,
    to-date blank. NSEED seeds the to-date. Output the solved to-date as y m d. }
  if mode = 'bk_per_todate' then
  begin
    Val(ParamStr(2), argAmount, e); Val(ParamStr(3), argRate, e);
    Val(ParamStr(4), argCola, e);
    argPerYr := StrToIntDef(ParamStr(5), 12);
    argN := StrToIntDef(ParamStr(6), 12);
    SetupPeriodicFrame(argPerYr, argN);
    b[1]^.amtnstatus := inp; b[1]^.amtn := argRate;
    b[1]^.todatestatus := empty;
    c[1]^.r.status := inp; c[1]^.r.rate := argCola;
    c[1]^.sumvaluestatus := inp; c[1]^.sumvalue := argAmount;
    Enter(no_tab);
    if OracleErrorFired then begin Writeln('ERR ', OracleLastError); Halt(0); end;
    Writeln('date ', b[1]^.todate.y, ' ', b[1]^.todate.m, ' ', b[1]^.todate.d);
    Halt(0);
  end;

  if mode = 'periodic' then
  begin
    Val(ParamStr(2), argAmount, e);
    Val(ParamStr(3), argRate,   e);
    argPerYr := StrToIntDef(ParamStr(4), 12);
    argN     := StrToIntDef(ParamStr(5), 12);
    argCola  := 0;
    if ParamCount >= 6 then Val(ParamStr(6), argCola, e);
    SetupPeriodicPV(argAmount, argRate, argPerYr, argN, argCola,
                    (ParamCount >= 7) and (ParamStr(7) = 'cnt'));
  end
  else
  begin
    if ParamCount >= 4 then
    begin
      Val(ParamStr(2), argAmount, e);
      Val(ParamStr(3), argRate,   e);
      argMonths := StrToIntDef(ParamStr(4), 12);
    end
    else
    begin
      argAmount := 10000; argRate := 0.08; argMonths := 12;
    end;
    SetupLumpPV(argAmount, argRate, argMonths);
  end;

  Enter(no_tab);

  if OracleErrorFired then
  begin
    Writeln('ERR ', OracleLastError);
    Halt(0);
  end;

  Writeln('pv ', c[1]^.sumvalue:0:6,
          ' status ', c[1]^.status,
          ' val0 ', a[1]^.val0:0:6,
          ' frontward ', frontward);
end.
