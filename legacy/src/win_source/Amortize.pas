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

INTERFACE

//uses OPCRT,OPMOUSE,VIDEODAT,NORTHWND,PETYPES,PEDATA,INPUT,INTSUTIL,PEPANE,COMDUTIL,IOUNIT,
//{$ifdef NO_FRILLS} HTXTSTUB, {$else} HTXTHELP, {$endif}
//{$ifdef CHRONO} MCMENU, {$endif}
//KCOMMAND,LOTUS,AMZUTIL,AMORTOP,TABLE
//{$ifdef TESTING} ,TESTHIGH {$endif}
uses VIDEODAT,PETYPES,PEDATA,INTSUTIL,AMORTOP,Globals, Classes;


procedure Enter(code :byte);
procedure PrepareScreen(code :byte);
function EstimateAndRefineAPRwithPoints: boolean;
function SufficientDataOnScreen: boolean;
{$ifdef TESTING}
procedure FirstPass;
{$endif}
procedure MakeTable( Output: TStringList; bCommaSeperated: boolean );
procedure ZeroAMZLoan( AMZ: AMZPtr );
procedure ZeroMoratorium( Moratorium: moratoriumptr );
procedure ZeroTarget( Target: targetptr );
procedure ZeroSkip( Skip: skipptr );
procedure ZeroBalloon( balloon: balloonptr );
procedure ZeroPrepayment( Prepayment: prepaymentptr );
procedure ZeroAdjustment( Adjustment: adjptr );
function AdjustmentIsEmpty( pADJ: adjptr ): boolean;
function BalloonIsEmpty( pBalloon: balloonptr ): boolean;
function PrepaymentIsEmpty( pPre: prepaymentptr ): boolean;

IMPLEMENTATION

uses HelpSystemUnit;

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

procedure ZeroMoratorium( Moratorium: moratoriumptr );
begin
  Moratorium.first_repaystatus     := empty;
  Moratorium.first_repay           := unkdate;
end;

procedure ZeroTarget( Target: targetptr );
begin
  Target.targetstatus          := empty;
  Target.target                := 0;
end;

procedure ZeroSkip( Skip: skipptr );
begin
  Skip.skipstatus            := empty;
  Skip.skipmonths            := '';
end;

procedure ZeroBalloon( balloon: balloonptr );
begin
  with balloon^ do begin
  datestatus            := empty;
  date                  := unkdate;
  amountstatus          := empty;
  amount                := 0;
  end;
end;

function BalloonIsEmpty( pBalloon: balloonptr ): boolean;
begin
  BalloonIsEmpty := (pBalloon.datestatus=empty) and (pBalloon.amountstatus=empty);
end;

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

function PrepaymentIsEmpty( pPre: prepaymentptr ): boolean;
begin
  PrepaymentIsEmpty := (pPre.startdatestatus=empty) and (pPre.nnstatus=empty) and
                       (pPre.stopdatestatus=empty) and (pPre.peryrstatus=empty) and
                       (pPre.paymentstatus=empty);
end;

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

function AdjustmentIsEmpty( pADJ: adjptr ): boolean;
begin
  AdjustmentIsEmpty := (pAdj.datestatus=empty) and (pAdj.loanratestatus=empty) and
                       (pAdj.amountstatus=empty);
end;

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
            if (skp^.skipstatus >= defp) then begin
              if not MonthSetFromString(skp^.skipmonths,skipmonthset)
                then RecordError(succ(ztop[AMZSkipMonthBlock]),skipmonthcol);
              end
            else skipmonthset:=[];
            CheckPrepayments;
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
  hard_payment:=(h^.payamtstatus=inp);
  end; {First pass}


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

  procedure FirstLastAndFF (var rate, first, last, ff: real; i: integer);
  begin
    first := exxp(-rate * YearsDif(pre[i]^.startdate, repay_from));
    last := exxp(-rate * YearsDif(pre[i]^.stopdate, repay_from));
    ff := exxp(-rate / pre[i]^.peryr);
  end;

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
    for i := 1 to user_nballoons do
      adjp := adjp - balloon[i]^.amount * exxp(-rate * YearsDif(balloon[i]^.date, repay_from));
    for i := 1 to npre do
      begin
        FirstLastAndFF(rate, first, last, ff, i);
        if (abs(1-ff)>teeny) then
          adjp := adjp - pre[i]^.payment * (first - last * ff) / (1 - ff)
        else adjp := adjp - pre[i]^.payment * pre[i]^.nn;
      end;
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

  procedure ComputeNextAndLoadPrintVariables;
  begin
    payment.principal := p;
    payment.usaprinc := usap;
    nextt := t;
    AddPeriod(nextt, h^.peryr, h^.firstdate.d, add);
    payment.date := t; {We have to put t into payment.date for output}
  end;

procedure CloseUpShop( Output: TStringList; bCommaSeperated: boolean );
          var ta   :byte;
              kiword :word absolute ki;
          begin
//          if (not abort) then begin
            if (not fancy) and (abs(payment.payamt-d)>0.005) then ReportFinalPayment( Output, bCommaSeperated );
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

function ComputeLoanAmount: boolean;
begin
  with h^ do
    ComputeLoanAmount := (peryrstatus >= defp) and (loanratestatus >= defp) and (payamtstatus >= defp)
                         and (firststatus >= defp) and (lastok) and (balloonsok) and (preok) and (amountstatus < defp);
end;

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
  procedure Enter (code: byte);
    var
      ygo :byte;
      i: integer;
      outoforder: boolean;
      save_last: daterec;
    label
       TABLE_START, CLOSE_UP;

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

    t := h^.firstdate;
    AddPeriod(t, h^.peryr, h^.firstdate.d, subtract);
    if (DateComp(t, h^.loandate) < 0) and (not df.c.in_advance) then
      begin
        prepaid := false;
             {Prepaid interest only makes sense if first repayment date is}
      {more than one interest period after loan date.  If it's exactly one interest}
      {period, then we can set prepaid to TRUE for free, and simplify calculations.}
      end;
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
          begin
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
          begin
            if not DetermineLastPaymentDate(p, usap) then
              exit;
          end
        else if (unkpre > 0) then
          begin
            if (not (pre[unkpre]^.paymentstatus >= defp)) then
              begin
                if not EstimateAndRefinePeriodicPrepayment then
                  exit;
              end
            else if (not (pre[unkpre]^.stopdatestatus >= defp)) then
              begin
                if not DeterminePrepaymentDuration then
                  exit;
              end
          end
        else if ComputeLoanAmount then
          begin
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
    else {not payamtok}
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
      with Payment do
        begin
          t := h^.firstdate;
          p := h^.amount;
          usap := 0;
          payamt := d;
          if (df.c.r78) then
            begin
              r78base := (h^.nperiods * d - h^.amount) / (half * h^.nperiods * (h^.nperiods + 1));
                {half must be first to avoid overflow for n>Sqrt(32000)}
              interest := r78base * (h^.nperiods + 1); {First int payment should be n*r78base}
              if hard_payment then Round2(interest); {@round}
            end
          else if (not prepaid) then
            begin
                    {Prorate interest on first payment if not prepaid.}
              interest := p * f_1 * prorate;
              if hard_payment then Round2(interest); {@round}
              p := p + interest - d;
              ComputeNextAndLoadPrintVariables;
              DecideWhetherToPrintALine(t, nextt, Output, bCommaSeperated);
              t := nextt;
            end;
          while (not abort) and (DateComp(t, h^.lastdate) <= 0) and (p > 0) do
            begin
              if (df.c.r78) then
                begin
                  interest := interest - r78base;
                  if hard_payment then Round2(interest); {@round}
                  p := p + interest - d;
                end
              else
                begin
                  if (df.c.in_advance) then
                    begin
                      if (p < d) then
                        interest := 0 {needed for odd last payment}
                      else
                        interest := (p - d) * f_1 / (2 - f);
                    end
                  else
                    interest := p * f_1;
                  if hard_payment then Round2(interest); {@round}
                  p := p + interest - d;
                end;
              ComputeNextAndLoadPrintVariables;
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
procedure PrepareScreen(code :byte);
{$ifndef OVERLAYS} {$ifndef TESTING} {$F-} {$endif} {$endif}
          begin
          fancycode:=code;
//          AmortizationScreen;
          end;
{$F+}
procedure Init;
{$ifndef OVERLAYS} {$F-} {$endif}
          begin
          thisrun:=iAMZ;
{$ifndef TOPMENUS}
          menuptr:=@menuline[iAMZ];
{$endif}
          end;

begin

PEData.EnterProc[iAMZ]:=Enter;
PEData.ScreenProc[iAMZ]:=PrepareScreen;
PEData.InitProc[iAMZ]:=Init;
PEData.CloseProc[iAMZ]:=NullProc;
{$ifdef TESTING}
TESTHIGH.AMZEnter:=AMORTIZE.Enter;
TESTHIGH.GetAMZNumbers:=AMORTIZE.FirstPass;
{$endif}

end.
