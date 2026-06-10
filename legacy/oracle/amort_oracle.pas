program amort_oracle;
{ Milestone 2 (first cut) of the headless source-oracle.

  Sets up the REAL global amortization state (h^, df.c, the option
  arrays) for a hard-coded vanilla loan, runs the REAL FirstPass +
  MakeTable, and dumps the raw schedule MakeTable produces. The point of
  this first cut is to SEE the output format so the next iteration can
  parse it into the oraclediff Result JSON contract.

  Once this runs, iteration 2 replaces the hard-coded loan with a
  Worksheet read from stdin and emits a Result JSON to stdout. }

uses
  SysUtils, Classes,
  Globals, peTypes, peData, INTSUTIL, AMORTOP, AMORTIZE;

var
  Output: TStringList;
  i: integer;

procedure SetupVanillaLoan;
begin
  { Allocate and clear the loan record. }
  New(h);
  ZeroAMZLoan(h);

  { No advanced options. The DOS code scans these pointer arrays and
    dereferences each entry (e.g. balloon[i]^.status), so they must point
    to ALLOCATED, ZEROED records — the UI pre-allocates them at startup —
    not nil. Allocate and zero each so FirstPass sees an empty set. }
  for i := 1 to maxballoon do begin New(balloon[i]); ZeroBalloon(balloon[i]); end;
  for i := 1 to maxprepay  do begin New(pre[i]);     ZeroPrepayment(pre[i]); end;
  for i := 1 to maxadj     do begin New(adj[i]);     ZeroAdjustment(adj[i]); end;
  New(mor);  ZeroMoratorium(mor);
  New(targ); ZeroTarget(targ);
  New(skp);  ZeroSkip(skp);

  { Short vanilla loan so MakeTable prints EVERY period (>14 installments
    switches to summary mode): $10,000 @ 12% nominal, 12 monthly payments.
    The payment is left blank (status empty) so the engine SOLVES it —
    giving a clean payment to cross-check against the Go port. Dates: loan
    1/1/2024, first payment 2/1/2024 (year stored as year-1900 => 124). }
  with h^ do
  begin
    amountstatus := inp;   amount   := 10000;
    loanratestatus := inp; loanrate := 0.12;
    nstatus := inp;        nperiods := 12;
    peryrstatus := inp;    peryr    := 12;
    payamtstatus := empty; payamt   := 0;   { solve the payment }
    loandatestatus := inp; loandate.d := 1; loandate.m := 1; loandate.y := 124;
    firststatus := inp;    firstdate.d := 1; firstdate.m := 2; firstdate.y := 124;
    laststatus := empty;
    pointsstatus := empty;
    aprstatus := empty;
    lastok := false;
  end;

  { Computational defaults: ordinary 30/360, monthly, no fancy modes. }
  df.c.basis       := x360;
  df.c.peryr       := 12;
  df.c.exact       := false;
  df.c.in_advance  := false;
  df.c.r78         := false;
  df.c.USARule     := false;
  df.c.prepaid     := false;
  df.c.plus_regular := false;
  df.c.colamonth   := 0;
  df.c.centurydiv  := 20;
end;

begin
  SetupVanillaLoan;

  { MakeTable calls Enter(no_tab) internally, which runs FirstPass + the
    full calculation before emitting the schedule — so we don't (and
    can't, it's TESTING-only-exported) call FirstPass ourselves. }
  Output := TStringList.Create;
  try
    MakeTable(Output, false);
    Writeln('--- MakeTable output (', Output.Count, ' lines) ---');
    for i := 0 to Output.Count - 1 do
      Writeln(i:4, ': ', Output[i]);
    Writeln('--- end ---');
    Writeln('solved payment (h^.payamt) = ', h^.payamt:0:4, '  status=', h^.payamtstatus);
    Writeln('OracleErrorFired=', OracleErrorFired, '  OracleLastError=', OracleLastError);
  finally
    Output.Free;
  end;
end.
