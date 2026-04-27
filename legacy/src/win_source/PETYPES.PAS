unit peTypes;

INTERFACE

//  uses VIDEODAT,NORTHWND,INPUT;
uses VIDEODAT, GLOBALS;

{           A brief essay on blocks and rows and arrays of data:     }
{ Blocks are numbered 1..nblocks.  All blocks are associated with data records.}
{ Some blocks are scrollable, and the number of data lines per block is variable.}
{ For these blocks, scrolls[block] is true.}

{ By convention, a variable called row refers to position on the screen, relative}
{ to the top of the block on the screen.  The corresponding position in the array is }
{ referred to with the variable i. A variable called y refers to screen coordinate, 1..25.}
{ -> It will always be true that i=row+scrollpos[block] and y=row+ztop[block].}

{ nlines is an array that tells how many lines are allocated in the array for each block.}
{ nlines must relate to i, not to y or row.}
{ screenlines is another array that tells how many of these lines are visible.}
{ Both row and i start their numbering with 1.}
{ i must never be allowed to exceed nlines. }
{ row must never be allowed to exceed screenlines. }

{  When you first put a piece of data into a row, a check is made whether }
{  the corresponding i is greater than nlines.  If so, all rows between the }
{  old nlines and the new i are added.  On the CHR screen, an extra row is }
{  even added and initialized beyond the last i containing data, because }
{  computations on that screen spill over into the following row. }

{Data is referred to in two ways: Each block has an array a, b,c, e, g, etc}
{These are arrays of maxlines pointers, unallocated at first.}
{Second, there is an encompassing variable called blockdata, a pointer to an array}
{of pointers.  It can point to a, b, c, etc.  The latter is useful because some of}
{the data-allocation routines, like AddRow, StringToArray, etc. need to be implemented }
{only once if they can be told what block they're working on.}

{nlines[block] is initialized to 0 for blocks that scroll.}
{After that, the StringToArray proc will always check that the i that it is entering data in}
{is not greater than nlines[block].  If it is,  AddRow will be called.  AddRow will allocate}
{the appropriate pointer.}
{AddRow also calls the procedure InitDataLine to initialize the new row as blank data.}

{$ifdef COPROC}
type SixByteReal=System.real;
        realreal=System.real;
         SysReal=System.real;
            real=double;
{$endif}

type
// following block comes from Input.pas
     charset=set of char;
     str12=string[12];
// end of moved block
// I'm guessing on this namestr.  I can't find the definition of it anywhere
     namestr=string;

type byteset=set of byte;


  const

    CR = #13;
    TAB = #9;
    datew = 8;
    currencyw = 10;
    shortw = 2;
    percentw = 7;
    pointsw = 4;
    taxw = 8;
    balloonw = 8;


    iffpro= {$ifdef PRO} true; {$else} false; {$endif}

{$ifdef SCROLLS}
  {$ifdef NO_FRILLS}
    maxlines=25;  {number of pointers for each block that scrolls}
  {$else not NO_FRILLS}
    {$ifdef CHEAP}
    maxlines=18;
    {$else not CHEAP}
    maxlines=127; {number of pointers for each block that scrolls}
    {$endif not CHEAP}
  {$endif NO_FRILLS}
{$else not SCROLLS}
    maxlines=18;
{$endif} {no SCROLLS}

  {For COLAmonth options}
    CNT = 98;
    ANN = 99;
{$ifdef V_3}
    const_signal = 89;
{$endif}

  { numeric field qualifiers }
    SHORT_FMT = 0;
    CURRENCY_FMT = 1;
    PERCENT_FMT = 2;
    PCT_FMT = PERCENT_FMT;
    DATE_FMT = 3;
    STRING_FMT = 4;
    THREE_DIGIT_FMT = 5;
    STR15_FMT = 6;
    SHORT_THREE:byteset=[SHORT_FMT,THREE_DIGIT_FMT];
    UNUSED_FMT = 99;
    vtypestr:array[SHORT_FMT..THREE_DIGIT_FMT] of str12=
          ('0 (short)','1 (currency)','2 (percent)','3 (date)','4 (string)','5 (3-digit)');

    empty = 0;
    calculated = 1; {same as outp}
    fromcalc = calculated; {same as outp}
    fromrate = 2;
    fromapr = 3;
    fromyield = 4;

// James sez - INPUT.ERROR_VAL was set to -8888.  I don't want to include
// INPUT as it's all DOS screen stuff.  So I'm just gonna set it here
//    error = INPUT.ERROR_VAL; {-9999}
    error = -8888;
    unk = -8888;
    blank = -7777;
    ERROR_VAL = error;
    add = false;
    subtract = true;
    literal = true;
    daily = 64;
    canadian = 128;
    canadianordaily = canadian or daily;
    unkbyte = -88;
    unkdate :daterec=(d:0;m:unkbyte;y:0);
    OneCell = 128; {used in gInternalClip.Block to say that only one cell is in scrap}

{The following are values of an inout, in order of "hardness"}
    badp = -1; {Returned by StringToArray if data improperly formatted}
        { or unreadable or out of bounds.}
 {empty=0 - declared elsewhere}
    outp = 1;
    defp = 2;
    inp = 3;

    fouryears = 1461;
    twenty_years = 7305;
    errorbyte = -99;
    small = 1E-4;
    teeny = 1E-10;
    half = 0.5;
{$ifdef MAC}
    twelfth = 0.08333333333333333333333333333333;
{$else}
    twelfth = 1/12;
{$endif}
    hundredth = 0.01;
    i_pvl = 0;
    i_chr = 1;
    i_mtg = 2;
    i_amz = 3;

    no_tab = 0;
    with_tab = 1;
    make_table = 2;
    with_life = 4;
    lotus_express = 8;
    APR_Comp = 16;
    APR_Report = 16;

    NOT_CONTINGENT=0; LIVING=1; DEAD=2;
    ONLY_1_LIVING=3; ONLY_2_LIVING=4; EITHER_LIVING=5; BOTH_LIVING=6;
    ONLY_1=ONLY_1_LIVING; ONLY_2=ONLY_2_LIVING; EITHER=EITHER_LIVING; BOTH=BOTH_LIVING;

    PVLlumpsumblock = 1;
    PVLperiodicblock = 2;
    PVLpresvalblock = 3; PVLRatesBlock=3; {this is a better description if PVLFancy}
    PVLExtraBlock = 4;  PVLXBlock = PVLExtraBlock;
    ActuarialBlock=6; {used for the Actuarial Options window}

    MTGblock = 5;

    SpecialBlock = 6;

    CHRblock = 7;

    AMZTopBlock = 8;
    AMZBalanceBlock =9;  AMZAsofBlock=AMZBalanceBlock;
    AMZPreBlock =10;
    AMZBalloonBlock = 11;
    AMZRateChangeBlock = 12; AMZChangesBlock = AMZRateChangeBlock; AMZAdjBlock = AMZRateChangeBlock;
    AMZMoratoriumBlock = 13;
    AMZTargetBlock = 14;
    AMZSkipMonthBlock = 15;

{$ifdef V_3}
    INVDLumpSumBlock=16;
    INVDPeriodicBlock=17;
    INVWLumpSumBlock=18;
    INVWPeriodicBlock=19;
    INVRatesBlock=20;

    RBTTopBlock=23;
    RBTBlock=24;
    nblocks=24;
{$else}
    nblocks = 15;
{$endif}

      {*** PRESENT VALUE SCREEN ***}
    datecol = 1;
    amountcol = 2;
    valuecol = 3;
    fromcol = 4;
    tocol = 5;
    thrucol = tocol;
    timescol = 6;
    pamountcol = 7;
    colacol = 8;
    pvaluecol = 9;
    asofcol = 10;
    tratecol = 11;
    lratecol = 12;
    yieldcol = 13;
    sumvaluecol = 14;
    xasofcol = 15;
    simplecol = 16;
    xvaluecol = 17; {These 3 for PVLX only}


      {*** MORTGAGE SCREEN    ***}
    pricecol = 20;
    pointscol = 21;
    pctcol = 22;
    cashcol = 23;
    financedcol = 24;
    yearscol = 25;
    mratecol = 26;
    taxcol = 27;
    monthlycol = 28;
    whencol = 29;
    howmuchcol = 30;
    aprxcol=31;  {not a real column - just used in script files}

      {******* Life expectancy window *****}
    lineselectcol = 32;
    dob1col = 33;
    fileselect1col = 34;
    dob2col = 35;
    fileselect2col = 36;
    nowcol = 37;
    podcol =38;
    termcol = 39;
    deathcol = termcol; {only one used at a time}

    ActWindowColumns=[33..39];
    LifeExpectancyColumns=ActWindowColumns;

      {*** CHRONOLGICAL SCREEN ***}
    vdatecol = 42;
    vprincipalcol = succ(vdatecol);
    vratecol = succ(vprincipalcol);
    vaprcol = succ(vratecol);
    vsumcol = succ(vaprcol);
    vinterestcol = succ(vsumcol);
    vdepositcol = succ(vinterestcol);
    vperyrcol = succ(vdepositcol);


      {*** AMORTIZATION SCREEN ***}
    aamountcol = 50;
    loandatecol = succ(aamountcol);
    aratecol = succ(loandatecol);
    firstdatecol = succ(aratecol);
    pdnumcol = succ(firstdatecol);
    lastdatecol = succ(pdnumcol);
    aperyrcol = succ(lastdatecol);
    paymentcol = succ(aperyrcol);
    apointscol = succ(paymentcol);
    aaprcol = succ(apointscol);  {59}

    aasofcol=60;
    abalancecol=succ(aasofcol); balancecol=abalancecol; {61}

    prefirstdatecol = 64;
    prepdnumcol = succ(prefirstdatecol);
    prelastdatecol = succ(prepdnumcol);
    preperyrcol = succ(prelastdatecol);
    prepaymentcol = succ(preperyrcol);  {68}

    balloondatecol = 69;
    balloonamtcol = succ(balloondatecol);

    adjdatecol = succ(balloonamtcol);
    adjratecol = succ(adjdatecol);
    adjaprcol = adjratecol;
    adjamtcol = succ(adjratecol); {73}

    sumpmtcol=74;  {not a real column - just used in script files}
        {unused 75..76}

    int_only_tilcol = 77;
    int_only_til_col = 77;
    moratoriumcol = 77;
    targetcol = 78;
    skipmonthcol = 79;

{$ifdef V_3}
      {*** INVESTMENT SCREEN ***}
    ddatecol = 81;
    damountcol = 82;
    ddollar0col = 83;
    dfromcol = 84;
    dtocol = 85;
    dthrucol = dtocol;
    dtimescol = 86;
    dpamountcol = 87;
    ddollarncol = 88;
    wdatecol = 91;
    wamountcol = 92;
    wdollar0col = 93;
    wfromcol = 94;
    wtocol = 95;
    wthrucol = wtocol;
    wtimescol = 96;
    wpamountcol = 97;
    wdollarncol = 98;
    iratecol = 101;
    inflationcol = 102;
    constdatecol = 103;

    rloanamtcol=105;
    rloandatecol=106;
    riratecol=107; rloanratecol=riratecol; rinitratecol=riratecol;
    rmethodcol=108;
    rperyrcol=109;
    rfirstdatecol=110;

    rdatecol=111;
    rpaymentcol=112;
    rratecol=113;
    rintpartcol=114;
    rchargescol=115;
    rprincpartcol=116;
    rnewprinccol=117;
    rexemptcol=118;

    ncols = 118;
{$else}
    ncols = 79;
{$endif}

{$ifdef TRUE}
    not_saved = 1;
    needs_calc = 2;
    did_something = 4;
    cell_changed = 8;
    needs_everything = 15;
    settings_changed = 128; {for screenstatus}

    saved = 255; {not not_saved}
    computed = 254; {not needs_calc}
    cell_recorded = 248; {not cell_changed}
    needs_nothing = 0;
{$endif}

{empty = 0; already declared}
    blank_line = empty; {0}
    needs_deposit = 1;
    needs_total = 2;
    needs_both = 3;
    missing_4 = 250;
    missing_3 = 251;
    missing_2 = 252;
    under_determined = missing_2;
    contains_unknown = 253;
    fully_specified = 254;
    over_determined = 255;

    maxprepay = 2;
    maxadj = maxlines;
    maxballoon = maxlines;
{$ifdef MAC}
    presvallines = 4;
{$else}
    presvallines = 3;
{$endif}

type
     str15=string[15];
     basistype = (x365,x360,x365_360);

     methodtype=(mCONTINUOUS,mPERDC_CONT,mDAILY,mPERIODIC,mPMT_TO_PMT,mSKIP_PMT,mUS_RULE,mP_BEFORE_I);

     compdefaults=record
         colamonth,centurydiv,peryr                             :byte;
         USARule                                                :boolean;
         basis                                                  :basistype;
         prepaid,in_advance,plus_regular,exact,r78              :boolean;
         end;

     hardwaredefaults=record
             color                            :(default_color,force_color,force_bw);
             userMem                          :word;
             saveheader,commas,bypass,
             printzeros,use_ems,dummy         :boolean; {dummy preserves record length from v1.14 to 1.15}
             hot_key                          :word;
             swappath                         :str80;
             end;

     printerdefaults=record
             port                                  :(prn,lpt1,lpt2,com1,com2,lpt3);
             baud                                  :(baud3,baud12,baud24,baud48,baud96);
             parity                                :(parityN,parityE,parityO);
             databits                              :7..8;
             stopbits                              :1..2;
             dummy1,dummy2,dummy3,dummy4           :byte;
             eject,pagepause                       :boolean;
             linesperpage,left,top,bottom          :byte;
             formfeeds,ibmset                      :boolean;
             initstring,boldonstring,
             boldoffstring,resetstring             :str80;
             lastselected                          :str40;
             noprnchk                              :char;
             end;

     directorydefaults=record
         screenpath,textpath,lotuspath                  :str80;
         end;

     defrec=record
       version                                              :word;
       percent                                              :char; {Identifier for Per%Sense files}
       lastrun                                              :byte;
       headerfilename                                       :str8;
       actuarialfilename                                    :array[1..2] of str8;
       c :compdefaults;
       h :hardwaredefaults;
       p :printerdefaults;
       xsimple :boolean;
       rmethod :methodtype;
       xsettings :array[1..37] of byte;  {these are padding, to keep settings file the same size}
       d :directorydefaults;
       end;

var
      version:word=0; {set in vx.pas now}

const
// is assigned to, so make it a var (above)
//      version:word=0; {set in vx.pas now}
      iopen=0; ipvl=1; ivar=2; ichr=ivar; imtg=3; iamz=4; {Identify the different screens}
{$ifdef V_3}
      iinv=5; iarb=6; irbt=iarb;  nscr=6;
      methodstr:array[mCONTINUOUS..mP_BEFORE_I] of str12=('CONTINUOUS','PERDC/CONT','  DAILY   ',' PERIODIC ',
                                                      'PMT-TO-PMT',' SKIP-PMT ',' US RULE  ','P BEFORE I');

{$else}
      nscr=4;
{$endif}
      maxexample=18; maxscript=21;
      disable :boolean=false;  global_done:boolean=false;
      inv365=1/365;

      eightblanks='        ';
      maxreal=1.7E38;
      allcols=ncols;


       {AMZ screen}
       maxballoons=maxballoon; extraballoon=succ(maxballoon);

type
       daterec0=daterec;
//       str12=string[12];
       inout=shortint; {This integer accompanies every piece of data to tell where it came from.}
       upto=(before, on_or_before, after, on_or_after);
       bytepercol=array[1..allcols] of byte;
       colarray = bytepercol;
       blockarray=array[0..nblocks] of byte;
       destinations=(none,tolotus,toprinter,totext);
           {The fact that (tolotus<toprinter) and (tolotus<totext) is used in several places}
       midscreenblock=array[9..22] of screenline;

       no_params=procedure;
       codeproctype=procedure(code :byte); {Called by screen Loading proc}
       actarray=array[1..maxlines] of NOT_CONTINGENT..BOTH_LIVING;

  placeholder = record
        xgo,ygo,col,block             :byte
        end;

const

{$ifdef PLANNER}
        TSRMode=false;
{$else}
        TSRMode:boolean=false;
{$endif}
        { UserName:str80='';  Not used in NE version.}
{$ifdef V_3}
        recently:array[0..nscr] of shortint=(0,0,0,0,0,0,0);
{$ifdef RBT}

{$endif RBT}
{$else}
        recently:array[0..nscr] of shortint=(0,0,0,0,0);
{$endif}

const not_saved=1; needs_calc=2; did_something=4; cell_changed = 8;
      needs_everything=15;  {for screenstatus}
      saved=not not_saved; computed=not needs_calc; cell_recorded=not cell_changed; needs_nothing=0;
 linewithcorners:str80= '읕컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴컴켸';

{$ifdef MAC}
      daterec = record
        d, m: integer;
        y: integer;
      end;
{$endif}

type

    integerPtr = ^integer;
    realPtr = ^real;
    inoutptr = ^inout;
    statusptr = inoutptr;
    boolPtr = ^boolean;
    wordPtr = ^word;
    datePtr = ^daterec;
    ratePtr = ^raterec;
    methodPtr = ^methodtype;

    tabarray = array[1..20] of integer; {for tables}
    genericblockarray=array[1..maxlines] of pointer;

    mortgageline = record
        pricestatus: inout;
        price: real;
        pointsstatus: inout;
        points: real;
        pctstatus: inout;
        pct: real;
        cashstatus: inout;
        cash: real;
        financedstatus: inout;
        financed: real;
        yearsstatus: inout;
        years: integer; {because its gColType is SHORT_FMT}
        ratestatus: inout;
        rate: real;
        taxstatus: inout;
        tax: real;
        monthlystatus: inout;
        monthly: real;
        whenstatus: inout;
        when: integer; {because its gColType is SHORT_FMT}
        howmuchstatus: inout;
        howmuch: real;
        balloonstatus: (balloon_known, balloon_unk, balloon_blank);
      end;

    mtgptr = ^mtgline;
    mtgline = mortgageline;
    mtgRecList = array[1..maxlines] of ^mortgageline;
    mtgarray = mtgRecList;

    lumpsum = record
         {status=0 :all blank }
        datestatus: inout;
        date: daterec;
        amt0status: inout;
        amt0: real;
        val0status: inout;
        val0: real;
        status: integer;
{                  1 :date=blank, 7 :date=unk, 13 :date=error}
{                  2 :amt0=blank, 8 :amt0=unk, 14 :amt0=error}
{                  3 :value=blank,  9 :value=unk, 15 :value=error}
        act0  : byte;
      end;
    lumpsumptr = ^lumpsum;

    periodic = record
        fromdatestatus: inout;
        fromdate: daterec;
        todatestatus: inout;
        todate: daterec;
        peryrstatus: inout;
        peryr: integer;
        amtnstatus: inout;
        amtn: real;
        colastatus: inout;
        cola: real;
        valnstatus: inout;
        valn: real;
        status, ninstallments: integer;
        actn  : byte;
      end;
    periodicptr = ^periodic;

    raterec = record
        status: inout;  {for compatibility with other status inouts, though it's used a bit differently. }
        rate: real;
        peryr :byte; {This is the currently controlling value of peryr for i/o.
                      It needs to be stored because if peryr changes and it was the
                      loan rate that was originally entered, then the raterec needs
                      to be smart enough to correct its value based on what peryr
                      used to be.}
      end;

    presval = record
        asofstatus: inout;
        asof: daterec;
        r: raterec;
        sumvaluestatus: inout;
        sumvalue: real;
        status: integer;
        durationstatus: inout;
        duration: real;
      end;
    presvalptr = ^presval;

    rateline = record
        datestatus: inout;
        date: daterec;
        r: raterec;
        status :integer;
      end;
    ratelineptr = ^rateline;      

   xpresval = record
        xasofstatus: inout;
        xasof: daterec;
        simplestatus :inout; {always inp or defp}
        simple: boolean;
        xvaluestatus: inout;
        xvalue: real;
        status: byte;
        end;
   xpresvalptr = ^xpresval;

{$ifdef V_3}
    ilumpsum=record
        datestatus: inout;
        date: daterec;
        amt0status: inout;
        amt0: real;
        conststatus :inout;
        constant :boolean;
        status :byte;
        act0  : byte;
        end;
    iperiodic=record
        fromdatestatus: inout;
        fromdate: daterec;
        todatestatus: inout;
        todate: daterec;
        peryrstatus: inout;
        peryr: integer;
        amtnstatus: inout;
        amtn: real;
        conststatus :inout;
        constant :boolean;
        status, ninstallments :integer;
        actn  : byte;
        end;
    irate = record
       irr, iir :raterec;
       datestatus :inout;
       constdate :daterec;
       end;
    iLumpsumArray = array[1..maxlines] of ^ilumpsum;
    iPeriodicArray = array[1..maxlines] of ^iperiodic;
{$endif}

    LumpsumArray = array[1..maxlines] of ^lumpsum;
    PeriodicArray = array[1..maxlines] of ^periodic;
    PresvalArray = array[1..presvallines] of ^presval;
    BigPresValArray = array[1..maxlines] of ^presval;
      {this type is used for c_ in order to
         1) be able to access all fields of a PresValArray, and
         2) Not generate range-check errors when c_[i] is used for i>3 }
    RateArray = array[1..maxlines] of ^rateline;
    RateLineArray = Ratearray;


    CHRline = record
        datstatus: inout;
        date: daterec;
        princstatus: inout;
        principal: real;
        r: raterec;
        depositsumstatus: inout;
        depositsum: real;
        intereststatus: inout; {This is always outp or empty}
        interest: real;
        depositstatus: inout;
        deposit: real;
        peryrstatus: inout;
        peryr: integer;
          {obiter dictum:}
        perstatus: integer; {as opposed to peryrstatus, this says "needs_deposit" or "needs_sum", etc}
        pdinterest: real;
        exprt, f: real;
        npayments: integer;
        status: integer;
      end;

    CHRLineArray = array[1..maxlines] of ^CHRline;
    CHRArray=CHRLineArray;

{$ifdef V_3}
    RBTstart = record
        rloanamtstatus  :inout;
        rloanamt        :real;
        rloandatestatus :inout;
        rloandate       :daterec;
        rbratestatus    :inout;
        rbrate          :real;
        methodstatus    :inout;
        method          :methodtype;
        peryrstatus     :inout;
        peryr           :integer; {all short_fmt columns (all peryr columns) are integers}
        firstdatestatus :inout;
        firstdate       :daterec;
          {obiter dicta}
        status          :byte;
        firstshowing    :boolean;
        end;

    RBTloan=RBTstart;

    RBTline = record
        datstatus       :inout;
        date            :daterec;
        paymentstatus   :inout;
        payment         :real;
        rbratestatus    :inout;
        rbrate          :real;
        intereststatus  :inout; {This is always outp or empty}
        interest        :real;
        chargestatus    :inout;
        charges         :real;
        princpartstatus :inout;
        princpart       :real;
        newprincstatus  :inout;
        newprinc        :real;
        exemptstatus    :inout;
        exempt          :real;
          {obiter dicta}
        status          :integer;
      end;

    RBTLineArray = array[1..maxlines] of ^RBTline;
    RBTArray=RBTLineArray;
{$endif}

    AMZLoan = record
        amountstatus: inout;
        amount: real;           {12}
        loandatestatus: inout;
        loandate: daterec;      {20}
        loanratestatus: inout;
        loanrate: real;         {32}
        firststatus: inout;
        firstdate: daterec;     {40}
        nstatus: inout;
        nperiods: integer;      {44}
        laststatus: inout;
        lastdate: daterec;      {52}
        peryrstatus: inout;
        peryr: integer;         {56}
        payamtstatus: inout;
        payamt: real;           {68}
        pointsstatus: inout;
        points: real;           {80}
        aprstatus: inout;
        apr: real;              {92}
{obiter dicta}
        lastok: boolean;
{       paymentsum :real;  added - then moved to PEDATA because it changes size of h^ when saved to disk}
      end;

    AMZPtr = ^AMZLoan;

    balloonrec = record
        datestatus: inout;
        date: daterec;
        amountstatus: inout;
        amount: real;
      end;
    balloonarray = array[1..maxballoon] of ^balloonrec;
    balloonptr = ^balloonrec;
    
    adjrec = record
        datestatus: inout;
        date: daterec;
        loanratestatus: inout;
        loanrate: real;
        amountstatus: inout;
        amount: real;
     {obiter}
        amtok :boolean;
      end;
    adjarray = array[1..maxadj] of ^adjrec;
    adjptr = ^adjrec;

    prepaymentrec = record
        startdatestatus: inout;
        startdate: daterec;
        nnstatus: inout;
        nn: integer;
        stopdatestatus: inout;
        stopdate: daterec;
        peryrstatus: inout;
        peryr: integer;
        paymentstatus: inout;
        payment: real;
{obiter dicta}
        nextdate: daterec;
      end;

    prepaymentarray = array[1..maxprepay] of ^prepaymentrec;
    prepaymentptr = ^prepaymentrec;

    moratoriumrec = record
        first_repaystatus: inout;
        first_repay: daterec;
      end;
    moratoriumptr = ^moratoriumrec;

    targetrec = record
        targetstatus: inout;
        target: real;
      end;
    targetptr = ^targetrec;

    skiprec = record
        skipstatus: inout;
        skipmonths: str15;
         end;
    skipptr = ^skiprec;

(* elsewhere
    str8 = string[8];
    str3 = string[3];
    str12 = string[12];
    str80 = DecStr;

    upto = (before, on_or_before, after, on_or_after);
*)

IMPLEMENTATION

end.
