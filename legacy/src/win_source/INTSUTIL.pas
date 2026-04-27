unit INTSUTIL;
{$ifdef CHEAP} {$define PLANNER} {$endif}
INTERFACE
//uses OPCRT,DOS,NORTHWND,INPUT,VIDEODAT,LOTUS,PETYPES,PEDATA
{$ifdef TESTING} ,TESTLOW {$endif}
//;
uses Globals, peTypes, peData, VIDEODAT;

type

     counter=object
         screencount :shortint;
         function Incr:shortint;
         end;

const sc       :counter=(screencount:0);
      tablestr :str12='table';

procedure AddDays(var f :daterec; days :longint);
procedure AddPeriod(var f :daterec; peryr :byte; orig_day:byte; negative :boolean);
procedure AddNPeriods(var firstdate,lastdate :daterec; peryr :byte; n :integer);
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
procedure LineCountsFromBBot;
//{$ifndef TOPMENUS} procedure Menu; {$endif}
procedure InsufficientDataMessage(noun :str12; HelpCode: integer);
{procedure Print(line,col:byte; color:inout; x:real); in pePane}
//procedure RealToScreen(line,col:byte; color:inout; x:real); {old form of Print}
{procedure PrintDate(line,col :byte; color:inout; date:daterec); in pePane}
procedure RecordError(line,errorcol :byte);
//procedure ReInterpretDatesAndRatesAccordingToNewSettings;
procedure ReInterpretRateTable; {toggle betw SIMPLE and COMPOUND}
//procedure RequestPatience;
{procedure Right; moved to COMDUTIL}
procedure Round2(var x :real);
{procedure Save(ext :ch3);}
{procedure SelectCanadian;}
procedure SetYrDays;
{procedure TabLt; moved to COMDUTIL}
{procedure TabRt; moved to COMDUTIL}
//procedure TempMessage(ws :str80);
{procedure TestDateFunctions;}
procedure TimesPerYearCases(var val :byte; ki :char);
procedure TimeTooLong;
//procedure ToggleStr(y,col :byte; msg1,msg2 :str8);
procedure UniversalScreenProc;
procedure UpdateSettings;
//procedure WK1File(filename :str80);
{procedure WriteOneBlockToLotus(block,first_lotus_row,first_lotus_col :byte);}

//function BlankLine(block,y :byte):boolean;
//function BoxColors:byte;
//function Bright(color :byte):byte;
//function Dim(color :byte):byte;
function DateComp(f1,f2 :daterec):shortint;
function dateok(f :daterec):boolean;
function DaysCloseEnough(date1,date2 :daterec; peryr :byte):boolean;
//function EditHeaderColors:byte;
function EmptyScreen(which :byte):boolean;
//function EscapePressedInMessage(s :str80):boolean;
function ExtendedJulian(x :daterec):longint;
//function EntryBoxColors:byte;
function exxp(x :real):real;
function FindBlock(col :byte):byte;
{function fix(arg:real):real; {Largest integer less than}
{ function FullName(ext :ch3):str8;}
//function GetByte(line,col:byte):byte;
//function GetColor(z :inout):byte;
function GetMemIfAvailable(var p :pointer; siz :word):boolean;
//function GetJulian(line,col:byte; var ok :boolean):longint;
//function GetNumber(line,col:byte; var ok :boolean):real;
//function GetString(line,col:byte):string;
//function HelpEmphColors:byte;
//function HelpColors:byte;
function InBounds(var x :real; line,col :byte):boolean;
function InterpretedRate(inputrate :real): real; {The inverse of ReportedRate}
function LastDayFn(f:daterec; peryr :byte):boolean;
{ function LastPaymentBefore(upperlim,anniv :longint; peryr:byte):longint;}
{ function LineNumberFromLetter(y,block :byte):byte; }
function lnn(x :real):real;
{ function min(a,b :real):real;}
//function MainScreenColors:byte;
//function MenuBarHighlightColors:byte;
//function MenuLineColors:byte;
function NumberOfInstallments(var f,l :daterec; peryr:byte; z:upto):integer;
function ok(v :real):boolean;
function PercentValueFromCell (block, i, col: shortint; var io: inout): real;
function PerYrString(peryr :byte; capitalization :byte):str80;
function Power(x,n :real):real;
{ function QuadraticInterpolation(x1,y1,x2,y2,x3,y3 :real):real; }
{function RecallScreen(which:byte):boolean; in pePane}
function ReportedRate(apr :real):real;
function QuadraticFormula(A,B,C :real):real;
function RateFromYield(yy :real; n:byte):real;
function RealPerYr(n :byte):real;
function RevVideo(line,col:byte):boolean;
{ function RoundDownAndSubtract(var date:longint; var yrs :real; peryr:byte):longint;}
{$ifdef BUGSIN} procedure Scavenger(which :str12); {$endif}
function sqrrt(x :real):real;
{ function StringFromWindow(line,start,length :byte):str80; }
{ function SwapScreen(which :byte):boolean; in pePane}
function VersionString:str12;
function YearsDif(z,a :daterec):real;
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

function Power(x,n :real):real;
         begin
         if (x<=0) then power:=0
         else power:=exxp(n*lnn(x));
         end;

procedure Round2(var x :real);
          const halfpenny : real=0.005 - teeny;
          begin
          if (x>0) then x:=x+halfpenny
          else x:=x-halfpenny;
          x:=Trunc(x*100)/100;
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
procedure SetYrDays; {3/94}
          begin
          if (df.c.basis=x365) then yrdays:=365.25
          else yrdays:=360;
          yrinv:=1/yrdays;
          end;

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
function dateok(f :daterec):boolean;
         begin
         if (f.m>0) and (f.m<13) then dateok:=true else dateok:=false;
         {More conservative than necessary.  By my definition, dates
          that are not ok are either m=-99 or m=-88}
         end;

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
procedure InsufficientDataMessage(noun :str12; HelpCode: integer );
          begin
          MessageBox('Insufficient data on screen - no '+noun+' can be calculated.', HelpCode);
          end;

procedure TimeTooLong;
          begin
          if (errorflag) then exit;
          MessageBox('Error - time period too long.', DO_TimeTooLong);
          errorflag:=true;
          end;

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

procedure AddDays(var f :daterec; days :longint);
          var j:longint;
          begin
          j:=Julian(f)+days;
          MDY(j,f);
          end;

function LastDayFn(f:daterec; peryr :byte):boolean;
         begin with f do begin
         if (d=daysinm(f)) or ((peryr=24) and (d=15)) then LastDayFn:=true
         else LastDayFn:=false;
         end; end;

function Criterion(d1,d2 :daterec; z:upto):boolean;
         begin
         case z of
            before : Criterion:= (DateComp(d1,d2)<0);
      on_or_before : Criterion:= (DateComp(d1,d2)<=0);
             after : Criterion:= (DateComp(d1,d2)>0);
       on_or_after : Criterion:= (DateComp(d1,d2)>=0);
         end; end;

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
         if (l.y=latest.y) then begin
            NumberOfInstallments:=maxint;
            exit;end;
         originall:=l;
         ChoosePaymentDate;
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

function RevVideo(line,col :byte):boolean;
         begin
         if (screen^[line,startof[col],2]>#31) then revvideo:=true
         else revvideo:=false;
         end;

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

function QuadraticFormula(A,B,C :real):real;
         begin
         QuadraticFormula:= (- B - sqrrt(sqr(B)-4*A*C)) / (2*A);
         end;

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

function RealPerYr(n :byte):real;
         begin
         case n of daily : RealPerYr:=yrdays; {changed 4/94 from round(yrdays)}
                      52 : RealPerYr:=yrdays/7;
                      26 : RealPerYr:=yrdays/14;
         else RealPerYr:=n and (not canadian);
         end;end;

function YieldFromRate(rr :real; n:byte):real;
         var nn :real;
         begin
         nn:=RealPerYr(n);
         YieldFromRate:=nn*(exxp(rr/nn)-1);
         end;

function RateFromYield(yy :real; n:byte):real;
         var nn :real;
         begin
         nn:=RealPerYr(n);
         RateFromYield:=nn*lnn(1+yy/nn);
         end;

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
  function ReportedRate(apr :real):real;
  begin
    if (df.c.peryr and canadianORdaily>0)
      then ReportedRate:=YieldFromRate(RateFromYield(apr,h^.peryr),df.c.peryr)
    else ReportedRate:=apr;
  end;

  function InterpretedRate(inputrate :real):real; {The inverse of ReportedRate}
  begin
    InterpretedRate:=YieldFromRate(RateFromYield(inputrate,df.c.peryr),h^.peryr);
  end;

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

procedure Init;
          begin
//          shiftstatus:=shiftstatus and 127; {Ins off}
//          SetCursorSize;
          {$ifndef CHEAP} fancy:=false; {$endif}
          SetYrDays;
          end;

function counter.Incr:shortint;
         begin
         inc(screencount);
         Incr:=screencount;
         end;


begin
Init
end.
