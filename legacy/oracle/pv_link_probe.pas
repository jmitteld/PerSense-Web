program pv_link_probe;
{ Milestone-1 link check for the PV computational chain (PRESVALU + PVLUTIL +
  PVLXSCRN + pvltable) against the headless Globals/HelpSystemUnit stubs. }
uses
  Globals, VIDEODAT, peTypes, peData, INTSUTIL, PVLUTIL, PVLXSCRN, pvltable, PRESVALU;
begin
  Writeln('PV link ok');
  Writeln('OracleErrorFired=', OracleErrorFired);
end.
