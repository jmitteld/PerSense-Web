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
  Globals, peTypes, peData, INTSUTIL, AMORTOP, AMORTIZE;

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
      schedule has no short odd first period (12/peryr months out). }
    firststatus := inp;    firstdate.d := 1;
                           firstdate.m := 1 + (12 div pPerYr);
                           firstdate.y := 124;
                           if firstdate.m > 12 then
                           begin firstdate.m := firstdate.m - 12; firstdate.y := firstdate.y + 1; end;
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
begin
  k := 0;
  for ai := 5 to ParamCount do
  begin
    tok := ParamStr(ai);
    if (Length(tok) >= 2) and ((tok[1] = 'b') or (tok[1] = 'B')) then
    begin
      eqpos := Pos('=', tok);
      if eqpos = 0 then continue;
      monthsVal := StrToIntDef(Copy(tok, 2, eqpos - 2), -1);
      amtStr := Copy(tok, eqpos + 1, Length(tok));
      Val(amtStr, amtVal, e);
      if (monthsVal < 0) or (e <> 0) then continue;
      inc(k);
      tot := (h^.loandate.m - 1) + monthsVal;
      balloon[k]^.datestatus   := inp;
      balloon[k]^.date.d       := h^.loandate.d;
      balloon[k]^.date.m       := (tot mod 12) + 1;
      balloon[k]^.date.y       := h^.loandate.y + (tot div 12);
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

var
  totalPaid, totalInt, payment: real;
  totalsLine: string;
  nbal: integer;
  wantRows, wantDump: boolean;
  rowInt, rowPrin, rowBal: real;
  ti: integer;

begin
  quiet := ParamCount >= 4;
  wantRows := false; wantDump := false; solvedPrepayIdx := 0; solvedDurationIdx := 0;
  for i := 1 to ParamCount do if ParamStr(i) = 'rows' then wantRows := true;
  for i := 1 to ParamCount do if ParamStr(i) = 'dumpraw' then wantDump := true;
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
  nbal := SetupBalloons;
  nbal := SetupPrepayments;

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

  { Computational-setting flags (distinct DOS code paths). These map 1:1 to the
    Go amortization Settings booleans. R78/in-advance/USA-rule all work in the
    ordinary (non-fancy) engine. }
  for i := 5 to ParamCount do
  begin
    if ParamStr(i) = 'inadv'   then df.c.in_advance := true;
    if ParamStr(i) = 'r78'     then df.c.r78        := true;
    if ParamStr(i) = 'usa'     then df.c.USARule    := true;
    if ParamStr(i) = 'prepaid' then df.c.prepaid    := true;
    { 365-day (actual/365.25) basis. Pre-setting it also avoids the biweekly
      auto-switch MessageBox (the engine only switches when basis is x360). }
    if ParamStr(i) = 'b365'    then begin df.c.basis := x365; SetYrDays; end;
    if ParamStr(i) = 'exact'   then df.c.exact      := true;
    { plus_regular ON: extras (prepayments/balloons) ADD to the regular payment;
      OFF (default) they REPLACE it (a payment schedule). }
    if ParamStr(i) = 'plusreg' then df.c.plus_regular := true;
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
    end;

  Output := TStringList.Create;
  try
    MakeTable(Output, false);

    if OracleErrorFired then
    begin
      Writeln('ERR ', OracleLastError);
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
      Writeln('payment ', payment:0:4, ' interest ', totalInt:0:2, ' paid ', totalPaid:0:2)
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
