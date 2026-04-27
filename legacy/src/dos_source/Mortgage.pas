{$ifdef OVERLAYS} {$O+,F+} {$endif}

unit MORTGAGE;

INTERFACE

//uses OPCRT,VIDEODAT,NORTHWND,INPUT,LOTUS,PETYPES,PEDATA,INTSUTIL,
//     PEPANE,COMDUTIL,KCOMMAND,
uses PEDATA, PETYPES, INTSUTIL, Globals;

{$ifdef 0}
{$ifdef NO_FRILLS}
HTXTSTUB
{$else}
HTXTHELP
{$endif}
{$ifdef TESTING}
,TESTHIGH
{$endif}
{$ifdef TOPMENUS}
,MCMENU
{$endif}
;
{$endif}

procedure CalculateRows( First, Last: integer );
procedure Calc (row: integer);
procedure ZeroMortgage( Mortgage: mtgptr );
function MortgageIsEmpty( Mortgage: mtgptr ): boolean;
function NumberOfValidRowsInArray( var Mortgage: MtgRecList; var first_valid, last_valid :byte):byte;
procedure ReportAPR (var ei: mtgline);
function EnoughDataForAPR(var e :mtgline):boolean;
function EnoughDataForRowGeneration(var e :mtgline):boolean;
procedure ReportComparisonOfAPRs( row1,row2 :byte; var Result1, Result2, FinalResult: string );

IMPLEMENTATION

uses SysUtils, HelpSystemUnit;

{$ifdef 0}
{$F+}
procedure MortgageScreen(code :byte);
          var i,attr:byte;
{$ifndef OVERLAYS} {$F-} {$endif}
          begin
          window(1,1,80,25); gotoxy(1,1);
          block:=MTGBlock;
{$ifdef TOPMENUS}
          DrawMenuBar; gotoxy(1,2);
          textattr:=MainScreenColors;
{$endif}
          write('ÚÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÂÄÄÄÄÄÄÄÄÄÄ¿');
          if (color) then attr:=(green shl 4 + white) else attr:=112;
{$ifdef TOPMENUS}
          FastWrite('MORTGAGE SCREEN',1,64,attr);
          write(
{$else}
          FastWrite('MORTGAGE SCREEN',1,3,attr);
          write('³                                                                   ³          ³',
{$endif}
                '³                %   Cash      Amount       Loan  Monthly   Monthly ³Balloon   ³',
                '³ Price     Pts Down Required  Borrowed Yrs Rate  Tax+Ins   Total   ³Yrs Amount³',
                'ÆÍÍÍÍÍÍÍÍÍÍÑÍÍÍÍÑÍÍÑÍÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÍÑÍÍÑÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÑÍÍÍÍÍÍÍÍÍØÍÍÑÍÍÍÍÍÍÍµ');
          for i:=1 to screenlines[block] do
          write('³          ³    ³  ³         ³         ³  ³      ³        ³         ³  ³       ³');
          write('ÀÄÄÄÄÄÄÄÄÄÄÁÄÄÄÄÁÄÄÁÄÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÁÄÄÁÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÁÄÄÄÄÄÄÄÄÄÁÄÄÁÄÄÄÄÄÄÄÙ');
          InitScrollBar(MTGblock);
          Menu; {=UpdateSettings; + memory.PlaceOnScreen; }
          DisplayAll;
          Home(MTGBlock);
          end;
{$endif}

procedure ZeroMortgage( Mortgage: mtgptr );
begin
  Mortgage.PriceStatus := empty;
  Mortgage.Price := 0;
  Mortgage.PointsStatus := empty;
  Mortgage.Points := 0;
  Mortgage.PctStatus := empty;
  Mortgage.Pct := 0;
  Mortgage.CashStatus := empty;
  Mortgage.Cash := 0;
  Mortgage.FinancedStatus := empty;
  Mortgage.Financed := 0;
  Mortgage.YearsStatus := empty;
  Mortgage.Years := 0;
  Mortgage.RateStatus := empty;
  Mortgage.Rate := 0;
  Mortgage.TaxStatus := empty;
  Mortgage.Tax := 0;
  Mortgage.MonthlyStatus := empty;
  Mortgage.Monthly := 0;
  Mortgage.WhenStatus := empty;
  Mortgage.When := 0;
  Mortgage.HowMuchStatus := empty;
  Mortgage.HowMuch := 0;
  Mortgage.BalloonStatus := Balloon_Blank;
end;

function MortgageIsEmpty( Mortgage: mtgptr ): boolean;
begin
  MortgageIsEmpty := (Mortgage.PriceStatus = empty) and (Mortgage.PointsStatus = empty) and
                     (Mortgage.PctStatus = empty) and (Mortgage.CashStatus = empty) and
                     (Mortgage.FinancedStatus = empty) and (Mortgage.YearsStatus = empty) and
                     (Mortgage.RateStatus = empty) and (Mortgage.TaxStatus = empty) and
                     (Mortgage.MonthlyStatus = empty) and (Mortgage.WhenStatus = empty) and
                     (Mortgage.HowMuchStatus = empty);
end;

  {************************************************}
  {*                                              *}
  {*  Calculation methods for the Mortgage screen *}
  {*                                              *}
  {************************************************}


  {************************************************}
  {* Formula for summing n payments over      *}
  {* time discounted at the appropriate interest  *}
  {* rate.                    *}
  {************************************************}

  function Summation (ei: mtgline; r, t: real): real;
    { r is the true rate;  t is duration in years.}
    { Normally this is called with "Summation(e.rate,e.years)", but}
    { for early termination and APR calculations we sometimes want to}
    { change rate or years in call.}
    var
      last, f: real;
  begin
    with ei do
      begin
        if (abs(r) < teeny) then
          summation := 12 * t
        else
          begin
            last := exxp(-r * t);
            f := exxp(-r * twelfth);
            summation := f * (1 - last) / (1 - f);
          end;
      end;
  end;


  {************************************************}
  {*                  FirstPass                   *}
  {************************************************}

  procedure FirstPass (row: integer);
    var
      i  :integer;
  begin
    errorflag:=false;
    overflowflag:=false;
    i := row + ScrollPos[MTGblock];
    with e[i]^ do
      begin
//        ClearOutputCellsInRow(MTGBlock,row);
        if (yearsstatus = inp) and (years <= 0) then
          begin
            RecordError(row+ztop[MTGBlock],yearscol);
            MessageBox('Mortgage term ("Yrs") must be a positive number.', DM_YearsNegative);
          end;
        if (whenstatus = inp) then
          begin
            if (howmuchstatus = inp) then
              balloonstatus := balloon_known
            else
              balloonstatus := balloon_unk;
          end
        else if (howmuchstatus = inp) then
          begin
            errorflag := true;
            MessageBox('You must specify how many years from now the balloon payment is to be made.', DM_SpecifyBalloonPayment);
          end
        else
          balloonstatus := balloon_blank;
        if (pricestatus = inp) and (financedstatus > empty) and (financed > price) then
          begin
            MessageBox('Amount borrowed cannot exceed price.', DM_AmountBorrowedExceedsPrice);
            RecordError(row+ztop[MTGBlock],financedcol);
          end;
      end;
  end;


  {************************************************}
  {*                      Calc                      *}
  {************************************************}

  procedure Calc (row: integer);  {Don't change to byte: negative values are possible if block is scrolled.}
    var
      balloonval: real;
      did_one: boolean;
      i: integer;

    procedure ComputeCashPctAndFinanced;  {within the scope of Calc}
    begin
      with e[i]^ do
        begin
          if (abs(price) < teeny) then
            begin
              MessageBox('Error - price too small. ', DM_PriceTooSmall);
              exit;
            end;
          if (pctstatus = inp) then
            begin
              cash := price * (pct + (1 - pct) * points);
              cashstatus:=outp;
              financed := price * (1 - pct);
              financedstatus:=outp;
            end
          else if (cashstatus = inp) then
            begin
              pct := (cash / price - points) / (1 - points);
              if (pct >= 0.995) then
                begin
                  RecordError(row+ztop[MTGBlock], cashcol);
                  pct := error;
                end
              else
                begin
                  pctstatus:=outp;
                  financed := price * (1 - pct);
                  financedstatus:=outp;
                end;
            end
          else if (financedstatus = inp) then
            begin
              pct := 1 - (financed / price);
              if (pct >= 0.995) then
                begin
                  RecordError(row+ztop[MTGBlock],financedcol);
                  pct := error;
                end
              else
                begin
                  pctstatus:=outp;
                  cash := price * (pct + (1 - pct) * points);
                  cashstatus:=outp;
                end;
            end;
        end;
    end;

    procedure BalloonCalc; {within the scope of Calc}
    begin
      with e[i]^ do
        begin
          did_one := true;
          balloonval := price * (1 - pct) - (monthly - tax) * Summation(e[i]^, rate, years) - balloonval;
          howmuch := balloonval * exxp(rate * when);
          howmuchstatus:=outp;
          { balloonstatus := balloon_known; This messes things up if you CALC twice.}
        end;
    end;

  begin {Calc}
    i := row + scrollpos[MTGblock];
    with e[i]^ do
      begin
        if (pricestatus = inp) then
          ComputeCashPctAndFinanced;
        if (errorflag) then exit;
        if ((pctstatus=inp) or (cashstatus=inp) or (financedstatus=inp)) and (yearsstatus=inp) and (ratestatus=inp) then
          begin
            balloonval := 0;
            if (balloonstatus = balloon_known) then
              balloonval := balloonval + howmuch * exxp(-rate * when);
            if (pricestatus = inp) then
              begin
                if (monthlystatus = inp) then
                  begin
                    did_one := false;
                    if (balloonstatus = balloon_unk) then
                      BalloonCalc;
                    if (not did_one) then
                      begin
                        MessageBox('Leave PRICE or MONTHLY PAYMENT or ANY BALLOON AMOUNT blank, to be computed.', DM_LeaveSomeBlank);
                        errorflag := true;
                      end;
                  end
                else
                  if (balloonstatus <> balloon_unk) then {price ok but not monthly}
                    begin
                      monthly := (price * (1 - pct) - balloonval) / Summation(e[i]^, rate, years) + tax;
                      monthlystatus:=outp;
                    end;
              end
            else if ((monthlystatus = inp)) and (balloonstatus <> balloon_unk) then
              begin
                if (pctstatus = inp) then
                  price := ((monthly - tax) * Summation(e[i]^, rate, years) + balloonval) / (1 - pct)
                else if (cashstatus = inp) then
                  price := cash + (1 - points) * ((monthly - tax) * Summation(e[i]^, rate, years) + balloonval)
                else
                  begin
                    MessageBox('Fill in Percent Down or Cash Required for computation of price. ', DM_FillPercentOrCash);
                    exit;
                  end;
                ComputeCashPctAndFinanced;
                pricestatus:=outp;
{$ifdef BUGSIN}
     if (price<0) then Scavenger('C-10');
{$endif}
              end
          end;
      end;
  end; {Calc}

  function TerminalBalloon (var ei: mortgageline; t: real): real;
        { What's the remaining balance for the mortgage represented}
    {  by line i at a time t years after loan date?  Answer includes the}
    {  regular payment that would be due on that date.}
    var
      theresult: real;
  begin
    with ei do
      begin
        theresult := financed - (monthly - tax) * Summation(ei, ei.rate, t - twelfth);
        if (balloonstatus <> balloon_blank) and (when <= t) then
          theresult := theresult - howmuch * exxp(-rate * when);
        theresult := theresult * exxp(rate * t);
        TerminalBalloon := theresult;
      end;
  end;

  function ValueOfPaymentsForTerminatedLoan (var ei: mortgageline; r, t: real): real;
          {What is the value (as of loan date, using rate=r) of all the}
      {loan payments up to time t, including a terminal balloon at t?}
    var
      theresult: real;
  begin
    with ei do
      begin
        theresult := (monthly - tax) * Summation(ei, r, t - twelfth);
        if (balloonstatus <> balloon_blank) and (when < t) then
          theresult := theresult + ei.howmuch * exxp(-r * when);
        if (overflowflag) then
          exit;
        if (t <= years) then
          theresult := theresult + TerminalBalloon(ei, t) * exxp(-r * t);
        ValueOfPaymentsForTerminatedLoan := theresult;
      end;
  end;


  function IterateToFindAPRofTerminatedLoan (ei: mortgageline; t: real; var apr: real): boolean;
      {ei  is not a var parameter because it is being modified within the function}
    const
      small = 0.0001;
    var
      delta, newdelta, value, oldvalue, target, denom: real;
      count: byte;
  begin
    with ei do
      begin
        target := financed * (1 - points);
        apr := rate + points / years; {First guess}
        value := ValueOfPaymentsForTerminatedLoan(ei, apr, t);
            {years+twelth => loan not terminated before due date.}
        oldvalue := value;
        delta := small;
        count := 0;
        apr := apr + delta;
        repeat
          count := succ(count);
          if (overflowflag) then
            exit;
          value := ValueOfPaymentsForTerminatedLoan(ei, apr, t);
          denom := value - oldvalue;
          if (abs(denom) > teeny) then
            newdelta := (target - value) * delta / denom
          else
            newdelta := small;
          delta := newdelta;
          apr := apr + delta;
          oldvalue := value;
        until (count = 20) or (abs(delta) < teeny);
        IterateToFindAPRofTerminatedLoan := (abs(delta) < tiny);
        apr := YieldFromRate(apr, 12);
      end;
  end;

  function IterateToFindAPR (var ei: mtgline; var apr: real): boolean;
  begin
    IterateToFindAPR := IterateToFindAPRofTerminatedLoan(ei, ei.years + twelfth, apr);
  end;

  function IterateToFindCrossoverAPRandTime (var e1, e2: mtgline; var apr, t: real): boolean;
    const
      maxcount = 40;
    var
      count                                                   :byte;
      apr1,apr2,r,lastt,lastr,target1,target2,lasttarget1,
      lasttarget2,dTarg1dt,dTarg2dt,dTarg1dr,
      dTarg2dr,dr,dt,det,invdet,baser,baset                   :real;

    function Target (e: mtgline): real;
            {The thing we're trying to get equal to zero.}
    begin
      Target := 1 - (ValueOfPaymentsForTerminatedLoan(e, r, t) / (e.financed * (1 - e.points)));
    end;

    procedure Reset (var r, t: real);
    begin
      baset := baset + 2;
      t := baset;
      r := baser;
    end;

    procedure OneIteration;
    begin
      if (t < 0) then
        Reset(r, t);
      dr := tiny;
      dt := small;
      lasttarget1 := Target(e1);
      if (overflowflag) then
        exit;
      lasttarget2 := Target(e2);
      if (overflowflag) then
        exit;
      lastr := r;
      r := r + dr;
      target1 := Target(e1);
      if (overflowflag) then
        exit;
      target2 := Target(e2);
      if (overflowflag) then
        exit;
      dTarg1dr := (target1 - lasttarget1) / dr;
      dTarg2dr := (target2 - lasttarget2) / dr;

      lasttarget1 := target1;
      lasttarget2 := target2;
      lastt := t;
      t := t + dt;
      target1 := Target(e1);
      if (overflowflag) then
        exit;
      target2 := Target(e2);
      if (overflowflag) then
        exit;
      dTarg1dt := (target1 - lasttarget1) / dt;
      dTarg2dt := (target2 - lasttarget2) / dt;

      det := dTarg1dt * dTarg2dr - dTarg1dr * dTarg2dt;
      if (abs(det) < teeny) then
        overflowflag := true
      else
        invdet := 1 / det;

      dr := (dTarg2dt * Target1 - dTarg1dt * Target2) * invdet;
      dt := (-dTarg2dr * Target1 + dTarg1dr * Target2) * invdet;

      r := lastr + dr;
      t := lastt + dt;
    end;

    function TryBalloonDates: boolean;
        { Maybe the iteration isn't converging because the target date}
      { is on a balloon date, where functions behave discontinuously.}
      var
        apr1before, apr2before, apr1after, apr2after: real;
      label
        1;
    begin
      overflowflag := false;
      TryBalloonDates := false;
      if (e1.balloonstatus <> balloon_blank) then
        begin
          if (not IterateToFindAPRofTerminatedLoan(e1, e1.when, apr1before)) then
            goto 1;
          if (not IterateToFindAPRofTerminatedLoan(e1, e1.when + twelfth, apr1after)) then
            goto 1;
          if (not IterateToFindAPRofTerminatedLoan(e2, e1.when, apr2before)) then
            goto 1;
          if (not IterateToFindAPRofTerminatedLoan(e2, e1.when + twelfth, apr2after)) then
            goto 1;
          if (apr1before>apr2before) xor (apr1after>apr2after) then
            begin
              apr := apr2before;
              t := e1.when;
              TryBalloonDates := true;
              exit;
            end;
        end;
1:
      if (e2.balloonstatus <> balloon_blank) then
        begin
          if (not IterateToFindAPRofTerminatedLoan(e1, e2.when, apr1before)) then
            exit;
          if (not IterateToFindAPRofTerminatedLoan(e1, e2.when + twelfth, apr1after)) then
            exit;
          if (not IterateToFindAPRofTerminatedLoan(e2, e2.when, apr2before)) then
            exit;
          if (not IterateToFindAPRofTerminatedLoan(e2, e2.when + twelfth, apr2after)) then
            exit;
          if ((apr1before>apr2before) xor (apr1after>apr2after)) then
            begin
              apr := apr1before;
              t := e2.when;
              TryBalloonDates := true;
            end;
        end;
    end;

  begin {IterateToFindCrossoverAPRandTime}
    IterateToFindCrossoverAPRandTime := false;
          {first guess:}
    t := 0.25 * (e1.years + e2.years);
    if IterateToFindAPR(e1, apr1) and IterateToFindAPR(e2, apr2) then
      r := RateFromYield(half * (apr1 + apr2), 12)
    else
      r := half * (e1.rate + e1.points / t + e2.rate + e2.points / t);
    baser := r;
    baset := 1;
    Reset(r, t);
    count := 0;
    repeat
      count := succ(count);
      OneIteration;
      if (overflowflag) then
        count := maxcount;
    until (count >= maxcount) or ((abs(target1) < teeny) and (abs(target2) < teeny));
    overflowflag := false;
    if (abs(target1) > tiny) or (abs(target2) > tiny) then
      if not (TryBalloonDates) then
        exit;
    apr := YieldFromRate(r, 12);
    t := twelfth * trunc(12 * t); {Round down to prev full month.}
    IterateToFindCrossoverAPRandTime := (t <= e1.years) and (t <= e2.years) and (t > 0) and (r < 1) and (r >= 0);
  end; {IterateToFindCrossoverAPRandTime}


  procedure ReportAPR (var ei: mtgline);
    var
      apr      :real;
      s1, s2   :str80;
  begin
    if (IterateToFindAPR(ei, apr)) then
      begin
        s1 := Concat('APR if held to full term : ', ftoa4(100 * apr, 7));
        s2 := '(To compare two mortgage lines, 1) highlight one with double click, 2) locate cursor'
                + 'in the other, and 3) Select menu option for APR comparison.)';
        MessageBox(s1, HELP_NULL );
      end
    else
      begin
        s1 := 'APR computation did not converge. ';
        MessageBox(s1, DM_APRDidNotConverge);
      end;
  end;

  function OneMonthAPR (var ei: mtgline): real;
      { The APR if you pay off the whole loan when the first payment}
      { is due.}
    var
      apr_rate: real;
  begin
    apr_rate := 12 * (1 + YieldFromRate(ei.rate, 12)/12) / (1 - ei.points);
    OneMonthAPR := YieldFromRate(apr_rate, 12);    {^corrected 2/94}
  end;

(*
    FROM OLD VERSION : ReportComparisnOfAPRs and CompareAPRs
*)

function EnoughDataForAPR(var e :mtgline):boolean;
         begin with e do
         EnoughDataForAPR:=(financedstatus>0) and (ok(financed)) and (monthlystatus>0) and (ok(monthly))
                             and (ratestatus>0) and (ok(rate)) and (yearsstatus>0) and (ok(years));
         end;

// This is a re-write of the old function NumberOfValidRowsOnScreen.  I tried
// to stick to the original as best as possible.  The problem with it was that
// it was directly reading from the screen.  This does the same thing, but it
// reads from the array instead.
function NumberOfValidRowsInArray( var Mortgage: MtgRecList; var first_valid, last_valid :byte):byte;
var
  y,theresult, current_row,maxy :byte;
begin
  NumberOfValidRowsInArray := 0;
  current_row := 1;
  // We're excluding the row with the cursor from first_valid, if anything
  // else is available.
  first_valid:=0;
  last_valid:=0;
  maxy:=maxlines;
  theresult := 0;
  for y:=1 to maxy do begin
    if( not MortgageIsEmpty(mtgptr(Mortgage[y])) ) then begin
      if (errorflag) then exit;
      FirstPass(y);
      Calc(y);
      if (errorflag) then exit;
      if (EnoughDataForAPR(e[y]^)) then begin
        if (first_valid=0) then
          first_valid:=y
        else
          last_valid:=y;
        inc( theresult );
      end;
    end;
  end;
  NumberOfValidRowsInArray := theresult;
end;

// This function needed to be changed as it was writing directly to the
// screen.  Now it writes to strings, which are output somewhere else.
procedure ReportComparisonOfAPRs( row1,row2 :byte; var Result1, Result2, FinalResult: string );
          const NoConvergeStr:string[40]=' Crossover computation did not converge.';
          var scrline1, scrline2 :screenline;
              e1,e2              :mtgline;
              char1,char2        :string;
              kiword             :word;
              apr                :real;
              apr1,apr1short,
              apr2,apr2short     :real;
              months,years       :integer;
              timestr            :str80;

          begin
          // screen is 0 based, e is 1 based
          char1:=IntToStr(row1-1); char2:=IntToStr(row2-1);
          FirstPass(row1); Calc(row1); //DisplayRowOfBlock(MTGBlock,row1);
          e1:=e[row1]^;
          FirstPass(row2); Calc(row2); //DisplayRowOfBlock(MTGBlock,row2);
          e2:=e[row2]^;
          if (errorflag) then begin
             MessageBox('Insufficient data in second mortgage.', DM_InsufficientDataIn2nd);
             exit; end;
{          if not scripting then begin
            MoveScreen(screen^[ztop[MTGblock]+row1],scrline1,80);
            MoveScreen(screen^[ztop[MTGblock]+row2],scrline2,80);
            PushWindow(11,ztop[MTGBlock]+4,70,ztop[MTGBlock]+12,yellow,yellow);
            MoveScreen(scrline1,screen^[ztop[MTGBlock]+1],80);
            MoveScreen(scrline2,screen^[ztop[MTGBlock]+2],80);
            FastWrite(char1,succ(ztop[MTGBlock]),pred(lleft[MTGBlock]),MessageColors);
            FastWrite(char2,ztop[MTGBlock]+2,pred(lleft[MTGBlock]),MessageColors);
            CenterText('Comparison of APR''s'); writeln;
            end;
            }
          if (IterateToFindAPR(e1,apr1)) then begin
             if (not scripting) then
               Result1 := 'Mortgage ' + char1 + ' if held to term, APR = ' + Double2StringFormat(100.0*apr1, DoubleDotFour);
             end
          else begin
             if (not scripting) then
               Result1 := 'Mortgage ' + char1 + ': APR computation did not converge.';
             end;
          if (IterateToFindAPR(e2,apr2)) then begin
             if (not scripting) then
               Result2 := 'Mortgage ' + char2 + ' if held to term, APR = ' + Double2StringFormat(100.0*apr2, DoubleDotFour);
             end
          else begin
             if (not scripting) then
               Result2 := 'Mortgage ' + char2 + ': APR computation did not converge.';
               end;
{
          if ((e1.rate<=e2.rate) and (e1.points<e2.points)) or ((e1.rate<e2.rate) and (e1.points<=e2.points))
          then writeln(' Mortgage ',char1,' is always better.')
          else if ((e2.rate<=e1.rate) and (e2.points<e1.points)) or ((e2.rate<e1.rate) and (e2.points<=e1.points))
          then writeln(' Mortgage ',char2,' is always better.')
}
          apr1short:=OneMonthAPR(e1); apr2short:=OneMonthAPR(e2);
          if ((apr1short<apr2short) and (apr1<=apr2)) or ((apr1short<=apr2short) and (apr1<apr2))
            then begin
            if (not scripting) then
              FinalResult := 'Mortgage ' +char1 + ' is always better.';
            apr_crossover:=0; {This number accessed by PEX files}
            end
          else if ((apr2short<apr1short) and (apr2<=apr1))  or ((apr2short<=apr1short) and (apr2<apr1))
            then begin
            if (not scripting) then
              FinalResult := 'Mortgage '+char2+' is always better.';
            apr_crossover:=0; {This number accessed by PEX files}
            end
          else if IterateToFindCrossoverAPRandTime(e1,e2, apr, apr_crossover )
             {apr_crossover is a global in PEDATA so it can be accessible to SCRIPTS.}
          then begin
               years:=trunc(apr_crossover);  months:=round(12*(apr_crossover-years)); {Already rounded down.}
               if (years=0) then timestr:=''
               else timestr:=strb(years,0)+' years';
               if (months<>0) then begin
                  if (timestr>'') then timestr:=timestr+', ';
                  timestr:=timestr+strb(months,0)+' months';
                  end;
               if (not scripting) then begin
                 FinalResult := Format( ' APR''s cross at %7.4f for duration '+timestr+'.', [100*apr]);
                 FinalResult := FinalResult + ' If mortgage is held for longer than '+timestr+',';
                 FinalResult := FinalResult + ' then Mortgage ';
                 if (apr1<apr2) then
                   FinalResult := FinalResult + char1
                 else
                   FinalResult := FinalResult + char2;
                 FinalResult := FinalResult + ' is better.';
                 end;
               end
          else begin
             if (scripting) then MessageBox(NoConvergeStr, DM_CrossoverDidNotConverge)
             else FinalResult := NoConvergeStr;
             end;
{          if (not scripting) then begin
            MessageAndCheckForPrintScreen(AnyKeyToReturn,literal,kiword);
            PopWindow;
            end;
}
          end;

(*
   FROM MAC
  procedure ReportComparisonOfAPRs (line1, line2: integer);
    var
      e1, e2: mtgline;
      apr, t: real;
      apr1, apr1short, apr2, apr2short: real;
      months, years: integer;
      timestr, ws: str80;
      ki       :char;
  begin
    e1 := e[line1 + scrollpos[MTGblock]]^;
    FirstPass(line2);
    Calc(line2);
    e2 := e[line2 + scrollpos[MTGblock]]^;
    if (errorflag) then
      begin
        Message('Insufficient data in second mortgage.',ki);
        exit;
      end;
    open(fout, APRCompName);
    rewrite(fout);
    writeln(fout, char(13), '                             Comparison of APR''s', char(13));
           {0123456789|123456789|123456789|123456789|123456789|}
    writeln(fout, '                 Rate      Points  Years     APR (if held to term)');

    with e1 do
      begin
        ws := Concat('  Mortgage #1:', ftoa4(100 * (YieldFromRate(rate, 12)), 8));
        ws := Concat(ws, ftoa4(100 * points, 10));
        ws := Concat(ws, strb(years, 8));
      end;
    if (IterateToFindAPR(e1, apr1)) then
      ws := Concat(ws, ftoa4(100 * (YieldFromRate(apr1, 12)), 8))
    else
      ws := concat(ws, char(13), ' APR computation did not converge.');
    writeln(fout, ws);

    with e2 do
      begin
        ws := Concat('  Mortgage #2:', ftoa4(100 * (YieldFromRate(rate, 12)), 8));
        ws := Concat(ws, ftoa4(100 * points, 10));
        ws := Concat(ws, strb(years, 8));
      end;
    if (IterateToFindAPR(e2, apr2)) then
      ws := Concat(ws, ftoa4(100 * (YieldFromRate(apr2, 12)), 8))
    else
      ws := concat(ws, char(13), ' APR computation did not converge.');
    writeln(fout, ws, char(13));

    apr1short := OneMonthAPR(e1);
    apr2short := OneMonthAPR(e2);
    if ((apr1short < apr2short) and (apr1 <= apr2)) or ((apr1short <= apr2short) and (apr1 < apr2)) then
      writeln(fout, ' Mortgage 1 is always better.')
    else if ((apr2short < apr1short) and (apr2 <= apr1)) or ((apr2short <= apr1short) and (apr2 < apr1)) then
      writeln(fout, ' Mortgage 2 is always better.')
    else if IterateToFindCrossoverAPRandTime(e1, e2, apr, t) then
      begin
        years := trunc(t);
        months := round(12 * (t - years)); {Already rounded down.}
        timestr := Concat(strb(years, 0), ' years');
        if (months <> 0) then
          timestr := Concat(timestr, ', ', strb(months, 0), ' months');
        writeln(fout, ' APR''s cross at ', 100 * apr : 7 : 4, ' for duration ', timestr, '.');
        writeln(fout, ' If mortgage is held for longer than ', timestr, ',');
        write(fout, '    then Mortgage #');
        if (apr1 < apr2) then
          write(fout, '1')
        else
          write(fout, '2');
        writeln(fout, ' is better.');
      end
    else
      write(fout, ' Crossover computation did not converge.');
    close(fout);
    ViewFile(APRCompName, pf.font, FontSizeFunction(pf.font), 'Comparison of APRs');
  end;

  procedure CpeMTGPane.CompareAPRs;
    var
      e1, e2: mtgline;
      line1, line2: integer;
  begin
    line1 := currentCell.v;
    e1 := e[line1 + scrollpos[MTGblock]]^;
    if (nlines[MTGblock] = 1) or ((nlines[MTGblock] > 2) and ((HilitRow.h <> MTGblock) or (HilitRow.v = line1))) then
      ReportAPR(e1)
    else
      begin
        if (nlines[MTGblock] = 2) then
          line2 := 3 - line1
        else
          line2 := HilitRow.v;
        FirstPass(line2);
        Calc(line2);
        e2 := e^[line2 + scrollpos[MTGblock]]^;
        if (e2.financedstatus>empty) and (e2.monthlystatus>empty)
           and (e2.ratestatus>empty) and (e2.yearsstatus>empty) then
          ReportComparisonOfAPRs(line1, line2);
      end;
  end; {procedure CompareAPRs}
  FROM MAC
*)
(*   Moved to PePane where CopyRowIncrementingDate can use it for RBT block
  procedure InsertLines (after_i, howmany: byte);
    var
      i,to_move :byte;
      p         :array[1..maxlines] of pointer;
  begin
    if (howmany+nlines[block]>maxlines) then begin
       Message('Not enough room for '+strb(howmany,0)+' more lines in Mortgage block.');
       exit; end;
    for i:=1 to howmany do
       if not GetMemIfAvailable(p[i],datasize[MTGblock]) {creating a new pointer for the old spot}
       then exit;
    {We seek all memory first - don't want to do anything if there's not enough
     memory for the whole job.  If we get here, there's enough memory.}
    to_move:=nlines[block]-after_i;
    for i:=to_move downto 1 do
       e[after_i+i+howmany]:=e[after_i+i];
    for i:=1 to howmany do
       e[after_i+i]:=p[i];
    nlines[block]:=nlines[block]+howmany;
  end;
*)

function EnoughDataForRowGeneration(var e :mtgline):boolean;
begin
  EnoughDataForRowGeneration := ((e.pricestatus = outp) or (e.monthlystatus = outp) or (e.howmuchstatus = outp));
end;  

{**************************************************}
{*         Generate What-if Table                 *}
{*    First get data, with WhatIf dialog          *}
{*    Then invoke CopyAndIncrement recursively    *}
{**************************************************}
{$ifdef 0}
Deals directly with the screen so it needs to be re-written

  procedure GenerateWhatIfTable;
    var
      row, i, j, linestoinsert : byte;
      n,saven       :SYSTEM.shortint;
      qcol, lines, product : array[1..3] of integer;
      increment: array[1..3] of real;
      saveline: screenLine;

    function GetWhatIfOptions:boolean;
             var c,xgo,ygo,i,totalines :byte;
                 input_ok,filled_in :boolean;
             label JUMP_OUT,READ_COLUMNS,READ_INCREMENTS,READ_NUMBER_OF_LINES;

           begin {GetWhatIfOptions}
            GetWhatIfOptions:=false;
            GetXY(xgo,ygo);
            if (ygo>19) then begin
              Message('Start higher up on the screen. ');
              exit; end;
            i:=0; repeat inc(i) until (screen^[ygo,i,1] in ['0'..'9']) or (i=81);
            if (i=81) then begin
               Message('Move cursor to a line containing data, then press <Ctrl-T> again.');
               exit;
               end;
            if (ygo>21) then begin
              Message(' Make more room on screen first.');
              exit;
              end;
            saveline:=screen^[ztop[block]];
            for c:=fcol[block] to lcol[block] do begin
                gotoxy( (startof[c]+endof[c]) div 2 , ztop[block] );
                textattr:=GetColor(outp);
                write(c - pred(fcol[block]));
                end;
            PushWindow(11,ygo+2,70,ygo+5,yellow,yellow);
            window(11,ygo+2,70,ygo+6); {Prevents scrolling}
            CenterText('WHAT-IF TABLE VARYING ONE OR MORE ENTRIES');
            READ_COLUMNS:
            repeat
              gotoxy(1,2);
              textattr:=BoxColors;
              n:=3;
              write('            Column(s) to vary (<Enter> for none): '); clreol; InputIntegers(n,@qcol,false);
              input_ok:=true; filled_in:=true;

{$ifdef NO_HELP}
              if (n<=0) or (qcol[1]=0) then goto JUMP_OUT;
{$else}
              if (n=-1) then begin
                 Help('MTG','What if');
                 goto JUMP_OUT;
                 end
              else if (n<=0) or (qcol[1]=0) then goto JUMP_OUT;
{$endif}
              if (n>3) then input_ok:=false;
              if (input_ok) then for i:=1 to n do begin
                  qcol[i]:=qcol[i]+pred(fcol[block]);
                  if (qcol[i]<fcol[block]) or (qcol[i]>lcol[block]) then input_ok:=false;
                  if (qcol[i]<fcol[block]) or (qcol[i]>lcol[block]) or (not ok(GetNumber(ygo,qcol[i],tabooli)))
                    then filled_in:=false;
                  end;
              if (not input_ok) or (not filled_in) then begin
                 gotoxy(1,2); textattr:=MessageColors; clreol;
                 if (not input_ok) then write(' Enter up to 3 column numbers.')
                 else write(' Columns you vary must contain input information.');
                 readkey;
                 end;
            until (input_ok) and (filled_in);
            saven:=n;
            READ_INCREMENTS:
            repeat
              gotoxy(1,3);
              textattr:=BoxColors;
              if (saven>1) then write('   Increments for each varied column (',saven,' numbers): ')
                           else write('                                       Increment: ');
              clreol;
              n:=saven;
              ReadReals(n,@increment,false);
{$ifndef NO_HELP}
              if (n=-1) then begin
                 Help('MTG','What if');
                 goto JUMP_OUT;
                 end
              else if (n<0) or (qcol[1]=0) then goto JUMP_OUT;
{$endif}
              if (n<>saven) then begin
                 gotoxy(1,3); textattr:=MessageColors; clreol;
                 if (saven=1) then write(' Enter one number only.')
                 else write(' Enter ',saven,' numbers - one for each of the specified columns.');
                 readkey;
                 end;
            until (n=saven);
  READ_NUMBER_OF_LINES:
            repeat
              gotoxy(1,4);
              textattr:=BoxColors;
              if (saven>1) then write(' Number of lines to write for each varied column: ')
                           else write('                        Number of lines to write: ');
              clreol;
              n:=saven;
              InputIntegers(n,@lines,false);
{$ifndef NO_HELP}
              if (n=-1) then begin
                 Help('MTG','What if');
                 goto JUMP_OUT;
                 end
              else if (n<0) or (qcol[1]=0) then goto JUMP_OUT;
{$endif}
              totalines:=1; for i:=1 to n do totalines:=totalines*succ(lines[i]);
              dec(totalines);
              if (n<>saven) or (totalines>(maxlines-(ygo-ztop[block])-scrollpos[block])) then begin
                 gotoxy(1,4); textattr:=MessageColors; clreol;
                 if (n<>saven) then begin
                   if (saven=1) then write(' Enter one number only.')
                   else write(' Enter ',saven,' numbers - one for each of the specified columns.');
                   end
                 else
                   write(' Room allocated for only ',(maxlines-(ygo-ztop[block])-scrollpos[block]),' lines in all.');
                 readkey;
                 end;
            until (n=saven) and (totalines<=(maxlines-(ygo-ztop[block])-scrollpos[block]));
            product[1]:=1;
            for i:=1 to pred(n) do product[succ(i)]:=lines[i]*product[i];
            JUMP_OUT:
            PopWindow; gotoxy(xgo,ygo);
            screen^[ztop[block]]:=saveline;
            if (n=0) or ((n=1) and (qcol[1]=0)) then InsertRowOfBlock(block,wherey,true)
            else if (n>0) then GetWhatIfOptions:=true;
            end;

    procedure CopyAndIncrement (basei, j: byte);
      var
        count: integer;
        x, basex: real;
        status: inout;
        intp: integerptr;
        realp: realptr absolute intp;
    begin
      x := RealFromArray(basei, qcol[j], status);
      basex := x;
      if (qcol[j] = mratecol) then
        basex := YieldFromRate(basex, 12);
      i := pred(i);
      for count := 0 to lines[j] do
        begin
          i := succ(i);
          if (ColType[qcol[j]] = PERCENT_FMT) then
            begin
              x := basex + 0.01 * count * increment[j];
              if (qcol[j] = mratecol) then
                x := RateFromYield(x, 12);
            end
          else
            x := basex + count * increment[j];
          e[i]^ := e[basei]^;

          intp:=AdvancePointer(e[i],dataoffset[qcol[j]]+1);
          case ColType[qcol[j]] of
            SHORT_FMT:
                intp^ := round(x);
            PERCENT_FMT, CURRENCY_FMT:
                realp^ := x;
          end;
          if (j > 1) then
            CopyAndIncrement(i, pred(j));
        end;
    end;

  begin {GenerateWhatIfTable}
    row:=wherey-ztop[block];
    i := row + scrollpos[MTGblock];
    FirstPass(row);
    if (errorflag) then
      errorflag := false
    else if (overflowflag) then
      overflowflag := false
    else
      Calc(row);
    with e[i]^ do
      if (not ((pricestatus = outp) or (monthlystatus = outp) or (howmuchstatus = outp))) then
        begin
          Message('Move cursor to a line full of data and ready to calculate, then try again.');
          exit;
        end;
    if (not GetWhatIfOptions) then exit;
    {PrepareForUndo;}
    linestoinsert:=1; for j:=1 to n do linestoinsert:=linestoinsert*succ(lines[j]);
    if (linestoinsert+nlines[MTGBlock]>=maxlines) then begin
      Message('That would be too many lines; try a smaller number, or delete some lines first.');
      exit; end;
    if not (InsertNLines(MTGBlock, i, linestoinsert)) then exit; {if InsertNLines failed to find enough memory}
    ClearRowOfBlock(MTGBlock,i+linestoinsert-scrollpos[MTGBlock],TRUE);
    CopyAndIncrement(i, n);
    UpdateScrollBar(MTGBlock);
    EnterProc[thisrun](no_tab); {calculates the whole mess and displays it}
  end;
{$endif}
  {****************************************************}
  {* Enter                                            *}
  {*                                                  *}
  {* Determine where the user wants to be next        *}
  {* with_tab means calculate current row only, then  *}
  {* move cursor.  Otherwise, calculate whole screen  *}
  {* and don't move cursor.                           *}
  {****************************************************}

{$ifdef 0}
This function has been removed.  Instead of using it just call the cases
I need directly.

{$F+}
  procedure Enter (code: byte);
{$ifndef OVERLAYS} {$F-} {$endif}
    var
      y, i1, i2, i, row: integer; {don't change to byte - negative values need to be discarded}
  begin
    case code of MAKE_TABLE : begin GenerateWhatIfTable; exit; end;
                   APR_COMP : begin CompareAPRs; exit; end;
                   WITH_TAB : begin
                              i1 := wherey + scrollpos[block] - ztop[block];
                              i2 := i1;
                              end
                   else begin
                     ClearOutputCells;    {this trashes Macintosh screen}
{
                     i1 := 1+scrollpos[block];
                     i2 := nlines[MTGblock];
                     if (i2>screenlines[MTGblock]+scrollpos[MTGBlock]) then
                       i2:=screenlines[MTGblock]+scrollpos[MTGBlock];
}
                     i1:=1; i2:=nlines[MTGBlock];
                      {Calculate even the ones that are off screen.  Otherwise,
                       you end up with junk off screen after doing a what-if table.}
                   end;
              end; {case CODE}
    y:=wherey;
    row:=y-ztop[block];
    i := row + ScrollPos[block];
  {Must move cursor before calculating because moving cursor clears complementary columns}
    if (code = with_tab) then
      begin
        if ((col = pctcol) and (e[i]^.pctstatus = inp)) or ((col = cashcol) and (e[i]^.cashstatus = inp)) then
          moveToCell(yearscol,y,startof[yearscol])
        else
          tabRt;
      end;

    errorflag := false;
    overflowflag := false;
    for i := i1 to i2 do
      begin
        row:=integer(i)-scrollpos[block];
        FirstPass(row);
        if (errorflag) then
          errorflag := false
        else if (overflowflag) then
          overflowflag := false
        else
          Calc(row);
          if (errorflag) then exit;
          if (row<=screenlines[block]) and (row>=1) then
            DisplayRowOfBlock(block,row);
      end;
  end;
{$endif}

// Below is the main entry point for calculating rows.
// First and Last are the first and last rows to be
// calculated.  Smallest value is 1.
procedure CalculateRows( First, Last: integer );
var
  i: integer;
begin
  errorflag := false;
  overflowflag := false;
  for i := First to Last do begin
    FirstPass(i);
    if (errorflag) then
      errorflag := false
    else if (overflowflag) then
      overflowflag := false
    else
      Calc(i);
      if (errorflag) then exit;
  end;
end;

{$F+}
procedure Init;
{$ifndef OVERLAYS} {$F-} {$endif}
          begin
          thisrun:=iMTG;
{$ifndef TOPMENUS}
          menuptr:=@menuline[iMTG];
{$endif}
          end;

begin

//PEData.EnterProc[iMTG]:=Enter;
//PEData.ScreenProc[iMTG]:=MortgageScreen;
//PEData.InitProc[iMTG]:=Init;
//PEData.CloseProc[iMTG]:=NullProc;
{$ifdef TESTING}
TESTHIGH.MTGEnter:=MORTGAGE.Enter;
{$endif}

end.
