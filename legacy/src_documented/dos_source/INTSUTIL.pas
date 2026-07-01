{ ============================================================================
  UNIT INTSUTIL  ("Interest / Utility")
  ----------------------------------------------------------------------------
  Core numeric, date, and interest-math service unit for Per%Sense.  This is
  the low-level engine that every financial screen (Mortgage, Amortization,
  Present Value, Compound-Interest, etc.) calls into.  Roughly four families
  of routines live here:

    1. Interest math
         - exxp / lnn       : exp() and ln() wrappers that work around a
                              Turbo Pascal compiler accuracy bug near x=1 and
                              that fail gracefully on overflow/underflow.
         - Power            : x^n implemented as exp(n*ln(x)).
         - YieldFromRate /  : continuous (true) rate <-> periodic-compounded
           RateFromYield      yield conversion.  The whole program stores rates
                              internally as CONTINUOUS rates and converts to a
                              periodic "yield" (APR-style) only for display and
                              input.  RealPerYr maps the peryr code (12, 52,
                              canadian, daily, ...) to a real periods-per-year.
         - ReportedRate /   : compose the two conversions to translate a rate
           InterpretedRate    between the user's compounding setting and the
                              header's compounding (inverse of each other).
         - Round2           : ROUND-HALF-DOWN money rounding (truncate at .5),
                              NOT banker's rounding; see note at the proc.
         - sqrrt / QuadraticFormula : safe sqrt and one root of a quadratic.

    2. Date utilities (operate on daterec = (d,m,y))
         - AddDays / AddYears / AddPeriod / AddNPeriods : advance a date by
                              days, fractional years, one payment period, or n
                              periods, honoring the peryr payment frequency and
                              the 30/360 vs actual-day basis.
         - Julian / MDY (from peData) are the serial<->calendar primitives.
         - ExtendedJulian   : Julian, but a synthetic y*360+m*30+d serial when
                              in 30/360 basis.
         - YearsDif         : elapsed time in years between two dates, with the
                              several day-count conventions (30/360, 365.25,
                              365/366 loan rule) selected by basis and screen.
         - DateComp / dateok / DaysCloseEnough / LastDayFn / NumberOfInstallments
                              : comparison, validity, month-end handling, and
                              counting whole payments between two dates.

    3. Settings / status helpers
         - SetYrDays, RealPerYr, PerYrString, TimesPerYearCases, UpdateSettings,
           ReInterpretRateTable, EmptyScreen, ok, InBounds, etc.

    4. Screen / UI plumbing (mostly $ifdef 0-disabled in this port, since the
       web port supplies its own UI) and a few message helpers.

  KEY TYPES (declared elsewhere, used throughout):
    daterec  - record of d,m,y (see peTypes); m=unkbyte/errorbyte flag bad/blank
    inout    - cell status enum (empty, inp, outp, defp, fromrate, ...)
    str12/str80 - short string types
    Sentinel reals: unk, blank, error, teeny, tiny, small, half - "magic"
                    values used to mark unknown/blank/error cells and to set
                    numeric tolerances.

  NOTE: Large stretches of the original UI code are preserved but disabled via
  $ifdef 0 ... $endif or comment brackets; they document the DOS screen
  behavior but are not compiled in this port.
  ============================================================================ }
unit INTSUTIL;
{ CHEAP build implies the cut-down PLANNER variant (disables UpdateSettings). }
{$ifdef CHEAP} {$define PLANNER} {$endif}
INTERFACE
//uses OPCRT,DOS,NORTHWND,INPUT,VIDEODAT,LOTUS,PETYPES,PEDATA
{$ifdef TESTING} ,TESTLOW {$endif}
//;
uses Globals, peTypes, peData, VIDEODAT;

type

     { Simple stateful counter object; Incr post-increments and returns count. }
     counter=object
         screencount :shortint;
         function Incr:shortint;
         end;

const sc       :counter=(screencount:0);
      tablestr :str12='table';

{ Advance date f forward (or back) by the given number of calendar days. }
procedure AddDays(var f :daterec; days :longint);
{ Advance date f by one payment period for frequency peryr (negative=backward). }
procedure AddPeriod(var f :daterec; peryr :byte; orig_day:byte; negative :boolean);
{ Set lastdate = firstdate advanced by n payment periods at frequency peryr. }
procedure AddNPeriods(var firstdate,lastdate :daterec; peryr :byte; n :integer);
{ Advance date a by a possibly-fractional number of years yrs. }
procedure AddYears(var a:daterec; yrs:real);
{procedure BackSpace; moved to COMDUTIL}
//procedure Beep;
{procedure ChooseTimesPerYear(ki :char); moved to PEPANE}
//procedure ClearCell(line,col :byte);
{procedure ClearComplementaryColumn(y,col:byte); Now in pePane}
//procedure Del(col :byte);
{procedure DiskError(name :str80; errno :byte);}
{procedure ExitExample; in pePane}
//procedure GetDate(line,col:byte; var f :daterec; var ok :boolean);
{procedure HighlightChoices;}
//procedure Home(b :byte);
{procedure Left; moved to COMDUTIL}
{ Recompute each block's display line count from its top/bottom screen rows. }
procedure LineCountsFromBBot;
//{$ifndef TOPMENUS} procedure Menu; {$endif}
{ Show "Insufficient data ... no <noun> can be calculated" message box. }
procedure InsufficientDataMessage(noun :str12; HelpCode: integer);
{procedure Print(line,col:byte; color:inout; x:real); in pePane}
//procedure RealToScreen(line,col:byte; color:inout; x:real); {old form of Print}
{procedure PrintDate(line,col :byte; color:inout; date:daterec); in pePane}
{ Flag an input error at a screen cell (sets errorflag; UI highlight stripped). }
procedure RecordError(line,errorcol :byte);
//procedure ReInterpretDatesAndRatesAccordingToNewSettings;
{ Re-store the PVL rate table when toggling between SIMPLE and COMPOUND mode. }
procedure ReInterpretRateTable; {toggle betw SIMPLE and COMPOUND}
//procedure RequestPatience;
{procedure Right; moved to COMDUTIL}
{ Round x to 2 decimals (cents) using round-half-DOWN (truncate at .5). }
procedure Round2(var x :real);
{procedure Save(ext :ch3);}
{procedure SelectCanadian;}
{ Set global yrdays/yrinv (days per year, 365.25 or 360) from the basis. }
procedure SetYrDays;
{procedure TabLt; moved to COMDUTIL}
{procedure TabRt; moved to COMDUTIL}
//procedure TempMessage(ws :str80);
{procedure TestDateFunctions;}
{ Map a keystroke to a payments-per-year value (cycles 1/12, 2/52, etc.). }
procedure TimesPerYearCases(var val :byte; ki :char);
{ Show the "time period too long" error message and set errorflag. }
procedure TimeTooLong;
//procedure ToggleStr(y,col :byte; msg1,msg2 :str8);
{ Redraw the current screen via the registered ScreenProc (fancy-aware). }
procedure UniversalScreenProc;
{ Redraw the on-screen settings banner (basis, peryr, COLA, ...); UI-only. }
procedure UpdateSettings;
//procedure WK1File(filename :str80);
{procedure WriteOneBlockToLotus(block,first_lotus_row,first_lotus_col :byte);}

//function BlankLine(block,y :byte):boolean;
//function BoxColors:byte;
//function Bright(color :byte):byte;
//function Dim(color :byte):byte;
{ Compare two dates: +1 if f1>f2, -1 if f1<f2, 0 if equal (bad dates sort last). }
function DateComp(f1,f2 :daterec):shortint;
{ True if f is a valid (non-blank, non-error) date. }
function dateok(f :daterec):boolean;
{ True if the two dates land on the same day-of-month (so 30/360 needs no day count). }
function DaysCloseEnough(date1,date2 :daterec; peryr :byte):boolean;
//function EditHeaderColors:byte;
{ True if screen 'which' has no data in any of its blocks. }
function EmptyScreen(which :byte):boolean;
//function EscapePressedInMessage(s :str80):boolean;
{ Serial day number for x: true Julian, or y*360+m*30+d in 30/360 basis. }
function ExtendedJulian(x :daterec):longint;
//function EntryBoxColors:byte;
{ Safe exp(x): guards overflow (>70) / underflow (<-70); Taylor series near 0. }
function exxp(x :real):real;
{ Return the screen block number that owns column col. }
function FindBlock(col :byte):byte;
{function fix(arg:real):real; {Largest integer less than}
{ function FullName(ext :ch3):str8;}
//function GetByte(line,col:byte):byte;
//function GetColor(z :inout):byte;
{ Allocate siz bytes into p; return false (and message) if out of memory. }
function GetMemIfAvailable(var p :pointer; siz :word):boolean;
//function GetJulian(line,col:byte; var ok :boolean):longint;
//function GetNumber(line,col:byte; var ok :boolean):real;
//function GetString(line,col:byte):string;
//function HelpEmphColors:byte;
//function HelpColors:byte;
{ Validate/normalize an input value for its column (e.g. % -> fraction); flag if out of range. }
function InBounds(var x :real; line,col :byte):boolean;
{ Convert a displayed rate back to internal compounding (inverse of ReportedRate). }
function InterpretedRate(inputrate :real): real; {The inverse of ReportedRate}
{ True if date f is the last payment day of its month for frequency peryr. }
function LastDayFn(f:daterec; peryr :byte):boolean;
{ function LastPaymentBefore(upperlim,anniv :longint; peryr:byte):longint;}
{ function LineNumberFromLetter(y,block :byte):byte; }
{ Safe ln(x): errors on x<=0; Taylor series near x=1 to dodge a Turbo ln() bug. }
function lnn(x :real):real;
{ function min(a,b :real):real;}
//function MainScreenColors:byte;
//function MenuBarHighlightColors:byte;
//function MenuLineColors:byte;
{ Count whole payments between f and l (also snaps l onto a payment day per z). }
function NumberOfInstallments(var f,l :daterec; peryr:byte; z:upto):integer;
{ True if v is a real value (not the unk/blank/error sentinels). }
function ok(v :real):boolean;
{ Read a cell's stored percentage value (rate/yield/etc.), converting per column & settings. }
function PercentValueFromCell (block, i, col: shortint; var io: inout): real;
{ Human-readable name of a payment frequency ("monthly", "weekly", ...). }
function PerYrString(peryr :byte; capitalization :byte):str80;
{ Compute x^n (real exponent) as exp(n*ln(x)); returns 0 for x<=0. }
function Power(x,n :real):real;
{ function QuadraticInterpolation(x1,y1,x2,y2,x3,y3 :real):real; }
{function RecallScreen(which:byte):boolean; in pePane}
{ Convert an internal rate to the rate as displayed under the header's compounding. }
function ReportedRate(apr :real):real;
{ Return the "(-B - sqrt(B^2-4AC))/2A" root of A*x^2+B*x+C (uses safe sqrrt). }
function QuadraticFormula(A,B,C :real):real;
{ Convert a periodic-compounded yield yy to the internal continuous rate (n=peryr). }
function RateFromYield(yy :real; n:byte):real;
{ Real number of compounding periods per year for peryr code n (handles daily/weekly/canadian). }
function RealPerYr(n :byte):real;
{ True if the cell at line,col is currently shown in reverse video. }
function RevVideo(line,col:byte):boolean;
{ function RoundDownAndSubtract(var date:longint; var yrs :real; peryr:byte):longint;}
{$ifdef BUGSIN} procedure Scavenger(which :str12); {$endif}
{ Safe sqrt: returns 0 for negatives (errors only if meaningfully negative). }
function sqrrt(x :real):real;
{ function StringFromWindow(line,start,length :byte):str80; }
{ function SwapScreen(which :byte):boolean; in pePane}
{ Format the program version (e.g. "1.23") plus optional internal/coproc tags. }
function VersionString:str12;
{ Elapsed years from a to z, honoring 30/360 / 365.25 / 365-366 conventions. }
function YearsDif(z,a :daterec):real;
{ Convert the internal continuous rate rr to a periodic-compounded yield (n=peryr). }
function YieldFromRate(rr :real; n:byte):real;
{ function YPos(block,i :byte):byte;}

IMPLEMENTATION

uses HelpSystemUnit;
{
procedure Beep;
          begin
          sound(220); delay(100); nosound;
          end;
 }
{$ifdef BUGSIN}
const A=440;                   triplet=200;
      E1=A*3 div 4;            hnote=3*triplet;
      Cs=A*5 div 4;            sixlet=triplet div 2;
      E2=E1*2;

procedure Note(freq, duration :integer);
	  begin
	  sound(freq); delay(duration-20);
	  nosound; delay(20);
	  end;
	
procedure Trumpet;
	  begin
          Note(E1,triplet); Note(A,triplet); Note(Cs,triplet);
          Note(E2,triplet); Note(E2,sixlet); Note(E2,sixlet);  Note(E2,triplet);
          Note(Cs,triplet); Note(Cs,sixlet); Note(Cs,sixlet); Note(Cs,triplet);
          Note(A,triplet); Note(Cs,triplet); Note(A,triplet);
	  Note(E1,hnote);
          Note(E1,triplet); Note(A,triplet); Note(Cs,triplet);
          Note(E2,triplet); Note(E2,sixlet); Note(E2,sixlet); Note(E2,triplet);
          Note(E2,triplet); Note(Cs,triplet); Note(A,triplet);
          Note(E1,triplet); Note(E1,sixlet); Note(E1,sixlet); Note(E1,triplet);
          Note(A,hnote);
	  end;

procedure Scavenger(which :str12);
	  begin
          Trumpet;
          Message('Good work!  You''ve hit on "Planted" bug '+which+'.  Please tell Josh.');
          end;
{$endif}

{ VersionString
  Purpose : Build the displayable program version string.
  Returns : str12 like "1.23" (from the global 'version' word: hi byte = major,
            lo byte = minor*10+patch), optionally suffixed with an internal
            build marker ("<chr>NN") and, under $ifdef COPROC, "-copr".
  Notes   : char(48+digit) is just digit-to-ASCII; '+48' converts 0..9 to '0'..'9'. }
{ Go port: n/a -- version-banner string is a DOS/UI presentation detail with no
  financial-engine equivalent in the Go port. }
function VersionString:str12;
         var internalstr:string[3];
         begin
         if (internalvx=0) then internalstr:=''
         else begin
            str(internalvx,internalstr);
            internalstr:='á'+internalstr;
            end;
         versionstring:= char(48+hi(version))+ '.' + char((lo(version) div 10)+48);  
         versionstring:= versionstring + char((lo(version) mod 10)+48);
         versionstring:= versionstring + internalstr
{$ifdef COPROC}         + '-copr'                             {$endif};
         end;

{ GetMemIfAvailable
  Purpose : Allocate siz bytes of heap and return them via var p.
  Params  : p   - (out) receives the new block (nil on failure)
            siz - number of bytes requested
  Returns : true on success; false (and an "Out of memory" message, errorflag
            set) if not enough heap is free.
  Two implementations follow: a BUGSIN debug build that checks MaxAvail and
  plants a diagnostic, and the normal port version below it. }
{ Go port: n/a -- manual heap allocation with out-of-memory guard is a DOS
  memory-management concern; the Go port relies on the garbage collector. }
{$ifdef BUGSIN}
function GetMemIfAvailable(var p :pointer; siz :word):boolean;
          var surrogatep   :longint absolute p;
              surrogatemor :longint absolute mor;
          begin
          if (siz=sizeof(pointer)) and (surrogatep<>surrogatemor) then
             Message('Suspicious call to GetMemIfAvailable.');
          if (MaxAvail<siz) then begin
            GetMemIfAvailable:=false;
            p:=nil;
            errorflag:=true;
             { Scavenger('I-7'); It's done its job. }
            Message('Out of memory. ');
            end
          else begin
            GetMemIfAvailable:=true;
            GetMem(p,siz);
            end;
          end;
{$else}
// A slight re-write of this function.
function GetMemIfAvailable(var p :pointer; siz :word):boolean;
          begin
            GetMemIfAvailable := true;
            if( siz = 0 ) then begin
              exit;
            end;
            GetMemIfAvailable:=true;
            GetMem(p,siz);
            if( p = nil ) then begin
              GetMemIfAvailable:=false;
              MessageBox('Out of memory. ', DO_OutOfMemory);
            end;
          end;
{$endif}

{ Power
  Purpose : Real-exponent power x^n.
  Params  : x - base, n - exponent (may be fractional/negative)
  Returns : exp(n*ln(x)) via the safe exxp/lnn wrappers; returns 0 when x<=0
            (ln undefined there), which the callers treat as a degenerate case. }
{ Go port: internal/finance/interest/math.go: Power (line 100) -- x^n via
  exp(n*ln(x)) using the safe Exxp/Lnn wrappers, same x<=0 -> 0 guard. }
function Power(x,n :real):real;
         begin
         if (x<=0) then power:=0
         else power:=exxp(n*lnn(x));
         end;

{ Round2
  Purpose : Round a monetary value to 2 decimal places (cents), IN PLACE.
  Param   : x - (var) value to round
  Rounding: ROUND-HALF-DOWN, i.e. exactly .xx5 truncates downward rather than
            up (and not banker's rounding).  Implemented by adding/subtracting
            (0.005 - teeny) toward zero's complement before truncating: the
            'teeny' bias means a value sitting exactly on the half-cent does not
            get pushed over the rounding boundary, so .5 truncates.  This MUST be
            reproduced bit-for-bit by the Go port (see docs/discrepancies.md). }
{ Go port: internal/finance/interest/math.go: Round2 (line 134) -- returns the
  rounded value (not in-place); same (0.005 - Teeny) round-half-DOWN bias. }
procedure Round2(var x :real);
          const halfpenny : real=0.005 - teeny;
          begin
          if (x>0) then x:=x+halfpenny       { bias away from zero, then... }
          else x:=x-halfpenny;
          x:=Trunc(x*100)/100;               { ...truncate to 2 decimals }
          end;
{
function EntryBoxColors:byte;
         begin
         if (color) then EntryBoxColors:=red shl 4 + white
         else EntryBoxColors:=white;
         end;

function MainScreenColors:byte;
         begin
         if (color) then MainScreenColors:=blue shl 4 + lightcyan else MainScreenColors:=7;
         end;

function EditHeaderColors:byte;
         begin
         if (color) then EditHeaderColors:=blue shl 4 + yellow
         else EditHeaderColors:=112;
         end;

function MenuLineColors:byte;
         begin
         if (color) then MenuLineColors:=yellow + lightgray shl 4
         else MenuLineColors:=112;
         end;

function MenuBarHighlightColors:byte;
         begin
         if (color) then MenuBarHighlightColors:=cyan shl 4
         else MenuBarHighlightColors:=white;
         end;
}
{$ifndef TOPMENUS} {look for this procedure in PEPANE}
{
procedure Menu;
          begin
          FastWrite(menuptr^,25,1,MenuLineColors);
          SetCursorSize; {because this sometimes comes after RequestPatience
          end;
}
{$endif}
{
function HelpColors:byte;
         begin
         if (color) then HelpColors:=red shl 4 + lightgray
         else HelpColors:=112;
         end;

function HelpEmphColors:byte;
         begin
         if (color) then HelpEmphColors:=red shl 4 + white
         else HelpEmphColors:=white;
         end;

function BoxColors:byte;
         begin
         if (color) then BoxColors:=cyan shl 4 else BoxColors:=112;
         end;
}
{ SelectCanadian
  Purpose : Refresh the on-screen "Canadian / Daily / Loan" basis label for the
            current screen.  In this port the body is essentially a no-op: the
            label-drawing case statement is commented out (screen output only),
            leaving just the guard that bails when the target area is covered.
  Side    : none material to financial logic (UI presentation only). }
{ Go port: n/a -- draws a text-mode basis label; pure DOS screen paint. }
procedure SelectCanadian;
          var xpvl,rblock :byte;
          begin
          rblock:=Fblock(thisrun);
          if (thisrun=iPVL) then rblock:=PVLPresValBlock
          else if (thisrun=iAMZ) then rblock:=AMZTopBlock;
          if (screen^[ztop[rblock],lleft[rblock]+rright[rblock] div 2,1]<#127) then exit;
             {This means there's something covering up the area}
          if (pvlfancy) then xpvl:=21 else xpvl:=32;
{
Looks like the following is screen output only, so I can safely comment it out
          case thisrun of
          ipvl : if (screen^[ztop[3]-2,xpvl,2]<#64) and (screen^[ztop[3],lleft[3],1]>#127) then begin
                   //In example mode this area is sometimes covered up;  sometimes you've just called ^O from a PVL table.
                   if (df.c.peryr and canadian >0) then FastWrite('Canadian',ztop[3]-2,xpvl,MainScreenColors)
                   else if (df.c.peryr and daily >0) then FastWrite(' Daily  ',ztop[3]-2,xpvl,MainScreenColors)
                   else FastWrite(' Loan   ',ztop[3]-2,xpvl,MainScreenColors);
                   end;
          ichr : if (df.c.peryr and canadian >0) then FastWrite('Can''dn',3,32,MainScreenColors)
                 else if (df.c.peryr and daily >0) then FastWrite('Daily ',3,32,MainScreenColors)
                 else FastWrite('Loan  ',3,32,MainScreenColors);
{ ifdef V_3
          irbt : if (df.c.peryr and canadian >0) then FastWrite('Can''dn',4,42,MainScreenColors)
                 else if (df.c.peryr and daily >0) then FastWrite(' Daily',4,42,MainScreenColors)
                 else FastWrite('  Loan',4,42,MainScreenColors);
  endif
          imtg : if (df.c.peryr and canadian >0) then FastWrite('Can''dn',3,44,MainScreenColors)
                 else if (df.c.peryr and daily >0) then FastWrite(' Daily',3,44,MainScreenColors)
                 else FastWrite(' Loan ',3,44,MainScreenColors);
          iamz : if (df.c.peryr and canadian >0) then FastWrite('Canadian',3,23,MainScreenColors)
                 else if (df.c.peryr and daily >0) then FastWrite('  Daily ',3,23,MainScreenColors)
                 else FastWrite('  Loan  ',3,23,MainScreenColors);
             end;}
          end;

{ LineCountsFromBBot
  Purpose : For every screen block, set linecount := (bottom row - top row),
            i.e. the number of usable data rows between the block's borders.
  Side    : writes the global linecount[] array. }
{ Go port: n/a -- derives text-screen row counts from block borders; UI layout. }
procedure LineCountsFromBBot;
          var bx :byte;
          begin
          for bx:=1 to nblocks do linecount[bx]:=bbot[bx]-ztop[bx];
          end;
{
procedure SetYrDays;
          begin
 Changed 3/94, somewhat blindly.
 The new idea is that 365/360 calculations are performed by changing
 YearsDif only, and using 360 for denominator, even though day count
 is Julian difference.
          if (df.c.basis=x360) then yrdays:=360
          else if (df.lastrun=iamz) or (df.c.basis=x365_360) then yrdays:=365
          else yrdays:=365.25;
          yrinv:=1/yrdays;
          end;
}
{ SetYrDays (revised 3/94)
  Purpose : Set the global day-count basis used by interest/time math.
  Effect  : yrdays := 365.25 when the settings basis is x365 (actual/365-style),
            otherwise 360 (covers x360 and the 365/360 family, whose 365
            numerator is applied inside YearsDif instead).  yrinv := 1/yrdays.
  Note    : The older, more elaborate version is preserved in the comment block
            above for historical reference. }
{ Go port: internal/finance/interest/rates.go: NewCalcContext (line 21) -- the
  same "x365 -> 365.25 else 360, yrinv=1/yrdays" logic is folded into the
  CalcContext constructor (there is no free-standing SetYrDays in Go). }
procedure SetYrDays; {3/94}
          begin
          if (df.c.basis=x365) then yrdays:=365.25
          else yrdays:=360;
          yrinv:=1/yrdays;
          end;

{ UpdateSettings
  Purpose : Repaint the one-line "settings banner" on the active screen,
            showing the COLA month, day-count basis (360 / 365 / 365/360),
            USA-rule vs actuarial, Rule-of-78, in-advance vs arrears,
            prepaid flag, century divider, exact flag, and payments-per-year.
  Under $ifdef PLANNER (the cut-down CHEAP build) it is a no-op stub.
  The full body is itself wrapped in $ifdef 0 in this port because it does
  heavy direct screen I/O; it is retained as the authoritative description of
  which settings the DOS banner displayed and in what order.  No financial
  computation happens here. }
{ Go port: n/a -- repaints the on-screen settings banner; pure DOS screen I/O. }
{$ifdef PLANNER}
procedure UpdateSettings; begin end;
{$else}
procedure UpdateSettings;
{$ifdef TOPMENUS}
          const startx=14; y=25; filler=' ';
{$else}
          const startx=25; y=1; filler='Ä';
{$endif}
          var saveattr,xgo,ygo   :byte;
              s                  :str80;
          begin
{$ifdef 0}
removing this because it does lots of screen stuff.  However it does need to
come back in some form.

          if (df.lastrun=0) or (scripting) then exit;
          AbsXY(xgo,ygo);
          SelectCanadian;
          SetYrDays;
          saveattr:=textattr;
{$ifdef TOPMENUS}
          textattr:=112;
          if (df.lastrun=iMTG) then s:=allblank(50)
          else s:='  Settings:  '+allblank(43);
          FastWrite(s,y,startx-13,112);
{$else}
          textattr:=GetColor(outp);
          s[0]:=char(58-startx);
          FillChar(s[1],ord(s[0]),filler);
          FastWrite(s,y,startx,MainScreenColors);
{$endif}
          window(1,1,80,25);
          gotoxy(startx,y);
          if (df.lastrun=iPVL) then begin
             write('COLA:');
             case df.c.colamonth of 1..12 : write(df.c.colamonth);
                                    ANN   : write('Ann');
                                    CNT   : write('Cnt');
                                    end;
             gotoxy(succ(wherex),y);
             end;
          if (df.lastrun <> iMTG) then begin
            case (df.c.basis) of
                  x360 : write('360');
              x365_360 : write('365/360')
                  else write('365');
                  end;
            gotoxy(succ(wherex),y);
            end;
{ ifdef V_3 USA is an option in RBT now
          if (df.lastrun=iRBT) then begin
            if (df.c.USARule) then write('USA') else write('Act');
            gotoxy(succ(wherex),y);
            end;
  endif}
          if (df.lastrun=iAMZ) then begin
            if (fancy) then begin
              if (df.c.USARule) then write('USA') else write('Act');
              gotoxy(succ(wherex),y);
              end
            else if (df.c.R78) then begin
              write('R78');
              gotoxy(succ(wherex),y);
              end;
            end;
          if (df.lastrun=iAMZ) then begin
            if (df.c.in_advance) and (not fancy) then write('Adv') else write('Arr'); {4/94 No ADV with FANCY}
            gotoxy(succ(wherex),y);
            end;
          if (df.lastrun=iAMZ) and (fancy) then begin
            if (df.c.plus_regular) then write('PlusReg') else write('InclReg');
            gotoxy(succ(wherex),y);
            end;
          if (df.lastrun=iAMZ) then begin
            if (df.c.prepaid) then write('PrePd') else write('No-PrePd');
            gotoxy(succ(wherex),y);
            end;
          if (df.lastrun<>iMTG) then begin
            write('19',df.c.centurydiv);
            gotoxy(succ(wherex),y);
            if (df.lastrun in [iCHR,iPVL]) then
              if (df.c.exact) then begin
                write('Exact');
                gotoxy(succ(wherex),y);
                end;
{$ifdef V_3}
            if (df.lastrun<>iINV) then
{$endif}
            case df.c.peryr of
               DAILY : write('DAY');
      CANADIAN..130  : write('CAN');
                 else write(df.c.peryr,'perYr');
                 end;
            gotoxy(succ(wherex),y);
            end;
          textattr:=saveattr;
          RestoreWindow;
          gotoxyabs(xgo,ygo);
{$endif}
          end;
{$endif}
(*
function Det3(x11,x12,x13,x21,x22,x23,x31,x32,x33 :real):real;
         begin
         Det3:= x11*x22*x33 + x12*x23*x31 + x13*x21*x32
              - x11*x23*x32 - x12*x21*x33 - x13*x22*x31;
         end;

function LinearInterpolation(x1,y1,x2,y2 :real):real;
          {If your last two stabs at getting y to be 0 were
           x1, and x2, and the results "missed" by y1 and y2,
           what should be your next guess x? }
         begin
         if (y2=y1) then LinearInterpolation:=0.5*(x1+x2) {to avoid divide by 0}
         else LinearInterpolation:=x2 - y2 * (x2-x1)/(y2-y1);
         end;

function QuadraticInterpolation(x1,y1,x2,y2,x3,y3 :real):real;
          {If your last three stabs at getting y to be 0 were
           x1, x2, and x3, and the results "missed" by y1, y2, and y3,
           what should be your next guess x? }
         var a,b,c,xx1,xx2,xx3,det,tryplus,tryminus,linear_result,radical  :real;
         begin
         {First, do linear interpolation, for approximation}
         linear_result:=LinearInterpolation(x1,y1,x2,y2);
         QuadraticInterpolation:=linear_result;
         xx1:=sqr(x1);  xx2:=sqr(x2);  xx3:=sqr(x3);
         det:=Det3(xx1,x1,1,xx2,x2,1,xx3,x3,1);
         if (det<teeny) then exit; {with linear result}
         a:=Det3(y1,x1,1,y2,x2,1,y3,x3,1) / Det;
         b:=Det3(xx1,y1,1,xx2,y2,1,xx3,y3,1) / Det;
         c:=Det3(xx1,x1,y1,xx2,x2,y2,xx3,x3,y3) / Det;
         if (abs(a)<teeny) then begin
            QuadraticInterpolation:=-c/b;
            exit;
            end;
         radical:=sqr(b) - 4*a*c;
         if (radical<0) then begin
            if (abs(radical)<tiny) then radical:=0
            else exit; {with linear result}
            end;
         tryplus:= (-b + sqrt(radical)) / (2*a);
         tryminus:= (-b - sqrt(radical)) / (2*a);
         {accept whichever is closer to linear result:}
         if (abs(tryplus-linear_result) < abs(tryminus-linear_result)) then QuadraticInterpolation:=tryplus
         else QuadraticInterpolation:=tryminus;
         end;

*)

{ TimesPerYearCases
  Purpose : Translate a keystroke into a payments-per-year code, in place.
  Params  : val - (var) current frequency, updated to the new one
            ki  - the key pressed (digit or letter)
  Behavior: Some keys toggle among related frequencies depending on the prior
            value, e.g. '2' yields 12 (monthly) if currently 1 (yearly) else 2
            (semi-annual); '6' yields 26 (biweekly) if currently 2 else 6;
            'C' = canadian|2, 'D' = daily (the 64 code meaning 360/365).
  NOTE: 'lastkey' is reset to 'X' on every call (it is a local, not retained),
        so the '5'-dependent branch of '2' can never fire here - apparently a
        latent bug carried over from the original. }
{ Go port: n/a -- keystroke-driven frequency toggle for the DOS input line; the
  web port sets peryr directly, so no keyboard-cycling equivalent exists. }
procedure TimesPerYearCases(var val :byte; ki :char);
          var lastkey :char;
          begin
          lastkey := 'X';
          case upcase(ki) of
                     '1','3' : val:=ord(ki)-48;
                         '2' : if (val=1) then val:=12
                               else if (lastkey<>'5') then val:=2
                               else val:=52;
                         '4' : if (val=2) then val:=24 else val:=4;
                         '5' : val:=52;
                         '6' : if (val=2) then val:=26 else val:=6;
                         'C' : val:=canadian or 2;
                         'D' : val:=daily
                           {daily actually=64.  This is a code for 360/365}
                         else  if (val=0) then val:=1;
                      end;
          lastkey:=ki;
          end;
(*
procedure ChooseTimesPerYear(ki :char);
          var val,y :byte;
          begin
          y:=wherey;
          screenstatus:=needs_everything;
          if (ki=' ') then begin ClearCellAndData(y,col); exit; end;
          val:=round(value(GetString(y,col)));
          TimesPerYearCases(val,ki);
          textattr:=GetColor(inp);
          gotoxy(startof[col],y); write(val:2); gotoxy(startof[col],y);
          end;
*)
{$ifdef 0}
procedure ToggleStr(y,col :byte; msg1,msg2 :str8);
          var  is_first  :boolean;
{$ifdef V_3}
               i         :shortint;
               statusp   :statusptr;
               boolp     :boolptr;
{$endif}
          begin
          screenstatus:=needs_everything;
          if (screen^[y,startof[col],1]=msg1[1]) and (screen^[y,succ(startof[col]),1]=msg1[2])
          then is_first:=true else is_first:=false;
          gotoxy(startof[col],y);
          textattr:=GetColor(inp);
          if (is_first) then write(msg2) else write(msg1);
          gotoxy(startof[col],y);
{$ifdef V_3}
          if (col in DollarColumns+[simplecol]) then begin
             i:=y-ztop[block]+scrollpos[block];
             statusp:=AdvancePointer(blockdata[block]^[i],dataoffset[col]);
             boolp:=AdvancePointer(statusp,1);
             statusp^:=inp;
             boolp^:=not boolp^;
             end;
{$endif}
          end;
procedure Home(b :byte);
          begin
          block:=b;
          col:=fcol[block];
          gotoxy(lleft[block],succ(ztop[block]));
          end;
{$endif}
(*
procedure LeftCol(b :byte);
          begin
          block:=b;
          col:=fcol[block];
          if (whereyAbs>bbot[block]) then gotoxy(startof[col],bbot[block])
          else gotoxy(startof[col],whereyAbs);
          end;

procedure HighlightChoices;
          var x,y    :byte;
              attr   :char;
          begin
          attr:=#14;
          for y:=24 to 25 do
            for x:=1 to 80 do begin
              if (screen^[y,x,1] in ['<','F']) then attr:='p' else if (screen^[y,pred(x),1]='>') then attr:=#14;
              screen^[y,x,2]:=attr;
              end;
          end;

procedure DiskError(name :str80; errno :byte);
          begin
          if (errno=101) then
             Message('Cannot write file '+name+'.  Your disk is full.')
          else Message('Problem writing file '+name+'.');
          end;
*)
{ dateok
  Purpose : Decide whether a daterec holds a real date.
  Param   : f - date to test
  Returns : true iff the month is 1..12.  Blank/unknown/error dates are encoded
            with sentinel months (e.g. m = -99 or -88), so a valid month alone
            distinguishes them. }
{ Go port: internal/dateutil/dateutil.go: DateOK (line 217) -- same 1..12 month
  validity test. }
function dateok(f :daterec):boolean;
         begin
         if (f.m>0) and (f.m<13) then dateok:=true else dateok:=false;
         {More conservative than necessary.  By my definition, dates
          that are not ok are either m=-99 or m=-88}
         end;

{ ok
  Purpose : Test whether a real cell value is an actual number.
  Param   : v - value to test
  Returns : true unless v equals one of the sentinel markers unk (unknown),
            blank (empty cell), or error. }
{ Go port: internal/finance/interest/math.go: OK (line 148) -- same
  not-unk/blank/error sentinel test. }
function ok(v :real):boolean;
         begin
         if (v<>unk) and (v<>blank) and (v<>error) then ok:=true
         else ok:=false;
         end;
{$ifdef 0}
function Bright(color :byte):byte;
         begin
         if (color shr 4 = blue) then bright:=color
         else begin
           color:=color and 127;
           if (color>15) then color:=color shr 4;
           bright:=8 or color;
           end;
         end;

function Dim(color :byte):byte;
         begin
         if (color shr 4 = blue) then dim:=color
         else begin
           color:=color and 127;
           if (color>15) then color:=color shr 4;
           dim:=7 and color;
           end;
         end;

function Dark(color:byte):byte;
         begin
         color:=color and 127;
         if (color<=15) then color:=(color and 7) shl 4;
         if (color=brown shl 4) then color:=color+7;
         dark:=color;
         end;

procedure ClearCell(line,col :byte);
          var w   :array[1..2] of char;
              xx  :byte;
          begin
          if (scripting) then exit;
{$ifdef PVLX}
          if (screen^[line,startof[col]+2,1]='X') then exit;
{$endif}
          w[1]:=' ';
          w[2]:=char(MainScreenColors);
          for xx:=startof[col] to endof[col] do MoveScreen(w,screen^[line,xx],1);
          end;

function GetColor(z :inout):byte;
         begin
         if (color) then case z of
              inp : GetColor:=blue shl 4 + white;
              defp : GetColor:=blue shl 4 + yellow;
              outp: GetColor:=7 shl 4 + 0;
              end
         else case z of
              inp : GetColor:=white;
              defp : GetColor:=white; {used to be 7}
              outp: GetColor:=112;
              end;
         end;

procedure TestAndClear(y,col :byte);
          begin
          textattr:=ord(screen^[y,startof[col],2]);
          if ((textattr and 15=0) or ((textattr>12) and (wherexAbs=startof[col]))) then ClearCell(y,col);
          end;

procedure ReInterpretDatesAndRatesAccordingToNewSettings;
          const blockswithdates=[PVLLumpSumBlock,PVLPeriodicBlock,PVLPresValBlock,CHRBlock,
                                 AMZTopBlock,AMZBalanceBlock,AMZPreBlock,AMZBalloonBlock,AMZChangesBlock,AMZMoratoriumBlock];
          var col,block,i :byte;
              s           :statusptr;
              d           :dateptr;
              yearmod     :byte;

          begin
          if (dd.c.centurydiv<>df.c.centurydiv) then begin
            for block:=1 to nblocks do
              if (block in blockswithdates) then
                for col:=fcol[block] to lcol[block] do
                  if (coltype[col]=date_fmt) then
                     for i:=1 to nlines[block] do begin
                       s:= AdvancePointer(blockdata[block]^[i],dataoffset[col]);
                       if (s^>=defp) then begin
                          d:=AdvancePointer(s,1);
                          yearmod:=d^.y mod 100;
                          if (yearmod<dd.c.centurydiv) and (yearmod>=df.c.centurydiv) and (d^.y>=100) then dec(d^.y,100)
                          else if (yearmod>=dd.c.centurydiv) and (yearmod<df.c.centurydiv) and (d^.y<100) then inc(d^.y,100);
                          end;
                       end;
            end;
          if (dd.c.peryr<>df.c.peryr) then begin
            if (PVLfancy) then begin
               for i:=1 to nlines[PVLRatesBlock] do
                  if (cc[i]^.r.status=fromapr) then
                     cc[i]^.r.rate:=RateFromYield(YieldFromRate(cc[i]^.r.rate,dd.c.peryr),df.c.peryr);
               end
            else begin {not PVLfancy}
               for i:=1 to nlines[PVLPresValBlock] do
                  if (c[i]^.r.status=fromapr) then
                     c[i]^.r.rate:=RateFromYield(YieldFromRate(c[i]^.r.rate,dd.c.peryr),df.c.peryr);
               end;
            for i:=1 to nlines[CHRBlock] do
               if (g[i]^.r.status=fromapr) and (g[i]^.peryr=0) then
                  g[i]^.r.rate:=RateFromYield(YieldFromRate(g[i]^.r.rate,dd.c.peryr),df.c.peryr);
            end;
          end;

procedure RealToScreen(line,col :byte; color:inout; x:real);
          var w         :byte;
              ws        :string[16];
          begin
          if errorflag then exit;
          if (coltype[col]=percent_fmt) then x:=100*x;
          textattr:=GetColor(color);
          w:=colwidth[col];
          case coltype[col] of
              short_fmt : str(round(x):w,ws);
              percent_fmt : ws:=ftoa4(x,w);
          else ws:=ftoa2(x,w,df.h.commas);
          end;
          if (ord(ws[0])>w) then ws[0]:=char(w);
          FastWrite(ws,line,startof[col],textattr);
          end;
{$endif}
{ DaysCloseEnough
  Purpose : In 30/360 mode, decide whether the span between two dates is an
            exact whole number of months (so interest can be taken month-wise)
            or whether actual days must be counted instead.
  Params  : date1,date2 - the two dates; peryr - payment frequency
  Returns : true if the days-of-month match, OR if the earlier date's day is a
            month-end (LastDayFn) and the later date's day is greater - i.e. the
            classic 30/360 "end-of-month maps to end-of-month" alignment. }
{ Go port: internal/dateutil/dateutil.go: DaysCloseEnough (line 604) -- same
  day-of-month match OR month-end alignment test. }
function DaysCloseEnough(date1,date2 :daterec; peryr :byte):boolean;
          {Do we calculate an exact number of months interest between the
           dates in question (in 360 mode), or do we need to count days?}
          begin
          if (date1.d=date2.d)
                    or
             (LastDayFn(date1,peryr) and (date2.d>date1.d))
                    or
             (LastDayFn(date2,peryr) and (date1.d>date2.d))
          then DaysCloseEnough:=true
          else DaysCloseEnough:=false;
          end;

(*
function DaysDif(z,a :daterec):longint;
         var til :longint;
         begin
         if (df.c.basis=x360) then begin
            til:=(z.y-a.y)*360 + (z.m-a.m)*30 + (z.d-a.d);
            if (a.d=31) and (z.d<31) then inc(til)
            else if (a.d=30) and (z.d=31) then til:=dec(til)
            else if (a.m=2) and (a.d>27) then til:=til-(30-a.d); {Feb 28 or 29}
            {This last line is a strange part of the 30/360 date specification.
            See "Std Secur Calc Methods", p 16.}
            DaysDif:=til;
            end
         else DaysDif:=Julian(z)-Julian(a);
         end;
*)
{ ExtendedJulian
  Purpose : Serial day number for a date, basis-aware.
  Param   : x - date
  Returns : In 30/360 basis, a synthetic serial y*360 + m*30 + d (so that day
            differences yield exact 30-day months); otherwise the true calendar
            Julian day number. }
{ Go port: internal/dateutil/dateutil.go: ExtendedJulian (line 622) -- same
  y*360+m*30+d synthetic serial in 30/360 basis, else true Julian. }
function ExtendedJulian(x :daterec):longint;
         begin                          
         if (df.c.basis=x360) then ExtendedJulian:=longint(x.y)*360 + longint(x.m)*30 + x.d
         else ExtendedJulian:=Julian(x);
         end;

(*  This function YearsDif used 365¬ days, abandoned 12/93 in favor of the one
    below that uses 365 and 366 
function YearsDif(z,a :daterec):real;
         var til  :real;
         begin
         if (df.c.basis=x360) then begin
            if (DateComp(a,z)>0) then begin
               YearsDif:=-YearsDif(a,z);
               exit end;
            til:=(z.y-a.y) + (z.m-a.m)/12 + (z.d-a.d)/360;
            if (a.d=31) and (z.d<31) then til:=til+1/360
            else if (a.d=30) and (z.d=31) then til:=til-1/360
            else if (a.m=2) and (a.d>27) then til:=til-(30-a.d)/360; {Feb 28 or 29}
            {This last line is a strange part of the 30/360 date specification.
             See "Std Secur Calc Methods", p 16.}
            YearsDif:=til;
            end
{
         else case df.lastrun of
            iamz : begin
                   til:=z.y-a.y;
                   a.y:=z.y;
                   YearsDif:=til+(Julian(z)-Julian(a))*yrinv;
                   end;
            else YearsDif:=(Julian(z)-Julian(a))*yrinv;
            end;
}
         else YearsDif:=(Julian(z)-Julian(a))*yrinv;
         end;
*)
(*  B of A claims they use 366 for denominator in leap years, 365 otherwise.
    But I'm reproducing their loan results using 365 all the time.
    Keep this version of YearsDif around in case it turns out you need it
    for some kinds of B of A loan.  In this case, you'll probably need to
    write it in as an option (maybe as a different kind of EXACT). *)

{ YearsDif (this version adopted 12/93; earlier variants kept in comments above)
  Purpose : Time elapsed, in years, from date a to date z (signed: negative if
            a is later than z).  This is the core "t" feeding every discount
            factor exp(-rate*t) in the financial engines.
  Params  : z - end date, a - start date
  Returns : elapsed years under the active day-count convention:
            * 30/360 basis: (yrs) + (months)/12 + (days)/360 with the special
              end-of-month corrections from "Standard Securities Calculation
              Methods" p.16 (the Feb-28/29 and 31-vs-30 day adjustments).
            * 365-day modes for PVL/INV/CHR screens, and the 365/360 family:
              plain Julian-day difference times yrinv (= 1/365.25 typically).
            * loan screens (iAMZ, possibly iRBT): an exact 365/366 rule that
              walks year-by-year, charging 366 days in leap years.  The
              12/31->1/1 boundary day is counted as belonging to the OLD year
              when deciding leap-ness.
  Side    : recurses (with arguments swapped) to handle a>z. }
{ Go port: internal/dateutil/dateutil.go: YearsDif (line 643) -- same three-way
  dispatch (30/360 with SSCM corrections; Julian*yrinv for PVL/INV/CHR & 365/360;
  year-walking 365/366 exact rule for loan screens via the isLoanCalc flag). }
function YearsDif(z,a :daterec):real; {This version adopted 12/93}
         var til   :real;
             yrdaz :real;
             wd    :daterec;
         begin
         if (df.c.basis=x360) then begin
            if (DateComp(a,z)>0) then begin
               YearsDif:=-YearsDif(a,z);
               exit end;
            til:=(z.y-a.y) + (z.m-a.m)/12 + (z.d-a.d)/360;
            if (a.d=31) and (z.d<31) then til:=til+1/360
            else if (a.d=30) and (z.d=31) then til:=til-1/360
            else if (a.m=2) and (a.d>27) then til:=til-(30-a.d)/360; {Feb 28 or 29}
            {This last line is a strange part of the 30/360 date specification.
             See "Std Secur Calc Methods", p 16.}
            YearsDif:=til;
            end
         else if (thisrun in [iPVL,{$ifdef V_3}iINV,{$endif}iCHR]) or (df.c.basis=x365_360)
            then YearsDif:=(Julian(z)-Julian(a))*yrinv
         {in 365-day mode in PVL, INV and CHR, you use Julian diff/365.25
          whereas in loan calculations, use 365 and 366 or a combination for denominator.}
         else begin {iAMZ or possibly iRBT}
            if (DateComp(a,z)>0) then begin
               YearsDif:=-YearsDif(a,z);
               exit end;
            if (a.y mod 4 = 0) and (a.y>0) then yrdaz:=366 else yrdaz:=365;
            if (z.y=a.y) then YearsDif:=(Julian(z)-Julian(a)) / yrdaz
            else begin
              til:=pred(z.y-a.y);
              wd.d:=31; wd.m:=12; wd.y:=a.y;
              til:=til+YearsDif(wd,a)+1/yrdaz; 
                 {The day from 12/31 to 1/1 is considered part of the OLD year
                  for purposes of determining whether it's in a leap year.}
              wd.d:=1; wd.m:=1; wd.y:=z.y;
              YearsDif:=til+YearsDif(z,wd);
              end;
            end;
         end;

// Resurected because the assembled version is for
// 16bit smeg.
{ DateComp
  Purpose : Three-way comparison of two dates.
  Params  : f1,f2 - dates to compare
  Returns : +1 if f1 is later than f2, -1 if earlier, 0 if equal.
            Blank/unknown (not dateok) dates compare as LATER than any real
            date, so unfilled cells sort to the end.
  Impl    : valid dates are packed into a 4-byte combo (date + pad) and
            compared as longints - works because daterec field order makes the
            integer ordering match chronological order. }
{ Go port: internal/dateutil/dateutil.go: DateComp (line 253) -- same +1/-1/0
  three-way compare with blank/unknown dates sorting as later. }
function DateComp(f1,f2 :daterec):shortint;
         {+1 if f1 later than f2; -1 if earlier; 0 if same
          Blank or unknown dates are later than everything.}
                 type combo=record f:daterec; z:byte; end;
          var w1,w2  :combo;
              x1     :longint absolute w1;
              x2     :longint absolute w2;
         begin
         if dateok(f1) then begin
            if (not dateok(f2)) then DateComp:=-1
            else begin
               w1.f:=f1; w2.f:=f2; w1.z:=0; w2.z:=0;
               if (x1>x2) then DateComp:=1
               else if (x1<x2) then DateComp:=-1
               else DateComp:=0;
               end;
            end
         else if (dateok(f2)) then DateComp:=1
         else DateComp:=0;
         end;

{$ifdef 0}
This section was included in original Persense.  But that calls to an
assembled file that has 16 bit code in it.  So I have to go back
to the above definition.  Hope that's cool
function DateComp(f1,f2 :daterec):shortint; external;
{$L DATECOMP}
{$endif}
{ floor
  Purpose : Largest integer <= x (true mathematical floor, unlike Trunc which
            rounds toward zero for negatives).
  Param   : x - real value
  Returns : floor(x) as a longint. }
{ Go port: internal/dateutil/dateutil.go: Floor (line 363) -- same
  largest-integer-<=x logic (duplicated at internal/finance/interest/math.go:
  Floor line 156). }
function floor(x :real):longint;
         var tr :longint;
         begin
         if (x>0) then floor:=trunc(x)
         else begin
              tr:=trunc(x);
              if (x=tr) then floor:=tr
              else floor:=pred(tr);
              end;
         end;
{
This has been rewritten by James to be MessageBoxWithCancel or something
It's in Globals.

function EscapePressedInMessage(s :str80):boolean;
         var kibyte :byte;
         begin
         kibyte:=(Message(s) and $FF);
         EscapePressedInMessage:=(kibyte=27);
         end;
}
{ InsufficientDataMessage
  Purpose : Pop the standard "not enough input to compute <noun>" dialog.
  Params  : noun - what could not be computed (e.g. "rate", "value")
            HelpCode - context-help id for the dialog. }
{ Go port: n/a -- pops a DOS message box; the Go engine returns an error value
  instead of showing a dialog. }
procedure InsufficientDataMessage(noun :str12; HelpCode: integer );
          begin
          MessageBox('Insufficient data on screen - no '+noun+' can be calculated.', HelpCode);
          end;

{ TimeTooLong
  Purpose : Report that a computed/entered time span exceeds the engine's
            limit, and raise errorflag so callers abort.  No-op if errorflag is
            already set (avoids stacking messages). }
{ Go port: n/a -- "time too long" is surfaced as a returned error in the Go
  engine (e.g. AddYears' |yrs|>128 guard), not a global errorflag + dialog. }
procedure TimeTooLong;
          begin
          if (errorflag) then exit;
          MessageBox('Error - time period too long.', DO_TimeTooLong);
          errorflag:=true;
          end;

{ AddYears
  Purpose : Advance date a in place by a possibly-fractional number of years.
  Params  : a   - (var) date to move; left as an error date if a is invalid
            yrs - signed year offset (may be fractional)
  Basis   : In 30/360 mode the fraction is split into whole years, whole
            months, and 30-day days, then normalized with the 30-day-month
            carry rules.  Otherwise it converts to days (yrs*yrdays), adds to
            the Julian serial, and converts back via MDY.
  Guards  : |yrs|>128 is rejected as "time too long" (an arbitrary safety cap). }
{ Go port: internal/dateutil/dateutil.go: AddYears (line 383) -- same 30/360
  split-into-years/months/30-day-days path vs Julian+yrs*yrdays path, same
  |yrs|>128 cap (returned as an error). }
procedure AddYears(var a:daterec; yrs:real);
         var years,months,days :integer;
             j                 :longint;

         begin
         if (abs(yrs)>128) then TimeTooLong {Arbitrary limit}
         else if (not dateok(a)) then a.m:=errorbyte
         else if (df.c.basis=x360) then begin
           years:=floor(yrs); yrs:=yrs-years;
           months:=trunc(yrs*12);
           days:=round(360*(yrs-months/12));
           a.y:=a.y+years;
           a.m:=a.m+months;
           a.d:=a.d+days;
           if (a.d>30) then begin a.d:=a.d-30; inc(a.m); end;
           while (a.m>12) do begin a.m:=a.m-12; inc(a.y); end;
           while (a.m<1) or (a.m>240) do begin a.m:=a.m+12; dec(a.y); end;
           CheckForDaysTooLarge(a);
           end
         else begin
              j:=Julian(a)+round(yrs*yrdays);
              MDY(j,a);
              end;
         end;

{ AddDays
  Purpose : Advance date f in place by 'days' calendar days (may be negative).
  Impl    : convert to Julian serial, add days, convert back with MDY. }
{ Go port: internal/dateutil/dateutil.go: AddDays (line 356) -- same
  Julian + days -> MDY round-trip. }
procedure AddDays(var f :daterec; days :longint);
          var j:longint;
          begin
          j:=Julian(f)+days;
          MDY(j,f);
          end;

{ LastDayFn
  Purpose : Is date f the last payment day of its (half-)month?
  Params  : f - date; peryr - payment frequency
  Returns : true if f.d is the last calendar day of its month, OR (for
            semi-monthly, peryr=24) the 15th, which is that scheme's mid-month
            payment day.  Used so month-end payments roll correctly. }
{ Go port: internal/dateutil/dateutil.go: LastDayFn (line 568) -- same
  last-day-of-month OR (peryr=24 and d=15) test. }
function LastDayFn(f:daterec; peryr :byte):boolean;
         begin with f do begin
         if (d=daysinm(f)) or ((peryr=24) and (d=15)) then LastDayFn:=true
         else LastDayFn:=false;
         end; end;

{ Criterion (local helper)
  Purpose : Evaluate one of the four ordering tests (before / on_or_before /
            after / on_or_after) between dates d1 and d2, selected by z.
  Returns : true if d1 relates to d2 as z specifies. }
{ Go port: internal/dateutil/dateutil.go: Criterion (line 584) -- same four-way
  before/on_or_before/after/on_or_after DateComp test. }
function Criterion(d1,d2 :daterec; z:upto):boolean;
         begin
         case z of
            before : Criterion:= (DateComp(d1,d2)<0);
      on_or_before : Criterion:= (DateComp(d1,d2)<=0);
             after : Criterion:= (DateComp(d1,d2)>0);
       on_or_after : Criterion:= (DateComp(d1,d2)>=0);
         end; end;

{ NumberOfInstallments
  Purpose : Count how many regular payments fall between first date f and last
            date l for a loan paying peryr times a year, AND snap l onto an
            exact payment date near the input l.
  Params  : f - (var) first/anchor payment date (its day-of-month defines the
                cycle); l - (var) last date, ADJUSTED on return to the nearest
                payment day in the direction given by z
            peryr - payment frequency (1,2,3,4,6,12 monthly-family; 24 semi-
                    monthly; 26 biweekly; 52 weekly)
            z - which payment to land on: before / on_or_before / after /
                on_or_after the input l
  Returns : number of installments (always succ(theresult) so that both the
            first and last payments are included in the count).  Returns maxint
            when l is the "latest" sentinel (open-ended).
  Nested  : ChoosePaymentDate does the frequency-specific snapping of l:
            * weekly/biweekly (26,52): work in Julian days, step by 364/peryr.
            * semi-monthly (24): estimate by half-months, then walk AddPeriod
              until Criterion(atry,l,z) holds.
            * monthly family: align on month boundaries, honoring month-end
              (flast/llast) via LastDayFn and the mod-arithmetic that can yield
              negatives. }
{ Go port: internal/dateutil/dateutil.go: NumberOfInstallments (line 720) --
  returns (count, snapped-l) pair; the nested ChoosePaymentDate snapping and the
  weekly/biweekly/semi-monthly/monthly-family branches are inlined there. }
function NumberOfInstallments(var f,l :daterec; peryr:byte; z:upto):integer;
           {This function not only returns a number of payments, but also
            adjusts l to be exactly on a payment day, in the vicinity of
            the input l, as specified by z:
            z can be one of four options - 1: payment on or before last
            2: payment before last  3:payment on or after last  4: payment after last}

         var ddiff,mdiff,monthsbtwn,theresult  :integer;
             orig_day                       :byte;
             llast,flast                    :boolean;
             last                           :longint;
             atry,originall                  :daterec;

   procedure ChoosePaymentDate;
             var daze :byte;
             begin
             case peryr of
          26, 52 : begin
                   ddiff:=Julian(l)-Julian(f);
                   daze:=364 div peryr;
                   ddiff:=ddiff mod daze;
                   if (ddiff=0) and (z in [before,on_or_after]) then ddiff:=daze;
                   if (z in [before,on_or_before]) then last:=Julian(l)-ddiff
                   else last:=Julian(l)+daze-ddiff;
                   MDY(last,l);
                   end;
              24 : begin
                   theresult:=2*(integer(l.m)-f.m) + 24*(integer(l.y)-f.y); {1st estimate}
                   case z of
                     before, on_or_before : begin
                       theresult:=theresult+2;
                       AddNPeriods(f,atry,peryr,theresult);
                       while (not Criterion(atry,l,z)) do begin
                         AddPeriod(atry,peryr,f.d,subtract);
                         dec(theresult);
                         end;
                       end;
                     after, on_or_after   : begin
                       theresult:=theresult-2;
                       atry:=f;
                       AddNPeriods(f,atry,peryr,theresult);
                       while (not Criterion(atry,l,z)) do begin
                         AddPeriod(atry,peryr,f.d,add);
                         inc(theresult);
                         end;
                       end;
                     end;
                   l:=atry;
                   end;
              else {peryr in [1,2,4,6,12]} begin
                   orig_day:=f.d;
                   flast:=LastDayFn(f,peryr);
                   llast:=LastDayFn(l,peryr);
                   if (peryr>12) then monthsbtwn:=1
                   else monthsbtwn:=12 div peryr;
                   ddiff:=integer(l.d)-f.d;
                   mdiff:=integer(l.m)-f.m;
                   mdiff:=mdiff mod monthsbtwn;
                   while (mdiff<0) do mdiff:=mdiff+monthsbtwn; {MOD can produce negatives}
                   if (mdiff=0) then case z of
                      before :
                         if (ddiff<=0) or (flast and llast) then l.m:=l.m-monthsbtwn;
                      on_or_before :
                         if (ddiff<0) and (not (flast and llast)) then l.m:=l.m - monthsbtwn;
                      after :
                         if (ddiff>=0) or (flast and llast) then l.m:=l.m + monthsbtwn;
                      on_or_after :
                         if (ddiff>0) and (not (flast and llast)) then l.m:=l.m + monthsbtwn;
                      end {case z}
                   else {mdiff>0} case z of
                       on_or_before,before : l.m:=l.m - mdiff;
                       on_or_after,after   : l.m:=l.m + monthsbtwn - mdiff;
                       end; {case z}
                   {Now correct if we've gone past end of year}
                   if (l.m<=0) {m is a shortint} then begin
                        dec(l.y);
                        l.m:=l.m + 12;
                        end
                   else if (l.m>12) then begin
                        inc(l.y);
                        l.m:=l.m - 12;
                        end;
                   if (flast) then l.d:=daysinm(l) else l.d:=f.d;
                   last:=Julian(l);
                   end; {else (peryr in [1,2,4,6,12])}
               end; {case peryr}
             end; {ChoosePaymentDay}


         begin {NumberOfInstallments}
         if (l.y=latest.y) then begin   { open-ended "latest" sentinel -> infinite }
            NumberOfInstallments:=maxint;
            exit;end;
         originall:=l;
         ChoosePaymentDate;             { snaps l to a payment day per z }
         { Count periods between f and the snapped l; +1 below to include both ends }
         case peryr of
              52 : theresult:=(Julian(l) - Julian(f)) div 7;
              26 : theresult:=(Julian(l) - Julian(f)) div 14;
              24 : ; {Do nothing.
                      We already computed result in ChoosePaymentDate.
                   Our answer, as always, didn't include both first and last
                   so we're ready to add one below.}
              else theresult:=( 12*(l.y-f.y)+ (l.m-f.m) ) div monthsbtwn;
              end;
         NumberOfInstallments:=succ(theresult);
         end;

{ RevVideo
  Purpose : Is the cell at (line,col) currently displayed reverse-video?
  Returns : true if the attribute byte there is > #31 (the reverse-video range).
  Note    : UI helper; inspects the screen buffer directly. }
{ Go port: n/a -- reads the DOS text-screen attribute buffer; no web equivalent. }
function RevVideo(line,col :byte):boolean;
         begin
         if (screen^[line,startof[col],2]>#31) then revvideo:=true
         else revvideo:=false;
         end;

{ FindBlock
  Purpose : Find which screen block a column belongs to.
  Param   : col - column index
  Returns : the block number whose [fcol..lcol] range contains col; falls back
            to SpecialBlock (with a diagnostic for non-special columns) if none. }
{ Go port: n/a -- maps a text-screen column to its layout block; DOS UI geometry. }
function FindBlock(col :byte):byte;
         var b :byte;
         begin
         b:=1;
         while (b<=nblocks) and ((col>lcol[b]) or (col<fcol[b])) do inc(b);
         if (b>nblocks) then begin
            if (not (col in SpecialColumns))
              then MessageBox('In FindBlock, Block not found : col='+strb(col,0), DO_FindBlockNotFound);
            b:=SpecialBlock;
            end;
         FindBlock:=b;
         end;

{ RecordError
  Purpose : Mark an input cell as erroneous.
  Params  : line, errorcol - screen location of the offending cell
  Effect  : sets the global errorflag so callers abort the calculation.  In the
            original it also blinked/highlighted the cell and moved the cursor;
            that screen manipulation is commented out in this port, leaving only
            the flag (which is what the financial logic depends on). }
{ Go port: n/a -- the Go engine reports bad cells via returned FieldError values
  (see internal/api handlers) rather than a global errorflag + cell highlight. }
procedure RecordError(line,errorcol :byte);
          var attr: byte;
          begin
          errorflag:=true;
          if (scripting) then exit;
{
setting the error flag is well and good.  But the rest of this function
seems like it's all visual stuff.  So no deal
          col:=errorcol; block:=FindBlock(col);
          if (line>ztop[block]+nlines[block]) then line:=ztop[block]+nlines[block]
          else if (line<=ztop[block]) then line:=succ(ztop[block])
          else begin
            attr:=GetColor(inp) or blink;
            ChangeAttribute(colwidth[col],line,startof[col],attr);
            end;
          gotoxyabs(startof[col],line);
          }
          end;
{$ifdef 0}
Following seem like functions for reading data from the screen.  Again, not needed
in this version of persense

function GetString(line,col :byte):string;
         var ws :string;
         begin
         FastRead(colwidth[col],line,startof[col],ws);
         GetString:=trim(ws);
         end;

procedure GetDate(line,col:byte; var f :daterec; var ok :boolean);
          var wdate        :str8;
              clear_rev    :boolean;
         {if (line>128), this is a signal not to erase output fields but to read them.}

          begin
          if (line>128) then begin line:=line-128; clear_rev:=false; end
          else clear_rev:=true;
          wdate:=GetString(line,col);
          if (RevVideo(line,col) and clear_rev) or (wdate='') or (wdate=eightblanks) or (pos('?',wdate)>0) then begin
            f.m:=unkbyte; ok:=false;
            textattr:=Bright(ord(screen^[line,startof[col],2]));
            ClearCell(line,col);
            end
          else begin
             Str2MDY(wdate,f);
             if dateok(f) then begin
                ok:=true;
                {if (ord(screen^[line,startof[col],2]) and 8 = 8) and (clear_rev) then PrintDate(line,col,defp,f);}
                end
             else begin
               RecordError(line,col);
               ok:=false;
               end;
             end;
          end;

function GetJulian(line,col:byte; var ok :boolean):longint;
         var f :daterec;
         begin
         GetDate(line,col,f,ok);
         if ok then GetJulian:=Julian(f)
         else if (f.m=unkbyte) then GetJulian:=unk
         else GetJulian:=error;
         end;
{$endif}

{ sqrrt
  Purpose : Numerically safe square root.
  Param   : x - radicand
  Returns : sqrt(x); for x<0 returns 0.  A negative within rounding noise
            (>= -teeny) is treated as 0 silently; a meaningfully negative value
            raises an "inconsistency" error and sets errorflag/overflowflag.
            Lets callers form sqrt(b^2-4ac) without crashing on tiny negatives. }
{ Go port: internal/finance/interest/math.go: Sqrrt (line 86) -- same
  negative-radicand handling (0 within -Teeny, error beyond); returns an error
  instead of setting overflowflag. }
function sqrrt(x :real):real;
         begin
         if (x<0) then begin
            sqrrt:=0;
            if (x<-teeny) then begin
              MessageBox('Error: The data you have specified contain an inconsistency.', DO_SqrrtTiny );
              sqrrt:=0;
              errorflag:=true; overflowflag:=true;
              end;
            end
         else sqrrt:=sqrt(x);
         end;

{ QuadraticFormula
  Purpose : One real root of A*x^2 + B*x + C = 0.
  Returns : the "(-B - sqrt(B^2-4AC)) / 2A" branch, using the safe sqrrt so a
            slightly-negative discriminant yields 0 rather than a crash.
  NOTE: caller must ensure A<>0; only this single root is returned. }
{ Go port: internal/finance/interest/math.go: QuadraticFormula (line 118) --
  same (-B - sqrt(B^2-4AC))/2A branch via the safe Sqrrt. }
function QuadraticFormula(A,B,C :real):real;
         begin
         QuadraticFormula:= (- B - sqrrt(sqr(B)-4*A*C)) / (2*A);
         end;

{ exxp
  Purpose : Numerically safe exponential exp(x) used throughout the discount
            and compounding math.
  Param   : x - exponent
  Returns : exp(x), with three guards:
            * x>70 : would overflow the real format -> returns 0, sets overflow
              and error flags, and warns the user.
            * x<-70 : underflows to ~0 -> returns a tiny 1E-32 floor.
            * |x|<small : uses the 4-term Taylor series 1+x+x^2/2+x^3/6 purely
              as a small speed optimization (not a bug workaround).
            otherwise defers to the library exp. }
{ Go port: internal/finance/interest/math.go: Exxp (line 43) -- same >70 overflow
  (returns error), <-70 -> 1e-32 floor, and |x|<Small 4-term Taylor guards. }
function exxp(x :real):real;
         const sixth=1/6;
         var x2 :real;
         begin
         if (x>70) then begin
            exxp:=0;
            MessageBox('Overflow error: answer too large for this computer''s numeric format.', DO_ExxpOverflow);
            overflowflag:=true; errorflag:=true;
            end
         else if (x<-70) then exxp:=1E-32
         else if (abs(x)<small) then begin
              x2:=sqr(x);
              exxp:=1 + x + half*x2 + sixth*x*x2;
              end
           {No compiler bug - this is just for a tiny improvement in speed}
         else exxp:=exp(x);
         end;

{ lnn
  Purpose : Numerically safe natural log ln(x), the inverse partner of exxp.
  Param   : x - argument
  Returns : ln(x).  Guards:
            * x<=0 : illegal -> returns 0, raises error/overflow flags, warns.
            * |x-1|<small : uses the Taylor series (x-1) - (x-1)^2/2 + (x-1)^3/3
              to WORK AROUND a genuine Turbo Pascal bug where library ln() drops
              to 0 too quickly near 1 (noticeable from ~1.0001, hence small=1E-4).
            otherwise defers to library ln. }
{ Go port: internal/finance/interest/math.go: Lnn (line 67) -- same x<=0 error
  and |x-1|<Small Taylor-series workaround for the Turbo ln() near-1 bug. }
function lnn(x :real):real;
         const third=1/3;
         var t,t2 :real;
         begin
         if (x<=0) then begin
            MessageBox('Error: The data you have specified contain an inconsistency.', DO_LnnNegative);
            lnn:=0;
            errorflag:=true; overflowflag:=true;
            end
         else if (abs(x-1)<small) then begin
              t:=x-1;
              t2:=sqr(t);
              lnn:=t - half*t2 + third*t*t2;
              end
           {Turbo compiler bug - ln goes to 0 too fast for x close to 1!
            The effect begins to be noticeable at about 1.0001, hence small=1E-4}
         else lnn:=ln(x);
         end;

(*
procedure PrintDate(line,col :byte; color:inout; date:daterec);
          begin
          if errorflag then exit;
{$ifdef TESTING}
          if (TestData.active) then TestData.CompareDate(line,col,date);
{$endif}
          textattr:=GetColor(color);
          FastWrite(DateStr(date),line,startof[col],textattr);
          end;
*)
         {
procedure TempMessage(ws :str80);
          begin
          FillChar(ws[succ(length(ws))],80-length(ws),' ');
          ws[0]:=#80;
          FastWrite(ws,25,1,MessageColors);
          HiddenCursor;
          end;

procedure RequestPatience;
          begin
          TempMessage(' Patience.. . .  .');
          end;
          }
{ AddPeriod
  Purpose : Move date f forward or back by exactly ONE payment period.
  Params  : f        - (var) date to advance
            peryr    - payment frequency
            orig_day - the loan's anchor day-of-month, so monthly payments stay
                       pinned to (e.g.) the 15th even after months with fewer
                       days nudged the day; the +/-4 snap restores it.
            negative - true to step backward
  Behavior: weekly/biweekly step by 364/peryr days (via Julian); semi-monthly
            (24) steps by 15 days with month carry and the orig_day snap;
            monthly family steps by 12/peryr months with year carry, resetting
            the day to orig_day.  CheckForDaysTooLarge clamps invalid days. }
{ Go port: internal/dateutil/dateutil.go: AddPeriod (line 442) -- same
  weekly/biweekly (364/peryr days), semi-monthly (15-day + orig_day snap), and
  monthly-family (12/peryr months, reset to orig_day) branches. }
procedure AddPeriod(var f :daterec; peryr :byte; orig_day:byte; negative :boolean);
          var t:longint;
          begin with f do begin
          case peryr of
          26,52 : begin
                  t:=Julian(f);
                  if negative then t:=t - (364 div peryr) else t:=t + (364 div peryr);
                  MDY(t,f);
                  end;
             24 : begin
                  if (abs(d-orig_day)<4) then d:=orig_day;
                  if (negative) then begin
                     d:=d-15;
                     if (d<1) then begin
                        dec(m); d:=d+30;
                        if (m<=0) then begin
                           dec(y); m:=m+12;
                           end;
                         end;
                     end
                  else {not negative} begin
                     d:=d+15;
                     if (d>=31) then begin
                        inc(m); d:=d-30;
                        if (m>12) then begin
                           inc(y); m:=m-12;
                           end;
                        end;
                     end;
                  if (abs(d-orig_day)<4) then d:=orig_day;
                  end;
           else begin {peryr=1,2,3,4,6,12}
                d:=orig_day;
                if negative then m:=m - (12 div peryr) else m:=m + (12 div peryr);
                if (m<1) or (m>240) then begin
                   m:=m+12;
                   dec(y);
                   end
                else if (m>12) then begin
                   m:=m-12;
                   inc(y);
                   end;
                end;
            end; {case peryr}
          CheckForDaysTooLarge(f);
          end; {with f} end; {AddPeriod}

{ RealPerYr
  Purpose : Convert a payments-per-year CODE into the actual real number of
            compounding periods per year used in the rate<->yield formulas.
  Param   : n - frequency code
  Returns : daily -> yrdays (360 or 365.25); 52 -> yrdays/7; 26 -> yrdays/14;
            otherwise n with the 'canadian' bit masked off (Canadian code is
            (canadian or 2), so this recovers the base period count, e.g. 2). }
{ Go port: internal/finance/interest/rates.go: RealPerYr (line 42) -- same
  daily->yrdays, 52->yrdays/7, 26->yrdays/14, else n with canadian bit masked;
  takes yrdays as an explicit arg (no global). }
function RealPerYr(n :byte):real;
         begin
         case n of daily : RealPerYr:=yrdays; {changed 4/94 from round(yrdays)}
                      52 : RealPerYr:=yrdays/7;
                      26 : RealPerYr:=yrdays/14;
         else RealPerYr:=n and (not canadian);
         end;end;

{ YieldFromRate
  Purpose : Convert the program's internal CONTINUOUS (true) rate rr into the
            periodic-compounded "yield" the user sees / enters.
  Params  : rr - continuous rate; n - frequency code
  Returns : nn*(e^(rr/nn) - 1), where nn = RealPerYr(n).  This is the per-year
            equivalent of "compound rr/nn each of nn periods".  Inverse of
            RateFromYield. }
{ Go port: internal/finance/interest/rates.go: YieldFromRate (line 63) -- same
  nn*(e^(rr/nn)-1) with nn=RealPerYr(n); returns an error from the Exxp guard. }
function YieldFromRate(rr :real; n:byte):real;
         var nn :real;
         begin
         nn:=RealPerYr(n);
         YieldFromRate:=nn*(exxp(rr/nn)-1);
         end;

{ RateFromYield
  Purpose : Convert a periodic-compounded yield yy back to the internal
            continuous rate.  Inverse of YieldFromRate.
  Params  : yy - periodic yield; n - frequency code
  Returns : nn*ln(1 + yy/nn), where nn = RealPerYr(n). }
{ Go port: internal/finance/interest/rates.go: RateFromYield (line 79) -- same
  nn*ln(1+yy/nn) with nn=RealPerYr(n); returns an error from the Lnn guard. }
function RateFromYield(yy :real; n:byte):real;
         var nn :real;
         begin
         nn:=RealPerYr(n);
         RateFromYield:=nn*lnn(1+yy/nn);
         end;

{ InBounds
  Purpose : Range-check (and normalize) a value just read from an input cell.
  Params  : x        - (var) value; converted from percent to fraction (x*0.01)
                        on success for percent columns, or set to ERROR on a
                        range violation
            line,col - cell location (for RecordError)
  Returns : true if x is in range for its column type.  Percent cells must have
            |x|<=99; points columns additionally require 0<=x<10.  Sentinel
            unknown/blank values return false without error. }
{ Go port: n/a -- per-cell range/percent-normalization validation is done at the
  API request-decoding boundary in internal/api rather than a shared helper. }
function InBounds(var x :real; line,col :byte):boolean;
         var theresult :boolean;
         begin
         if (x=UNK) or (x=UNKBYTE) or (x=BLANK) then begin InBounds:=false; exit; end;
         if (coltype[col]=percent_fmt) then begin
            if (abs(x)>99) then theresult:=false else begin
               if (col in [pointscol,apointscol]) and ((x<0) or (x>=10)) then theresult:=false
               else theresult:=true;
               end;
             end
         else theresult:=true;
         if (not theresult) then begin
            RecordError(line,col);
            x:=ERROR;
            end
         else if (coltype[col]=percent_fmt) then x:=0.01*x;
         InBounds:=theresult;
         end;

{$ifdef 0}
function BlankLine(block,y :byte):boolean;
         var ws :str80;
             i  :byte;
         begin
         if (scripting) then begin
            BlankLine:=(y-ztop[block]>nlines[block])  {a kludge}
            end
         else begin
            ws:=StringFromScreen(lleft[block],y,succ(rright[block]-lleft[block]));
            i:=0;
            repeat inc(i) until (i>length(ws)) or (ws[i] in ['0'..'9']);
            BlankLine:=(i>length(ws));
            end;
         end;

More functions for reading and doing things to the screen

function GetNumber(line,col:byte; var ok :boolean):real;
         {if (line>128), this is a signal not to erase output fields but to read them.}
         var ws        :string[20];
             gn        :real;
             clear_rev :boolean;

         begin
         if (line>128) then begin
            line:=line-128;
            clear_rev:=false;
            end
         else clear_rev:=true;
         ws:=GetString(line,col);
         if (ws='?') then begin
            GetNumber:=UNK;
            ok:=false;
            end
         else if (RevVideo(line,col)) and (clear_rev) then begin
            GetNumber:=UNK;
            textattr:=Bright(ord(screen^[line,startof[col],2]));
            ClearCell(line,col);
            ok:=false;
            end
         else begin
            if (ws[0]=#0) then begin gn:=BLANK; ok:=false; end
            else gn:=value(ws); {includes ERROR}
            if (gn=ERROR) then begin RecordError(line,col); ok:=false; end
            else if (InBounds(gn,line,col)) then begin
              ok:=true;
              {if (ord(screen^[line,startof[col],2]) and 8 = 8) and (clear_rev) then Print(line,col,defp,gn);}
              end
            else ok:=false;
            GetNumber:=gn;
            end;
         end;

function GetByte(line,col:byte):byte;
         var ws        :string[20];
             gn        :real;

         begin
         ws:=GetString(line,col);
         if (ws='?') then GetByte:=0
         else if (RevVideo(line,col)) then begin
            GetByte:=0;
            textattr:=Bright(ord(screen^[line,startof[col],2]));
            ClearCell(line,col);
            end
         else begin
            if (ws[0]=#0) then gn:=0
            else gn:=value(ws); {includes ERROR}
            if (gn=ERROR) then begin RecordError(line,col); gn:=0; end
{            else if (gn>0) and (ord(screen^[line,startof[col],2]) and 8 = 8)
              then Print(line,col,defp,gn)} ;
            if (gn>=0) and (gn<=255) then GetByte:=round(gn)
            else begin
                 RecordError(line,col);
                 GetByte:=0;
                 end;
            end;
         end;

procedure Del(col :byte);
          var i,x,y  :byte;
          begin
          AbsXY(x,y);
          if (col=asofcol) and (screen^[y,startof[col]+3,1]='X') then exit; {PVLRatesBlock}
          if ((shiftstatus and 3 > 0) or (x=startof[col])) and (coltype[col]<>STRING_FMT) then ClearCell(y,col)
          else if (coltype[col]=DATE_FMT) then begin
               textattr:=GetColor(inp); write(' ',#8); end
          else if (coltype[col]<>STRING_FMT) then begin
            TestAndClear(y,col);
            for i:=x to pred(endof[col]) do screen^[y,i]:=screen^[y,succ(i)];
            screen^[y,endof[col],1]:=' ';
            end;
          screenstatus:=needs_everything;
          end;
{$endif}
{ AddNPeriods
  Purpose : Set lastdate = firstdate advanced by n whole payment periods.
  Params  : firstdate - (var) anchor date (read; its day-of-month is the cycle)
            lastdate  - (out) result date
            peryr     - payment frequency; n - number of periods (may be 0/neg)
  Behavior: monthly-family/semi-monthly: jump whole years first (n div peryr),
            then step the remaining (non-negative) periods with AddPeriod.
            weekly/biweekly: just add n*(365 div peryr) days. }
{ Go port: internal/dateutil/dateutil.go: AddNPeriods (line 526) -- same
  whole-years-then-remaining-periods for monthly/semi-monthly, n*(365/peryr)
  days for weekly/biweekly; returns the computed lastDate. }
procedure AddNPeriods(var firstdate,lastdate :daterec; peryr :byte; n :integer);
          var i              :byte;
              nyears         :integer;
              ndays          :longint;
          begin
          case peryr of
             1,2,3,4,6,12,24 : begin
                 lastdate:=firstdate;
                 nyears:=n div peryr;
                 if (n mod peryr<0) then dec(nyears);
                 lastdate.y:=firstdate.y + nyears;
                 n:=n - peryr*nyears; {Always non-negative}
                 if (n=0) then CheckForDaysTooLarge(lastdate)
                 else for i:=1 to n do AddPeriod(lastdate,peryr,firstdate.d,add);
                 end;
             else {26,52} begin
                 ndays:=longint(n) * (365 div peryr); {7*n or 14*n}
                 MDY(ndays + Julian(firstdate), lastdate);
                 end;
             end; {case}
          end;

{$ifdef 0}
No lotus support at the moment

procedure WritePVLHeadersToLotus;
          begin
          inc(lotusrow);  {skip a line}
          if (pvlfancy) then begin
            lotuscol:=0; {1st col}
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Effective');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'True');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Loan');
            inc(lotuscol,4);
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Interest');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Total');
            inc(lotusrow); lotuscol:=0;
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Date');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Rate');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Rate');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Yield');
            inc(lotuscol,2);
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'As of');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Computation');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Value');
            inc(lotusrow);
            end
          else begin
            lotuscol:=1; {2d col}
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'True');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Loan');
            inc(lotusrow); lotuscol:=0;
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'As of');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Rate');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Rate');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Yield');
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'Value');
            inc(lotusrow); lotuscol:=5;
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'<Present');
            inc(lotusrow); dec(lotuscol);
            WriteOneStringCell(true,'^',lotusrow,lotuscol,'<Value');
            dec(lotusrow);
            end;
          end;

(*
procedure WriteAMZHeadersToLotus(bottomstart :byte);
          begin
          WriteOneStringCell(true,'''',3,0,'Additional periodic pmts:');
          WriteOneStringCell(true,'^',6,0,'Balloon Payments');
          WriteOneStringCell(true,'^',7,0,'Date');
          WriteOneStringCell(true,'^',7,1,'Amount');
          WriteOneStringCell(true,'^',6,3,'Rate Changes and Changes in Payment');
          WriteOneStringCell(true,'^',7,3,'Date');
          WriteOneStringCell(true,'^',7,4,'Rate');
          WriteOneStringCell(true,'^',7,5,'Amount');
          WriteOneStringCell(true,'^',bottomstart,0,'Principal');
          WriteOneStringCell(true,'^',1+bottomstart,0,'Repayment');
          WriteOneStringCell(true,'^',2+bottomstart,0,'Delayed');
          WriteOneStringCell(true,'^',bottomstart,1,'Date');
          WriteOneStringCell(true,'^',bottomstart,2,'Targeted');
          WriteOneStringCell(true,'^',1+bottomstart,2,'Principal');
          WriteOneStringCell(true,'^',2+bottomstart,2,'Reduction');
          WriteOneStringCell(true,'^',bottomstart,3,'Amount');
          WriteOneStringCell(true,'^',bottomstart,5,'Date');
          WriteOneStringCell(true,'^',bottomstart,6,'Balance as of');
          end;
*)
procedure WriteAMZHeadersToLotus(bottomstart :byte);
          begin
          if (fancy) then begin
            WriteOneStringCell(true,'''',3,0,'Additional periodic');
            WriteOneStringCell(true,'''',4,0,'& skipped payments:');
            WriteOneStringCell(true,'^',6,0,'Balloon Payments');
            WriteOneStringCell(true,'^',7,0,'Date');
            WriteOneStringCell(true,'^',7,1,'Amount');
            WriteOneStringCell(true,'^',6,3,'Rate Changes and Changes in Payment');
            WriteOneStringCell(true,'^',7,3,'Date');
            WriteOneStringCell(true,'^',7,4,'Rate');
            WriteOneStringCell(true,'^',7,5,'Amount');
            WriteOneStringCell(true,'^',bottomstart,0,'Princ Repaymt Delayed');
            WriteOneStringCell(true,'^',bottomstart,2,'Targeted Principal Reduction');
            end;
          WriteOneStringCell(true,'^',bottomstart,5,'Date');
          WriteOneStringCell(true,'^',bottomstart,7,'Balance as of');
          end;
{$endif}
{ ReportedRate
  Purpose : Translate an internal apr into the rate as shown to the user under
            the current per-year setting.
  Param   : apr - internal rate
  Returns : when the setting is Canadian or Daily, re-expresses apr from the
            header's compounding (h^.peryr) into the display compounding
            (df.c.peryr) via RateFromYield then YieldFromRate; otherwise apr
            unchanged.  Inverse of InterpretedRate. }
{ Go port: internal/finance/interest/rates.go: ReportedRate (line 102) -- same
  Canadian/Daily re-expression via RateFromYield then YieldFromRate; takes the
  loan and settings peryr as explicit args instead of reading globals h^/df. }
  function ReportedRate(apr :real):real;
  begin
    if (df.c.peryr and canadianORdaily>0)
      then ReportedRate:=YieldFromRate(RateFromYield(apr,h^.peryr),df.c.peryr)
    else ReportedRate:=apr;
  end;

{ InterpretedRate
  Purpose : Inverse of ReportedRate - turn a user-entered display rate back
            into the internal header compounding.
  Param   : inputrate - rate as displayed/entered
  Returns : YieldFromRate(RateFromYield(inputrate, df.c.peryr), h^.peryr). }
{ Go port: internal/finance/interest/rates.go: InterpretedRate (line 119) --
  same inverse composition; takes loan/settings peryr as explicit args. }
  function InterpretedRate(inputrate :real):real; {The inverse of ReportedRate}
  begin
    InterpretedRate:=YieldFromRate(RateFromYield(inputrate,df.c.peryr),h^.peryr);
  end;

{ PerYrString
  Purpose : English name of a payment frequency.
  Params  : peryr - frequency code; capitalization - 0 none, 1 first letter,
            2 all caps
  Returns : "yearly", "monthly", "biweekly", etc., cased as requested. }
{ Go port: internal/finance/interest/rates.go: PerYrString (line 136) -- same
  frequency-code -> English-name table with the 0/1/2 capitalization modes. }
  function PerYrString(peryr :byte; capitalization :byte):str80;
  var ws :str80;
  begin
    case peryr of
       1 : ws:='yearly';
       2 : ws:='semi-annually';
       3 : ws:='thrice-annually';
       4 : ws:='quarterly';
       6 : ws:='bi-monthly';
      12 : ws:='monthly';
      24 : ws:='twice-monthly';
      26 : ws:='biweekly';
      52 : ws:='weekly';
      end;
    if (capitalization=1) then ws[1]:=UpCase(ws[1])
    else if (capitalization=2) then ws:=ToUpper(ws);
    PerYrString:=ws;
  end;

{ ReInterpretRateTable
  Purpose : Re-store every rate in the PVL rate table when the user toggles the
            SIMPLE/COMPOUND interest setting, so the stored continuous rates
            still mean what the new setting expects.
  Effect  : for each row, depending on how the rate was entered (fromyield /
            fromapr), converts between yield and rate using period 1 (simple,
            annual) or df.c.peryr.  'fromrate' rows need no change.
  Side    : mutates the global PVL rate-table cells. }
{ Go port: n/a -- re-stores the stateful PVL rate-table grid on a SIMPLE/COMPOUND
  toggle; the stateless Go API converts rates per-request instead of holding a
  mutable table. (RateFromYield/YieldFromRate themselves are ported in rates.go.) }
  procedure ReInterpretRateTable; {toggle betw SIMPLE and COMPOUND}
    var
      i    :shortint;
  begin
    if (d^.simple) then
      for i:=1 to nlines[PVLRatesBlock] do
        begin
          case c_^[i]^.r.status of
           fromyield : c_^[i]^.r.rate:=YieldFromRate(c_^[i]^.r.rate,1);
             fromapr : c_^[i]^.r.rate:=YieldFromRate(c_^[i]^.r.rate,df.c.peryr);
            {fromrate - do nothing}
          end; {case}
        end {for i}
    else
      for i:=1 to nlines[PVLRatesBlock] do
        begin
          case c_^[i]^.r.status of
           fromyield : c_^[i]^.r.rate:=RateFromYield(c_^[i]^.r.rate,1);
             fromapr : c_^[i]^.r.rate:=RateFromYield(c_^[i]^.r.rate,df.c.peryr);
            {fromrate - do nothing}
          end; {case}
        end {for i}
  end;

{ PercentValueFromCell
  Purpose : Read the percentage-type value stored at a grid cell and convert it
            to the form appropriate for that column and the current settings.
  Params  : block,i,col - locate the cell (block, row i, column col)
            io - (out) receives the cell's input/output status (inp if the
                 value was user-entered, outp if derived, etc.)
  Returns : the column-appropriate percentage:
            * tratecol/lratecol/yieldcol/vratecol/vaprcol : various
              rate<->yield<->APR conversions, with an extra /kicker scaling
              under the 365/360 basis to reproduce that convention.
            * COLAcol/mratecol/inflationcol : annual / 12-period yields.
            * pointscol/pctcol/apointscol : raw stored fraction.
            * aratecol/adjratecol/aaprcol : via ReportedRate (display compounding).
  This is the central translator between stored continuous rates and the many
  rate flavors the various screens display. }
{ Go port: n/a -- reads a value out of the stateful screen-grid data structure
  and picks a display conversion by column id; the web port has no such grid.
  The underlying conversions (YieldFromRate/RateFromYield/ReportedRate) live in
  internal/finance/interest/rates.go. }
  function PercentValueFromCell (block, i, col: shortint; var io: inout): real;
    var
      rp     :realptr;
      sp     :statusptr;
      n      :shortint;
  begin
    sp:=AdvancePointer(blockdata[block]^[i],dataoffset[col]);
    rp:=AdvancePointer(sp,1);
    if (pvlfancy) and (d^.simple) and (col in [tratecol,lratecol,yieldcol]) then begin
       io:=inp;
       if (df.c.basis=x365_360) then
         PercentValueFromCell:= RateFromYield(YieldFromRate(rp^,df.c.peryr)/kicker,df.c.peryr)
       else
         PercentValueFromCell := rp^;
       exit;
       end;
    case col of
      tratecol:
        begin
          if (df.c.basis=x365_360) then
            PercentValueFromCell:= RateFromYield(YieldFromRate(rp^,df.c.peryr)/kicker,df.c.peryr)
          else
            PercentValueFromCell := rp^;
          if (sp^ = fromrate) then
            io := inp
          else io := outp;
        end;
      lratecol:
        begin
          if (df.c.basis=x365_360) then
            PercentValueFromCell:= YieldFromRate(rp^,df.c.peryr)/kicker
          else
            PercentValueFromCell := YieldFromRate(rp^, df.c.peryr);
          if (sp^ = fromapr) then
            io := inp
          else io := outp;
        end;
      yieldcol:
        begin
          if (df.c.basis=x365_360) then
            PercentValueFromCell:= YieldFromRate(RateFromYield(YieldFromRate(rp^,df.c.peryr)/kicker,df.c.peryr),1)
          else
            PercentValueFromCell := YieldFromRate(rp^, 1);
          if (sp^ = fromyield) then
            io := inp
          else io := outp;
        end;
      COLAcol:
        begin
          PercentValueFromCell := YieldFromRate(rp^, 1);
          if (io = defp) then
            io := empty; {no need to display default zeros}
        end;
      vratecol:
        begin
          if (df.c.basis=x365_360) then
            begin
              n:=df.c.peryr; {YieldFromRate is smart about n=canadian, etc.}
              if (df.c.peryr and CANADIANorDAILY =0) and (g[i]^.peryr > 0) then
                n := g[i]^.peryr;
              PercentValueFromCell:= RateFromYield(YieldFromRate(rp^,n)/kicker,n)
            end
          else
            PercentValueFromCell := rp^;
          if (sp^ = fromrate) then
            io := inp
          else io := outp;
        end;
      vaprcol:
        begin
          n:=df.c.peryr; {YieldFromRate is smart about n=canadian, etc.}
          if (df.c.peryr and CANADIANorDAILY =0) and (g[i]^.peryr > 0) then
            n := g[i]^.peryr;
          if (df.c.basis=x365_360) then
            PercentValueFromCell := YieldFromRate(rp^, n) / kicker
          else
            PercentValueFromCell := YieldFromRate(rp^, n);
          if (sp^ = fromapr) then
            io := inp
          else io := outp;
        end;
      mratecol : PercentValueFromCell:=YieldFromRate(rp^, 12);
{$ifdef V_3}
      iratecol : begin
                  if (df.c.basis=x365_360) then
                    PercentValueFromCell:= RateFromYield(YieldFromRate(rp^,df.c.peryr)/kicker,df.c.peryr)
                  else
                    PercentValueFromCell:=rp^;
                  if (sp^=fromcalc) then io:=outp else io:=inp;
                  end;
      inflationcol : begin PercentValueFromCell:=YieldFromRate(rp^, 1); if (sp^=fromcalc) then io:=outp else io:=inp; end;
riratecol,rratecol : begin PercentValueFromCell:=rp^; io:=sp^; end;
{$endif}
      pointscol,pctcol : PercentValueFromCell:=rp^;
      aratecol,adjratecol,aaprcol : begin
                                    if (df.c.basis=x365_360) and (col<>aaprcol) then
                                       PercentValueFromCell:=ReportedRate(rp^)/kicker
                                    else PercentValueFromCell:=ReportedRate(rp^);
                                    end;
      apointscol : PercentValueFromCell:=rp^;
    end; {case}
  end; {PercentValueFromCell}

{$ifdef 0}
no lotus support
procedure WriteOneBlockToLotus(block,first_lotus_row,first_lotus_col :byte);
          var i,col            :byte;
              empty_row        :boolean;
              empty_cell       :array[0..12] of boolean;
              d                :array[0..12] of real;
              format           :char;
              sp               :statusptr;
              dp               :dateptr;
              ip               :integerptr absolute dp;
              rp               :realptr absolute dp;
              dummy            :inout;

          begin
          lotusrow:=first_lotus_row;
          for i:=1 to nlines[block] do begin
              empty_row:=true;
              lotuscol:=first_lotus_col;
              for col:=fcol[block] to lcol[block] do begin
                 sp:=AdvancePointer(blockdata[block]^[i],dataoffset[col]);
                 dp:=AdvancePointer(sp,1);
                 empty_cell[lotuscol]:=(sp^<=0);
                 if not (empty_cell[lotuscol]) then begin
                    case coltype[col] of
                        Date_fmt : d[lotuscol]:=succ(Julian(dp^));
                                    {Lotus doesn't recognize that 1900 wasn't a leap year,
                                     so its Julian numbers after Feb 29, 1900 are off by 1.}
                      string_fmt : d[lotuscol]:=0;
       three_digit_fmt,short_fmt : if (col in peryrcolumns) and (ip^=0) then empty_cell[lotuscol]:=true
                                   else d[lotuscol]:=ip^;
                    currency_fmt : d[lotuscol]:=rp^;
                         pct_fmt : d[lotuscol]:=PercentValueFromCell(block,i,col,dummy);
                      end; {case}
                    empty_row:=false;
                    end;
                 if (col=aasofcol) then inc(lotuscol,2) {special case}
                 else inc(lotuscol);
                 end; {for col}
              lotuscol:=first_lotus_col;
              if (not empty_row) then
                for col:=fcol[block] to lcol[block] do
                  if (not empty_cell[lotuscol]) then begin
                    case coltype[col] of
                     date_fmt : format:=lotus_date_fmt;
                     pct_fmt  : format:=lotus_percent_fmt;
    three_digit_fmt,short_fmt : format:=lotus_short_fmt;
                       {Overlap column (shortset and percent set) should be percent format}
                    else if (df.h.commas) then format:=lotus_comma_fmt
                         else format:=lotus_real_fmt;
                       end;  {case}
                    if (coltype[col]=string_fmt) then
                      WriteOneStringCell(false,'^',lotusrow,lotuscol,GetString(i+ztop[block]-scrollpos[block],col))
                    else WriteOneCell(format,d[lotuscol]);
                    {lotuscol is incremented in WriteOneCell}
                    if (col=aasofcol) then inc(lotuscol); {special case}
                    end
                  else {if nothing to print}
                    if (col=aasofcol) then inc(lotuscol,2)  {special case}
                    else inc(lotuscol);
              inc(lotusrow);
              end; {for i}
          end; {WriteOneBlockToLotus}

procedure WK1File(filename :str80);
          var ext              :str3;
              maxrow,saverow   :byte;
          begin
          abort:=false;
          ext:=ScreenExt(thisrun);
          ext:='%'+ext[1];
          if not PrepareLotusFile(impath+'TEMPLATE.'+ext,filename) then begin
             message('Cannot find file "'+impath+'TEMPLATE.'+ext+'" needed for WK1 output.');
             abort:=true;
             exit;
             end;
          case thisrun of
            ipvl : begin
               WriteOneBlockToLotus(PVLLumpSumBlock,3,0);
               maxrow:=lotusrow;
               WriteOneBlockToLotus(PVLPeriodicBlock,3,3);
               if (maxrow>lotusrow) then lotusrow:=maxrow;
               WritePVLHeadersToLotus;
               if (pvlfancy) then begin
                  saverow:=lotusrow;
                  WriteOneBlockToLotus(PVLRatesBlock,lotusrow,0);
                  maxrow:=lotusrow;
                  WriteOneBlockToLotus(PVLExtraBlock,saverow,6);
                  if (lotusrow<maxrow) then lotusrow:=maxrow;
                  end
               else begin
                  WriteOneBlockToLotus(PVLPresValBlock,lotusrow,0);
                  end;
               end;
{$ifdef V_3}
            irbt : WriteOneBlockToLotus(RBTTopBlock,3,0);
{$endif}
            imtg, ichr : WriteOneBlockToLotus(block,3,0);
            iamz : begin
               WriteOneBlockToLotus(AMZTopBlock,2,0);
               if (fancy) then begin
                 WriteOneBlockToLotus(AMZPreBlock,3,3);
                 WriteOneBlockToLotus(AMZBalloonBlock,8,0);
                 maxrow:=lotusrow;
                 WriteOneBlockToLotus(AMZChangesBlock,8,3);
                 if (lotusrow<maxrow) then lotusrow:=maxrow;
                 saverow:=lotusrow+2;
                 WriteOneBlockToLotus(AMZMoratoriumBlock,saverow,1);
                 dec(lotusrow,3);
                 end
               else saverow:=5;
               WriteAMZHeadersToLotus(succ(lotusrow));
               WriteOneBlockToLotus(AMZTargetBlock,saverow,3);
               WriteOneBlockToLotus(AMZAsOfBlock,saverow,5);
               end;
{$ifdef V_3}
            iinv : begin
                   WriteOneBlockToLotus(INVDLumpSumBlock,3,0);
                   WriteOneBlockToLotus(INVDPeriodicBlock,3,3);
                   WriteOneBlockToLotus(INVWLumpSumBlock,13,0);
                   WriteOneBlockToLotus(INVWPeriodicBlock,13,3);
                   lotusrow:=21; lotuscol:=1;
                   WriteOneCell(lotus_percent_fmt,u^.irr.rate);
                   inc(lotuscol,2); {add 3, really - one is already in WriteOneCell}
                   WriteOneCell(lotus_percent_fmt,YieldFromRate(u^.iir.rate,1));
                   lotuscol:=7;
                   WriteOneCell(lotus_date_fmt,succ(Julian(u^.constdate)));
                   end;
{$endif}
            end; {case}
          blockwrite(lotusfile,CODA,4);
          close(lotusfile);
          abort:=(MIOResult<>0);
          end;
{$endif}

{ EmptyScreen
  Purpose : Is the given screen completely empty (no data entered anywhere)?
  Param   : which - screen id
  Returns : true if every block of the screen has zero data lines.  The trailing
            PVL present-value (and INV rates) block is excluded when it holds
            only its single default row, so a fresh screen still counts empty. }
{ Go port: n/a -- tests the stateful DOS screen blocks for emptiness; the
  stateless Go API instead classifies row status per request (StatusEmpty). }
function EmptyScreen(which :byte):boolean;
         var theresult           :boolean;
             bx,last          :byte;
         begin
         theresult:=true;  bx:=fblock(which); last:=lblock(which);
{$ifdef V_3}
         if ((last=PVLPresValBlock) or (last=INVRatesBlock)) and (nlines[last]=1) then dec (last);
{$else}
         if (last=PVLPresValBlock) and (nlines[last]=1) then dec (last);
{$endif}
         while (theresult) and (bx<=last) do begin
            if (nlines[bx]>0) then theresult:=false;
            inc(bx);
            end;
         EmptyScreen:=theresult;
         end;

{ UniversalScreenProc
  Purpose : Repaint the current screen by dispatching to that screen's
            registered ScreenProc, passing the right "fancy" flag (PVL uses its
            own PVLFancy under $ifdef PVLX; others use the global 'fancy').
  Side    : UI rendering only. }
{ Go port: n/a -- dispatches to a per-screen repaint routine; DOS UI rendering. }
procedure UniversalScreenProc;
          var fancybyte :^byte;
          begin
{$ifdef CHEAP}
          ScreenProc[thisrun](0);
{$else not CHEAP}
{$ifdef PVLX}
          if (thisrun=iPVL) then fancybyte:=@PVLFancy
          else fancybyte:=@fancy;
          ScreenProc[thisrun](fancybyte^);
{$else}
          ScreenProc[thisrun](byte(fancy));
{$endif PVLX}
{$endif not CHEAP}
          end;

{ Init (unit initializer)
  Purpose : Reset per-session state when the unit comes up: clear the 'fancy'
            mode (non-CHEAP builds) and establish the day-count basis via
            SetYrDays.  Called from the unit's begin..end block below. }
{ Go port: n/a -- unit-load session reset (clears global 'fancy', calls
  SetYrDays); the Go port has no global session state to reset, and the yrdays
  setup is done per-request by NewCalcContext (rates.go:21). }
procedure Init;
          begin
//          shiftstatus:=shiftstatus and 127; {Ins off}
//          SetCursorSize;
          {$ifndef CHEAP} fancy:=false; {$endif}
          SetYrDays;
          end;

{ counter.Incr
  Purpose : Pre-increment this counter's screencount and return the new value. }
{ Go port: n/a -- a DOS screen-instance tally object; no web-port equivalent. }
function counter.Incr:shortint;
         begin
         inc(screencount);
         Incr:=screencount;
         end;


{ Unit initialization: run Init once when the program loads INTSUTIL. }
begin
Init
end.
