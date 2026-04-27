unit PVLTABLE;

INTERFACE

//uses OPCRT,VIDEODAT,NORTHWND,INPUT,PETYPES,PEDATA,INTSUTIL,PEPANE,KCOMMAND,LOTUS,TABLE,IOUNIT
//{$ifdef ACTU} ,ACTUARY {$endif}
//{$ifdef PVLX} ,PVLUTIL,PVLXSCRN {$endif}
//;
uses PETYPES,PEDATA,INTSUTIL, Globals, Classes, videodat
{$ifdef PVLX} ,PVLUTIL,PVLXSCRN {$endif}
;

procedure MakePVLTable(var a:lumpsumarray; var b:periodicarray; na,nb :byte; OutputList: TStringList; bCommaSeperated: boolean );
procedure GetTableParams(var b :periodicarray; nb :byte; var tdate :daterec; var tperyr :byte);


IMPLEMENTATION

uses HelpSystemUnit;

const
        hdr:str80='         Date        Payment          Value      Cum Value';
  life_hdr1:str80='                                      Value    Actuarial';
  life_hdr2:str80='         Date        Payment        if paid  Probability         Value';
{
  life_hdr1:str80='                               Value    Actuarial';
  life_hdr2:str80='  Date        Payment        if paid  Probability         Value     Cum Value';
}
       extramargin='       ';
            indent=5;
       linelength=59;
  life_linelength=79;

type daterecarray=array[1..maxlines] of daterec;
     realarray=array[1..maxlines] of real;

var
       lineacross  :str80;
       colamult    :^realarray;
       coladate    :^daterecarray;

procedure GetTableParams(var b :periodicarray; nb :byte; var tdate :daterec; var tperyr :byte);
          var i :byte;
          begin
          if (nb=0) then begin
            tdate.m:=1; tdate.y:=1; tdate.d:=1; tperyr:=12;
            exit; end
          else begin
            tdate:=latest; tperyr:=1;
            for i:=1 to nb do begin
               if (b[i]^.fromdatestatus>EMPTY) and (DateComp(b[i]^.fromdate,tdate)<0) then tdate:=b[i]^.fromdate;
               if (ok(b[i]^.peryr)) and (b[i]^.peryr>tperyr) then tperyr:=b[i]^.peryr;
               end;
            end;
          if (DateComp(tdate,maxdate)=0) then tdate.y:=1;
          {if (nlines[PVLLumpSumBlock]>0) then tperyr:=12;{If there are unscheduled payments, then summary always makes sense.}
          end;

{$F+}
{$ifdef 0}
procedure NewPage;
{$ifndef OVERLAYS} {$F-} {$endif}
          begin
          if not (FormFeed and TopMargin) then abort:=true;
          if (fold_in_life) then begin
               OutputLine(extramargin+life_hdr1);
               OutputLine(extramargin+life_hdr2);
             end
          else OutputLine(extramargin+hdr);
          if not OutputLine(extramargin+lineacross) then abort:=true;
          end;
{$endif}

{$ifndef PVLX}
procedure InitializeColaData(var b :periodic; var colamult:real; var coladate:daterec);
          begin
          colamult:=1;
          coladate:=b.fromdate;
          if (df.c.colamonth in [1..12]) then begin
             if (df.c.colamonth<=b.fromdate.m) then inc(coladate.y);
             coladate.m:=df.c.colamonth;
             coladate.d:=1;
             end
          else inc(coladate.y);
          end;
{$endif}

procedure MakePVLTable(var a:lumpsumarray; var b:periodicarray; na,nb :byte; OutputList: TStringList; bCommaSeperated: boolean);
          const podsignal=-44;
          var t,oldt :daterec; {These are date of next payment and last payment printed.}
            q,v,ifpd :real; {Amount of next payment to be printed.}
         contingency :byte; {For life expectancy, whether living, dead, or not_contingent}
            cntg_let :char;
              nexta  :byte;
            nextdate :array[1..maxlines] of daterec;
                 {This keeps track of how far we are along in each periodic payment.}
       lumpsumn,periodicn :byte;
                 {The number of lines of each block that are being used.}
     subtotal,total  :record q,v,ifpd:real; end;
           prevdate  :daterec; {Used for determining whether it's time for summary.}
{
  procedure LineAcrossSpreadsheet;
            var i,max :byte;
            begin
            inc(lotusrow); lotuscol:=0;
            if (fold_in_life) then max:=5 else max:=3;
            for i:=1 to max do
               WriteOneStringCell(true,'\',lotusrow,lotuscol,'-');
            lotuscol:=0;
            end;
}
{$ifdef 0}
{$I-}
  function OpenFileAndPrintHeader:boolean;
     const tblmsg:str80=' Making table...                                                                ';
           var  result    :boolean;
                i         :byte;
                k2,ki     :char;
                kiword    :word absolute ki;

           begin
           OpenFileAndPrintHeader:=false;  result:=true;
           if (fold_in_life) then lineacross[0]:=char(life_linelength)
           else lineacross[0]:=char(linelength);
           if (df.p.ibmset) or (destin=none) then
              for i:=1 to length(lineacross) do lineacross[i]:='Ä' {Horiz line, #196}
           else
              for i:=1 to length(lineacross) do lineacross[i]:='-'; {Dash, #45}
           case destin of
             totext : begin
                      assign(fout,df.d.textpath+outfilename);
                      reset(fout); if (MIOresult=0) then begin
                        append(fout);
                        writeln(fout,crlf,#12);
                        end
                      else rewrite(fout);
                      result:=(MIOResult=0);
                      end;
             toprinter : begin
                      ki:=#0;
                      TestPrinter(kiword);
                      if (ki=#27) then result:=false
                      else begin
                        assign(fout,prnport); rewrite(fout);
                        result:=true;
                        end;
                      end;
             tolotus : begin
{$ifdef BUGSIN}
                       if (fold_in_life) then Scavenger('I-5');
{$endif}
                       FastWrite(tblmsg,25,1,MessageColors);
                       WK1File(df.d.lotuspath+outfilename);
                       if (abort) then {#begin Menu;} exit; {#end;} {Template not found}
                       reset(lotusfile,1); seek(lotusfile,filesize(lotusfile)-4);
                       inc(lotusrow,2); lotuscol:=0;
                       if (fold_in_life) then begin
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Date');
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Amount');
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Valu if Pd');
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Actu Prob');
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Value');
                          end
                      else begin
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Date');
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Amount');
                          WriteOneStringCell(true,'^',lotusrow,lotuscol,'Value');
                          end;
                       if (MIOResult>0) then begin abort:=true; {#Menu;} exit; end;
                       LineAcrossSpreadsheet; inc(lotusrow);
                       if (MIOResult>0) then begin abort:=true; {#Menu;} end;
                       end;
             none    : begin
                       textattr:=MainScreenColors;
                       PushWindowNoBorder(1,1,80,25);
                       end;
             end; {case destin}
           linesthispage:=0;
           if (result) then
             if (destin>tolotus) then begin
                PrintScreen('',false); {Includes header. Header includes top margin}
                  {'' is a signal for appending - no open/close, but 2 linefeeds at end.}
                result:=not errorflag;
                if result then
                  if (fold_in_life) then
                     result:= OutputLine(extramargin+life_hdr1) and OutputLine(extramargin+life_hdr2)
                   else
                     result:= OutputLine(extramargin+hdr);
                if (result) then result:=OutputLine(extramargin+lineacross);
                if (result) then
                   FastWrite(tblmsg,25,1,MessageColors);
                end
             else if (destin=none) then begin
                textattr:=MainScreenColors;
                if (fold_in_life) then begin
                  writeln(life_hdr1);
                  writeln(life_hdr2);
                  writeln(lineacross);
                  end
                else
                  writeln(hdr,crlf,lineacross);
                if (fold_in_life) then wy1[windex]:=4 else wy1[windex]:=3;
                RestoreWindow;
                end;
           OpenFileAndPrintHeader:=result;
           abort:=(not result);
           end;
{$endif}           
{$I+}

  procedure SortPayments;
            var i    :byte;
                p    :lumpsum;
                pp   :periodic;
                done :boolean;
                one  :real; {throw-away parameter to InitializeColaData}

      procedure CheckOut(i :byte);
                begin
                if (b[i]^.fromdatestatus>empty) and (b[i]^.todatestatus>empty) and (b[i]^.amtnstatus>empty) then begin
                   nextdate[i]:=b[i]^.fromdate;
                   periodicn:=i;
                   end;
{$ifdef BUGSIN}
                with b[i]^ do
                  if (df.c.colamonth<=12) and (peryr<12) and (fromdate.m mod peryr <> df.c.colamonth mod peryr)
                  then Scavenger('C-1');
{$endif}
                end;

            begin {SortPayments}
            lumpsumn:=1 and na;
            if (na>1) then repeat {bubble sort of single payments}
              done:=true;
              for i:=1 to pred(na) do begin
                if (DateComp(a[i]^.date,a[succ(i)]^.date)>0) then begin
                   p:=a[i]^; a[i]^:=a[succ(i)]^; a[succ(i)]^:=p; done:=false;
                   end;
                if (a[i]^.datestatus>empty) and (a[i]^.amt0status>empty) then lumpsumn:=i;
                end;
                if (a[na]^.datestatus>empty) and (a[na]^.amt0status>empty) then lumpsumn:=na;
                  {needed because the loop above only goes through 2d to bottom line.}
            until done;
            if (nb>0) then repeat {Deleting blank lines in periodic block.}
                periodicn:=0;
                done:=true;
                for i:=1 to pred(nb) do begin
                  if (b[succ(i)]^.fromdatestatus>empty) and (b[succ(i)]^.todatestatus>empty) and (b[succ(i)]^.amtnstatus>empty)
                                                and
                     (not ((b[i]^.fromdatestatus>empty) and (b[i]^.todatestatus>empty) and (b[i]^.amtnstatus>empty)))
                     then begin
                       pp:=b[i]^; b[i]^:=b[succ(i)]^; b[succ(i)]^:=pp; done:=false;
                       end;
                  CheckOut(i);
                  end;
                CheckOut(nb);
                  {needed because the loop above only goes through pred(nb)}
              until done
            else periodicn:=0;
            subtotal.q:=0; subtotal.v:=0; subtotal.ifpd:=0;
            total.q:=0; total.v:=0; total.ifpd:=0;
            nexta:=1; {Initialize next single payment index.}
            prevdate.m:=-88; {Signal to TimeIsRipe.}
            for i:=1 to periodicn do begin
               InitializeColaData(b[i]^,one,coladate^[i]);
               if (b[i]^.todate.y=latest.y) then b[i]^.todate.y:=b[i]^.fromdate.y+50;
                  {When you print a table that's supposed to go on forever, cut it off at 50 yrs}
               colamult^[i]:=1;
               end;
            end;

  function SelectNextPayment:boolean;
           var i          :byte;
               kiword     :word;
           begin
{           if (keypressed) and (readkey=#27) then
             if MessageYN('Abort output [Y/N] ? ',kiword) then begin
                abort:=true; SelectNextPayment:=false;
                end;
}
           oldt:=t;
           if (nexta<=lumpsumn) then t:=a[nexta]^.date
           else t.m:=unkbyte; {After everything}
           for i:=1 to periodicn do
               if (DateComp(t,nextdate[i])>0) then t:=nextdate[i];
           {Now we've got the date.  What's the amount?}
           if (abort) or (not dateok(t)) then begin
              SelectNextPayment:=false; exit;
              end
           else SelectNextPayment:=true;
           q:=0;
           while (DateComp(t,a[nexta]^.date)=0) do begin
              q:=q + a[nexta]^.amt0;
{$ifdef PRO}
              contingency:=a[nexta]^.act0;
              inc(nexta);
              if (fold_in_life) then exit;
                {Don't combine payments on the same date for life expectancy
                 calc, because they may have different contingencies.}
{$else}
              inc(nexta);
{$endif}
              end;
           for i:=1 to periodicn do
             if (DateComp(t,nextdate[i])=0) then begin
{$ifdef PVLX}
                UpdateAmountWithCola(b[i]^,colamult^[i],t,coladate^[i]);
                q:=q+b[i]^.amtn*colamult^[i];
                contingency:=b[i]^.actn;
{$else}
                if (df.c.COLAmonth=CNT) or (b[i]^.cola=0) then
                  q:=q+b[i]^.amtn*exxp(b[i]^.cola*YearsDif(t,b[i]^.fromdate))
                else begin
                  if (DateComp(t,coladate^[i])>=0) then begin
                    colamult^[i]:=colamult^[i]*exxp(b[i]^.cola);
                    inc(coladate^[i].y);
                    end;
                  q:=q+b[i]^.amtn*colamult^[i];
                  end;
{$endif}
                AddPeriod(nextdate[i],b[i]^.peryr,b[i]^.fromdate.d,add);
                if (DateComp(nextdate[i],b[i]^.todate)>0) then nextdate[i].m:=unkbyte;
{$ifdef PRO}
                if (fold_in_life) then exit;
                  {Don't combine payments on the same date for life expectancy
                   calc, because they may have different contingencies.}
{$endif}
                end;
           end;

  function TimeIsRipe(t :daterec):boolean;
           var tt       :daterec;
               monthset :set of byte;

           begin
           tt:=prevdate; monthset:=[];
           if (prevdate.m<0) then begin TimeIsRipe:=false; prevdate:=t; exit; end;
           while (tt.y<t.y) or (tt.m<t.m) do begin
              monthset:=monthset+[tt.m];
              AddPeriod(tt,12,tt.d,add);
              end;
           if ((monthset * cumset)<>[]) then begin
              TimeIsRipe:=true;
              prevdate:=t;
              end
           else TimeIsRipe:=false;
              {In other words:  is there intersection between the months we've
               passed through since the last payment and the cumset?}
           end;
{$ifdef 0}
  procedure GrandTotalsToLotus;
            var format :char;
            begin
            inc(lotusrow);
            LineAcrossSpreadsheet;
            if (df.h.commas) then format:=lotus_comma_fmt else format:=lotus_real_fmt;
            inc(lotusrow);
            lotuscol:=0;
            WriteOneStringCell(true,'''',lotusrow,lotuscol,'Grand total:');
            WriteOneCell(format,total.q);
            if (fold_in_life) then begin
               WriteOneCell(format,total.ifpd);
               if (abs(total.ifpd)>teeny) then
                 WriteOneCell(format,total.v/total.ifpd)
               else
                 inc(lotuscol);
               end;
            WriteOneCell(format,total.v);
            end;
{$endif}
  procedure GrandTotals( Output: TStringList; bCommaSeperated: boolean );
            var ws   :str80;
                prob :real;
            begin
//            CheckRoomOnPage(2,false,NewPage);
            if( not bCommaSeperated ) then begin
              if (destin=none) then begin {textattr:=MainScreenColors;}
                if( Output<>nil) then Output.Add( lineacross );
              end
//            else if (destin=tolotus) then GrandTotalsToLotus
              else begin
                if( Output<>nil ) then Output.Add(extramargin+lineacross);
              end;
            end;
{
            ws:='Grand Totals:';
            if (destin=none) then begin
//               textattr:=MainScreenColors;
              if( Output <> nil ) then Output.Add( ws );
              ws:='';
            end;
}
{$ifdef ACTU}
            if (fold_in_life) then begin
              if (abs(total.ifpd)>teeny) then prob:=total.v/total.ifpd else prob:=0;
              ws:=ws+ftoa2(total.q,15,df.h.commas)+' '+ftoa2(total.ifpd,14,df.h.commas)
                  +ftoa4(prob,13)+ftoa2(total.v,14,df.h.commas) {+ftoa2(total.v,14,df.h.commas)}
              end
            else
{$endif}
              if( bCommaSeperated ) then
                ws:='Grand Totals,'+ftoa2(total.q,14,df.h.commas)+','+ftoa2(total.v,14,df.h.commas)+','+ftoa2(total.v,14,df.h.commas)
              else
                ws:='Grand Totals: '+ftoa2(total.q,14,df.h.commas)+' '+ftoa2(total.v,14,df.h.commas)+' '+ftoa2(total.v,14,df.h.commas);
            if (destin=none) then begin
//               textattr:=GetColor(defp);
               if( Output <> nil ) then Output.Add( ws );
//               ob.StoreAndWrite(ws);
               end
            else if (destin=tolotus) then
            else begin
              if( Output <> nil ) then Output.Add(extramargin+ws);
            end;
            end;
{$ifdef 0}
  procedure SummaryLineToLotus;
            var format :char;
            begin
            LineAcrossSpreadsheet;
            if (df.h.commas) then format:=lotus_comma_fmt else format:=lotus_real_fmt;
            inc(lotusrow);
            if (cum in ['A'..'Z']) then WriteOneStringCell(true,'''',lotusrow,lotuscol,'Subtotal:')
            else WriteOneCell(lotus_date_fmt,Julian(oldt)+1);
            WriteOneCell(format,subtotal.q);
            if (fold_in_life) then begin
               WriteOneCell(format,subtotal.ifpd);
               if (abs(subtotal.ifpd)>teeny) then
                 WriteOneCell(format,subtotal.v/subtotal.ifpd)
               else
                 inc(lotuscol);
               end;
            WriteOneCell(format,subtotal.v);
            inc(lotusrow); {skip a line}
            end;

  procedure DetailLineToLotus;
            var format :char;
            begin
            if (df.h.commas) then format:=lotus_comma_fmt else format:=lotus_real_fmt;
            inc(lotusrow);
            lotuscol:=0;
            if (t.m=podsignal) then WriteOneStringCell(false,'"',lotusrow,lotuscol,'On death')
            else WriteOneCell(lotus_date_fmt,Julian(t)+1);
            WriteOneCell(format,q);
            if (fold_in_life) then begin
               WriteOneCell(format,ifpd);
               if (abs(ifpd)>teeny) then
                 WriteOneCell(format,v/ifpd)
               else
                 inc(lotuscol);
               end;
            WriteOneCell(format,v);
            end;
{$endif}
  procedure PrintSummary( Output:TStringList; bCommaSeperated: boolean );
            var ws   :str80;
                prob :real;
            begin
//            CheckRoomOnPage(3,false,NewPage);
            if (cum in ['A'..'Z']) then begin
              if (destin=none) then begin
//              textattr:=MainScreenColors;
                if( (Output<>nil) and (not bCommaSeperated) ) then Output.Add( lineacross );
//              ob.StoreAndWrite(lineacross);
              end else begin
                if( (Output<>nil) and (not bCommaSeperated) ) then Output.Add(extramargin+lineacross);
              end;
              ws:='   Subtotals: ';
              if (destin=none) then begin
//                  textattr:=MainScreenColors;
//                  if( Output<> nil ) then Output.Add( ws );
//                  ws:='';
              end;
            end else
              ws:=AllBlank(indent)+DateStr(oldt)+ ' ';
{$ifdef ACTU}
            if (fold_in_life) then begin
              if (abs(subtotal.ifpd)>teeny) then prob:=subtotal.v/subtotal.ifpd else prob:=0;
              ws:=ws+ftoa2(subtotal.q,15,df.h.commas)+' '+ftoa2(subtotal.ifpd,14,df.h.commas)
                    +ftoa4(prob,13)+ftoa2(subtotal.v,14,df.h.commas)
                    {+ftoa2(total.v,14,df.h.commas)} ;
              end
            else
{$endif}
              if( not bCommaSeperated ) then
                ws:=ws+ftoa2(subtotal.q,14,df.h.commas)+' '+ftoa2(subtotal.v,14,df.h.commas)+' '+ftoa2(total.v,14,df.h.commas)
              else
                ws:=ws+','+ftoa2(subtotal.q,14,df.h.commas)+','+ftoa2(subtotal.v,14,df.h.commas)+','+ftoa2(total.v,14,df.h.commas);
            if (destin=none) then begin
//              textattr:=GetColor(defp);
              if( Output<>nil ) then Output.Add( ws );
//              ob.StoreAndWrite(ws);
              end
//            else if (destin=tolotus) then SummaryLineToLotus
            else if( Output<>nil ) then Output.Add(extramargin+ws);
            if (cum in ['A'..'Z']) then begin
               if (destin=none) then begin
                 if( Output<>nil ) then Output.Add('');
               end
               else if (destin<>tolotus) then
//                 if (not OutputLineFeeds(1)) then abort:=true;
               end;
            subtotal.q:=0; subtotal.v:=0; subtotal.ifpd:=0;
            end;

  procedure PrintNextPayment( Output: TStringList; bCommaSeperated: boolean );
            var ws   :str80;
                prob :real;
            begin
//            CheckRoomOnPage(0,false,NewPage);
            if TimeIsRipe(t) then PrintSummary( Output, bCommaSeperated );
{$ifdef PVLX}
            if (pvlfancy) then v:=ValueOfOnePayment(q,t)
            else v:= q * exxp(-c[1]^.r.rate*YearsDif(t,c[1]^.asof));
{$ifdef ACTU}
            if (fold_in_life) then begin
               ifpd:=v;
               v:=ifpd*LifeProb(t,contingency);
               cntg_let:=actchar[contingency];
               end;
{$endif}
{$else}
            v:= q * exxp(-c[1]^.r.rate*YearsDif(t,c[1]^.asof));
{$endif}
            subtotal.q:=subtotal.q+q; total.q:=total.q+q;
            subtotal.v:=subtotal.v+v; total.v:=total.v+v;
            if (fold_in_life) then begin
              subtotal.ifpd:=subtotal.ifpd+ifpd; total.ifpd:=total.ifpd+ifpd;
              end;
            if (cum in [' ','A'..'Z']) then begin {detail line}
{$ifdef ACTU }
               if (fold_in_life) then begin
                  if (abs(ifpd)>teeny) then prob:=v/ifpd else prob:=0;
                  ws:=AllBlank(indent)+DateStr(t)+' '+ftoa2(q,14,df.h.commas)+' '
                      +ftoa2(ifpd,14,df.h.commas)+'   '+cntg_let
                      +ftoa4(prob,9)+ftoa2(v,14,df.h.commas);
                  ws:=AllBlank(indent)+DateStr(t)+' '+ftoa2(q,14,df.h.commas)+' '
                      +ftoa2(ifpd,14,df.h.commas)+'   '+cntg_let
                      +ftoa4(prob,9)+ftoa2(v,14,df.h.commas)
                      {+ftoa2(total.v,14,df.h.commas)} ;
                  end
               else
{$endif}
                  if( not bCommaSeperated ) then
                    ws:=AllBlank(indent)+DateStr(t) + ' '+ftoa2(q,14,df.h.commas)+' '+
                              ftoa2(v,14,df.h.commas)+' '+ftoa2(total.v,14,df.h.commas)
                  else
                    ws:=DateStr(t) + ','+ftoa2(q,14,df.h.commas)+','+
                              ftoa2(v,14,df.h.commas)+','+ftoa2(total.v,14,df.h.commas);
               if (destin=none) then begin
                 //textattr:=GetColor(defp);
                 if( Output<>nil ) then Output.Add( ws );
//                 ob.StoreAndWrite(ws);
               end
//               else if (destin=tolotus) then DetailLineToLotus
               else if( Output <> nil ) then Output.Add(extramargin+ws);
               end;
            end;

{$ifdef ACTU}
  procedure PrintPOD;
            var ws :str80;
            begin
            CheckRoomOnPage(0,false,NewPage);
            q:=pod;
            if (pvlfancy) then podval:=XPODValue else podval:=PODValue(c[1]^.asof,c[1]^.r.rate);
            ws:=AllBlank(indent)+ Ondeath   + ftoa2(q,15,df.h.commas)+' '
                 +ftoa2(podval,14,df.h.commas)+'  '+'POD'+ftoa4(1,8)+ftoa2(podval,14,df.h.commas);
            total.q:=total.q+q;
            total.v:=total.v+podval;
            total.ifpd:=total.v+podval;
            if (destin=none) then begin textattr:=GetColor(defp); ob.StoreAndWrite(ws); end
            else if (destin=tolotus) then begin t.m:=podsignal; ifpd:=podval; DetailLineToLotus; end
            else if (not OutputLine(extramargin+ws)) then abort:=true;
            end;
{$endif}
{$I-}
  procedure CloseUpShop();
            var ta         :byte;
                ws         :str80;
            begin
            if (not abort) then begin
//               if (linesthispage>0) and (destin=toprinter) then
//                  if not FormFeed then abort:=true;
//               if (destin=toprinter) then
//                 if (ParseCodeString(df.p.resetstring,ws)) then
//                   if (not OutputString(ws)) then abort:=true;
//               if (destin=tolotus) then begin
//                  blockwrite(lotusfile,coda,4);
//                  close(lotusfile);
//                  end;
//               ta:=MIOResult;
               end;
            if (destin=none) then begin
//               PopWindow;
//               ob.Free;
               end
            else begin
//               close(fout); ta:=ioresult;
                 {not MIOResult - if it's already failed, we know about it.}
               end;
//            ta:=MIOResult;
            {#Menu;}
            end;
{$I+}
          begin {MakePVLTable}
          lineacross := '----------------------------------------------------------';
          if not (GetMemIfAvailable(pointer(colamult),nlines[PVLPeriodicBlock]*sizeof(real))) then abort:=true;
          if (not abort) and (not (GetMemIfAvailable(pointer(coladate),nlines[PVLPeriodicBlock]*sizeof(daterec)))) then begin
             abort:=true;
             FreeMem(colamult,nlines[PVLPeriodicBlock]*sizeof(real));
             end;
          if (abort) then begin
            MessageBox('Insufficient memory to print table.', DP_NoMemoryForTable);
            exit; end;
          SortPayments;
//          if OpenFileAndPrintHeader then
             while (not abort) and (SelectNextPayment) do PrintNextPayment( OutputList, bCommaSeperated );
(*
          if abort then begin
             if (destin=toprinter) then if FormFeed then;
This results in Error 105's if printer is turned off and you abort.
             end
          else
*)
          if (not abort) then begin
             if (subtotal.q<>0) and (cum>' ') then PrintSummary( OutputList, bCommaSeperated );
{$ifdef ACTU}
             if (fold_in_life) and (pod<>0) then PrintPOD;
{$endif}
             GrandTotals( OutputList, bCommaSeperated );
//             if (destin=none) then RespondToTablePageCommand(TRUE);
             { else MessageAndCheckForPrintScreen(AnyKeyToReturn,literal,kiword);}
             end;
          CloseUpShop;
          FreeMem(colamult,nlines[PVLPeriodicBlock]*sizeof(real));
          FreeMem(coladate,nlines[PVLPeriodicBlock]*sizeof(daterec));
          end; {MakePVLTable}


end.

