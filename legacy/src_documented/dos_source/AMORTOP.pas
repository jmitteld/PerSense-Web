unit AMORTOP;
{$ifdef OVERLAYS} {$F+,O+} {$endif}

{ ============================================================================
  UNIT AMORTOP  --  Amortization "operations" / engine core for Per%Sense.

  This is the low-level computational engine that backs the Amortization
  screen (driven by unit AMORTIZE).  Where AMORTIZE handles field-presence
  dispatch (deciding WHICH unknown to solve for), AMORTOP owns the actual
  period-by-period repayment walk, the Newton-style solver (Iterate), and
  all the "advanced option" handling: balloons, periodic prepayments, rate
  adjustments (ARMs / Re_Amortize), moratorium (interest-only deferral),
  principal-reduction target, and skip-months.

  Key shared state (declared in the var blocks below, used across both units):
    h^        : the loan header record (amount, dates, rate, payment, n, etc.)
    d         : the regular periodic payment amount currently in play
    p         : running principal balance during a repayment walk
    usap      : USA-rule "exempt" portion of principal (unpaid-interest bucket)
    f         : growth factor per period (1 + rate/periods); see GrowthPerPeriod
    very_last : the latest payment date in the whole schedule (incl. balloons)
    repay_from: date amortization effectively begins (prepaid/moratorium aware)
    nballoons / npre / nadj : counts of active balloons / prepayments / adjustments
    payment, nextpayment : the two paymenttype objects that carry one row's
                           computed values; ComputeNext advances NextPayment.

  Status convention (from PETYPES): empty=0, outp=1 (computed), defp=2
  (defaulted/known), inp=3 (hard user input).  ">=defp" therefore means
  "known" (user-entered or defaulted); ">defp" means a hard input cell.

  Rounding: Round2 is round-HALF-DOWN (truncate at exactly .5), applied only
  when hard_payment is set (the "Dav Holle" standard-penny treatment).
  ============================================================================ }

INTERFACE

//uses OPCRT,VIDEODAT,NORTHWND,INPUT,PETYPES,PEDATA,LOTUS,INTSUTIL,PEPANE,IOUNIT,COMDUTIL,KCOMMAND,TABLE,AMZUTIL
//{$ifdef TOPMENUS} ,MCMENU {$endif}
//{$ifdef TESTING} ,TESTHIGH {$endif}
//     ;
uses VIDEODAT,PETYPES,PEDATA,INTSUTIL, Globals, Classes;

       const
    minpmt = 1.0;          { threshold balance (one dollar) below which the loan
                             is treated as paid off; also min "meaningful" payment }
    FR_BALLOON = 1;         { xsource bit flag: an extra payment came FRom a BALLOON }
    bright = true;
    dim = false;
    value_calc = true;      { RepayFancyLoan flag: accumulate discounted cashflows
                             into aprvalue (used by APR/points solver) }
    no_value_calc = false;
    entire = true;          { RepayFancyLoan flag: walk the WHOLE schedule (for
                             printing/final), vs. stopping at the next adjustment }
    til_adj = false;        { ...stop early, only amortize up to the next rate adj }
{$ifdef MAC}
    linesperpage = 58;
    lineacross = '\-';
{$else}
    lineacross = '--------------------------------------------------------------------------------';
{$endif}

  type
    { Snapshot of the mutable "advanced option" cursor state so a trial
      repayment walk (e.g. inside Iterate) can be run and then rolled back.
      The prepayment array is deep-copied on the heap because ComputeNext /
      CheckOffBalloon mutate pre[i]^ in place (advancing nextdate, deleting
      exhausted series). }
    saved_balloon_state = object
        save_next_balloon, save_npre, save_next_adj: integer;
        saved: real;                                  { saved copy of d (payment) }
        save_pre: array[1..maxprepay] of ^prepaymentrec;
        procedure Free;
        procedure Restore;
        procedure Save;
      end;

    { One amortization row in progress.  Carries everything needed to emit a
      printed line and to advance to the next period.
        base_date : the last REGULAR payment date (drives AddPeriod for next date)
        date      : the current payment date (may be a balloon, off the regular grid)
        prevdate  : last payment date of any kind (drives interest accrual interval)
        payamt    : total cash paid this period (regular +/- balloon/prepay/target)
        interest  : interest portion accrued since prevdate
        principal : remaining principal balance AFTER this payment
        usaprinc  : USA-rule exempt principal bucket after this payment
        paynum    : 1-based payment counter (-1 used for the settlement-day prepaid row) }
    paymenttype = object
        base_date, date, prevdate: daterec;
        payamt, interest, principal, usaprinc, nextpayamt: real;
        paynum :integer;
        procedure ComputeNext (var p, usap: real);
{        procedure ComputeNextForAdvancedInterestPayment (var p: real);  No more ADV with FANCY loans}
        procedure Init (bdate, pdate: daterec; npayamt: real);
      end;

                   {-------- ------------- -------------- ------------- ------------- --------------}
  const  fancyheader='  DATE      AMT of PMT  PRINC BALANCE PRINC THIS PD   INT THIS PD    INT TO DATE';

    indent:ch2 = '  ';
                   {-------- ------------- -------------- ------------- ------------- --------------}
fancyheader1:str80='ÃÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÁÄÄÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÄÄÄ´';
{
fancyheader2:str80='³              Payment      Principal     Principal      Interest      Interest³';
fancyheader3:str80='³   Date        Amount        Balance   This Period   This Period       To Date³';
}
fancyheader2:str80='³              Payment      Interest     Principal      Principal      Interest³';
fancyheader3:str80='³   Date        Amount   This Period   This Period    New Balance       To Date³';
     header1:str80='ÃÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄ´';
{
     header2:str80='³                Outstanding        Principal       Interest       Interest    ³';
     header3:str80='³ Date             Principal      This Period    This Period        To Date    ³';
}
     header2:str80='³                     Interest       Principal       Principal        Interest ³';
     header3:str80='³ ##      Date     This Period     This Period     New Balance         To Date ³';
     header4:str80='ÀÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÙ';
     r78text:string[23]='Rule of 78 Amortization';

    noadvancedmsg:str80 = 'Sorry - you can''t change rates when interest is computed in advance.';

  { ---- Engine-wide working variables (shared with AMORTIZE) ----
    f         : growth factor per period (1 + periodic rate)
    f_1       : f-1 (the periodic interest fraction), precomputed for speed
    p         : running principal balance during a walk
    usap      : USA-rule exempt-principal bucket
    d         : current regular payment amount
    int_to_date,cumint,cumamt : running interest/payment totals for output
    prorate   : fractional first-period length (odd first period factor)
    r78base   : Rule-of-78 base interest unit per period
    nrepay    : number of periods over which principal is actually amortized
    t,nextt,paidthru,very_last : working dates (current/next row, last accrual, end) }
  var
    f, f_1, p, usap, d, int_to_date, prorate, cumint, cumamt, r78base: real;
    nrepay, count, lines: integer;
    t, paidthru, nextt, very_last: daterec;
    more_dates_than_lines,balance_calc,hard_payment: boolean;
    ws: str80;
    old_pre: prepaymentarray;          { saved prepayment state for Re_Amortize rollback }
    old_npre, old_next_balloon: integer;

  var
    aprvalue, v_rate, truerate: real;  { aprvalue: accumulated PV for APR solve;
                                         v_rate: trial discount rate; truerate:
                                         effective rate for DAILY compounding }
    repay_from: daterec;               { date amortization effectively begins }
    { Counts/indices for the advanced options:
        nballoons   : active balloons; next_balloon: 1-based walk cursor
        unkballoon  : index of the one balloon whose amount is to be SOLVED (0=none)
        nadj        : active rate adjustments; next_adj: walk cursor
        user_nballoons : balloons the user actually typed (excludes auto-tacked terminal)
        npre        : active prepayment series; unkpre: series to solve (0=none, 3=both)
      }
    nballoons, next_balloon, unkballoon, nadj, user_nballoons, next_adj, npre, unkpre: integer;
    ki: char;
    prepaid, adj_fully_specified: boolean;  { prepaid: interest paid at settlement;
                                              adj_fully_specified: all ARM rows known }

    balloonsok, adjok, preok: boolean; {lastok summarizes lastdate and n}

    payment, nextpayment: paymenttype;  { the two rows; ComputeNext advances nextpayment }
    skipmonthset: byteset;              { set of month numbers (1..12) with no payment }

 {templates}
//  procedure AmortizationScreen;
  procedure CheckPrepayments;                  { classify each prepayment series; set npre/unkpre/preok }
  procedure ComputeTrueRate;                   { derive truerate for DAILY compounding from loanrate }
  procedure DecideWhetherToPrintALine (t, nextt: daterec; Output: TStringList; bCommaSeperated: boolean );  { accumulate + maybe emit one row, triggering Re_Amortize at adj dates }
  procedure DetermineVeryLast;                 { set very_last = latest of lastdate / balloons / prepay stops }
  function DetermineLastPaymentDate (p, usap: real): boolean;  { solve for last date / #periods given a payment }
  function GrowthPerPeriod: real;              { 1 + periodic rate (weekly/biweekly use 7/14-day basis) }
  function Iterate (p, usap: real; loandate, firstdate: daterec; var x: real; entire_or_no: boolean): boolean;  { Newton refine of x (payment/balloon/rate) so terminal balance ~= 0 }
//  procedure LineAcrossSpreadsheet;
//  procedure NewPage;
  function PrepaidInterest: real;              { settlement-date prepaid interest (0 if not prepaid) }
  procedure PrintGrandTotals( Output: TStringList; bCommaSeperated: boolean );  { emit Total/Principal/Interest footer }
  procedure PrintSummaryLine(Output: TStringList; bCommaSeperated: boolean );   { emit a subtotal line and reset cumulators }
  function R78Header1:str80;                   { header line centered with "Rule of 78" caption }
{procedure Re_Amortize (var p: real; with_print: boolean); }
  procedure RepayLoan (var p: real);          { fast closed-form repay walk for the PLAIN (non-fancy) loan }
  procedure ReportFinalPayment( Output: TStringList );  { emit the adjusted final-payment amount }
  procedure SortBalloons (maxballoons: shortint);  { sort/merge balloons by date; detect the unknown balloon }
  procedure SortAdj (maxadj: shortint);  {Not integer! you take pred(0) at one point}  { sort rate adjustments by date; validate }
  procedure RepayFancyLoan(var p,usapart:real; loandate,firstdate:daterec; Output: TStringList; bCommaSeperated: boolean; entire,value_calc:boolean; adjnum:byte);  { full advanced-option period-by-period walk }
//  procedure WriteOneLotusLine (var amt, int: real);
  function template: str12;                    { template filename for fancy vs plain output }
  function TimeForSummary (t, nextt: daterec): boolean;  { is a subtotal due between t and nextt? }
//  function TwoLinesOut (first: boolean): boolean;
  function VeryLastRegularAmount: real;        { regular payment amount falling on very_last (for plus_regular) }

implementation

uses Amortize, HelpsystemUnit;

  { template -- returns the printer template filename to use.
    Fancy (advanced-option) schedules use a different column layout than the
    plain amortization table, so a different template file is selected. }
  { Go port: n/a -- printer template filename selection; the web layer owns
    output formatting. }
  function template: str12;
  begin
    if fancy then
      template := 'TEMPLATE.%X'
    else
      template := 'TEMPLATE.%A';
  end;

  { saved_balloon_state.Save -- snapshot the mutable advanced-option cursor
    state (balloon/prepay/adj walk indices and a deep heap copy of every
    active prepayment record) plus the current payment d, so a speculative
    repayment walk can later be undone with Restore.  Sets errorflag on OOM. }
  { Go port: dosport_walk.go: saveState (line 38) -- captures the same balloon/
    prepay/adj cursor + payment into a dpSavedState value; Go uses value copies
    rather than heap-allocated prepayment records. }
  procedure saved_balloon_state.Save;
    var
      i : byte;
  begin
    for i := 1 to maxprepay do
      save_pre[i] := nil;
    save_next_balloon := next_balloon;
    save_npre := npre;
    save_next_adj := next_adj;
    saved := d;
    for i := 1 to npre do begin
       if not GetMemIfAvailable(pointer(save_pre[i]),sizeof(prepaymentrec)) then errorflag:=true;
       save_pre[i]^ := pre[i]^;
       end;
  end;

  { saved_balloon_state.Free -- release the heap copies made by Save.
    Must be called exactly once after Save (even after Restore). }
  { Go port: n/a -- Go's dpSavedState is a value type reclaimed by GC, so there
    is no explicit Free counterpart to dosport_walk.go: saveState (line 38). }
  procedure saved_balloon_state.Free;
  var i :byte;
  begin
    for i := 1 to maxprepay do
      if (save_pre[i]<>nil) then begin
         FreeMem(save_pre[i],sizeof(prepaymentrec));
         save_pre[i]:=nil;
         end;
  end;

  { saved_balloon_state.Restore -- roll the cursor state and prepayment
    records back to the values captured by Save (also resets usap to 0).
    The heap copies are NOT freed here; call Free separately. }
  { Go port: dosport_walk.go: restoreState (line 49) -- rolls the cursor state +
    prepayment records back to the saveState snapshot (and resets usap to 0). }
  procedure saved_balloon_state.Restore;
    var
      i: byte;
  begin
    next_balloon := save_next_balloon;
    npre := save_npre;
    next_adj := save_next_adj;
    d := saved;
    for i := 1 to npre do
      pre[i]^:=save_pre[i]^;
    usap := 0;
  end;

  { PrepaidInterest -- interest collected up front at settlement (loan date).
    Returns 0 unless prepaid is set.  For interest-in-advance the prepaid
    amount is one full period (loandate..firstdate).  Otherwise it is the
    interest accrued from loandate up to one period before firstdate; for
    DAILY compounding this uses the exact compounded truerate, else simple
    interest at loanrate. }
  { Go port: engine.go: PrepaidInterest (line 56) -- same settlement-date prepaid
    interest (0 if not prepaid; one full period for in-advance; else accrual to
    one period before firstdate, compounded truerate for DAILY). }
  function PrepaidInterest: real;
    var
      t: daterec;
  begin
    with h^ do
      begin
        if not prepaid then
          PrepaidInterest := 0
        else
          begin
            if (df.c.in_advance) then
              PrepaidInterest := amount * loanrate * (YearsDif(firstdate, loandate))
            else
              begin
                t := firstdate;
                AddPeriod(t, peryr, firstdate.d, subtract);
                if (df.c.peryr=DAILY) then
                  PrepaidInterest := amount * (exxp(truerate * (YearsDif(t, loandate))) - 1)
                else 
                  PrepaidInterest := amount * loanrate * (YearsDif(t, loandate));
              end;
          end;
      end;
  end;

  { PromptForBalloonSettingsChange -- ask the user whether the "Balloon
    includes regular payment" setting should flip.  yesno=true offers to set
    it to "YES" (so plus_regular becomes false), and vice versa.  Honors a
    Cancel that leaves the setting untouched. }
  { Go port: n/a -- interactive DOS prompt to flip a setting; the web port takes
    the plus_regular setting as given from the request. }
  procedure PromptForBalloonSettingsChange (yesno: boolean);
    var
      ws: string[3];
      kiword: word;
      CancelPressed: boolean;
  begin
    if (yesno) then
      ws := 'YES'
    else
      ws := 'NO';
    MessageBoxWithCancel('Do you want to change "Balloon includes regular pmt" setting to "'+ ws + '"? ', CancelPressed, DA_SetBalloonIncPmt );
    if( not CancelPressed ) then
      df.c.plus_regular := not yesno;
  end;

  { CheckBalloonSetting -- heuristic that nudges the user about the
    "Balloon includes regular pmt" setting when a balloon falls ON a regular
    payment date.  If plus_regular is on but this balloon's amount is 0, or
    if it's off but the balloon equals the regular payment, the setting is
    probably wrong; offer to flip it. }
  { Go port: n/a -- UI heuristic that nudges the user about the "Balloon includes
    regular pmt" setting; no equivalent in the stateless Go service. }
  procedure CheckBalloonSetting (payment: real);
  begin
    if (df.c.plus_regular) then
      begin
        if (payment = 0) then
          PromptForBalloonSettingsChange(true);
      end
    else
      if (abs(payment - h^.payamt) < 0.02) and (h^.payamtstatus >= defp) then
         PromptForBalloonSettingsChange(false);
  end;

  { SortBalloons -- bubble-sort the balloon[] pointer array into ascending
    date order, merging any two balloons that land on the same date.  Sets
    nballoons (count of non-empty entries), validates that no balloon precedes
    the first regular payment, and detects the single "unknown" balloon (one
    whose amount XOR date is blank => unkballoon) to be solved later.
    maxballoons is the number of on-screen balloon rows to consider. }
  { Go port: engine.go: SortBalloons (line 85) -- sorts balloons by date; the
    same-date merge and the unknown-balloon detection (unkballoon) are handled
    in firstpass.go: FirstPass (line 43) when classifying the balloon rows. }
  procedure SortBalloons (maxballoons: shortint);
    var
      i, j, order,cursortoi,cursorfromi: shortint;
      again,redisplay: boolean;

    { Swap -- exchange balloon pointers i and j, keeping unkballoon/cursor
      index references pointing at the same logical balloon. }
    { Go port: engine.go: SortBalloons (line 85) uses Go's sort with slice
      element swaps; unkballoon index tracking is done in FirstPass. }
    procedure Swap;
      var
        temp: ^balloonrec;
    begin
      temp := pointer(balloon[i]);
      balloon[i] := balloon[j];
      balloon[j] := pointer(temp);
      if (unkballoon = i) then
        unkballoon := j
      else if (unkballoon = j) then
        unkballoon := i;
      if (cursortoi=j) then cursortoi := i
      else if (cursortoi=i) then cursortoi := j;
      redisplay := true;
    end;

{ Merge -- combine balloon j into balloon i (same date): add amounts, then
  shift every later balloon down one slot and blank the vacated last slot.
  Sets `again` so the outer sort re-scans after the array shifts. }
{ Go port: engine.go: SortBalloons (line 85) -- same-date balloon amounts are
  summed when merging coincident balloons (Go compacts the slice). }
procedure Merge;
          var k :byte;
          begin
          again := true;
          balloon[i]^.amount := balloon[i]^.amount+balloon[j]^.amount;
          for k := j to pred(maxballoons) do begin
              balloon[k]^:=balloon[succ(k)]^;
//              if (not scripting) then
//                  MoveScreen(screen^[ztop[AMZBalloonBlock]+succ(k),lleft[AMZBalloonBlock]],
//                             screen^[ztop[AMZBalloonBlock]+k,lleft[AMZBalloonBlock]],
//                             succ(rright[AMZBalloonBlock]-lleft[AMZBalloonBlock]));
              end;
          ZeroBalloon( balloonptr(balloon[k]) );
//          DeleteRowOfBlock(AMZballoonBlock,nlines[AMZballoonblock]-scrollpos[AMZBalloonblock]);
          end;

  begin  {SortBalloons}
    redisplay := false;
//    if (col<fcol[AMZBalloonBlock]) or (col>lcol[AMZBalloonBlock]) then cursortoi := 0
//    else cursortoi := whereyabs-ztop[AMZBalloonBlock];
    cursorfromi := cursortoi;
    balloonsok := true;
    unkballoon := 0;
    { O(n^2) bubble sort with in-pass merge; repeat until a pass makes no
      change (Merge sets `again` because it compacts the array). }
    repeat
      again := false;
      for i := 1 to pred(maxballoons) do
        for j := succ(i) to maxballoons do
          begin
            order := DateComp(balloon[i]^.date, balloon[j]^.date);
            if (order > 0) then
              Swap
            else if (balloon[i]^.datestatus>empty) and (order = 0) then
              Merge;
          end;
    until not again;
    { count contiguous non-empty balloons (sort pushed all blanks to the end) }
    nballoons := 0;
    while (nballoons < nlines[AMZBalloonBlock]) and (balloon[succ(nballoons)]^.datestatus>empty) do
      inc(nballoons);
    if (nballoons > 0) then
      if (h^.firststatus>defp) and (DateComp(balloon[1]^.date, h^.firstdate) < 0) then
        begin
          MessageBox('Balloon cannot precede first regular payment.', DA_BalloonPrecedeFirstPay);
          errorflag := true;
        end;
    for i := 1 to nballoons do
      begin
        if (balloon[i]^.date.d = h^.firstdate.d) then
          CheckBalloonSetting(balloon[i]^.amount);
                { \ This works only for peryr=12 - not as smart as it could be.}
        { A balloon with exactly one of amount/date known is the unknown to
          solve for.  If more than one such balloon exists, balloonsok stays
          false and unkballoon is cleared (can't solve two at once). }
        if ((balloon[i]^.amountstatus >= defp) xor (balloon[i]^.datestatus >= defp)) then
          begin
            if (balloonsok) then
              unkballoon := i
            else
              unkballoon := 0;
            balloonsok := false;
          end;
      end;
//      if (redisplay) then begin
//         DisplayBlock(AMZBalloonBlock);
//         if (cursortoi<>cursorfromi) then MoveToCell(col,cursortoi+ztop[AMZBalloonBlock],0);
//         end;
  end; {SortBalloons}

  { SortAdj -- sort the rate-adjustment (ARM) rows adj[] into ascending date
    order and validate them.  Rejects duplicate adjustment dates, adjustments
    before the loan date, and adjustments on/after the last payment.  Sets
    nadj (count) and adj_fully_specified (every adj row has date+rate+amount
    known, OR there are no adjustments and the top-line payment is known --
    which together with the unknown-cell logic decides whether the schedule is
    solvable).  maxadj is the on-screen adjustment row count. }
  { Go port: engine.go: SortAdjustments (line 93) -- sorts ARM adjustments by
    date; the duplicate-date / before-loan / after-last validation and the
    adj_fully_specified flag are computed in firstpass.go: FirstPass (line 43)
    and validate.go: ValidateInputs (line 42). }
  procedure SortAdj (maxadj: shortint);
    var
      i, j, cursortoi,cursorfromi: shortint;
      amtok, whenok, aprok, allamtok, allwhenok, allaprok: boolean;

    { Swap -- exchange adjustment pointers i and j (and, in the original, the
      on-screen rows). }
    procedure Swap;
              var temp              :^adjrec;
                  ilineptr,jlineptr :pointer;
                  buf               :array[1..80] of char;
              begin
              temp := pointer(adj[i]);
              adj[i]:=adj[j];
              adj[j]:=pointer(temp);
              {now swap them on the screen}
{              if (not scripting) then begin
                 ilineptr := ptr(seg(screen^),160*(ztop[AMZAdjBlock]+pred(i)) + 2*pred(lleft[AMZAdjBlock]));
                 jlineptr:=ptr(seg(screen^),160*(ztop[AMZAdjBlock]+pred(j)) + 2*pred(lleft[AMZAdjBlock]));
                 MoveScreen(ilineptr^,buf,succ(rright[AMZAdjBlock]-lleft[AMZAdjBlock]));
                 MoveScreen(jlineptr^,ilineptr^,succ(rright[AMZAdjBlock]-lleft[AMZAdjBlock]));
                 MoveScreen(buf,jlineptr^,succ(rright[AMZAdjBlock]-lleft[AMZAdjBlock]));
                 if (cursortoi=j) then cursortoi:=i
                 else if (cursortoi=i) then cursortoi:=j;
                 end;
                 }
{$ifdef BUGSIN}
              Scavenger('I-9');
{$endif}
              end;

  begin {SortAdj}
//    if (col<fcol[AMZAdjBlock]) or (col>lcol[AMZAdjBlock]) then cursortoi:=0
//    else cursortoi:=whereyabs-ztop[AMZAdjBlock];
//    cursorfromi:=cursortoi;
    adjok := true;
    for i := 1 to pred(maxadj) do
      for j := succ(i) to maxadj do
        if (DateComp(adj[i]^.date, adj[j]^.date) > 0) then
          Swap
        else if (adj[i]^.datestatus>empty) then
          if (DateComp(adj[i]^.date, adj[j]^.date) = 0) then
            begin
              MessageBox('You can''t adjust the interest rate twice on the same date.', DA_2RateAdjustsPerDay);
              errorflag := true;
            end;
    nadj := 0;
    while (nadj < maxadj) and (adj[succ(nadj)]^.datestatus>empty) do
      inc(nadj);
    allaprok := true;
    allwhenok := true;
    allamtok := true;
    for i := 1 to nadj do
      begin
        amtok := (adj[i]^.amountstatus >= defp);
        aprok := (adj[i]^.loanratestatus >= defp);
        whenok := (adj[i]^.datestatus >= defp);
(*
    This business of defaulting the loan rate shouldn't be in the calc
    section.  It should be in PEPANE, when a new line is created, if I
    decide to retain it at all.
        if (whenok) and (amtok) and (not aprok) then
          begin {default to previous loan rate}
            if (i = 1) then
              adj[i]^.loanrate := h^.loanrate
            else
              adj[i]^.loanrate := adj[pred(i)]^.loanrate;
            if ok(adj[i]^.loanrate) then adj[i]^.loanratestatus:=defp;
            aprok := true;
          end;
*)
        allaprok := allaprok and aprok;
        allwhenok := allwhenok and whenok;
        allamtok := allamtok and amtok;
      end;
    if (nadj > 0) then
      begin
        if (h^.loandatestatus>empty) and (DateComp(adj[1]^.date, h^.loandate) <= 0) then
          begin
            MessageBox('Rate change cannot precede date of loan.', DA_RateChangePrecedeDate);
            errorflag := true;
          end;
        if (h^.loandatestatus>empty) and (DateComp(adj[nadj]^.date, h^.lastdate) >= 0) then
          begin
            MessageBox('You can''t change rate or payment amount after the last regular payment.', DA_RateChangeAfterPay);
            errorflag := true;
          end;
        adj_fully_specified := allaprok and allwhenok and allamtok;
      end
    else
      adj_fully_specified := (h^.payamtstatus >= defp);
//    if (cursortoi<>cursorfromi) then MoveToCell(col,cursortoi+ztop[AMZAdjBlock],0);
  end;

  { CheckPrepayments -- classify each extra-payment ("prepayment") series.
    A series is defined by startdate, periods-per-year, payment amount, and
    either a stop date OR a count (nn).  Given start+freq, if nn is known the
    stop date is derived (AddNPeriods); if the stop date is known the count is
    derived (NumberOfInstallments).  Per series the locals mean:
      ok1=startdate known, ok2=freq known(&>0), ok3=count known, okp=payment known.
    Outputs: npre (count of non-blank series), preok (true if all series are
    fully determined), and unkpre (index of the single under-determined series
    to solve; the special value 3 signals "both series need a duration"). }
  { Go port: firstpass.go: FirstPass (line 43) -- classifies each prepayment
    series (deriving stop date from count or count from stop date), and sets the
    npre / preok / unkpre equivalents used to route the prepayment-amount and
    prepayment-duration solves. }
  procedure CheckPrepayments;
    var
      i: integer;
      ok1, ok2, ok3, okp: boolean;
      blank: array[1..maxprepay] of boolean;
      pre_needing_n: integer;
      savestop: daterec;
  begin
    preok := true;
    unkpre := 0;
    pre_needing_n := 0;
    for i := 1 to nlines[AMZpreblock] do
      with pre[i]^ do
        begin
          ok1 := (startdatestatus >= defp);
          ok2 := (peryrstatus >= defp) and (pre[i]^.peryr > 0);
          ok3 := (nnstatus >= defp);
          if (ok1 and ok2) then
            begin
              if (ok3) then
                begin
                  AddNPeriods(startdate, stopdate, pre[i]^.peryr, pred(nn));
                  stopdatestatus := outp;
                end
              else if (stopdatestatus >= defp) then
                begin
                  ok3 := true;
                  savestop := stopdate;
                  nn := NumberOfInstallments(startdate, stopdate, peryr, ON_OR_BEFORE);
                  nnstatus := outp;
                  if (DateComp(stopdate, savestop) <> 0) then
                    stopdatestatus := defp;
                end
              else
                pre[i]^.stopdate.m := unkbyte;
            end;
          okp := (paymentstatus >= defp);
          if (ok1 or ok2 or ok3 or okp) then
            begin
              blank[i] := false;
              nextdate := startdate;
              if (ok1 and ok2) then
                begin
                  if not (okp or ok3) then
                    preok := false
                  else if (okp xor ok3) then
                    begin
                      if (okp) and (not ok3) then
                        pre_needing_n := (pre_needing_n or i);
                      if (preok) then
                        unkpre := i
                      else
                        unkpre := 0;
                      preok := false;
                    end;
                end
                {else if (ok1 or ok2) then unkpre:=0 WHY? creates RT error if just date specified.}
              else
                preok := false
            end
          else
            blank[i] := true;
        end;
    npre := 0;
    for i := 1 to nlines[AMZpreblock] do
      if not (blank[i]) then
        inc(npre);
{for i := firstdatecol to paymentcol do gpeAMZpane.ClearCell(AMZtopblock, 1, i);  What for?  removed JJM 12/23/90}
    if (pre_needing_n = 3) and (h^.laststatus < defp) then
      unkpre := pre_needing_n;
             {unkpre=3 is a signal that duration is needed for both extra payment series.}
    for i := 1 to npre do
      if (pre[i]^.startdate.d = h^.firstdate.d) then
        CheckBalloonSetting(pre[i]^.payment);
                                { \ This works only for peryr=12 - not as smart as it could be.}
  end;

{procedure FirstPass; (GetNumbers) used to be here }

  { Paymenttype.Init -- seed a payment object before the walk begins.
    bdate  -> base_date (last regular pmt date, drives the next-date step)
    pdate  -> prevdate  (last accrual date, drives the interest interval)
    npayamt-> nextpayamt (the first payment amount, e.g. an in-advance prepay) }
  { Go port: dosport.go: dpPayment.init (line 74) -- seeds base_date/prevdate on
    the Go payment struct before the walk (npayamt handling is inline). }
  procedure Paymenttype.Init (bdate, pdate: daterec; npayamt: real);
  begin
    base_date := bdate;
    prevdate := pdate;
    nextpayamt := npayamt;
  end;

  { FindNextExtra -- determine the next "extra" cashflow (balloon and/or one
    or more prepayment series) and where it comes from, WITHOUT advancing.
    Returns:
      nextextra : date and combined amount of the soonest extra payment
      xsource   : bitmask of contributors -- bit FR_BALLOON(=1) set if a
                  balloon contributes, bit i set if prepayment series i
                  contributes (multiple may coincide and are summed).
    When plus_regular is off, a coinciding balloon REPLACES rather than adds. }
  { Go port: dosport.go: findNextExtra (line 132) -- returns the soonest extra
    cashflow and an xsource bitmask (FR_BALLOON + per-prepay-series bits),
    summing coincident contributors, exactly as here. }
  procedure FindNextExtra (var xsource: byte; var nextextra: balloonrec);
    var
      i: integer;
  begin
    if (npre = 0) then
      begin
        if (next_balloon > nballoons) then
          xsource := 0
        else
          begin
            nextextra := balloon[next_balloon]^;
            xsource := FR_BALLOON;
          end
      end
    else
      begin
        nextextra.date := pre[1]^.nextdate;
        xsource := 2;  { 1 shl 1 }
        nextextra.amount := pre[1]^.payment;
        for i := 2 to npre do
          case DateComp(pre[i]^.nextdate, nextextra.date) of
            0:
              begin {Extra payment comes from i as well as whatever it used to be}
                xsource := (xsource or (1 shl i));
                nextextra.amount := nextextra.amount + pre[i]^.payment;
              end;
            -1:
              begin  {Extra payment comes from i instead of whatever it used to be}
                xsource := 1 shl i;
                nextextra.date := pre[i]^.nextdate;
                nextextra.amount := pre[i]^.payment;
              end;
          end;
        if (next_balloon <= nballoons) then
          case DateComp(balloon[next_balloon]^.date, nextextra.date) of
            0:
              begin
                xsource := (xsource or FR_BALLOON);
                if (df.c.plus_regular) then
                  nextextra.amount := nextextra.amount + balloon[next_balloon]^.amount
                else
                  nextextra.amount := balloon[next_balloon]^.amount;
              end;
            -1:
              begin
                xsource := FR_BALLOON;
                nextextra.date := balloon[next_balloon]^.date;
                nextextra.amount := balloon[next_balloon]^.amount;
              end;
          end;
      end;
{$ifdef BUGSIN}
     if (xsource and 3 = 3) {both FR_BALLOON and from pre[1]} and (not df.c.plus_regular)
     and (DateComp(payment.date,h^.firstdate)=0) {do it only once!}
     and (screen^[20,1,1]<'³')  {this last criterion says we're PRINTING A TABLE}
     then Scavenger('C-9');
{$endif}
  end; {FindNextExtra}

  { CheckOffBalloon -- consume the extra payment(s) identified by xsource
    after they have been applied this period.  Advances next_balloon past a
    used balloon, and for each contributing prepayment series steps its
    nextdate forward by one period; a series whose nextdate runs past its
    stopdate is deleted by compacting pre[] down (and xsource is re-mapped
    because pre[1] then becomes the former pre[2]). }
  { Go port: dosport.go: checkOffBalloon (line 178) -- consumes the extra
    payment(s) named by xsource: advances next_balloon, steps each contributing
    prepay series' nextdate, and deletes/compacts an exhausted series with the
    same xsource re-mapping. }
  procedure CheckOffBalloon (xsource: byte);
    var
      i, j: integer;
  begin
           {if Time_To_Re_Amortize then exit;}
      {This will be done when ComputeNext is called from within Re_Amortize}
    if ((xsource and FR_BALLOON) = FR_BALLOON) then
      inc(next_balloon);
    i := 1;
    while (i <= npre) do
      begin
        if (((1 shl i) and xsource) > 0) then
          with pre[i]^ do
            begin
              AddPeriod(nextdate, pre[i]^.peryr, pre[i]^.startdate.d, add);
              if (DateComp(nextdate, stopdate) > 0) then
                begin
                  dec(npre);
                  for j := i to npre do
                    pre[j]^ := pre[succ(j)]^;
                  dec(i);
                  xsource := ((xsource div 2) and ((xsource and 1) or 254));
         {Adjusting to the fact that pre[1]^ is now the former pre[2]^}
                end;
            end;
        inc(i);
      end;
  end;

  { ComputeNext -- advance ONE period of the repayment walk: step the date,
    set the base payment (0 on skip-months), fold in the soonest balloon/prepay
    (before / on / after the regular date), accrue interest over the actual
    interval, apply moratorium / principal-reduction target, then reduce the
    principal and update the USA-rule bucket.  The canonical DOS per-period
    engine; see the numbered ORDER OF OPERATIONS in the body. }
  { Go port: dosport.go: computeNext (line 213) -- a faithful port of this whole
    per-period sequence (skip-months, balloon/prepay before/on/after, 360-vs-
    exact interest interval, DAILY compounding, moratorium, target override of
    skip, and the USA-rule usap bucket). }
  procedure Paymenttype.ComputeNext (var p, usap: real);
    {Advances date to next payment date (incl balloons) and}
    {computes total amount of payment (incl balloons).}
    {base_date is always the last _regular_ payment date,}
    {used with AddPeriod to find the next date.}
    {prevdate is the last payment date, regular or no,}
    {used to compute interest which has accrued since last pmt . }

    var
      balloonpos: shortint;
      {balloonpos is 1 if the next payment is a regular pmt,}
      { -1 if the next payment is a balloon,}
      {and 0 if next regular and balloon coincide.}
      nextextra: balloonrec;
      xsource: byte;
      timedif: real;

         {Note on use of xsource:  xsource tells you where nextextra comes from.}
    {It may come from multiple sources.  If bit i of xsource is set,}
    {then pre[i]^ was involved.  If bit 7 is set (128), then a balloon}
    {was involved}

  begin {ComputeNext}
    { ORDER OF OPERATIONS (one period):
      1. advance the regular payment date one period from base_date;
      2. set the base payment to d, or 0 if this month is a skip-month;
      3. find the next extra cashflow (balloon/prepay) and decide whether it
         falls before (-1), on (0), or after (1) the regular date;
      4. accrue interest over the actual interval since prevdate;
      5. apply moratorium / principal-reduction target adjustments;
      6. reduce principal and update the USA-rule bucket. }
    date := base_date;
    AddPeriod(date, h^.peryr, h^.firstdate.d, add);          { step 1 }
    if (date.m in skipmonthset) then payamt:=0 else payamt := d;  { step 2: skip-month }
    FindNextExtra(xsource, nextextra);                       { step 3 }

    balloonpos := 1;
    if (xsource > 0) then
      begin
        balloonpos := DateComp(nextextra.date, date);
        if (DateComp(date, h^.lastdate) > 0) then
          balloonpos := -1;  { past last regular pmt => only the extra remains }
        if (balloonpos < 0) then
          begin {extra falls BEFORE next regular date: pay only the extra}
            payamt := nextextra.amount;
            date := nextextra.date;
            CheckOffBalloon(xsource);
          end
        else if (balloonpos = 0) then
          begin {extra coincides with regular date}
            if (df.c.plus_regular) then
              payamt := payamt + nextextra.amount   { balloon ADDS to regular }
            else
              payamt := nextextra.amount;           { balloon REPLACES regular }
            CheckOffBalloon(xsource);
          end;
      end;
    if (balloonpos >= 0) then
      base_date := date;  { only advance the regular grid when a regular pmt occurred }
    { step 4: interest interval.  For 360-basis / non-exact, snap to whole
      calendar months (and a half-month nudge for semimonthly) so payments
      land on clean amounts; otherwise use exact day-count YearsDif. }
    if ((df.c.basis=x360) or (not df.c.exact)) and DaysCloseEnough(date, prevdate, h^.peryr) then {start "Tried to move this"}
      begin
        timedif := (date.y - prevdate.y) + (date.m - prevdate.m) / 12;
        if (h^.peryr = 24) then
          timedif := timedif + round((2 * (integer(date.d) - prevdate.d)) / 30) / (2 * 12);
      end
    else
      timedif := YearsDif(date, prevdate);
    if (df.c.peryr=DAILY) then
      interest := (exxp(truerate*timedif)-1) * (p - usap)  {truerate is re-computed from adj[i]^.loanrate on re-amortize 4/94}
    else
      interest := h^.loanrate * timedif * (p - usap); {h^.loanrate is set to adj[i]^.loanrate on re-amortize 10/4/92}
    if (hard_payment) then Round2(interest); {@round  -- round-half-down to the cent}

    { step 5: moratorium and target.  During moratorium (date before
      first_repay) only interest is collected (no principal reduction).
      Otherwise, if the period's principal reduction (payamt-interest) falls
      short of the target minimum, bump the payment up to meet it.  NOTE:
      target overrides skip-month zeroing -- a skip month still pays the
      target minimum + interest. }
    case (balloonpos) of {added 10/93 to compensate for disastrous effort to move whole interest computation area north}
       0: begin {payamt comes in as (regular+balloon_or_pre)}
          if (mor^.first_repaystatus >= defp) and (DateComp(date, mor^.first_repay) < 0) then
            payamt := payamt - d + interest
          else if (payamt - interest < targ^.target) then
            payamt := payamt - d + targ^.target + interest; 
          end;
       1: begin {regular payment only:}
          if (mor^.first_repaystatus >= defp) and (DateComp(date, mor^.first_repay) < 0) then
            payamt := interest
          else if (payamt - interest < targ^.target) then
            payamt := targ^.target + interest; 
          end;
       {-1: balloon only: do nothing}
      end; {case xsource} {end of "Tried to move this"}
    prevdate := date;
    p := p + interest - payamt;  { step 6: new balance = old + interest - payment }
    if (df.c.USARule) then
      begin
        { USA rule: track unpaid interest separately; principal that hasn't had
          its interest covered does not itself accrue interest.  Floored at 0. }
        usap := usap + interest - payamt;
        if (usap < 0) then
          usap := 0;
      end;
    principal := p;
    usaprinc := usap;
  end; {ComputeNext}

{$ifdef 0}
New version doesn't work with Lotus

  procedure WriteOneLotusLine (var amt, int: real);
    var
      format: char;
  begin
    with Payment do
      begin
        if (df.h.commas) then
          format := LOTUS_comma_fmt
        else
          format := LOTUS_real_fmt;
        inc(lotusrow);
        lotuscol := 0;
        WriteOneCell(LOTUS_date_fmt, Julian(date) + 1);
        if (fancy) then
          WriteOneCell(format, amt);
        WriteOneCell(format, principal);
        WriteOneCell(format, amt - int);
        WriteOneCell(format, int);
        WriteOneCell(format, int_to_date);
      end;
  end;

  procedure LineAcrossSpreadsheet;
    var
      i, n: integer;
  begin
    inc(lotusrow);
    lotuscol := 0;
    if (fancy) then
      n := 7
    else
      n := 5;
    for i := 1 to n do
      WriteOneStringCell(true, '\', lotusrow, lotuscol, '-');
            {lotuscol is incremented within WriteOneStringCell}
    lotuscol := 0;
  end;

  procedure WriteSummaryToLotus;
    var
      format: char;
  begin
    with payment do
      begin
        if (df.h.commas) then
          format := LOTUS_comma_fmt
        else
          format := LOTUS_real_fmt;
        LineAcrossSpreadsheet;
        inc(lotusrow);
        WriteOneStringCell(true, '^', lotusrow, 0, 'Subtot:');
        if (fancy) then
          WriteOneCell(format, cumamt);
        WriteOneCell(format, principal);
        WriteOneCell(format, cumamt - cumint);
        WriteOneCell(format, cumint);
        if (fancy) then
          inc(lotuscol);
        WriteOneCell(format, int_to_date);
        inc(lotusrow); {skip a line}
      end;
  end;
{$endif}
  { R78Header1 -- build header line 1 with the "Rule of 78 Amortization"
    caption centered into the standard header bar. }
  { Go port: n/a -- table header text; the web frontend renders column headers. }
  function R78Header1:str80;
           var p  :byte;
               ws :str80;
           begin
           ws:=header1;
           p:=(length(ws)-length(r78text)) div 2;
           Move(r78text[1],ws[p],length(r78text));
           R78Header1:=ws;
           end;

{$ifdef MAC}
  { TwoLinesOut (MAC build) -- emit the two-line column header (and the R78
    caption / divider) to the output device.  Returns false and sets abort on
    an output failure. }
  { Go port: n/a -- emits column-header lines to the DOS output device. }
  function TwoLinesOut (first: boolean): boolean;
    var
      ws: str80;
  begin
    TwoLinesOut := false;
    if (not fancy) and (df.c.R78) then OutputLine(R78Header1);
    if fancy then
      ws := fancyheader2
    else
      ws := header2;
    ws[1] := ' ';
    {ws[80] := ' ';}
    if not OutputLine(ws) then
      begin
        abort := true;
        exit;
      end;
    if fancy then
      ws := fancyheader3
    else
      ws := header3;
    ws[1] := ' ';
    {ws[80] := ' ';}
    if not (OutputLine(ws) and OutputLine(lineacross)) then
      begin
        abort := true;
        exit;
      end;
    TwoLinesOut := true;
  end;

{$else}
{$ifdef 0}
works with the screen.  No good
function TwoLinesOut(first :boolean):boolean;
         var ws :str80;
         begin
         TwoLinesOut:=false;
         if (fancy and first) then ws:=fancyheader1
         else if (not fancy) and (df.c.R78) then ws:=R78Header1
         else ws:=header1;
         ws[1]:=' '; ws[80]:=' ';  if not OutputLine(ws) then begin
             abort:=true; exit;
             end;
         if fancy then ws:=fancyheader2 else ws:=header2;
         ws[1]:=' '; ws[80]:=' ';  if not OutputLine(ws) then begin
             abort:=true; exit;
             end;
         if fancy then ws:=fancyheader3 else ws:=header3;
         ws[1]:=' '; ws[80]:=' ';  if not OutputLine(ws) then begin
             abort:=true; exit;
             end;
         ws:=header4; ws[1]:=' '; ws[80]:=' ';
         if not ((OutputLine(ws)) and (OutputLineFeeds(1))) then begin
             abort:=true; exit;
             end;
         TwoLinesOut:=true;
         end;
{$endif}
{$endif}

{$ifdef 0}
 {$F+}
  procedure NewPage;
{$ifndef OVERLAYS} {$F-} {$endif}
  begin
    if not (FormFeed and TopMargin and TwoLinesOut(false)) then
      abort := true;
  end;

  procedure WriteGrandTotalsToLotus;
    var
      format: char;
  begin
    if (df.h.commas) then
      format := LOTUS_comma_fmt
    else
      format := LOTUS_real_fmt;
    LineAcrossSpreadsheet;
    inc(lotusrow);
    WriteOneStringCell(true, '^', lotusrow, 0, 'Total:');
    WriteOneCell(format, h^.amount + int_to_date);
    WriteOneStringCell(true, '^', lotusrow, 2, 'Principal:');
    WriteOneCell(format, h^.amount);
    if (fancy) then
      inc(lotuscol);
    WriteOneStringCell(true, '^', lotusrow, 4, 'Interest:');
    if (not fancy) then
      inc(lotuscol);
    WriteOneCell(format, int_to_date);
  end;
{$endif}

{ PrintSummaryLine -- emit a "Subtotal:" line accumulating cumamt/cumint over
  a summary period (yearly/quarterly/etc.) plus the running principal and
  interest-to-date, then zero the cumulators for the next period.  Output goes
  to the Output string list; bCommaSeperated selects CSV vs. fixed-width text. }
{ Go port: n/a -- subtotal-line formatting/emission; the Go engine returns raw
  rows and the web layer aggregates/renders summaries. }
procedure PrintSummaryLine(Output: TStringList; bCommaSeperated: boolean);
          var ws  :str80;
              len :byte absolute ws;
              Seperator: char;
          begin
            with payment do begin
//             CheckRoomOnPage(2,false,NewPage);
             if( not bCommaSeperated ) then begin
               fillchar(ws[1],80,'-'); ws[0]:=#80;
               Output.Add(ws);
               Seperator := ' ';
             end else
               Seperator := ',';
             if (fancy) then
               ws:='Subtotal:' + Seperator + ftoa2(cumamt,12,df.h.commas) + Seperator
                  + ftoa2(cumint,13,df.h.commas) + Seperator + ftoa2(cumamt-cumint,13,df.h.commas) + Seperator
                  + ftoa2(principal,14,df.h.commas) + Seperator + ftoa2(int_to_date,14,df.h.commas)
             else
               ws:= Seperator + '  Subtotal:'+ Seperator + ftoa2(cumint,16,df.h.commas) + Seperator + ftoa2(cumamt-cumint,16,df.h.commas)
                  + Seperator + ftoa2(principal,16,df.h.commas) + Seperator + ftoa2(int_to_date,16,df.h.commas);
             if (destin=none ) then begin
               if (len<80) then FillChar(ws[succ(len)],80-len,' ');
               len:=80;
               Output.Add(ws);
               end
             end;
             Output.Add( '' );
          cumint:=0;  cumamt:=0;
          end; {PrintSummaryLine}

  { PrintGrandTotals -- emit the final footer: total payments (principal +
    all interest), principal, and total interest.  Picks a wider numeric
    layout when totals exceed 8 digits, and a CSV layout when requested. }
  { Go port: n/a -- footer/grand-totals formatting; totals are carried on the Go
    AmortResult and rendered by the web layer. }
  procedure PrintGrandTotals( Output: TStringList; bCommaSeperated: boolean );
    var
      ws: str80;
  begin
      with payment do
        begin
//          CheckRoomOnPage(1,false,NewPage);
          if( not bCommaSeperated ) then begin
            ws := lineacross;
            if (destin = none) then
              Output.Add(ws);
          end;
{          else if (destin > tolotus) then
            if not OutputLine(ws) then
              begin
                abort := true;
                exit;
              end;
 }
          if( not bCommaSeperated ) then begin
            if (h^.amount+int_to_date>99999999.99) then begin
              ws:='Total pmts:' + ftoa2(h^.amount+int_to_date,16,df.h.commas);
              ws:=ws + ' Principal:';
              ws:= ws + ftoa2(h^.amount,16,df.h.commas) + ' Interest:'
                 + ftoa2(int_to_date,16,df.h.commas);
            end else begin
              ws:='Total payments: ' + ftoa2(h^.amount+int_to_date,13,df.h.commas);
              ws:=ws + '   Principal: ' + ftoa2(h^.amount,13,df.h.commas) + '  Interest:'
                 + ftoa2(int_to_date,13,df.h.commas);
            end;
          end else begin
            ws := '';
            if( not fancy ) then
              ws := ws + ',';
            ws := ws + 'Total payments,' + ftoa2(h^.amount+int_to_date,13,df.h.commas);
            if( fancy ) then
              ws := ws + ',';
            ws:=ws + ',' + ftoa2(h^.amount,13,df.h.commas) + ',,' + ftoa2(int_to_date,13,df.h.commas);
          end;
          if (destin = none) then
            Output.Add(ws)
{          else if (destin > tolotus) then
            begin
              if not ((OutputLine(ws)) and (OutputLineFeeds(1))) then
                begin
                  abort := true;
                  exit;
                end;
            end;}
        end;
  end;

  { ReportFinalPayment -- emit the (often irregular) final payment amount as a
    separate note, used when the last computed payment differs from the
    regular amount. }
  { Go port: n/a -- prints the irregular final-payment note; the Go last row
    already carries the folded-in residual for the web layer to display. }
  procedure ReportFinalPayment( Output: TStringList );
    const fpamount:string[22]='Final payment amount: ';
    var
      ws: string[20];
  begin
//    CheckRoomOnPage(0, false, NewPage);
    ws := ftoa2(payment.payamt, 12, df.h.commas);
    Output.Add( ws );
//    writeln(indent,fpamount,ws);
//    if (destin > tolotus) then
//      if not OutputLine(Concat(indent,fpamount, ws)) then
//        abort := true;
  end;

  { TimeForSummary -- decide whether a subtotal line should print between the
    current payment date t and the next payment date nextt.  True when a month
    that is a summary boundary (in cumset) is reached before the next payment's
    month and there are no more payments in this month, or when t is the very
    last payment.  cumset holds the month numbers that close a summary period
    (e.g. just December for yearly, or months 3/6/9/12 for quarterly). }
  { Go port: n/a -- summary-period boundary test used only for on-screen/printed
    subtotals; the Go engine emits every row and defers summary aggregation to
    the presentation layer (V6-14, deferred; see docs/dispatch_gaps.md). }
  function TimeForSummary (t, nextt: daterec): boolean;
    var
      mm: integer;
      NoMorePaymentsThisMonth, CumsetBeforeNext: boolean;
  begin
    NoMorePaymentsThisMonth := ((nextt.m > t.m) or (nextt.y > t.y));
    mm := t.m;
    if (h^.peryr >= 12) then
      begin
      end
    else
      while (not (mm in cumset)) and (mm <> nextt.m) do
        begin
          inc(mm);
          if (mm > 12) then
            mm := mm - 12;
        end;
    CumsetBeforeNext := (mm in cumset) and (mm <> nextt.m);
    TimeForSummary := (CumsetBeforeNext and NoMorePaymentsThisMonth) or (DateComp(t, very_last) = 0);
  end;

{ PrintAndReset (PC version) -- format and emit one amortization row for the
  current `payment`, updating int_to_date and (in detail mode) honoring the
  final-payment adjustment that folds the remaining principal into the last
  row.  In summary ('A'..'Z') mode it accumulates and emits subtotals at
  period boundaries; in detail (' ') mode it prints each row.  Also announces
  ARM rate changes that fall on this date. }
{ Go port: n/a (output formatting) -- but the load-bearing bits are reproduced in
  the Go generators: the "fold remaining principal into the very-last payment"
  step and the int_to_date accumulation live in engine.go: generateSimpleSchedule
  (line 881) / generateFancySchedule (line 1270). }
procedure PrintAndReset(t,nextt :daterec; Output: TStringList; bCommaSeperated: boolean ); {PC version}
          var ta,xgo,ygo             :byte;
              amt,int                :real;
              ki                     :char;
              precum,lastline        :boolean;
              nextnext               :daterec;
              Seperator              : char;

   { AnnounceRateChangeIfTimely -- if an adjustment (ARM) date equals this
     payment date, emit an explanatory "--->On <date>, re-computed at <rate>%"
     note (and, if the adjustment fixed a payment, the new payment amount). }
   { Go port: n/a -- emits an ARM rate-change annotation line; the web layer
     surfaces adjustments from the returned schedule. }
   procedure AnnounceRateChangeIfTimely;
             var i,j :byte;
                 ws  :str80;
             begin
             if( bCommaSeperated ) then exit;
             for i:=1 to nadj do
                if (DateComp(Payment.date,adj[i]^.date)=0) then begin
                   ws:= '--->On '
                        +DateStr(adj[i]^.date)
                        +', re-computed at '
                        +ftoa4(100*adj[i]^.loanrate,7)
                        +'%';
                   if (ok(adj[i]^.amount)) then begin
                      ws:=ws+':  Payment fixed at ';
                      if (abs(adj[i]^.amount)>1E6) then ws:=ws+ftoa2(adj[i]^.amount,11,df.h.commas)
                      else if (abs(adj[i]^.amount)>1E4) then ws:=ws+ftoa2(adj[i]^.amount,10,df.h.commas)
                      else ws:=ws+ftoa2(adj[i]^.amount,8,df.h.commas);
                      end;
                   if (destin=tolotus) or ((destin=toprinter) and (not df.p.ibmset)) then
                      for j:=1 to 4 do ws[j]:='-'; {Lotus won't take #196 - change to dash.}
                   case destin of
//                      none : ob.StoreAndWrite(ws+crlf);
                      none : if( Output<>nil ) then Output.Add( ws );
//                      tolotus : begin
//                                lotusrow:=2+lotusrow;  lotuscol:=0;
//                              WriteOneStringCell(false,'''',lotusrow,lotuscol,ws);
//                                end;
//                         else if not (OutputLineFeeds(1)
//                                      and OutputLine(ws)) then abort:=true;
                         end; {case}
                   end;
             end;

          begin {PrintAndReset} with payment do begin
            if( bCommaSeperated ) then
              Seperator := ','
            else
              Seperator := ' ';
//          while keypressed do begin
//             ki:=readkey;
//             if (ki=#27) then abort:=true;
//             end;
          if (abort) then exit;
          if (DateComp(date,very_last)=0) then begin
           {Adjust last payment to cover entire remaining principal (a small
            residual from rounding becomes part of the final payment).}
              payamt:=payamt+principal;
              cumamt:=cumamt+principal;
              principal:=0;
              end;
          if (cum in ['A'..'Z']) then begin
             amt:=payamt;
             int:=interest;
             end
          else begin
             amt:=cumamt;
             int:=cumint;
             end;
          int_to_date:=int_to_date+int;
          nextnext:=nextt; AddPeriod(nextnext,h^.peryr,h^.firstdate.d,add);
          precum:=TimeForSummary(nextt,nextnext);
            {This is a flag that says there's a subtotal line coming up - end the
             page early to avoid a widowed line.  It isn't implemented quite right,
             because it doesn't take account of balloon payments.}
          if (fancy) then begin
            ws:=DateStr(date) + Seperator + ftoa2(amt,13,df.h.commas) + Seperator
               + ftoa2(int,13,df.h.commas) +  Seperator + ftoa2(amt-int,13,df.h.commas) + Seperator
               + ftoa2(principal,14,df.h.commas) + Seperator + ftoa2(int_to_date,14,df.h.commas);
          end else begin
            ws:=strb(paynum,3)+Seperator+DateStr(date) + Seperator + ftoa2(int,16,df.h.commas)
               + Seperator + ftoa2(amt-int,16,df.h.commas) + Seperator + ftoa2(principal,16,df.h.commas)
               + Seperator + ftoa2(int_to_date,16,df.h.commas);
          end;
          inc(count);
//          ob.StoreAndWrite(ws);
          if( Output<>nil ) then Output.Add( ws );
{          if (destin>none) and (more_dates_than_lines) and (count=6) then begin
             write('...'); clreol; writeln;
             AbsXY(xgo,ygo);
             window(1,ygo,80,24);
             end;}
          if (fancy) and (df.c.in_advance) then AnnounceRateChangeIfTimely;
          if ((df.h.printzeros) or (amt<>0)) or (cum in ['a'..'z']) then
{             if (destin>tolotus) then begin if not OutputLine(ws) then begin
                 abort:=true; exit; end;
                 end
             else if (destin=tolotus) then WriteOneLotusLine(amt,int);
             }
//          ta:=MIOResult; if (ta>0) then abort:=true;
          if (fancy) and (not df.c.in_advance) then AnnounceRateChangeIfTimely;

          if (cum in ['A'..'Z']) and (TimeForSummary(t,nextt)) then PrintSummaryLine( Output, bCommaSeperated );
          lastline:=(DateComp(date,very_last)=0);
//          if (precum) then CheckRoomOnPage(2,lastline,NewPage) else CheckRoomOnPage(0,lastline,NewPage);
          if (cum in [' ','a'..'z']) then begin
             cumint:=0;  cumamt:=0;
             end;
          end; end; {PrintAndReset}

  procedure Re_Amortize (var p: real);
  forward;

  { DecideWhetherToPrintALine -- per-period output gatekeeper.  Bumps the
    payment number, folds this row's interest/payment into the running
    cumulators, and calls PrintAndReset when a line is actually due (detail
    mode, summary-period boundary, or last payment).  Crucially, after each
    line it checks whether an ARM adjustment date has been crossed and, if so,
    triggers Re_Amortize to recompute rate/payment for the remaining schedule.
    The t/nextt args matter because of the recursive Re_Amortize calls. }
  { Go port: dosport_walk.go: repayFancyLoan (line 72) -- the Go walk inlines this
    per-row bookkeeping and, crucially, the same "if an adjustment date was
    crossed, call reAmortize" trigger (dosport_walk.go: reAmortize line 264).
    The print/summarize half is n/a (web renders rows). }
  procedure DecideWhetherToPrintALine (t, nextt: daterec; Output: TStringList; bCommaSeperated: boolean );
              {These arguments are essential, in ways that aren't obvious}
        { but have to do with recursive calls to Re-Amortize}
  begin
    inc(payment.paynum);
    cumint := cumint + payment.interest;
    cumamt := cumamt + payment.payamt;
    if (cum in [' ', 'A'..'Z']) or (TimeForSummary(t, nextt)) then
      PrintAndReset(t, nextt, Output, bCommaSeperated);
           {Print out only if 1. No accumulation specified, |}
      {2. This month is in cumset and the next one won't be.}
      {3. This is the last payment.}
           {if (Time_To_Re_Amortize) then Re_Amortize(p);}
    if (next_adj <= nadj) and (DateComp(nextt, adj[next_adj]^.date) > 0) then
      Re_Amortize(p);
  end;

  { SaveDataForReAmortize -- snapshot npre, next_balloon, and the active
    prepayment records into the old_* globals before each ComputeNext, so that
    Re_Amortize (which steps back one payment) can restore the exact extra-
    payment cursor state it had before the period that crossed an adj date. }
  { Go port: dosport_walk.go: saveDataForReAmortize (line 59) -- snapshots npre,
    next_balloon, and the active prepay records so reAmortize can step back one
    payment with the correct extra-payment cursor. }
  procedure SaveDataForReAmortize;
    var
      i :byte;
  begin
  old_npre:=npre; old_next_balloon:=next_balloon;
  for i:=1 to npre do begin
    if (old_pre[i]=nil) then
       if not GetMemIfAvailable(pointer(old_pre[i]),sizeof(prepaymentrec)) then errorflag:=true;
    old_pre[i]^:=pre[i]^;
    end;
  end;

  { DisposeOfOld_Pre -- free the heap prepayment copies made by
    SaveDataForReAmortize once a fancy-loan walk completes. }
  { Go port: n/a -- the Go saveDataForReAmortize (dosport_walk.go line 59) uses
    value copies reclaimed by GC, so there is no explicit dispose step. }
  procedure DisposeOfOld_Pre;
    var
      i :byte;
    begin
    for i:=1 to maxprepay do if (old_pre[i]<>nil) then begin
       FreeMem(pointer(old_pre[i]),sizeof(prepaymentrec));
       old_pre[i]:=nil;
       end;
    end;

  { RepayFancyLoan -- the master period-by-period repayment walk for loans with
    any advanced options.  Drives NextPayment.ComputeNext repeatedly from
    firstdate forward, optionally printing each row (when Output<>nil) and/or
    accumulating discounted cashflows for APR (when value_calc).
    Parameters:
      p        (var) : starting principal; on return, the residual balance
      usapart  (var) : USA-rule exempt principal (carried through the walk)
      loandate,firstdate : loan settlement date and first payment date
      Output   : destination row list, or nil to compute silently (e.g. to
                 refine a payment inside Iterate)
      bCommaSeperated : CSV vs fixed-width output formatting
      entire   : walk the whole schedule (true) vs only up to adj #adjnum (false)
      value_calc : accumulate aprvalue (PV at v_rate) instead of just balances
      adjnum   : when >0, stop at adjustment adjnum's date (used by the ARM /
                 adjusted-payment solvers)
    Loop termination: balance hits ~0, the requested stop date is reached, a
    balance_calc target is met, or abort.  ARM adjustments encountered mid-walk
    trigger Re_Amortize.  Restores h^.loanrate on exit. }
  { Go port: dosport_walk.go: repayFancyLoan (line 72) -- the master advanced-
    option walk: drives computeNext (dosport.go line 213) period by period, folds
    a sub-dollar residual into the payment, triggers reAmortize at adjustment
    dates, and supports the entire/til_adj and value_calc (APR) modes.  The
    row-output side is n/a; balance/APR accumulation carries through.
    The dosEng that owns this walk is assembled by dosport_entry.go: buildDosEng
    (line 24) and driven from AmortizeDOS (line 424); dosPortCanHandle (line 269)
    gates when the DOS-faithful engine (vs. the simpler generators) is used. }
  procedure RepayFancyLoan(var p,usapart:real; loandate,firstdate:daterec; Output:TStringList; bCommaSeperated: boolean; entire,value_calc:boolean; adjnum:byte);
            {USApart is the part of the principal that is exempt from accruing}
      {interest because of the USA rule.}
    var
      firstd: real;
      WhenToStop: ^paymenttype;
      stopdate: daterec;
      saverate: real;
{$ifdef BUGSIN}
      ia,ib :byte;
{$endif}

    { BalanceStop -- (balance_calc mode) true once the running balance has
      dropped to the user's "as of" target amount (in_advance discounts the
      next period's interest first). }
    { Go port: engine.go: BalanceAtDate (line 2098) / DateForBalance (line 2119)
      implement the "Balance As Of" stop condition against the generated
      schedule rather than as an inline walk predicate. }
    function BalanceStop:boolean;
             begin
             if (df.c.in_advance) then
               BalanceStop:= (WhenToStop^.principal+WhenToStop^.payamt-WhenToStop^.interest<w^.amount+minpmt)
             else
               BalanceStop:= (WhenToStop^.principal+WhenToStop^.payamt<w^.amount+minpmt);
             end;

  begin {RepayFancyLoan}
{$ifdef BUGSIN}
     if (df.c.in_advance) and (mor^.first_repaystatus>empty) and (with_print) then Scavenger('C-7');
     if (with_print) then
        for ia:=1 to nadj do if (adj[ia]^.amountstatus<=outp) then
           for ib:=1 to nballoons do
             if (DateComp(balloon[ib]^.date,adj[ia]^.date)=0) then Scavenger('C-9');
{$endif}
    saverate := h^.loanrate;
    if (Output<>nil) or ((adjnum > 0) and (not balance_calc)) then
      WhenToStop := @Payment
    else
      WhenToStop := @NextPayment;
             {This function is called without_print in order to refine the}
      {payment amount.  Then the final p is controlling, and we want}
      {to stop when nextpayment.date=very_last.}
      {When printing out (entire), we need to go one further in}
      {order to print the last line.}
    if (adjnum > 0) then
      stopdate := adj[adjnum]^.date
    else
      stopdate := very_last;
    if (not dateok(stopdate)) then
      begin
        stopdate := firstdate;
        stopdate.y := 100 + pred(df.c.centurydiv);
      end;             {Keep going as long as possible}
    t := firstdate;
    abort := false;
    AddPeriod(t, h^.peryr, firstdate.d, subtract);
    if (prepaid) then
      begin
        paidthru := firstdate;
        if (not df.c.in_advance) then
          AddPeriod(paidthru, h^.peryr, firstdate.d, subtract);
      end
    else
      paidthru := loandate;
    if (df.c.in_advance) then
      begin
             {base_date must be 1 period later for df.c.in_advance calcs}
        { Interest-in-advance: the first payment is due at settlement, so the
          base date is shifted forward a period and a balloon coinciding with
          firstdate is folded into the first payment. }
        if (nballoons = 0) or (DateComp(balloon[1]^.date, firstdate) > 0) then
          begin
            firstd := d;
            AddPeriod(t, h^.peryr, firstdate.d, add);
          end
        else if (DateComp(balloon[1]^.date, firstdate) = 0) then
          begin
            firstd := balloon[1]^.amount + d;
            AddPeriod(t, h^.peryr, firstdate.d, add);
          end
        else
          begin
            firstd := balloon[1]^.amount;
            paidthru := balloon[1]^.date;
          end;
      end;
    if (entire) then
      next_balloon := 1;
    NextPayment.Init(t, paidthru, firstd);
           {Initialize base_date to t and prevdate to paidthru.}
      {Note that it is NextPayment that is being initialized, and only}
      {NextPayment will be advanced.  Payment will just be used to copy}
      {and save NextPayment.}
    if (df.c.in_advance) and (nballoons > 0) and (DateComp(balloon[1]^.date, firstdate) <= 0) then
      inc(next_balloon);
              {You've just paid a balloon above.}
{ 
    if (df.c.in_advance) then
      nextpayment.ComputeNextForAdvancedInterestPayment(p)
    else
}
      NextPayment.ComputeNext(p, usapart);
    if (value_calc) then
      aprvalue := aprvalue + NextPayment.payamt * exxp(-v_rate * YearsDif(NextPayment.date, loandate));
    repeat
      if (overflowflag) then
        exit;
      NextPayment.paynum:=Payment.paynum;
      payment:=NextPayment;
      SaveDataForReAmortize;
{
      if (df.c.in_advance) then
        NextPayment.ComputeNextForAdvancedInterestPayment(p)
      else
}
        NextPayment.ComputeNext(p, usapart);
      { When the residual balance drops below a dollar, fold it into this
        payment so the schedule terminates cleanly at zero. }
      if ((not h^.lastok) or (entire)) and (WhenToStop^.principal < minpmt) and (not value_calc) then
        begin
          WhenToStop^.payamt := WhenToStop^.payamt + WhenToStop^.principal;
          WhenToStop^.principal := 0;
        end;
      if (Output<>nil) then
        DecideWhetherToPrintALine(payment.date, nextpayment.date, Output, bCommaSeperated)
      else if ((next_adj<=adjnum) or entire) and (next_adj<=nadj) and (DateComp(nextpayment.date,adj[next_adj]^.date)>0) then
        Re_Amortize(p);
      if (value_calc) then
        aprvalue := aprvalue + NextPayment.payamt * exxp(-v_rate * YearsDif(NextPayment.date, loandate));
    until (((not h^.lastok) or (Output<>nil)) and (WhenToStop^.principal = 0))
           or ((balance_calc) and (BalanceStop))
           or (DateComp(WhenToStop^.date, stopdate) >= 0) or (abort);
       {I decided to cut short a table even if it's been explicity requested to go beyond balance=0}
{$ifndef BOFA} {In BofA, the APR calculation is done in 360, while the interest is calculated at 365}
    if (value_calc) and (NextPayment.principal <> 0) then {aprvalue of tacked-on balloon}
      aprvalue := aprvalue + NextPayment.principal * exxp(-v_rate * YearsDif(NextPayment.date, loandate));
{$endif}
    if ((not h^.lastok) and (WhenToStop^.principal = 0)) then
      begin
        if (not entire) then
          h^.lastdate := WhenToStop^.date;
      end
    else if (DateComp(WhenToStop^.date, very_last) > 0) and (not balance_Calc) then
      MessageBox('Internal error - last payment not found.  Please contact Ones & Zeros.', DA_InternalError );
    h^.loanrate := saverate;
    ComputeTrueRate;
    DisposeOfOld_Pre;
  end; {RepayFancyLoan}

  { GrowthPerPeriod -- the per-period principal growth factor f = 1 + periodic
    rate.  Weekly (52) and biweekly (26) use an explicit 7- or 14-day fraction
    of a year rather than 1/52 or 1/26 so the day-count matches a 365 basis;
    all other frequencies use rate / periods-per-year. }
  { Go port: engine.go: GrowthPerPeriod (line 29) -- identical f = 1 + periodic
    rate, with the same explicit 7-day / 14-day weekly/biweekly fractions. }
  function GrowthPerPeriod: real;
  begin
    case h^.peryr of
      52:
        GrowthPerPeriod := 1 + 7 * yrinv * h^.loanrate;
      26:
        GrowthPerPeriod := 1 + 14 * yrinv * h^.loanrate;
      else
        GrowthPerPeriod := 1 + h^.loanrate / RealPerYr(h^.peryr);
    end;
  end;

  { ComputeTrueRate -- derive the effective (compounded) rate used for DAILY
    interest from the nominal loanrate.  Only meaningful for DAILY compounding;
    uses a 365.25-day convenience denominator (see the note below). }
  { Go port: engine.go: ComputeTrueRate (line 44) -- same RateFromYield-based
    conversion of the reported rate to a true daily rate. }
  procedure ComputeTrueRate;
    {This is only used for DAILY interest calculations.
     There is a very tiny cheat here, leading to tiny inaccuracies:
     YearsDif accurately calculates with denominator of 366 for the part of
        a difference that's in a leap year, and 365 in other years.
     TRUERATE ought strictly to be computed with RateFromYield using 365
        and 366.  However, in a compromise with convenience, I've used 
        365.25 always.  
     For nominal 10% interest, using 365 you get a true rate=10.515578,
                         while using 366 you get a true rate=10.515582.
     The error can never be more than 3/4 of this difference.}
    var 
      rr: real;
  begin
    rr := ReportedRate(h^.loanrate);
    truerate := RateFromYield(rr,df.c.peryr);
  end;

  { RepayLoan -- fast closed-form repayment walk for a PLAIN loan (no advanced
    options, 360-basis or non-exact).  Iterates the balance recurrence over
    nperiods without building any output rows; used by Iterate to evaluate a
    trial payment/rate/amount.  Two branches:
      in_advance : interest charged at the start of the period;
      arrears    : ordinary annuity, with the first period scaled by prorate
                   (the odd-first-period factor).  Once the balance goes
                   negative the loan is overpaid and only the payment is
                   subtracted (no further interest). }
  { Go port: engine.go: RepayLoan (line 103) -- the fast closed-form plain-loan
    recurrence used to evaluate a trial payment/rate/amount: same in-advance vs
    arrears branches and the same prorate-scaled odd first period. }
  procedure RepayLoan (var p: real);
    var
      ff: real;
      i: word;
  begin
    f := GrowthPerPeriod;
    if (df.c.in_advance) then
      begin
        ff := (f - 1) / (2 - f);     {interest in advance = (p-d)*(f-1)/(2-f) }
        for i := 1 to h^.nperiods do
          p := p + ff * (p - d) - d;
      end
    else
      begin
        ff := 1 + (f - 1) * prorate;
        p := p * ff - d;         {First payment}
        for i := 2 to h^.nperiods do
          if (p < 0) then
            p := p - d
          else
            p := p * f - d;      {All other payments}
      end;
  end;

  { DetermineVeryLast -- set the global very_last to the latest date in the
    whole schedule: the maximum of the last regular payment, the last balloon,
    and every prepayment series' stop date.  Used as the loop terminus. }
  { Go port: engine.go: Amortize (line 139) -- the very_last terminus (max of
    last regular date, last balloon, and prepay stop dates) is computed inline in
    the Go dispatch/generator setup. }
  procedure DetermineVeryLast;
    var
      i: integer;
  begin
    if (nballoons > 0) and (DateComp(balloon[nballoons]^.date, h^.lastdate) > 0) then
      very_last := balloon[nballoons]^.date
    else
      very_last := h^.lastdate;
    for i := 1 to npre do
      if (DateComp(pre[i]^.stopdate, very_last) > 0) then
        very_last := pre[i]^.stopdate;
  end;

  { VeryLastRegularAmount -- the regular (non-balloon) payment that happens to
    fall on very_last: a prepayment payment if one ends there, else the last
    regular/adjusted loan payment if the loan's last date coincides.  Used to
    net out the regular component when "Balloon includes regular pmt" is on. }
  { Go port: dosport_walk.go: solveUnknownBalloon (line 358) -- the plus_regular
    netting (subtracting the regular component falling on very_last) is applied
    inside the Go unknown-balloon solve. }
  function VeryLastRegularAmount: real;
    var
      i: integer;
      theresult: real;
  begin
    theresult := 0;
    for i := 1 to npre do
      if (DateComp(pre[i]^.stopdate, very_last) = 0) then
        theresult := pre[i]^.payment;
    if (theresult = 0) and (DateComp(h^.lastdate, very_last) = 0) then
      if (nadj > 0) then
        theresult := adj[nadj]^.amount
      else
        theresult := h^.payamt;
    VeryLastRegularAmount := theresult;
  end;

  { DetermineLastPaymentDate -- solve for the loan's term (last payment date
    and #periods) given a known payment amount, by repaying until the balance
    reaches zero.  Fancy path: run RepayFancyLoan silently to walk to payoff,
    then back out each prepayment series' stop date / count to match.  Plain
    path: closed-form via the annuity formula (with the odd-first-period
    factor), rounding the period count up.  Returns false (and messages) when
    the payment is too small to ever retire the principal. p/usap are starting
    balance and USA bucket. }
  { Go port: backward.go: solveFancyTermFromPayment (line 945) for the fancy path
    (walk to payoff, back out each prepay series' stop/count) and
    solveNPeriodsFromPayment (line 380) for the plain closed-form annuity/round-up
    branch; the too-small-payment guard is preserved. }
  function DetermineLastPaymentDate (p, usap: real): boolean;
    var
      ff, p1: real;
      i: word;
      save_balloon: saved_balloon_state;
      calc_pre: array[1..maxprepay] of record
          stopdate: daterec;
          nn: integer;
        end;
    label
      ABORT,DONE;

  begin
    if fancy then
      begin
        save_balloon.Save; {Added 1/8/90}
        for i := 1 to npre do
          begin
            calc_pre[i].stopdate := pre[i]^.stopdate;
            calc_pre[i].nn := pre[i]^.nn;
          end;
        RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, no_value_calc, 0);
        if (p > minpmt) then
          goto ABORT;
        h^.lastdate := NextPayment.date; {otherwise computation has failed}
        h^.laststatus := outp;
        npre := save_balloon.save_npre;
        for i := 1 to npre do
          if (pre[i]^.stopdatestatus<defp) then
            with pre[i]^ do
              begin
                calc_pre[i].stopdate := nextdate;
                while (DateComp(h^.lastdate, calc_pre[i].stopdate) < 0) do
                  AddPeriod(calc_pre[i].stopdate, pre[i]^.peryr, startdate.d, subtract);
                        {go back to on or before last regular payment date}
                calc_pre[i].nn := NumberOfInstallments(startdate, calc_pre[i].stopdate, pre[i]^.peryr, on_or_before);
              end;
        save_balloon.Restore; {Added 1/8/90}
        for i := 1 to npre do
          begin
                 {Must do this AFTER Restore, or it will be lost}
            pre[i]^.stopdate := calc_pre[i].stopdate;
            pre[i]^.stopdatestatus := outp;
            pre[i]^.nn := calc_pre[i].nn;
            pre[i]^.nnstatus := outp;
          end;
        if (nballoons > 0) and (DateComp(balloon[nballoons]^.date, h^.lastdate) > 0) then
          begin
                 {Computation of duration doesn't work when there's a balloon}
        {after the last payment date.  Fix this here some time.}
        {8/21/90, vx 1.10a-8}
{$ifdef BUGSIN}
        Scavenger('C-6');
{$endif}
          end;
        h^.nperiods := NumberOfInstallments(h^.firstdate, h^.lastdate, h^.peryr, on_or_before);
        h^.nstatus := outp;
      end
    else {not fancy}
      begin
        f := GrowthPerPeriod;
        if (d * h^.peryr < 1.001 * p * h^.loanrate) then
          goto ABORT
        else
          begin
            ff := 1 + (f - 1) * prorate;
            p1 := p * ff - d;    {principal remaining after first payment}
            ff := 1 / f;
            if (abs(1 - ff) < teeny) then
              h^.nperiods := round(1.4999+p1/d)
            else
              h^.nperiods := round(1.4999 + lnn(1 - p1*(1-ff)/(ff*d)) / lnn(ff) );
                  { .5 to round up, 1 because first period separated.}
            AddNPeriods(h^.firstdate, h^.lastdate, h^.peryr, pred(h^.nperiods));
            h^.nstatus := outp; h^.laststatus:=outp;
          end;
      end;
    DetermineVeryLast;
    DetermineLastPaymentDate := (h^.laststatus>empty);
    goto DONE;

ABORT:
    MessageBox('Payment amount is too small to compute number of periods.', DA_PaymentTooSmall);
    DetermineLastPaymentDate := false;
    h^.nperiods := ERROR;
    errorflag := true;

DONE:
    if (fancy) then save_balloon.Free;
  end;

 (*  Linear version - frozen as of 6/11/90 *)
  { Iterate -- the universal backward solver.  Refines the var parameter x
    (which may be the payment d, a balloon amount, the loan amount, or the
    interest rate) by a secant/Newton-style root find: each pass runs a full
    repayment walk (RepayFancyLoan or RepayLoan) and adjusts x so the terminal
    balance approaches zero.  Uses a finite-difference derivative (no analytic
    form).  Keeps the best-so-far x, bails out of diverging iterations early,
    and after 20 passes or convergence reports success/failure.  hard_payment
    is forced false during iteration (no penny-rounding) and restored after.
    target_is_loan_amount handles the special case where x IS the principal. }
  { Go port: dosport_walk.go: iterate (line 193) -- the general secant/Newton
    refinement of x (payment / balloon / amount / rate) driving the terminal
    balance to ~0, with the same finite-difference derivative, best-so-far keep,
    and divergence bail-out.  The schedule-oracle bisection variant used by the
    fancy backward solvers is fancybisect.go: dosIteratePayment (line 114) /
    fancyBisect (line 217). }
  function Iterate (p, usap: real; loandate, firstdate: daterec; var x: real; entire_or_no: boolean): boolean;
    {This is written as a general Newton's Method refinement of the var}
{parameter x, which can be payment, any balloon, or interest rate.}
{Based on difference rather than analytic derivative.}

    const
      small = 0.001;
      halfpenny = 0.005;
      acc_limit = 2E-8;

    var
      init, initusa, final, delta, newdelta, savex, bestp, bestx: real;
      count: integer;
      save_balloon: saved_balloon_state;
      target_is_loan_amount :boolean;
    label
      1;
  begin {Iterate}
    hard_payment:=false;  {temporarily, for iteration}
    save_balloon.Save;
    init := p;
    initusa := usap;
    f := GrowthPerPeriod;
    if (fancy) or ((df.c.exact) and (df.c.basis<>x360)) then
      RepayFancyLoan(p, usap, loandate, firstdate, nil, false, til_adj, no_value_calc, 0)
    else
      RepayLoan(p);
    save_balloon.Restore;
    if (abs(p) < halfpenny) then
      goto 1;
    final := p;
    delta := small * x;
    count := 0;
    x := x + delta;
    bestp := maxint;
    target_is_loan_amount:=EquivalentAddresses(@x,addr(h^.amount));
    repeat
      f:=GrowthPerPeriod; {in case it's the loan rate that's the target}
      inc(count);
      if (overflowflag) then
        goto 1;
      if (target_is_loan_amount) then p:=x else p := init;
      usap := initusa;
             {if (@d=@x) then}
      save_balloon.saved := d;
        {This is necessary here because sometimes d is changed}
        {in RepayFancyLoan, but sometimes d=x is the target}
        {that we're iterating to try to compute.}
      savex := x;
      if (fancy) or ((df.c.exact) and (df.c.basis<>x360)) then
        RepayFancyLoan(p, usap, loandate, firstdate, nil, false, entire_or_no, no_value_calc, 0)
      else
        RepayLoan(p);
      save_balloon.Restore; {Must restore before x:=x+delta, in case d=x is our target.}
      x := savex;
      if abs(final - p) > teeny then
        newdelta := delta * p / (final - p)
      else
        newdelta := 0;
      if (abs(delta) < teeny) or (abs(newdelta / delta) > 1) then
        count := count + 5;
                {Helps to get out of diverging iterations before they blow up.}
      delta := newdelta;
      x := x + delta;
      final := p;
      if (abs(p) < bestp) then
        begin
          bestp := abs(p);
          bestx := x;
        end;
    until (count >= 20) or (bestp < halfpenny) or (abs(h^.loanrate) > 2);
    x := bestx;
    if (bestp > halfpenny) and (bestp > acc_limit * init) then
      begin
        MessageBox('Computation of payment amount or interest rate did not converge.', DA_PayOrInterestNoConverge);
        Iterate := false;
      end
    else
      Iterate := true;
1:
    save_balloon.Free;
    hard_payment:=(h^.payamtstatus=inp); {because it was made false temporarily for iteration.}
  end;

  { Re_Amortize -- apply the ARM adjustment adj[next_adj] partway through a
    schedule walk.  Resets the running rate (and recomputes truerate) and/or
    payment to the adjustment's values; when the adjusted payment is blank it
    is recomputed so the loan still retires by the last date (closed-form
    estimate refined by Iterate when balloons/prepayments/exact-day make the
    estimate inexact).  Restores the extra-payment cursor from the old_*
    snapshot (because the walk steps back one payment), re-runs ComputeNext for
    the boundary period, and advances next_adj.  Forward-declared because
    DecideWhetherToPrintALine calls it recursively at each adjustment date. }
  { Go port: dosport_walk.go: reAmortize (line 264) -- applies the ARM adjustment
    mid-walk: resets rate/truerate and/or payment, recomputes a blank adjusted
    payment (closed-form estimate refined by iterate when balloons/prepays/exact-
    day make it inexact), restores the extra-payment cursor from the snapshot,
    re-runs computeNext for the boundary period, and advances next_adj. }
  procedure Re_Amortize (var p: real);
    var
      t: daterec;
      n: integer;
      i, save_nballoons: integer;
      adjp, denom: real;
      {SaveNextPayment :paymenttype;}

  begin
    p := Payment.principal;
    usap := Payment.usaprinc;
    if (adj[next_adj]^.loanratestatus >= defp) then
      begin
        h^.loanrate := adj[next_adj]^.loanrate;
        ComputeTrueRate;
      end;
    if (adj[next_adj]^.amtok) then
      begin
        d := adj[next_adj]^.amount;
        if (adj[next_adj]^.loanratestatus < outp) then
             {If it's already outp, we don't want to re-compute it.
              This saves time and it's essential for APR value calculation. }
          begin
            {SaveNextPayment :=NextPayment;}
            if Iterate(p, usap, payment.date, nextpayment.date, h^.loanrate, til_adj) then
              begin
                adj[next_adj]^.loanrate := h^.loanrate;
                adj[next_adj]^.loanratestatus := outp;
                f := GrowthPerPeriod;
              end
            else
              begin
                errorflag:=true;
              end;
            {NextPayment:=SaveNextPayment;}
            p := h^.amount;
            usap := 0;
          end
        else
          if (adj[next_adj]^.loanratestatus = outp) then
            begin
              h^.loanrate := adj[next_adj]^.loanrate;
              ComputeTrueRate;
              f := GrowthPerPeriod;
            end;
      end
    else { not (adj[next_adj]^.amtok) - compute new payment amount}
      begin
        n := NumberOfInstallments(adj[next_adj]^.date, h^.lastdate, h^.peryr, on_or_after);
        f := GrowthPerPeriod;
        adjp := p;
        next_balloon := old_next_balloon;

        if (old_npre > 0) then
          begin
            npre := old_npre;
            for i:= 1 to npre do
              pre[i]^ := old_pre[i]^;
          end
        else
          npre := 0;

        for i := next_balloon to user_nballoons do
          adjp := adjp - balloon[i]^.amount * exxp(-RateFromYield(h^.loanrate, h^.peryr)
                                            * YearsDif(balloon[i]^.date, adj[next_adj]^.date));

        denom := (1 - exxp(-pred(n) * lnn(f)));
        if (abs(denom) < teeny) then
          d := adjp / pred(n)
        else
          d := adjp * (f - 1) / denom;

        if (user_nballoons > 0) or (npre > 0) or ((df.c.exact) and (df.c.basis<>x360)) then {or (df.c.in_advance)}
          begin
            save_nballoons := nballoons;
            nballoons := user_nballoons;
            t := NextPayment.date;
            n := NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after);
            if Iterate(p, usap, Payment.date, t, d, til_adj) then
              begin
                adj[next_adj]^.amount := d;
                adj[next_adj]^.amountstatus := outp;
                adj[next_adj]^.amtok := true;
              end
            else
              begin
                abort := true;
                errorflag := true;
              end;
            nballoons := save_nballoons;
          end;

        adj[next_adj]^.amount := d;
        adj[next_adj]^.amountstatus := outp;
      end; {End things to do if d needs to be computed.}

    next_balloon := old_next_balloon;
    for i:=1 to old_npre do begin
      if( old_pre[i] <> nil ) then
        pre[i]^ := old_pre[i]^;
    end;
    npre := old_npre;
      {Because we're going back one payment, and we may have passed}
      {a balloon last time we ran ComputeNext}

    NextPayment := Payment;
{
    if (df.c.in_advance) then
      NextPayment.ComputeNextForAdvancedInterestPayment(p)
    else
}
      NextPayment.ComputeNext(p, usap);
    inc(next_adj);
    p := NextPayment.principal;
  end;

{$ifdef 0}
Obviously we don't want this one.  This screen will be created
by the windows stuff outside this layer

{ Go port: n/a -- DOS text-mode screen painter (dead code here, inside $ifdef 0);
  the web frontend renders the amortization screen. }
procedure AmortizationScreen;
          var attr :byte;
          begin
          window(1,1,80,25); gotoxy(1,1);
          block:=AMZTopBlock;
{$ifdef TOPMENUS} DrawMenuBar; gotoxy(1,2); {$endif}
          textattr:=MainScreenColors;
          if (color) then attr:=(brown shl 4 + white) else attr:=112;
          write('ÚÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄ¿');
{$ifdef TOPMENUS}
          FastWrite('AMORTIZATION SCREEN',1,60,attr);
          write(
{$else}
          FastWrite('AMORTIZATION SCREEN',1,3,attr);
          write('³                                                                              ³',
{$endif}
                '³    Amount     Loan    Loan   1st Pmt  #of   Last   Pmts                      ³',
                '³   Borrowed    Date    Rate %   Date   Pds Pmt Date /Yr  Payment   Pts  APR % ³',
                'ÔÍÍÍÍÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÑÍÍÍÑÍÍÍÍÍÍÍÍÑÍÍÑÍÍÍÍÍÍÍÍÍÍÑÍÍÍÍÍÑÍÍÍÍÍÍ¾',
                '             ³        ³       ³        ³   ³        ³  ³          ³     ³       ',
                'ÚÄÄÄÄÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÁÄÄÄÄÄÄÄÄÄÄÁÄÄÄÄÄÁÄÄÄÄÄÄ¿');

          UpdateSettings;
          npre:=0;
          if (fancy) then ExtraLines
          else ClearLowerAMZScreen;
          Menu; {UpdateSettings; memory.PlaceOnScreen;}
          DisplayAll;
          Home(block);
          end;
{$endif}
begin

{ Unit initialization: clear the old_pre[] heap-pointer cache so the first
  SaveDataForReAmortize allocates fresh records (avoids freeing garbage). }
for old_npre := 1 to maxprepay do
  old_pre[old_npre]:=nil;

end.

