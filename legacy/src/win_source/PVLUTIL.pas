unit PVLUTIL;
{$ifdef OVERLAYS} {$F+,O+} {$endif}
INTERFACE

uses VIDEODAT,Globals,PETYPES,PEDATA,INTSUTIL;

type bfrecord = object
      status        :^byte; {points to status byte for target cell of backward/frontward calc}
      line          :pointer;
      nbytes        :byte; {size of what is held in storage}
      storage       :^periodic; {actually, a buffer big enough to hold lumpsum or periodic line}
      procedure Store;
      procedure Recall;
      procedure FixPointers(block,i :byte);
      end;

var bf                       :bfrecord;

function ValueOfOnePayment(howmuch :real; when :daterec):real;

IMPLEMENTATION

uses HelpSystemUnit;

function ValueOfOnePayment(howmuch :real; when :daterec):real;
         var k                    :byte;
             theresult,years,rt      :real;
             date1,date2          :daterec;
         begin
         k:=0;
         theresult:=howmuch;
         case (DateComp(when,d^.xasof)) of
          { 0 : we're done; }
           -1 : begin
                repeat inc(k) until (DateComp(cc[k]^.date,when)>0) or (k>nlines[PVLPresValBlock]);
                dec(k);
                date1:=when;
                repeat
                  if (k=nlines[PVLPresValBlock]) or (DateComp(cc[succ(k)]^.date,d^.xasof)>0) then date2:=d^.xasof
                  else date2:=cc[succ(k)]^.date;
                  years:=YearsDif(date2,date1);
                  if (d^.simple) then theresult:=theresult+ howmuch*(cc[k]^.r.rate*years)
                  else theresult:=theresult * exxp(cc[k]^.r.rate*years);
                  date1:=date2;
                  inc(k);
                until (DateComp(date2,d^.xasof)=0);
                end;
            1 : begin
                repeat inc(k) until (DateComp(cc[k]^.date,d^.xasof)>0) or (k>nlines[PVLPresValBlock]);
                dec(k);
                date1:=d^.xasof;
                rt:=0;
                repeat
                  if (k=nlines[PVLPresValBlock]) or (DateComp(cc[succ(k)]^.date,when)>0) then date2:=when
                  else date2:=cc[succ(k)]^.date;
                  years:=YearsDif(date2,date1);
                  if (d^.simple) then rt:=rt + cc[k]^.r.rate*years
                  else theresult:=theresult * exxp(-cc[k]^.r.rate*years);
                  date1:=date2;
                  inc(k);
                until (DateComp(date2,when)=0);
                if (d^.simple) then theresult:=howmuch/(1+rt);
                end;
            end;
         ValueOfOnePayment:=theresult;
         end;

procedure bfrecord.FixPointers(block,i :byte);
          var col :byte;
          begin
          line:=blockdata[block]^[i];
          col:=pred(fcol[block]);
          repeat
             inc(col);
             status:=AdvancePointer(line,dataoffset[col]);
          until (status^<defp);
          case block of
              PVLLumpSumBlock : nbytes:=sizeof(lumpsum);
             PVLPeriodicBlock : nbytes:=sizeof(periodic);
                 end;
          end;

procedure bfrecord.Store;
          begin
          if (pvlfancy) then exit; {needed because FixPointers hasn't been called}
          if (storage=nil) then new(storage);
          if (storage=nil) then begin messageBox('Out of memory.', DP_OutOfMemory); errorflag:=true; exit; end;
          Move(line^,storage^,nbytes);
          status^:=defp;
          end;

procedure bfrecord.Recall;
          begin
          if (pvlfancy) then exit; {needed because FixPointers hasn't been called}
          Move(storage^,line^,nbytes);
          dispose(storage); storage:=nil;
          end;

begin
bf.storage:=nil;
end.