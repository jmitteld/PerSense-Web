{******************************************}
{*                                        *}
{* UNIT AMORTIZE                          *}
{*                                        *}
{* Amortization screen for Per%Sense      *}
{*                                        *}
{*                                        *}
{******************************************}


unit AMORTIZE;
{$ifdef OVERLAYS} {$F+,O+} {$endif}

{ ============================================================================
  UNIT AMORTIZE  --  Amortization screen logic / field-presence dispatcher.

  This unit is the high-level controller for the Amortization screen.  It owns
  the "what do I solve for?" decision, while the actual repayment math lives in
  unit AMORTOP (the engine).

  THE FIELD-PRESENCE DISPATCH PATTERN (the app's defining feature):
  The user fills in some of the loan's fields (amount borrowed, loan date,
  rate, first-payment date, # of payments / last date, payments-per-year,
  payment amount, points, APR) and leaves exactly one unknown blank.  Each
  cell carries a status (empty / outp / defp / inp).  FirstPass classifies the
  inputs; Enter then routes to the matching backward solver in AMORTOP:
    - rate blank      -> EstimateAndRefineRate
    - payment blank   -> EstimateAndRefinePayment
    - amount blank    -> EstimateAndRefineLoanAmount
    - term blank      -> DetermineLastPaymentDate
    - balloon blank   -> EstimateAndRefineBalloon
    - prepay blank    -> EstimateAndRefinePeriodic Prepayment / Duration
  plus ARM (adjustment) payment/rate solves and an APR-with-points solve.

  ADVANCED OPTIONS handled here + in AMORTOP: balloons, periodic prepayments,
  rate/payment adjustments (ARMs), moratorium (interest-only deferral),
  principal-reduction target, and skip-months.  Presence of any advanced
  option puts the loan in "fancy" mode, using RepayFancyLoan rather than the
  fast closed-form RepayLoan.

  Key shared records (defined in PEDATA/PETYPES/Globals):
    h^   loan header (amounts, dates, rate, payment, n, points, apr + *status)
    mor^ moratorium, targ^ target, skp^ skip-months, balloon[], pre[], adj[]
    df.c computational settings (basis, prepaid, in_advance, R78, USARule, ...)

  Rounding: Round2 = round-HALF-DOWN (truncate at .5), applied to "hardened"
  amounts only when the top-line payment is a hard number (the Dav Holle rule).
  ============================================================================ }

INTERFACE

//uses OPCRT,OPMOUSE,VIDEODAT,NORTHWND,PETYPES,PEDATA,INPUT,INTSUTIL,PEPANE,COMDUTIL,IOUNIT,
//{$ifdef NO_FRILLS} HTXTSTUB, {$else} HTXTHELP, {$endif}
//{$ifdef CHRONO} MCMENU, {$endif}
//KCOMMAND,LOTUS,AMZUTIL,AMORTOP,TABLE
//{$ifdef TESTING} ,TESTHIGH {$endif}
uses VIDEODAT,PETYPES,PEDATA,INTSUTIL,AMORTOP,Globals, Classes;


procedure Enter(code :byte);                          { main controller: classify, dispatch a solve, optionally build the table }
procedure PrepareScreen(code :byte);                  { stash the fancy/plain code before the screen is drawn }
function EstimateAndRefineAPRwithPoints: boolean;     { solve effective APR given points/prepaid interest }
function SufficientDataOnScreen: boolean;             { is there enough known data to compute a schedule? }
{$ifdef TESTING}
procedure FirstPass;                                  { classify inputs, set status flags, prep advanced options }
{$endif}
procedure MakeTable( Output: TStringList; bCommaSeperated: boolean );  { run Enter then emit the amortization rows }
procedure ZeroAMZLoan( AMZ: AMZPtr );                 { reset a loan-header record to all-blank }
procedure ZeroMoratorium( Moratorium: moratoriumptr );{ reset moratorium record to blank }
procedure ZeroTarget( Target: targetptr );            { reset principal-reduction target to blank }
procedure ZeroSkip( Skip: skipptr );                  { reset skip-months record to blank }
procedure ZeroBalloon( balloon: balloonptr );         { reset a balloon record to blank }
procedure ZeroPrepayment( Prepayment: prepaymentptr );{ reset a prepayment series record to blank }
procedure ZeroAdjustment( Adjustment: adjptr );       { reset a rate-adjustment (ARM) record to blank }
function AdjustmentIsEmpty( pADJ: adjptr ): boolean;  { true if an adjustment row has no user data }
function BalloonIsEmpty( pBalloon: balloonptr ): boolean;  { true if a balloon row has no user data }
function PrepaymentIsEmpty( pPre: prepaymentptr ): boolean; { true if a prepayment row has no user data }

IMPLEMENTATION

uses HelpSystemUnit;

{ ZeroAMZLoan -- reset every field of a loan-header record to its blank state
  (status:=empty, value:=0 or unkdate).  Used when clearing the screen or
  initializing a new loan. }
procedure ZeroAMZLoan( AMZ: AMZPtr );
begin
  with AMZ^ do begin
  amountstatus          := empty;
  amount                := 0;
  loandatestatus        := empty;
  loandate              := unkdate;
  loanratestatus        := empty;
  loanrate              := 0;
  firststatus           := empty;
  firstdate             := unkdate;
  nstatus               := empty;
  nperiods              := 0;
  laststatus            := empty;
  lastdate              := unkdate;
  peryrstatus           := empty;
  peryr                 := 0;
  payamtstatus          := empty;
  payamt                := 0;
  pointsstatus          := empty;
  points                := 0;
  aprstatus             := empty;
  apr                   := 0;
  end;
end;

{ ZeroMoratorium -- blank the moratorium (interest-only deferral) record. }
procedure ZeroMoratorium( Moratorium: moratoriumptr );
begin
  Moratorium.first_repaystatus     := empty;
  Moratorium.first_repay           := unkdate;
end;

{ ZeroTarget -- blank the principal-reduction target record. }
procedure ZeroTarget( Target: targetptr );
begin
  Target.targetstatus          := empty;
  Target.target                := 0;
end;

{ ZeroSkip -- blank the skip-months record (the month-list string). }
procedure ZeroSkip( Skip: skipptr );
begin
  Skip.skipstatus            := empty;
  Skip.skipmonths            := '';
end;

{ ZeroBalloon -- blank one balloon record (date + amount). }
procedure ZeroBalloon( balloon: balloonptr );
begin
  with balloon^ do begin
  datestatus            := empty;
  date                  := unkdate;
  amountstatus          := empty;
  amount                := 0;
  end;
end;

{ BalloonIsEmpty -- true if neither date nor amount of a balloon was entered. }
function BalloonIsEmpty( pBalloon: balloonptr ): boolean;
begin
  BalloonIsEmpty := (pBalloon.datestatus=empty) and (pBalloon.amountstatus=empty);
end;

{ ZeroPrepayment -- blank one periodic-prepayment series record. }
procedure ZeroPrepayment( Prepayment: prepaymentptr );
begin
  with prepayment^ do begin
  startdatestatus       := empty;
  startdate             := unkdate;
  nnstatus              := empty;
  nn                    := 0;
  stopdatestatus        := empty;
  stopdate              := unkdate;
  peryrstatus           := empty;
  peryr                 := 0;
  paymentstatus         := empty;
  payment               := 0;
  nextdate              := unkdate;
  end;
end;

{ PrepaymentIsEmpty -- true if no field of a prepayment series was entered. }
function PrepaymentIsEmpty( pPre: prepaymentptr ): boolean;
begin
  PrepaymentIsEmpty := (pPre.startdatestatus=empty) and (pPre.nnstatus=empty) and
                       (pPre.stopdatestatus=empty) and (pPre.peryrstatus=empty) and
                       (pPre.paymentstatus=empty);
end;

{ ZeroAdjustment -- blank one rate-adjustment (ARM) record. }
procedure ZeroAdjustment( Adjustment: adjptr );
begin
  with Adjustment^ do begin
  datestatus            := empty;
  date                  := unkdate;
  loanratestatus        := empty;
  loanrate              := 0;
  amountstatus          := empty;
  amount                := 0;
  amtok                 := false;
  end;
end;

{ AdjustmentIsEmpty -- true if no field of a rate adjustment was entered. }
function AdjustmentIsEmpty( pADJ: adjptr ): boolean;
begin
  AdjustmentIsEmpty := (pAdj.datestatus=empty) and (pAdj.loanratestatus=empty) and
                       (pAdj.amountstatus=empty);
end;

  { MonthSetFromString -- parse a skip-months string such as "6-8,12" into a
    set of month numbers (1..12).  Supports comma-separated values and "a-b"
    ranges, including ranges that wrap the year boundary (e.g. "11-2" => Nov,
    Dec, Jan, Feb).  Returns false on a malformed string or out-of-range month;
    monthset is filled with the parsed months.
      s        : the user's skip-months text
      monthset : (out) parsed set of month numbers }
  function MonthSetFromString(s :str15; var monthset :byteset):boolean;
    var
      i,n,nn           :byte;
      ws               :string[2];
      thruflag         :boolean;
  begin
    MonthSetFromString:=false;
    monthset:=[];
    i:=0; nn:=0; thruflag:=false;
    while (i<length(s)) do begin
      repeat inc(i) until (i>length(s)) or (s[i] in intcharset);
      if (s[i]='-') then begin
        thruflag:=true;
        repeat inc(i) until (i>length(s)) or (s[i] in digitset);
        end
      else thruflag:=false;
      ws:=s[i];
      inc(i);
      if (s[i] in digitset) then ws:=ws+s[i]
      else if (s[i]='-') then dec(i);
      n:=round(value(ws));
      if (n>=1) and (n<=12) then begin
        if (thruflag) then begin
          if (nn=0) then exit; {return false}
          if (nn<=n) then monthset:=monthset+[nn..n]
          else monthset:=monthset+[nn..12]+[1..n];
          end
        else monthset:=monthset+[n];
        nn:=n;
        end
      else exit;{n out of range}
      end;
    MonthSetFromString:=true;
  end;

  { DefaultFirstPaymentDate -- when the user leaves the first-payment date
    blank, default it.  Start from the loan date; if the loan day-of-month is
    past the 1st, snap to the 1st and add a period; then add one more period,
    so the first payment is at least a full period after settlement.  Marks
    firststatus as defp (defaulted, not user input). }
  procedure DefaultFirstPaymentDate;
  begin with h^ do begin
     firstdate:=loandate;
     if (firstdate.d>1) then begin
       firstdate.d:=1;
       AddPeriod(firstdate,peryr,1,false);
       end;
     AddPeriod(firstdate,peryr,1,false);
     firststatus:=defp;
//     DisplayCell(amztopblock,1,firstdatecol);
  end;end;

  { FirstPass (a.k.a. GetNumbers) -- the classification / preprocessing pass.
    Reads the raw screen fields and:
      * resolves the prepaid flag from the settings (in_advance forces prepaid);
      * defaults the first-payment date if needed;
      * cross-fills #periods <-> last date (whichever is blank, given the other);
      * computes the true (effective) rate for DAILY compounding;
      * in fancy mode: parses skip-months, classifies prepayments
        (CheckPrepayments), snaps each ARM date to a payment date, sorts
        balloons and adjustments;  in plain mode: zeroes all advanced options;
      * forces a 365-day basis for weekly/biweekly payments;
      * records user_nballoons (the count the user typed, before any auto-tack).
    Side effects: sets errorflag/overflowflag, the *status fields, and the many
    AMORTOP globals (nballoons/npre/nadj/unkpre/unkballoon/skipmonthset/...).
    This is the heart of the field-presence dispatch -- Enter reads the flags
    it sets to decide which backward solve to run. }
  procedure FirstPass;
    var
      i: integer;
      t: daterec;
      ta: integer;
  begin {FirstPass}
//    if (h=nil) then AddRow(AMZTopBlock);
    { ClearOutputCells; experimental removal of this line to after preprocessing in Enter() 6/6/91}
    errorflag := false; overflowflag:=false;
    balance_calc:=false;
    if (df.c.in_advance) then
      prepaid := true
    else
      prepaid := df.c.prepaid;
//    if (mor=nil) then AddRow(AMZMoratoriumBlock);
//    if (targ=nil) then AddRow(AMZTargetBlock);
//    if (skp=nil) then AddRow(AMZSkipMonthBlock);
    {These two lines not exactly in keeping with the spirit of data management
     in PE vx 2, but this is an easy solution, and it only costs a few bytes.}
    aprpmtsum:=0;
    with h^ do
      begin
        if (firststatus < defp) and (loandatestatus>defp) and (peryrstatus>=defp)
          then DefaultFirstPaymentDate;
        { Cross-fill term: if #periods is known, derive the last date; if the
          last date is known, derive #periods.  If neither, the term is the
          unknown to be solved later (lastok stays false). }
        if (firststatus >= defp) and (nstatus >= defp) then
          begin
            AddNPeriods(h^.firstdate, lastdate, peryr, pred(h^.nperiods));
            laststatus := outp;
//            DisplayCell(amztopblock,1,lastdatecol);
            lastok := true;
          end
        else if (firststatus >= defp) and (laststatus >= defp) then
          begin
            h^.nperiods := NumberOfInstallments(h^.firstdate, lastdate, peryr, on_or_before);
            if (h^.nperiods<0) then begin
              MessageBox('Your last payment comes before your first.', DA_LastPayBeforeFirst);
              RecordError(succ(ztop[AMZTopBlock]), lastdatecol);
              nstatus := empty;
              end
            else begin
              nstatus := outp;
//              DisplayCell(amztopblock,1,pdnumcol);
              lastok := true;
              end;
          end
        else
          begin
            lastok := false;
            lastdate.m := unkbyte;
          end;
        next_balloon := 1;
        next_adj := 1;
        if (loanratestatus>=defp) then
          ComputeTrueRate;
        if (fancy) then
          begin
            { Parse the skip-months string into skipmonthset (or empty). }
            if (skp^.skipstatus >= defp) then begin
              if not MonthSetFromString(skp^.skipmonths,skipmonthset)
                then RecordError(succ(ztop[AMZSkipMonthBlock]),skipmonthcol);
              end
            else skipmonthset:=[];
            CheckPrepayments;
            { Snap each ARM adjustment date onto a regular payment date (a rate
              change can only take effect on a payment date); flag it as defp
              if it had to be moved. }
            for i := 1 to (nlines[AMZadjblock]) do
              begin
                if (adj[i]^.datestatus >= defp) and (h^.firststatus >= defp) then
                  begin
                    t := adj[i]^.date;
                    ta := NumberOfInstallments(h^.firstdate, adj[i]^.date, peryr, on_or_before);
                    if (DateComp(t, adj[i]^.date) <> 0) then
                      adj[i]^.datestatus := defp;
                          {Let user know we've adjusted rate change date to be on a payment date.}
                  end;
                if (adj[i]^.loanratestatus>0) and (adj[i]^.loanrate = error) then
                  RecordError(i + scrollpos[AMZadjblock] + ztop[AMZAdjBlock], adjaprcol);
                adj[i]^.amtok:= (adj[i]^.amountstatus>=defp);
              end;
            SortAdj(nlines[AMZadjblock]);
            SortBalloons(nlines[AMZballoonblock]);
          end {if fancy}
        else {not fancy}
          begin
            { Plain loan: force every advanced option off/empty so the fast
              closed-form path is used. }
            nballoons := 0;
            balloonsok := true;
            unkballoon := 0;
            npre := 0;
            preok := true;
            unkpre := 0;
            adj_fully_specified := (payamtstatus >= defp);
            nadj := 0;
            adjok := true;
            mor^.first_repaystatus := empty;
            mor^.first_repay.m := unkbyte;
            targ^.targetstatus := empty;
            targ^.target := BLANK;
          end;
        if (targ^.targetstatus < defp) then
          targ^.target := -infinity; {less than everything}
           {for i:=1 to nadj do adjamtstatus[i]:=(adj[i]^.amountstatus>=defp); Not nec in Mac}
        user_nballoons := nballoons;
             {This is the number of balloons that the user has put on the}
      {screen, and it doesn't change when a balloon is tacked on.}
        { Weekly/biweekly payments are incompatible with a 360-day basis;
          silently switch to 365 (the inverse $ifdef CHEAP path forces 360
          back for non-weekly loans). }
        if (peryr in [26, 52]) and (df.c.basis=x360) then
          begin
{$ifndef CHEAP}
            MessageBox('Changing to 365 day basis for weekly/biweekly payments.', DA_ChangeTo365);
{$endif}
            df.c.basis := x365;
            SetYrDays;
            UpdateSettings;
          end;
{$ifdef CHEAP}
        if (not (peryr in [26, 52])) and (df.c.basis=x365) then
          begin
            df.c.basis := x360;
            SetYrDays;
            UpdateSettings;
          end;
{$endif}
 {$ifdef TESTING}
        TESTHIGH.npretest := npre;
        TESTHIGH.nballoonstest := nballoons;
        TESTHIGH.nadjtest := nadj;
 {$endif}
      end; {with h}
  { hard_payment is true only when the user typed an exact payment amount (inp);
    it enables the round-half-down penny treatment of interest/amounts. }
  hard_payment:=(h^.payamtstatus=inp);
  end; {First pass}


  { EstimateAndRefineAdjPayment -- compute the new payment that applies after
    ARM rate change #adjnum, by amortizing up to that adjustment (til_adj) and
    letting Re_Amortize inside RepayFancyLoan recompute the payment.  Saves and
    restores the balloon/prepay cursor state and the loan rate.  Returns true
    on success (errorflag clear).  adjnum is the 1-based adjustment index. }
  function EstimateAndRefineAdjPayment (adjnum: integer): boolean;
   {Estimate what the new monthly payment should be,}
   {after rate change #adjnum, and Refine with Iterate, above.}
    var
      save_rate: real;
      adj_save_balloon: saved_balloon_state;

  begin
    adj_save_balloon.Save;
    save_rate := h^.loanrate;
    p := h^.amount;
    usap := 0;
    f := GrowthPerPeriod;
    RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, til_adj, no_value_calc, adjnum);
    EstimateAndRefineAdjPayment := (not errorflag);
    p := h^.amount;
    usap := 0;
    adj_save_balloon.Restore;
    h^.loanrate := save_rate;
    ComputeTrueRate;
    adj_save_balloon.Free;
  end;

  { EstimateAndRefineAdjRate -- the inverse of the above: given the payment the
    user fixed at adjustment #adjnum, fit the loan rate that produces it,
    again by walking til_adj and letting Re_Amortize solve.  Saves/restores
    cursor state and rate.  Returns true on success. }
  function EstimateAndRefineAdjRate (adjnum: integer): boolean;
   {Fit an adjusted loan rate to the new payment amount, for adj[adjnum].}
   {Refine with Iterate, above.}
    var
      save_rate: real;
      adj_save_balloon: saved_balloon_state;

  begin
    adj_save_balloon.Save;
    save_rate := h^.loanrate;
    p := h^.amount;
    usap := 0;
    f := GrowthPerPeriod;
    RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, til_adj, no_value_calc, adjnum);
    EstimateAndRefineAdjRate := (not errorflag);
    p := h^.amount;
    usap := 0;
    adj_save_balloon.Restore;
    h^.loanrate := save_rate;
    ComputeTrueRate;
    adj_save_balloon.Free;
  end;

  { FirstLastAndFF -- helper that computes the three discount factors used in
    the closed-form value of prepayment series i:
      first = discount factor at the series' start date,
      last  = discount factor at its stop date,
      ff    = per-period discount factor (exp(-rate/periods)).
    These feed the geometric-series formula value = pmt*(first-last*ff)/(1-ff).
    rate is the continuous discount rate; all relative to repay_from. }
  procedure FirstLastAndFF (var rate, first, last, ff: real; i: integer);
  begin
    first := exxp(-rate * YearsDif(pre[i]^.startdate, repay_from));
    last := exxp(-rate * YearsDif(pre[i]^.stopdate, repay_from));
    ff := exxp(-rate / pre[i]^.peryr);
  end;

  { EstimateAndRefinePayment -- solve for the regular payment d when it is the
    blank field.  Forms a closed-form estimate: subtract the present value of
    all balloons and prepayments from the loan amount, then divide by the
    annuity factor (with a small-rate guard).  For a simple loan (no advanced
    options, prepaid, not exact, not in-advance) that estimate is exact and is
    returned directly; otherwise it is refined to a zero terminal balance with
    Iterate.  Returns false if Iterate fails to converge. }
  function EstimateAndRefinePayment: boolean;
 {Estimate what the monthly payment should be, and Refine with Iterate, above.}
    var
      i: integer;
      adjp, rate, first, last, ff, denom, savetarget: real;
  begin
    EstimateAndRefinePayment := true;
    p := h^.amount;
    usap := 0;
    adjp := h^.amount;
    rate := RateFromYield(h^.loanrate, h^.peryr);
    { Reduce the amount to be amortized by the present value of every balloon }
    for i := 1 to user_nballoons do
      adjp := adjp - balloon[i]^.amount * exxp(-rate * YearsDif(balloon[i]^.date, repay_from));
    { ...and of every prepayment series (geometric series; near-zero-rate uses
      the simple count*payment limit). }
    for i := 1 to npre do
      begin
        FirstLastAndFF(rate, first, last, ff, i);
        if (abs(1-ff)>teeny) then
          adjp := adjp - pre[i]^.payment * (first - last * ff) / (1 - ff)
        else adjp := adjp - pre[i]^.payment * pre[i]^.nn;
      end;
    { Divide the remaining principal by the ordinary-annuity factor to get the
      estimated regular payment (denom~0 => degenerate, use straight division). }
    denom := (1 - exxp(-nrepay * lnn(f)));
    if (abs(denom) < teeny) then
      d := adjp / nrepay
    else
      d := adjp * (f - 1) / denom;
    if (not df.c.exact) and (prepaid) and (nballoons = 0) and (npre = 0)
       and (not df.c.in_advance) and (targ^.targetstatus<=empty) and (skipmonthset=[]) then
      begin
        h^.payamt := d;
        exit;
      end;
(*
2/94 This was removed.  It made it impossible to find payment
when TPR specified.  Why was it put into 2.05?
    if (fancy) then begin {Find payment irrespective of principal reduction target}
      savetarget:=targ^.target;
      targ^.target:=-infinity;
      end;
*)
    if Iterate(p, usap, h^.loandate, h^.firstdate, d, til_adj) then
{$ifdef BOFA}
       begin d:=d+0.01; Round2(d); h^.payamt := d; end
{$else}
      h^.payamt := d
{$endif}
    else
      begin
        errorflag := true;
        EstimateAndRefinePayment := false;
      end;
(* 2/94 see above
    if (fancy) then targ^.target:=savetarget;
*)
  end;

  { EstimateAndRefineLoanAmount -- solve for the principal (amount borrowed)
    when it is blank: the present value of the regular payment stream (annuity
    factor * d) plus the PV of all balloons and prepayments.  Fails if the rate
    is so small the annuity factor is degenerate.  For the simple 360/non-exact
    arrears case the closed form is exact; otherwise refine with Iterate. }
  function EstimateAndRefineLoanAmount: boolean;
 {Estimate what the loan amount should be, and Refine with Iterate, above.}
    var
      i: integer;
      padj, rate, first, last, ff, numerator: real;
  begin
    if (abs(f - 1) < tiny) then
      begin
        MessageBox('Cannot determine loan amount - interest rate too small.', DA_InterestTooSmall);
        EstimateAndRefineLoanAmount := false;
        exit;
      end;
    EstimateAndRefineLoanAmount := true;
    p := h^.amount;
    usap := 0;
    padj := 0;
    rate := RateFromYield(h^.loanrate, h^.peryr);
    for i := 1 to user_nballoons do
      padj := padj + balloon[i]^.amount * exxp(-rate * YearsDif(balloon[i]^.date, repay_from));
    for i := 1 to npre do
      begin
        FirstLastAndFF(rate, first, last, ff, i);
        padj := padj + pre[i]^.payment * (first - last * ff) / (1 - ff);
      end;
    numerator := (1 - exxp(-nrepay * lnn(f)));
    h^.amount := numerator / (f - 1) * d + padj;
    if ((df.c.basis=x360) or (not df.c.exact)) and (prepaid) and (nballoons = 0) and (npre = 0) and (not df.c.in_advance) then
      exit;
    if not Iterate(h^.amount, usap, h^.loandate, h^.firstdate, h^.amount, til_adj) then
      begin
        errorflag := true;
        EstimateAndRefineLoanAmount := false;
      end;
  end;

  { EstimateAndRefineRate -- solve for the loan rate when it is blank.  Seeds a
    deliberately-high first guess (payment*periods/amount, floored at 2% so the
    iteration doesn't start at zero), then drives Iterate to the rate that
    retires the loan exactly.  On success sets loanratestatus:=outp and
    recomputes the daily true rate; on failure clears the rate. Errors out for
    a zero-principal loan. }
  function EstimateAndRefineRate: boolean;
  begin with h^ do begin
    if (abs(amount)<tiny) then
      begin
        MessageBox('Rate cannot be computed for a loan of zero.', DA_ZeroRateLoan);
        errorflag:=true;
        exit;
      end;
    loanrate := payamt * peryr / amount; {first guess - better high than low.}
    if (loanrate<0.02) then loanrate := 0.02; {Iterate won't work if you start with zero.}
    if Iterate(amount, usap, loandate, firstdate, loanrate, til_adj) then
      begin
        EstimateAndRefineRate := true;
        loanratestatus := outp;
        f := GrowthPerPeriod;
        ComputeTrueRate;
      end
    else
      begin
        EstimateAndRefineRate := false;
        loanratestatus := empty;
      end;
    p := amount;
    usap := 0;
  end; end;

  { CalculateValueForPlainLoan -- closed-form present value of the regular
    payment stream discounted at the trial rate v_rate, used by the APR solver
    for plain (non-fancy, 360-basis, non-exact) loans.  For a near-zero rate it
    uses a second-order Taylor approximation of the annuity factor to avoid
    catastrophic cancellation; otherwise the exact geometric-series form.
    Prepaid interest is intentionally omitted (it lives in the target). }
  procedure CalculateValueForPlainLoan;
    const sixth=1/6;
    var
      first, last, ff: real;
      n: integer;
  begin
    first := exxp(-v_rate * YearsDif(h^.firstdate, h^.loandate));
    if (abs(v_rate) < small) then
      begin
            {Second order approx to (1-exp(-n*v_rate/h^.peryr)) / (1-exp(-v_rate/h^.peryr)) }
        n := round(YearsDif(h^.lastdate, h^.loandate) * RealPerYr(h^.peryr));
        ff := v_rate / RealPerYr(h^.peryr);
        aprvalue := first * d * n * (1 - half * n * ff + sixth * (sqr(succ(n)) + half) * sqr(ff));
      end
    else
      begin
        ff := exxp(-v_rate / RealPerYr(h^.peryr));
        last := exxp(-v_rate * YearsDif(h^.lastdate, h^.loandate));
        aprvalue := d * (first - last * ff) / (1 - ff);
      end
           {Omit prepaid interest - it's in target}
  end;

  { EstimateAndRefineAPRwithPoints -- compute the effective APR (Reg-Z style)
    that equates the discounted value of all payments to the net amount the
    borrower actually received: amount*(1-points) - prepaid interest.  Solves
    by a secant iteration on v_rate (the discount rate): each pass values the
    schedule (via RepayFancyLoan with value_calc, or CalculateValueForPlainLoan)
    and steps v_rate toward the target, up to 20 iterations.  On convergence,
    converts v_rate to a yield and stores it in h^.apr.  Under $ifdef BOFA the
    APR is always computed on a 360 basis and the terminal balloon is hardened.
    Returns false if it fails to converge. }
  function EstimateAndRefineAPRwithPoints: boolean;
    const
      small = 0.0001;
    var
      save_rate, delta, newdelta, oldvalue, target, denom: real;
      count, py: integer;
      save_balloon: saved_balloon_state;
{$ifdef BOFA}
      savebasis: basistype;
      balloon_hardened :boolean;
{$endif}
    label
      GET_OUT;
  begin
{$ifdef BOFA}
     {B of A always computes APRs on a 360 basis}
    savebasis:=df.c.basis;
    df.c.basis:=x360;
    SetYrDays;
    {The following is necessary because our trick in RepayFancyLoan for including an implied
     terminal balloon doesn't work with BOFA defined. }
    if (nlines[amzballoonblock]>nballoons) and (balloon[nlines[amzballoonblock]]^.datestatus=outp) then begin
      inc(nballoons);
      balloon[nballoons]^.amountstatus:=inp;
      balloon[nballoons]^.datestatus:=inp;
      balloon_hardened:=true;
      end
    else balloon_hardened:=false;
{$endif}
    save_balloon.Save;
    save_rate := h^.loanrate;
    target := h^.amount * (1 - h^.points) - PrepaidInterest;
             {PrepaidInterest function is 0 if prepaid is false}
    aprvalue := 0;
    if (h^.loanratestatus>defp) then v_rate := h^.loanrate {First guess}
    else v_rate:=0.1;
    p := h^.amount;
    if (fancy) or (df.c.exact) or (not (df.c.basis=x360)) then
      RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, value_calc, 0)
    else
      CalculateValueForPlainLoan;
    oldvalue := aprvalue;
    save_balloon.Restore;
    h^.loanrate := save_rate;
    ComputeTrueRate;
    f := GrowthPerPeriod;
    p := h^.amount;
    usap := 0;
    delta := small;
    count := 0;
    v_rate := v_rate + delta;
    repeat
      inc(count);
      if (overflowflag) then
        goto GET_OUT;
      aprvalue := 0;
      if (fancy) or (df.c.exact) or (not (df.c.basis=x360)) then
        RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, value_calc, 0)
      else
        CalculateValueForPlainLoan;
      p := h^.amount;
      usap := 0;
      h^.loanrate := save_rate;
      ComputeTrueRate;
      f := GrowthPerPeriod;
      denom := aprvalue - oldvalue;
      if (abs(denom) > teeny) then
        newdelta := (target - aprvalue) * delta / denom
      else
        newdelta := small;
      delta := newdelta;
      v_rate := v_rate + delta;
      oldvalue := aprvalue;
      save_balloon.Restore;
    until (count = 20) or (abs(delta) < teeny);
    if (abs(delta) < teeny) then
      EstimateAndRefineAPRwithPoints := true
    else
      EstimateAndRefineAPRwithPoints := false;
    if (abs(delta) < tiny) then
      begin
        if ((df.c.peryr and CanadianORdaily) > 0) then
          py := df.c.peryr
        else
          py := h^.peryr;
        h^.apr := YieldFromRate(v_rate, py);
        h^.aprstatus := outp;
{Print(ttop[10], aaprcol, outp, YieldFromRate(v_rate, py));}
      end;
GET_OUT:
    save_balloon.Free;
{$ifdef BOFA}
    df.c.basis:=savebasis;
    SetYrDays;
    if balloon_hardened then begin
      balloon[nballoons]^.amountstatus:=outp;
      balloon[nballoons]^.datestatus:=outp;
      end;
{$endif}
  end;

  { RateInForce -- the loan rate effective on a given date, accounting for ARM
    adjustments.  With no adjustments it is the base loanrate; otherwise it
    returns the rate of the latest adjustment whose date is on or before the
    requested date. }
  function RateInForce(date :daterec):real;
           var i :byte;
           begin
           if (nlines[AMZAdjBlock]=0) then RateInForce:=h^.loanrate
           else begin
              i:=1;
              while (i<nlines[AMZAdjBlock]) and (DateComp(date,adj[i]^.date)>=0) do inc(i);
              RateInForce:=adj[i]^.loanrate;
              end
           end;

  { EstimateAndRefineBalloon -- solve for the unknown balloon amount
    (balloon[unkballoon]).  Two cases:
      * the unknown balloon is the LAST event (its date = very_last): amortize
        with the balloon zeroed so the leftover principal+payment reveals
        exactly the balloon needed to retire the loan (netting the regular
        payment when plus_regular is set);
      * otherwise (a balloon mid-schedule): seed half the loan amount and drive
        Iterate over the entire schedule to a zero terminal balance.
    Sets the balloon's amountstatus to outp and balloonsok on success. }
  function EstimateAndRefineBalloon: boolean;
    var
      save_p: real;
      save_balloon: saved_balloon_state;

  begin
  if (DateComp(balloon[unkballoon]^.date,very_last)=0) then begin
      save_balloon.Save; save_p:=p;
      balloon[unkballoon]^.amount:=0; {for now - so that principal remaining tells us what balloon is needed.}
      if fancy then RepayFancyLoan(p,usap,h^.loandate,h^.firstdate,nil,false,entire,no_value_calc,0)
      else RepayLoan(p);
      save_balloon.Restore;
      balloon[unkballoon]^.amount:=nextpayment.payamt+nextpayment.principal;
      if (df.c.plus_regular) then balloon[unkballoon]^.amount:=balloon[unkballoon]^.amount-VeryLastRegularAmount;
{
      balloon[unkballoon]^.amount:=nextpayment.payamt;
}
      balloon[unkballoon]^.amountstatus:=outp;
      p:=save_p;
      EstimateAndRefineBalloon:=true;
      balloonsok:=true;
      save_balloon.Free;
      end
    else
      begin
        balloon[unkballoon]^.amount := half * h^.amount;  {first guess}
        if Iterate(h^.amount, usap, h^.loandate, h^.firstdate, balloon[unkballoon]^.amount, entire) then
          begin
            EstimateAndRefineBalloon := true;
            balloon[unkballoon]^.amountstatus := outp;
            balloonsok := true;
          end
        else
          EstimateAndRefineBalloon := false;
      end;
  end;

  { EstimateAndRefinePeriodicPrepayment -- solve for the unknown prepayment
    amount (pre[unkpre]^.payment).  Subtracts the PV of the regular payments,
    all balloons, and the OTHER prepayment series from the loan amount, then
    divides the residual by the unknown series' annuity factor for a first
    guess (a zero-rate branch handles the degenerate case), and refines with
    Iterate over the whole schedule.  Sets paymentstatus:=outp on success. }
  function EstimateAndRefinePeriodicPrepayment: boolean;
    var
      adjp, first, last, ff, rate: real;
      i: integer;

  begin
    p := h^.amount;
    usap := 0;
    adjp := h^.amount;
    rate := RateFromYield(h^.loanrate, h^.peryr);
    if (abs(rate)<teeny) then begin
      adjp:=adjp-h^.nperiods*h^.payamt;
      for i:=1 to user_nballoons do
         adjp:=adjp-balloon[i]^.amount;
      for i:=1 to npre do if (i<>unkpre) then with pre[i]^ do
         adjp:=adjp-nn*payment;
      with pre[unkpre]^ do
         payment:=adjp / nn; {nn is guaranteed to be >0}
      end
    else begin
      first := exxp(-rate * YearsDif(h^.firstdate, repay_from));
      last := exxp(-rate * YearsDif(h^.lastdate, repay_from));
      ff := exxp(-rate / RealPerYr(h^.peryr));
      adjp := p - d * (first - last * ff) / (1 - ff);
      for i := 1 to user_nballoons do
        adjp := adjp - balloon[i]^.amount * exxp(-rate * YearsDif(balloon[i]^.date, repay_from));
      for i := 1 to npre do
        if (i <> unkpre) then
          begin
            FirstLastAndFF(rate, first, last, ff, i);
            adjp := adjp - pre[i]^.payment * (first - last * ff) / (1 - ff);
          end;
      FirstLastAndFF(rate, first, last, ff, unkpre);
      pre[unkpre]^.payment := adjp * (1 - ff) / (first - last * ff); {a good first guess}
      end;
    if Iterate(h^.amount, usap, h^.loandate, h^.firstdate, pre[unkpre]^.payment, entire) then
      begin
        EstimateAndRefinePeriodicPrepayment := true;
        pre[unkpre]^.paymentstatus := outp;
      end
    else
      EstimateAndRefinePeriodicPrepayment := false;
  end;

  { DeterminePrepaymentDuration -- solve for how long the unknown prepayment
    series runs (its stop date / count) given a known payment amount.  Nets the
    PV of regular payments, balloons, and other prepayments out of the loan,
    then inverts the geometric series to get the number of years the extra
    payments must continue, nudging by half a period to compensate for the
    "before" rounding in NumberOfInstallments.  Requires the
    "Balloon includes regular payment" setting to be off (offers to set it).
    Sets h^.nperiods/nstatus and calls DetermineVeryLast. }
  function DeterminePrepaymentDuration: boolean;
    var
      i: integer;
      adjp, first, last, ff, nyrs, rate: real;
      kiword :word;
      CancelPressed: boolean;
  begin
    with pre[unkpre]^ do
      if not ((h^.amountstatus >= defp) and (h^.peryr > 0) and (h^.firststatus >= defp)) then
        begin
          errorflag := true;
          exit;
        end;
    DeterminePrepaymentDuration := false;
    if (not df.c.plus_regular) then begin
      MessageBoxWithCancel('Must set "Balloon includes regular payment" to "NO".  OK? ',CancelPressed, DA_SetBalloonIncludesToNo );
      if( not CancelPressed ) then
        df.c.plus_regular := true
      else
        exit;
    end;
    rate := RateFromYield(h^.loanrate, h^.peryr);
    adjp := h^.amount;
          {First, subtract off value of all regular payments}
    first := exxp(-rate * YearsDif(h^.firstdate, repay_from));
    last := exxp(-rate * YearsDif(h^.lastdate, repay_from));
    ff := exxp(-rate / h^.peryr);
    adjp := adjp - h^.payamt * (first - last * ff) / (1 - ff);
          {Then subtract off value of all balloons and known prepayments.}
    for i := 1 to user_nballoons do
      adjp := adjp - balloon[i]^.amount * exxp(-rate * YearsDif(balloon[i]^.date, repay_from));
    for i := 1 to npre do
      if (i <> unkpre) then
        begin
          FirstLastAndFF(rate, first, last, ff, i);
          adjp := adjp - pre[i]^.payment * (first - last * ff) / (1 - ff);
        end;
    with pre[unkpre]^ do
      begin
        if (adjp < payment) then
          begin
            MessageBox('Principal is more than covered by fixed payments - duration is "negative".', DA_DurationIsNegative);
            errorflag := true;
            exit;
          end;
        stopdate := startdate; {To avoid a R-T error in FirstLastAndFF, initialize now.}
        FirstLastAndFF(rate, first, last, ff, unkpre);
        if (abs(ff) < tiny) then
          begin
            MessageBox('Rate too small to determine duration of extra payments.', DA_RateTooSmall);
            errorflag := true;
            exit;
          end;
        last := (first - adjp * (1 - ff) / payment) / ff;
        nyrs := -lnn(last) / rate - YearsDif(startdate, repay_from);
        nyrs := nyrs + half / RealPerYr(pre[unkpre]^.peryr);
              {This helps compensate for rounding down below,}
        {"before" in NumberOfInstallments calculation.}
        AddYears(stopdate, nyrs); {Remember, stopdate has been initialized to startdate.}
        nn := NumberOfInstallments(startdate, stopdate, pre[unkpre]^.peryr, before);
        h^.nperiods := nn;
        h^.nstatus := outp;
      end;
    DeterminePrepaymentDuration := true;
    DetermineVeryLast;
  end;

{$ifdef 0}
Looks like it's an output thing
function ThreeLinesOut:boolean;
         var y   :byte;
             ws  :string;
         begin
         ThreeLinesOut:=false;
         if (not CopyHeaderToOutput) then begin abort:=true; exit; end;
         for y:=3 to 7 do begin
           FastRead(79,y,1,ws);
           if (ws[1]>'9') then ws[1]:=' ';
           if not OutputLine(ws) then begin abort:=true; exit; end;
           end;
{        if not OutputLineFeeds(1) then begin abort:=true; exit; end;  }
         if not (TwoLinesOut(true)) then begin abort:=true; exit; end;
         ThreeLinesOut:=true;
         end;
{$endif}

  { ComputeNextAndLoadPrintVariables -- (plain-loan table path only) copy the
    current balance/USA bucket into the payment record, advance nextt one
    period for the look-ahead, and stamp the current date for output. }
  procedure ComputeNextAndLoadPrintVariables;
  begin
    payment.principal := p;
    payment.usaprinc := usap;
    nextt := t;
    AddPeriod(nextt, h^.peryr, h^.firstdate.d, add);
    payment.date := t; {We have to put t into payment.date for output}
  end;

{ CloseUpShop -- finish a table: emit a final-payment note when the last
  payment differs from the regular amount, emit the trailing subtotal (if a
  detail+summary mode left a non-zero accumulation), and print the grand
  totals.  Also flags screenstatus as needing recalc because the repayment
  walk destroyed the adj/prepay working state. }
procedure CloseUpShop( Output: TStringList; bCommaSeperated: boolean );
          var ta   :byte;
              kiword :word absolute ki;
          begin
//          if (not abort) then begin
            if (not fancy) and (abs(payment.payamt-d)>0.005) then ReportFinalPayment( Output );
            if (cum<='Z') {upcase=Detail+Summary} then
               if (cumamt<>0) then PrintSummaryLine( Output, bCommaSeperated );
            PrintGrandTotals( Output, bCommaSeperated );
{            case destin of
         toprinter : begin
                           if (FormFeed)
                             and (ParseCodeString(df.p.resetstring,ws))
                               and (OutputString(ws)) then close(fout);
                           end;
               totext    : begin linesthispage:=0; close(fout); end;
               tolotus   : begin
                           blockwrite(lotusfile,CODA,4);
                           close(lotusfile);
                           end
               end; {case}
//{$I+}       ta:=MIOResult; if (ta>0) then abort:=true;
//            if (destin=none) then RespondToTablePageCommand(TRUE)
//            else {$ifdef RBT} if (global_call=0) then {$endif}
//                 MessageAndCheckForPrintScreen(AnyKeyToReturn,literal,kiword);
//            end;
//          PopWindow; {Don't want "Balance As Of" block to look funny.}
//          Memory.PlaceOnScreen; {Memory may have changed if you did a calculation during table output}
          {#Menu;}
//          ExitExample;
(*
          if (windex>1) or ((not examplemode) and (windex>0)) then begin
             if (fancy) then PopWindow
             else begin
                window(1,1,80,25);
                if (mouseInstalled) then MouseWindow(1,1,80,25);
                dec(windex);
                end;
             end;
          if (examplemode) then begin
             PopWindow;
             ExitExample;
             end;
*)
          screenstatus:=screenstatus or (needs_calc);
             {Because adj and pre information is destroyed in doing a repayment.}
//          if (destin=NONE) then ob.Free;
          end;

{ ComputeLoanAmount -- true exactly when the amount borrowed is the (single)
  blank field and everything else needed to back it out is known: periods,
  rate, payment, first date, term, balloons and prepayments all resolved, and
  amount itself still blank. }
function ComputeLoanAmount: boolean;
begin
  with h^ do
    ComputeLoanAmount := (peryrstatus >= defp) and (loanratestatus >= defp) and (payamtstatus >= defp)
                         and (firststatus >= defp) and (lastok) and (balloonsok) and (preok) and (amountstatus < defp);
end;

{ SufficientDataOnScreen -- gate that decides whether the screen holds enough
  known fields to compute a schedule at all.  Requires periods, a rate or a
  payment, and the first/loan dates; then checks that the amount is known or
  derivable, that adjustments are consistent, and that the term is known or
  derivable from the remaining knowns (handling the prepayment-duration and
  unknown-balloon cases).  Returns false (and Enter then shows "not enough
  data") if any precondition is missing. }
function SufficientDataOnScreen: boolean;
  var
    preknown: boolean;
begin
  with h^ do
    begin
      SufficientDataOnScreen := false;
      if (not ((peryrstatus >= defp) and ((loanratestatus >= defp) or (payamtstatus >= defp))
         and (firststatus >= defp) and (loandatestatus >= defp)))
      then exit;
      if not (((amountstatus >= defp) or ComputeLoanAmount) and (adjok)) then
        exit;
      if (preok) or (not fancy) then
        preknown := true
      else if (unkpre = 0) then
        preknown := false
      else if (pre[unkpre]^.paymentstatus >= defp) then
        preknown := true
      else
        preknown := false;

{if not ((lastok) or (((preok) or ((unkpre > 0) and (pre[unkpre]^.paymentstatus >= defp)))
and (payamtstatus >= defp) and (loanratestatus >= defp) and (adjok) and ((nadj = 0)
or (adj_fully_specified)) and (balloonsok))) then}

      if not ((lastok) or ((preknown) and (payamtstatus >= defp) and (loanratestatus >= defp)
         and (adjok) and ((nadj = 0) or (adj_fully_specified)) and (balloonsok)))
          {if not lastok, then compute is there enough info to compute last?}
        then exit;
      if not (balloonsok or ((preok) and (unkballoon > 0) and (payamtstatus >= defp)
         and (loanratestatus >= defp) and (adj_fully_specified)))
        then exit;
{           if not ((preok or ((unkpre>0) and (balloonsok) and (payamtok) and (aprokok) and (nadj=0)) ) ) then exit;}
      if not (((preok) or ((unkpre > 0) and (balloonsok) and (payamtstatus >= defp)
        and (loanratestatus >= defp) and (adj_fully_specified))))
        then exit;
          {It looks as though calculation of duration can be done with rate changes included.}
    end;
  SufficientDataOnScreen := true;
end;

 {$F+}
  { Enter -- the Amortization screen's main entry point and field-presence
    dispatcher.  Called with a `code` that says why (with_tab cursor move,
    make_table, apr_report, no_tab, ...).  Flow:
      1. FirstPass classifies inputs and prepares advanced options;
      2. validate date ordering, moratorium, balloons, and the two-payment min;
      3. set up repay_from / nrepay (prepaid / moratorium aware);
      4. dispatch to ONE backward solver based on which field is blank:
           rate / payment / balloon / term / prepayment / amount;
      5. apply ARM payment/rate solves and the points-APR solve;
      6. compute any "balance as of" date<->amount;
      7. (if make_table) prepare the output.
    All paths exit early on errorflag.  This is where the dispatch table that
    CLAUDE.md describes is actually wired up. }
  procedure Enter (code: byte);
    var
      ygo :byte;
      i: integer;
      outoforder: boolean;
      save_last: daterec;
    label
       TABLE_START, CLOSE_UP;

    { MoveCursor -- (UI stub in this port) originally repositioned the screen
      cursor to the next logical cell after a calculation. }
    procedure MoveCursor;
    begin
{      ygo:=whereyabs;
      if (col = Lcol[block]) and (ygo < LineCount[block]) then
        MoveToCell(Fcol[block], succ(ygo+ztop[block]),0)
      else if (col in [lastdatecol,pdnumcol]) and (h^.laststatus=outp) then
        MoveToCell(paymentcol, succ(ztop[block]),0)
      else if (col in [balloonamtcol,adjamtcol]) then begin
        KCOMMAND.Down; MoveToCell(Fcol[block], wherey,0);
        end
      else if (col=pred(firstdatecol)) and (h^.firststatus>=defp) then
        MoveToCell(succ(firstdatecol),ygo,0)
      else
        TabRt;
}//      FirstPass; {Again - you may have erased an output cell}
 //     ygo:=whereyabs; {This variable is used elsewhere in ENTER - it is NOT LOCAL}
    end;

    { AllRatesAreBlank -- (fancy loans) true if neither the base loan rate nor
      any ARM adjustment supplies a rate, meaning the rate must be derived from
      payments/points rather than given. }
    function AllRatesAreBlank:boolean;
             var i :byte;
             begin
             AllRatesAreBlank:=false;
             if (not fancy) then exit;
             if (h^.loanratestatus>=defp) then exit;
             for i:=1 to nadj do
               if (adj[i]^.loanratestatus>=defp) then exit;
             AllRatesAreBlank:=true;
             end;

 {$ifdef 0}
 no Lotus integration
    procedure SimpleHeaderToLotus;
    begin
      inc(lotusrow);
      lotuscol := 1;
      WriteOneStringCell(true, '''', lotusrow, lotuscol, 'Outstanding');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' Principal');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' Interest');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' Interest');
      inc(lotusrow);
      lotuscol := 0;
      WriteOneStringCell(true, '^', lotusrow, lotuscol, 'Date');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, 'Principal');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' This Per''d');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' This Per''d');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' To Date');
      LineAcrossSpreadsheet;
        {inc(lotusrow); Don't skip a line}
    end;

    procedure FancyHeaderToLotus;
    begin
      inc(lotusrow);
      lotuscol := 1;
      WriteOneStringCell(true, '''', lotusrow, lotuscol, '  Payment');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, 'Outstanding');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' Principal');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' Interest');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' Interest');
      inc(lotusrow);
      lotuscol := 0;
      WriteOneStringCell(true, '^', lotusrow, lotuscol, 'Date');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, '  Amount');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, 'Principal');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' This Per''d');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' This Period');
      WriteOneStringCell(true, '''', lotusrow, lotuscol, ' To Date');
      LineAcrossSpreadsheet;
        {inc(lotusrow); Don't skip a line}
    end;
{$endif}
{ PrepareScreenForOutput -- (mostly disabled in this port) originally chose the
  output destination (printer / text file / Lotus), wrote the column headers,
  and set up the output window before the table is generated.  The body is
  $ifdef 0'd out here because the Windows host owns output. }
function PrepareScreenForOutput:boolean;
         var kiword :word;
         begin
{$ifdef 0}
no screen output from in here
         if (code=lotus_express) then destin:=tolotus;
         if (not GetTableOptions(h^.firstdate,h^.peryr)) then begin
            PrepareScreenForOutput:=false;
            exit;
            end
         else PrepareScreenForOutput:=true;
         if (destin=toprinter) then begin
            ws:=prnport;
            {InitSerial;}
            TestPrinter(kiword);
            if (kiword=27) then begin PrepareScreenForOutput:=false; exit; end
            else begin assign(fout,prnport); rewrite(fout); end;
            end
         else if (destin<>none) then begin
            if (destin=tolotus) then begin
               WK1File(df.d.lotuspath+outfilename);
               if (abort) then exit; {maybe template wasn't available}
               reset(lotusfile,1); seek(lotusfile,filesize(lotusfile)-4);
               if (fancy) then FancyHeaderToLotus
               else SimpleHeaderToLotus;
               end
            else begin
               assign(fout,df.d.textpath+outfilename);
               reset(fout); if (MIOresult=0) then begin
                 append(fout);
                 writeln(fout,crlf,#12);
                 end
               else rewrite(fout);
               end;
            end;

         textattr:=MainScreenColors;
         if (fancy) then PushWindowNoBorder(1,8,80,24)
         else begin
           inc(windex);
           wx1[windex]:=1; wx2[windex]:=80; wy1[windex]:=8; wy2[windex]:=24;
           RestoreWindow;
           clrscr;
           end;
         gotoxy(1,1);
         if fancy then writeln(fancyheader1,fancyheader2,fancyheader3,header4)
         else begin
           if (df.c.r78) then write(R78Header1) else write(header1);
           writeln(header2,header3,header4);
           end;
         window(wx1[windex],12,wx2[windex],wy2[windex]);
         wy1[windex]:=12;
         textattr:=GetColor(defp);
         if (destin>tolotus) then  {Send header tofile or toprinter}
            PrepareScreenForOutput:=ThreeLinesOut;
{$endif}
         end;

    { TackOnFinalBalloon -- when the loan is over-specified (amount, rate,
      payment and term all known) the schedule generally won't end at exactly
      zero, so add an implied terminal balloon at very_last that absorbs the
      residual.  If a user balloon already sits on very_last, adjust it (and
      warn); otherwise append a new computed balloon, solve its amount with
      EstimateAndRefineBalloon, then mark it not-really-used (lastok restored)
      so table generation still truncates when the balance goes non-positive. }
    procedure TackOnFinalBalloon;
      var
        oldamt: real;
        merge_w_existing, save_lastok: boolean;
    begin
          {Computation is overspecified - compute last balloon}
      save_last := very_last;
      save_lastok := h^.lastok;
      merge_w_existing := (nballoons > 0) and (dateok(very_last)) and (DateComp(very_last, balloon[nballoons]^.date) = 0);
      if (merge_w_existing) then
        begin
          h^.lastok := true;
          oldamt := balloon[nballoons]^.amount;
        end
      else
        begin
          if (df.c.plus_regular) then
            oldamt := 0
          else
            oldamt := h^.payamt;
          if (not dateok(very_last)) then
            DetermineVeryLast;
          inc(nballoons);
//          while (nballoons>nlines[AMZBalloonBlock]) do AddRow(AMZBalloonBlock);
          balloon[nballoons]^.date := very_last;
          balloon[nballoons]^.datestatus := outp;
        end;
      unkballoon := nballoons;
      if EstimateAndRefineBalloon then
        begin
          if (abs(balloon[unkballoon]^.amount - oldamt) >= minpmt) then
            begin
              if (merge_w_existing) then
                MessageBox('Please note that the amount of your terminating balloon has been ajusted.', DA_TerminatingBalloonChanged)
              else
                begin
                  dec(nballoons);
                end;
            end;
        end;
      unkballoon := 0;
      very_last := save_last;
      h^.lastok := save_lastok;
            {This says, don't really use this last balloon in generating a table.}
            {Table should truncate when balance goes negative, and this should do the trick.}
            {Otherwise, if too large a number is entered in # of payments, it generates}
            {a large negative balloon, and this keeps the table printing long beyond where}
            {the balance is negative.}
    end;

    { ComputeBalanceFromDate -- the "Balance As Of" feature, forward direction:
      given a date in w^.date, compute the outstanding loan balance on that
      date.  Special cases: before the loan date (nonsensical), before the
      first payment (accrued interest only, minus refundable prepaid interest),
      after the very last payment (zero).  Otherwise it amortizes up to that
      date (RepayFancyLoan with balance_calc) and interpolates within the
      period, or uses the closed-form Rule-of-78 balance for non-fancy R78
      loans.  Stores the result in w^.amount/w^.amountstatus. }
    procedure ComputeBalanceFromDate;
      var r78interest :real;
          lastpmtdate :daterec;
          SaveBalloon :saved_balloon_state;
    begin
       if (datecomp(w^.date,h^.loandate)<0) then
          MessageBox('It makes no sense to ask for the loan balance before the loan date.', DA_LoanBalanceBeforeDate)
       else if (datecomp(w^.date,h^.firstdate)<0) then begin
          w^.amount:=h^.amount * (1+h^.loanrate*YearsDif(w^.date,h^.loandate));
          if (prepaid) then w^.amount:=w^.amount-PrepaidInterest; {you get some of it back}
          w^.amountstatus:=outp;
          end
       else if (datecomp(w^.date,very_last)>0) then begin
          w^.amount:=0;
          w^.amountstatus:=outp;
          end
       else begin
          if (not df.c.R78) or (fancy) then begin
             SaveBalloon.Save;
             save_last:=very_last;
             very_last:=w^.date;
             { AddDays(very_last,-1); This seems to give bad result for next day right after pmt REMOVED 6/6/91}
             p:=h^.amount; usap:=0;
             balance_calc:=true;
             if (DateComp(w^.date,h^.firstdate)>0) then
               RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, no_value_calc, 0)
             else begin
               payment.principal:=p;
               if prepaid then begin
                 payment.date:=h^.firstdate;
                 AddPeriod(payment.date,h^.peryr,payment.date.d,subtract);
                 end
               else payment.date:=h^.loandate;
               end;
             if (df.c.in_advance) then
               w^.amount:=payment.principal * (1- RateInForce(w^.date)*YearsDif(nextpayment.date,w^.date))
             else {ARR}
               w^.amount:=payment.principal * (1+ RateInForce(w^.date)*YearsDif(w^.date,payment.date));
             w^.amountstatus:=outp;
             balance_calc:=false;
             very_last:=save_last;
             SaveBalloon.Restore;
             SaveBalloon.Free;
             end
          else begin {R78 amortization, (not fancy)}
             { Rule of 78 ("sum of the digits"): total interest is allocated
               across periods in proportion to remaining term.  r78base is the
               interest assigned to the LAST period; period k gets
               r78base*(n+1-k).  i = #payments made on or before w^.date. }
             lastpmtdate:=w^.date;
             i:=NumberofInstallments(h^.firstdate, lastpmtdate, h^.peryr, on_or_before);
             r78base := (h^.nperiods * d - h^.amount) / (half * h^.nperiods * (h^.nperiods + 1));
               {half must be first to avoid overflow for n>Sqrt(32000)}
            {r78interest := r78base*half*
                      ( h^.nperiods*(1.0+h^.nperiods) - (h^.nperiods-i)*(1.0+h^.nperiods-i) );
               The above simplifies to: }
             r78interest := r78base*i*(-half*i + (h^.nperiods+half));
             w^.amount := h^.amount + r78interest - i*h^.payamt; {as of last paydate}
             if (DateComp(w^.date,lastpmtdate)=0) then
               w^.amount := w^.amount+h^.payamt
             else
               w^.amount := w^.amount*(1+h^.loanrate*YearsDif(w^.date,lastpmtdate));
             w^.amountstatus:=outp;
             end
          end;
       end;

  { ComputeDateFromBalance -- the "Balance As Of" feature, inverse direction:
    given a target balance in w^.amount, find the payment date on which the
    loan first reaches that balance.  Fancy / non-R78 path amortizes with
    balance_calc until BalanceStop and reports nextpayment.date.  The non-fancy
    R78 path inverts the quadratic Rule-of-78 balance formula directly (note
    payamt is subtracted because the formula is for the pre-payment balance).
    Sets w^.date/w^.datestatus (and corrects w^.amount if it drifted). }
  procedure ComputeDateFromBalance;
            var corrected_amt :real;
                save_balloon  :saved_balloon_state;
            begin
            if (fancy) or (not df.c.R78) or (not (df.c.basis=x360)) then begin
              balance_calc:=true;
              p:=h^.amount; usap:=0;
              save_balloon.Save;
              RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, no_value_calc, 0);
              w^.date:=nextpayment.date;
              w^.datestatus:=outp;
              if (df.c.in_advance) then
                 corrected_amt:=nextpayment.principal+nextpayment.payamt-nextpayment.interest
              else  {ARR}
                 corrected_amt:=nextpayment.principal+nextpayment.payamt;
              if (abs(corrected_amt-w^.amount)>0.01) then begin
                w^.amount:=corrected_amt;
                w^.amountstatus:=defp;
                end;
              balance_calc:=false;
              save_balloon.Restore;
              save_balloon.Free;
              end
            else begin {not fancy, R78 amortization}
              r78base := (h^.nperiods * d - h^.amount) / (half * h^.nperiods * (h^.nperiods + 1));
                {half must be first to avoid overflow for n>Sqrt(32000)}
              i:=round(0.49+QuadraticFormula(-half*r78base,r78base*(half+h^.nperiods)-h^.payamt,
                                                       h^.amount-w^.amount+h^.payamt));
                   {In the last parameter, payamt has been subtracted from w^.amount because
                    the quadratic formula was derived for the balance BEFORE a payment was made.}
              AddNPeriods(h^.firstdate,w^.date,h^.peryr,i-1);
              w^.datestatus:=outp;
              corrected_amt:=w^.amount;
              ComputeBalanceFromDate;
              if (abs(corrected_amt-w^.amountstatus)>0.01) then w^.amountstatus:=defp;
              end;
            end;

  begin {Enter}
//    ygo:=whereyabs;
    if (code=make_table) and (screenstatus and needs_calc = 0) then
      begin
        if (SufficientDataOnScreen) then
          goto TABLE_START
        else
          begin
            InsufficientDataMessage(tablestr, DA_NotEnoughDataForTable);
            exit;
          end;
      end;
    FirstPass;
    if (errorflag) then exit;
    if (code = with_tab) then
      begin
        MoveCursor;
        if (screenstatus and needs_calc = 0) then exit;
      end;
    if (not SufficientDataOnScreen) then
      begin
//        ClearOutputCells;
        if (code = make_table) then
          InsufficientDataMessage(tablestr, DA_NotEnoughDataForTable);
        exit;
      end;
//    ClearOutputCells;  {Experimental placement of this line - 6/6/91 (used to be in FirstPass) }
    FirstPass;         {(again) Added 6/6/91 because ClearOutputCells kills the last payment date}
    if errorflag then
      exit;
    if (DateComp(h^.firstdate, h^.lastdate)>=0) then
      begin
        MessageBox('There must be at least two regular payments.', DA_2PaymentsMin);
        RecordError(succ(ztop[AMZTopBlock]), pdnumcol);
        exit;
      end;
    if (DateComp(h^.loandate, h^.firstdate) > 0) then
      outoforder := true
    else
      outoforder := false;
    for i := 1 to npre do
      if (DateComp(h^.loandate, pre[i]^.startdate) >= 0) then
        outoforder := true;
    if (outoforder) then
      begin
        MessageBox('Your dates are out of order.', DA_DateOutOfOrder);
        exit;
      end;
    if (nballoons > 0) and (mor^.first_repaystatus >= defp) and (DateComp(balloon[1]^.date, mor^.first_repay) < 0) then
      begin
        MessageBox('No balloon can precede first repayment of principal.', DA_BalloonPrecedesRepay);
        exit;
      end;
    if (mor^.first_repaystatus >= defp) and (dateok(mor^.first_repay) and (DateComp(h^.firstdate, mor^.first_repay) > 0)) then
      begin
        MessageBox('Date that principal repayment begins cannot precede first payment date.', DA_PrincPrecedesFirstPay);
        exit;
      end;
    f := GrowthPerPeriod;

    { If the first payment is exactly one period after the loan date, the
      "prepaid interest" simplification is unnecessary, so turn it off. }
    t := h^.firstdate;
    AddPeriod(t, h^.peryr, h^.firstdate.d, subtract);
    if (DateComp(t, h^.loandate) < 0) and (not df.c.in_advance) then
      begin
        prepaid := false;
             {Prepaid interest only makes sense if first repayment date is}
      {more than one interest period after loan date.  If it's exactly one interest}
      {period, then we can set prepaid to TRUE for free, and simplify calculations.}
      end;
    { Set up repay_from (the date amortization of principal effectively begins)
      and prorate (the odd-first-period length factor).  Three cases:
        - moratorium set: principal repayment is deferred to first_repay;
        - prepaid: repay_from is one period before the first payment;
        - otherwise: repay_from is the loan date, with a fractional first
          period (prorate) covering loandate..first_repay. }
    if (mor^.first_repaystatus >= defp) then
      begin
        t := mor^.first_repay; {save for comparison}
        nrepay := NumberOfInstallments(h^.firstdate, mor^.first_repay, h^.peryr, on_or_after);
        if (DateComp(t, mor^.first_repay) <> 0) then
          mor^.first_repaystatus := defp;
                {Let user know we've adjusted first_repay to be on a payment date.}
        repay_from := mor^.first_repay;
        AddPeriod(repay_from, h^.peryr, h^.firstdate.d, subtract);
        prorate := 1;
      end
    else
      begin
        if (nballoons > 0) and (DateComp(balloon[1]^.date, h^.firstdate) < 0) then
          mor^.first_repay := balloon[1]^.date
        else
          mor^.first_repay := h^.firstdate;
        if prepaid then
          begin
            repay_from := h^.firstdate;
            AddPeriod(repay_from, h^.peryr, h^.firstdate.d, subtract);
            prorate := 1;
          end
        else
          begin
            repay_from := h^.loandate;
            prorate := YearsDif(mor^.first_repay, repay_from) * h^.peryr;
          end;
      end;
            {repay_from is the date on which you begin amortizing.}
      {first_repay is the first payment date that includes principal.}
      {If first_repay is specified, then repay_from is one period before first_repay.}
      {If prepaid="Y" is specified, then repay_from is one period before firstdate.}
    {Otherwise, repay_from is just the loan date . }
    if (df.c.in_advance) and (nadj > 0) then
      begin
        MessageBox(NoAdvancedMsg, DA_NoAdvanceMsg);
        exit;
      end;
    if (h^.lastok) and (DateComp(mor^.first_repay, h^.firstdate) <> 0) then
      begin
        save_last:=h^.lastdate;
        nrepay := NumberOfInstallments(mor^.first_repay, h^.lastdate, h^.peryr, on_or_before);
                 {This is the real number of payments over which to amortize.}
        h^.lastdate:=save_last;
        if (nrepay <= 0) then
          begin
            MessageBox('Principal repayment must begin before the last payment date.', DA_PrincPrecedeLastPay);
                {Computing d results in divide by 0 otherwise.}
            exit;
          end;
        if (h^.amount / nrepay < targ^.target) then
          begin
            MessageBox('Your principal reduction target is too high.', DA_PrincipalReductionTooHigh);
                {Loan will be paid off before last period.}
            exit;
          end;
      end {if lastok and DateComp(mor^.first_repay,h^.firstdate)<>0)}
    else
      nrepay := h^.nperiods;
    DetermineVeryLast;
    if (h^.loanratestatus >= defp) then
      p := h^.amount;
    usap := 0;
    t := h^.firstdate;
//    if (fancy) or (df.c.exact) or (not (df.c.basis=x360)) then
//      RequestPatience;
    { ============================ FIELD-PRESENCE DISPATCH ===================
      The payment is known => the unknown is something else; pick the solver by
      which field is blank (rate, then balloon, then term, then prepayment,
      then amount).  If the payment itself is blank, solve for it (else branch
      below). }
    if (h^.payamtstatus >= defp) then
      begin
        d := h^.payamt;
(*
        if (AllRatesAreBlank) then begin
           if (h^.pointsstatus=empty) then h^.points:=0;
           EstimateAndRefineAPRWithPoints;
           {not finished}
           end
        else
*)
        if (h^.loanratestatus < defp) then
          begin {rate is the unknown}
            if not EstimateAndRefineRate then
              exit;
          end
        else if (unkballoon > 0) then
          begin
            if EstimateAndRefineBalloon then
              balloon[unkballoon]^.amountstatus := outp
            else
              exit;
          end
        else if (not h^.lastok) then
          begin {term (last date / #periods) is the unknown}
            if not DetermineLastPaymentDate(p, usap) then
              exit;
          end
        else if (unkpre > 0) then
          begin {a prepayment series field is the unknown}
            if (not (pre[unkpre]^.paymentstatus >= defp)) then
              begin {...its payment amount}
                if not EstimateAndRefinePeriodicPrepayment then
                  exit;
              end
            else if (not (pre[unkpre]^.stopdatestatus >= defp)) then
              begin {...its duration}
                if not DeterminePrepaymentDuration then
                  exit;
              end
          end
        else if ComputeLoanAmount then
          begin {amount borrowed is the unknown}
            if (fancy) and (targ^.targetstatus>=defp) then
              begin
                MessageBox('Cannot compute Amount Borrowed when using Targ Principal Reduction.', DA_BorrowedUsingReduction);
                exit;
              end;
            if not EstimateAndRefineLoanAmount then
              exit;
            h^.amountstatus := outp;
          end;
      end
    else {not payamtok -- the payment itself is the unknown}
      begin
        if not EstimateAndRefinePayment then
          exit;
        h^.payamtstatus := outp;
      end;
    if (fancy) and (h^.amountstatus >= defp) and (h^.loanratestatus >=defp) and
      (((nadj = 0) and (h^.payamtstatus >= defp)) or (adj_fully_specified))
      and (unkballoon = 0)
{$ifdef SCROLLS}
      and (nballoons < maxlines)
{$else}
      and (nballoons < LineCount[AMZBalloonblock])
{$endif}
      then TackOnFinalBalloon;
    { Pre-size the on-screen line count by summary mode (Yearly/Semiann/
      Quarterly/Monthly/detail).  Pascal integer div is used deliberately. }
    case cum of
      'Y':
        lines := h^.nperiods div h^.peryr;
      'S':
        lines := 2 * h^.nperiods div h^.peryr;
      'Q':
        lines := 4 * h^.nperiods div h^.peryr;
      'M':
        lines := 12 * h^.nperiods div h^.peryr;
      ' ':
        lines := h^.nperiods + nballoons;
    end;
    inc(lines);  {Better one too few lines on screen than one too many.}
    { For each ARM adjustment, solve whichever of payment/rate it left blank:
      rate-given => compute the new payment; payment-given => fit the new rate. }
    for i := 1 to nadj do
      if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
        begin
          if (not EstimateAndRefineAdjPayment(i)) then
            exit;
          d := h^.payamt;
        end
      else if (adj[i]^.loanratestatus < defp) and (adj[i]^.amountstatus >= defp) then
        begin
          if (not EstimateAndRefineAdjRate(i)) then
            exit;
        end;
    if (h^.pointsstatus > defp) then {if =defp, then it's zero by default and we skip the APR computation.}
      if (not EstimateAndRefineAPRwithPoints) then
        MessageBox('Computation of APR failed to converge.', DA_APRNoConverge);
    { "Balance As Of" block: if the user gave a date, compute the balance; if
      they gave a balance, compute the date. }
    if (nlines[AMZBalanceBlock]>0) then
      if (w^.datestatus>=defp) then ComputeBalanceFromDate
      else if (w^.amountstatus>=defp) then ComputeDateFromBalance;

TABLE_START:
//    ygo:=whereyabs;
//    SetCursorSize;
    if (hard_payment) and (fancy) then begin
      {The Dav Holle provision - as long as payment amount on top line is a
       hard number, a "standard" treatment of pennies will be used.   So we
       automatically harden balloon amounts and adjusted payments.}
       for i:=1 to nadj do Round2(adj[i]^.amount);
       for i:=1 to nballoons do Round2(balloon[i]^.amount);
       end;
    if (code=apr_report) then begin
//        APRReport;
        if (scripting) then exit;
        end;
    if (not errorflag) then begin
      screenstatus:=screenstatus and computed;
//      DisplayAll;
      end;
    if (code<>make_table) then exit;
    abort:=(not PrepareScreenForOutput);
    if (abort) then exit;
end;

// this procedure used to be the end of the Enter function.  If Enter was
// called with code=make_table then it would execute.  I've split it out.
// just make sure it calls Enter at the begining.
{ MakeTable -- generate the full amortization schedule into Output.  First runs
  Enter(no_tab) to solve the unknown and validate, then emits rows:
    - an optional settlement-day line for prepaid interest and/or points;
    - the body, via RepayFancyLoan for fancy/exact/non-360 loans, or a fast
      inline closed-form loop for plain loans (with a separate Rule-of-78
      branch and an interest-in-advance branch);
    - the footer, via CloseUpShop.
  bCommaSeperated chooses CSV vs fixed-width formatting.  The balloon cursor is
  saved/restored around RepayFancyLoan because the walk mutates it. }
procedure MakeTable( Output: TStringList; bCommaSeperated: boolean );
var
  balloons_before_table: saved_balloon_state;
begin
  Enter( no_tab );
  if( errorflag ) then exit;
  if( not SufficientDataOnScreen ) then exit;
  {------------------------------START MAKING TABLE--------------------------}

    int_to_date := 0;
    cumint := 0;
    cumamt := 0;
    count := 0;
    payment.paynum := 0;
    p := h^.amount;
    f_1 := f - 1;
    t := very_last;
    if (NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_before) > 14) then
      more_dates_than_lines := true
    else
      more_dates_than_lines := false;
           {First, print out a prepayment at settlement if necessary.}
    abort := false;
    if ((prepaid) and (PrepaidInterest>0)) or ((h^.pointsstatus>empty) and (h^.points<>0)) then
      begin
              {Line entry for prepaid interest}
        with Payment do
          begin
            payment.paynum := -1;
            interest := PrepaidInterest + h^.points*h^.amount;
            if hard_payment then Round2(interest); {@round}
            payamt := interest;
            date := h^.loandate;
            principal := h^.amount;
          end;
        nextpayment.date := h^.firstdate;
               {Trigger a summary line if a summary period ends before first}
        {payment is due. }
        DecideWhetherToPrintALine(h^.loandate, h^.firstdate, Output, bCommaSeperated);
      end; {t is h^.loandate; nextt is h^.firstdate}
    if (fancy) or ((df.c.exact) and (not df.c.R78)) or (not (df.c.basis=x360)) then begin
      balloons_before_table.Save;
      RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, Output, bCommaSeperated, entire, no_value_calc, 0);
      balloons_before_table.Restore;
      balloons_before_table.Free;
      end
    else
      { Plain-loan fast path: emit rows with closed-form interest each period
        rather than running the heavy fancy engine. }
      with Payment do
        begin
          t := h^.firstdate;
          p := h^.amount;
          usap := 0;
          payamt := d;
          if (df.c.r78) then
            begin
              { Rule of 78: first period's interest is n*r78base, then it
                decreases by r78base every period (see r78base derivation). }
              r78base := (h^.nperiods * d - h^.amount) / (half * h^.nperiods * (h^.nperiods + 1));
                {half must be first to avoid overflow for n>Sqrt(32000)}
              interest := r78base * (h^.nperiods + 1); {First int payment should be n*r78base}
              if hard_payment then Round2(interest); {@round}
            end
          else if (not prepaid) then
            begin
                    {Prorate interest on first payment if not prepaid: the
                     odd first period may be shorter/longer than a full period.}
              interest := p * f_1 * prorate;
              if hard_payment then Round2(interest); {@round}
              p := p + interest - d;
              ComputeNextAndLoadPrintVariables;
              DecideWhetherToPrintALine(t, nextt, Output, bCommaSeperated);
              t := nextt;
            end;
          { Main row loop: stop at the last payment date or when paid off. }
          while (not abort) and (DateComp(t, h^.lastdate) <= 0) and (p > 0) do
            begin
              if (df.c.r78) then
                begin
                  interest := interest - r78base;  { R78: step interest down }
                  if hard_payment then Round2(interest); {@round}
                  p := p + interest - d;
                end
              else
                begin
                  if (df.c.in_advance) then
                    begin
                      { Interest-in-advance closed form; the odd last payment
                        (p<d) carries no further interest. }
                      if (p < d) then
                        interest := 0 {needed for odd last payment}
                      else
                        interest := (p - d) * f_1 / (2 - f);
                    end
                  else
                    interest := p * f_1;   { arrears: simple periodic interest }
                  if hard_payment then Round2(interest); {@round}
                  p := p + interest - d;
                end;
              ComputeNextAndLoadPrintVariables;
              { Fold a sub-dollar residual into this payment to end at zero. }
              if (payment.principal < minpmt) then
                begin
                  payment.payamt := payment.payamt + payment.principal;
                  payment.principal := 0;
                end;
              DecideWhetherToPrintALine(t, nextt, Output, bCommaSeperated );
              t := nextt;
            end;
        end;
        CloseUpShop( Output, bCommaSeperated );
   end;

{$F+}
{ PrepareScreen -- remember the fancy/plain mode code for this run before the
  amortization screen is drawn.  (The screen-painting call is stubbed in this
  port; the Windows host draws the UI.) }
procedure PrepareScreen(code :byte);
{$ifndef OVERLAYS} {$ifndef TESTING} {$F-} {$endif} {$endif}
          begin
          fancycode:=code;
//          AmortizationScreen;
          end;
{$F+}
{ Init -- mark the Amortization screen as the active one and point the menu at
  its menu line (called when the user enters the screen). }
procedure Init;
{$ifndef OVERLAYS} {$F-} {$endif}
          begin
          thisrun:=iAMZ;
{$ifndef TOPMENUS}
          menuptr:=@menuline[iAMZ];
{$endif}
          end;

begin

{ Unit initialization: register this screen's handlers in the PEData dispatch
  tables, keyed by the screen id iAMZ.  The framework calls EnterProc on
  recalc, ScreenProc to (re)draw, InitProc on screen entry, CloseProc on exit. }
PEData.EnterProc[iAMZ]:=Enter;
PEData.ScreenProc[iAMZ]:=PrepareScreen;
PEData.InitProc[iAMZ]:=Init;
PEData.CloseProc[iAMZ]:=NullProc;
{$ifdef TESTING}
TESTHIGH.AMZEnter:=AMORTIZE.Enter;
TESTHIGH.GetAMZNumbers:=AMORTIZE.FirstPass;
{$endif}

end.
