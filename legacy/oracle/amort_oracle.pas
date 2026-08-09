program amort_oracle;
{ Headless source-oracle: drives the REAL DOS amortization engine
  (peData/INTSUTIL/AMORTOP/AMORTIZE) against the headless Globals /
  HelpSystemUnit stubs, for differential testing vs the Go port.

  Usage:
    amort_oracle                         -> verbose dump of a sample loan
    amort_oracle AMOUNT RATE NPER PERYR [bMONTHS=AMT ...]
        -> solve the payment and print one machine-readable result line:
        payment <p> interest <i> paid <t>      (or:  ERR <message>)

  RATE is the nominal loan rate as a fraction (0.12 = 12%). The payment is
  left blank so the engine SOLVES it; interest/paid come from MakeTable's
  total line. 30/360 basis, ordinary (no prepaid/in-advance/R78).

  Optional trailing tokens add BALLOON payments (which switch the engine into
  fancy mode): `bMONTHS=AMT` puts a balloon of AMT dollars MONTHS months after
  the loan date (e.g. b6=5000). With balloons present the solved payment is the
  fancy backward solve — the path validated against the Go SolvePayment fancy
  solver. The 'Balloon includes regular payment' (PlusRegular) flag is OFF, so
  a balloon ADDS to that period's regular payment (DOS plus_regular=false). }

uses
  SysUtils, Classes,
  Globals, VIDEODAT, peTypes, peData, INTSUTIL, AMORTOP, AMORTIZE, OracleBits;

type
  { Overlay for emitting a float64 as its raw 64-bit pattern (intutil *bits). }
  TAmzBitCast = record
    case boolean of
      true:  (d: double);
      false: (q: qword);
  end;

var
  bitcast: TAmzBitCast;
  Output: TStringList;
  i: integer;
  argAmount, argRate: real;
  argN, argPerYr: integer;
  solvedPrepayIdx: integer;
  solvedDurationIdx: integer;
  quiet: boolean;

procedure SetupLoan(pAmount, pRate: real; pN, pPerYr: integer);
begin
  New(h);
  ZeroAMZLoan(h);
  for i := 1 to maxballoon do begin New(balloon[i]); ZeroBalloon(balloon[i]); end;
  for i := 1 to maxprepay  do begin New(pre[i]);     ZeroPrepayment(pre[i]); end;
  for i := 1 to maxadj     do begin New(adj[i]);     ZeroAdjustment(adj[i]); end;
  New(mor);  ZeroMoratorium(mor);
  New(targ); ZeroTarget(targ);
  New(skp);  ZeroSkip(skp);

  with h^ do
  begin
    amountstatus := inp;   amount   := pAmount;
    loanratestatus := inp; loanrate := pRate;
    nstatus := inp;        nperiods := pN;
    peryrstatus := inp;    peryr    := pPerYr;
    payamtstatus := empty; payamt   := 0;   { solve the payment }
    loandatestatus := inp; loandate.d := 1; loandate.m := 1; loandate.y := 124; { 2024 }
    { First payment exactly ONE regular period after the loan date, so the
      schedule has no short odd first period. For weekly/biweekly the period is
      day-based (364/peryr days), so step by days via the date utilities;
      otherwise it is 12/peryr months out. }
    firststatus := inp;
    if (pPerYr = 26) or (pPerYr = 52) then
      begin
        firstdate := loandate;
        AddPeriod(firstdate, pPerYr, loandate.d, add);
      end
    else
      begin
        firstdate.d := 1;
        firstdate.m := 1 + (12 div pPerYr);
        firstdate.y := 124;
        if firstdate.m > 12 then
        begin firstdate.m := firstdate.m - 12; firstdate.y := firstdate.y + 1; end;
      end;
    laststatus := empty;
    pointsstatus := empty;
    aprstatus := empty;
    lastok := false;
  end;

  { The real Globals initializes cum:=' ' (Globals.pas:464); the headless stub
    doesn't, so set it here. cum in [' ','A'..'Z'] makes the table print EVERY
    payment as a detail line (AMORTOP.pas:1069) instead of summary buckets. }
  cum := ' ';

  df.c.basis        := x360;
  df.c.peryr        := pPerYr;
  df.c.exact        := false;
  df.c.in_advance   := false;
  df.c.r78          := false;
  df.c.USARule      := false;
  df.c.prepaid      := false;
  df.c.plus_regular := false;
  df.c.colamonth    := 0;
  { centurydiv MUST be 50 — the shipped DOS default (PEDATA.pas:67 and :697,
    VIDEODAT.pas:25). It is not merely a 2-digit-year parsing preference: it is
    the ONLY input to the "keep going as long as possible" horizon inside
    RepayFancyLoan,

      if (not dateok(stopdate)) then
        begin
          stopdate := firstdate;
          stopdate.y := 100 + pred(df.c.centurydiv);
        end;             (* Keep going as long as possible *)
                                                    (AMORTOP.pas:1143-1147)

    which is reached whenever very_last is not a valid date — precisely the
    fancy TERM-SOLVE path, where DetermineLastPaymentDate (AMORTOP.pas:1323)
    walks the loan with no known last date. `dateok` only tests
    `(f.m>0) and (f.m<13)` (INTSUTIL.pas:584), and the `noterm` blanking below
    leaves h^.lastdate zeroed, so m=0 and the fallback always fires.

    This oracle previously hardcoded 20, putting that horizon at year 119 =
    2019, i.e. BEHIND the first payment of every modern loan date. The walk then
    exited on its `DateComp(WhenToStop^.date, stopdate) >= 0` clause after a
    single iteration with the principal essentially untouched, tripped
    `if (p > minpmt) then goto ABORT` (AMORTOP.pas:1400), and reported

      ERR Payment amount is too small to compute number of periods.

    for EVERY fancy term solve. Found 2026-07-29 by the fuzzer5 backward-solve
    widening: six `noterm` cases where the port produced a schedule and the
    oracle refused. Ablation showed ANY single advanced option (targ=1 alone
    sufficed) flipped DetermineLastPaymentDate out of its closed-form
    "else (not fancy)" arm into the fancy walk and thereby into the refusal,
    while the same case with no advanced options solved cleanly. At 50 the
    horizon is year 149 = 2049 and the two agree: `solvedterm 43 last 2034-1-2`.
    The port was correct in all six cases; the oracle was the outlier.

    Blast radius of the 20 -> 50 change is exactly this one site: centurydiv's
    other uses in the DOS sources are the settings display (INTSUTIL.pas:419),
    the data-file century re-base (INTSUTIL.pas:667-678, compares a loaded
    dd.c against df.c and is unreachable headless), and 2-digit year entry
    (VIDEODAT.pas:475). No oracle calls EvalDateStr — ParseDMY below takes an
    explicit 4-digit year — so no date the oracle parses is affected. }
  df.c.centurydiv   := 50;
end;

{ Parse "D.M.Y" (Y = full year, e.g. 2024) into a daterec (y stored as year-1900). }
procedure ParseDMY(const s: string; var dr: daterec);
var p1, p2: integer; ds, ms, ys: string;
begin
  p1 := Pos('.', s);
  if p1 = 0 then exit;
  ds := Copy(s, 1, p1 - 1);
  p2 := Pos('.', Copy(s, p1 + 1, Length(s)));
  if p2 = 0 then exit;
  ms := Copy(s, p1 + 1, p2 - 1);
  ys := Copy(s, p1 + p2 + 1, Length(s));
  dr.d := StrToIntDef(ds, 1);
  dr.m := StrToIntDef(ms, 1);
  dr.y := StrToIntDef(ys, 1924) - 1900;
end;

{ Pull the number that follows `lbl` in s (e.g. lbl='Interest:'). }
function NumAfter(const s, lbl: string): real;
var p, q: integer; t: string; e: integer; v: double;
begin
  NumAfter := -1;
  p := Pos(lbl, s);
  if p = 0 then exit;
  q := p + Length(lbl);
  while (q <= Length(s)) and (s[q] = ' ') do inc(q);
  t := '';
  while (q <= Length(s)) and (s[q] in ['0'..'9', '.', '-']) do begin t := t + s[q]; inc(q); end;
  Val(t, v, e);
  if e = 0 then NumAfter := v;
end;

{ Parse trailing `bMONTHS=AMT` tokens into the balloon globals and switch the
  engine into fancy mode. Returns the number of balloons added. The balloon
  date is the loan date plus MONTHS months (day-of-month = 1, matching the
  loan/first dates SetupLoan uses). }
function SetupBalloons: integer;
var
  k, ai, eqpos, monthsVal, e, tot: integer;
  tok: string; amtStr: string; amtVal: double;
  body, ds, ms, ys: string; p1, p2, colon: integer;
begin
  k := 0;
  for ai := 5 to ParamCount do
  begin
    tok := ParamStr(ai);
    { bdate=D.M.Y:AMT — an OFF-CYCLE balloon at an explicit calendar date (the
      b<months>= form always lands on the loan day-of-month, i.e. a payment
      date; this expresses a balloon between payment dates). }
    if (Length(tok) > 6) and (Copy(tok, 1, 6) = 'bdate=') then
    begin
      body := Copy(tok, 7, Length(tok));
      colon := Pos(':', body); if colon = 0 then continue;
      Val(Copy(body, colon + 1, Length(body)), amtVal, e); if e <> 0 then continue;
      body := Copy(body, 1, colon - 1);
      p1 := Pos('.', body); if p1 = 0 then continue;
      ds := Copy(body, 1, p1 - 1); body := Copy(body, p1 + 1, Length(body));
      p2 := Pos('.', body); if p2 = 0 then continue;
      ms := Copy(body, 1, p2 - 1); ys := Copy(body, p2 + 1, Length(body));
      inc(k);
      balloon[k]^.datestatus   := inp;
      balloon[k]^.date.d       := StrToIntDef(ds, 1);
      balloon[k]^.date.m       := StrToIntDef(ms, 1);
      balloon[k]^.date.y       := StrToIntDef(ys, 2024) - 1900;
      balloon[k]^.amountstatus := inp;
      balloon[k]^.amount       := amtVal;
      continue;
    end;
    if (Length(tok) >= 2) and ((tok[1] = 'b') or (tok[1] = 'B')) then
    begin
      eqpos := Pos('=', tok);
      if eqpos = 0 then continue;
      monthsVal := StrToIntDef(Copy(tok, 2, eqpos - 2), -1);
      amtStr := Copy(tok, eqpos + 1, Length(tok));
      Val(amtStr, amtVal, e);
      if (monthsVal < 0) or (e <> 0) then continue;
      inc(k);
      { The driver carries the loan's day-of-month across to the option date and
        then CLAMPS it, which is what DOS itself does everywhere a date is moved
        by whole months: AddPeriod (INTSUTIL.pas:1208-1252) restores d:=orig_day
        BEFORE stepping the month and finishes with CheckForDaysTooLarge, and
        that routine (VIDEODAT.pas:349-354) clamps rather than normalises --
        `last:=DaysInM(f); if (f.d>last) then f.d:=last;`. So 31 Jan steps to
        28 Feb and then back to 31 Mar; the original day is sticky and the month
        never rolls forward. Without the clamp the driver could hand the engine
        a 31 Feb, which is not a date any DOS screen can produce: option dates
        are typed into validated date cells, so they are always real calendar
        days. This is what lets the fuzzer draw loan days 29..31. }
      tot := (h^.loandate.m - 1) + monthsVal;
      balloon[k]^.datestatus   := inp;
      balloon[k]^.date.d       := h^.loandate.d;
      balloon[k]^.date.m       := (tot mod 12) + 1;
      balloon[k]^.date.y       := h^.loandate.y + (tot div 12);
      CheckForDaysTooLarge(balloon[k]^.date);
      balloon[k]^.amountstatus := inp;
      balloon[k]^.amount       := amtVal;
    end;
  end;
  if k > 0 then
  begin
    fancy := true;
    nlines[AMZBalloonBlock] := k;   { count the engine scans up to }
    df.c.plus_regular := false;      { balloon ADDS to regular payment }
  end;
  SetupBalloons := k;
end;

{ Parse `adj=MONTHS:RATE:AMOUNT` tokens (rate change / payment change at a date).
  RATE and/or AMOUNT may be blank: `adj=12:0.07:` is a rate-only change,
  `adj=12::1500` is a payment-only change. MONTHS is months after the loan date;
  it should land on a payment date (a multiple of 12/peryr) — the engine snaps it
  to the nearest on-or-before payment date otherwise. SortAdj counts the rows. }
function SetupAdjustments: integer;
var
  k, ai, p1, p2, monthsVal, e, tot: integer;
  tok, body, rateStr, amtStr: string;
  rateVal, amtVal: double;
begin
  k := 0;
  for ai := 5 to ParamCount do
  begin
    tok := ParamStr(ai);
    { adjdmy=D.M.Y:RATE:AMOUNT — like adj= but at an explicit calendar date
      (absolute), so it does not depend on the loan date or its override order. }
    if (Length(tok) > 7) and (Copy(tok, 1, 7) = 'adjdmy=') then
    begin
      body := Copy(tok, 8, Length(tok));
      p1 := Pos('.', body); if p1 = 0 then continue;
      monthsVal := StrToIntDef(Copy(body, 1, p1 - 1), -1);  { day }
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos('.', body); if p2 = 0 then continue;
      tot := StrToIntDef(Copy(body, 1, p2 - 1), -1);        { month }
      body := Copy(body, p2 + 1, Length(body));
      p1 := Pos(':', body); if p1 = 0 then continue;
      e := StrToIntDef(Copy(body, 1, p1 - 1), -1);          { year (4-digit) }
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos(':', body); if p2 = 0 then continue;
      rateStr := Copy(body, 1, p2 - 1);
      amtStr := Copy(body, p2 + 1, Length(body));
      if (monthsVal < 1) or (tot < 1) or (e < 1900) then continue;
      inc(k);
      adj[k]^.datestatus := inp;
      adj[k]^.date.d := monthsVal;
      adj[k]^.date.m := tot;
      adj[k]^.date.y := e - 1900;
      if Length(rateStr) > 0 then
      begin
        Val(rateStr, rateVal, e);
        if e = 0 then begin adj[k]^.loanratestatus := inp; adj[k]^.loanrate := rateVal; end;
      end;
      if Length(amtStr) > 0 then
      begin
        Val(amtStr, amtVal, e);
        if e = 0 then
        begin
          adj[k]^.amountstatus := inp; adj[k]^.amount := amtVal; adj[k]^.amtok := true;
        end;
      end;
      continue;
    end;
    if (Length(tok) > 4) and (Copy(tok, 1, 4) = 'adj=') then
    begin
      body := Copy(tok, 5, Length(tok));
      p1 := Pos(':', body); if p1 = 0 then continue;
      monthsVal := StrToIntDef(Copy(body, 1, p1 - 1), -1);
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos(':', body); if p2 = 0 then continue;
      rateStr := Copy(body, 1, p2 - 1);
      amtStr := Copy(body, p2 + 1, Length(body));
      if monthsVal < 0 then continue;
      inc(k);
      tot := (h^.loandate.m - 1) + monthsVal;
      adj[k]^.datestatus := inp;
      adj[k]^.date.d := h^.loandate.d;
      adj[k]^.date.m := (tot mod 12) + 1;
      adj[k]^.date.y := h^.loandate.y + (tot div 12);
      CheckForDaysTooLarge(adj[k]^.date);
      if Length(rateStr) > 0 then
      begin
        Val(rateStr, rateVal, e);
        if e = 0 then begin adj[k]^.loanratestatus := inp; adj[k]^.loanrate := rateVal; end;
      end;
      if Length(amtStr) > 0 then
      begin
        Val(amtStr, amtVal, e);
        if e = 0 then
        begin
          adj[k]^.amountstatus := inp; adj[k]^.amount := amtVal; adj[k]^.amtok := true;
        end;
      end;
    end;
  end;
  if k > 0 then
  begin
    fancy := true;
    nlines[AMZAdjBlock] := k;
  end;
  SetupAdjustments := k;
end;

{ Parse `pre=STARTMONTHS:NN:PERYR:AMOUNT` tokens into the prepayment globals and
  switch the engine into fancy mode. A prepayment is NN extra payments of AMOUNT
  each, at PERYR/yr, starting STARTMONTHS after the loan date. CheckPrepayments
  (AMORTOP.pas:400) derives the stop date from NN. Returns the count. }
function SetupPrepayments: integer;
var
  k, ai, eqpos, p1, p2, p3, tot, e: integer;
  tok, body: string;
  startM, nnVal, pyVal: integer; amtVal: double;
begin
  k := 0;
  for ai := 5 to ParamCount do
  begin
    tok := ParamStr(ai);
    if (Length(tok) > 4) and (Copy(tok, 1, 4) = 'pre=') then
    begin
      body := Copy(tok, 5, Length(tok));
      p1 := Pos(':', body); if p1 = 0 then continue;
      startM := StrToIntDef(Copy(body, 1, p1 - 1), -1);
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos(':', body); if p2 = 0 then continue;
      nnVal := StrToIntDef(Copy(body, 1, p2 - 1), -1);
      body := Copy(body, p2 + 1, Length(body));
      p3 := Pos(':', body); if p3 = 0 then continue;
      pyVal := StrToIntDef(Copy(body, 1, p3 - 1), -1);
      Val(Copy(body, p3 + 1, Length(body)), amtVal, e);
      if (startM < 0) or (nnVal < 1) or (pyVal < 1) or (e <> 0) then continue;
      inc(k);
      tot := (h^.loandate.m - 1) + startM;
      pre[k]^.startdatestatus := inp;
      pre[k]^.startdate.d := h^.loandate.d;
      pre[k]^.startdate.m := (tot mod 12) + 1;
      pre[k]^.startdate.y := h^.loandate.y + (tot div 12);
      CheckForDaysTooLarge(pre[k]^.startdate);
      pre[k]^.nnstatus := inp;       pre[k]^.nn := nnVal;
      pre[k]^.peryrstatus := inp;    pre[k]^.peryr := pyVal;
      pre[k]^.paymentstatus := inp;  pre[k]^.payment := amtVal;
    end
    else if (Length(tok) > 7) and (Copy(tok, 1, 7) = 'predmy=') then
    begin
      { predmy=D.M.Y:NN:PERYR:AMOUNT — like pre= but with an OFF-CYCLE start date
        at an explicit calendar day (pre= forces the loan day-of-month). Lets the
        rig express a prepay series that starts on the 15th while payments are on
        the 1st. }
      body := Copy(tok, 8, Length(tok));
      { D.M.Y }
      p1 := Pos('.', body); if p1 = 0 then continue;
      startM := StrToIntDef(Copy(body, 1, p1 - 1), -1);  { day }
      body := Copy(body, p1 + 1, Length(body));
      p1 := Pos('.', body); if p1 = 0 then continue;
      nnVal := StrToIntDef(Copy(body, 1, p1 - 1), -1);   { month }
      body := Copy(body, p1 + 1, Length(body));
      p1 := Pos(':', body); if p1 = 0 then continue;
      pyVal := StrToIntDef(Copy(body, 1, p1 - 1), -1);   { year (4-digit) }
      body := Copy(body, p1 + 1, Length(body));
      inc(k);
      pre[k]^.startdatestatus := inp;
      pre[k]^.startdate.d := startM;
      pre[k]^.startdate.m := nnVal;
      pre[k]^.startdate.y := pyVal - 1900;
      { NN:PERYR:AMOUNT }
      p1 := Pos(':', body); if p1 = 0 then continue;
      nnVal := StrToIntDef(Copy(body, 1, p1 - 1), -1);
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos(':', body); if p2 = 0 then continue;
      pyVal := StrToIntDef(Copy(body, 1, p2 - 1), -1);
      Val(Copy(body, p2 + 1, Length(body)), amtVal, e);
      if (nnVal < 1) or (pyVal < 1) or (e <> 0) then continue;
      pre[k]^.nnstatus := inp;       pre[k]^.nn := nnVal;
      pre[k]^.peryrstatus := inp;    pre[k]^.peryr := pyVal;
      pre[k]^.paymentstatus := inp;  pre[k]^.payment := amtVal;
    end
    else if (Length(tok) > 9) and (Copy(tok, 1, 9) = 'presolve=') then
    begin
      { presolve=STARTMONTHS:NN:PERYR — prepayment with BLANK amount; the engine
        solves it (EstimateAndRefinePeriodicPrepayment, Amortize.pas:665). }
      body := Copy(tok, 10, Length(tok));
      p1 := Pos(':', body); if p1 = 0 then continue;
      startM := StrToIntDef(Copy(body, 1, p1 - 1), -1);
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos(':', body); if p2 = 0 then continue;
      nnVal := StrToIntDef(Copy(body, 1, p2 - 1), -1);
      pyVal := StrToIntDef(Copy(body, p2 + 1, Length(body)), -1);
      if (startM < 0) or (nnVal < 1) or (pyVal < 1) then continue;
      inc(k);
      tot := (h^.loandate.m - 1) + startM;
      pre[k]^.startdatestatus := inp;
      pre[k]^.startdate.d := h^.loandate.d;
      pre[k]^.startdate.m := (tot mod 12) + 1;
      pre[k]^.startdate.y := h^.loandate.y + (tot div 12);
      CheckForDaysTooLarge(pre[k]^.startdate);
      pre[k]^.nnstatus := inp;       pre[k]^.nn := nnVal;
      pre[k]^.peryrstatus := inp;    pre[k]^.peryr := pyVal;
      pre[k]^.paymentstatus := empty; pre[k]^.payment := 0;  { solve this }
      solvedPrepayIdx := k;
      { EstimateAndRefinePeriodicPrepayment (Amortize.pas:1355) is only reached
        when the last payment date is KNOWN — the `not h^.lastok` guard at :1350
        diverts to DetermineLastPaymentDate otherwise. Pin lastdate from
        firstdate + (nperiods-1) regular periods so the unkpre branch is taken. }
      tot := (h^.firstdate.m - 1) + (h^.nperiods - 1) * (12 div h^.peryr);
      h^.lastdate.d := h^.firstdate.d;
      h^.lastdate.m := (tot mod 12) + 1;
      h^.lastdate.y := h^.firstdate.y + (tot div 12);
      CheckForDaysTooLarge(h^.lastdate);
      h^.laststatus := inp;
      h^.lastok := true;
    end
    else if (Length(tok) > 7) and (Copy(tok, 1, 7) = 'predur=') then
    begin
      { predur=STARTMONTHS:PERYR:AMOUNT — prepayment with a KNOWN amount but
        BLANK stop date and BLANK count; the engine solves the duration
        (DeterminePrepaymentDuration, Amortize.pas:709). That routine forces
        plus_regular ON (additive) internally. }
      body := Copy(tok, 8, Length(tok));
      p1 := Pos(':', body); if p1 = 0 then continue;
      startM := StrToIntDef(Copy(body, 1, p1 - 1), -1);
      body := Copy(body, p1 + 1, Length(body));
      p2 := Pos(':', body); if p2 = 0 then continue;
      pyVal := StrToIntDef(Copy(body, 1, p2 - 1), -1);
      Val(Copy(body, p2 + 1, Length(body)), amtVal, e);
      if (startM < 0) or (pyVal < 1) or (e <> 0) then continue;
      inc(k);
      tot := (h^.loandate.m - 1) + startM;
      pre[k]^.startdatestatus := inp;
      pre[k]^.startdate.d := h^.loandate.d;
      pre[k]^.startdate.m := (tot mod 12) + 1;
      pre[k]^.startdate.y := h^.loandate.y + (tot div 12);
      CheckForDaysTooLarge(pre[k]^.startdate);
      pre[k]^.peryrstatus := inp;    pre[k]^.peryr := pyVal;
      pre[k]^.paymentstatus := inp;  pre[k]^.payment := amtVal;
      pre[k]^.nnstatus := empty;     pre[k]^.nn := 0;        { solve duration }
      pre[k]^.stopdatestatus := empty;
      solvedDurationIdx := k;
      { DeterminePrepaymentDuration (Amortize.pas:1362) is also behind the
        `not h^.lastok` guard, and uses h^.lastdate. Pin it as for presolve. }
      tot := (h^.firstdate.m - 1) + (h^.nperiods - 1) * (12 div h^.peryr);
      h^.lastdate.d := h^.firstdate.d;
      h^.lastdate.m := (tot mod 12) + 1;
      h^.lastdate.y := h^.firstdate.y + (tot div 12);
      CheckForDaysTooLarge(h^.lastdate);
      h^.laststatus := inp;
      h^.lastok := true;
    end;
  end;
  if k > 0 then
  begin
    fancy := true;
    nlines[AMZPreBlock] := k;
  end;
  SetupPrepayments := k;
end;

{ Return the n-th (1-based) whitespace-delimited token of s, or '' if absent. }
function GetTok(const s: string; n: integer): string;
var i, len, count: integer; inTok: boolean; r: string;
begin
  r := ''; count := 0; inTok := false; len := Length(s);
  i := 1;
  while i <= len do
  begin
    if s[i] <> ' ' then
    begin
      if not inTok then begin inTok := true; inc(count); end;
      if count = n then r := r + s[i];
    end
    else inTok := false;
    inc(i);
  end;
  GetTok := r;
end;

{ Number of whitespace-delimited tokens in s. }
function CountToks(const s: string): integer;
var i, len, count: integer; inTok: boolean;
begin
  count := 0; inTok := false; len := Length(s); i := 1;
  while i <= len do
  begin
    if s[i] <> ' ' then begin if not inTok then begin inTok := true; inc(count); end; end
    else inTok := false;
    inc(i);
  end;
  CountToks := count;
end;

{ Is tok a positive integer (the paynum that begins every detail line)? }
function IsPosInt(const s: string): boolean;
var v, e: integer;
begin
  Val(s, v, e);
  IsPosInt := (e = 0) and (v >= 1);
end;

{ Does s parse as a real? }
function IsFloat(const s: string): boolean;
var v: double; e: integer;
begin
  Val(s, v, e); IsFloat := (e = 0) and (Length(s) > 0);
end;

{ PadSkipMonths — zero-pad every month number in a skip-months string to TWO
  digits, so that "6" -> "06", "1,7" -> "01,07", "5-7" -> "05-07". The month SET
  is unchanged; only the spelling is.

  ⚠️ WHY THIS EXISTS. Round 31. `MonthSetFromString` (Amortize.pas:149-181)
  READS ONE BYTE PAST THE END OF ITS ARGUMENT:

      ws := s[i];
      inc(i);
      if (s[i] in digitset) then ws := ws + s[i]
      else if (s[i] = '-') then dec(i);
      n := round(value(ws));
      if (n >= 1) and (n <= 12) then ... else exit;    -- exit returns FALSE

  After consuming the LAST digit of the string it evaluates `s[i]` at
  `length(s)+1`. `s` is a by-value `str15` parameter, so that byte is the
  callee's own stack, not anything the caller supplied — zeroing the caller's
  `skp^.skipmonths` tail provably does NOT change it (measured, round 31).

  When that byte happens to be a digit the two characters are scored TOGETHER:
  the `-mode msf` trace on `skip=1,7` reads

      MSF tok i=4 len=3 ord=53 ws=[75] n=75
      MSF bad n=75 -> FALSE

  — month "7" plus a stray '5' became month 75, out of range, parse FALSE.
  `FirstPass` (Amortize.pas:253-255) then calls `RecordError`, which sets
  `errorflag` and, under `scripting`, returns with NO MessageBox; `MakeTable`
  exits at `if (errorflag) then exit` having emitted nothing at all — `lines 0`,
  totals -1, `h^.lastdate.m` still at `unkbyte` (-88 signed), `nperiods` 0.
  That is precisely the "garbage horizon cells" signature §65's second subclass
  was scoring HARD.

  A string whose LAST number has two digits cannot over-read: the second digit
  lands exactly on `length(s)` and the lookahead never fires. Hence the padding.
  Verified round 31: every skip string that already parsed answers BYTE-IDENTICALLY
  when padded, and every one that did not now answers, matching the port to the
  cent.

  This is a HARNESS correction and it is scoped to the ARGUMENT ENCODING (R21):
  no DOS source is touched, the screen under test is the same screen, and the
  PORT still receives whatever string the fuzzer generated. DOS's parser is not
  a computation under test here — and it could not be, because its result is not
  a function of its input. See docs/discrepancies.md §65.

  Anything outside the strict grammar [0-9,-] is returned UNCHANGED, so an
  intentionally malformed probe still reaches DOS verbatim. }
function PadSkipMonths(const s: string): string;
var k: integer; r: string; runLen: integer;
begin
  PadSkipMonths := s;
  if s = '' then exit;
  for k := 1 to Length(s) do
    if not ((s[k] >= '0') and (s[k] <= '9')) and (s[k] <> ',') and (s[k] <> '-') then
      exit;                      { outside the grammar — hand it over verbatim }
  r := '';
  k := 1;
  while k <= Length(s) do
  begin
    if (s[k] >= '0') and (s[k] <= '9') then
    begin
      runLen := 0;
      while (k + runLen <= Length(s)) and (s[k + runLen] >= '0')
            and (s[k + runLen] <= '9') do inc(runLen);
      if runLen = 1 then r := r + '0';
      r := r + Copy(s, k, runLen);
      k := k + runLen;
    end
    else
    begin
      r := r + s[k];
      inc(k);
    end;
  end;
  if Length(r) > 15 then exit;   { str15 cannot hold it — leave the original }
  PadSkipMonths := r;
end;

{ A schedule detail line — in BOTH the ordinary format
  (`paynum date int prin bal cumint`) and the fancy format
  (`date payamt int prin bal cumint`) the trailing four numbers are
  int/prin/bal/cumint. Detect a detail line as: >=6 tokens, last token numeric,
  and not the dashes / "Total payments:" line. }
function IsDetailLine(const s: string): boolean;
var firstNonSpace, j: integer; t1: string;
begin
  IsDetailLine := false;
  if CountToks(s) < 6 then exit;
  if Pos('Total', s) > 0 then exit;
  firstNonSpace := 0;
  for j := 1 to Length(s) do if s[j] <> ' ' then begin firstNonSpace := j; break; end;
  if (firstNonSpace > 0) and (s[firstNonSpace] = '-') then exit;   { dashes }
  if not IsFloat(GetTok(s, CountToks(s))) then exit;               { last col numeric }
  { A real payment row starts with a positive paynum (ordinary format) or a
    date token (fancy format, contains '/'). The in-advance / prepaid
    settlement-interest line begins with paynum 0 (or -1) and is excluded so the
    row sequence matches the per-payment schedule. }
  t1 := GetTok(s, 1);
  IsDetailLine := IsPosInt(t1) or (Pos('/', t1) > 0);
end;

{ ---- dispatch differential support ------------------------------------- }

{ Set up the loan for an `eval` field-presence pattern over the four solvable
  top-row fields Amount, Rate, Payment, NumPeriods, holding a VALID context
  (Pmts/Yr, Loan Date, 1st Pmt Date present; Last Pmt Date blank). The present
  fields take a self-consistent tuple (10000 at 12% nominal, payment 888.4879,
  n=12 monthly), so any single blank is solvable and recovers the others. The
  real MakeTable dispatch then decides which field to solve (or refuses). }
procedure SetupEval(haveA, haveR, haveP, haveN: boolean);
begin
  New(h); ZeroAMZLoan(h);
  for i := 1 to maxballoon do begin New(balloon[i]); ZeroBalloon(balloon[i]); end;
  for i := 1 to maxprepay  do begin New(pre[i]);     ZeroPrepayment(pre[i]); end;
  for i := 1 to maxadj     do begin New(adj[i]);     ZeroAdjustment(adj[i]); end;
  New(mor);  ZeroMoratorium(mor);
  New(targ); ZeroTarget(targ);
  New(skp);  ZeroSkip(skp);
  with h^ do
  begin
    if haveA then begin amountstatus := inp; amount := 10000; end
    else begin amountstatus := empty; amount := 0; end;
    if haveR then begin loanratestatus := inp; loanrate := 0.12; end
    else begin loanratestatus := empty; loanrate := 0; end;
    if haveP then begin payamtstatus := inp; payamt := 888.4879; end
    else begin payamtstatus := empty; payamt := 0; end;
    if haveN then begin nstatus := inp; nperiods := 12; end
    else begin nstatus := empty; nperiods := 0; end;
    peryrstatus := inp; peryr := 12;
    loandatestatus := inp; loandate.d := 1; loandate.m := 1; loandate.y := 124;
    firststatus := inp; firstdate.d := 1; firstdate.m := 2; firstdate.y := 124;
    laststatus := empty; lastok := false;
    pointsstatus := empty; aprstatus := empty;
  end;
  cum := ' ';
  df.c.basis := x360; df.c.peryr := 12; df.c.exact := false;
  df.c.in_advance := false; df.c.r78 := false; df.c.USARule := false;
  df.c.prepaid := false; df.c.plus_regular := false; df.c.colamonth := 0;
  { 50 = shipped DOS default; see the long note on the other centurydiv
    assignment above for why 20 broke every fancy term solve. }
  df.c.centurydiv := 50;
end;

var
  totalPaid, totalInt, payment: real;
  totalsLine: string;
  nbal: integer;
  { mordmy= parsing (2026-08-07) }
  tokBody, dStr, mStr, yStr: string;
  dotA, dotB, dv, mv, yv: integer;
  wantRows, wantDump, wantAdjDump: boolean;
  rowInt, rowPrin, rowBal: real;
  ti: integer;
  evalOut: TStringList;
  hasDetail: boolean;
  rx, ry: real;
  ec: integer;
  d1, d2: daterec;
  niCount, niN: integer;
  niZ: upto;
  wantAmt, wantRate, wantTerm: boolean;
  havePayoff: boolean;
  haveDFB: boolean;

begin
  { intutil FN ARGS : evaluate a single core INTSUTIL math/date primitive and
    print it to full precision, for a boundary differential vs the Go port.
      intutil exxp X            -> e^X (DOS exxp, guarded against overflow)
      intutil lnn X             -> ln X (guarded)
      intutil power X N         -> X^N
      intutil round2 X          -> DOS Round2 (round-half-DOWN at the half-cent)
      intutil yearsdif Y1 M1 D1 Y2 M2 D2  -> YearsDif(date1,date2) on 30/360 }
  if (ParamCount >= 1) and (ParamStr(1) = 'intutil') then
  begin
    df.c.basis := x360; SetYrDays;
    if ParamStr(2) = 'exxp' then
      begin Val(ParamStr(3), rx, ec); Writeln(exxp(rx):0:12); end
    else if ParamStr(2) = 'lnn' then
      begin Val(ParamStr(3), rx, ec); Writeln(lnn(rx):0:12); end
    else if ParamStr(2) = 'power' then
      begin Val(ParamStr(3), rx, ec); Val(ParamStr(4), ry, ec); Writeln(Power(rx, ry):0:12); end
    else if ParamStr(2) = 'round2' then
      begin Val(ParamStr(3), rx, ec); Round2(rx); Writeln(rx:0:6); end
    else if ParamStr(2) = 'yearsdif' then
    begin
      d1.y := StrToInt(ParamStr(3)) - 1900; d1.m := StrToInt(ParamStr(4)); d1.d := StrToInt(ParamStr(5));
      d2.y := StrToInt(ParamStr(6)) - 1900; d2.m := StrToInt(ParamStr(7)); d2.d := StrToInt(ParamStr(8));
      Writeln(YearsDif(d1, d2):0:12);
    end
    else if ParamStr(2) = 'noi' then
    begin
      { noi Y1 M1 D1 Y2 M2 D2 PERYR Z : NumberOfInstallments(f,l,peryr,z).
        Z is before / on_or_before / after / on_or_after. Prints the count and
        the RAW adjusted last date (d/m/y as the record holds them). }
      d1.y := StrToInt(ParamStr(3)) - 1900; d1.m := StrToInt(ParamStr(4)); d1.d := StrToInt(ParamStr(5));
      d2.y := StrToInt(ParamStr(6)) - 1900; d2.m := StrToInt(ParamStr(7)); d2.d := StrToInt(ParamStr(8));
      niN := StrToInt(ParamStr(9));
      if ParamStr(10) = 'before' then niZ := before
      else if ParamStr(10) = 'on_or_before' then niZ := on_or_before
      else if ParamStr(10) = 'after' then niZ := after
      else niZ := on_or_after;
      niCount := NumberOfInstallments(d1, d2, niN, niZ);
      Writeln('n ', niCount, ' last ', (d2.y + 1900), ' ', d2.m, ' ', d2.d);
    end
    else if ParamStr(2) = 'addn' then
    begin
      { addn Y1 M1 D1 PERYR N : AddNPeriods(first,last,peryr,n). Prints the RAW
        resulting last date (d/m/y as the record holds them, un-normalized). }
      d1.y := StrToInt(ParamStr(3)) - 1900; d1.m := StrToInt(ParamStr(4)); d1.d := StrToInt(ParamStr(5));
      niN := StrToInt(ParamStr(7));
      AddNPeriods(d1, d2, StrToInt(ParamStr(6)), niN);
      Writeln('last ', (d2.y + 1900), ' ', d2.m, ' ', d2.d);
    end
    else if ParamStr(2) = 'rfybits' then
    begin
      { rfybits YIELD N : RateFromYield(yy,n) (INTSUTIL.pas:1270), printed as the
        RAW float64 bit pattern. A NEW fn name, so no existing intutil caller is
        affected; the whole-stdout parsers only ever invoke the names above.
        Bits, not decimals, because ':0:6' double-rounds (see OracleBits.pas). }
      Val(ParamStr(3), rx, ec);
      bitcast.d := RateFromYield(rx, StrToInt(ParamStr(4)));
      Writeln(HexStr(bitcast.q, 16));
    end
    else if ParamStr(2) = 'yfrbits' then
    begin
      { yfrbits RATE N : YieldFromRate(rr,n) (INTSUTIL.pas:1263), raw bits. }
      Val(ParamStr(3), rx, ec);
      bitcast.d := YieldFromRate(rx, StrToInt(ParamStr(4)));
      Writeln(HexStr(bitcast.q, 16));
    end
    else if ParamStr(2) = 'kickbits' then
    begin
      { kickbits RATE N SCALE : DOS's 365/360 rate round trip
        RateFromYield(YieldFromRate(rr,n)*scale, n) — the PercentValueFromCell
        vratecol/x365_360 arm, INTSUTIL.pas:1611-1614 (which divides; pass
        scale<1 for that direction). Raw bits. }
      Val(ParamStr(3), rx, ec);
      Val(ParamStr(5), ry, ec);
      bitcast.d := RateFromYield(YieldFromRate(rx, StrToInt(ParamStr(4))) * ry,
                                 StrToInt(ParamStr(4)));
      Writeln(HexStr(bitcast.q, 16));
    end
    else
      Writeln('ERR unknown intutil fn');
    Halt(0);
  end;

  { eval A R P N : run the REAL DOS amortization dispatch over a field-presence
    pattern (each of A/R/P/N is '1' present or '0' blank) and report the
    observable outcome — refused (ERR/INSUF) or solved (ok, with the resulting
    payment). The Go engine must agree on which patterns are solvable and on the
    payment. }
  if (ParamCount >= 1) and (ParamStr(1) = 'eval') then
  begin
    SetupEval(ParamStr(2) = '1', ParamStr(3) = '1',
              ParamStr(4) = '1', ParamStr(5) = '1');
    OracleErrorFired := false; OracleLastError := '';
    evalOut := TStringList.Create;
    MakeTable(evalOut, false);
    if OracleErrorFired then
      Writeln('ERR ', OracleFirstError)
    else
    begin
      hasDetail := false;
      for i := 0 to evalOut.Count - 1 do
        if IsDetailLine(evalOut[i]) then begin hasDetail := true; break; end;
      if hasDetail and (h^.payamt > 0) then
        Writeln('ok payment ', h^.payamt:0:4)
      else
        Writeln('INSUF');
    end;
    Halt(0);
  end;

  quiet := ParamCount >= 4;
  wantRows := false; wantDump := false; solvedPrepayIdx := 0; solvedDurationIdx := 0;
  wantAmt := false; wantRate := false; wantTerm := false;
  for i := 1 to ParamCount do if ParamStr(i) = 'rows' then wantRows := true;
  for i := 1 to ParamCount do if ParamStr(i) = 'dumpraw' then wantDump := true;
  wantAdjDump := false;
  for i := 1 to ParamCount do if ParamStr(i) = 'adjdump' then wantAdjDump := true;
  if quiet then
  begin
    Val(ParamStr(1), argAmount, i);
    Val(ParamStr(2), argRate,   i);
    argN     := StrToIntDef(ParamStr(3), 0);
    argPerYr := StrToIntDef(ParamStr(4), 12);
  end
  else
  begin
    argAmount := 10000; argRate := 0.12; argN := 12; argPerYr := 12;
  end;

  SetupLoan(argAmount, argRate, argN, argPerYr);

  { `loandmy=D.M.Y` / `firstdmy=D.M.Y` override the loan and first-payment dates
    explicitly (Y is the full year, e.g. 2024). Lets the differential rig drive
    odd-DAYS first periods (loan day-of-month != first day-of-month), which the
    month-only `first=` cannot express.

    This MUST run between SetupLoan and SetupBalloons/SetupPrepayments/
    SetupAdjustments. Those three anchor every option date on h^.loandate —
    `tot := (h^.loandate.m - 1) + monthsVal; date.d := h^.loandate.d; ...` at
    :172-176, :254-258 and :310-314 — so if the loan date is overridden AFTER
    them, the balloons/prepayments/adjustments stay pinned to SetupLoan's
    default 1.1.2024 while the loan itself moves. The caller, which computes the
    same option months off the date it asked for, then compares two screens
    whose option ROWS sit on different dates.

    2026-07-25: this was the state of the driver when fuzzer5 grew a loan-date
    axis, and it turned 85 of 95 compared cases divergent in a single run — all
    of them the same harness artifact, none a port bug. The token was previously
    only ever used on cases with no advanced options at all, which is why the
    ordering had never mattered. }
  for i := 5 to ParamCount do
  begin
    if (Length(ParamStr(i)) > 8) and (Copy(ParamStr(i), 1, 8) = 'loandmy=') then
      ParseDMY(Copy(ParamStr(i), 9, Length(ParamStr(i))), h^.loandate);
    if (Length(ParamStr(i)) > 9) and (Copy(ParamStr(i), 1, 9) = 'firstdmy=') then
      ParseDMY(Copy(ParamStr(i), 10, Length(ParamStr(i))), h^.firstdate);
  end;

  nbal := SetupBalloons;
  nbal := SetupPrepayments;
  nbal := SetupAdjustments;

  { Optional `pay=X` token: give the payment instead of solving it, so a caller
    can feed both engines the SAME payment and compare the per-row split without
    the payment-solve precision difference as a confound. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 4) and (Copy(ParamStr(i), 1, 4) = 'pay=') then
    begin
      Val(Copy(ParamStr(i), 5, Length(ParamStr(i))), argRate, nbal);
      { defp (not inp): the engine USES this payment but does not treat it as a
        "hard" user input, so it does NOT round each period's interest to cents
        (hard_payment := payamtstatus=inp, AMORTIZE.pas:320). That isolates the
        per-row split from per-period rounding for a clean comparison. }
      h^.payamtstatus := defp;
      h^.payamt := argRate;
    end;

  { Optional `payhard=X` token: like pay= but treats the payment as a HARD user
    input (payamtstatus = inp), so the engine rounds each period's interest to
    cents (hard_payment, AMORTIZE.pas:320) — exactly how a user-entered payment
    behaves. Used to differentially validate the port's hard-payment Round2
    propagation against the real DOS engine. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 8) and (Copy(ParamStr(i), 1, 8) = 'payhard=') then
    begin
      Val(Copy(ParamStr(i), 9, Length(ParamStr(i))), argRate, nbal);
      h^.payamtstatus := inp;
      h^.payamt := argRate;
    end;

  { `pts=X` token: enter points (status inp) so the APR solver runs. X may be 0. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 4) and (Copy(ParamStr(i), 1, 4) = 'pts=') then
    begin
      Val(Copy(ParamStr(i), 5, Length(ParamStr(i))), argRate, nbal);
      h^.points := argRate;
      h^.pointsstatus := inp;
    end;

  { Computational-setting flags (distinct DOS code paths). These map 1:1 to the
    Go amortization Settings booleans. R78/in-advance/USA-rule all work in the
    ordinary (non-fancy) engine. }
  for i := 5 to ParamCount do
  begin
    { --- backward-solve field blanking (2026-07-11 audit extension) ---
      noamt / norate: blank the amount / rate so MakeTable SOLVES it and emit
      `solvedamount X` / `solvedrate X` after the totals (unlike `solverate`,
      these do not Halt early, so rows/totals remain inspectable).
      noterm: blank BOTH n and last date (term solve). non: blank n only,
      leaving a supplied lastdmy= in force (FirstPass derives n from it).
      lastdmy=D.M.Y: set an explicit last payment date. }
    if ParamStr(i) = 'noamt' then
      begin h^.amountstatus := empty; h^.amount := 0; wantAmt := true; end;
    if ParamStr(i) = 'norate' then
      begin h^.loanratestatus := empty; h^.loanrate := 0; wantRate := true; end;
    if ParamStr(i) = 'noterm' then
      begin h^.nstatus := empty; h^.nperiods := 0;
            h^.laststatus := empty; h^.lastok := false; wantTerm := true; end;
    if ParamStr(i) = 'non' then
      begin h^.nstatus := empty; h^.nperiods := 0; end;
    if (Length(ParamStr(i)) > 8) and (Copy(ParamStr(i), 1, 8) = 'lastdmy=') then
      begin ParseDMY(Copy(ParamStr(i), 9, Length(ParamStr(i))), h^.lastdate);
            h^.laststatus := inp; end;
    if ParamStr(i) = 'inadv'   then df.c.in_advance := true;
    if ParamStr(i) = 'r78'     then df.c.r78        := true;
    if ParamStr(i) = 'usa'     then df.c.USARule    := true;
    if ParamStr(i) = 'prepaid' then df.c.prepaid    := true;
    { 365-day (actual/365.25) basis. Pre-setting it also avoids the biweekly
      auto-switch MessageBox (the engine only switches when basis is x360). }
    if ParamStr(i) = 'b365'    then begin df.c.basis := x365; SetYrDays; end;
    { actual/360 hybrid day-count (x365_360): actual calendar days over a
      360-day year. Mirrors Go types.Basis365360 / the UI "365/360" option. }
    if ParamStr(i) = 'b365_360' then begin df.c.basis := x365_360; SetYrDays; end;
    if ParamStr(i) = 'exact'   then begin df.c.exact      := true; end;
    { plus_regular ON: extras (prepayments/balloons) ADD to the regular payment;
      OFF (default) they REPLACE it (a payment schedule). }
    if ParamStr(i) = 'plusreg' then df.c.plus_regular := true;
  end;

  { `solverate` — blank the loan rate so MakeTable SOLVES it from amount + payment
    + term (EstimateAndRefineRate). Requires payhard=/pay= to supply the payment.
    Emitted below as `rate <value>`. Differentially validates the Go SolveRate,
    including the negative-rate (under-funded loan) case. }
  for i := 5 to ParamCount do
    if ParamStr(i) = 'solverate' then
    begin
      h^.loanratestatus := empty;
      h^.loanrate := 0;
    end;

  { `first=MONTHS` overrides the first-payment date to MONTHS months after the
    loan date (default is one full period out). MONTHS < one period gives a
    SHORT odd first stub; > one period gives a LONG one — exercising the
    prorated first-period interest. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 6) and (Copy(ParamStr(i), 1, 6) = 'first=') then
    begin
      nbal := StrToIntDef(Copy(ParamStr(i), 7, Length(ParamStr(i))), 1);
      nbal := (h^.loandate.m - 1) + nbal;
      h^.firstdate.d := h^.loandate.d;
      h^.firstdate.m := (nbal mod 12) + 1;
      h^.firstdate.y := h^.loandate.y + (nbal div 12);
      CheckForDaysTooLarge(h^.firstdate);
    end;

  { `mor=MONTHS` — moratorium: interest-only until first_repay, set to MONTHS
    months after the loan date (must land on a payment date). }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 4) and (Copy(ParamStr(i), 1, 4) = 'mor=') then
    begin
      nbal := StrToIntDef(Copy(ParamStr(i), 5, Length(ParamStr(i))), -1);
      if nbal >= 0 then
      begin
        nbal := (h^.loandate.m - 1) + nbal;
        mor^.first_repay.d := h^.loandate.d;
        mor^.first_repay.m := (nbal mod 12) + 1;
        mor^.first_repay.y := h^.loandate.y + (nbal div 12);
        CheckForDaysTooLarge(mor^.first_repay);
        mor^.first_repaystatus := inp;
        fancy := true;
        nlines[AMZMoratoriumBlock] := 1;
      end;
    end;

  { `mordmy=D.M.Y` — moratorium at an EXPLICIT CALENDAR DATE, the absolute twin of
    `mor=MONTHS`. ADDED 2026-08-07 (rule 7: a NEW token, default output unchanged).
    `mor=` derives the date as loandate + MONTHS and carries `h^.loandate.d` across,
    so on a loan whose FIRST PAYMENT falls on a different day of the month than the
    loan date — 1 Jan origination, 15 Feb first payment, which is an ordinary
    commercial shape — it can only ever express day-1 moratoria. DOS's own screen
    puts the "Int only til" date on a PAYMENT date, so the whole payment-day family
    was unreachable and a real reported screen could not be driven at all. Same
    reasoning, and same naming, as `adjdmy=` beside `adj=` and `bdate=` beside
    `b<months>=`. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 7) and (Copy(ParamStr(i), 1, 7) = 'mordmy=') then
    begin
      tokBody := Copy(ParamStr(i), 8, Length(ParamStr(i)));
      dotA := Pos('.', tokBody);
      if dotA > 0 then
      begin
        dStr := Copy(tokBody, 1, dotA - 1);
        tokBody := Copy(tokBody, dotA + 1, Length(tokBody));
        dotB := Pos('.', tokBody);
        if dotB > 0 then
        begin
          mStr := Copy(tokBody, 1, dotB - 1);
          yStr := Copy(tokBody, dotB + 1, Length(tokBody));
          { REJECT, do not DEFAULT. StrToIntDef silently substitutes on a bad
            parse, so `mordmy=0.0.0` and `mordmy=x.y.z` both passed the
            two-dot shape test and were APPLIED — the first one changing the
            oracle's answer with no error at all. In a binary whose entire job is
            to be the authority, a typo that quietly moves a number is worse than
            a crash. Uses -12345 as the sentinel because StrToIntDef's default is
            the only signal it gives. }
          dv := StrToIntDef(dStr, -12345);
          mv := StrToIntDef(mStr, -12345);
          yv := StrToIntDef(yStr, -12345);
          { NF-6, FIXED ROUND 41. The bound was `yv <= 2200`, and daterec.y is a
            BYTE based at 1900, so every year from 2156 to 2200 passed the check
            and then WRAPPED: `mordmy=15.2.2190` stored 290 mod 256 = 34 and the
            oracle silently computed 1934 — a 256-year error, in the binary whose
            whole job is to be the authority, with no error and no sentinel.
            2155 is 1900+255, the last year the field can hold.

            SCOPE, so this is not read as more than it is: the token was added in
            round 39 and NO published measurement has ever used it — dos_fuzzer5
            does not emit `mordmy=` (verified round 41). This closes a latent
            hazard; it does not move a number.

            AND THE EXPOSURE IS WIDER THAN NF-6 SAYS. This is the ONLY date token
            in the file that range-checks its year at all: `bdate=`, `adjdmy=`,
            `pre=`'s start date and the loan/first dates (lines 159, 208, 289,
            404) all do `- 1900` with NO bound, so the identical wrap is reachable
            through every one of them. FILED, NOT FIXED here — fixing them means
            choosing a rejection behaviour for tokens whose current
            silent-default shape existing corpora may depend on, which is a rule-7
            decision, not a typo fix. }
          if (dv >= 1) and (dv <= 31) and (mv >= 1) and (mv <= 12) and
             (yv >= 1900) and (yv <= 2155) then
          begin
            mor^.first_repay.d := dv;
            mor^.first_repay.m := mv;
            mor^.first_repay.y := yv - 1900;
            CheckForDaysTooLarge(mor^.first_repay);
            mor^.first_repaystatus := inp;
            fancy := true;
            nlines[AMZMoratoriumBlock] := 1;
          end
          else
          begin
            { HALT, do not SILENTLY IGNORE. Narrowing the bound from 2200 to 2155
              on its own would only have traded one silent failure for another:
              before, 2156-2200 wrapped and computed a schedule for the wrong
              century; after, the token would fail the test and simply not be
              applied, and the oracle would print a PLAIN loan's answer to a
              command line that asked for a moratorium. Both are silent, and the
              second is the shape this very token's own comment above objects to
              ("a typo that quietly moves a number is worse than a crash").
              Measured round 41: `mordmy=15.2.2190` returned
              "payment 1321.5074" — the plain loan — under the narrowed bound
              alone.

              So an out-of-range or unparseable mordmy= is now a loud refusal.

              RULE 7, STATED EXACTLY — the round-41 audit refuted the first,
              stronger version of this claim and it is corrected here rather than
              quietly dropped (R43). It is NOT true that this "cannot fire on any
              input the old binary accepted". Two counterexamples the audit found,
              both of which the old binary answered CORRECTLY and the new one now
              refuses:
                amort_oracle 100000 0.10 120 12 mordmy=15.2.2190 mordmy=15.2.2030
                  OLD: payment 2580.1972   (the loop has no last-token-wins rule,
                                            but the valid token still landed)
                amort_oracle 100000 0.10 120 12 mor=6 mordmy=0.0.0
                  OLD: payment 1355.1405   (the valid mor=6 moratorium)
              One junk or duplicate mordmy= anywhere on the line now suppresses an
              otherwise-correct answer.

              THE HONEST STATEMENT: default output is unchanged on every
              WELL-FORMED command line — verified OLD vs NEW over a 378-line
              corpus (scripts/rule7_mordmy_corpus.sh, checked in this round so the
              claim is reproducible) and independently over a 1,150-case audit
              cross-product, 0 differing in both. The stdout footprint of this
              change is exactly: the 14 malformed / out-of-range mordmy= forms,
              plus the two suppression shapes above. Nothing in the tree emits a
              junk or duplicate mordmy= — dos_fuzzer5 emits no mordmy= at all
              (verified round 41) — so no measurement moves. }
            { ASCII ONLY. The first draft of this line carried an em-dash and it
              was the ONLY non-ASCII string literal in ANY of the three oracle
              drivers (audit, round 41). A harness that decodes oracle stdout as
              ascii/latin-1 would raise UnicodeDecodeError here instead of reading
              a refusal. No in-tree harness pins an encoding today, so nothing was
              broken — but the authority binary's stdout is not the place to be
              the first byte of a new hazard. }
            Writeln('ERR mordmy= out of range (d=', dv, ' m=', mv, ' y=', yv,
                    '); day 1..31, month 1..12, year 1900..2155 -- the daterec ',
                    'year is a BYTE based at 1900, so 2156 and above WRAP.');
            Halt(0);
          end;
        end;
      end;
    end;

  { `targ=AMOUNT` — target: minimum principal reduction per payment. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 5) and (Copy(ParamStr(i), 1, 5) = 'targ=') then
    begin
      Val(Copy(ParamStr(i), 6, Length(ParamStr(i))), argRate, nbal);
      if nbal = 0 then
      begin
        targ^.target := argRate;
        targ^.targetstatus := inp;
        fancy := true;
        nlines[AMZTargetBlock] := 1;
      end;
    end;

  { `skip=STR` — skip months string like "6-8" or "1,6,12" (no spaces). }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 5) and (Copy(ParamStr(i), 1, 5) = 'skip=') then
    begin
      { PadSkipMonths: two-digit every month so MonthSetFromString's read at
        length(s)+1 cannot fire. Same month set; see the function's header. }
      skp^.skipmonths := PadSkipMonths(Copy(ParamStr(i), 6, Length(ParamStr(i))));
      skp^.skipstatus := inp;
      fancy := true;
      nlines[AMZSkipMonthBlock] := 1;
    end;

  { `payoff=D.M.Y` — the balance/payoff owed as of a date. Drives the REAL DOS
    ComputeBalanceFromDate (Amortize.pas:1090) by setting the w^ payoff pointer's
    date and letting MakeTable solve w^.amount (Amortize.pas:1423-1424). Emits
    `payoff <amount>`. This is how the Go PayoffBalance port is differentially
    validated across arrears / in-advance / R78 / basis / prepaid. }
  havePayoff := false;
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 7) and (Copy(ParamStr(i), 1, 7) = 'payoff=') then
    begin
      New(w);
      w^.amount := 0;
      ParseDMY(Copy(ParamStr(i), 8, Length(ParamStr(i))), w^.date);
      w^.datestatus := inp;
      w^.amountstatus := empty;
      nlines[AMZBalanceBlock] := 1;
      havePayoff := true;
    end;

  { `solveterm` — blank #periods (and last date) so MakeTable SOLVES the term
    from amount + rate + payment (DetermineLastPaymentDate). Requires payhard=.
    Emitted below as `term <n>`. }
  for i := 5 to ParamCount do
    if ParamStr(i) = 'solveterm' then
    begin
      h^.nstatus := empty;
      h^.laststatus := empty;
    end;

  { `solveballoon=MONTHS` — a terminating balloon MONTHS months after the loan
    date with a BLANK amount; MakeTable SOLVES it (EstimateAndRefineBalloon).
    Requires payhard=. Emitted below as `balloon <amount>`. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 13) and (Copy(ParamStr(i), 1, 13) = 'solveballoon=') then
    begin
      nbal := (h^.loandate.m - 1) + StrToIntDef(Copy(ParamStr(i), 14, Length(ParamStr(i))), 120);
      balloon[1]^.datestatus   := inp;
      balloon[1]^.date.d       := h^.loandate.d;
      balloon[1]^.date.m       := (nbal mod 12) + 1;
      balloon[1]^.date.y       := h^.loandate.y + (nbal div 12);
      CheckForDaysTooLarge(balloon[1]^.date);
      balloon[1]^.amountstatus := empty;
      balloon[1]^.amount       := 0;
      nlines[AMZBalloonBlock]  := 1;
      fancy := true;
      df.c.plus_regular := false;
    end;

  { `dateballoon=MONTHS` — same setup as solveballoon= (a date-only balloon
    with a BLANK amount, plus_regular=false) but does NOT halt after MakeTable,
    so the totals line AND the balloon outcome both print. Used to observe the
    DISPATCH when the payment is ALSO blank (does DOS solve the payment, the
    balloon, both, neither?). Emits `balloonsolved AMOUNT STATUS` after totals. }
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 12) and (Copy(ParamStr(i), 1, 12) = 'dateballoon=') then
    begin
      nbal := (h^.loandate.m - 1) + StrToIntDef(Copy(ParamStr(i), 13, Length(ParamStr(i))), 120);
      balloon[1]^.datestatus   := inp;
      balloon[1]^.date.d       := h^.loandate.d;
      balloon[1]^.date.m       := (nbal mod 12) + 1;
      balloon[1]^.date.y       := h^.loandate.y + (nbal div 12);
      CheckForDaysTooLarge(balloon[1]^.date);
      balloon[1]^.amountstatus := empty;
      balloon[1]^.amount       := 0;
      nlines[AMZBalloonBlock]  := 1;
      fancy := true;
      df.c.plus_regular := false;
      wantAmt := wantAmt; { no-op; keep block non-empty pattern consistent }
    end;

  { `datefrombalance=AMOUNT` — inverse of payoff=: given a target BALANCE, let
    MakeTable SOLVE the date it is reached (ComputeDateFromBalance,
    Amortize.pas:1153). Emitted below as `date D/M/Y`. }
  haveDFB := false;
  for i := 5 to ParamCount do
    if (Length(ParamStr(i)) > 16) and (Copy(ParamStr(i), 1, 16) = 'datefrombalance=') then
    begin
      New(w);
      Val(Copy(ParamStr(i), 17, Length(ParamStr(i))), rx, ec);
      w^.amount := rx;
      w^.amountstatus := inp;
      w^.datestatus := empty;
      nlines[AMZBalanceBlock] := 1;
      haveDFB := true;
    end;

  Output := TStringList.Create;
  try
    MakeTable(Output, false);

    if OracleErrorFired then
    begin
      { FIRST error, not last: see Globals.pas OracleFirstError. }
      Writeln('ERR ', OracleFirstError);
      Halt(0);
    end;

    { `bdump` token: emit the balloon GRID as the DOS screen would show it after
      MakeTable, i.e. including any row TackOnFinalBalloon (Amortize.pas:1040)
      computed and then de-activated with dec(nballoons). That row stays visible
      on the DOS balloon block (nlines) with datestatus/amountstatus = outp even
      though it is excluded from the table walk and from the APR. Emitting it is
      the only way to differentially test the "DOS shows a terminating balloon
      the web does not" report. }
    for i := 5 to ParamCount do
      if ParamStr(i) = 'bdump' then
      begin
        Writeln('nballoons ', nballoons, ' nlines ', nlines[AMZBalloonBlock]);
        for nbal := 1 to maxballoon do
          if (balloon[nbal]^.datestatus > empty) or (balloon[nbal]^.amountstatus > empty) then
            Writeln('balloonrow ', nbal,
                    ' date ', balloon[nbal]^.date.m, '/', balloon[nbal]^.date.d,
                    '/', balloon[nbal]^.date.y + 1900,
                    ' dstatus ', balloon[nbal]^.datestatus,
                    ' amount ', balloon[nbal]^.amount:0:4,
                    ' astatus ', balloon[nbal]^.amountstatus);
        Writeln('lastdate ', h^.lastdate.m, '/', h^.lastdate.d, '/', h^.lastdate.y + 1900,
                ' nperiods ', h^.nperiods);
      end;

    (* `pdump` token: emit the PREPAYMENT grid as it stands after MakeTable, i.e.
       AFTER DetermineLastPaymentDate's post-term-solve window rewrite
       (AMORTOP.pas:1350-1368) has written stopdate/nn back with status outp.

       That rewrite is the sole differential surface for a whole family of
       two-series divergences: it re-stamps each COUNT-specified series from the
       walk-end CURSOR rather than from the entered count, and with two or more
       series it does so through slot contents that CheckOffBalloon has shuffled.
       Without this dump the only observable is the table total, which conflates
       the window with everything else in the walk. Emitted unconditionally (no
       Halt) so it can be stacked with the totals comparison and with bdump. *)
    for i := 5 to ParamCount do
      if ParamStr(i) = 'pdump' then
      begin
        Writeln('npre ', npre, ' nlines ', nlines[AMZpreblock]);
        for nbal := 1 to maxprepay do
          if (pre[nbal]^.startdatestatus > empty) or (pre[nbal]^.nnstatus > empty) or
             (pre[nbal]^.stopdatestatus > empty) or (pre[nbal]^.paymentstatus > empty) then
            Writeln('prerow ', nbal,
                    ' start ', pre[nbal]^.startdate.m, '/', pre[nbal]^.startdate.d,
                    '/', pre[nbal]^.startdate.y + 1900,
                    ' sstatus ', pre[nbal]^.startdatestatus,
                    ' stop ', pre[nbal]^.stopdate.m, '/', pre[nbal]^.stopdate.d,
                    '/', pre[nbal]^.stopdate.y + 1900,
                    ' pstatus ', pre[nbal]^.stopdatestatus,
                    ' nn ', pre[nbal]^.nn, ' nstatus ', pre[nbal]^.nnstatus,
                    ' peryr ', pre[nbal]^.peryr,
                    ' amount ', pre[nbal]^.payment:0:4);
      end;

    { Payoff query: emit the DOS-computed as-of balance and stop. }
    if havePayoff then
    begin
      Writeln('payoff ', w^.amount:0:4);
      RawBitsAdd('payoff', w^.amount); RawBitsFlush;
      Halt(0);
    end;

    { APR query: emit the DOS-computed APR (h^.apr) and stop. }
    for i := 5 to ParamCount do
      if ParamStr(i) = 'apr' then
      begin
      begin
        Writeln('apr ', h^.apr:0:6, ' status ', h^.aprstatus);
        RawBitsAdd('apr', h^.apr); RawBitsFlush;
      end;
        Halt(0);
      end;

    { solverate query: emit the DOS-solved loan rate (h^.loanrate) and stop. }
    for i := 5 to ParamCount do
      if ParamStr(i) = 'solverate' then
      begin
      begin
        Writeln('rate ', h^.loanrate:0:6, ' status ', h^.loanratestatus);
        RawBitsAdd('rate', h^.loanrate); RawBitsFlush;
      end;
        Halt(0);
      end;

    { datefrombalance query: emit the DOS-solved date (w^.date) and stop.
      Year is emitted as the 4-digit calendar year (pascal year + 1900). }
    if haveDFB then
    begin
      Writeln('date ', w^.date.m, '/', w^.date.d, '/', w^.date.y + 1900,
              ' status ', w^.datestatus);
      Halt(0);
    end;

    { solveterm query: emit the DOS-solved number of periods and stop. }
    for i := 5 to ParamCount do
      if ParamStr(i) = 'solveterm' then
      begin
        Writeln('term ', h^.nperiods, ' last ', h^.lastdate.m, '/', h^.lastdate.d,
                '/', h^.lastdate.y + 1900, ' status ', h^.nstatus);
        Halt(0);
      end;

    { solveballoon query: emit the DOS-solved balloon amount and stop. }
    for i := 5 to ParamCount do
      if (Length(ParamStr(i)) > 13) and (Copy(ParamStr(i), 1, 13) = 'solveballoon=') then
      begin
        Writeln('balloon ', balloon[1]^.amount:0:4, ' status ', balloon[1]^.amountstatus);
        Halt(0);
      end;

    { presolve mode: the engine solved the unknown prepayment amount
      (EstimateAndRefinePeriodicPrepayment). Emit it for the differential test. }
    if solvedPrepayIdx > 0 then
    begin
      Writeln('prepay ', pre[solvedPrepayIdx]^.payment:0:4);
      Halt(0);
    end;

    { duration solve: the engine solved the unknown prepayment COUNT
      (DeterminePrepaymentDuration). Emit the solved nn for the differential test. }
    if solvedDurationIdx > 0 then
    begin
      Writeln('duration ', pre[solvedDurationIdx]^.nn);
      Halt(0);
    end;

    payment := h^.payamt;
    if wantAdjDump then
      for i := 1 to nadj do
        Writeln('adjrow ', i,
                ' date ', adj[i]^.date.m, '/', adj[i]^.date.d, '/', (adj[i]^.date.y + 1900),
                ' rate ', adj[i]^.loanrate:0:10, ' ratestatus ', adj[i]^.loanratestatus,
                ' amount ', adj[i]^.amount:0:6, ' amtstatus ', adj[i]^.amountstatus,
                ' amtok ', adj[i]^.amtok);
    totalsLine := '';
    for i := 0 to Output.Count - 1 do
      if Pos('Total payments:', Output[i]) > 0 then totalsLine := Output[i];
    totalPaid := NumAfter(totalsLine, 'Total payments:');
    totalInt  := NumAfter(totalsLine, 'Interest:');

    if wantDump then
    begin
      Writeln('payment ', payment:0:4, ' lines ', Output.Count);
      for i := 0 to Output.Count - 1 do Writeln('L', i, '|', Output[i]);
      Writeln('end');
    end
    else if wantRows then
    begin
      { Emit one clean line per payment: the trailing 4 numbers on each detail
        line are interest, principal-this-period, balance-after, cum-interest.
        Taking them from the end is robust to however the date tokenizes. }
      Writeln('payment ', payment:0:4);
      for i := 0 to Output.Count - 1 do
        if IsDetailLine(Output[i]) then
        begin
          ti := CountToks(Output[i]);
          Val(GetTok(Output[i], ti - 3), rowInt,  nbal);
          Val(GetTok(Output[i], ti - 2), rowPrin, nbal);
          Val(GetTok(Output[i], ti - 1), rowBal,  nbal);
          Writeln('row ', GetTok(Output[i], 1),
                  ' int ', rowInt:0:4, ' prin ', rowPrin:0:4, ' bal ', rowBal:0:4);
        end;
      Writeln('end');
    end
    else if quiet then
    begin
      Writeln('payment ', payment:0:4, ' interest ', totalInt:0:2, ' paid ', totalPaid:0:2);
      { Only `payment` gets raw bits here. totalInt / totalPaid are NOT engine
        doubles — they are re-parsed out of the already-formatted 2-decimal
        totals line a few lines above (NumAfter(totalsLine, ...)), so their low
        bits carry no information about the engine and a bit comparison against
        them would report false divergences. Compare those two as decimals, the
        way the existing sweeps do. `payment` is h^.payamt, a real engine value. }
      RawBitsAdd('payment', payment); RawBitsFlush;
      if wantAmt then
      begin
        Writeln('solvedamount ', h^.amount:0:6);
        RawBitsAdd('solvedamount', h^.amount); RawBitsFlush;
      end;
      if wantRate then
      begin
        Writeln('solvedrate ', h^.loanrate:0:10);
        RawBitsAdd('solvedrate', h^.loanrate); RawBitsFlush;
      end;
      if wantTerm then
        Writeln('solvedterm ', h^.nperiods, ' last ', (h^.lastdate.y + 1900), '-', h^.lastdate.m, '-', h^.lastdate.d);
      for i := 5 to ParamCount do
        if (Length(ParamStr(i)) > 12) and (Copy(ParamStr(i), 1, 12) = 'dateballoon=') then
          Writeln('balloonsolved ', balloon[1]^.amount:0:4, ' status ', balloon[1]^.amountstatus);
    end
    else
    begin
      Writeln('--- MakeTable output (', Output.Count, ' lines) ---');
      for i := 0 to Output.Count - 1 do Writeln(i:4, ': ', Output[i]);
      Writeln('--- end ---');
      Writeln('payment=', payment:0:4, ' interest=', totalInt:0:2, ' paid=', totalPaid:0:2);
    end;
  finally
    Output.Free;
  end;
end.
