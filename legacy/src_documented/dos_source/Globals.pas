{ ==========================================================================
  Globals.pas

  PURPOSE / ROLE
    Central grab-bag unit for the Per%Sense application.  It hosts the shared
    constants, fundamental types, and low-level string/number/date conversion
    helpers that nearly every other unit relies on.  Historically these lived
    in petypes.pas, Input.pas, Videodat.pas and Northwnd.pas; they were pulled
    up into Globals to break circular unit references during the Delphi/Windows
    port of the original DOS code base.

  KEY TYPES
    daterec   - the application's compact date record (d,m: shortint; y: byte),
                shared with VIDEODAT.  y is years since 1900 (see latest/earliest).
    NumberFormat - selects display precision for Double2StringFormat.
    str40/str80, str3/str6/str8, ch2/ch3/ch12, screenline/screenblock -
                fixed-length string/char/array aliases used for the legacy
                80x25 text-screen model and field formatting.

  KEY HELPERS
    - Message-box wrappers that delegate to the Delphi MessageDialog plus the
      HelpSystem (status/help codes).
    - String<->number<->date converters that accept user-entered, comma- and
      fraction-formatted text (e.g. "3 3/4", "1,234.50").
    - ftoa2/ftoa4/value/Evaluate: the original DOS-era numeric formatting and
      free-form expression evaluators, preserved for output fidelity.
  ========================================================================== }
unit Globals;

interface

uses
  SysUtils, DateUtils;

const
  // Constants for Message dialogs
  // Return codes echoed back from the message-box helpers.
  DLG_YES         = 6;     // user chose "Yes"
  DLG_CANCEL      = 2;     // user chose "Cancel"
  // NumberFormat types
  // Selector values passed to Double2StringFormat to pick display precision.
  DoubleDotTwo    = 1;     // fixed-point, 2 decimals (currency)
  DoubleDotFour   = 2;     // fixed-point, 4 decimals (rates)
  Int             = 3;     // integer, no decimals

  DateCellLength  = 10;    // width of a date input cell ("MM/DD/YYYY")

// This came from petypes.pas
// James sez - again, I don't want to include INPUT so I'm
// setting this to the value from that file.
//    tiny :real= INPUT.tiny;
    // tiny: convergence/round-to-zero threshold used by numeric formatters
    // and solvers.  Typed-constant default 1E-5.
    tiny :real= 1E-5;
// this came from Input.pas
    digitset=['0'..'9'];               // character set: decimal digits
    intcharset=digitset+['-'];         // digits plus leading minus sign
    ERROR_VAL = -8888;                 // sentinel "no valid value" returned by parsers
    INFINITY=1.6986727435E38; {an arbitrary large number}
    // tentothe[n] = 10^n, used for comma placement and magnitude tests in
    // ftoa2; indices -2 and -1 are padding zeros so lookups never underflow.
    tentothe:array[-2..30] of real=(0,0,1,10,100,1000,1E4,1E5,1E6,1E7,1E8,1E9,1E10,
                                    1E11,1E12,1E13,1E14,1E15,1E16,1E17,1E18,1E19,1E20,
                                    1E21,1E22,1E23,1E24,1E25,1E26,1E27,1E28,1E29,1E30);

var
    // taken from table.pas
    // cum: single-letter "cumulative summary" mode flag for PV table output
    //      (' ' = detail only; 'A'..'Z' = cumulate by period). Defaulted in
    //      the unit's initialization section below.
    cum                                          :char;
    // cumset: set of month numbers at which the table emits a subtotal line.
    cumset {,precumset}                          :set of byte;

type
  // number formatting types
  NumberFormat = shortint;   // one of DoubleDotTwo / DoubleDotFour / Int
  pathstr = string;          // alias for file-path / general strings

// to avoid circular refrences, some types from peTypes and Videodat are moved here
// following block moved here from videodat.pas
type
     // daterec: the core date type. d=day, m=month (shortint so it can hold
     // error sentinels like -88/-99), y=year offset (0 == 1900).
     daterec=record
       d,m:shortint; y : byte;
     end;
     str8=string[8];                     // "MM/DD/YY" date strings
     ch2=array[1..2] of char;            // one screen cell (char + attribute)
     ch3=array[1..3] of char;            // 3-letter month abbreviation
     str6=string[6];                     // packed "YYMMDD" date string
     str3=string[3];
     ch12=array[1..12] of char;          // formatted "Mmm DD, YYYY" date
     w13=array[1..13] of word;           // days-before-month lookup (13 = sentinel)
     shortline=array[1..60] of char;
     screenline = array[1..80] of ch2;   // one 80-column text row
     screenblock = array[1..25] of screenline; // full 25-row text screen
// end of moved block
// This is from Northwnd.pas
type str40=string[40];
     str80=string[80];
// end of block
const
     // Inclusive bounds of representable dates. y is the 1900-offset year:
     // earliest = 1900-01-01, latest = 1900+249 = 2149-12-01 (also used as the
     // "....", open-ended / not-yet-known sentinel date).
     earliest:daterec=(d:1;m:1;y:0);    // from videodat
     latest  :daterec=(d:1;m:12;y:249);


// Show an error message dialog with a string and an appended integer code.
procedure EMessage(s :pathstr; x :integer);
// Show a plain informational message box, with help text keyed by HelpCode.
procedure MessageBox( const Output: string; HelpCode: integer );
// Show a message box with a Cancel button; returns whether Cancel was pressed.
procedure MessageBoxWithCancel( const Output: string; var CancelPressed: boolean; HelpCode: integer );
// Show a Yes/No/Cancel message box; returns the choice via RetCode.
procedure MessageBoxYesNoCancel( const Output: string; var RetCode: integer; HelpCode: integer );
{ string to number converters }
// Format a double to a string per the NumberFormat selector.
function Double2StringFormat( Input: double; Format: NumberFormat ): string;
// Parse a (possibly comma/fraction-formatted) string into a double; sets ErrorFlag on failure.
function StringFormat2Double( const Input: string; var ErrorFlag: boolean ): double;
// Same as StringFormat2Double but truncates to an integer.
function StringFormat2Int( const Input: string; var ErrorFlag: boolean ): integer;
// Parse a date string into a daterec (year stored as offset from 1900); sets ErrorFlag on failure.
function StringFormat2Date( const Input: string; var ErrorFlag: boolean): daterec;
// Remove all commas from TheString in place.
procedure StripCommas( var TheString: string );
// Remove all spaces from TheString in place.
procedure StripSpaces( var TheString: string );
// Render a daterec as "M/D/YYYY".
function DateToStr( const TheDate: daterec ) : string;
{ useful string tools }
// Extract the bare file name (no directory, no extension) from a path.
function ExtractNameFromPath( ThePath: string ): string;
// Right-justify Src within Dest so it ends at column Position (note: Dest is by value).
procedure RightJustifyTo( Dest, Src: string; Position: integer );
{ this comes from Input.pas }
// Return a string of `len` spaces.
function AllBlank(len :byte):str80;
// Evaluate a free-form numeric/fraction string to a real (ERROR_VAL on failure).
function value(x:str80):real;
// Convert integer x to a string, optionally right-justified in width n (n=0 = no padding).
function strb(x,n:integer):string;
// Format a real with 2 decimals into `width` columns, optionally inserting thousands commas.
function ftoa2(x :real; width:byte; commas:boolean):str80;
// Format a real with up to 4 decimals into `width` columns (drops decimals to fit).
function ftoa4(x :real; width:byte):str80;
// Parse "[whole] num/den" fraction text into a real; returns false if malformed.
function EvaluateFractionString(s :str80; var value :real):boolean;
// Parse comma-grouped number or fraction text into a real; returns false if malformed.
function Evaluate(s :str80; var value :real):boolean;

implementation

uses
  Dialogs, LogUnit, MessageDialogUnit, Controls, HelpSystemUnit;

{ EMessage
  PURPOSE: low-level error popup; concatenates a message string with a numeric
           code/value and shows a modal error dialog with an OK button.
  PARAMS:  s - leading message text; x - integer appended to the message.
  SIDE EFFECTS: displays a modal Delphi MessageDlg. No return value.
  INTENT:  legacy diagnostic path (e.g. "Bad date passed to ... m=<x>"). }
procedure EMessage(s :pathstr; x :integer);
var
  Output: string;
begin
  Output := s + IntToStr( x );
  MessageDlg( Output, mtError, [mbOK], 0 );
end;

{ MessageBox
  PURPOSE: show a simple informational dialog with optional context help.
  PARAMS:  Output - body text; HelpCode - key looked up in HelpSystem for the
           help-pane string shown alongside the message.
  SIDE EFFECTS: displays a modal dialog; no Yes/No or Cancel buttons.
  INTENT:  the application's standard one-button notification. }
{ Abstraction of the Message Box }
procedure MessageBox( const Output: string; HelpCode: integer );
var
  bUseCancel: boolean;
  bUseYesNo: boolean;
begin;
{ TheString := Format( 'cursor at %d %d', [TheRow, TheCol] ); }
  bUseCancel := false;
  bUseYesNo := false;
  MessageDialog.ShowMessage( Output, HelpSystem.GetHelpString(HelpCode), bUseYesNo, bUseCancel );
end;

{ MessageBoxWithCancel
  PURPOSE: show a message with an OK/Cancel pair and report the user's choice.
  PARAMS:  Output - body text; CancelPressed (var) - set true if Cancel chosen;
           HelpCode - help-string key.
  SIDE EFFECTS: modal dialog. NOTE: CancelPressed is primed true and passed
           by-ref into ShowMessage, which updates it to the actual result. }
procedure MessageBoxWithCancel( const Output: string; var CancelPressed: boolean; HelpCode: integer );
var
  bUseYesNo: boolean;
begin;
{ TheString := Format( 'cursor at %d %d', [TheRow, TheCol] ); }
  CancelPressed := true;
  bUseYesNo := false;
  MessageDialog.ShowMessage( Output, HelpSystem.GetHelpString(HelpCode), bUseYesNo, CancelPressed );
end;

{ MessageBoxYesNoCancel
  PURPOSE: three-way prompt (Yes / No / Cancel) returning the chosen button.
  PARAMS:  Output - body text; RetCode (var) - set to mrYes, mrNo, or mrCancel;
           HelpCode - help-string key.
  SIDE EFFECTS: modal dialog. The local bUseYesNo/bUseCancel flags are both
           primed true, passed by-ref to ShowMessage, then decoded into RetCode:
           Cancel takes precedence, then Yes, else No. }
procedure MessageBoxYesNoCancel( const Output: string; var RetCode: integer; HelpCode: integer );
var
  bUseYesNo: boolean;
  bUseCancel: boolean;
begin
  bUseCancel := true;
  bUseYesNo := true;
  MessageDialog.ShowMessage( Output, HelpSystem.GetHelpString(HelpCode), bUseYesNo, bUseCancel );
  if( bUseCancel ) then
    RetCode := mrCancel
  else if( bUseYesNo ) then
    RetCode := mrYes
  else
    RetCode := mrNo;
end;

//
// string to number converters
//

{ Double2StringFormat
  PURPOSE: format a double for display at a fixed precision.
  PARAMS:  Input - value; Format - DoubleDotFour/DoubleDotTwo/Int selector.
  RETURNS: grouped (thousands-separated) numeric string via FloatToStrF.
  NOTE: no default case; an out-of-range Format leaves Result undefined. }
// Converts a double to a string, with the correct formatting
function Double2StringFormat( Input: double; Format: NumberFormat ): string;
begin;
  case Format of
    DoubleDotFour: Result := FloatToStrF( Input, ffNumber, 18, 4 );
    DoubleDotTwo: Result := FloatToStrF( Input, ffNumber, 18, 2 );
    Int: Result := FloatToStrF( Input, ffNumber, 18, 0 );
  end;
end;

{ StripCommas
  PURPOSE: delete every comma from TheString (in place).
  PARAMS:  TheString (var) - modified to remove all ',' characters.
  SIDE EFFECTS: mutates the argument. Loops splice the string around each
           found comma until Pos returns 0. }
// removes commas from the string.  Wouldn't it be great if there already was
// a delphi function to do this?  I'm sure there is.
procedure StripCommas( var TheString: string );
var
  Holder: string;
  Holder2: string;
  Position: integer;
begin
  Position := -1;
  while( Position <> 0 ) do begin
    Position := Pos( ',', TheString );
    if( Position <> 0 ) then begin
      Holder := Copy( TheString, 0, Position-1 );
      Holder2 := Copy( TheString, Position+1, Length(TheString)-Position );
      TheString := Holder + Holder2;
    end;
  end;
end;

{ StripSpaces
  PURPOSE: delete every space from TheString (in place).
  PARAMS:  TheString (var) - rebuilt without any ' ' characters.
  SIDE EFFECTS: mutates the argument. }
procedure StripSpaces( var TheString: string );
var
  i: integer;
  ResultString: string;
begin
  ResultString := '';
  for i:=1 to length( TheString ) do begin
    if( TheString[i] <> ' ' ) then begin
      ResultString := Resultstring + TheString[i];
    end;
  end;
  TheString := ResultString;
end;

{ StringFormat2Double
  PURPOSE: parse user-entered numeric text into a double, tolerating thousands
           commas and mixed fractions (e.g. "1,234 3/4").
  PARAMS:  Input - source text; ErrorFlag (var) - set true on bad input or
           division-by-zero in the fraction.
  RETURNS: the parsed value (0 for empty input).
  SIDE EFFECTS: writes ErrorFlag; logs to MasterLog on divide-by-zero.
  NOTE: a '/' triggers fraction handling: text before the space is the whole
        part, "top/bottom" is added as a decimal fraction. }
// Converts a string, with or without formatting, to a double.  Does it in
//  an ugly way.  Ten bucks to whoever can come up with something better (like
//  a function in Delphi that just strips out the commas from a string).
function StringFormat2Double( const Input: string; var ErrorFlag: boolean ): double;
var
  NoCommas: string;
  SpacePosition, SlashPosition: integer;
  StrTop, StrBottom: string;
  Top, Bottom: double;
  Division: double;
  WholeNumber: double;
begin
  ErrorFlag := false;
  if( Input = '' ) then exit;
  StringFormat2Double := 0;
  NoCommas := Input;
  StripCommas( NoCommas );
  SlashPosition := Pos( '/', NoCommas );
  try
    if( SlashPosition <> 0 ) then begin
      { Also handles fractions written in, so 3/4 will become .75 }
      SpacePosition := Pos( ' ', NoCommas );
      StrTop := Copy( NoCommas, SpacePosition+1, SlashPosition-SpacePosition-1 );
      StrBottom := Copy( NoCommas, SlashPosition+1, Length(NoCommas)-SlashPosition );
      Top := StrToFloat( StrTop );
      Bottom := StrToFloat( StrBottom );
      if( Bottom = 0 ) then begin
        MasterLog.Write( LVL_MED, 'StringFormat2Double: NNNNoooooo...... not division by zero...  For the love of God man!' );
        ErrorFlag := true;
        exit;
      end;
      Division := Top/Bottom;
      NoCommas := copy( NoCommas, 0, SpacePosition );
      if( SpacePosition <> 0 ) then
        WholeNumber := StrToFloat( NoCommas )
      else
        WholeNumber := 0;
      StringFormat2Double := WholeNumber + Division;
    end else
     StringFormat2Double := StrToFloat( NoCommas );
  except
    on EConvertError do ErrorFlag := true;
  end;
end;

{ StringFormat2Int
  PURPOSE: parse numeric text and truncate to an integer.
  PARAMS:  Input - source text; ErrorFlag (var) - set true on bad input.
  RETURNS: Trunc() of the parsed double (toward zero).
  SIDE EFFECTS: writes ErrorFlag. }
// Sams as StringFormat2Double, except it returns an int
function StringFormat2Int( const Input: string; var ErrorFlag: boolean ): integer;
var
  Holder : double;
begin;
  Holder := StringFormat2Double( Input, ErrorFlag );
  StringFormat2Int := Trunc( Holder );
end;

{ StringFormat2Date
  PURPOSE: parse a locale date string into the app's daterec.
  PARAMS:  Input - date text; ErrorFlag (var) - set true on conversion error.
  RETURNS: daterec with y stored as (calendar year - 1900), m, d.
  SIDE EFFECTS: writes ErrorFlag.
  NOTE: relies on Delphi StrToDate, so it honors the system date format. }
function StringFormat2Date( const Input: string; var ErrorFlag: boolean): daterec;
var
  Hold: TDateTime;
  Retval: daterec;
begin
  ErrorFlag := false;
  try
    Hold := StrToDate( Input );
    RetVal.y := YearOf( Hold )-1900;
    RetVal.m := MonthOf( Hold );
    RetVal.d := DayOf( Hold );
  except
    on EConvertError do ErrorFlag := true;
  end;
  StringFormat2Date := RetVal;
end;

{ DateToStr
  PURPOSE: render a daterec as "M/D/YYYY".
  PARAMS:  TheDate - the date record (y is the 1900-offset year).
  RETURNS: human-readable date string (re-adds the 1900 offset). }
function DateToStr( const TheDate: daterec ) : string;
begin
  DateToStr := IntToStr(TheDate.m) + '/' + IntToStr(TheDate.d) + '/' + IntToStr(TheDate.y+1900);
end;

{ ExtractNameFromPath
  PURPOSE: pull the bare base name out of a "dir\name.ext" path.
  PARAMS:  ThePath - a backslash-delimited path string.
  RETURNS: the name between the last '\' and the last '.' (extension dropped;
           leading directory dropped).
  NOTE: scans backward from Length-1 for the slash and dot; the various
        index branches handle paths with no slash and/or no dot. }
function ExtractNameFromPath( ThePath: string ): string;
var
  SlashPosition, DotPosition: integer;
begin
  SlashPosition := Length( ThePath ) - 1;
  while( SlashPosition > 0 ) do begin
    if( ThePath[SlashPosition] = '\' ) then
      break;
    Dec(SlashPosition);
  end;
  DotPosition := Length( ThePath ) - 1;
  while( DotPosition > 0 ) do begin
    if( ThePath[DotPosition] = '.' ) then
      break;
    Dec(DotPosition);
  end;
  if( DotPosition = 0 ) then begin
    if( SlashPosition = 0 ) then
      ExtractNameFromPath := copy( ThePath, SlashPosition, Length(ThePath)-SlashPosition )
    else
      ExtractNameFromPath := copy( ThePath, SlashPosition+1, Length(ThePath)-SlashPosition );
  end else begin
    if( SlashPosition = 0 ) then
      ExtractNameFromPath := copy( ThePath, SlashPosition, DotPosition-SlashPosition-1 )
    else
      ExtractNameFromPath := copy( ThePath, SlashPosition+1, DotPosition-SlashPosition-1 );
  end;
end;

{ RightJustifyTo
  PURPOSE: pad Dest with spaces then append Src so Src ends at column Position.
  PARAMS:  Dest, Src - by-VALUE strings; Position - target end column.
  SIDE EFFECTS: none observable by the caller.
  NOTE: both parameters are passed by value, so the computed result is not
        returned anywhere - effectively dead code as written. }
procedure RightJustifyTo( Dest, Src: string; Position: integer );
begin
  while Length(Dest)+Length(Src) < Position do
    Dest := Dest + ' ';
  Dest := Dest + Src;
end;

{ AllBlank
  PURPOSE: build a string of `len` spaces (column padding / blank field).
  PARAMS:  len - number of spaces.
  RETURNS: str80 whose length byte is set directly and body filled with ' '. }
// the following comes from Input.pas
function AllBlank(len :byte):str80;
         var ws:str80;
         begin
         ws[0]:=char(len);                 // set the short-string length byte
         FillChar(ws[1],len,' ');          // fill body with spaces
         AllBlank:=ws;
         end;

{ ftoa2
  PURPOSE: format a real to 2 decimal places, right-justified in a fixed field
           width, with optional thousands-comma grouping. The workhorse used
           for currency columns in tables and screens.
  PARAMS:  x - value; width - total field width; commas - insert ',' groups.
  RETURNS: a width-wide string. If the number cannot fit in `width` columns at
           2 decimals it degrades gracefully: fewer decimals, then 0 decimals,
           then scientific (EFORMAT), shaving characters to honor the width.
  INTENT:  faithful reproduction of the DOS-era numeric formatter; the half
           rounding (+/-0.5 before trunc) and the comma-fit fallbacks must be
           preserved for output parity. Recurses once (commas->false) if a
           comma'd value overflows. }
function ftoa2(x :real; width:byte; commas:boolean):str80;

         var ws,nocommas   :str80;
             plus0:string[4];
             len           :byte absolute ws;
             p,w           :byte;
             abx           :longint;
             half          :real;
         label EFORMAT,DONE;

         begin
         plus0 := '+000';
         if (abs(x)>maxlongint) then goto EFORMAT;   // too big for longint -> scientific
         if (x<0) then half:=-0.5 else half:=0.5;     // sign-aware rounding offset
         x:=0.01*trunc(x*100+half);                   // round x to 2 decimals (half away from zero)
         abx:=trunc(abs(x));                          // integer magnitude, for digit-count tests
         w:=width;
         // reserve column(s) for the commas that will be inserted later
         if (commas) then
            if (abx>=tentothe[9]) then w:=w-3          // billions -> 3 commas
            else if (abx>=tentothe[6]) then w:=w-2     // millions -> 2 commas
            else if (abx>=tentothe[3]) then dec(w)     // thousands -> 1 comma
            else commas:=false;                        // < 1000: no comma needed
         if (x<0) then dec(w);  {why was this removed?}  // reserve a column for the minus sign
         // Choose decimal count that fits: 2 dp if room, else 1 dp, else 0 dp.
         if (abx<tentothe[w-3]) then str(x:w:2,ws)               // fits with 2 decimals
         else if (abx<tentothe[w-2]) then begin                  // only room for 1 decimal
            str(x:w:1,ws);
            x:=0.1*trunc(x*10+half);                             // re-round to 1 dp
            abx:=trunc(abs(x));     {in case 999.99 is bumped to 1,000.0 by removing a decimal}
            end
         else begin
           if (abx<tentothe[w]) then str(x:w:0,ws)               // integer only
           else if (commas) then begin
              ftoa2:=ftoa2(x,width,false); exit;                 // overflow: retry without commas
              end
           else len:=succ(width);                                // force the EFORMAT path below
           end;
         if (len>width) then begin
EFORMAT:                                                         // scientific-notation fallback
            str(x:width+5,ws);                       // raw scientific form, extra room
            while (ws[1]=' ') do delete(ws,1,1);      // trim leading spaces
            p:=pos(plus0,ws);
            // strip a redundant "+000"/"+00"/... exponent prefix to save columns
            repeat
              if (p>0) then delete(ws,p,length(plus0))
              else begin
                dec(plus0[0]);                        // shorten the pattern and retry
                p:=pos(plus0,ws);
                end;
            until (p>0) or (length(plus0)=0);
            while (len>width) do delete(ws,pred(pos('E',ws)),1);  // drop mantissa digits to fit
            goto DONE;
            end;
         // Insert thousands separators back into the formatted integer part.
         if (commas) then begin
            nocommas:=ws;                             // keep a fallback copy
            w:=pos('.',ws);  if (w=0) then w:=succ(len);  // position of decimal point
            if (abx>=tentothe[3]) then insert(',',ws,w-3);
            if (abx>=tentothe[6]) then insert(',',ws,w-6);
            if (abx>=tentothe[9]) then insert(',',ws,w-9);
            if (length(ws)>width) then ws:=nocommas;  // if commas overflowed, revert
            end;
         if (copy(ws,length(ws)-4,5)='-0.00') then ws[length(ws)-4]:=' ';  // suppress "-0.00" sign
DONE:
         while (len<width) do ws:=' '+ws;             // left-pad to the requested width
         ftoa2:=ws;
         end;

{ ftoa4
  PURPOSE: format a real with as many decimals (up to 4) as fit in `width`.
  PARAMS:  x - value; width - total field width.
  RETURNS: a fixed-point string. Values smaller than `tiny` print as 0.
  INTENT:  used for rate/probability columns that want up to 4 decimals.
           Starts at width-1 decimals (capped at 4) and decrements until the
           rendered string fits within width. }
function ftoa4(x :real; width:byte):str80;
         var ws :str80;
              d :shortint;
              ok:boolean;
         begin
         if (abs(x)<tiny) then begin
            d:=1; x:=0;
            end
         else d:=width-1;
         if (d>4) then d:=4;
         repeat
           ok:=true;
           str(x:width:d,ws);
(* Why strip the zero out?
           if (x>0) then begin
             if (x<1) and (ws[1]='0') then delete(ws,1,1); end
           else if (x<0) then begin
             if (x>-1) and (ws[2]='0') then delete(ws,2,1); end;
*)
           if (length(ws)>width) then begin
              ok:=false; dec(d);
              end;
         until (ok) or (d<0);
         ftoa4:=ws;
         end;

{ strb
  PURPOSE: convert an integer to its string form, optionally width-padded.
  PARAMS:  x - value; n - field width (0 = no padding/minimum width).
  RETURNS: the decimal text of x, right-justified in n columns when n>0. }
function strb(x,n:integer):string;
         var ws :string;
         begin
         if (n=0) then str(x,ws) else str(x:n,ws);
         strb:=ws;
         end;

{ EvaluateFractionString
  PURPOSE: parse "[whole] numerator/denominator" text into a real value.
  PARAMS:  s - source text; value (var) - result, or ERROR_VAL on failure.
  RETURNS: true if parsed; false (and value=ERROR_VAL) if malformed or den=0.
  NOTE: requires the space to precede the slash; v1=whole part (0 if absent),
        v2/v3 = numerator/denominator. Result = v1 + v2/v3. }
function EvaluateFractionString(s :str80; var value :real):boolean;
         var p_,ps :byte;
             ss        :str80;
             v1,v2,v3  :real;
             vok       :integer;
         begin
         EvaluateFractionString:=false;
         value:=ERROR_VAL;
         p_:=pos(' ',s);
         ps:=pos('/',s);
         if (p_>=ps) then exit;
         if (p_=0) then v1:=0 else begin
           ss:=Copy(s,1,pred(p_)); val(ss,v1,vok); if (vok>0) then exit;
           end;
         ss:=Copy(s,succ(p_),pred(ps-p_)); val(ss,v2,vok); if (vok>0) then exit;
         ss:=Copy(s,succ(ps),80); val(ss,v3,vok); if (vok>0) then exit;
         if (v3=0) then exit;
         value:=v1 + v2/v3;
         EvaluateFractionString:=true;
         end;

{ Evaluate
  PURPOSE: parse a free-form numeric entry: blank, comma-grouped number, or
           fraction. The general-purpose input evaluator.
  PARAMS:  s - source text (trimmed); value (var) - result or ERROR_VAL.
  RETURNS: true if a valid number was parsed; false otherwise.
  NOTE: validates comma grouping (every gap between commas, and from the last
        comma to the decimal point, must be a multiple of 4 chars i.e. 3 digits
        plus the comma) before stripping commas. Falls back to
        EvaluateFractionString when plain val() fails. }
function Evaluate(s :str80; var value :real):boolean;
         var ta   :integer;
             p,q  :byte;
         begin
         s:=trim(s);
         Evaluate:=true;
         if (s='') then value:=0
         else begin
            Evaluate:=false;
            p:=pos(',',s);
            if (p=1) then begin value:=ERROR_VAL; exit; end;
            q:=pos('.',s);
            if (q=0) then q:=succ(length(s));
            while (p>0) do begin
              if ((q-p) mod 4 <> 0) then begin value:=ERROR_VAL; exit; end;
              delete(s,p,1);
              p:=pos(',',s); dec(q);
              end;
            val(s,value,ta);
            if (ta>0) then Evaluate:=EvaluateFractionString(s,value)
            else Evaluate:=true;
            end;
         end;

{ value
  PURPOSE: convenience wrapper around Evaluate returning the parsed real.
  PARAMS:  x - source text.
  RETURNS: the parsed value, or ERROR_VAL if x is not a valid number. }
function value(x:str80):real;
         var v :real;
         begin
         if (Evaluate(x,v)) then value:=v
         else value:=ERROR_VAL;
         end;

{ Unit initialization: set the default cumulative-summary mode to "off"
  (a single space means detail rows only, no subtotal grouping). }
begin
  // Short for cumulative (get your mind out of the gutter).
  cum := ' ';
end.
