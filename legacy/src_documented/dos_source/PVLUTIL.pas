{ ==========================================================================
  PVLUTIL.pas

  PURPOSE / ROLE
    Low-level utility unit for the Present Value "X" (fancy / rate-table)
    screen. It provides two things:
      1. ValueOfOnePayment - the core time-value kernel that discounts (or
         accretes) a single payment between its own date and the global "as of"
         date (d^.xasof), walking the piecewise rate table (cc[]) so each
         sub-interval uses the rate in effect over that span. Supports both
         simple and compound interest.
      2. bfrecord - a small "backward/frontward" helper object that locates the
         one input cell to solve for, and can save/restore (Store/Recall) the
         raw bytes of that cell's line during a backward calculation.

  KEY TYPES
    bfrecord with fields: status (-> the target cell's status byte), line
    (-> the block line), nbytes (size of the saved line), storage (scratch
    buffer big enough for a lumpsum or periodic line).
    The global singleton `bf` is the shared instance.
  ========================================================================== }
unit PVLUTIL;
{$ifdef OVERLAYS} {$F+,O+} {$endif}
INTERFACE

uses VIDEODAT,Globals,PETYPES,PEDATA,INTSUTIL;

// bfrecord: tracks the single solve-for cell in a backward calc and can
// save/restore that line's bytes around the computation.
type bfrecord = object
      status        :^byte; {points to status byte for target cell of backward/frontward calc}
      line          :pointer;
      nbytes        :byte; {size of what is held in storage}
      storage       :^periodic; {actually, a buffer big enough to hold lumpsum or periodic line}
      // Save the target line's bytes and mark its status defp (default/present).
      procedure Store;
      // Restore the previously saved line bytes and free the scratch buffer.
      procedure Recall;
      // Locate the target cell (status ptr, line ptr, size) for block/line i.
      procedure FixPointers(block,i :byte);
      end;

var bf                       :bfrecord;   // shared backward/frontward solve helper

// Time-value of a single payment between its date and d^.xasof, applying the
// piecewise rate table (simple or compound).
{ Go port: internal/finance/presentvalue/variablerate.go: VRDiscountFactor (line 124) -- piecewise-rate time-value kernel; simple/compound + rate-schedule walk in vrPeriodicValue/integrateRateForward (lines 159/73) }
function ValueOfOnePayment(howmuch :real; when :daterec):real;

IMPLEMENTATION

uses HelpSystemUnit;

{ ValueOfOnePayment
  PURPOSE: compute the value, as of d^.xasof, of a single payment `howmuch`
           occurring on date `when`, using the piecewise interest-rate schedule
           held in the rate-table block cc[1..nlines[PVLPresValBlock]].
  PARAMS:  howmuch - the payment amount; when - the date it occurs.
  RETURNS: the accreted value when the payment precedes xasof, or the discounted
           (present) value when it follows xasof; howmuch itself if when=xasof.
  INTENT:  segments the span [when, xasof] (or [xasof, when]) at every rate-
           change date, and over each segment applies either simple interest
           (linear, rate*years) or compound interest (exxp(+/- rate*years)).
           d^.simple selects the method; YearsDif gives the year fraction.
  NOTE: case on DateComp(when,xasof): -1 = payment in the past (accrete forward
        to xasof, '+' rate); +1 = payment in the future (discount back, '-'
        rate); 0 = same date, value is just howmuch. For simple interest in the
        future case the total simple rate `rt` is accumulated then applied as
        howmuch/(1+rt). }
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
                {payment is in the past: accrete forward from `when` to xasof}
                {find the rate-table row in effect at `when`}
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
                {payment is in the future: discount back from `when` to xasof}
                {find the rate-table row in effect at xasof}
                repeat inc(k) until (DateComp(cc[k]^.date,d^.xasof)>0) or (k>nlines[PVLPresValBlock]);
                dec(k);
                date1:=d^.xasof;
                rt:=0;                                   {accumulated simple-interest rate*time}
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

{ bfrecord.FixPointers
  PURPOSE: locate the single "unknown" cell to solve for within a block line,
           and record its address and the line's size.
  PARAMS:  block - PVLLumpSumBlock or PVLPeriodicBlock; i - line index.
  SIDE EFFECTS: sets this.line (the line record), this.status (-> the first
           column whose status is < defp, i.e. the blank/solve-for field), and
           this.nbytes (size of a lumpsum or periodic line).
  NOTE: scans columns from fcol[block] using dataoffset[] to reach each status
        byte via AdvancePointer until it finds one below defp. }
{ Go port: internal/finance/presentvalue/backward.go: FirstPass (line 163) -- locating the single solve-for cell is handled inline by FirstPass' status classification; no separate bf pointer object in Go }
procedure bfrecord.FixPointers(block,i :byte);
          var col :byte;
          begin
          line:=blockdata[block]^[i];
          col:=pred(fcol[block]);
          repeat
             inc(col);
             status:=AdvancePointer(line,dataoffset[col]);
          until (status^<defp);                          {stop at the first unspecified column}
          case block of
              PVLLumpSumBlock : nbytes:=sizeof(lumpsum);
             PVLPeriodicBlock : nbytes:=sizeof(periodic);
                 end;
          end;

{ bfrecord.Store
  PURPOSE: snapshot the target line's bytes before a backward solve, and mark
           the solve-for cell as defp (so the forward pass will fill it).
  SIDE EFFECTS: allocates `storage` if needed; copies nbytes from line^ into it;
           sets status^ := defp. On allocation failure shows an out-of-memory
           message and sets errorflag.
  NOTE: no-op in pvlfancy mode (FixPointers is not used there). }
{ Go port: n/a -- pointer save/restore is unnecessary in Go; backward.go copies PVInput values by struct rather than patching a shared buffer }
procedure bfrecord.Store;
          begin
          if (pvlfancy) then exit; {needed because FixPointers hasn't been called}
          if (storage=nil) then new(storage);
          if (storage=nil) then begin messageBox('Out of memory.', DP_OutOfMemory); errorflag:=true; exit; end;
          Move(line^,storage^,nbytes);
          status^:=defp;
          end;

{ bfrecord.Recall
  PURPOSE: restore the line bytes saved by Store, undoing the temporary defp
           edit once the backward calculation has read its result.
  SIDE EFFECTS: copies nbytes from storage^ back into line^; disposes storage
           and nils it.
  NOTE: no-op in pvlfancy mode. }
{ Go port: n/a -- see bfrecord.Store; Go passes PVInput by value so no line-byte restore is needed }
procedure bfrecord.Recall;
          begin
          if (pvlfancy) then exit; {needed because FixPointers hasn't been called}
          Move(storage^,line^,nbytes);
          dispose(storage); storage:=nil;
          end;

{ Unit initialization: ensure the scratch buffer starts unallocated. }
begin
bf.storage:=nil;
end.