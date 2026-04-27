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
      fouryears=1461; twenty_years=7305;   sl:char='/';  errorbyte=-99;

      centurydiv :byte=50;
      mon:array[1..12] of ch3 = ('Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec');
      monstr:array[0..12] of string[9]=('December','January','February','March','April','May','June',
                                           'July','August','September','October','November','December');
      earliest:daterec=(d:1;m:1;y:0);
      latest  :daterec=(d:1;m:12;y:249);
var
     video                        :word;
//     shiftstatus                  :byte absolute 0000:1047;
       {1=RShift  2=LShift  4=Ctrl  8=Alt  16=ScrollLock  32=Num Lock  64=Caps Lock  128=Insert state}
     bottomcursor                 :byte;
     date                         :ch12;
     screen                       :^screenblock;
     color                        :boolean;
     colorcard                    :boolean;
     revattr                      :char;
{$ifdef CTR_SCR}
     ctrscr,savctr                :array[0..7] of ^shortline; 
     ctrword                      :array[0..7] of longint absolute ctrscr;
{$endif}
     now                          :daterec;
     jnow                         :longint;
     leapyear                     :boolean;
     saveexit                     :pointer;
     mode                         :byte;
//     currentmode                  :byte absolute 0000:$449;

procedure CheckForDaysTooLarge(var f :daterec);
//procedure DetermineDateAndVideoMode;
//procedure GetXY(var xgo,ygo :byte);
//procedure AbsXY(var xgo,ygo :byte);
{procedure HideCursor;}
procedure Normalize(x :daterec);
procedure MDY(daynumber:longint; var x :daterec); {Compute M,D,Y from Julian}
procedure SetNow;
//procedure SetCursorSize;
procedure Str2MDY(datestr :str8; var f :daterec);
function AdvancePointer(p :pointer; x :integer):pointer;
function Date6(x :daterec):str6;
function dateok(f :daterec):boolean;
function DateStr(x :daterec):str8;
function DaysInM(f :daterec):byte;
function EquivalentAddresses(a1, a2 :pointer):boolean;
function EvalDateStr(datestr :str8; var f :daterec):boolean;
function Julian(x:daterec):longint;
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
  daysin:array[0..13] of byte=(31,31,28,31,30,31,30,31,31,30,31,30,31,31);
  daysbefore                             :^w13;
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
function Date6(x :daterec):str6;
         var ws :str3;
         begin
         str(x.y mod 100:2,ws);  if ws[1]=' ' then ws[1]:='0';
         date6[1]:=ws[1]; date6[2]:=ws[2];
         str(x.m:2,ws); if ws[1]=' ' then ws[1]:='0';
         date6[3]:=ws[1]; date6[4]:=ws[2];
         str(x.d:2,ws); if ws[1]=' ' then ws[1]:='0';
         date6[5]:=ws[1]; date6[6]:=ws[2];
         date6[0]:=#6;
         end;

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

procedure DecideAboutFeb29(wy :byte);
          begin
          if (wy mod 4 = 0) and (wy>0) then begin
               daysin[2]:=29; leapyear:=true;  daysbefore:=@leapdaysbefore end
          else begin
               daysin[2]:=28; leapyear:=false; daysbefore:=@notleapdaysbefore; end;
          end;

function DaysInM(f :daterec):byte;
         begin with f do begin
         if (m=2) then begin
            if (y mod 4 = 0) then daysinm:=29 else daysinm:=28;
            end
         else if (m<=12) and (m>=1) then DaysInM:=daysin[m]
         else DaysInM:=30; {just avoiding range check errors}
         end;end;

procedure CheckForDaysTooLarge(var f :daterec);
          var last :byte;
          begin
          last:=DaysInM(f);
          if (f.d>last) then f.d:=last;
          end;

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

procedure MDY(daynumber:longint; var x :daterec); {Compute M,D,Y from Julian}
         var  days  :integer;
              fourx :longint;

         begin
         if (daynumber<0) or (daynumber>70000) then begin x.m:=errorbyte; exit; end;
         fourx:=daynumber shl 2;
         x.y:= (fourx div fouryears);
         DecideAboutFeb29(x.y);
         days:=succ((fourx-longint(x.y)*fouryears) shr 2);
         if (days<=daysbefore^[7]) then begin
            if (days<=daysbefore^[4]) then x.m:=1 else x.m:=4; end
         else begin
            if (days<=daysbefore^[10]) then x.m:=7 else x.m:=10; end;
         repeat inc(x.m) until (daysbefore^[x.m]>=days); dec(x.m);
         x.d:=days-daysbefore^[x.m];
(*
write(x.m:2,'/',x.d:2,'/',x.y:2,'   ',daynumber);
if (Julian(x)=daynumber) then write('  ') else begin writeln(' .NE. ',Julian(x),'  '); while not keypressed do; end;
*)
         end;

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
function DateStr(x :daterec):str8;
         var  ws:pathstr; ls:string[3];
         begin
         str(x.m:2,ws);
         str(x.d:2,ls);  ws:=ws+sl+ls;
         str(x.y mod 100,ls); if (ls[0]=#1) then ls:='0'+ls; ws:=ws+sl+ls;
         Datestr:=ws;
         if (x.y=latest.y) then DateStr:='  ....  ';
         end;

function dateok(f :daterec):boolean;
         begin
         if (f.m>0) and (f.m<13) then dateok:=true else dateok:=false;
         {More conservative than necessary.  By my definition, dates
          that are not ok are either m=-99 or m=-88}
         end;

function EvalDateStr(datestr :str8; var f :daterec):boolean;
          var substr :string[2];
              p      :byte;
          label BAD_DATE;

    function Value2:shortint;
             var len    :byte absolute substr;
                 Theresult :shortint;
             begin
             Theresult:=ord(substr[1])-48;
             if (len=2) then Theresult:=Theresult*10 +  ord(substr[2])-48
             else if (len<>1) then begin Theresult:=0; f.m:=errorbyte; end;
             Value2:=Theresult;
             end;

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
