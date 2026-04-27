unit PVLXSCRN;
{$ifdef OVERLAYS} {$F+,O+} {$endif}

INTERFACE
//uses OPCRT,OPMOUSE,VIDEODAT,NORTHWND,PETYPES,PEDATA,INPUT,INTSUTIL,PEPANE,PVLUTIL;
uses VIDEODAT,PETYPES,PEDATA,INTSUTIL,PVLUTIL, Globals;

type
      readfunc=function(y :byte; erase :boolean):byte;
      writeproc=procedure(y,j :byte);

procedure FancyFirstPass;
function FancySummation(j :byte):real;
procedure InitializeColaData(var b :periodic; var scaledamt:real; var coladate:daterec);
procedure PVLPlainFancy;
//procedure PresValScreen(code :byte);
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

procedure UpdateAmountWithCola(var b :periodic; var scaledamt :real; var t,coladate :daterec);
          begin
          if (df.c.COLAmonth=CNT) or (b.cola=0) then
            scaledamt:=exxp(b.cola*YearsDif(t,b.fromdate))
          else begin
            if (DateComp(t,coladate)>=0) then begin
               scaledamt:=scaledamt*exxp(b.cola);
               inc(coladate.y);
               end;
            end;
          end;

procedure InitializeColaData(var b :periodic; var scaledamt:real; var coladate:daterec);
          begin
          scaledamt:=1;
          coladate:=b.fromdate;
          if (df.c.colamonth in [1..12]) then begin
             if (df.c.colamonth<=b.fromdate.m) then inc(coladate.y);
             coladate.m:=df.c.colamonth;
             coladate.d:=1;
             end
          else inc(coladate.y);
          end;

procedure SwapBytes(var x,y :byte);
          var z: byte;
          begin
          z:=x; x:=y; y:=z;
          end;

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

procedure DetermineStatus1(i :byte);
          begin with aa[i]^ do begin
          if (datestatus>=defp) and ((amt0status>=defp) or (val0status>=defp)) then status:=fully_specified
          else if (datestatus>=defp) then status:=contains_unknown
            {Backwards calculation of date is not supported on the X screen}
          else status:=EMPTY;
          end; end;

procedure DetermineStatus2(j :byte);
          var saveto              :daterec;
              ok4,ok5,ok6,ok7,ok9 :boolean;
          begin with bb[j]^ do begin
          ok4:=(fromdatestatus>=defp);
          ok5:=(todatestatus>=defp);
          ok6:=(peryrstatus>=defp);
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

procedure ReadSimple;
          begin
// James sez:
// No reason to read the screen to get the d^.simple.  When this function is called
// the UI layer has already filled in the correct value.          
//          d^.simple:=(screen^[succ(ztop[PVLXBlock]),startof[simplecol],1]<>'C');
          end;

function ValueOfPaymentSeries(date1,date2,asof :daterec; rate :real; simple :boolean):real;
         begin
         end;

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

function ValueOfFuturePayments(j :byte):real;
         var
           date0,date3,      {Beginning and end dates of each partial period to be computed}
           date1,date2,      {Start and stop dates of the periodic payments}
           coladate          {Date on which next cola escalation is due}
                            :daterec;
         begin with bb[j]^ do begin
         date0:=fromdate
         end;end;

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

procedure ComputeGrandTotal;
          var i,j :byte;
          begin
          d^.xvalue:=0;
          for i:=1 to nlines[PVLLumpSumBlock] do d^.xvalue:=d^.xvalue+aa[i]^.val0;
          for j:=1 to nlines[PVLPeriodicBlock] do d^.xvalue:=d^.xvalue+bb[j]^.valn;
          if (fold_in_life) then d^.xvalue:=d^.xvalue+podval;
          d^.xvaluestatus:=outp;
          end;

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

begin
PEDATA.PVLPlainFancy:=PVLPlainFancy;
end.
