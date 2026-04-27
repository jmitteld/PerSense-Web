unit PRESVALU;

INTERFACE
{$ifdef OVERLAYS} {$F+,O+} {$endif}
{$ifdef CHEAP} {$define PLANNER} {$endif}

//uses OPCRT,VIDEODAT,NORTHWND,INPUT,LOTUS,PETYPES,PEDATA,INTSUTIL,PEPANE,COMDUTIL,KCOMMAND,PVLUTIL
//{$ifndef PLANNER}
//,TABLE,PVLTABLE
//{$endif}
//{$ifdef TESTING} ,TESTLOW,IOUNIT {$endif}
//{$ifdef ACTU} ,ACTUARY  {$endif}
//{$ifdef PVLX} ,PVLXSCRN {$else} ,MCMENU {$endif}
//;

uses VIDEODAT, PETYPES, PEDATA, INTSUTIL, Globals, PVLUTIL, PVLXSCRN, pvltable, Classes;

procedure Enter(code :byte);
procedure MakeTable( Output: TStringList; bCommaSeperated: boolean );
{$ifdef V_3}
procedure BackwardCalc;
procedure FrontwardCalc(k :byte);
procedure ComputeLumpsumLineValues(k :byte);
procedure ComputePeriodicLineValues(k :byte);
{$ifdef ACTU}
procedure PrepareForLife;
{$endif ACTU}
{$endif V_3}
procedure ZeroPresVal( TheRec: presvalptr );
procedure ZeroPeriodic( TheRec: periodicptr );
procedure ZeroLumpSum( TheRec: lumpsumptr );
procedure ZeroRateLine( TheRec: ratelineptr );
procedure ZeroXPresVal( TheRec: xpresvalptr );
function LumpSumIsEmpty( ThePtr: lumpsumptr ): boolean;
function PeriodicIsEmpty( ThePtr: periodicptr ): boolean;
function PresValIsEmpty( ThePtr: presvalptr ): boolean;
function RateLineIsEmpty( ThePtr: ratelineptr ): boolean;
function XPresValIsEmpty( ThePtr: xpresvalptr ): boolean;

IMPLEMENTATION

uses HelpSystemUnit;

{$ifdef ACTU}
const no_time_with_life:str80='Sorry - can''t perform date-targeted computations with Life Expectancy Table.';
{$endif}
const positive_negative_message='Inconsistent data: positive amount can never have negative value.';

procedure ZeroPresVal( TheRec: presvalptr );
begin
  with TheRec^ do begin
  asofstatus    := empty;
  asof          := unkdate;
  r.status      := empty;
  r.rate        := 0;
  r.peryr       := 0;
  sumvaluestatus:= empty;
  sumvalue      := 0;
  status        := 0;
  durationstatus:= empty;
  duration      := 0;
  end;
end;

procedure ZeroPeriodic( TheRec: periodicptr );
begin
  with TheRec^ do begin
  fromdatestatus        := empty;
  fromdate              := unkdate;
  todatestatus          := empty;
  todate                := unkdate;
  peryrstatus           := empty;
  peryr                 := 0;
  amtnstatus            := empty;
  amtn                  := 0;
  colastatus            := empty;
  cola                  := 0;
  valnstatus            := empty;
  valn                  := 0;
  status                := 0;
  ninstallments         := 0;
  actn                  := 0;
  end;
end;

procedure ZeroLumpSum( TheRec: lumpsumptr );
begin
  with TheRec^ do begin
  datestatus    := empty;
  date          := unkdate;
  amt0status    := empty;
  amt0          := 0;
  val0status    := empty;
  val0          := 0;
  status        := 0;
  act0          := 0;
  end;
end;

procedure ZeroRateLine( TheRec: ratelineptr );
begin
  with TheRec^ do begin
    datestatus  := empty;
    date        := unkdate;
    r.status    := empty;
    r.rate      := 0;
    r.peryr     := 0;
    status      := 0;
  end;
end;

procedure ZeroXPresVal( TheRec: xpresvalptr );
begin
  with TheRec^ do begin
    xasofstatus         := empty;
    xasof               := unkdate;
    simplestatus        := empty;
    simple              := true;
    xvaluestatus        := empty;
    xvalue              := 0;
    status              := 0;
  end;
end;

function PresValIsEmpty( ThePtr: presvalptr ): boolean;
begin
  if( (ThePtr.asofstatus=empty) and (ThePtr.r.status=empty) and
      (ThePtr.sumvaluestatus=empty) and (ThePtr.durationstatus=empty) ) then
    PresValIsEmpty := true
  else
    PresValIsEmpty := false;
end;

function PeriodicIsEmpty( ThePtr: periodicptr ): boolean;
begin
  if( (ThePtr.fromdatestatus=empty) and (ThePtr.todatestatus=empty) and
      (ThePtr.peryrstatus=empty) and (ThePtr.amtnstatus=empty) and
      (ThePtr.colastatus=empty) and (ThePtr.valnstatus=empty) ) then
    PeriodicIsEmpty := true
  else
    PeriodicIsEmpty := false;
end;

function LumpSumIsEmpty( ThePtr: lumpsumptr ): boolean;
begin
  if( (ThePtr.datestatus=empty) and (ThePtr.amt0status=empty) and
      (ThePtr.val0status=empty) ) then
    LumpSumIsEmpty := true
  else
    LumpSumIsEmpty := false;
end;

function RateLineIsEmpty( ThePtr: ratelineptr ): boolean;
begin
  if( (ThePtr.datestatus=empty) and (ThePtr.r.status=empty) ) then
    RateLineIsEmpty := true
  else
    RateLineIsEmpty := false;
end;

function XPresValIsEmpty( ThePtr: xpresvalptr ): boolean;
begin
  if( (ThePtr.xasofstatus=empty) and (ThePtr.simplestatus=empty) and (ThePtr.xvaluestatus=empty) ) then
    XPresValIsEmpty := true
  else
    XPresValIsEmpty := false;
end;

procedure ComputeLumpsumLineValues(k :byte); {k is line in Block 3 to use for rate and date}
             var i        :byte;
                 p        :real;
          begin
          for i:=1 to nlines[PVLLumpSumBlock] do with a[i]^ do begin
              if  (datestatus>=defp) and (amt0status>=defp) and (val0status>=defp) then begin
                  messageBox('To compute present value, enter DATE and AMOUNT but not VALUE in line '+strb(i,0), DP_DateAmountNoValue);
                  status:=over_determined;
                  end
              else if (datestatus>=defp) and ((amt0status>=defp) or (val0status>=defp)) then status:=fully_specified
              else if (datestatus>=defp)  or ((amt0status>=defp) or (val0status>=defp)) then status:=contains_unknown
              else status:=blank_line;
              if (status=contains_unknown) then
                 if (backward) and (k=1) and (thisrun=iPVL) and (not fold_in_life) then begin
                    MessageBox('Only one line should contain two unknowns in upper blocks.', DP_1Line2Unknowns);
                    errorflag:=true;
                    end
                 else if ((amt0status>=defp) and (amt0=0)) 
                    then RecordError(i-scrollpos[PVLLumpSumBlock]+ztop[PVLLumpSumBlock],amountcol)
                 else if ((val0status>=defp) and (val0=0)) 
                    then RecordError(i-scrollpos[PVLLumpSumBlock]+ztop[PVLLumpSumBlock],valuecol)
                 else if (c[1]^.status>=fully_specified) then begin
                    backward:=true;
                    bf.FixPointers(PVLLumpSumBlock,i);
                    end;
              end;

          {6/27/91 - replaced ok() with status check.
           To make it work, I also put provisional outp status
           bytes in before the two iteration loops in FrontwardCalc

           ...if ok(c[k]^.r.rate) and dateok(c[k]^.asof) then with c[k]^ do
              Can't use status instead of ok() because this procedure is
              called in iterations that generate rate or asof.}

          with c[k]^ do if (r.status>empty) and (asofstatus>empty) then
            for i:=1 to nlines[PVLLumpSumBlock] do with a[i]^ do begin
              if (status=over_determined) then begin
                 MessageBox('You may specify only 2 of 3 columns in PAYMENT block, line '+strb(i,0), DP_TooMuchInPaymentBlock);
                 end
                 else if (datestatus>=defp) and (amt0status>=defp) then begin
                    val0:=amt0*exxp(r.rate*YearsDif(asof,date));
{$ifdef ACTU}
                    if (fold_in_life) then val0:=val0*LifeProb(date,a[i]^.act0);
{$endif}
                    if (k=1) then val0status:=outp;
                    end
(* --------- TAKING THIS OUT 4/1/92 because val0status is never inp anymore *)
                 else if (datestatus>=defp) and (val0status>=defp) then begin
                    amt0:=val0*exxp(-r.rate*YearsDif(asof,date));
{$ifdef ACTU}
                    if (fold_in_life) then begin
                       p:=LifeProb(date,a[i]^.act0);
                       if (p>teeny) then amt0:=amt0/p
                       else begin
                          MessageBox('Error: date of lump sum payment'+strb(i,2)+' is beyond life span.');
                          errorflag:=true;
                          exit;
                          end;
                       end;
{$endif}
                    if (k=1) then amt0status:=outp;
                    end
                 else if (amt0status>=defp) and (val0status>=defp) then begin
{$ifdef ACTU}
                    if (fold_in_life) then begin
                       MessageBox(no_time_with_life);
                       errorflag:=true;
                       end;
{$endif}
                    date:=asof;
                    AddYears(date, - (yrdays/r.rate) * lnn (val0/amt0));
                    if (k=1) then datestatus:=outp;
                    end;
(*----------- val0status is never inp anymore *)
                 end;
          lastk:=k;
          end;

function SumFormula(lnf,n :real):real;
         var secondorder                      :boolean;
             arg,oneminusexpnrt,oneminusf     :real;
         begin
         if (abs(lnf)<teeny) then {zeroth order calculation}
            SumFormula:=n
         else begin
            secondorder:=(abs(lnf)<tiny);
            if (secondorder) then begin
               arg:=n*lnf;
               oneminusexpnrt:= - arg - half*sqr(arg);
               oneminusf:= -lnf - half*sqr(lnf);
               end
            else begin
               oneminusexpnrt:=1-exxp(n*lnf);
               oneminusf:=1-exxp(lnf);
               end;
            SumFormula:= oneminusexpnrt / oneminusf;
            end;
         end;

function SummationForSteppedCola(k,j :byte):real;  {Bug fixed, 2/91}
         {k is the line number in BLOCK 3 from which to take RATE and DATE.
          j is the line number in BLOCK 2 which we're computing.}

         var first,value_of_1,theresult,
             current_pmt,normalized_amt,
             exp_cola,part                   :real;
             lastof2d,t,coladate             :daterec;
             nfullyears                      :integer;

         begin with b[j]^ do with c[k]^ do begin
         normalized_amt:=1;
         exp_cola:=exxp(cola);
         theresult:=0;  t:=fromdate;
         coladate:=fromdate;
         if (df.c.colamonth in [1..12]) then begin
            if (df.c.colamonth<=fromdate.m) then inc(coladate.y);
            coladate.m:=df.c.colamonth;
            coladate.d:=1;
            end
         else inc(coladate.y);
         if (df.c.exact) or (peryr in [26,52]) {$ifdef ACTU} or (fold_in_life) {$endif} then begin
           {Note that you MUST use the exact method if payments are weekly or
            biweekly, because there is no assurance that each year has the same
            number of payments in it as the other years.}
            while (DateComp(t,todate)<=0) do begin
{$ifdef ACTU}
               part:=normalized_amt*exxp(-YearsDif(t,asof)*r.rate);
               if (fold_in_life) then part:=part*LifeProb(t,b[j]^.actn);
               theresult:=theresult + part;
{$else}
               theresult:=theresult + normalized_amt*exxp(-YearsDif(t,asof)*r.rate);
{$endif}
               AddPeriod(t,b[j]^.peryr,fromdate.d,add);
               if (DateComp(t,coladate)>=0) then begin
                  normalized_amt:=normalized_amt*exp_cola;
                  inc(coladate.y);
                  end;
               end;
            SummationForSteppedCola:=theresult;
            exit;
            end;

         {We divide time up into three periods:  the first and last are handled
          with the "exact" method above, doing the summation explicitly.  The
          middle period is an exact number of years, and the summation formula
          is used (twice) to compute it more quickly.}

         {I. If the "Anniversary" option is selected, the first period is nil.
             Otherwise, the first period extends from fromdate up to but
             not including coladate:}
          if (df.c.COLAmonth<>ANN) then
            while (DateComp(t,coladate)<0) do begin
               theresult:=theresult + exxp(-YearsDif(t,asof)*r.rate);
               AddPeriod(t,b[j]^.peryr,fromdate.d,add);
               end;

         {II. The value of t that comes out of the above is the beginning
             of the second period.  The second period continues up to but
             not including the last coladate that is strictly before todate.}

         if (DateComp(t,coladate)>=0) then begin {changed from > to >=, 2/13/91}
           current_pmt:=exp_cola;
           inc(coladate.y);
           end
         else current_pmt:=1;

         lastof2d:=t;
         AddPeriod(lastof2d,b[j]^.peryr,fromdate.d,subtract);
         lastof2d.y:=todate.y;
         if (DateComp(lastof2d,todate)>0) then dec(lastof2d.y);
         nfullyears:=lastof2d.y-t.y;
         if (lastof2d.m>t.m) then inc(nfullyears);

         {Use the summation formula once to get the value of the first full
          year of the 2d period.}
         first:=exxp(-r.rate*YearsDif(t,asof)); {fromdate?}
         value_of_1:=first*SumFormula(-r.rate/RealPerYr(peryr),RealPerYr(peryr)) * current_pmt;

         {Now use the summation formula again to sum over the exact number of
          years in the 2d period.}
         theresult:=theresult+value_of_1*SumFormula(cola-r.rate,nfullyears);

         {III. The third period begins nfullyears after the t that we were
             left with after the calculation of I (which hasn't been altered
             in the calculation of II).  It ends with todate, and the sum
             is done explicitly.}
         t.y:=t.y+integer(nfullyears);
         current_pmt:=current_pmt*exxp(nfullyears*cola); {current_pmt* added 2/13/91}
         while (DateComp(t,todate)<=0) do begin
           theresult:=theresult + exxp(-YearsDif(t,asof)*r.rate) * current_pmt;
           AddPeriod(t,b[j]^.peryr,fromdate.d,add);
           end;
         SummationForSteppedCola:=theresult;
         end;end;

function Summation(k,j :byte):real; {This version is currently active.}
         {Modified 7/19/89 for closer agreement with std amortization.}
         {k is the line number in BLOCK 3 from which to take RATE and DATE.
          j is the line number in BLOCK 2 which we're computing.}

         var lnf,sum,exprt,oneminusf,
             ff,arg,part,since,
             oneminusexpnrt,theresult           :real;
             since_from,secondorder,
             zerothorder                     :boolean;
             stdloandate,t                   :daterec;

         begin with b[j]^ do with c[k]^ do begin
         lnf:=(cola-r.rate)/RealPerYr(peryr);
         if (lnf>=0) and (todate.y=latest.y) and (not fold_in_life) then begin
            errorflag:=true;
            MessageBox('Value of payments that extend forever is infinite if interest rate ó 0.', DP_PaymentInfinite );
            sumvalue:=1; sumvaluestatus:=badp; Summation:=1; {Zero produces error 200}
            exit; end;
         if (cola<>0) and (df.c.COLAmonth<>CNT) and (b[j]^.peryr>1) then begin
//            RequestPatience;
            Summation:=SummationForSteppedCola(k,j);
            exit;
            end;
         if (df.c.exact) {$ifdef ACTU} or (fold_in_life) {$endif} then begin
//            RequestPatience;
            theresult:=0;
            t:=fromdate;
            part:=1;
            while (DateComp(t,todate)<=0) and (abs(part)>teeny) do begin
{$ifdef ACTU}
               part:=exxp(YearsDif(t,fromdate)*cola-YearsDif(t,asof)*r.rate);
               if (fold_in_life) then part:=part*LifeProb(t,b[j]^.actn);
               theresult:=theresult + part;
               if (b[j]^.actn in [DEAD,ONLY_1,ONLY_2]) and (DateComp(t,actu_now)<=0) then part:=1;
                   {Under this condition, part will be zero, even though the non-zero
                    part of the contingent payements is yet to come.  We don' want (part<teeny)
                    to cause the summation above to truncate (JJM 2/28/93).}
{$else}
               theresult:=theresult + exxp(YearsDif(t,fromdate)*cola-YearsDif(t,asof)*r.rate);
{$endif}
               AddPeriod(t,b[j]^.peryr,fromdate.d,add);
               end;
            Summation:=theresult;
            exit;
            end;
         zerothorder:=false;
         if (abs(lnf)<teeny) then begin
            zerothorder:=true;
            sum:=ninstallments;
            since:=YearsDif(asof,fromdate);
            oneminusf:=teeny;
            end
         else secondorder:=(abs(lnf)<tiny);
         if (not zerothorder) then begin
             {If FROMDATE is after ASOFDATE, then we assume
              this is a loan, and amortize it according to standards of the
              industry.  We'll make it agree exactly for an ASOFDATE
              (loan date) exactly one interest period earlier than FROMDATE:}
            if (DateComp(asof,fromdate)<=0) or (todate.y=latest.y) then begin
                                                  { \ needed to prevent overflow}
               since_from:=true;
               if (secondorder) then begin
                  arg:=ninstallments*lnf;
                  oneminusexpnrt:= - arg - half*sqr(arg);
                  end
               else oneminusexpnrt:=1-exxp(ninstallments*lnf);
               stdloandate:=fromdate;
               AddPeriod(stdloandate,peryr,stdloandate.d,subtract);
               since:=YearsDif(asof,stdloandate);
               if (secondorder) then oneminusf:= -lnf - half*sqr(lnf)
               else oneminusf:=1-exxp(lnf);
               end
            else begin
               since_from:=false;
               if (secondorder) then begin
                  arg:=ninstallments*lnf;
                  oneminusexpnrt:=arg - half*sqr(arg);
                  end
               else oneminusexpnrt:=1 - exxp(-ninstallments*lnf);
               since:=YearsDif(asof,todate);
               if (secondorder) then oneminusf:=lnf - half*sqr(lnf)
               else oneminusf:=1-exxp(-lnf);
               end;
            if (fromdate.y=latest.y) then oneminusexpnrt:=1;
            sum:= oneminusexpnrt / oneminusf;  {(1-fü)/(1-f)}
            if (since_from) then begin
              {Can't use secondorder here because rate doesn't include cola}
               ff:=exxp(-r.rate/RealPerYr(peryr));
               sum:=sum*ff;
               end
             {Because since is measured from one period before first payment is due.}
            else {zeroth order} if (cola<>0) then sum:=sum * exxp(YearsDif(todate,fromdate)*cola);
             {Because lnf above is defined with (r.rate-cola), initial amount
              needs to be adjusted.}
            end;
         exprt:=exxp(r.rate*since);
         theresult:=exprt*sum;
         Summation:=theresult;
         end;end;

procedure ComputePeriodicLineValues(k :byte); {k is line in Block 3 to use for rate and date}
          var j    :byte;
          begin
          for j:=1 to nlines[PVLPeriodicBlock] do with b[j]^ do begin
(*
              if (not ((peryrstatus>=defp) and (peryr>0))) then status:=0 {times per year and cola necessary}
              else if (fromdatestatus>=defp) and (todatestatus>=defp) and (amtnstatus>=defp) and (valnstatus>=defp) then begin
                  MessageBox('To compute present value, enter DATES and AMOUNT but not VALUE in line '+strb(j,0));
                  status:=over_determined; {Should never happen}
                  end
              else if (fromdatestatus>=defp) and (todatestatus>=defp) and ((amtnstatus>=defp) or (valnstatus>=defp))
                then status:=fully_specified
              else if ((fromdatestatus>=defp) or (todatestatus>=defp) and (amtnstatus>=defp))
                or (fromdatestatus>=defp) and (todatestatus>=defp)
                then status:=contains_unknown
              else status:=blank_line;
*)
{THIS IS NEW, 3/31/92}
              if (peryrstatus>=defp) and (peryr>0) then status:=fully_specified
              else status:=missing_4;
              if (fromdatestatus<defp) then dec(status);
              if (todatestatus<defp) then dec(status);
              if (amtnstatus<defp) then dec(status);
              if (valnstatus>defp) then inc(status);
{THIS IS NEW, 3/31/92}

              if (status=contains_unknown) then
                 if (backward) and (k=1) then begin
                    MessageBox('Only one line should contain two unknowns in Upper Right block.', DP_1Line2UnknownsUpperRight);
                    errorflag:=true;
                    end
                 else if ((amtnstatus>=defp) and (amtn=0)) 
                    then RecordError(j-scrollpos[PVLPeriodicBlock]+ztop[PVLPeriodicBlock],pamountcol)
                 else if ((valnstatus>=defp) and (valn=0)) 
                    then RecordError(j-scrollpos[PVLPeriodicBlock]+ztop[PVLPeriodicBlock],pvaluecol)
                 else if (nlines[PVLPresValBlock]>0) and (c[1]^.status>=fully_specified) then begin
                   backward:=true;
                   bf.FixPointers(PVLPeriodicBlock,j);
                   end;
              end;

          {6/27/91 - replaced ok() with status check.
           To make it work, I also put provisional outp status
           bytes in before the two iteration loops in FrontwardCalc

           if ok(c[k]^.r.rate) and dateok(c[k]^.asof) then with c[k]^ do
             {Can't use status instead of ok() because this procedure is
              called in iterations that generate rate or asof.}

          with c[k]^ do if (r.status>empty) and (asofstatus>empty) then
            for j:=1 to nlines[PVLPeriodicBlock] do with b[j]^ do begin
              if (status=over_determined) then begin
                 MessageBox('You may specify only 2 of 3 columns in PAYMENT block, line '+strb(j,0), DP_Only2Of3InPayment);
                 end
              else if (fromdatestatus>=defp) and (todatestatus>=defp) and ((amtnstatus>=defp) or (valnstatus>=defp)) then begin
                   if (amtnstatus>=defp) then begin
                      valn:=amtn*Summation(k,j);
                      if (k=1) then valnstatus:=outp;
                      end
(* --------- TAKING THIS OUT 4/1/92 because valnstatus is never inp anymore *)
                   else if (valnstatus>=defp) then begin
                      amtn:=valn/Summation(k,j);
                      if (k=1) then amtnstatus:=outp;
                      end;
(*  -------- because valnstatus is never inp anymore *)
                   end;
               end;
          end;

function YieldRateTranslation (k: byte): byte;
         begin with c[k]^.r do begin
            if (status <= outp) then
               YieldRateTranslation := missing_3
            else
               YieldRateTranslation := missing_2;
//         DisplayCell(PVLPresValBlock, k, tratecol); {displays other 2 as well}
         end;
                                                                                            end;
procedure FirstPass;
      var
        i, j, k: integer;
        saveto :daterec;
    begin {FirstPass}
      errorflag := false;
      overflowflag := false;

{ ifdef ACTU
      if (not fold_in_life) then podunk:=false;
         7/93, added the "if" clause, because it was resetting podunk to false after it had been set true.
         2/94 - this whole clause removed and podunk set to false when ENTER is pressed. 
 $endif}
{$ifdef PVLX}
      if (PVLfancy) then
        begin
          FancyFirstPass;
//          DisplayAll;
          exit;
        end;
{$endif}

              {BLOCK 3}
      k:=1;
      while (k<=nlines[PVLpresvalblock]) do
        {Don't use FOR loop because DeleteRow can change nlines, so it must be re-evealuated each time.}
        with c[k]^ do
          begin
            status := YieldRateTranslation(k);
            if (status < over_determined) then
              begin
                if (asofstatus > outp) then
                  status := succ(status);
                if (sumvaluestatus > outp) then
                  status := succ(status);
                if (status=missing_3) and (nlines[PVLPresValBlock]>1) then
                  begin
                    ZeroPresVal( presvalptr(c[k]) );
                    dec(nlines[PVLpresvalblock]);
//                    DeleteRowOfBlock(PVLPresValBlock,k-scrollpos[PVLPresValBlock]);
                    dec(k);
                  end;
              end;
            inc(k);
          end;
      backward := false;

      ComputeLumpsumLineValues(1); {also determines status byte, etc}

             {BLOCK 2}
      j:=1;
      while (j<=nlines[PVLperiodicblock]) do
        with b[j]^ do
          begin
            if (fromdatestatus>=defp) and (todatestatus>=defp) and (peryrstatus>=defp) and (peryr>0) then
             if (DateComp(fromdate, todate) >= 0) then
                begin
                  {MessageBox(Concat('Your dates are out of order, line ', strb(j, 0)));}
                  MessageBox('Your dates are out of order, line '+strb(j, 0)+'. (Check setting for Yr to divide century)', DP_DatesOutOfOrder);
                  errorflag := true;
                end
              else begin
                saveto:=todate;
                ninstallments:=NumberOfInstallments(fromdate,todate,peryr,on_or_before);
                if (DateComp(saveto,todate)<>0) then todatestatus:=defp;
                end;
            if (colastatus<inp) then
              cola := 0;

  {Determine status:}
            status := fully_specified;
            if (fromdatestatus < defp) then
              dec(b[j]^.status);
            if (todatestatus < defp) then
              dec(b[j]^.status);
            if (amtnstatus < defp) and (valnstatus < defp) then
              dec(b[j]^.status);
            if (status=missing_3) then begin
              ZeroPeriodic( periodicptr(b[j]) );
              dec( nlines[PVLPeriodicBlock] );
//              DeleteRowOfBlock(PVLPeriodicBlock,j-scrollpos[PVLPeriodicBlock])
            end else begin
              if (b[j]^.peryrstatus<defp) then dec(b[j]^.status,4);
              inc(j);
              end;
          end;
      ComputePeriodicLineValues(1); {also determines status byte, etc}
      i:=1;
      while (i<nlines[PVLLumpSumBlock]) do with a[i]^ do
        begin
          if (a[i]^.status=empty) then begin
             ZeroLumpSum( lumpsumptr(a[i]) );
             dec( nlines[PVLLumpSumBlock] );
//             DeleteRowOfBlock(PVLLumpSumBlock,i-scrollpos[PVLLumpSumBlock])
          end else
             inc(i);
        end;
      frontward := false;
      for i := 1 to nlines[PVLlumpsumblock] do
        if (a[i]^.status = fully_specified) then
          frontward := true;
      for j := 1 to nlines[PVLperiodicblock] do
        if (b[j]^.status = fully_specified) then
          frontward := true;
      for i := 1 to nlines[PVLlumpsumblock] do
        if (a[i]^.status <= contains_unknown) and (a[i]^.status > empty) then
          frontward := false;
      for j := 1 to nlines[PVLperiodicblock] do
        if (b[j]^.status <= contains_unknown) and (b[j]^.status > empty) then
          frontward := false;
    end; {FirstPass}

procedure FrontwardCalc(k :byte);
          var i,j,count,nlines1,nlines2              :byte;
              sum,diff,yrs,exprt,oldsum,denom        :real;
              second_time                            :boolean;
{$ifdef TESTING}
              saveactive                             :boolean;
              label START_LOOP;
{$endif}
              label START_AGAIN_FROM_0;
          begin
          nlines1:=nlines[PVLLumpSumBlock];
          nlines2:=nlines[PVLPeriodicBlock];
          if (c[k]^.status=contains_unknown) then with c[k]^ do begin
             if (sumvaluestatus<defp) then begin
                sumvalue:=0;
                for i:=1 to nlines1 do
                    if (a[i]^.status>=fully_specified) then with a[i]^ do
                       if (val0status>=defp) then sumvalue:=sumvalue+val0
                       else if (amt0status>=defp) then begin
{$ifdef ACTU}
                         if (fold_in_life) then
                            sumvalue:=sumvalue+amt0*exxp(r.rate*YearsDif(asof,date))*LifeProb(date,a[i]^.act0)
                         else
{$endif}
                            sumvalue:=sumvalue+amt0*exxp(r.rate*YearsDif(asof,date));
                         end;
                for j:=1 to nlines2 do
                    if (b[j]^.status>=fully_specified) then with b[j]^ do begin
                       if (valnstatus<inp) then
                         if (valnstatus<=empty) or (k<>lastk) then valn:=amtn*Summation(k,j);
                       sumvalue:=sumvalue + valn;
                       end;
{$ifdef ACTU}
                if (fold_in_life) then sumvalue:=sumvalue+PodValue(asof,r.rate);
{$endif}
                sumvaluestatus:=outp;
                end
             else if (r.status<defp) then begin
                r.status:=fromcalc; {Provisional. Needed so ComputeLineValues will use it.}
                r.rate:=0.1;  {First guess}  count:=0;
//                RequestPatience;
                second_time:=false;
START_AGAIN_FROM_0:
                repeat
                  if (overflowflag) then begin
                    r.status:=empty;
                    exit; end;
                  inc(count); sum:=0;
                  for j:=1 to nlines1 do
                    if (a[j]^.status>=fully_specified) then with a[j]^ do begin
                       yrs:=YearsDif(asof,date);
                       exprt:=exxp(r.rate*yrs);
                       if (val0status>=inp) or ((val0status>=defp) and (lastk=k)) then sum:=sum+val0
                       else sum:=sum + amt0*exprt;
                       end;
{$ifdef ACTU}
                  if (fold_in_life) then sum:=sum+PODValue(asof,r.rate);
{$endif}
                  for j:=1 to nlines2 do
                    if (b[j]^.status>=fully_specified) then with b[j]^ do begin
                       if ((valnstatus<defp) or (lastk<>k)) then valn:=amtn*Summation(k,j);
                       sum:=sum + valn;
                       end;
                  {Substitute numerical derivative for the Keith Altman region.
                     Also works better in general.}
                  denom:=(sum-oldsum);
                  if (abs(denom)<teeny) then denom:=teeny; {prevent divide by 0}
                  if (count=1) then diff:=0.001
                  else diff:= (sumvalue-sum) * diff / denom;
                  if (count=2) and (diff=0) then begin
                     MessageBox('Rate is not determined - specify amounts instead of values.', DP_RateNotDetermind);
                     errorflag:=true; r.status:=empty;
                     {#Menu}
                     exit;
                     end;
                  oldsum:=sum;
                  if (diff<-0.04) then diff:=-0.04
                  else if (diff>0.04) then diff:=0.04;
                  r.rate:=r.rate - diff;
                until (abs(diff)<teeny) or (count=30);
                if (count=30) then begin
                  if (second_time) then begin
                    MessageBox('"Rate" computation did not converge, line '+strb(k,0)+'.', DP_RateDidNotConverge);
                    r.status:=empty;
                    end
                  else begin
                    second_time:=true;
                    r.rate:=0;
                    goto START_AGAIN_FROM_0;
                    end;
                  end
                else if (not backward) then begin
                  for i:=1 to nlines1 do if (a[i]^.status>=fully_specified) then dec(a[i]^.status);
                  ComputeLumpsumLineValues(k);
                  for j:=1 to nlines2 do if (b[j]^.status>=fully_specified) then dec(b[j]^.status);
                  ComputePeriodicLineValues(k);
                  end;
                {#Menu;}
                end
          else if (asofstatus<defp) then with c[k]^ do begin
                if (abs(r.rate)<teeny) then begin
                   MessageBox('Cannot compute date - interest rate too small (line '+strb(k,1)+').', DP_InterestTooSmall);
                   errorflag:=true;
                   exit;
                   end;
                asof.d:=1; asof.m:=1; asof.y:=100;   {First guess}  count:=0;
                asofstatus:=outp; {Provisional. Needed so ComputeLineValues will use it.}
//                RequestPatience;
                repeat
{$ifdef TESTING}
START_LOOP:
                  if (count=0) then begin
                     saveactive:=TestData.active;
                     TestData.active:=false;
                     end;
                 {This procedure goes through 2 "iterations", during which
                   ComputeLumpsumLineValues computes and prints out the
                   values.  The printout is harmless if testing is not active,
                   because it's overwritten, but when testing, it can generate
                   a lot of extraneous error messages.}
{$endif}
                  inc(count);
                  if (overflowflag) then begin
                    asofstatus:=empty;
                    exit;end;
                  if (not backward) or (k>1) then begin
                     ComputeLumpsumLineValues(k);
                     ComputePeriodicLineValues(k);
                     end;
                  sum:=0;
                  for i:=1 to nlines[PVLLumpSumBLock] do
                    {if (a[i]^.status>=fully_specified) then}
                       sum:=sum + a[i]^.val0;
{$ifdef ACTU}
                  if (fold_in_life) then sum:=sum+PODValue(asof,r.rate);
{$endif}
                  for j:=1 to nlines[PVLPeriodicBlock] do
                    {if (b[j]^.status>=fully_specified) then}
                       sum:=sum + b[j]^.valn;
                  diff:=lnn(sumvalue/sum) / r.rate;
                  AddYears(asof,diff);
                until (count>=10) or (abs(diff)<0.002); {<1 day}
{$ifdef TESTING}
                TestData.active:=saveactive;
                count:=99;
                if (count<11) then goto START_LOOP;
{$endif}
                {This doesn't really require iteration, and should
                 always "converge" on the first pass.  Last (second)
                 pass recalculates line values and sums.}
                if (count=10) or (DateComp(asof,maxdate)>0)
                   then begin
                      MessageBox('"As of" computation did not converge, line '+strb(k,0)+'.', DP_ComputationNoConvergeBy);
                      asofstatus:=empty;
                      end
                else begin
                   if (abs(sum-sumvalue)>0.01) then begin
                      sumvalue:=sum;
                      sumvaluestatus:=defp;
                      end;
                   end; {count<10 -> converged}
                {#Menu;}
                end; {if asofstatus...}
             end {if status=contains_unknown}
          else if (k=1) then begin
             ComputeLumpSumLineValues(1);
             ComputePeriodicLineValues(1);
               {For overdetermined line 1 - to get line 1 values back into arrays}
             end;
          lastk:=k;
          end; {FrontwardCalc}

procedure BackwardCalc;
          var j,count                      :byte;
              val,dval,yrs,exprt,altval,
              diff,targetval,
              first,last,f,cor_amt         :real;
              altdate,wdate                :daterec;
              really_subtract,done         :boolean;
              label PUNT;
          begin
{$ifdef PVLX}
          if (pvlfancy) then val:=d^.xvalue
          else val:=c[1]^.sumvalue;
{$ifdef ACTU}
          if (fold_in_life) then
             if PVLfancy then val:=val-XPODValue
             else val:=val-PODValue(c[1]^.asof,c[1]^.r.rate);
{$endif}
{$else}
          val:=c[1]^.sumvalue;
{$ifdef ACTU}
          if (fold_in_life) then
             val:=val-PODValue(c[1]^.asof,c[1]^.r.rate);
{$endif ACTU}
{$endif PVLX}
          for j:=1 to nlines[PVLLumpSumBlock] do
              if (a[j]^.status>=fully_specified) then with a[j]^ do
                 val:=val-val0;
(*
                 if (pvlfancy) then val:=val-val0
                 else val:=val-amt0*exxp(c[1]^.r.rate*YearsDif(c[1]^.asof,date));
             Can't do this /:  it gives the wrong answer if you're in an actuarial calc.
*)
          for j:=1 to nlines[PVLPeriodicBLock] do
              if (b[j]^.status>=fully_specified) then with b[j]^ do
                 val:=val-valn;
{         -> Now we've got the target value, and we'll look for the line that needs
             filling in. }
          j:=0; repeat inc(j) until (j>nlines[PVLLumpSumBlock]) or (a[j]^.status=contains_unknown);
          if (j<=nlines[PVLLumpSumBlock]) then with a[j]^ do begin
             val0:=val;
             val0status:=outp;
             if (datestatus>=defp) {date} then begin
{$ifdef PVLX}
                if (pvlfancy) then amt0:=val0 / ValueOfOnePayment(1,date)
                else amt0:=val0 * exxp(c[1]^.r.rate*YearsDif(date,c[1]^.asof));
{$ifdef ACTU}
                if (fold_in_life) then begin
                   f:=LifeProb(date,a[j]^.act0);
                   if (f>teeny) then amt0:=amt0/f
                   else begin
                      MessageBox('Error: date of lump sum payment'+strb(j,2)+' is beyond life span.');
                      errorflag:=true;
                      exit;
                      end;
                   end;
{$endif}                   
{$else}
                amt0:=val0 * exxp(c[1]^.r.rate*YearsDif(date,c[1]^.asof));
{$endif}
                amt0status:=outp;
                inc(a[j]^.status);
                   {This is a message for the backward/frontward calculator
                    saying that the amount should be treated as known.}
                end
             else if (amt0status>=defp) {amt0} then with c[1]^ do begin
{$ifdef ACTU}
                  if (fold_in_life) then begin
                     MessageBox(no_time_with_life);
                     errorflag:=true;
                     end;
{$endif}
                  if (val0>0) xor (amt0>0) then begin
                     MessageBox(Positive_Negative_Message, DP_PositiveNegativeMessage);
                     errorflag:=true;
                     exit; end;
                  wdate:=asof; count:=0;    {First guess}
//                  RequestPatience;
                  repeat
                    inc(count); if (overflowflag) then exit;
                    yrs:=YearsDif(asof,wdate);
                    exprt:=exxp(r.rate*yrs);
                    val:=amt0*exprt;
                    dval:=amt0*c[1]^.r.rate*exprt;
                    diff:=(val0-val)/dval;
                    if (diff>20) then diff:=20
                    else if (diff<-20) then diff:=-20;
                    AddYears(wdate,-diff);
                    if (wdate.y>199) then count:=30;
                      {Abort if date is too far out (or too early) as it would be if target
                       value is very small and amount is finite.}
                  until (abs(diff)<0.003) {one day} or (count=30);
                  if (count=30) then MessageBox('"Date" computation did not converge, line '+strb(j,0)+'.', DP_DateNotConvergeBy)
                  else begin
                     date:=wdate;
                     datestatus:=outp;
                     inc(a[j]^.status);
                     {a[j]^.datestatus:=outp; {?}
                       {Usually, dateok, amt0ok, etc all indicate whether the USER has
                        specified them initially.  But this is a message for
                        the backward/frontward calculator saying that the date
                        should be treated as known}
                     end;
                  {#Menu;}
                  end
             else {value specified - neither date ok nor amt0 ok} begin
                  MessageBox('Specify either date or amount in Single Payments, line '+strb(j,1), DP_DateOrAmountInSingle);
                  errorflag:=true;
                  end;
             exit;
             end;

          j:=0; repeat inc(j) until (j>nlines[PVLPeriodicBlock] ) or (b[j]^.status=contains_unknown);
          if (j<=nlines[PVLPeriodicBlock] ) then with b[j]^ do begin
             valn:=val;
             valnstatus:=outp;
             if (fromdatestatus>=defp) and (todatestatus>=defp) {fromdate and todate} then begin
{$ifdef PVLX}
{$ifdef BUGSIN}
                if (pvlfancy) and (df.c.colamonth<fromdate.m)
                then Scavenger('C-2');
{$endif}
                if (pvlfancy) then amtn:=valn/FancySummation(j) else
{$endif}
                amtn:=valn/Summation(1,j);
                amtnstatus:=outp;
                inc(b[j]^.status);
                   {This is a message for the backward/frontward calculator
                    saying that the amount should be treated as known}
                end
             else if ((fromdatestatus>=defp) or (todatestatus>=defp)) and (amtnstatus>=defp) then begin
               {fromdate or todate specifed and amount specified}
               targetval:=valn;
               if (targetval>0) xor (amtn>0) then begin
                  MessageBox(positive_negative_message, DP_PositiveNegativeMessage);
                  errorflag:=true;
                  exit; end;
               f:=exxp((cola-c[1]^.r.rate)/RealPerYr(peryr));
               if (todatestatus<defp) then with c[1]^ do begin
                  first:=exxp(r.rate*YearsDif(asof,fromdate));
                  last:=(first - (1-f)*val/amtn) / f;
                  todate:=fromdate;
                  if (abs(r.rate-cola)<teeny) then AddNPeriods(fromdate,todate,peryr,round(val/amtn))
                  else AddYears(todate, - (lnn(last*f/first)/(r.rate-cola) + 1/RealPerYr(peryr)));
                  ninstallments:=NumberOfInstallments(fromdate,todate,peryr,ON_OR_BEFORE);
                  altval:=amtn*Summation(1,j);
                  really_subtract:=(targetval<altval) xor (amtn<0);
                  altdate:=todate;
                     {Now we've got the first approx.  Try adding or subtracting one period:}
                  done:=false;
                  repeat
                    AddPeriod(todate,peryr,fromdate.d,really_subtract);
                    ninstallments:=NumberOfInstallments(fromdate,todate,peryr,ON_OR_BEFORE);
                    valn:=amtn*Summation(1,j);
                       {Test to see whether the original one was better}
                    if (abs(targetval-altval)<abs(targetval-valn)) then begin
                       valn:=altval;  todate:=altdate;  done:=true;
                       end
                    else begin
                       altval:=valn;  altdate:=todate;
                       end;
                  until done or errorflag;
                  if (errorflag) then exit;
                  ninstallments:=NumberOfInstallments(fromdate,todate,peryr,ON_OR_BEFORE);
                    {one last time}
                  todatestatus:=outp;
                  inc(b[j]^.status);
                  {b[j]^.todatestatus:=out; {?}
                    {Usually, dateok, amt0ok, etc all indicate whether the USER has
                     specified them initially.  But this is a message for
                     the backward/frontward calculator saying that the date
                     should be treated as known}
                  end {if todate=unk}
               else if (fromdatestatus<defp) then with c[1]^ do begin
                 {First, we approximate fromdate to find out when to apply rate
                  and when to apply (r.rate-cola) in the formula for fromdate}
{$ifdef V_3}
                  if (colastatus=const_signal) then begin
                     r.rate:=r.rate-cola;
                     cola:=0;
                     end;
{$endif}
                 {First approx:}
                  if (todate.y=latest.y) then begin
                    first:=(1-f)*val/amtn;
                    yrs:= (-1/(r.rate-cola)) * lnn(first);
                    fromdate:=asof;
                    AddYears(fromdate,yrs);
                    end
                  else begin
                    last:=exxp((r.rate-cola)*YearsDif(asof,todate));
                    first:= f*last + (1-f)*val/amtn;
                    fromdate:=todate;
                    if (first<=0) or (abs(r.rate-cola)<teeny) then goto PUNT;
                      {This avoids ln(-x) error when cola>rate and you're calculating
                       a fromdate.  It results in a correct, but very slow calculation.
                       It's such a rare case, I'm not going to make it more elegant now.
                       (5/5/90)}
                      {Second part, about (r.rate-cola<teeny) added 4/28/92 to avoid a divide
                       by zero error in the following line.}
                    AddYears(fromdate, lnn(last*f/first)/(r.rate-cola) + 1/RealPerYr(peryr));
                    end; {method if todate is not "...."}
                 {Second approx:}
                  if (cola<>0) then begin
                     last:=exxp((r.rate)*YearsDif(asof,fromdate)) * exxp((r.rate-cola)*YearsDif(fromdate,todate));
                     first:= f*last + (1-f)*val/amtn;
                     fromdate:=todate;
                     AddYears(fromdate, lnn(last*f/first)/(r.rate-cola) + 1/RealPerYr(peryr));
                     end;
PUNT:
                  ninstallments:=NumberOfInstallments(todate,fromdate,peryr,ON_OR_BEFORE);
                       {This is used backwards to get fromdate to be an
                        even number of periods before todate.}
                  ninstallments:=NumberOfInstallments(fromdate,todate,peryr,ON_OR_BEFORE);
                       {This one actually computes ninstallments.}
                  altval:=amtn*Summation(1,j);
                  really_subtract:=(targetval>altval) xor (amtn<0);
                  altdate:=fromdate;
                     {Now we've got the first approx.  Try adding or subtracting one period
                       until we find the result gets worse:}
                  done:=false;
                  repeat
                    AddPeriod(fromdate,peryr,fromdate.d,really_subtract);
                    ninstallments:=NumberOfInstallments(fromdate,todate,peryr,ON_OR_BEFORE);
                    valn:=amtn*Summation(1,j);
                       {Test to see whether the original one was better}
                    if (abs(targetval-altval)<abs(targetval-valn)) then begin
                       valn:=altval;  fromdate:=altdate; done:=true;
                       end
                    else begin
                       altval:=valn;  altdate:=fromdate;
                       end;
                  until done;
                  ninstallments:=NumberOfInstallments(fromdate,todate,peryr,ON_OR_BEFORE);
                    {We do this again because the last one calculated is one off - the
                     above loop keeps going until things get worse.}
                  fromdatestatus:=outp;
                  inc(b[j]^.status);
                  {b[j]^.fromdatestatus:=outp; {?}
                    {Usually, dateok, amt0ok, etc all indicate whether the USER has
                     specified them initially.  But this is a message for
                     the backward/frontward calculator saying that the date
                     should be treated as known}
                  end;
               {If amount is off by more than 1› then correct AMOUNT}
               if (abs(valn)>small) then cor_amt:=amtn*(targetval/valn) else amtn:=0;
               if (abs(cor_amt-amtn)>0.01) then begin
                  amtn:=cor_amt;
                  amtnstatus:=defp;
                  inc(b[j]^.status);
                  valn:=targetval;
                  end; {if cor_amt}
               end  {fromdate=unk or todate=unk}
             else begin {value and one date specified - can't compute both date and amount.}
               MessageBox('Specify either other date or amount in Periodic Payments, line '+strb(j,1), DP_DateOrAmountInPeriodic);
               errorflag:=true;
               end;
             end; {Compute with b[j]^}
          end; {BackwardCalc}

{$ifdef ACTU}
procedure PrepareForLife;
          begin
          PushWindowNoBorder(0,0,0,0);
          wx1[windex]:=wx1[pred(windex)]; wx2[windex]:=wx2[pred(windex)];
          wy1[windex]:=wy1[pred(windex)]; wy2[windex]:=wy2[pred(windex)];
            {0 is a signal not to change window parameters, or to clear screen.}

        { Puts an extra screen image on the screen stack.  The first time
          we PopWindow, we'll have an image to play with, put "L,N,D" on,
          and calculate a life-expectancy-modified Present Value on.
          The next time we PopWindow, the original PVL screen will be there. }
          end;
{$endif}



{$F+}
procedure Enter(code :byte); {Calculates and tries to decide where you'll want the cursor next.}
          var y,row,i,stopat   :byte;
              kiword                  :word;
              CancelPressed           : boolean;
          label LIFE_CALC;
{$ifndef OVERLAYS} {$F-} {$endif}

(*
   function EscapePressedInOverdeterminedMessage:boolean;
            var kibyte :byte;
            begin
            if (code=with_life) and (i=1) then
              kibyte:=(MessageBox('All blanks are filled in.  Proceed only to compute an unknown POD amount.')
                      and $FF)
            else
              kibyte:=(MessageBox('Warning: value entered in line '+strb(i,0)+' below is already determined by data above.')
                      and $FF);
            EscapePressedInOverdeterminedMessage:=(kibyte=27);
            end;
*)

{$ifdef 0}
no need to do cursor movement in the new version

procedure MoveCursor;
          begin
          y:=whereyabs;
          row:=y-ztop[block];
          i:=row+scrollpos[block];
          case (block) of PVLLumpSumBlock  : if (a[i]^.status>=fully_specified)
                                  then begin col:=datecol; KCOMMAND.Down; end
                              else if (col<valuecol) then TabRt
                              else begin col:=datecol; KCOMMAND.Down; end;
                          PVLPeriodicBlock : if (b[i]^.status>=fully_specified)
                                  then begin col:=fromcol; KCOMMAND.Down; end
                              else if (col<pvaluecol) then TabRt
                              else begin col:=fromcol; gotoxy(startof[col],y); end;
                          PVLRatesBlock : if (pvlfancy) and (cc[i]^.status>=fully_specified) and (y<bbot[block])
                                  then begin col:=asofcol; KCOMMAND.Down; end
                                  else TabRt;
                          else begin {bottom blocks}
                               TabRt;
                               if ((col=lratecol) or (col=yieldcol)) and (c[i]^.r.status>outp)
                                   then MoveToCell(sumvaluecol,y,0);
                               end;
                          end;
          end;
{$endif}

          begin {Enter}
{$ifdef ACTU}
          fold_in_life:=false; podunk:=false;
{$endif}
          if ((screenstatus and needs_calc)>0) or (code<>with_tab) then FirstPass;
             {otherwise you leave Patience.... on the screen}
          if (code=with_tab) then begin
//             MoveCursor;
             if ((screenstatus and needs_calc)=0) then exit;
             end;
          if (errorflag) then exit;
          if (frontward) then
             if (pvlfancy) then begin
                if (d^.xasofstatus=inp) and (d^.xvaluestatus=inp) then begin
                  MessageBoxWithCancel( 'Warning: value entered in line '+strb(i,0)+
                                        ' below is already determined by data above.', CancelPressed, DP_ReDeterminedData );
                  if( CancelPressed ) then exit;
                end;
             end
             else for i:=1 to {scree}nlines[PVLPresValBlock] do
               if (c[i]^.status>=fully_specified) then begin
{$ifdef ACTU}
                 if  (code=with_life) and (i=1) then begin
                   if EscapePressedInMessage('All blanks are filled in.  Proceed only to compute an unknown POD amount.')
                     then exit
                   else podunk:=true;
                   end
                 else begin
{$endif ACTU}
                   MessageBoxWithCancel( 'Warning: value entered in line '+strb(i,0)+
                                             ' below is already determined by data above.', CancelPressed, DP_ReDeterminedData);
                   if( CancelPressed ) then exit;
{$ifdef ACTU}
                 end;
{$endif}
               end;

//          ClearOutputCells;
          FirstPass;
             {...because this information has been lost in ClearOutputCells.}

          if (errorflag) then exit;
{$ifdef ACTU}
          if (code=with_life) then PrepareForLife;  {pushes an extra window on stack}
          if (pvlfancy) and (code=with_life) then
             if (not ActuarialCalc(code)) then begin
               PopWindow;
               DisplayAll;
               exit; end;
          if (not (frontward or backward)) then begin
             if (code=make_table) then begin InsufficientDataMessage(tablestr); exit; end
             else if (code=with_life) then begin
                if (nlines[PVLLumpSumBlock]=0) and (nlines[PVLPeriodicBlock]=0) and (c[1]^.status>=contains_unknown) then begin
                  podunk:=c[1]^.status>contains_unknown;
                  if EscapePressedInMessage('No payments to value. Proceed only to evaluate a POD amount (or press <Esc>).')
                    then begin
                      PopWindow;
                      exit;
                      end;
                  end
                else begin
                  MessageBox('Insufficient data on screen for an actuarial calculation.');
                  PopWindow;
                  exit;
                  end;
                end
             else exit;
             end;

          if (code=with_life) then
             if (pvlfancy) then begin FirstPass; DisplayAll; end
             else {not pvlfancy} begin
                if (ActuarialCalc(code)) then begin
                   FirstPass; {Needed for POD computation?}
                   DisplayAll;
                   {PlacePODOnScreen; not yet}
                   end
                else begin
                   PopWindow;
                   exit;
                   end;
                end;
{$else}
          if (not (frontward or backward)) then begin
             if (code=make_table) then InsufficientDataMessage(tablestr, DP_InsufficientDataForTable);
             exit;
             end;
{$endif}
          if (frontward and backward) then MessageBox('Too many unknowns.', DP_TooManyUnknowns)
          else if (backward) then BackwardCalc;
          if (errorflag) then exit;
          if (backward) then begin
             stopat:=2;
             bf.Store;
             end
          else stopat:=1;
             {If we've done a backwards calc from line 1, don't now try
              to do a frontward calc from that line.}
          if (not pvlfancy) then begin
            for i:=nlines[PVLPresValBlock] downto stopat do FrontwardCalc(i);
              {Use downto so first line is what stays in memory afterward}
            if (stopat>1) then begin
              {First line not yet what's left on screen for Backward/Frontward calc.}
              backward:=false; {avoiding error message}
              ComputeLumpSumLineValues(1);
              ComputePeriodicLineValues(1);
              bf.Recall;
              end;
            end;
LIFE_CALC:
//          DisplayAll; {Must display all before making table, or Value doesn't
//                       show up when we get StringWithCodesFromScreen}
{$ifdef ACTU}
          if (code and with_life > 0) then begin
             if (podunk) then ComputeUnknownPOD;
             PlacePODOnScreen;
             MessageAndCheckForPrintScreen(copy(AnyKeyToReturn,1,34)+', <Ctrl-T>ùTable)...',not literal,kiword);
             if (lo(kiword)=ord(CTL_T)) then begin
                code:=code or make_table;
                goto LIFE_CALC;
                end;
{$ifdef TESTING}
             if (kiword=ord(CTL_W)) then ChooseAndSaveAnyFile(df.d.screenpath,ScreenExt(thisrun),SaveDataScreen);
{$endif}
             PopWindow;
             end;
{$endif}
          end; {Enter}

procedure MakeTable( Output: TStringList; bCommaSeperated: boolean );
var
  tdate:daterec;
  tperyr: byte;
begin
  Enter( make_table );
  if( errorflag ) then exit;
  if (not pvlfancy) and (c[1]^.status<contains_unknown) then begin
    InsufficientDataMessage(tablestr, DP_InsufficientDataForTable);
    exit;
  end;
  GetTableParams(b_^,nlines[PVLPeriodicBlock],tdate,tperyr);
//  if GetTableOptions(tdate,tperyr) then
    MakePVLTable(a_^,b_^,nlines[PVLLumpSumBLock],nlines[PVLPeriodicBLock], Output, bCommaSeperated );
end;

end.
