unit peData;

INTERFACE

//  uses OPCRT,DOS,VIDEODAT,SCROLBAR,NORTHWND,PETYPES;
uses PETYPES, VIDEODAT, GLOBALS;
    {ùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùù}
    {ù             G  L  O  B  A  L  S                  ù}
    {ùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùùù}

// the following directive makes it so constants can be assigned to
{$J+}
const
{$ifdef PLANNER}
     thisrun=iINV;
{$endif}
{$ifdef CHEAP}
    fancycode :byte=0;
{$endif CHEAP}
     date_of_release:daterec=(d:1; m:4; y:91);
     examplemenu:str80=' EscùExit example                                                       CùCalc  ';
{$ifdef TOPMENUS}
    { menuline:str80='  F1ùHelp  Alt-F1ùExamples  F10ùMenu                                     CùCalc '; }
{$else}
     menuline:array[iPVL..iAMZ] of str80=
                   (' F1ùHelp  F10ùMenu  XùExtndedÿScr  Ctrl-AùActuarial  Ctrl-OùOutput/Prnt  CùCalc ',
                    '  F1ùHelp  F10ùMenu                                 Ctrl-OùOutput/Print  CùCalc ',
                    '  F1ùHelp  F10ùMenu   Ctrl-AùCompareÿAPRs           Ctrl-OùOutput/Print  CùCalc ',
                    '  F1ùHelp  F10ùMenu  XùExtra Options On/Off                CùCalc  Ctrl-TùTable ');
{$endif}

    mostrecentfilename:array[0..nscr] of namestr=('','','','','','','');
    impath:str80='';  { netpath:str80=''; NEVER USED }
// this is assigned to in the not cheap case, so find it again in var
{$ifdef CHEAP}
    fancy =false;
{$endif}

    pvlfancy {$ifdef PVLX}:boolean {$endif} =false;  {untyped const for consumer version}

// transplanted from iounit.pas
    destin :destinations=none;

    fancytop3=16;
    plaintop3:byte=19;
    fancyleft3=5;
// is assigned to, so made it a var
//    plainleft3:byte=15;
{$ifdef V_3}
    currentstr:str8='CURRENT '; {'CURRENTÿ' w/#255s}  constantstr:str8='CONSTANT';
{$ifndef PVLX}
    simplestr:str8=' SIMPLE '; {'ÿSIMPLEÿ' w/#255s}  compoundstr:str8='COMPOUND';
{$endif}
{$endif}
{$ifdef PVLX}
    simplestr:str8=' SIMPLE '; {'ÿSIMPLEÿ' w/#255s}  compoundstr:str8='COMPOUND';
    fancybot : array[1..3] of byte = (12,12,22);
    scrollpos3:byte=0;    swapnlines3:byte=0;
{$endif}

    needs_refresh : boolean = false;

  df:defrec=(
  version:0;  percent:'%';  lastrun:0;
  headerfilename:'NONE';
  actuarialfilename:('MALE','FEMALE');
  c:(colamonth:ANN; centurydiv:50; peryr:12; USARule:false; basis:x360;
     prepaid:true; in_advance:false; plus_regular:false; exact:false; r78 :false);
  h:(color:default_color; UserMem:64; {64K} saveheader:false;
     commas:true; bypass:false; printzeros:false;
     use_ems:true; dummy:false; hot_key:6 shl 8 + $19 {LShf Ctl P};
     swappath:'');
  p:(port:prn; baud:baud96; parity:parityN; databits:8; stopbits:1;
     dummy1:6; dummy2:7; dummy3:8; dummy4:9; eject:true; pagepause:false;
     linesperpage:66; left:0; top:6; bottom:6;
     formfeeds:true; ibmset:false;
     initstring:''; boldonstring:''; boldoffstring:''; resetstring:''; lastselected:'NO PRINTER CODES'; noprnchk :#0);
  xsimple:false; rmethod:mPERDC_CONT;
  xsettings : (0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0);
  d:(screenpath:''; textpath:'';
     lotuspath:'')
      );

// in the not CHEAP case this is a var.  Else find above in the const section
{$ifndef CHEAP}
    fancy :boolean  =false;
{$endif}

startof:colarray=(2,11,22,  33,42,51,54,64,70,  15,24,32,40,48,   {PresVal}
                    44,53,62,                                     {PVLFancy}
                    1,1,                                          {unused}
                    2,13,18,21,31,41,44,51,60,70,73, 1,           {Mortgage}
                    2,16,                                         {unused}
                    28,28,28,28,42,65,42,65,                      {Life expectancy}
                    2,11,24,32,  40,  53,66,78,                   {Chronological}
                    1,15,24,32,41,45,54,57,68,74,                 {Amortization}
                    53,62,                                        {AsOf-Balance}
                    1,1,                                          {unused}
                            32,41,45,54,57,                       {AMZ preblock}
                    10,19,42,51,60,                               {Balloon and Changes}
                    1,1,1,                                        {unused}
                    5,18,33                                       {Fancy - Moratorium, Target and Skip month}
{$ifdef V_3} ,
                    1,                                            {unused}
                    8,17,29, 38,47,56,59,71,                      {Investment, top}
                    1,1,                                          {unused}
                    8,17,29, 38,47,56,59,71,                      {Investment, bottom}
                    1,1,                                          {unused}
                    16,42,68,
                    1,                                            {unused}
                    20,34,43,51,62,65,                            {Arbitrary}
                    2,11,24,32,45,54,67,0
{$endif}
);
  endof:colarray=(9,20,31,  40,49,52,62,68,79,  22,30,38,46,58,    {PresVal}
                    51,60,74,                                      {PVLFancy}
                    1,1,                                           {unused}
                    11,16,19,29,39,42,49,58,68,71,79, 1,           {Mortgage}
                    14,23,                                         {unused}
                    28,28,28,28,49,74,49,74,                       {Life expectancy}
                    9,22,30,38,  51,  64,76,79,                    {Chronological}
                    13,22,30,39,43,52,55,66,72,80,                 {Amortization}
                    60,76,                                         {AsOf-Balance}
                    1,1,                                           {unused}
                             39,43,52,55,66,                       {AMZ preblock}
                    17,30,49,58,71,
                    1,1,1,                                         {unused}
                    12,28,47                                       {Fancy - Moratorium, Target and Skip Month}
{$ifdef V_3} ,
                    1,                                            {unused}
                    15,27,36, 45,54,57,69,78,                     {Investment, top}
                    1,1,                                          {unused}
                    15,27,36, 45,54,57,69,78,                     {Investment, top}
                    1,1,                                          {unused}
                    24,50,75,
                    1,                                            {unused}
                    32,41,49,60,63,72,                            {Arbitrary}
                    9,22,30,43,52,65,79,79
{$endif}
);
    kicker:real=365/360;

{$ifdef ACTU}
    actchar:array[NOT_CONTINGENT..BOTH_LIVING] of char=('N','L','D','1','2','E','B');
{$else}
    fold_in_life=false; {if ACTU then it's a variable.}
{$endif}

{$ifndef PVLX}
    pvlfancycode             :byte=0;  {if PVLX then it's a variable, not a const - see below}
{$endif}

  var

{$ifdef ACTU}
    actset                   :set of char;
{$endif}
    internalvx               :byte;
    dd                       :defrec;
    errorflag, overflowflag  :boolean;
    frontward,backward       :boolean;
    examplemode,scripting    :boolean;
{$ifdef ACTU}
    fold_in_life             :boolean;
{$endif}
    abort                    :boolean; {for table output}
    yrdays, yrinv            :real;
    apr_crossover            :real;
{$ifndef PLANNER}
    thisrun                  :shortint; {ranges from 1 to 4, iPVL, iCHR, iMTG, iAMZ; V_3 then iINV=5 also allowed}
{$endif}
     {MAC: not to be confused with peType, which is a variable of each CpePane, and which}
     {runs from 1024..1027 = iAMZ to iCHR}

  {The global variable col identifies what column you're in.
   It must be defined in NORTHWND, so it can be saved with PushWindow.}

    block {,saveblock}       :byte; {saveblock now in HTXTHELP, 6/92}
    lastk                    :byte; {which line of PVLPresValBlock last used to compute upper screen?}
{$ifdef MAC}
    mon                      :array[1..12] of ch3;
    monstr                   :array[0..12] of string[9];
{$endif}
    maxdate                  :daterec; {latest admissible date}

    nlines                   :blockarray;
    scrollpos                :blockarray;

    blockdata                :array[1..nblocks] of ^genericblockarray;
              {This array contains pointers to pointer arrays }
              {associated with all nblocks blocks. }

    a   : lumpsumarray;
    aa  : lumpsumarray absolute a;
    a_  : ^lumpsumarray;
    b   : periodicarray;
    bb  : periodicarray absolute b;
    b_  : ^periodicarray;
    c : presvalarray;
    cc: ratelinearray;
    c_: ^bigpresvalarray;
    d : ^xpresval;

    e: mtgarray;

    g: CHRarray;
    h: AMZptr;
    w: ^balloonrec; {asof-balance}

{$ifdef V_3}
    wa,da :ilumpsumarray;
    wb,db :iperiodicarray;
    ia  :lumpsumarray;   {  These two are data buffers for interface between }
    ib  :periodicarray;  {  INV screen I/O and PVL screen computations.      }
    u   :^irate;
    q   :^rbtloan;
    rb  :rbtarray;
{$endif}

    pre: prepaymentarray;
    balloon: balloonarray;
    adj: adjarray;
    mor: moratoriumptr;
    targ: targetptr;

    skp: skipptr;
    InputOnlyColumns: byteset;
    SpecialColumns: byteset;
    OutputOnlyColumns: byteset;
    ColumnsThatDefaultToZero: byteset;
{$ifdef V_3}
    DollarColumns: byteset;
{$ifdef RBT}
    xcolset,xmptset: set of methodtype;
{$endif}
{$endif}
    peryrset, PerYrColumns: byteset;

{$ifdef MAC}
    AcceptableChars: array[SHORT_FMT..THREE_DIGIT_FMT] of set of char;
    headerstr :array[1..nblocks] of string;
    gDocList: array[iAMZ..iCHR] of CpeDoc;
    gguidance: guidanceptr;
    headerstr: headerptr;
    gHelpDoc: cHelpDoc;
    gEditDoc: CEditDoc;
    gEditBox: CEditBox;
    gEditWindow: CEditWindow;
    gpeMTGpane: CpeMTGpane;
    gpePVLpane: CpePVLpane;
    gpeAMZpane: CpeAMZpane;
    gpeCHRpane: CpeCHRpane;
    gTableDoc: cTableDoc;
    CurrentPEpane: CPePane;
    gIUH: Intl0Hndl;    {* handle to the international utilities record *}
    gDateStrings: Intl1Hndl;{* Month and Day names from Int'l package   *}
{$endif}

    ColWidth :colArray;  {* width of each column in CHARACTERS! *}
    Fcol,Lcol :blockarray;
    lleft,rright :blockarray;
    ztop,bbot,defbot : blockarray;
    datasize: blockarray;
    dataoffset: colarray;
    scrolls: array[1..nblocks] of boolean;
    colType: colarray;
    lineCount: blockarray;        {* number of data lines appearing on screen   *}
    screenlines :blockarray absolute lineCount; {a synonym}

//    watch_me                               :real absolute 0000:0000;
    screenstatus                           :byte;

    pedone,tabooli                         :boolean;
{$ifndef CHEAP}
    fancycode                              :byte absolute fancy;
{$endif}
{$ifdef PVLX}
    pvlfancycode                           :byte absolute pvlfancy;
{$endif}
    {1..3 for PresVal, 5 for Mortgage, 7,8 for Compound Screeen, 10..13 for Amortization}
              {4 for Fancy PresVal }
    menuptr {,menu2ptr}                    :^str80;
    ph                                     :array[1..nscr] of placeholder;
    aprpmtsum                              :real;

{These are included even in non-Pro version because they are written
 out to file.}
    actu_now,termdate                      :daterec;
    dob                                    :array[1..2] of daterec;
    pod,podval                             :real;
{$ifdef ACTU}
    deathdate                              :daterec;
{$endif}

    EnterProc,ScreenProc                   :array[0..nscr] of codeproctype;
    InitProc,CloseProc                     :array[0..nscr] of no_params;
    PVLPlainFancy,AMZPlainFancy            :no_params; {Pointer to procedures in PVLXSCRN and AMZUTIL}

// moved here from const section
    plainleft3:byte=15;


{$ifdef MAC}
  procedure peDataInit;  only need to interface this in MAC version}
{$endif}

  procedure SetWhichBlocksScroll;
     {All blocks don't scroll in example mode.  This is how to restore.}

function Fblock(which :byte) :byte;
function Lblock(which :byte) :byte;
function ScreenExt(which :byte):str3;
function PctExt(which :byte):str3;
function WhichRun(ch :char):byte;
function WhichFancyCode(which :byte):byte;
procedure NullProc;
function VeryLCol(block :byte):byte;
{$ifdef COPROC}
procedure ConvertDoublesInLineToReals(block,i :byte; DiskData :pointer);
procedure ConvertRealsInLineToDoubles(block,i :byte; DiskData :pointer);
function LineSizeOnDisk(block :byte):byte;
{$endif}

IMPLEMENTATION

{$F+}
  procedure NullScreenProc(code :byte); begin end;
  procedure NullProc; begin end;
  procedure SetThisRunToZero; begin {$ifndef PLANNER} thisrun:=0; {$endif} end;
{$ifndef OVERLAYS} {$F-} {$endif}

{$ifdef MAC}
  procedure SetAcceptableChars;
  begin
    AcceptableChars[SHORT_FMT] := ['0'..'9'];
    AcceptableChars[CURRENCY_FMT] := ['0'..'9', ',', '.'];
    AcceptableChars[PERCENT_FMT] := ['0'..'9', '.'];
    AcceptableChars[DATE_FMT] := ['0'..'9', '-', '/'];
    AcceptableChars[THREE_DIGIT_FMT] := ['0'..'9'];
  end;
{$endif}

  procedure SetColumnSets;
  begin
    InputOnlyColumns := [timescol, colacol, simplecol, xasofcol, pointscol, yearscol, taxcol, whencol, vperyrcol,
       loandatecol, firstdatecol, aperyrcol, apointscol, adjdatecol, {adjaprcol, NO LONGER, 3/92} prefirstdatecol,
       preperyrcol, int_only_tilcol, targetcol
       {$ifdef V_3} ,wdollar0col,ddollar0col,wdollarncol,ddollarncol,
                     rloanamtcol,rloandatecol,rloanratecol,rmethodcol,rperyrcol,rfirstdatecol,
                     rratecol,rchargescol {$endif}
       ];
    OutputOnlyColumns := [vinterestcol, aaprcol {$ifdef V_3} , rintpartcol, rprincpartcol, rexemptcol {$endif}];
       { 4/92 tried to make lumpsumvalues and periodic values output only - users complained}
    ColumnsThatDefaultToZero := [pointscol, taxcol, colacol, vperyrcol, apointscol{$ifdef V_3},rchargescol,rexemptcol{$endif}];
                                            {rexemptcol is in here so it never has an un-initialized number in it,
                                             which can cause RT error 205}
{$ifdef V_3}
    DollarColumns := [ddollar0col, wdollar0col, ddollarncol, wdollarncol, constdatecol];
{$ifdef RBT}
    xcolset:=[mSKIP_PMT,mPERIODIC]; {methods that require extra columns in RBT Top block}
    xmptset:=[mUS_RULE,mPERIODIC,mP_BEFORE_I]; {methods that require exempt column in RBT block}
{$endif}
    ColumnsThatDefaultToZero := ColumnsThatDefaultToZero + DollarColumns + [inflationcol];
{$endif}
    SpecialColumns := [sumpmtcol,aprxcol,lineselectcol,dob1col,fileselect1col,dob2col,fileselect2col,nowcol,podcol,termcol];

  end;

{$ifdef MAC}
  procedure SetHeaderStrings;
  begin
  headerstr[PVLlumpsumblock] := Concat('Single Payments', CR, tab, 'Date', tab, 'Amount', tab, 'Value');
  headerstr[PVLperiodicblock] := Concat('Periodic Payments', CR, tab, 'From', tab, 'Through ', tab, 'PerYr', tab, 'Amount',
                                 tab, 'COLA', tab, 'Value');
  headerstr[PVLpresvalblock] := Concat(tab, tab, 'True  ', tab, 'Loan  ', tab, tab, 'Total', CR, tab, 'As of', tab, 'Rate %',
                                       tab, 'Rate %', tab, 'Yield %', tab, 'Value', CR, CR, 'Present', CR, 'Value');
  headerstr[MTGblock] := Concat(tab, tab, tab, '%', tab, 'Cash', tab, 'Amount', tab, tab, 'Loan', tab, 'Monthly', tab,
                                'Monthly', tab, '       Balloon');
  headerstr[MTGblock] := Concat(headerstr[MTGblock], CR, tab, 'Price', tab, 'Pts', tab, 'Down', tab, 'Required',
                         tab, 'Financed', tab, 'Yrs', tab, 'Rate', tab, 'Tax+Ins', tab, 'Total', tab, 'Yrs', tab, ' Amount ');
  headerstr[CHRblock] := Concat(tab, tab, tab, ' True ', tab, ' Loan ', tab, ' Single Deposit ', tab, tab,
                         ' Periodic ', tab, ' Per ');
  headerstr[CHRblock] := Concat(headerstr[CHRblock], CR, tab, 'Date', tab, 'Principal $', tab, 'Rate %', tab,
                         'Rate %', tab, 'or Sum $', tab, 'Interest $', tab, 'Deposit $', tab, 'Yr');
  headerstr[AMZTargetBlock] := Concat(tab, 'Targeted', CR, tab, 'Principal', CR, tab, 'Reduction');
  headerstr[AMZStatusBlock] := 'Settings in force';
  end;


  procedure SetGuidanceStrings;
  begin
    gguidance := guidancehand(NewHandle(ncols * sizeof(guidancestr)));
    gguidance^^[pricecol] := 'Purchase price of the property';
    gguidance^^[pointscol] := 'Points, an up-front one-time charge by bank';
    gguidance^^[pctcol] := 'Downpayment percentage (or enter Cash Required or Amount Financed)';
    gguidance^^[cashcol] := 'Cash available for settlement (or enter % Down or Amount Financed)';
    gguidance^^[financedcol] := 'Amount financed (or enter % Down or Amount Financed)';
    gguidance^^[yearscol] := 'Life of mortgage, in years';
    gguidance^^[mratecol] := 'Loan rate charged by lender';
    gguidance^^[taxcol] := 'Any monthly payments in addition to principal and interest (optional)';
    gguidance^^[monthlycol] := 'Total monthly payment, including tax and insurance';
    gguidance^^[whencol] := 'Years from settlement to balloon payment (optional)';
    gguidance^^[howmuchcol] := 'Amount of balloon payment (optional)';

    gguidance^^[datecol] := 'Date of a one-time payment';
    gguidance^^[amountcol] := 'Amount of a one-time payment';
    gguidance^^[valuecol] := 'Value of this payment, as of the date specified at bottom of screen';
    gguidance^^[fromcol] := 'Starting date of a set of periodic payments';
    gguidance^^[tocol] := 'Ending date of a set of periodic payments';
    gguidance^^[timescol] := 'Number of times per year';
    gguidance^^[pamountcol] := 'Amount of periodic payments';
    gguidance^^[colacol] := 'Cost of Living Adjustment - % yearly increase in periodic payments (optional)';
    gguidance^^[pvaluecol] := 'Value of this set of periodic payments, as of date specified at bottom of screen';
    gguidance^^[asofcol] := 'Date on which you compute present value.  (For loan: enter settlement date here)';
    gguidance^^[tratecol] := 'True interest rate, before any compounding';
    gguidance^^[lratecol] := 'Loan interest rate,  with monthly compounding already accounted for';
    gguidance^^[yieldcol] := '1-year yield of the rate on this line';
    gguidance^^[sumvaluecol] := 'Total value of all payments listed in the top two blocks';

    gguidance^^[vdatecol] := 'Date of payment, or start date for periodic pmts.  For end date, should be AFTER last pmt.';
    gguidance^^[vprincipalcol] := 'Amount of loan, deposit, withdrawal or payment.  (Use - for loan or withdrawal.).';
    gguidance^^[vratecol] := 'True interest rate, before any compounding.';
    gguidance^^[vaprcol] := 'Loan interest rate,  with monthly compounding already accounted for';
    gguidance^^[vsumcol] := 'A single pmt amount, if PerYr column is empty, or simple sum of periodic pmts otherwise.';
    gguidance^^[vdepositcol] := 'Amount paid periodically.  If you use this column, then "Per Yr" column should be filled in.';
    gguidance^^[vperyrcol] := 'Times per year, for periodic payment.  Leave blank for lump sum payments or deposits';

    gguidance^^[aamountcol] := 'Amount of the loan.';
    gguidance^^[loandatecol] := 'Date of closing.';
    gguidance^^[aratecol] := 'Loan interest rate.';
    gguidance^^[firstdatecol] := 'Date of first regular payment on the loan.';
    gguidance^^[pdnumcol] := 'Number of regular interest periods for repayment.';
    gguidance^^[lastdatecol] := 'Date of last regular payment on the loan.';
    gguidance^^[aperyrcol] := 'Number of times per year regular payments are made.';
    gguidance^^[paymentcol] := 'Amount of each regular payment.';
    gguidance^^[apointscol] := 'Enter points charge here if you want APR computed, next column.';
    gguidance^^[aaprcol] := 'APR computed using the points charge you specified.   Output only.';

    gguidance^^[prefirstdatecol] := 'Date of first of a series of extra payments (or skipped payments).';
    gguidance^^[prepdnumcol] := 'Number of times extra payments (or skipped payments) in this series.';
    gguidance^^[prelastdatecol] := 'Date of last in this series of extra payments (or skipped payments).';
    gguidance^^[prepdnumcol] := 'Number of times per year for extra payments (or skipped payments)';
    gguidance^^[prepaymentcol] := 'Amount of extra payments in this series (0 for skipped payments)';

    gguidance^^[balloondatecol] := 'Date of a balloon (lump sum) payment.';
    gguidance^^[balloonamtcol] := 'Amount of a balloon (lump sum) payment.';

    gguidance^^[adjdatecol] := 'Change shows up in the next period FOLLOWING this date.';
    gguidance^^[adjratecol] := 'New interest rate.  (You may leave blank if unchanged.)';
    gguidance^^[adjamtcol] := 'New payment amount.';

    gguidance^^[int_only_til_col] := 'Interest-only will be paid before this date.';
    gguidance^^[targetcol] := 'Payments will be increased if necessary so that principal portion is at least this amount.'
  end;
{$endif}

procedure SetTopsAndBottoms;
    var block :byte;
    begin
    ztop[PVLlumpsumBlock] := 5;
    ztop[PVLperiodicBlock] := ztop[PVLLumpSumBlock];
    ztop[PVLpresvalblock] := plaintop3; {19}
    ztop[PVLXBlock] := 20;
    ztop[ActuarialBlock]:=0;

    ztop[MTGblock] := 5;

    ztop[CHRblock] := 5;

    ztop[AMZtopblock] := 5;
    ztop[AMZBalanceBlock]:=21;
    ztop[AMZPreBlock]:=6;
    ztop[AMZballoonblock] :=12;
    ztop[AMZratechangeblock] :=12;
    ztop[AMZMoratoriumblock] :=21;
    ztop[AMZSkipMonthBlock] :=21;
    ztop[AMZTargetBlock] :=21;

{$ifdef V_3}
    ztop[INVDLumpSumBlock]:=4;
    ztop[INVDPeriodicBlock]:=ztop[INVDLumpSumBlock];
    ztop[INVWLumpSumBlock]:=2*ztop[INVDLumpSumBlock]+screenlines[INVDLumpSumBlock];
    ztop[INVWPeriodicBlock]:=ztop[INVWLumpSumBlock];
    ztop[INVRatesBlock]:=22;

    ztop[RBTTopBlock]:=5;
    ztop[RBTBlock]:=11;
{$endif}
    for block:=1 to nblocks do if (blockdata[block]<>nil) then begin
       defbot[block]:=ztop[block]+linecount[block];
       lleft[block]:=startof[fcol[block]];
       rright[block]:=endof[lcol[block]];
       end;
    plainleft3:=lleft[3];

    bbot:=defbot;
    end;

  procedure SetColTypes;
    var c :byte;
        col: integer;
  begin
    for c:=1 to ncols do coltype[c]:=unused_fmt;
    coltype[pricecol] := currency_fmt;
    coltype[pointscol] := percent_fmt;
    coltype[pctcol] := percent_fmt;
    coltype[cashcol] := currency_fmt;
    coltype[financedcol] := currency_fmt;
    coltype[yearscol] := short_fmt;
    coltype[mratecol] := percent_fmt;
    coltype[taxcol] := currency_fmt;
    coltype[monthlycol] := currency_fmt;
    coltype[whencol] := short_fmt;
    coltype[howmuchcol] := currency_fmt;

    coltype[datecol] := date_fmt;
    coltype[amountcol] := currency_fmt;
    coltype[valuecol] := currency_fmt;
    coltype[fromcol] := date_fmt;
    coltype[tocol] := date_fmt;
    coltype[timescol] := short_fmt;
    coltype[pamountcol] := currency_fmt;
    coltype[colacol] := percent_fmt;
    coltype[pvaluecol] := currency_fmt;
    coltype[asofcol] := date_fmt;
    coltype[tratecol] := percent_fmt;
    coltype[lratecol] := percent_fmt;
    coltype[yieldcol] := percent_fmt;
    coltype[sumvaluecol] := currency_fmt;

    coltype[xasofcol] := date_fmt;
    coltype[simplecol] := string_fmt;
    coltype[xvaluecol] := currency_fmt; {These 3 for PVLX only}

    coltype[lineselectcol]:=string_fmt;
    coltype[fileselect1col]:=string_fmt;
    coltype[fileselect2col]:=string_fmt;
    coltype[dob1col]:=date_fmt;
    coltype[dob2col]:=date_fmt;
    coltype[nowcol]:=date_fmt;
    coltype[podcol]:=currency_fmt;
    coltype[deathcol]:=date_fmt;
    coltype[termcol]:=date_fmt;  {These for Actuarial window}

    coltype[vdatecol] := date_fmt;
    coltype[vprincipalcol] := currency_fmt;
    coltype[vratecol] := percent_fmt;
    coltype[vaprcol] := percent_fmt;
    coltype[vsumcol] := currency_fmt;
    coltype[vinterestcol] := currency_fmt;
    coltype[vdepositcol] := currency_fmt;
    coltype[vperyrcol] := short_fmt;

    // I'm guessing amortization screen
    coltype[aamountcol] := currency_fmt;
    coltype[loandatecol] := date_fmt;
    coltype[aratecol] := percent_fmt;
    coltype[firstdatecol] := date_fmt;
    coltype[pdnumcol] := three_digit_fmt;
    coltype[lastdatecol] := date_fmt;
    coltype[aperyrcol] := short_fmt;
    coltype[paymentcol] := currency_fmt;
{coltype[methodcol] := string_fmt; from PE version 1}
    coltype[apointscol] := percent_fmt;
    coltype[aaprcol] := percent_fmt;
    coltype[aasofcol] := date_fmt;
    coltype[abalancecol] := currency_fmt;

    for col:=prefirstdatecol to prepaymentcol do
      coltype[col] := coltype[col+firstdatecol-prefirstdatecol];

    coltype[balloondatecol] := date_fmt;
    coltype[balloonamtcol] := currency_fmt;

    coltype[adjdatecol] := date_fmt;
    coltype[adjratecol] := percent_fmt;
    coltype[adjamtcol] := currency_fmt;

    coltype[int_only_til_col] := date_fmt;
    coltype[targetcol] := currency_fmt;
    coltype[skipmonthcol] := str15_fmt;

{$ifdef V_3}
    coltype[ddatecol] := date_fmt;
    coltype[damountcol] := currency_fmt;
    coltype[ddollar0col] := string_fmt;
    coltype[dfromcol] := date_fmt;
    coltype[dtocol] := date_fmt;
    coltype[dtimescol] := short_fmt;
    coltype[dpamountcol] := currency_fmt;
    coltype[ddollarncol] := string_fmt;
    coltype[wdatecol] := date_fmt;
    coltype[wamountcol] :=currency_fmt;
    coltype[wdollar0col] := string_fmt;
    coltype[wfromcol] := date_fmt;
    coltype[wthrucol] := date_fmt;
    coltype[wtimescol] := short_fmt;
    coltype[wpamountcol] := currency_fmt;
    coltype[wdollarncol] := string_fmt;
    coltype[iratecol] := percent_fmt;
    coltype[inflationcol] := percent_fmt;
    coltype[constdatecol] := date_fmt;

    // maybe present value screen?
    coltype[rloanamtcol] := currency_fmt;
    coltype[rloandatecol] := date_fmt;
    coltype[rloanratecol] := percent_fmt;
    coltype[rmethodcol] := string_fmt;
    coltype[rperyrcol] := short_fmt;
    coltype[rfirstdatecol] := date_fmt;

    coltype[rdatecol] := date_fmt;
    coltype[rratecol] := pct_fmt;
    coltype[rpaymentcol] := currency_fmt;
    coltype[rintpartcol] := currency_fmt;
    coltype[rchargescol] := currency_fmt;
    coltype[rprincpartcol] := currency_fmt;
    coltype[rnewprinccol] := currency_fmt;
    coltype[rexemptcol] := currency_fmt;

{$endif}
  end;

  procedure SetFColandLCol;
    var
      block: shortint;
  begin
    for block := 0 to nblocks do begin
      Fcol[block] := 255; {so that the unused ones don't catch FindBlock}
      Lcol[block] := 0;
      end;

    Fcol[PVLlumpsumblock] := datecol;
    Fcol[PVLperiodicblock] := fromcol;
    Fcol[PVLpresvalblock] := asofcol;
    Fcol[PVLXBlock] := xasofcol;
    Fcol[ActuarialBlock] := lineselectcol;

    Fcol[MTGblock] := pricecol;

    Fcol[CHRblock] := vdatecol;

    Fcol[AMZtopblock] := aamountcol;
    Fcol[AMZBalanceBlock] := aasofcol;
    Fcol[AMZballoonblock] := balloondatecol;
    Fcol[AMZratechangeblock] := adjdatecol;
    Fcol[AMZpreblock] := prefirstdatecol;
    Fcol[AMZMoratoriumblock] := int_only_tilcol;
    Fcol[AMZTargetBlock] := targetcol;
    Fcol[AMZSkipMonthBlock] := skipmonthcol;

    Lcol[PVLlumpsumblock] := valuecol;
    Lcol[AMZBalanceBlock] := balancecol;
    Lcol[PVLperiodicblock] := pvaluecol;
    Lcol[PVLpresvalblock] := sumvaluecol;
    Lcol[PVLXBlock] := xvaluecol;
    Lcol[ActuarialBlock] := podcol;

    Lcol[MTGblock] := howmuchcol;

    Lcol[CHRblock] := vperyrcol;

    Lcol[AMZtopblock] := aaprcol;
    Lcol[AMZballoonblock] := balloonamtcol;
    Lcol[AMZratechangeblock] := adjamtcol;
    Lcol[AMZpreblock] := prepaymentcol;
    Lcol[AMZMoratoriumblock] := int_only_tilcol;
    Lcol[AMZTargetblock] := targetcol;
    Lcol[AMZSkipMonthBlock] := skipmonthcol;

{$ifdef V_3}
    Fcol[INVDLumpSumBlock]:=ddatecol;
    Fcol[INVDPeriodicBlock]:=dfromcol;
    Fcol[INVWLumpSumBlock]:=wdatecol;
    Fcol[INVWPeriodicBlock]:=wfromcol;
    Fcol[INVRatesBlock]:=iratecol;
    Fcol[RBTTopBlock]:=rloanamtcol;
    Fcol[RBTBlock]:=rdatecol;
    Lcol[INVDLumpSumBlock]:=ddollar0col;
    Lcol[INVDPeriodicBlock]:=ddollarncol;
    Lcol[INVWLumpSumBlock]:=wdollar0col;
    Lcol[INVWPeriodicBlock]:=wdollarncol;
    Lcol[INVRatesBlock]:=constdatecol;
    Lcol[RBTTopBlock]:=rfirstdatecol;
    Lcol[RBTBlock]:=rexemptcol;
{$endif}

  end;

{$ifdef MAC}
  procedure SetDefaults;
  begin
    with df do
      begin
        version := version;
        colamonth := ANN;
        centurydiv := 50;
        peryr := 12;
        USARule := false;
        basis := x360;
        R78 := false;
        prepaid := true;
        in_advance := false;
        plus_regular := true;
        exact := false;
      end;
    with pf do
      begin
        large := false;
        OpenLast := 0;
        sl := '/';
        font := Geneva;
        commas := true;
        default_fancy := false;
      end;
  end;

  procedure SetMonthStrings;
    var
      i, j: integer;
  begin
    monstr[1] := 'January';
    monstr[2] := 'February';
    monstr[3] := 'March';
    monstr[4] := 'April';
    monstr[5] := 'May';
    monstr[6] := 'June';
    monstr[7] := 'July';
    monstr[8] := 'August';
    monstr[9] := 'September';
    monstr[10] := 'October';
    monstr[11] := 'November';
    monstr[12] := 'December';
    for i := 1 to 12 do
      for j := 1 to 3 do
        mon[i, j] := monstr[i, j];
  end;
{$endif}

  procedure SetAssortedJunk;
    var
      i: integer;
  begin

    maxdate.y := 199;
    maxdate.m := 12;
    maxdate.d := 31;

    for i := 1 to nblocks do
      begin
        scrollpos[i] := 0;
        nlines[i] := 0;
      end;
  end;

  procedure SetWhichBlocksScroll;
  var bx   :byte;
  begin
    for bx:=1 to nblocks do
      scrolls[bx]:=false;  {other than those listed}

{$ifdef SCROLLS}
    scrolls[PVLLumpSumBlock] := true;  {PVL}
    scrolls[PVLPeriodicBlock] := true;
    {scrolls[PVLPresValBLock] := false;}
    {scrolls[PVLXBLock] := false;}

    scrolls[MTGBlock] := true; {MTG}

    scrolls[CHRBlock] := true; {CHR}

    {scrolls[AMZTopBlock] := false;}    {AMZ}
    {scrolls[AMZBalanceBlock :=false;}
    {scrolls[AMZPreBlock] := false;}
    scrolls[AMZBalloonBlock] := true;
    scrolls[AMZChangesBlock] := true;
    {scrolls[AMZMoratoriumBlock] := false;}
    {scrolls[AMZTargetBlock] := false;}

    scrolls[INVDLumpSumBlock]:=true;   {INV}
    scrolls[INVDPeriodicBlock]:=true;
    scrolls[INVWLumpSumBlock]:=true;
    scrolls[INVWPeriodicBlock]:=true;

{$endif SCROLLS}
{$ifdef V_3}
    scrolls[RBTBlock] := true; {always - this screen useless w/o scrolling}
{$endif}
  end;


  procedure SetBlockDataArray;
  var bx :byte;
  begin
    a_:=@a;  b_:=@b;  c_:=@c;
    for bx:=1 to nblocks do
      blockdata[bx]:=nil; {all except those used}
    blockdata[PVLLumpSumBlock] := @a;
    blockdata[PVLPeriodicBlock] := @b;
    blockdata[PVLPresValBlock] := @c;
    blockdata[PVLXBlock]:=@d;
    blockdata[MTGblock] := @e;
    blockdata[CHRblock] := @g;
    blockdata[AMZTopBlock] := @h;
    blockdata[AMZBalanceBlock] := @w;
    blockdata[AMZPreBlock] := @pre;
    blockdata[AMZBalloonBlock] := @balloon;
    blockdata[AMZAdjBlock] := @adj;
    blockdata[AMZMoratoriumBlock] := @Mor;
    blockdata[AMZTargetBlock] := @Targ;
    blockdata[AMZSkipMonthBlock] := @Skp;
{$ifdef V_3}
    blockdata[INVdLumpSumBlock] := @da;
    blockdata[INVdPeriodicBlock] := @db;
    blockdata[INVwLumpSumBlock] := @wa;
    blockdata[INVwPeriodicBlock] := @wb;
    blockdata[INVRatesBlock]:=  @u;

    blockdata[RBTTopBlock]:=  @q;
    blockdata[RBTBlock]:=  @rb
{$endif}
  end;

  procedure SetPointersToNil;
  var bx,i,imax :shortint;
  begin
    for bx:=1 to nblocks do
      if (blockdata[bx]<>nil) then begin
        if (scrolls[bx]) then imax:=maxlines
        else imax:=screenlines[bx];
        for i:=1 to imax do
          blockdata[bx]^[i]:=nil;
        end;
    for i:=1 to maxlines do cc[i]:=nil;
     {because blockdata[3] points to c, not cc.}
  end;

  procedure SetDataLineSizes;
  begin
    datasize[PVLLumpSumBlock] := sizeof(lumpsum);
    datasize[PVLPeriodicBlock] := sizeof(periodic);
    datasize[PVLPresValBlock] := sizeof(presval);
    datasize[PVLXBlock]:= sizeof(xpresval);
    datasize[MTGblock] := sizeof(mortgageline);
    datasize[CHRblock] := sizeof(chrline);
    datasize[AMZTopBlock] := sizeof(AMZLoan);
    datasize[AMZBalanceBlock] := sizeof(balloonrec);
    datasize[AMZPreBlock] := sizeof(prepaymentrec);
    datasize[AMZBalloonBlock] := sizeof(balloonrec);
    datasize[AMZAdjBlock] := sizeof(adjrec);
    datasize[AMZMoratoriumBlock] := sizeof(MoratoriumRec);
    datasize[AMZTargetBlock] := sizeof(TargetRec);
    datasize[AMZSkipMonthBlock] := sizeof(SkipRec);
{$ifdef V_3}
    datasize[INVDLumpSumBlock]:=sizeof(iLumpSum);
    datasize[INVWLumpSumBlock]:=sizeof(iLumpSum);
    datasize[INVDPeriodicBlock]:=sizeof(iPeriodic);
    datasize[INVWPeriodicBlock]:=sizeof(iPeriodic);
    datasize[INVRatesBlock]:=sizeof(irate);
    datasize[RBTTopBlock]:=sizeof(RBTLoan);
    datasize[RBTBlock]:=sizeof(RBTLine);
{$endif}
  end;

  procedure SetPerYrSet;
  begin
    peryrset := [1, 2, 3, 4, 6, 12, 24, 26, 52];
    PerYrColumns := [timescol, vperyrcol, aperyrcol, preperyrcol {$ifdef V_3}, dtimescol, wtimescol, rperyrcol {$endif}];
  end;

  procedure SetDataOffsets;

     function PercentDataSize(col :byte):shortint;
              begin
              case col of
{$ifdef V_3}
                 riratecol,rratecol,
{$endif}
                 colacol,mratecol,pctcol,pointscol,aratecol,apointscol,aaprcol,adjratecol :
                      PercentDataSize:=sizeof(real);
{$ifdef V_3}
//                 iratecol,inflationcol,rratecol,rloanratecol,
                 iratecol,inflationcol,
{$endif}
                 vaprcol,yieldcol :
                      PercentDataSize:=sizeof(real) + sizeof(byte); {peryr byte in raterec}
                 tratecol,lratecol,vratecol :
                      PercentDataSize:=-sizeof(inout);
                 else begin
                      writeln('Column unaccounted for: ',col);
//                      readkey;
                      end;
                 end; {case}
              end;

    procedure DataOffsetsForOneBlock(block :byte);
              var col,size_of_last : integer;
              begin
              dataoffset[fcol[block]]:=0;
              for col:=succ(fcol[block]) to lcol[block] do begin
                 case coltype[pred(col)] of
                    CURRENCY_FMT : size_of_last:=sizeof(real);
                    PERCENT_FMT  : size_of_last:=PercentDataSize(pred(col));
                    DATE_FMT : size_of_last:=sizeof(daterec);
    SHORT_FMT,THREE_DIGIT_FMT: size_of_last:=sizeof(integer);
                   STRING_FMT: size_of_last:=sizeof(boolean); {simplecol and rsimplecol}
                    else begin
                         writeln('Bad format type : col ',pred(col));
//                         readkey;
                         end;
                    end;
                 dataoffset[col]:=dataoffset[pred(col)] + size_of_last + sizeof(inout);
                 end;
              end;

  var col : integer;
  begin
    DataOffsetsForOneBlock(PVLLumpSumBlock);
    DataOffsetsForOneBlock(PVLPeriodicBlock);
    DataOffsetsForOneBlock(PVLPresValBlock);
    DataOffsetsForOneBlock(PVLXBlock); 

    DataOffsetsForOneBlock(CHRBlock);

    DataOffsetsForOneBlock(MTGBlock);

    DataOffsetsForOneBlock(AMZTopBlock);
    DataOffsetsForOneBlock(AMZBalanceBlock);
    for col:=fcol[AMZPreBlock] to lcol[AMZPreBlock] do
      dataoffset[col]:=dataoffset[col-fcol[AMZPreBlock]+firstdatecol] - dataoffset[firstdatecol];
    DataOffsetsForOneBlock(AMZBalloonBlock);
    DataOffsetsForOneBlock(AMZAdjBlock);
{$ifdef V_3}
    DataOffsetsForOneBlock(INVWLumpSumBlock);    
    DataOffsetsForOneBlock(INVDLumpSumBlock);
    DataOffsetsForOneBlock(INVWPeriodicBlock);    
    DataOffsetsForOneBlock(INVDPeriodicBlock);
    DataOffsetsForOneBlock(INVRatesBlock);

    DataOffsetsForOneBlock(RBTTopBlock);
    DataOffsetsForOneBlock(RBTBlock);
{$endif}
    dataOffset[fcol[AMZMoratoriumBlock]]:=0;
    dataOffset[fcol[AMZTargetBlock]]:=0;
 end;

{$ifdef MAC}
   procedure SetColumnWidths;
       var i: integer;
       begin
       for i := 1 to ncols do
          ColWidth[i] := 0;

          {Mortgage}
    colwidth[pricecol] := currencyw;
    colwidth[pointscol] := pointsw;
    colwidth[pctcol] := 3;
    colwidth[cashcol] := currencyw;
    colwidth[financedcol] := currencyw;
    colwidth[yearscol] := shortw;
    colwidth[mratecol] := percentw;
    colwidth[taxcol] := taxw;
    colwidth[monthlycol] := currencyw;
    colwidth[whencol] := 3;
    colwidth[howmuchcol] := balloonw;

{Present Value}
    colwidth[datecol] := datew;
    colwidth[amountcol] := currencyw;
    colwidth[valuecol] := currencyw;
    colwidth[fromcol] := datew;
    colwidth[tocol] := datew;
    colwidth[timescol] := shortw;
    colwidth[pamountcol] := currencyw;
    colwidth[colacol] := percentw;
    colwidth[pvaluecol] := currencyw;
    colwidth[asofcol] := datew;
    colwidth[tratecol] := percentw;
    colwidth[lratecol] := datew;
    colwidth[yieldcol] := datew;
    colwidth[sumvaluecol] := currencyw;

{$ifdef PVLX}
    colwidth[xasofcol] := datew;
    colwidth [ simplecol ] :=;
    colwidth[xvaluecol] := shortw; {These 3 for PVLX only}
{$endif}

{Chronological}
    colwidth[vdatecol] := datew;
    colwidth[vprincipalcol] := currencyw + 2;
    colwidth[vratecol] := percentw + 1;
    colwidth[vaprcol] := percentw + 1;
    colwidth[vsumcol] := currencyw + 2;
    colwidth[vinterestcol] := currencyw + 2;
    colwidth[vdepositcol] := currencyw + 2;
    colwidth[vperyrcol] := shortw;

{Amortization}
    colwidth[aamountcol] := currencyw;
    colwidth[loandatecol] := datew;
    colwidth[aratecol] := percentw;
    colwidth[firstdatecol] := datew;
    colwidth[pdnumcol] := 3;
    colwidth[lastdatecol] := datew;
    colwidth[aperyrcol] := shortw;
    colwidth[paymentcol] := currencyw;
    colwidth[apointscol] := pointsw;
    colwidth[aaprcol] := percentw;

    colwidth[aasofcol] := datew;
    colwidth[balancecol] := currencyw;

    colwidth[balloondatecol] := datew;
    colwidth[balloonamtcol] := currencyw;

    colwidth[adjdatecol] := datew;
    colwidth[adjratecol] := percentw;
    colwidth[adjamtcol] := currencyw;

    colwidth[prefirstdatecol] := datew;
    colwidth[prepdnumcol] := 3;
    colwidth[prelastdatecol] := datew;
    colwidth[preperyrcol] := shortw;
    colwidth[prepaymentcol] := currencyw;

    colwidth[int_only_tilcol] := datew;
    colwidth[targetcol] := currencyw;
    colwidth[statuscol] := 45;
  end;

{$else}
   procedure SetColumnWidths;
       var col      :byte;
       begin
       for col := 1 to ncols do
         colwidth[col]:=succ(endof[col])-startof[col];
       end;
{$endif}

  procedure SetLineCounts;
     var bx :byte;
  begin

    for bx:=1 to nblocks do begin 
      {Define them all, just to avoid range errors later on.}
      linecount[bx]:=0;
      defbot[bx]:=0;
      ztop[bx]:=0;
      nlines[bx]:=0
      end;

    LineCount[PVLlumpsumblock] := 10;
    LineCount[PVLperiodicblock] := linecount[PVLLumpSumBlock];
    LineCount[PVLpresvalblock] := 3;
    LineCount[PVLXBlock]:=1;

    LineCount[MTGblock] := 18;

    LineCount[CHRblock] := 18;

    LineCount[AMZtopblock] := 1;
    LineCount[AMZBalanceBlock] := 1;
    LineCount[AMZballoonblock] := 6;
    LineCount[AMZpreblock] := 2;
    LineCount[AMZratechangeblock] := 6;
    LineCount[AMZMoratoriumBlock] := 1;
    LineCount[AMZTargetBlock] := 1;
    LineCount[AMZSkipMonthBlock] := 1;

{$ifdef V_3}
    LineCount[INVDLumpSumBlock]:=6;
    LineCount[INVWLumpSumBlock]:=6;
    LineCount[INVDPeriodicBlock]:=6;
    LineCount[INVWPeriodicBlock]:=6;
    LineCount[INVRatesBlock]:=1;

    LineCount[RBTTopBlock]:=1;
    LineCount[RBTBlock]:=12;
{$endif}

  end;

  procedure SetVersionNumber;
  begin
{$ifdef V_3}
  internalvx:=0;       {vx 3.28-0}
  version:= 3 shl 8 + 28;  {major and minor version numbers}
{$else not V_3}
  internalvx:=0;       {vx 2.27-0}
  version:= 2 shl 8 + 27;  {major and minor version numbers}
{$endif}
                      { + 01 =".01", etc}
  df.version:=version; 
  end;

  procedure InitActuarial;
    var i :byte;
  begin
  pod:=0; podval:=0;
  dob[1].m:=unkbyte;
  dob[2].m:=unkbyte;
  actu_now:=VIDEODAT.now;
  termdate.m:=unkbyte;
{$ifdef ACTU}
  fold_in_life:=false;
  actset:=[];
  for i:=NOT_CONTINGENT to BOTH_LIVING do 
    actset:=actset+[actchar[i]];
{$endif}
  end;

{$ifdef BUGSIN}
  procedure FillMemoryWithFs;
    const fillvalue=0; {or $FF}
    var
      x1,x2  :longint;
      p1     :pointer absolute x1;
      p2     :pointer absolute x2;
      siz    :longint;
  begin
    p1:=@actset;
    p2:=@podval;
    siz:=x2-x1;
    FillChar(actset,siz,fillvalue);
    if (nlines[3]<>fillvalue) then begin
      writeln('FillMemory failed.');
      halt; end;
  end;
{$endif}

  procedure SetProcsToNull;
     var i :byte;
  begin
     {Normally, all these proc's are assigned in the initialization code of
      each screen's unit.  The following code is in here so that if in some
      versions there's a screen that's not complied in, pressing this screen's 
      hot key won't cause a crash.}
     for i:=0 to nscr do 
       begin 
        ScreenProc[i]:=NullScreenProc; 
        EnterProc[i]:=NullScreenProc;
        InitProc[i]:=SetThisRunToZero; 
        CloseProc[i]:=NullProc;
       end;
  end;

  procedure peDataInit;
  begin
{$ifdef BUGSIN}
    FillMemoryWithFs;
{$endif}
    SetVersionNumber;
    SetColumnSets;
{$ifdef MAC}
    SetGuidanceStrings;
    SetAcceptableChars;
    SetDefaults;
{$endif}
    SetFColandLCol;
      {SetMonthStrings;}
    SetPerYrSet;
    SetAssortedJunk;
    SetColTypes;
     {AllocateUndoBuffer;}
    SetBlockDataArray;
    SetWhichBlocksScroll;
    SetLineCounts;
    SetPointersToNil;
    SetDataLineSizes;
    SetDataOffsets;
{$ifdef MAC}
    SetHeaderStrings;
{$endif}
    SetColumnWidths;
    SetTopsAndBottoms;
    InitActuarial; {need to do this even if not ACTU}
    examplemode:=false;
    scripting:=false;
    SetProcsToNull;
  end;

function Fblock(which:byte):byte;
         begin
         case which of
            iPVL : fblock:=PVLLumpSumBlock;
            iAMZ : fblock:=AMZTopBlock;
            iCHR : fblock:=CHRBlock;
            iMTG : fblock:=MTGBlock;
{$ifdef V_3}
            iINV : fblock:=INVDLumpSumBlock;
            iRBT : fblock:=RBTTopBlock;
{$endif}
            else   fblock:=101;  {FBlock>LBlock for lastrun=0, so "for" statements don't execute.}
            end;
         end;

function Lblock(which :byte):byte;
         begin
         case which of
            iPVL : if (pvlfancy) then lblock:=PVLXBlock else lblock:=PVLPresValBlock;
            iAMZ : if (fancy) then lblock:=AMZSkipMonthBlock else lblock:=AMZBalanceBlock;
            iCHR : lblock:=CHRBlock;
            iMTG : lblock:=MTGBlock;
{$ifdef V_3}
            iINV : lblock:=INVRatesBlock;
            iRBT : lblock:=RBTBlock;
{$endif}
            else   lblock:=100;  {FBlock>LBlock for lastrun=0, so "for" statements don't execute.}
            end;
         end;

function ScreenExt(which :byte):str3;
         begin
         case which of
            iPVL : ScreenExt:='PVL';
            iMTG : ScreenExt:='MTG';
            iCHR : ScreenExt:='CHR';
            iAMZ : ScreenExt:='AMZ';
{$ifdef V_3}
            iINV : ScreenExt:='INV';
            iRBT : ScreenExt:='RBT';
{$endif}
            end; {case code}
         end;

function WhichRun(ch :char):byte;
         begin
         case ch of
            'P' : WhichRun:=iPVL;
            'C' : WhichRun:=iCHR;
            'M' : WhichRun:=iMTG;
            'A' : WhichRun:=iAMZ;
{$ifdef V_3}
            'I' : WhichRun:=iINV;
            'R' : WhichRun:=iRBT;
{$endif}
            else WhichRun:=0;
            end;
         end;

function PctExt(which :byte):str3;
         begin
         case which of
            iPVL : PctExt:='%P';
            iMTG : PctExt:='%M';
            iCHR : PctExt:='%C';
            iAMZ : PctExt:='%A';
{$ifdef V_3}
            iINV : PctExt:='%I';
            iRBT : PctExt:='%R';
{$endif}
            end; {case code}
         end;

function WhichFancyCode(which :byte):byte;
         begin
{$ifdef CHEAP}
WhichFancyCode:=0;
{$else not CHEAP}
         case which of iAMZ : WhichFancyCode:=fancycode;
{$ifdef PVLX}          iPVL : WhichFancyCode:=pvlfancycode;  {$endif}
              else            WhichFancyCode:=0;
              end;
{$endif not CHEAP}
         end;

function VeryLCol(block :byte):byte;
         begin
         case block of RBTTopBlock : verylcol:=FCol[block]+5;
                        RBTBlock   : verylcol:=FCol[block]+7;
                        else         verylcol:=LCol[block];
             end; {case}
         end;

{$ifdef COPROC}
function LineSizeOnDisk(block :byte):byte;
         var result,c         :byte;
              justdidrate     :boolean;
         begin
         if (block=CHRBlock) then begin LineSizeOnDisk:=67; exit; end;
         result:=DataSize[block];
         justdidrate:=false;
         for c:=FCol[block] to VeryLcol(block) do begin
           if (not ((justdidrate) and (coltype[c]=PCT_FMT))) then
             if (ColType[c] in [PERCENT_FMT,CURRENCY_FMT]) then
                dec(result,sizeof(real)-sizeof(sixbytereal));
           justdidrate:=(coltype[c]=PERCENT_FMT) and (c<>pointscol) and (c<>apointscol)
                        {$ifdef V_3} and (c<>iratecol) {$endif};
           end;
         if (block=PVLPresValBlock) and (not pvlfancy) then dec(result,2); {for duration, obiter dicta}
         LineSizeOnDisk:=result;
         end;

const pct_but_not_raterec_set:byteset=[mratecol,pointscol,colacol,pctcol,aratecol,apointscol,adjratecol,riratecol,rratecol];

procedure ConvertDoublesInLineToReals(block,i :byte; DiskData :pointer);
          var pin                :^real;
              pout               :^SixByteReal;
              instatus,outstatus :^byte;
              c,ct               :byte;
              justdidrate        :boolean;
          begin
          outstatus:=DiskData;
          justdidrate:=false;
          for c:=fcol[block] to verylcol(block) do begin
            if (c in pct_but_not_raterec_set) then ct:=CURRENCY_FMT
            else ct:=ColType[c];
            if (not ((justdidrate) and (ct=PCT_FMT))) then begin
              instatus:=AdvancePointer(BlockData[block]^[i],dataoffset[c]);
              outstatus^:=instatus^;
              pin:=AdvancePointer(instatus,1);  pout:=AdvancePointer(outstatus,1);
              case ct of
               CURRENCY_FMT : begin
                              if (instatus^>empty) then pout^:=pin^ {do data conversion}
                              else pout^:=blank;
                              outstatus:=AdvancePointer(outstatus,succ(sizeof(SixByteReal)));
                              end;
                PERCENT_FMT : if (not justdidrate) then begin
                              if (instatus^>empty) then pout^:=pin^ {do data conversion}
                              else pout^:=blank;
                                {There's an extra byte after a PERCENT_FMT that keeps track of peryr,
                                 and we'll copy that now as if it were a status byte.}
                              outstatus:=AdvancePointer(outstatus,succ(sizeof(SixByteReal)));
                              instatus:=AdvancePointer(pin,sizeof(real));
                              outstatus^:=instatus^;
                              outstatus:=AdvancePointer(outstatus,1);
                              end;
                   DATE_FMT : begin
                              Move(pin^,pout^,sizeof(daterec));
                              outstatus:=AdvancePointer(outstatus,succ(sizeof(daterec)));
                              end;
  SHORT_FMT,THREE_DIGIT_FMT : begin
                              Move(pin^,pout^,sizeof(integer));
                              outstatus:=AdvancePointer(outstatus,succ(sizeof(integer)));
                              end;
                 STRING_FMT : begin
                              Move(pin^,pout^,sizeof(byte));
                              outstatus:=AdvancePointer(outstatus,succ(sizeof(byte)));
                              end;
                  STR15_FMT : begin
                              Move(pin^,pout^,sizeof(str15));
                              outstatus:=AdvancePointer(outstatus,succ(sizeof(byte)));
                              end;
                     end; {case ct}
              justdidrate:=(ct=PERCENT_FMT) {$ifdef V_3} and (c<>iratecol) {$endif};
              end;
            end;
            if (block in [PVLLumpSumBlock,PVLPeriodicBlock {$ifdef V_3},INVWLumpSumBlock,INVDLumpSumBlock,
                          INVWPeriodicBlock,INVDPeriodicBlock {$endif}]) then begin
               outstatus:=AdvancePointer(DiskData,pred(LineSizeOnDisk(block)));
               instatus:=AdvancePointer(BlockData[block]^[i],pred(DataSize[block]));
               outstatus^:=instatus^; {This is copying the act0 or actn byte at end of record.}
               end;
          end;

procedure ConvertRealsInLineToDoubles(block,i :byte; DiskData :pointer);
          var pin                :^SixByteReal;
              pout               :^real;
              instatus,outstatus :^byte;
              c,ct               :byte;
              justdidrate        :boolean;
          begin
          instatus:=DiskData;
          justdidrate:=false;
          for c:=fcol[block] to verylcol(block) do begin
            if (c in pct_but_not_raterec_set) then ct:=CURRENCY_FMT
            else ct:=ColType[c];
            if (not ((justdidrate) and (ct=PCT_FMT))) then begin
              outstatus:=AdvancePointer(BlockData[block]^[i],dataoffset[c]);
              outstatus^:=instatus^;
              pin:=AdvancePointer(instatus,1);  pout:=AdvancePointer(outstatus,1);
              case ct of
               CURRENCY_FMT : begin
                              pout^:=pin^; {do data conversion}
                              instatus:=AdvancePointer(instatus,succ(sizeof(SixByteReal)));
                             end;
                PERCENT_FMT : if (not justdidrate) then begin
                              pout^:=pin^; {do data conversion}
                                {There's an extra byte after a PERCENT_FMT that keeps track of peryr,
                                 and we'll copy that now as if it were a status byte.}
                              instatus:=AdvancePointer(instatus,succ(sizeof(SixByteReal)));
                              outstatus:=AdvancePointer(pout,sizeof(real));
                              outstatus^:=instatus^;
                              instatus:=AdvancePointer(instatus,1);
                              end;
                   DATE_FMT : begin
                              Move(pin^,pout^,sizeof(daterec));
                              instatus:=AdvancePointer(instatus,succ(sizeof(daterec)));
                              end;
  SHORT_FMT,THREE_DIGIT_FMT : begin
                              Move(pin^,pout^,sizeof(integer));
                              instatus:=AdvancePointer(instatus,succ(sizeof(integer)));
                              end;
                 STRING_FMT : begin
                              Move(pin^,pout^,sizeof(byte));
                              instatus:=AdvancePointer(instatus,succ(sizeof(byte)));
                              end;
                  STR15_FMT : begin
                              Move(pin^,pout^,sizeof(str15));
                              instatus:=AdvancePointer(instatus,succ(sizeof(byte)));
                              end;
                     end; {case ct}
              justdidrate:=(ct=PERCENT_FMT) {$ifdef V_3} and (c<>iratecol) {$endif};
              end;
            end;
            if (block in [PVLLumpSumBlock,PVLPeriodicBlock{$ifdef V_3},INVWLumpSumBlock,INVDLumpSumBlock,
                          INVWPeriodicBlock,INVDPeriodicBlock {$endif}]) then begin
               instatus:=AdvancePointer(DiskData,pred(LineSizeOnDisk(block)));
               outstatus:=AdvancePointer(BlockData[block]^[i],pred(DataSize[block]));
               outstatus^:=instatus^; {This is copying the act0 or actn byte at end of record.}
               end;
          end;
{$endif}

begin
peDataInit;
end.
