unit AMORTOP;
{$ifdef OVERLAYS} {$F+,O+} {$endif}


INTERFACE

//uses OPCRT,VIDEODAT,NORTHWND,INPUT,PETYPES,PEDATA,LOTUS,INTSUTIL,PEPANE,IOUNIT,COMDUTIL,KCOMMAND,TABLE,AMZUTIL
//{$ifdef TOPMENUS} ,MCMENU {$endif}
//{$ifdef TESTING} ,TESTHIGH {$endif}
//     ;
uses VIDEODAT,PETYPES,PEDATA,INTSUTIL, Globals, Classes;

       const
    minpmt = 1.0;
    FR_BALLOON = 1;
    bright = true;
    dim = false;
    value_calc = true;
    no_value_calc = false;
    entire = true;
    til_adj = false;
{$ifdef MAC}
    linesperpage = 58;
    lineacross = '\-';
{$else}
    lineacross = '--------------------------------------------------------------------------------';
{$endif}

  type
    saved_balloon_state = object
        save_next_balloon, save_npre, save_next_adj: integer;
        saved: real;
        save_pre: array[1..maxprepay] of ^prepaymentrec;
        procedure Free;
        procedure Restore;
        procedure Save;
      end;

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

  var
    f, f_1, p, usap, d, int_to_date, prorate, cumint, cumamt, r78base: real;
    nrepay, count, lines: integer;
    t, paidthru, nextt, very_last: daterec;
    more_dates_than_lines,balance_calc,hard_payment: boolean;
    ws: str80;
    old_pre: prepaymentarray;
    old_npre, old_next_balloon: integer;

  var
    aprvalue, v_rate, truerate: real;
    repay_from: daterec;
    nballoons, next_balloon, unkballoon, nadj, user_nballoons, next_adj, npre, unkpre: integer;
    ki: char;
    prepaid, adj_fully_specified: boolean;

    balloonsok, adjok, preok: boolean; {lastok summarizes lastdate and n}

    payment, nextpayment: paymenttype;
    skipmonthset: byteset;

 {templates}
//  procedure AmortizationScreen;
  procedure CheckPrepayments;
  procedure ComputeTrueRate;
  procedure DecideWhetherToPrintALine (t, nextt: daterec; Output: TStringList; bCommaSeperated: boolean );
  procedure DetermineVeryLast;
  function DetermineLastPaymentDate (p, usap: real): boolean;
  function GrowthPerPeriod: real;
  function Iterate (p, usap: real; loandate, firstdate: daterec; var x: real; entire_or_no: boolean): boolean;
//  procedure LineAcrossSpreadsheet;
//  procedure NewPage;
  function PrepaidInterest: real;
  procedure PrintGrandTotals( Output: TStringList; bCommaSeperated: boolean );
  procedure PrintSummaryLine(Output: TStringList; bCommaSeperated: boolean );
  function R78Header1:str80;
{procedure Re_Amortize (var p: real; with_print: boolean); }
  procedure RepayLoan (var p: real);
  procedure ReportFinalPayment( Output: TStringList; bCommaSeperated: boolean );
  procedure SortBalloons (maxballoons: shortint);
  procedure SortAdj (maxadj: shortint);  {Not integer! you take pred(0) at one point}
  procedure RepayFancyLoan(var p,usapart:real; loandate,firstdate:daterec; Output: TStringList; bCommaSeperated: boolean; entire,value_calc:boolean; adjnum:byte);
//  procedure WriteOneLotusLine (var amt, int: real);
  function template: str12;
  function TimeForSummary (t, nextt: daterec): boolean;
//  function TwoLinesOut (first: boolean): boolean;
  function VeryLastRegularAmount: real;

implementation

uses Amortize, HelpsystemUnit;

  function template: str12;
  begin
    if fancy then
      template := 'TEMPLATE.%X'
    else
      template := 'TEMPLATE.%A';
  end;

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

  procedure saved_balloon_state.Free;
  var i :byte;
  begin
    for i := 1 to maxprepay do
      if (save_pre[i]<>nil) then begin
         FreeMem(save_pre[i],sizeof(prepaymentrec));
         save_pre[i]:=nil;
         end;
  end;

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

  procedure SortBalloons (maxballoons: shortint);
    var
      i, j, order,cursortoi,cursorfromi: shortint;
      again,redisplay: boolean;

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

  procedure SortAdj (maxadj: shortint);
    var
      i, j, cursortoi,cursorfromi: shortint;
      amtok, whenok, aprok, allamtok, allwhenok, allaprok: boolean;

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

  procedure Paymenttype.Init (bdate, pdate: daterec; npayamt: real);
  begin
    base_date := bdate;
    prevdate := pdate;
    nextpayamt := npayamt;
  end;

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
    date := base_date;
    AddPeriod(date, h^.peryr, h^.firstdate.d, add);
    if (date.m in skipmonthset) then payamt:=0 else payamt := d;
    FindNextExtra(xsource, nextextra);

    balloonpos := 1;
    if (xsource > 0) then
      begin
        balloonpos := DateComp(nextextra.date, date);
        if (DateComp(date, h^.lastdate) > 0) then
          balloonpos := -1;
        if (balloonpos < 0) then
          begin
            payamt := nextextra.amount;
            date := nextextra.date;
            CheckOffBalloon(xsource);
          end
        else if (balloonpos = 0) then
          begin
            if (df.c.plus_regular) then
              payamt := payamt + nextextra.amount
            else
              payamt := nextextra.amount;
            CheckOffBalloon(xsource);
          end;
      end;
    if (balloonpos >= 0) then
      base_date := date;
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
    if (hard_payment) then Round2(interest); {@round}
    
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
    p := p + interest - payamt;
    if (df.c.USARule) then
      begin
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

  procedure ReportFinalPayment( Output: TStringList; bCommaSeperated: boolean );
    const fpamount:string[22]='Final payment amount: ';
    var
      ws: string;
  begin
//    CheckRoomOnPage(0, false, NewPage);
    if( bCommaSeperated ) then
      ws := ',' + fpamount + ',' + ftoa2(payment.payamt, 12, df.h.commas)
    else
      ws := fpamount + ftoa2(payment.payamt, 12, df.h.commas);
    Output.Add( ws );
//    writeln(indent,fpamount,ws);
//    if (destin > tolotus) then
//      if not OutputLine(Concat(indent,fpamount, ws)) then
//        abort := true;
  end;

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

procedure PrintAndReset(t,nextt :daterec; Output: TStringList; bCommaSeperated: boolean ); {PC version}
          var ta,xgo,ygo             :byte;
              amt,int                :real;
              ki                     :char;
              precum,lastline        :boolean;
              nextnext               :daterec;
              Seperator              : char;

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
           {Adjust last payment to cover entire remaining principal.}
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
            ws:=strb(paynum,4) + ' ' +Seperator+' '+DateStr(date) + Seperator + ftoa2(int,14,df.h.commas)
               + Seperator + ftoa2(amt-int,15,df.h.commas) + Seperator + ftoa2(principal,16,df.h.commas)
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

  procedure DisposeOfOld_Pre;
    var
      i :byte;
    begin
    for i:=1 to maxprepay do if (old_pre[i]<>nil) then begin
       FreeMem(pointer(old_pre[i]),sizeof(prepaymentrec));
       old_pre[i]:=nil;
       end;
    end;

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

for old_npre := 1 to maxprepay do
  old_pre[old_npre]:=nil;

end.

