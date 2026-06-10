program link_probe;
{ Milestone 1 of the headless source-oracle: prove the REAL DOS
  computational chain (peTypes -> peData -> INTSUTIL -> AMORTOP ->
  AMORTIZE, plus VIDEODAT) compiles and links against the headless
  Globals / HelpSystemUnit stubs in this directory. No computation yet —
  this just resolves every symbol. Once it links, the driver
  (amort_oracle.pas, milestone 2) populates the loan globals and calls
  the real Amortize. }

uses
  Globals, VIDEODAT, peTypes, peData, INTSUTIL, AMORTOP, AMORTIZE;

begin
  Writeln('link ok: the real computational chain linked against the stubs');
  Writeln('OracleErrorFired=', OracleErrorFired);
end.
