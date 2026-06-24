{ ==========================================================================
  VIDEODAT.pas

  PURPOSE / ROLE
    Originally the DOS screen/video and calendar data unit. After the Windows
    port most of the direct-video and BIOS-interrupt machinery is commented out
    (the UI is now Delphi-drawn), so what remains live is the unit's CALENDAR
    ENGINE: the application's proleptic Julian day-number arithmetic and date
    record parsing/formatting that the financial logic depends on.

    The legacy screen model is still described by the (now mostly inert)
    globals: an 80x25 text screen (`screen: ^screenblock`), color/monochrome
    attribute selection, and CGA/MDA video segment addresses ($B800/$B000).
    Those reflect the original IBM PC codepage-437 text mode.

  DATE MODEL
    A daterec stores y = years since 1900, m = 1..12, d = 1..31, with negative
    m values acting as error sentinels (errorbyte=-99, plus -88 in Julian).
    Julian() maps a daterec to a day number using a 4-year (1461-day) cycle and
    a precomputed "days before month" table (leap/non-leap variants), and MDY()
    is its inverse. SetNow seeds `now`/`jnow` from the system clock.

  KEY CONSTANTS
    fouryears=1461, twenty_years=7305  - day counts of the Julian cycle.
    centurydiv=50    - 2-digit-year pivot: yy<50 => 20xx, else 19xx.
    mon[], monstr[]  - month abbreviations and full names.
    earliest/latest  - representable date bounds (mirrors Globals).
    video=$B800/$B000, color/colorcard, revattr - legacy text-mode video state.
  ========================================================================== }
unit VIDEODAT;

INTERFACE
//uses OPCRT,DOS;
uses Globals, DateUtils;

{ moving to Globals to avoid circular refrences.
  James
type
     daterec=record   d,m:shortint; y : byte;   end;
     str8=string[8];
     ch2=array[1..2] of char;
     ch3=array[1..3] of char;
     str6=string[6];
     str3=string[3];
     ch12=array[1..12] of char;
     w13=array[1..13] of word;
     shortline=array[1..60] of char;
     screenline = array[1..80] of ch2;
     screenblock = array[1..25] of screenline;
        }
const
      // fouryears = days in a 4-year Julian cycle (3*365+366); twenty_years = 5 cycles.
      // sl = the date separator '/'; errorbyte = sentinel month value for a bad date.
      fouryears=1461; twenty_years=7305;   sl:char='/';  errorbyte=-99;

      // centurydiv: 2-digit-year cutoff. y < 50 is treated as 20xx (+100 offset),
      // otherwise 19xx. Typed constant so it could in principle be changed.
      centurydiv :byte=50;
      // mon: 3-letter month abbreviations, indexed 1..12.
      mon:array[1..12] of ch3 = ('Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec');
      // monstr: full month names. NOTE index 0 = 'December' so a 0/wrap month
      // still names the prior December; 1..12 are January..December.
      monstr:array[0..12] of string[9]=('December','January','February','March','April','May','June',
                                           'July','August','September','October','November','December');
      // Representable date bounds (mirror Globals); latest doubles as the
      // open-ended "...." sentinel.
      earliest:daterec=(d:1;m:1;y:0);
      latest  :daterec=(d:1;m:12;y:249);
var
     // ---- Legacy text-mode video state (mostly inert under the Delphi UI) ----
     video                        :word;   // video memory segment: $B800 color / $B000 mono
//     shiftstatus                  :byte absolute 0000:1047;
       {1=RShift  2=LShift  4=Ctrl  8=Alt  16=ScrollLock  32=Num Lock  64=Caps Lock  128=Insert state}
     bottomcursor                 :byte;          // scan line of the text cursor bottom (CGA cursor sizing)
     date                         :ch12;          // formatted "Mmm DD, YYYY" today string
     screen                       :^screenblock;  // pointer to video memory as an 80x25 cell grid
     color                        :boolean;       // true if running in color text mode
     colorcard                    :boolean;       // true if a color video card is present
     revattr                      :char;          // reverse-video attribute selector
{$ifdef CTR_SCR}
     ctrscr,savctr                :array[0..7] of ^shortline;
     ctrword                      :array[0..7] of longint absolute ctrscr;
{$endif}
     now                          :daterec;       // today's date (see SetNow)
     jnow                         :longint;       // Julian day number of `now`
     leapyear                     :boolean;       // set by DecideAboutFeb29 for the current year
     saveexit                     :pointer;       // saved Turbo Pascal exit-proc chain pointer
     mode                         :byte;          // text video mode constant (MONO/CO80)
//     currentmode                  :byte absolute 0000:$449;

// Clamp f.d down to the last valid day of its month.
procedure CheckForDaysTooLarge(var f :daterec);
//procedure DetermineDateAndVideoMode;
//procedure GetXY(var xgo,ygo :byte);
//procedure AbsXY(var xgo,ygo :byte);
{procedure HideCursor;}
// Carry over-/under-flowing months into years so 1<=m<=12 (by-value; see note).
procedure Normalize(x :daterec);
// Convert a Julian day number back into a daterec (inverse of Julian).
procedure MDY(daynumber:longint; var x :daterec); {Compute M,D,Y from Julian}
// Set `now`/`jnow` from the system clock (year stored as year-1950, see note).
procedure SetNow;
//procedure SetCursorSize;
// Parse a date string into f via EvalDateStr (ignores the boolean result).
procedure Str2MDY(datestr :str8; var f :daterec);
// Advance a pointer by x bytes (post-port: plain pointer arithmetic).
function AdvancePointer(p :pointer; x :integer):pointer;
// Format a date as a 6-char "YYMMDD" string.
function Date6(x :daterec):str6;
// True if f looks like a valid date (1<=m<=12).
function dateok(f :daterec):boolean;
// Format a date as "M/D/YY" (or "  ....  " for the open-ended latest date).
function DateStr(x :daterec):str8;
// Number of days in the month of f (leap-aware for February).
function DaysInM(f :daterec):byte;
// True if two far pointers resolve to the same linear address.
function EquivalentAddresses(a1, a2 :pointer):boolean;
// Parse a date string into f; returns false (and f.m=errorbyte) on bad input.
function EvalDateStr(datestr :str8; var f :daterec):boolean;
// Convert a daterec to its Julian day number.
function Julian(x:daterec):longint;
// Return s uppercased.
function ToUpper(s:string):string;
//function wleft:byte;
//function wtop:byte;
//function EquipmentList:word;
{$ifdef BugsIn}
procedure FreeMem(p :pointer; siz :word);
{$endif}

IMPLEMENTATION

// const   daysin:array[0..13] of byte=(31,31,28,31,30,31,30,31,31,30,31,30,31,31);

var
  // daysin[m] = days in month m. Indices 0 and 13 are guard/wrap entries; index
  // 2 (February) is patched between 28 and 29 by DecideAboutFeb29 per year.
  daysin:array[0..13] of byte=(31,31,28,31,30,31,30,31,31,30,31,30,31,31);
  // daysbefore -> cumulative days before month m; repointed at the leap or
  // non-leap table by DecideAboutFeb29.
  daysbefore                             :^w13;
  // Precomputed cumulative "days before month" tables for leap and common years
  // (filled in the unit's initialization section). Entry 13 = maxword sentinel.
  leapdaysbefore,notleapdaysbefore       :w13;
      {
procedure EMessage(s :pathstr; x :integer);
          var xs     :str8;
              attr   :byte;
              line25 :ScreenLine;
          begin
          str(x,xs);
          MoveScreen(screen^[25],line25,80);
          if (color) then attr:=red shl 4 + white else attr:=112;
          FastWrite(s+xs,25,1,attr);
          while KeyPressed do ReadKey;
          ReadKey;
          MoveScreen(line25,screen^[25],80);
          end;
       }
{ EquivalentAddresses
  PURPOSE: compare two real-mode (segment:offset) far pointers for pointing at
           the same physical byte, despite different seg:ofs encodings.
  PARAMS:  a1, a2 - the pointers (read via absolute longints).
  RETURNS: true if their normalized 20-bit linear addresses match.
  INTENT:  DOS-era pointer-aliasing sanity check; largely vestigial post-port. }
function EquivalentAddresses(a1, a2 :pointer):boolean;
         var x1    :longint absolute a1;
             x2    :longint absolute a2;
             c1,c2 :longint;
         begin
         c1:=($FFFF0 and (x1 shr 12)) + (x1 and $0000FFFF);
         c2:=($FFFF0 and (x2 shr 12)) + (x2 and $0000FFFF);
         EquivalentAddresses:=(c1=c2);
         end;

{$ifdef BUGSIN}
procedure FreeMem(p :pointer; siz :word);
          var px      :longint absolute p;
          begin
          if ((px and $FF00)<>0) then
             EMessage('Suspicious pointer passed to FreeMem: ofs=',px and $FFFF);
          SYSTEM.FreeMem(p,siz);
          end;

function AdvancePointer(p :pointer; x :integer):pointer;
         var px         :longint absolute p;
             result     :pointer;
             resultx    :longint absolute result;
             oldresult  :pointer;
             oldresultx :longint absolute oldresult;
             c          :longint;
         begin
         if ((px and $F000)<>0) then
            EMessage('Suspicious pointer passed to AdvancePointer: ofs=',px and $FF00);
         oldresultx:=px + x;
         c:=($FFFF0 and (px shr 12)) + (px and $0000FFFF);
         c:=c+x;
         resultx:= ((c and $FFFF0) shl 12) + (c and $0000000F);
         if not (EquivalentAddresses(result,oldresult)) then EMessage('Old AdvancePointer would have gotten it wrong: x=',x);
         AdvancePointer:=result;
         end;
{$else}
{
function AdvancePointer(p :pointer; x :integer):pointer;
         var px         :longint absolute p;
             theresult     :pointer;
             resultx    :longint absolute theresult;
             c          :longint;
         begin
         c:=($FFFF0 and (px shr 12)) + (px and $0000FFFF);
         c:=c+x;
         resultx:= ((c and $FFFF0) shl 12) + (c and $0000000F);
         AdvancePointer:=theresult;
         end;
}
{ AdvancePointer (active, non-BUGSIN build)
  PURPOSE: return p advanced by x bytes.
  PARAMS:  p - base pointer; x - signed byte offset.
  RETURNS: p+x. Post-port this is plain flat-model pointer arithmetic; the
           BUGSIN/disabled variants above did real-mode segment normalization. }
function AdvancePointer(p :pointer; x :integer):pointer;
         var px      :longint absolute p;
             theresult  :pointer;
             resultx :longint absolute theresult;
         begin
         resultx:=px + x;
         AdvancePointer:=theresult;
         end;

{$endif}
     {
procedure SetCursorSize;
          begin
          if (shiftstatus and 128 = 128) then BlockCursor
          else NormalCursor;
          end;
      }
(*
procedure SetCursorSize;
          var top,bottom  :byte;
          var regs :registers;
          begin with regs do begin
          CL:=bottomcursor;
          if (shiftstatus and 128 = 128) then CH:=1 else CH:=pred(bottomcursor);
          AH:=1; BH:=0; Intr(16,regs);
          end;end;
*)         {
procedure GetXY(var xgo,ygo :byte);
          begin
          xgo:=wherex; ygo:=wherey;
          end;

procedure AbsXY(var xgo,ygo :byte);
          var regs :registers;
          begin with regs do begin
          AH:=3; BH:=0; Intr(16,regs);
          xgo:=succ(DL); ygo:=succ(DH);
          end;end;

function wleft:byte;
         var x,y,x1,y1 :byte;
         //left edge of current window
         begin
         GetXY(x,y); AbsXY(x1,y1);
         wleft:=succ(x1-x);
         end;

function wtop:byte;
         var x,y,x1,y1 :byte;
         //left edge of current window
         begin
         GetXY(x,y); AbsXY(x1,y1);
         wtop:=succ(y1-y);
         end;

procedure GetCursorBottom;
          var regs :registers;
          begin
          if (color) then with regs do begin
          {color at this point means exactly a color video card, since
           the command line parameters haven't been read yet.
             AH:=3; BH:=0; Intr(16,regs);
             bottomcursor:=CL;
             end
          else bottomcursor:=13;
          end;

procedure HideCursor;
          var regs :registers;
          begin with regs do begin
          AH:=1; BH:=0; CH:=32; {CH=32  makes cursor disappear
          Intr(16,regs);
          end;end;
            }
{ Date6
  PURPOSE: format a date into a fixed 6-char "YYMMDD" string (zero-padded).
  PARAMS:  x - the date (y is the 1900-offset year; only its mod-100 part used).
  RETURNS: a str6 with two digits each of year, month, day.
  NOTE: writes directly into the function-result short string Date6[1..6]. }
function Date6(x :daterec):str6;
         var ws :str3;
         begin
         str(x.y mod 100:2,ws);  if ws[1]=' ' then ws[1]:='0';   // YY, leading-zero padded
         date6[1]:=ws[1]; date6[2]:=ws[2];
         str(x.m:2,ws); if ws[1]=' ' then ws[1]:='0';
         date6[3]:=ws[1]; date6[4]:=ws[2];
         str(x.d:2,ws); if ws[1]=' ' then ws[1]:='0';
         date6[5]:=ws[1]; date6[6]:=ws[2];
         date6[0]:=#6;
         end;

{ ToUpper
  PURPOSE: return an uppercased copy of s.
  PARAMS:  s - input string.
  RETURNS: s with each char passed through upcase; length preserved. }
function ToUpper(s:string):string;
         var i:byte;
             RetVal: string;
         begin
         for i:=1 to length(s) do RetVal[i]:=upcase(s[i]);
         SetLength( RetVal, length(s) );
         ToUpper := RetVal;
         end;

{$ifdef 0}
procedure DetermineDateAndVideoMode;  {Date From TURBO DOSFCALL.DOC file }

    procedure SetMono;
              begin
              mode:=MONO;
              video:=$B000;
              color:=false;
              revattr:='p';
              checksnow:=false;
              end;

    procedure SetColor;
              begin
              mode:=CO80;
              video:=$B800;
              color:=true;
              revattr:='p';
              checksnow:=(CurrentDisplay=CGA);
              end;

     function UsingColorCard:boolean;
              var  ki   :char;
              begin
              if (toupper(paramstr(1))='COLOR') then UsingColorCard:=true
              else if (toupper(paramstr(1))='MONO') then UsingColorCard:=false
              else case lastmode of
 FONT8X8,BW40,BW80,CO80,CO40 : UsingColorCard:=true;
                   MONO      : UsingColorCard:=false;
                   else begin
                        clrscr;
                        write('Are you using a color video card? ');
                        repeat ki:=upcase(readkey) until (ki in ['Y','N']);
                        UsingColorCard:=(ki='Y');
                        end;
                   end;
              end;


          var  regs           :registers;
               j              :byte;
               ws             :string[16];

          begin
          with regs do begin
               ax := $2A00;
               MsDos(regs);
               thisyear:=cx mod 100;
               today:=dx mod 256;
               thismonth:=dx shr 8;
               end;
          for j:=1 to 3 do date[j]:=mon[thismonth,j];
          date[4]:=' ';
          str(today:2,ws);  for j:=1 to 2 do date[j+4]:=ws[j];
          date[7]:=','; date[8]:=' ';date[9]:='1'; date[10]:='9';
          str(thisyear:2,ws); for j:=1 to 2 do date[j+10]:=ws[j];
         {writeln(date);
          write(thisyear,'/',thismonth,'/',today); halt;}

          {Now determine video mode}
(*
          Intr($11,regs);
          with regs do
              if ((al and $30) = $30) then SetMono else SetColor;
*)
          colorcard:=UsingColorCard;
          if colorcard then SetColor else SetMono;
          screen:=ptr(video,0);
{$ifdef CTR_SCR}
          ctrscr[0]:=ptr(video,1496);
          for j:=1 to 7 do ctrword[j]:=ctrword[j-1]+160;
{$endif}
          end;
{$endif}

{ SetNow
  PURPOSE: initialize the global `now` date and its Julian number `jnow` from
           the system clock. Called from the unit initialization.
  SIDE EFFECTS: writes globals now and jnow.
  NOTE: here now.y is stored as (year - 1950), which differs from the 1900-offset
        convention used by daterec elsewhere (e.g. earliest/latest and
        StringFormat2Date). // TODO: verify logic - this 1950 offset looks
        inconsistent with the unit-wide 1900 base. }
procedure SetNow;
          var
          CurrentDate: TDateTime;
          begin
          CurrentDate := Today();
          now.y:=YearOf( CurrentDate )-1950;
          now.d:=DayOf( CurrentDate );
          now.m:=MonthOf( CurrentDate );
          jnow:=Julian(now);
          end;

{ DecideAboutFeb29 (private)
  PURPOSE: configure the day tables for a given year as leap or common.
  PARAMS:  wy - the year (1900-offset). Treated as leap when divisible by 4
           and > 0.
  SIDE EFFECTS: sets daysin[2] to 28/29, the leapyear flag, and repoints
           daysbefore at the matching cumulative table.
  NOTE: uses the simple "divisible by 4" rule - no 100/400 century correction
        (acceptable within the limited 1900..2149 representable range). }
procedure DecideAboutFeb29(wy :byte);
          begin
          if (wy mod 4 = 0) and (wy>0) then begin
               daysin[2]:=29; leapyear:=true;  daysbefore:=@leapdaysbefore end
          else begin
               daysin[2]:=28; leapyear:=false; daysbefore:=@notleapdaysbefore; end;
          end;

{ DaysInM
  PURPOSE: number of days in the month given by f.
  PARAMS:  f - a date (only m and y are used).
  RETURNS: 28/29 for February (leap rule on y), daysin[m] for 1..12, else 30
           as a safe fallback to avoid range-check errors on a bad month. }
function DaysInM(f :daterec):byte;
         begin with f do begin
         if (m=2) then begin
            if (y mod 4 = 0) then daysinm:=29 else daysinm:=28;
            end
         else if (m<=12) and (m>=1) then DaysInM:=daysin[m]
         else DaysInM:=30; {just avoiding range check errors}
         end;end;

{ CheckForDaysTooLarge
  PURPOSE: clamp a day-of-month that overshoots the month length (e.g. Feb 31
           becomes Feb 28/29).
  PARAMS:  f (var) - date; f.d reduced to the month's last valid day if needed.
  SIDE EFFECTS: may modify f.d. }
procedure CheckForDaysTooLarge(var f :daterec);
          var last :byte;
          begin
          last:=DaysInM(f);
          if (f.d>last) then f.d:=last;
          end;

{ Julian
  PURPOSE: convert a daterec into a serial day number used as the engine's
           internal time axis (and by date-difference / period math).
  PARAMS:  x - the date.
  RETURNS: the day number, or -88 (after an EMessage) for an out-of-range month.
  INTENT:  cycle-based formula: floor((1461*y - 1)/4) gives days through the end
           of the prior year (4-year Julian cycle), plus days-before-month and
           the day-of-month. Sets up the leap tables via DecideAboutFeb29 first.
  NOTE: integer div by 4 must be preserved exactly for date-arithmetic parity. }
function Julian(x:daterec):longint;
         var daynumber   :longint;
         begin with x do begin
         DecideAboutFeb29(y);
         if (m>13) or (m<1) then begin
            EMessage('Bad date passed to Julian function: m=',m);
            daynumber:=-88;
            end
         else daynumber:=(fouryears * longint(y)-1) div 4 + daysbefore^[m] + d;
         Julian:=daynumber;
         end; end;

{ MDY
  PURPOSE: inverse of Julian - decompose a day number into y, m, d.
  PARAMS:  daynumber - serial day number; x (var) - filled with the date.
  RETURNS: via x; on an out-of-range daynumber sets x.m:=errorbyte and exits.
  INTENT:  recover the year from the 4-year cycle (fourx = daynumber*4), then
           the day-of-year, then bracket the month using the days-before table
           (first by quarter via [4],[7],[10], then a linear scan), and finally
           the day-of-month. The shl 2 / shr 2 are the *4 and /4 of the cycle. }
procedure MDY(daynumber:longint; var x :daterec); {Compute M,D,Y from Julian}
         var  days  :integer;
              fourx :longint;

         begin
         if (daynumber<0) or (daynumber>70000) then begin x.m:=errorbyte; exit; end;
         fourx:=daynumber shl 2;                          // daynumber * 4
         x.y:= (fourx div fouryears);                     // year from the 4-year cycle
         DecideAboutFeb29(x.y);
         days:=succ((fourx-longint(x.y)*fouryears) shr 2);// 1-based day-of-year
         // narrow the month to a quarter, then scan for the exact month
         if (days<=daysbefore^[7]) then begin
            if (days<=daysbefore^[4]) then x.m:=1 else x.m:=4; end
         else begin
            if (days<=daysbefore^[10]) then x.m:=7 else x.m:=10; end;
         repeat inc(x.m) until (daysbefore^[x.m]>=days); dec(x.m);  // first month whose start passes `days`
         x.d:=days-daysbefore^[x.m];                       // remaining days = day-of-month
(*
write(x.m:2,'/',x.d:2,'/',x.y:2,'   ',daynumber);
if (Julian(x)=daynumber) then write('  ') else begin writeln(' .NE. ',Julian(x),'  '); while not keypressed do; end;
*)
         end;

{ Normalize
  PURPOSE: roll an out-of-range month into the year so 1<=m<=12, and refresh
           the leap-year tables.
  PARAMS:  x - date (passed BY VALUE).
  SIDE EFFECTS: updates the global leap tables via DecideAboutFeb29.
  NOTE: x is by value, so the normalized y/m are NOT propagated back to the
        caller; the day-normalization that would also clamp d is commented out.
        Effectively this only reinitializes the leap tables for x.y as a side
        effect. // TODO: verify logic - likely intended a var parameter. }
procedure Normalize(x :daterec);
          begin with x do begin
          while (m>12) do begin inc(y); m:=m-12; end;
          while (m<1)  do begin dec(y); m:=m+12; end;
          DecideAboutFeb29(y);
(*
          if (m>13) or (m<1) then begin write('RANGE ERROR in NORMALIZE: m=',m); readln; end;
          while (d>daysin[m]) do begin d:=d-daysin[m]; inc(m); end;
          while (d<1) do begin dec(m); d:=d+daysin[m]; end;
*)
          end; end;

(*
function DateStr(daynumber:longint):str8;
         var  ws:str80; ls:string[3];
              x:daterec;
         begin
         MDY(daynumber,x);
         str(x.m:2,ws);
         str(x.d:2,ls);  ws:=ws+sl+ls;
         str(x.y mod 100,ls); if (ls[0]=#1) then ls:='0'+ls; ws:=ws+sl+ls;
         if (ws[0]<#8) then writeln('How did I come out shorter than 8 in DateStr?');
         Datestr:=ws;
         end;
*)
{ DateStr
  PURPOSE: format a date as a fixed 8-char "MM/DD/YY" string for table output.
  PARAMS:  x - the date.
  RETURNS: "M/D/YY" with year shown mod 100 (zero-padded), or "  ....  " when
           x is the open-ended `latest` sentinel year. }
function DateStr(x :daterec):str8;
         var  ws:pathstr; ls:string[3];
         begin
         str(x.m:2,ws);
         str(x.d:2,ls);  ws:=ws+sl+ls;
         str(x.y mod 100,ls); if (ls[0]=#1) then ls:='0'+ls; ws:=ws+sl+ls;  // 2-digit year, leading zero
         Datestr:=ws;
         if (x.y=latest.y) then DateStr:='  ....  ';  // open-ended date shows as dots
         end;

{ dateok
  PURPOSE: cheap validity test for a daterec.
  PARAMS:  f - the date.
  RETURNS: true iff 1<=f.m<=12. Error dates carry m=-99 or m=-88, so a positive
           in-range month is taken as "ok". }
function dateok(f :daterec):boolean;
         begin
         if (f.m>0) and (f.m<13) then dateok:=true else dateok:=false;
         {More conservative than necessary.  By my definition, dates
          that are not ok are either m=-99 or m=-88}
         end;

{ EvalDateStr
  PURPOSE: parse a "MM/DD/YY"-style date string into a daterec.
  PARAMS:  datestr - the input; f (var) - the parsed date, or f.m=errorbyte.
  RETURNS: true if parsed and in range; false (and f.m=errorbyte) otherwise.
  INTENT:  tokenizes up to three numeric fields separated by non-digits; applies
           the centurydiv pivot to the 2-digit year (yy<50 => +100), then range-
           checks month and day-of-month (leap-aware). The literal "..." input
           maps to the open-ended `latest` date.
  NOTE: nested helpers Value2 (digit->number) and NextBreak (advance to next
        field) drive the scan via the shared p cursor and substr buffer. }
function EvalDateStr(datestr :str8; var f :daterec):boolean;
          var substr :string[2];
              p      :byte;
          label BAD_DATE;

    { Value2: convert the 1-2 digit chars in `substr` to a number; on a length
      other than 1 or 2 it flags the date bad (f.m:=errorbyte). }
    function Value2:shortint;
             var len    :byte absolute substr;
                 Theresult :shortint;
             begin
             Theresult:=ord(substr[1])-48;
             if (len=2) then Theresult:=Theresult*10 +  ord(substr[2])-48
             else if (len<>1) then begin Theresult:=0; f.m:=errorbyte; end;
             Value2:=Theresult;
             end;

    { NextBreak: scan datestr from cursor p, collecting up to ~3 digit chars
      into substr until a non-digit separator (or end of field) is hit, then
      skip trailing spaces. Returns true if a separator was found. }
   function NextBreak:boolean;
            const numberset=['0'..'9'];
            var i      :byte;
                Theresult :boolean;
            begin
            i:=0; substr:='';
            if (p>=8) then begin NextBreak:=false; exit; end;
            repeat
              inc(p); inc(i);
              Theresult:=not (datestr[p] in numberset);
              if (not Theresult) then substr:=substr+datestr[p];
            until ((Theresult) and (i>1)) or (i=3) or (p>=8);
            while (datestr[p]=' ') and (p<length(datestr)) do inc(p);
            NextBreak:=Theresult;
            end;

          begin with f do begin
          EvalDateStr:=true;
          if (pos('...',datestr)>0) then begin f:=latest; exit; end;
          if (length(datestr)<6) then goto BAD_DATE;
          p:=0;
          if not (NextBreak) then goto BAD_DATE;
          m:=Value2; if (m=0) or (m>12) then goto BAD_DATE;
if (m<0) then begin write('datestr="'+datestr+'".  BREAK OUT!'); readln; end;
          if (not NextBreak) then goto BAD_DATE;
          d:=Value2; if (m<=0) or (m>12) then goto BAD_DATE;
          if (NextBreak) then;
          y:=Value2; if (m<=0) or (m>12) then goto BAD_DATE;
          if (y<centurydiv) then y:=y+100;
          DecideAboutFeb29(y);
          if (d=0) or (d>daysin[m]) then goto BAD_DATE;
          exit;
BAD_DATE: f.m:=ERRORBYTE;
          EvalDateStr:=false;
          end; end;

{ Str2MDY
  PURPOSE: parse a date string into f, ignoring the success flag.
  PARAMS:  datestr - input; f (var) - parsed date (f.m=errorbyte if invalid).
  NOTE: thin wrapper over EvalDateStr; the boolean result is discarded. }
procedure Str2MDY(datestr :str8; var f :daterec);
          var ta :boolean;
          begin
          ta:=EvalDateStr(datestr,f);
          end;
{
I'm not sure what it does, but anything calling interupts and reading
the registers directly can't be good.
function EquipmentList:word;
          //From Norton, p 219
         var regs :registers;
         begin
         Intr(17,regs);
         EquipmentList:=regs.AX;
         end;
 }
{ Unit initialization
  - Save the current exit-proc chain (legacy DOS housekeeping).
  - Build the cumulative "days before month" tables:
      notleapdaysbefore[m] = sum of days in months 1..m-1 (common year);
      leapdaysbefore is the same but +1 from March on (the leap day);
      entry [13] is a maxword sentinel so scans terminate.
  - Seed today's date via SetNow. (Video-mode/cursor setup is disabled in the
    Windows port.) }
var i:byte;
const maxword = 65535;
begin
saveexit:=exitproc;
notleapdaysbefore[1]:=0; for i:=2 to 12 do notleapdaysbefore[i]:=notleapdaysbefore[pred(i)]+daysin[pred(i)];
notleapdaysbefore[13]:=maxword;
for i:=1 to 2 do leapdaysbefore[i]:=notleapdaysbefore[i];
for i:=3 to 12 do leapdaysbefore[i]:=succ(notleapdaysbefore[i]);
leapdaysbefore[13]:=maxword;
//DetermineDateAndVideoMode;
//GetCursorBottom;
SetNow;
end.
