unit Globals;
{ Headless stub of legacy/src/dos_source/Globals.pas for the binary-oracle
  driver. The interface is mirrored verbatim from the real unit; the
  implementation is rewritten to drop the GUI dependencies (Dialogs,
  Controls, MessageDialogUnit) — MessageBox just records that an error
  fired and prints to stderr. The financial units (INTSUTIL/AMORTOP/
  Amortize) are linked REAL against this stub. }

interface

uses
  SysUtils, DateUtils;

const
  DLG_YES         = 6;
  DLG_CANCEL      = 2;
  DoubleDotTwo    = 1;
  DoubleDotFour   = 2;
  Int             = 3;
  DateCellLength  = 10;
  tiny :real= 1E-5;
  digitset=['0'..'9'];
  intcharset=digitset+['-'];
  ERROR_VAL = -8888;
  INFINITY=1.6986727435E38;
  tentothe:array[-2..30] of real=(0,0,1,10,100,1000,1E4,1E5,1E6,1E7,1E8,1E9,1E10,
                                  1E11,1E12,1E13,1E14,1E15,1E16,1E17,1E18,1E19,1E20,
                                  1E21,1E22,1E23,1E24,1E25,1E26,1E27,1E28,1E29,1E30);

var
    cum                                          :char;
    cumset                                       :set of byte;

type
  NumberFormat = shortint;
  pathstr = string;

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
type str40=string[40];
     str80=string[80];
const
     earliest:daterec=(d:1;m:1;y:0);
     latest  :daterec=(d:1;m:12;y:249);

{ A headless driver can inspect this after a compute to detect that the
  engine raised an error (instead of a GUI dialog). }
var
  OracleErrorFired: boolean;
  OracleLastError:  string;

procedure EMessage(s :pathstr; x :integer);
procedure MessageBox( const Output: string; HelpCode: integer );
procedure MessageBoxWithCancel( const Output: string; var CancelPressed: boolean; HelpCode: integer );
procedure MessageBoxYesNoCancel( const Output: string; var RetCode: integer; HelpCode: integer );
function Double2StringFormat( Input: double; Format: NumberFormat ): string;
function StringFormat2Double( const Input: string; var ErrorFlag: boolean ): double;
function StringFormat2Int( const Input: string; var ErrorFlag: boolean ): integer;
function StringFormat2Date( const Input: string; var ErrorFlag: boolean): daterec;
procedure StripCommas( var TheString: string );
procedure StripSpaces( var TheString: string );
function DateToStr( const TheDate: daterec ) : string;
function ExtractNameFromPath( ThePath: string ): string;
procedure RightJustifyTo( Dest, Src: string; Position: integer );
function AllBlank(len :byte):str80;
function value(x:str80):real;
function strb(x,n:integer):string;
function ftoa2(x :real; width:byte; commas:boolean):str80;
function ftoa4(x :real; width:byte):str80;
function EvaluateFractionString(s :str80; var value :real):boolean;
function Evaluate(s :str80; var value :real):boolean;

implementation

procedure noteError(const s: string);
begin
  OracleErrorFired := true;
  OracleLastError := s;
  Writeln(StdErr, 'ENGINE ERROR: ', s);
end;

procedure EMessage(s :pathstr; x :integer);
begin
  noteError(s + IntToStr(x));
end;

procedure MessageBox( const Output: string; HelpCode: integer );
begin
  noteError(Output);
end;

procedure MessageBoxWithCancel( const Output: string; var CancelPressed: boolean; HelpCode: integer );
begin
  CancelPressed := true;
  noteError(Output);
end;

procedure MessageBoxYesNoCancel( const Output: string; var RetCode: integer; HelpCode: integer );
begin
  RetCode := DLG_CANCEL;
  noteError(Output);
end;

function Double2StringFormat( Input: double; Format: NumberFormat ): string;
begin
  case Format of
    DoubleDotTwo:  Result := FormatFloat('0.00', Input);
    DoubleDotFour: Result := FormatFloat('0.0000', Input);
  else             Result := IntToStr(Round(Input));
  end;
end;

function StringFormat2Double( const Input: string; var ErrorFlag: boolean ): double;
var v: double;
begin
  ErrorFlag := not TryStrToFloat(Trim(Input), v);
  if ErrorFlag then v := 0;
  Result := v;
end;

function StringFormat2Int( const Input: string; var ErrorFlag: boolean ): integer;
var v: integer;
begin
  ErrorFlag := not TryStrToInt(Trim(Input), v);
  if ErrorFlag then v := 0;
  Result := v;
end;

function StringFormat2Date( const Input: string; var ErrorFlag: boolean): daterec;
begin
  Result := earliest;
  ErrorFlag := true; // not needed by the headless compute path
end;

procedure StripCommas( var TheString: string );
begin
  TheString := StringReplace(TheString, ',', '', [rfReplaceAll]);
end;

procedure StripSpaces( var TheString: string );
begin
  TheString := StringReplace(TheString, ' ', '', [rfReplaceAll]);
end;

function DateToStr( const TheDate: daterec ) : string;
begin
  Result := Format('%.2d/%.2d/%.4d', [TheDate.m, TheDate.d, 1900 + TheDate.y]);
end;

function ExtractNameFromPath( ThePath: string ): string;
begin
  Result := ExtractFileName(ThePath);
end;

procedure RightJustifyTo( Dest, Src: string; Position: integer );
begin
  // presentation only — unused by the compute path
end;

function AllBlank(len :byte):str80;
begin
  Result := StringOfChar(' ', len);
end;

function value(x:str80):real;
var v: double; e: boolean;
begin
  v := StringFormat2Double(x, e);
  if e then v := 0;
  Result := v;
end;

function strb(x,n:integer):string;
begin
  Result := IntToStr(x);
end;

function ftoa2(x :real; width:byte; commas:boolean):str80;
begin
  Result := FormatFloat('0.00', x);
end;

function ftoa4(x :real; width:byte):str80;
begin
  Result := FormatFloat('0.0000', x);
end;

function EvaluateFractionString(s :str80; var value :real):boolean;
var e: boolean;
begin
  value := StringFormat2Double(s, e);
  Result := not e;
end;

function Evaluate(s :str80; var value :real):boolean;
begin
  Result := EvaluateFractionString(s, value);
end;

begin
  OracleErrorFired := false;
  OracleLastError := '';
end.
