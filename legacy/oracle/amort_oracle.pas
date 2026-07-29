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
  Globals, VIDEODAT, peTypes, peData, INTSUTIL, AMORTOP, AMORTIZE;

var
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
  df.c.centurydiv   := 20;
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
  df.c.centurydiv := 20;
end;

var
  totalPaid, totalInt, payment: real;
  totalsLine: string;
  nbal: integer;
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
      skp^.skipmonths := Copy(ParamStr(i), 6, Length(ParamStr(i)));
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

    { Payoff query: emit the DOS-computed as-of balance and stop. }
    if havePayoff then
    begin
      Writeln('payoff ', w^.amount:0:4);
      Halt(0);
    end;

    { APR query: emit the DOS-computed APR (h^.apr) and stop. }
    for i := 5 to ParamCount do
      if ParamStr(i) = 'apr' then
      begin
        Writeln('apr ', h^.apr:0:6, ' status ', h^.aprstatus);
        Halt(0);
      end;

    { solverate query: emit the DOS-solved loan rate (h^.loanrate) and stop. }
    for i := 5 to ParamCount do
      if ParamStr(i) = 'solverate' then
      begin
        Writeln('rate ', h^.loanrate:0:6, ' status ', h^.loanratestatus);
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
      if wantAmt then
        Writeln('solvedamount ', h^.amount:0:6);
      if wantRate then
        Writeln('solvedrate ', h^.loanrate:0:10);
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
