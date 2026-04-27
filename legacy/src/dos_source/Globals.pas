unit Globals;

interface

uses
  SysUtils, DateUtils;

const
  // Constants for Message dialogs
  DLG_YES         = 6;
  DLG_CANCEL      = 2;
  // NumberFormat types
  DoubleDotTwo    = 1;
  DoubleDotFour   = 2;
  Int             = 3;

  DateCellLength  = 10;

// This came from petypes.pas
// James sez - again, I don't want to include INPUT so I'm
// setting this to the value from that file.
//    tiny :real= INPUT.tiny;
    tiny :real= 1E-5;
// this came from Input.pas
    digitset=['0'..'9'];
    intcharset=digitset+['-'];
    ERROR_VAL = -8888;
    INFINITY=1.6986727435E38; {an arbitrary large number}
    tentothe:array[-2..30] of real=(0,0,1,10,100,1000,1E4,1E5,1E6,1E7,1E8,1E9,1E10,
                                    1E11,1E12,1E13,1E14,1E15,1E16,1E17,1E18,1E19,1E20,
                                    1E21,1E22,1E23,1E24,1E25,1E26,1E27,1E28,1E29,1E30);

var
    // taken from table.pas
    cum                                          :char;
    cumset {,precumset}                          :set of byte;

type
  // number formatting types
  NumberFormat = shortint;
  pathstr = string;

// to avoid circular refrences, some types from peTypes and Videodat are moved here
// following block moved here from videodat.pas
type
     daterec=record
       d,m:shortint; y : byte;
     end;
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
// end of moved block
// This is from Northwnd.pas
type str40=string[40];
     str80=string[80];
// end of block
const
     earliest:daterec=(d:1;m:1;y:0);    // from videodat
     latest  :daterec=(d:1;m:12;y:249);


procedure EMessage(s :pathstr; x :integer);
procedure MessageBox( const Output: string; HelpCode: integer );
procedure MessageBoxWithCancel( const Output: string; var CancelPressed: boolean; HelpCode: integer );
procedure MessageBoxYesNoCancel( const Output: string; var RetCode: integer; HelpCode: integer );
{ string to number converters }
function Double2StringFormat( Input: double; Format: NumberFormat ): string;
function StringFormat2Double( const Input: string; var ErrorFlag: boolean ): double;
function StringFormat2Int( const Input: string; var ErrorFlag: boolean ): integer;
function StringFormat2Date( const Input: string; var ErrorFlag: boolean): daterec;
procedure StripCommas( var TheString: string );
procedure StripSpaces( var TheString: string );
function DateToStr( const TheDate: daterec ) : string;
{ useful string tools }
function ExtractNameFromPath( ThePath: string ): string;
procedure RightJustifyTo( Dest, Src: string; Position: integer );
{ this comes from Input.pas }
function AllBlank(len :byte):str80;
function value(x:str80):real;
function strb(x,n:integer):string;
function ftoa2(x :real; width:byte; commas:boolean):str80;
function ftoa4(x :real; width:byte):str80;
function EvaluateFractionString(s :str80; var value :real):boolean;
function Evaluate(s :str80; var value :real):boolean;

implementation

uses
  Dialogs, LogUnit, MessageDialogUnit, Controls, HelpSystemUnit;

procedure EMessage(s :pathstr; x :integer);
var
  Output: string;
begin
  Output := s + IntToStr( x );
  MessageDlg( Output, mtError, [mbOK], 0 );
end;

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

procedure MessageBoxWithCancel( const Output: string; var CancelPressed: boolean; HelpCode: integer );
var
  bUseYesNo: boolean;
begin;
{ TheString := Format( 'cursor at %d %d', [TheRow, TheCol] ); }
  CancelPressed := true;
  bUseYesNo := false;
  MessageDialog.ShowMessage( Output, HelpSystem.GetHelpString(HelpCode), bUseYesNo, CancelPressed );
end;

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

// Converts a double to a string, with the correct formatting
function Double2StringFormat( Input: double; Format: NumberFormat ): string;
begin;
  case Format of
    DoubleDotFour: Result := FloatToStrF( Input, ffNumber, 18, 4 );
    DoubleDotTwo: Result := FloatToStrF( Input, ffNumber, 18, 2 );
    Int: Result := FloatToStrF( Input, ffNumber, 18, 0 );
  end;
end;

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

// Sams as StringFormat2Double, except it returns an int
function StringFormat2Int( const Input: string; var ErrorFlag: boolean ): integer;
var
  Holder : double;
begin;
  Holder := StringFormat2Double( Input, ErrorFlag );
  StringFormat2Int := Trunc( Holder );
end;

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

function DateToStr( const TheDate: daterec ) : string;
begin
  DateToStr := IntToStr(TheDate.m) + '/' + IntToStr(TheDate.d) + '/' + IntToStr(TheDate.y+1900);
end;

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

procedure RightJustifyTo( Dest, Src: string; Position: integer );
begin
  while Length(Dest)+Length(Src) < Position do
    Dest := Dest + ' ';
  Dest := Dest + Src;
end;

// the following comes from Input.pas
function AllBlank(len :byte):str80;
         var ws:str80;
         begin
         ws[0]:=char(len);
         FillChar(ws[1],len,' ');
         AllBlank:=ws;
         end;

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
         if (abs(x)>maxlongint) then goto EFORMAT;
         if (x<0) then half:=-0.5 else half:=0.5;
         x:=0.01*trunc(x*100+half);
         abx:=trunc(abs(x));
         w:=width;
         if (commas) then
            if (abx>=tentothe[9]) then w:=w-3
            else if (abx>=tentothe[6]) then w:=w-2
            else if (abx>=tentothe[3]) then dec(w)
            else commas:=false;
         if (x<0) then dec(w);  {why was this removed?}
         if (abx<tentothe[w-3]) then str(x:w:2,ws)
         else if (abx<tentothe[w-2]) then begin
            str(x:w:1,ws);
            x:=0.1*trunc(x*10+half);
            abx:=trunc(abs(x));     {in case 999.99 is bumped to 1,000.0 by removing a decimal}
            end
         else begin
           if (abx<tentothe[w]) then str(x:w:0,ws)
           else if (commas) then begin
              ftoa2:=ftoa2(x,width,false); exit;
              end
           else len:=succ(width);
           end;
         if (len>width) then begin
EFORMAT:
            str(x:width+5,ws);
            while (ws[1]=' ') do delete(ws,1,1);
            p:=pos(plus0,ws);
            repeat
              if (p>0) then delete(ws,p,length(plus0))
              else begin
                dec(plus0[0]);
                p:=pos(plus0,ws);
                end;
            until (p>0) or (length(plus0)=0);
            while (len>width) do delete(ws,pred(pos('E',ws)),1);
            goto DONE;
            end;
         if (commas) then begin
            nocommas:=ws;
            w:=pos('.',ws);  if (w=0) then w:=succ(len);
            if (abx>=tentothe[3]) then insert(',',ws,w-3);
            if (abx>=tentothe[6]) then insert(',',ws,w-6);
            if (abx>=tentothe[9]) then insert(',',ws,w-9);
            if (length(ws)>width) then ws:=nocommas;
            end;
         if (copy(ws,length(ws)-4,5)='-0.00') then ws[length(ws)-4]:=' ';
DONE:
         while (len<width) do ws:=' '+ws;
         ftoa2:=ws;
         end;

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

function strb(x,n:integer):string;
         var ws :string;
         begin
         if (n=0) then str(x,ws) else str(x:n,ws);
         strb:=ws;
         end;

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

function value(x:str80):real;
         var v :real;
         begin
         if (Evaluate(x,v)) then value:=v
         else value:=ERROR_VAL;
         end;

begin
  // Short for cumulative (get your mind out of the gutter).
  cum := ' ';
end.
