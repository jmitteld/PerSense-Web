{ ==========================================================================
  PVLXSCRN.pas

  PURPOSE / ROLE
    Logic backing the Present Value "fancy" / "X" screen - the extended PV mode
    that uses a TABLE of interest rates that change over time (rather than a
    single rate) plus optional cost-of-living (COLA) escalation on periodic
    payments and optional life-contingency folding. It owns the screen's
    first-pass classification and the value computations for that mode.

    The plain PV screen lives in PRESVALU.pas; PVLPlainFancy toggles between
    the two layouts (swapping block geometry, data sizes, and the active data
    pointers a_/b_/c_ between the plain a/b/c arrays and the fancy aa/bb/cc).

  KEY ROUTINES
    FancyFirstPass        - classify every input line (EMPTY / contains_unknown
                            / fully_specified), validate ordering, decide
                            frontward vs backward, and compute the grand total.
    FancySummation        - value (scaled to a unit payment) of one periodic
                            series with COLA, by exact period-by-period walk.
    InitializeColaData /
    UpdateAmountWithCola  - drive the COLA escalation schedule.
    PVLPlainFancy         - toggle plain<->fancy screen state.

  KEY DATA (from PEDATA/PETYPES)
    aa[] lumpsum lines, bb[] periodic lines, cc[] rate-table rows, d^ the
    screen-level record (xasof, xvalue, simple, status, ...). Status values are
    EMPTY/contains_unknown/fully_specified/over_determined.

  NOTE: large $ifdef 0 ... $endif blocks below preserve the original DOS
        direct-screen-drawing code (PresValScreen, etc.); they are NOT compiled
        in the Windows port and are left verbatim (including box-draw glyphs).
  ========================================================================== }
unit PVLXSCRN;
{$ifdef OVERLAYS} {$F+,O+} {$endif}

INTERFACE
//uses OPCRT,OPMOUSE,VIDEODAT,NORTHWND,PETYPES,PEDATA,INPUT,INTSUTIL,PEPANE,PVLUTIL;
uses VIDEODAT,PETYPES,PEDATA,INTSUTIL,PVLUTIL, Globals;

type
      readfunc=function(y :byte; erase :boolean):byte;     // cell-reader callback signature (legacy)
      writeproc=procedure(y,j :byte);                      // cell-writer callback signature (legacy)

// First pass over the fancy PV screen: classify lines, validate, compute totals.
procedure FancyFirstPass;
// Value (scaled to a unit payment) of periodic series j, including COLA.
function FancySummation(j :byte):real;
// Initialize COLA scaling state (multiplier and next-escalation date) for series b.
procedure InitializeColaData(var b :periodic; var scaledamt:real; var coladate:daterec);
// Toggle the screen between plain and fancy PV layouts/state.
procedure PVLPlainFancy;
//procedure PresValScreen(code :byte);
// Advance the COLA multiplier for the current payment date t per the COLA schedule.
procedure UpdateAmountWithCola(var b :periodic; var scaledamt :real; var t,coladate :daterec);

IMPLEMENTATION

uses Presvalu, HelpSystemUnit;

{$ifdef 0}
We don't need to include the window stuff as it's handled by Delphi now.
{$F+}
procedure PresValScreen(code :byte);
          var i,attr :byte;
{$ifndef OVERLAYS} {$F-} {$endif}
          begin
          window(1,1,80,25);
clrscr;
          if (boolean(code) xor pvlfancy) then PVLPlainFancy;
          if (color) then attr:=(magenta shl 4 + white) else attr:=112;
{$ifdef TOPMENUS}
          DrawMenuBar; gotoxy(1,2);
          textattr:=MainScreenColors;
{$endif}
             write('ÚÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÒÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄ¿',
                   '³ Single Payments:             º Periodic Payments:                            ³');
{$ifdef TOPMENUS}
             FastWrite('PRESENT VALUE SCREEN',1,59,attr);
             write(
{$else}
             FastWrite('PRESENT VALUE SCREEN',1,3,attr);
             write('³                                                                   ³          ³',
{$endif}
                   '³ Date     Amount      Value   º From    Through PerYr  Amount  COLA%  Value   ³',
                   'ÆÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÍÍÎÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÑÍÍÑÍÍÍÍÍÍÍÍÍÑÍÍÍÍÍÑÍÍÍÍÍÍÍÍÍÍµ');
             for i:=1 to screenlines[PVLLumpSumBlock] do
             write('³        ³          ³          º        ³        ³  ³         ³     ³          ³');
             write('ÃÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÄĞÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÁÄÄÄÄÄÄÄÄÄÁÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÄ´');
          if (code=0) {not pvlfancy} then
             write('³                       True    Loan                                           ³',
                   '³              As of    Rate %  Rate %  Yield %  Value                         ³',
                   '³            ÕÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÍÍÍ¸                    ³',
                   '³ Present    ³        ³       ³       ³       ³           ³                    ³',
                   '³  Value     ³        ³       ³       ³       ³           ³                    ³',
                   '³            ³        ³       ³       ³       ³           ³                    ³',
                   '³            ÀÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÄÄÙ                    ³')
          else {pvlfancy}
             write('³  Effective True    Loan                                                      ³',
                   '³    Date    Rate %  Rate %  Yield %             Value computed with           ³',
                   '³  ÕÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍ¸            rate table at left:           ³',
                   '³  ³   XX   ³       ³       ³       ³                                          ³',
                   '³  ³        ³       ³       ³       ³              Interest                    ³',
                   '³  ³        ³       ³       ³       ³      As of  Computation  Total value     ³',
                   '³  ³        ³       ³       ³       ³     ÕÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÍÍÍÍÍ¸    ³',
                   '³  ³        ³       ³       ³       ³     ³        ³COMPOUND³             ³    ³',
                   '³  ³        ³       ³       ³       ³     ÀÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÄÄÄÄÙ    ³',
                   '³  ÀÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÙ                                          ³');
          FastWrite(LineWithCorners,24,1,textattr);
          InitScrollBar(PVLLumpSumBlock);
          InitScrollBar(PVLPeriodicBlock);
          if (code=0) then begin
             if (nlines[PVLPresValBlock]=0) then AddRow(PVLPresValBlock);
             end
          else {code<>0; pvlfancy} begin
             InitScrollBar(PVLRatesBlock);
            {Note: AddRow has a call to UpdateScrollBar, so it must be called
             after InitScrollBar}
             if (nlines[PVLPresValBlock]=0) then begin
               AddRow(PVLPresValBlock);
               cc[1]^.date:=earliest;  cc[1]^.datestatus:=defp;
               if (swapnlines3>=1) and (c[1]^.r.status=inp) then
                  cc[1]^.r:=c[1]^.r;
               end;
             if (nlines[PVLXBlock]=0) then begin
               AddRow(PVLXBlock);
               if (swapnlines3>=1) and (c[1]^.asofstatus=inp) then begin
                  d^.xasof:=c[1]^.asof; d^.xasofstatus:=inp;
                  end;
               if (swapnlines3>=1) and (c[1]^.sumvaluestatus=inp) then begin
                  d^.xvalue:=c[1]^.sumvalue; d^.xvaluestatus:=inp;
                  end;
               end;
             end;
          Menu; {UpdateSettings; memory.PlaceOnScreen;}
          DisplayAll;
          Home(PVLPeriodicBlock); screenstatus:=needs_nothing;
          end;
{$endif}

{ UpdateAmountWithCola
  PURPOSE: compute the COLA escalation multiplier applied to a periodic payment
           occurring on date t.
  PARAMS:  b - the periodic series (b.cola is the COLA rate, b.fromdate its
           start); scaledamt (var) - the running COLA multiplier; t - current
           payment date; coladate (var) - the date the next yearly step is due.
  SIDE EFFECTS: updates scaledamt and may advance coladate by a year.
  INTENT:  two COLA conventions. When COLAmonth=CNT (continuous) or cola=0, the
           multiplier is continuously compounded from fromdate: exp(cola*years).
           Otherwise it steps discretely: each time t reaches coladate, multiply
           by exp(cola) and push coladate forward one year. }
{ Go port: internal/finance/presentvalue/variablerate.go: vrPeriodicValue (line 159) -- applies exp(cola) at each anniversary step; step-date advance mirrors firstCOLAStepDate (calc.go:230) }
procedure UpdateAmountWithCola(var b :periodic; var scaledamt :real; var t,coladate :daterec);
          begin
          if (df.c.COLAmonth=CNT) or (b.cola=0) then
            scaledamt:=exxp(b.cola*YearsDif(t,b.fromdate))     {continuous compounding from start}
          else begin
            if (DateComp(t,coladate)>=0) then begin            {reached the next anniversary?}
               scaledamt:=scaledamt*exxp(b.cola);              {apply one annual step}
               inc(coladate.y);
               end;
            end;
          end;

{ InitializeColaData
  PURPOSE: prime the COLA multiplier and the date of the first COLA step for a
           periodic series, before walking its payments.
  PARAMS:  b - the periodic series; scaledamt (var) - set to 1.0; coladate (var)
           - set to the first escalation date.
  INTENT:  if a specific COLA month (1..12) is configured, the first step falls
           on that month/day-1 of the year on/after fromdate; otherwise the
           first step is one year after fromdate. }
{ Go port: internal/finance/presentvalue/calc.go: firstCOLAStepDate (line 230) -- prime COLA step date and running scaled amount }
procedure InitializeColaData(var b :periodic; var scaledamt:real; var coladate:daterec);
          begin
          scaledamt:=1;
          coladate:=b.fromdate;
          if (df.c.colamonth in [1..12]) then begin
             if (df.c.colamonth<=b.fromdate.m) then inc(coladate.y);   {month already passed -> next year}
             coladate.m:=df.c.colamonth;
             coladate.d:=1;
             end
          else inc(coladate.y);                                        {default: one year out}
          end;

{ SwapBytes (private)
  PURPOSE: exchange two byte variables. Used by PVLPlainFancy to swap the live
           line count with the saved one when toggling screen modes. }
{ Go port: n/a -- DOS text UI helper; no Go equivalent }
procedure SwapBytes(var x,y :byte);
          var z: byte;
          begin
          z:=x; x:=y; y:=z;
          end;

{ FillOutLandmarks (private)
  PURPOSE: recompute the screen-column "landmarks" (start/end of the As-of,
           True-rate, Loan-rate, and Yield columns) and wire up the active data
           pointers after a plain<->fancy mode change.
  SIDE EFFECTS: fills startof[]/endof[] for the rate columns; sets the present-
           value block's last column and right edge; selects which arrays are
           live: in fancy mode a_/b_/c_ point at aa/bb/cc and the rates block
           scrolls; in plain mode they point at a/b/c. Updates blockdata[].
  NOTE: in fancy mode aa==a and bb==b (shared) but cc and c are distinct, so the
        top-screen data persists when the user presses "X". }
{ Go port: n/a -- DOS screen-layout helper; superseded by web frontend }
procedure FillOutLandmarks;
          begin
          LineCountsfromBbot;
          startof[asofcol]:=lleft[3];
          startof[tratecol]:=startof[asofcol]+9;
          startof[lratecol]:=startof[tratecol]+8;
          startof[yieldcol]:=startof[lratecol]+8;
          endof[asofcol]:=startof[tratecol]-2;
          endof[tratecol]:=startof[lratecol]-2;
          endof[lratecol]:=startof[yieldcol]-2;
          endof[yieldcol]:=startof[yieldcol]+6;
          if (pvlfancy) then lcol[PVLPresValBlock]:=yieldcol
          else lcol[PVLPresValBlock]:=sumvaluecol;
          rright[PVLPresValBlock]:=endof[lcol[PVLPresValBlock]];
          if (pvlfancy) then begin
            {For now, aa is the same as a and bb is the same as b
             but cc and c are distinct.  This is so data on top of
             screen states there when you press "X".}
             a_:=@aa; b_:=@bb; c_:=@cc;
             scrolls[PVLRatesBlock]:=true;
             end
          else begin
             a_:=@a; b_:=@b; c_:=@c;
             scrolls[PVLPresValBlock]:=false;
             end;
          blockdata[PVLLumpSumBlock] := pointer(a_);
          blockdata[PVLPeriodicBlock] := pointer(b_);
          blockdata[PVLPresValBlock] := pointer(c_);
          end;

{ PVLPlainFancy
  PURPOSE: toggle the Present Value screen between the plain (single-rate) and
           fancy (rate-table) layouts, swapping all the geometry/state needed.
  SIDE EFFECTS: flips the global pvlfancy flag; switches block tops/lefts/bottoms
           (fancy* vs plain*/def* constants), the rate block's record size
           (rateline vs presval), and the scroll position; swaps the live and
           saved line counts via SwapBytes; then calls FillOutLandmarks to
           re-derive columns and active pointers.
  INTENT:  registered as PEDATA.PVLPlainFancy in the unit init so the rest of the
           app can request a mode change through that hook. }
{ Go port: n/a -- plain<->fancy (variable-rate) mode toggle is a UI concern; the API selects forwardVariableRate when a rate schedule is present (variablerate.go:273) }
procedure PVLPlainFancy;
          var b :shortint;
          begin
          pvlfancy:=not pvlfancy;
          if (pvlfancy) then begin
             ztop[3]:=fancytop3;
             lleft[3]:=fancyleft3;
             for b:=PVLLumpSumBlock to PVLPresValBlock do bbot[b]:=fancybot[b];
             scrollpos[3]:=scrollpos3;
             datasize[3]:=sizeof(rateline);
             end
          else begin
             ztop[3]:=plaintop3;
             lleft[3]:=plainleft3;
             for b:=PVLLumpSumBlock to PVLPresValBlock do bbot[b]:=defbot[b];
             scrollpos3:=scrollpos[PVLRatesBlock];
             scrollpos[PVLPresValBlock]:=0; {PVLPresValBlock=PVLRatesBlock}
             datasize[3]:=sizeof(presval);
             end;
          SwapBytes(nlines[3],swapnlines3);
          FillOutLandmarks;
          end;

{ DetermineStatus1 (private)
  PURPOSE: classify lump-sum (single-payment) line i on the fancy screen.
  PARAMS:  i - line index into aa[].
  SIDE EFFECTS: sets aa[i]^.status to:
           fully_specified - date present AND (amount OR value) present;
           contains_unknown - date present but neither amount nor value
                              (solve for the missing amount/value);
           EMPTY            - no date.
  NOTE: backward solving FOR the date is not supported on the X screen. }
{ Go port: internal/finance/presentvalue/backward.go: FirstPass (line 163) -- lump-row field-presence status classification }
procedure DetermineStatus1(i :byte);
          begin with aa[i]^ do begin
          if (datestatus>=defp) and ((amt0status>=defp) or (val0status>=defp)) then status:=fully_specified
          else if (datestatus>=defp) then status:=contains_unknown
            {Backwards calculation of date is not supported on the X screen}
          else status:=EMPTY;
          end; end;

{ DetermineStatus2 (private)
  PURPOSE: classify periodic-payment line j on the fancy screen and compute its
           installment count.
  PARAMS:  j - line index into bb[].
  SIDE EFFECTS: sets bb[j]^.status and ninstallments; may set errorflag and show
           a message if From is not before Through.
  INTENT:  ok4/ok5/ok6 = From/Through/PerYr present; ok7/ok9 = amount/value
           present. Result: 0 (PerYr required) if no PerYr; fully_specified if
           From,Through and (amount or value); contains_unknown if there are
           installments (solve for the amount); else MISSING_3 (won't calc).
  NOTE: the only backward unknown permitted here is the periodic amount. }
{ Go port: internal/finance/presentvalue/backward.go: FirstPass (line 163) -- periodic-row field-presence status classification }
procedure DetermineStatus2(j :byte);
          var saveto              :daterec;
              ok4,ok5,ok6,ok7,ok9 :boolean;
          begin with bb[j]^ do begin
          ok4:=(fromdatestatus>=defp);                {From date present?}
          ok5:=(todatestatus>=defp);                  {Through date present?}
          ok6:=(peryrstatus>=defp);                   {payments-per-year present?}
          ninstallments:=0;
          if (ok4) and (ok5) and (ok6) then begin
            if (DateComp(fromdate,todate)>=0) then begin
               MessageBox('Your "From" date is later than your "Through" date, line '+strb(j,0)+'.', DP_FromLaterThenThrough);
               errorflag:=true;
               end
            else begin
               saveto:=todate;
               ninstallments:=NumberOfInstallments(fromdate,todate,peryr,on_or_before);
//               if (DateComp(saveto,todate)<>0) then PrintDate(j,tocol,defp,todate);
               end;
            end;
          ok7:=(amtnstatus>=defp);
          ok9:=(valnstatus>=defp);
          if (not ok6) then status:=0 {times per year necessary}
          else if (ok4) and (ok5) and ((ok7) or (ok9)) then status:=fully_specified
          else if (ninstallments>0) then status:=contains_unknown
               {the only unknown we're permitting is the amount.}
          else status:=MISSING_3; {won't calc}
          end; end;

{ ReadSimple (private)
  PURPOSE: (formerly) read the simple-vs-compound interest toggle off the screen.
  NOTE: now a no-op stub - the Delphi UI layer fills d^.simple before this is
        called, so the original direct screen read is commented out. }
{ Go port: n/a -- DOS screen input read; superseded by JSON request parsing in internal/api/handlers.go HandlePVCalc (line 1317) }
procedure ReadSimple;
          begin
// James sez:
// No reason to read the screen to get the d^.simple.  When this function is called
// the UI layer has already filled in the correct value.          
//          d^.simple:=(screen^[succ(ztop[PVLXBlock]),startof[simplecol],1]<>'C');
          end;

{ ValueOfPaymentSeries (private)
  PURPOSE: intended fast closed-form value of a payment series over [date1,date2]
           discounted to `asof` at a single rate.
  NOTE: UNIMPLEMENTED stub (empty body) - the "fast" path was never written;
        FancySummation always uses the slow exact per-payment walk instead. }
function ValueOfPaymentSeries(date1,date2,asof :daterec; rate :real; simple :boolean):real;
         begin
         end;

{ ComputeFancyLumpsumLineValues (private)
  PURPOSE: for each single-payment line, either compute its known direction
           (amount->value or value->amount) or flag it as the backward-solve
           line.
  SIDE EFFECTS: writes val0/amt0 and their statuses (outp); may set `backward`;
           may show a "two unknowns" error if more than one line is unknown.
  INTENT:  fully_specified lines: if amount given, value = ValueOfOnePayment;
           if value given, amount = value / ValueOfOnePayment(1). The ACTU
           ifdef folds in life-contingency probability when enabled. }
{ Go port: internal/finance/presentvalue/variablerate.go: solveVariableRateAmount (line 434) + VRDiscountFactor (line 124) -- per-lump value=amt*VRfactor or amount=value/VRfactor(1); classify via forwardVariableRate (line 273) }
procedure ComputeFancyLumpsumLineValues;
             var i,y          :byte;
          begin
          for i:=1 to nlines[PVLLumpSumBlock] do with aa[i]^ do begin
             if (status=contains_unknown) then begin
                 if (backward) then
                    MessageBox('Only one line should contain two unknowns in upper blocks.', DP_1Line2UnknownsUpperBlock)
                 else backward:=(d^.status=fully_specified);
                 end
              else if (status=fully_specified) then
                if (amt0status>=defp) then begin
                   val0:=ValueOfOnePayment(amt0,aa[i]^.date);
                   val0status:=outp;
{$ifdef ACTU}
                   if (fold_in_life) then val0:=val0*LifeProb(aa[i]^.date,aa[i]^.act0);
{$endif}
                   end
                else if (val0status>=defp) then begin
                   amt0:=val0/ValueOfOnePayment(1,aa[i]^.date);
                   amt0status:=outp;
{$ifdef ACTU}
                   if (fold_in_life) then amt0:=amt0/LifeProb(aa[i]^.date,aa[i]^.act0);
{$endif}
                   end;
              end;
          end;

{ ValueOfPastPayments (private)
  PURPOSE: (intended) value of the portion of periodic series j that falls
           before the as-of date, accreted forward to as-of.
  PARAMS:  j - periodic line index.
  RETURNS: valn (the accumulated value field).
  NOTE: part of the never-reached "fast" branch (FancySummation forces the exact
        path). The body is incomplete - it walks rate-table segments but its
        payment-summation block is stubbed out in a comment. Treat as legacy /
        not load-bearing. // TODO: verify logic }
{ Go port: n/a -- unused DOS fast-path stub (never reached); the port values every payment in vrPeriodicValue (variablerate.go:159) }
function ValueOfPastPayments(j :byte):real;
         var
           date1,date2,      {Start and stop dates of the periodic payments}
           date0, date3,     {Beginning and end dates of each partial period to be computed}
           coladate          {Date on which next cola escalation is due}
                            :daterec;
           paymentsum,years :real;
           k                :byte;
         begin with bb[j]^ do begin
         amtn:=0; date0:=fromdate; date1:=fromdate;
         k:=0;
         repeat inc(k) until (DateComp(cc[k]^.date,date0)>0) or (k>nlines[PVLPresValBlock]);
         dec(k);
         repeat
            if (DateComp(cc[k]^.date,todate)<0) then date2:=cc[k]^.date
            else date2:=todate;
            if (k<nlines[PVLPresValBlock]) and (DateComp(cc[succ(k)]^.date,d^.xasof)<0) then date3:=cc[succ(k)]^.date
            else date3:=d^.xasof;
            years:=YearsDif(date3,date0);
            if (d^.simple) then amtn:=amtn+ paymentsum*(cc[k]^.r.rate*years)
            else amtn:=amtn * exxp(cc[k]^.r.rate*years);
{ For improved speed, FILL THIS OUT WHEN YOU GET A CHANCE -----
            if (DateComp()>0) then begin
              valn:=valn+amtn*ValueOfPaymentSeries(date1,date2,d^.xasof,cc[k]^.r.rate,d^.simple);
              paymentsum:=paymentsum+NumberOfInstallments();
              end;
}
            date1:=date2;
            AddPeriod(date1,peryr,fromdate.d,add);
            inc(k);
         until (DateComp(date3,d^.xasof)=0);
         ValueOfPastPayments:=valn;
         end;end;

{ ValueOfFuturePayments (private)
  PURPOSE: (intended) value of the portion of periodic series j that falls on/
           after the as-of date, discounted back to as-of.
  PARAMS:  j - periodic line index.
  NOTE: essentially UNIMPLEMENTED (sets date0 only). Part of the unused "fast"
        path; the exact walk in FancySummation is what actually runs. }
{ Go port: n/a -- unused/unimplemented DOS fast-path stub; superseded by the exact walk in vrPeriodicValue (variablerate.go:159) }
function ValueOfFuturePayments(j :byte):real;
         var
           date0,date3,      {Beginning and end dates of each partial period to be computed}
           date1,date2,      {Start and stop dates of the periodic payments}
           coladate          {Date on which next cola escalation is due}
                            :daterec;
         begin with bb[j]^ do begin
         date0:=fromdate
         end;end;

{ FancySummation
  PURPOSE: present value (as of d^.xasof) of periodic series j, scaled to a unit
           payment amount, including COLA escalation and (optionally) life
           contingency. Multiply the result by amtn for the actual value, or
           divide valn by it to back out amtn.
  PARAMS:  j - periodic line index.
  RETURNS: the per-unit summed value.
  INTENT:  only the exact, period-by-period method is implemented (note the
           "if (true) or ..." guard): walk each payment date from fromdate to
           todate, apply the COLA multiplier, value it with ValueOfOnePayment,
           and accumulate, stopping when past todate or the term goes negligible
           (|part|<teeny). The "else" branch sketches a faster past/future split
           but is dead code (never reached). }
{ Go port: internal/finance/presentvalue/variablerate.go: vrPeriodicValue (line 159) -- per-unit PV of a periodic stream under the piecewise rate schedule with COLA; exact period-by-period walk }
function FancySummation(j :byte):real;
   {As with Summation in PRESVALU unit, this computation is scaled to an
    amount of unity, and should later be multiplied by amtn.  This is so
    it can also be used in backwards calculations of amtn from valn.
    Hence colamt always starts at 1.}
         var colamt,theresult            :real;
             movingdate,coladate      :daterec;
             savefrom,saveto          :daterec;
             part                     :real;
             ta                       :integer;
         begin with bb[j]^ do begin
         if (true) or (df.c.exact) or (fold_in_life) then begin
           {Only the slow, exact method is implemented so far, hence "if true"}
           InitializeColaData(bb[j]^,colamt,coladate);
           theresult:=0;
           movingdate:=fromdate;
//           if (todate.y=latest.y) then RequestPatience;
           repeat
             UpdateAmountWithCola(bb[j]^,colamt,movingdate,coladate);
{$ifdef ACTU}
             if (fold_in_life) then
               part:=ValueOfOnePayment(colamt,movingdate)*LifeProb(movingdate,bb[j]^.actn)
             else
{$endif}
               part:=ValueOfOnePayment(colamt,movingdate);
             theresult:=theresult+part;
             AddPeriod(movingdate,peryr,fromdate.d,add);
           until (DateComp(movingdate,todate)>0) or (abs(part)<teeny);
           end
         else begin
          {You should, sometime, write a faster version than the "Exact" version above
           now, this section is never reached because of "if true then else" above.}
           if (DateComp(fromdate,d^.xasof)>0) then valn:=ValueOfFuturePayments(j)
           else if (DateComp(todate,d^.xasof)<0) then valn:=ValueOfPastPayments(j)
           else begin
             saveto:=todate; todate:=d^.xasof;
             ta:=NumberOfInstallments(fromdate,todate,peryr,before);
             theresult:=ValueOfPastPayments(j); {past part only}
             savefrom:=fromdate;
             fromdate:=todate;
             todate:=saveto;
             AddPeriod(fromdate,peryr,savefrom.d,add);
             theresult:=theresult+ValueOfFuturePayments(j); {past part + future part}
             fromdate:=savefrom;
             end;
           end; {not exact} {this section not yet written}
         FancySummation:=theresult;
        end; {with bb} end;

{ ComputeFancyPeriodicLineValues (private)
  PURPOSE: for each periodic line, compute value from amount or amount from value
           (via FancySummation), or flag the single backward-solve line.
  SIDE EFFECTS: writes valn/amtn and statuses (outp); may set `backward`; shows
           a "two unknowns" error if more than one line is unknown.
  INTENT:  fully_specified lines: amount given -> valn = amtn*FancySummation;
           value given -> amtn = valn/FancySummation. }
procedure ComputeFancyPeriodicLineValues;
             var j    :byte;
          begin
          for j:=1 to nlines[PVLPeriodicBlock] do with bb[j]^ do begin
            if (status=contains_unknown) then begin
              if (backward) then
                 MessageBox('Only one line should contain two unknowns in Upper Right block.', DP_1Line2UnknownsTopRight)
              else if (d^.xvaluestatus=inp) then backward:=(d^.status=fully_specified);
              end
            else if (status=fully_specified) then
              if (amtnstatus>=defp) then begin
                 valn:=amtn*FancySummation(j);
                 valnstatus:=outp;
                end
              else if (valnstatus>=defp) then begin
                 amtn:=valn/FancySummation(j);
                 amtnstatus:=outp;
                end
            end; {for j}
          end;

(*
{ Go port: internal/finance/presentvalue/variablerate.go: vrPeriodicValue (line 159) + solveVariableRateAmount (line 434) -- valn=amtn*FancySummation or amtn=valn/FancySummation }
procedure ComputeFancyPeriodicLineValues;
          var j             :byte;
              saveto        :daterec;
          begin
          for j:=1 to nlines[PVLPeriodicBlock]) do with b[j]^ do begin
              if (not (ok6 and ok8)) then status:=0 {times per year and cola necessary}
              else if (ok4) and (ok5) and (ok7) and (ok9) then begin
                  MessageBox('To compute present value, enter DATES and AMOUNT but not VALUE in line '+strb(j,0));
                  status:=over_determined; {Should never happen}
                  end
              else if (ok4) and (ok5) and (ok7 or ok9) then status:=fully_specified
              else if ((ok4 or ok5) and (ok7))  or  (ok4 and ok5) then status:=contains_unknown
              else status:=EMPTY;
              if (status=contains_unknown) then
                 if (backward) and then
                    MessageBox('Only one line should contain two unknowns in Upper Right block.')
                 else if (c[1].status=fully_specified) then backward:=true;

              if (ok4) and (ok5) and (ok6) then if (k=1) then begin
                 saveto:=todate;
                 ninstallments:=NumberOfInstallments(fromdate,todate,peryr,on_or_before);
                 if (DateComp(saveto,todate)<>0) then PrintDate(pred(j)+ttop[2],tocol,def,todate);
                 end;
              end;

          if ok(c[k].r.rate) and dateok(c[k].asof) then with c[k] do
          for j:=1 to succ(bbot[2]-ttop[2]) do with b[j] do begin
              if (status=over_determined) then begin
                 MessageBox('You may specify only 2 of 3 columns in PAYMENT block, line '+strb(j,0));
                 end
              else if (ok4 and ok5 and ok6 and ok8) then begin
                   if (ok7) then begin
                      valn:=amtn*Summation(k,j);
                      if (k=1) then Print(pred(j)+ttop[1],pvaluecol,outp,valn);
                      end
                   else if (ok9) then begin
                      amtn:=valn/Summation(k,j);
                      if (k=1) then Print(pred(j)+ttop[1],pamountcol,outp,amtn);
                      end;
                   end;
               end;
          end;
*)

{ ComputeGrandTotal (private)
  PURPOSE: sum all computed line values into the screen total d^.xvalue.
  SIDE EFFECTS: sets d^.xvalue to the sum of all lump-sum val0 plus all periodic
           valn (plus the pay-on-death value podval when life-contingency is
           folded in), and marks d^.xvaluestatus as outp (computed output). }
{ Go port: internal/finance/presentvalue/variablerate.go: forwardVariableRate (line 273) -- total is PVResult.SumValue summed over all rows }
procedure ComputeGrandTotal;
          var i,j :byte;
          begin
          d^.xvalue:=0;
          for i:=1 to nlines[PVLLumpSumBlock] do d^.xvalue:=d^.xvalue+aa[i]^.val0;
          for j:=1 to nlines[PVLPeriodicBlock] do d^.xvalue:=d^.xvalue+bb[j]^.valn;
          if (fold_in_life) then d^.xvalue:=d^.xvalue+podval;
          d^.xvaluestatus:=outp;
          end;

{ FancyFirstPass
  PURPOSE: the entry point that validates and classifies the entire fancy PV
           screen, then drives the forward (or sets up the backward) calc.
  SIDE EFFECTS: sets per-line and screen statuses; may set errorflag and show
           messages; deletes EMPTY lines; computes line values and (when the
           screen is fully forward-determined) the grand total.
  INTENT / STRUCTURE (block numbering matches the screen regions):
    BLOCK 4 (d^): require the rate list, the X block, and the as-of date; derive
            the screen status from xasof/xvalue presence.
    BLOCK 3 (cc[] rate list): every row needs a rate and a date; dates must be
            strictly increasing (else error).
    BLOCK 1 (aa[] lump sums): classify each, drop EMPTY rows.
    BLOCK 2 (bb[] periodics): classify each, drop EMPTY rows.
    Then compute lump-sum and periodic line values; decide `frontward` (all
    relevant lines fully specified, none left unknown); if frontward, either
    warn that the lower-right value is already determined (error) or compute the
    grand total. The optional ACTU branch handles life-contingency. }
{ Go port: internal/finance/presentvalue/variablerate.go: forwardVariableRate (line 273) -- classify rows against the piecewise rate table; unknown detection in vrUnknownAmount (line 407)/vrUnknownDate (line 542) }
procedure FancyFirstPass;
          var i,j                     :byte;

          begin {FancyFirstPass}
          {BLOCK 4}
          with d^ do begin
            if (nlines[PVLPresValBlock]=0) or (nlines[PVLXBlock]=0) or (xasofstatus<>inp) then begin
               errorflag:=true;
               exit; end;
            status:=fully_specified;
            if (xasofstatus<defp) then dec(status);
            if (xvaluestatus<defp) then dec(status);
            end;
          ReadSimple;

          {BLOCK 3}
          for i:=1 to nlines[PVLRatesBlock] do with cc[i]^ do begin
             if (r.status<=empty) or (datestatus<=empty) then errorflag:=true
             else if (i>1) and (DateComp(date,cc[pred(i)]^.date)<=0) then begin
               MessageBox('Your dates are out of order in Rate List Block, lines '+strb(pred(i),0)+' and '+strb(i,0)+'.', DP_OutOfOrderRateList);
               errorflag:=true;
               end;
             status:=missing_2;
             if (r.status>=defp) then inc(status);
             if (datestatus>=defp) then inc(status);
             end;
          backward:=false;


          {BLOCK 1}
          i:=1;
          while (i<=nlines[PVLLumpSumBlock]) do begin
             DetermineStatus1(i);
             if (aa[i]^.status=EMPTY) then begin
               ZeroLumpSum( lumpsumptr(aa[i]) );
               dec( nlines[PVLLumpSumBlock] );
//                DeleteRowOfBlock(PVLLumpSumBlock,i-scrollpos[PVLLumpSumBlock])
             end else inc(i);
             end;

          {BLOCK 2}
          j:=1;
          while (j<=nlines[PVLPeriodicBlock]) do begin
             DetermineStatus2(j);
             if (bb[j]^.status=EMPTY) then begin
               ZeroPeriodic( periodicptr(bb[j]) );
               dec( nlines[PVLPeriodicBlock] );
//                DeleteRowOfBlock(PVLPeriodicBlock,i-scrollpos[PVLPeriodicBlock])
             end else inc(j);
             end;

          if (errorflag) then exit;

          ComputeFancyLumpsumLineValues;
          ComputeFancyPeriodicLineValues;

          frontward:=false;
          for i:=1 to nlines[PVLLumpSumBlock] do
              if (aa[i]^.status=fully_specified) then frontward:=true;
          for j:=1 to nlines[PVLPeriodicBlock] do
              if (bb[j]^.status=fully_specified) then frontward:=true;
         for i := 1 to nlines[PVLlumpsumblock] do
           if (aa[i]^.status <= contains_unknown) and (aa[i]^.status > empty) then
             frontward := false;
         for j := 1 to nlines[PVLperiodicblock] do
           if (bb[j]^.status <= contains_unknown) and (bb[j]^.status > empty) then
             frontward := false;

          if (frontward) then begin
{$ifdef ACTU}
          if (not (fold_in_life and podunk)) then
{$else}
          if (true) then  // false ANDed with anything is false, fold_in_life=false
{$endif}
             if (d^.xasofstatus=inp) and (d^.xvaluestatus=inp) then begin
               MessageBox('Warning: value entered at lower right is already determined by data above.', DP_RedeterminedValue);
               errorflag:=true; exit;
               end
             else ComputeGrandTotal;
             end;

{$ifdef BUGSIN}
          if (pvlfancy) then
            if (d^.simple) then
               for i:=1 to nlines[PVLRatesBlock] do
                 if (datecomp(cc[i]^.date,d^.xasof)=0) then Scavenger('C-4');
{$endif}
          end; {FancyFirstPass}

{ Unit initialization: register this unit's PVLPlainFancy with PEDATA so other
  units can toggle the plain/fancy PV screen through the shared procedure hook
  (avoids a direct circular dependency on PVLXSCRN). }
begin
PEDATA.PVLPlainFancy:=PVLPlainFancy;
end.
