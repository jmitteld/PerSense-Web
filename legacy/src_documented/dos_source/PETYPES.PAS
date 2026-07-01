{ ============================================================================
  UNIT peTypes  --  Per%Sense core data types and screen-layout constants

  ROLE:
    This unit is the central "data dictionary" for the entire DOS Per%Sense
    financial application.  It defines:
      * the record types that hold the user-entered data for each calculator
        screen (Present Value, Mortgage, Chronological, Amortization, and --
        under V_3 -- Investment and Rebate screens);
      * the family of named status codes (the "inout" / "status" bytes) that
        accompany every data field and drive the field-presence dispatch logic
        (blank vs. user-entered vs. computed vs. over-determined);
      * the screen-geometry constants: block numbers, column numbers, numeric
        format qualifiers, and per-screen layout indices;
      * numerical tolerances and magic sentinels (teeny, small, unk, blank,...)
        used throughout the financial engine.

    Almost everything here is a TYPE, CONST, or pointer-type declaration; the
    IMPLEMENTATION section is empty.  The records defined here are the on-screen
    data model: one record instance per visible row of a block, pointed at by
    the global block arrays (a, b, c, e, g, h, ...) declared in PEDATA.

  KEY CONCEPTS (financial domain):
    * inout / status byte: every numeric field X has a companion Xstatus of
      type "inout" telling WHERE the value came from -- empty (blank), outp
      (computed by us), defp (defaulted), inp (typed by the user), or badp
      (unparseable).  The solver decides what to compute for from which fields
      are blank vs. filled.
    * daterec: a (day, month, year) calendar date; year is a 2-digit-ish value
      (see centurydiv) so watch for Y2K-style century splitting.
    * basistype: the day-count convention (365, 360, or 365/360) for interest.
    * COLA: Cost Of Living Adjustment -- an annual % escalation applied to a
      stream of periodic payments.
    * line-status codes (fully_specified, contains_unknown, over_determined,
      missing_2/3/4, ...): the per-row classification produced by FirstPass in
      PRESVALU and the other screens.

  See the leading essay below (original author's notes) for the block/row/array
  addressing scheme.
  ============================================================================ }

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
     charset=set of char;{ set of characters, e.g. accepted input chars }
     str12=string[12];{ fixed-capacity 12-char string }
// end of moved block
// I'm guessing on this namestr.  I can't find the definition of it anywhere
     namestr=string;{ filename / general name string }

{ byteset: a set whose members are byte values 0..255.  Used for column-id
  sets (e.g. InputOnlyColumns), format-qualifier sets, etc. }
type byteset=set of byte;


  const

{ ASCII control characters and on-screen field display WIDTHS (in chars).
      The *w constants size each kind of column when rendered on the 80-col
      DOS text screen: date=8, currency=10, short int=2, percent=7, etc. }
    CR = #13;{ carriage return }
    TAB = #9;{ tab (used as field separator in table output) }
    datew = 8;{ display width of a date column }
    currencyw = 10;{ display width of a currency column }
    shortw = 2;{ display width of a short integer column }
    percentw = 7;{ display width of a percent column }
    pointsw = 4;{ display width of a points column }
    taxw = 8;{ display width of a tax/insurance column }
    balloonw = 8;{ display width of a balloon-amount column }


{ iffpro: compile-time flag, true in the "PRO" (professional) build. }
    iffpro= {$ifdef PRO} true; {$else} false; {$endif}

{ maxlines: number of row-pointer slots allocated per scrolling block.
      This is the hard ceiling on how many data rows a block can hold; it
      varies by build configuration (frills / cheap / no-frills). }
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

{ COLAmonth special sentinels.  The compdefaults.colamonth field normally
    holds a month number 1..12 (the month in which the annual COLA step is
    applied).  Two sentinel values override that:
      CNT (98) = "continuous" COLA: escalate the payment stream continuously
                 rather than in discrete annual steps.
      ANN (99) = "anniversary": step the COLA on each payment-stream
                 anniversary (the fromdate's month) rather than a fixed month.
    const_signal (V_3 only, 89) flags a "constant-dollar" / inflation-adjusted
    mode in the Investment screen (treated specially in BackwardCalc). }
{For COLAmonth options}
    CNT = 98;
    ANN = 99;
{$ifdef V_3}
    const_signal = 89;
{$endif}

{ numeric field qualifiers: per-column data-format codes stored in coltype[].
    They tell the I/O layer how to parse/format a cell and how many bytes the
    value occupies on disk and in memory. }
    SHORT_FMT = 0;{ small integer field }
    CURRENCY_FMT = 1;{ money amount (real) }
    PERCENT_FMT = 2;{ interest/percent field (real, may carry raterec peryr byte) }
    PCT_FMT = PERCENT_FMT;{ alias }
    DATE_FMT = 3;{ calendar date (daterec) }
    STRING_FMT = 4;{ boolean-backed toggle string (e.g. SIMPLE/COMPOUND) }
    THREE_DIGIT_FMT = 5;{ 3-digit integer (e.g. number of periods) }
    STR15_FMT = 6;{ up to 15-char string (skip-months expression) }
    SHORT_THREE:byteset=[SHORT_FMT,THREE_DIGIT_FMT];{ the two integer formats }
    UNUSED_FMT = 99;{ column not in use }
    vtypestr:array[SHORT_FMT..THREE_DIGIT_FMT] of str12=
          ('0 (short)','1 (currency)','2 (percent)','3 (date)','4 (string)','5 (3-digit)');

{ Go port: internal/types/constants.go: StatusEmpty/StatusCalculated/
  StatusFromCalc/StatusFromRate/StatusFromAPR/StatusFromYield (line 41) --
  rate-provenance codes for RateRec.Status. }
{ Rate/value PROVENANCE codes -- where a rate (or raterec.status) came from.
      empty=blank; calculated/fromcalc=we computed it; fromrate/fromapr/
      fromyield = derived from the true rate / APR / yield column respectively.
      These let the rate cell remember which of the three rate columns the user
      actually typed, so it can re-derive the others if peryr changes. }
    empty = 0;{ field is blank / unentered }
    calculated = 1; {same as outp}{ value was computed by the engine }
    fromcalc = calculated; {same as outp}
    fromrate = 2;{ derived from the True Rate column }
    fromapr = 3;{ derived from the Loan-rate/APR column }
    fromyield = 4;{ derived from the Yield column }

// James sez - INPUT.ERROR_VAL was set to -8888.  I don't want to include
// INPUT as it's all DOS screen stuff.  So I'm just gonna set it here
//    error = INPUT.ERROR_VAL; {-9999}
{ Go port: internal/types/constants.go: ErrorVal/Unk/Blank/UnkByte (line 65)
  -- the same numeric sentinels. }
{ Magic sentinel values stuffed into numeric fields to mean "not a real
      number".  error/unk mark an erroneous or unknown cell; blank marks an
      empty cell.  Chosen far outside any plausible financial magnitude. }
    error = -8888;{ value-level error marker }
    unk = -8888;{ value-level "unknown" marker }
    blank = -7777;{ value-level "blank" marker }
    ERROR_VAL = error;
{ Direction flags for date arithmetic (AddPeriod etc.): add=forward,
      subtract=backward in time. }
    add = false;
    subtract = true;
    literal = true;{ passed to message routines to print text verbatim }
    daily = 64;{ peryr bit flag: daily compounding }
    canadian = 128;{ peryr bit flag: Canadian (semi-annual) convention }
    canadianordaily = canadian or daily;
    unkbyte = -88;{ sentinel stored in a date's month to mean "unknown date" }
{ Go port: internal/types/records.go: UnknownDate (line 32) -- returns the
  DateRec whose Month=UnkByte; DateRec.IsUnknown (line 26) tests for it. }
    unkdate :daterec=(d:0;m:unkbyte;y:0);{ the canonical "no date entered" daterec }
    OneCell = 128; {used in gInternalClip.Block to say that only one cell is in scrap}

{The following are values of an inout, in order of "hardness"}
{ Go port: internal/types/constants.go: InOutBad/InOutEmpty/InOutOutput/
  InOutDefault/InOutInput (line 54) -- the same provenance ladder for the
  InOut status byte. }
{ The "inout"/status ladder, from softest to hardest.  A harder status wins
      when deciding which value to trust.  badp < empty < outp < defp < inp:
      a user-typed (inp) value beats a default (defp) which beats a computed
      (outp) one.  Comparisons like ">=defp" throughout the engine mean
      "this field has a usable value (defaulted or stronger)". }
    badp = -1; {Returned by StringToArray if data improperly formatted}
{ or unreadable or out of bounds.}
{empty=0 - declared elsewhere}
    outp = 1;{ value was OUTPut (computed) by the engine }
    defp = 2;{ value was DEFaulted }
    inp = 3;{ value was INput (typed) by the user }

{ Date-span constants and small-number tolerances used by the solvers. }
    fouryears = 1461;{ days in 4 years (incl. one leap day) }
    twenty_years = 7305;{ days in 20 years }
    errorbyte = -99;{ byte-level error marker }
    small = 1E-4;{ "a penny" scale: amounts smaller than this are treated as 0 }
    teeny = 1E-10;{ near-zero cutoff for rates / convergence (avoids div-by-0) }
    half = 0.5;{ 1/2, used in 2nd-order Taylor expansions }
{$ifdef MAC}
    twelfth = 0.08333333333333333333333333333333;
{$else}
    twelfth = 1/12;
{$endif}
    hundredth = 0.01;
{ i_*: alternate screen-index constants (see also iopen/ipvl/... below). }
    i_pvl = 0;{ Present Value screen }
    i_chr = 1;{ Chronological screen }
    i_mtg = 2;{ Mortgage screen }
    i_amz = 3;{ Amortization screen }

{ "code" bit flags passed to Enter()/screen procs to select what action to
      perform.  They combine (OR together): e.g. with_life or make_table. }
    no_tab = 0;{ plain calculate, no follow-on }
    with_tab = 1;{ triggered by TAB key (cursor-movement calc) }
    make_table = 2;{ after calc, build an output amortization/value table }
    with_life = 4;{ fold in actuarial life-expectancy weighting }
    lotus_express = 8;{ export to Lotus 1-2-3 }
    APR_Comp = 16;{ APR-comparison action }
    APR_Report = 16;{ (same bit) APR report }

{ Go port: internal/types/constants.go: NotContingent/Living/Dead/Only1Living/
  Only2Living/EitherLiving/BothLiving (line 131); consumed by
  internal/finance/actuarial/contingency.go: LifeProb / ContingencyLabel. }
{ Actuarial contingency codes: under whose survival a payment is contingent.
      Used by the life-expectancy (POD = Probability Of Death) calculations.
      N=not contingent; L=living; D=dead; 1/2/E/B = only-1 / only-2 / either /
      both of two named lives must be living for the payment to occur. }
    NOT_CONTINGENT=0; LIVING=1; DEAD=2;
    ONLY_1_LIVING=3; ONLY_2_LIVING=4; EITHER_LIVING=5; BOTH_LIVING=6;
    ONLY_1=ONLY_1_LIVING; ONLY_2=ONLY_2_LIVING; EITHER=EITHER_LIVING; BOTH=BOTH_LIVING;

{ Go port: n/a -- block numbers are a DOS screen-layout concern (which grid
      region is where on the 80x25 text screen).  The Go port has no screen; the
      REST API addresses payment/loan rows directly.  The block IDs do survive on
      disk in the .PVL/.AMZ/.MTG grid headers -- see internal/fileio/loader.go
      (loadMortgageFromHeaderAndData etc.) which switches on the slot-0 GridID. }
{ BLOCK NUMBERS.  A "block" is one rectangular data region on a screen
      (e.g. the Single-Payments grid).  Each block has its own row array and an
      entry in nlines[], scrollpos[], ztop[], etc.  Blocks are numbered 1..
      nblocks; the same number can be reused across different screens since only
      one screen is active at a time.

      Present Value screen blocks: }
    PVLlumpsumblock = 1;{ "Single Payments": one-time lump sums (array a) }
    PVLperiodicblock = 2;{ "Periodic Payments": payment streams (array b) }
    PVLpresvalblock = 3; PVLRatesBlock=3; {this is a better description if PVLFancy}
{ lower block: As-of date, rate(s), Total Value (array c) }
    PVLExtraBlock = 4;  PVLXBlock = PVLExtraBlock;{ fancy-PVL extra value block (array d) }
    ActuarialBlock=6; {used for the Actuarial Options window}

    MTGblock = 5;{ Mortgage screen single block (array e) }

    SpecialBlock = 6;{ reused slot for special/actuarial windows }

    CHRblock = 7;{ Chronological screen block (array g) }

{ Amortization screen blocks: a top loan block plus several Advanced-Option
      blocks (prepayments, balloons, rate/payment adjustments, moratorium,
      target principal reduction, skip-months). }
    AMZTopBlock = 8;{ main loan terms (amount, dates, rate, payment) }
    AMZBalanceBlock =9;  AMZAsofBlock=AMZBalanceBlock;{ as-of date / balance query }
    AMZPreBlock =10;{ prepayment series }
    AMZBalloonBlock = 11;{ one-time balloon payments }
    AMZRateChangeBlock = 12; AMZChangesBlock = AMZRateChangeBlock; AMZAdjBlock = AMZRateChangeBlock;
{ ARM rate/payment adjustments }
    AMZMoratoriumBlock = 13;{ interest-only deferment until a date }
    AMZTargetBlock = 14;{ minimum principal reduction per payment }
    AMZSkipMonthBlock = 15;{ months in which payments are skipped }

{$ifdef V_3}
{ Investment ("INV") screen blocks (V_3 builds only): symmetric Deposit (D)
      and Withdrawal (W) lump-sum / periodic grids, plus a rates block. }
    INVDLumpSumBlock=16;{ Investment deposits, lump sum }
    INVDPeriodicBlock=17;{ Investment deposits, periodic }
    INVWLumpSumBlock=18;{ Investment withdrawals, lump sum }
    INVWPeriodicBlock=19;{ Investment withdrawals, periodic }
    INVRatesBlock=20;{ Investment rate/inflation block }

    RBTTopBlock=23;{ Rebate screen loan-terms block }
    RBTBlock=24;{ Rebate screen amortization line block }
    nblocks=24;{ total number of blocks (V_3 build) }
{$else}
    nblocks = 15;{ total number of blocks (non-V_3 build) }
{$endif}

{ Go port: n/a -- column IDs are DOS screen-cell addresses used only for
        on-screen editing, format lookup (coltype[]) and on-disk field offsets.
        The Go port keeps per-field values as named struct fields (see the row
        records below), so no column-id table is needed. }
{ COLUMN NUMBERS.  Every editable cell belongs to a global column id.
        Columns are unique across ALL screens (so column->screen is implicit).
        startof[]/endof[]/coltype[] in PEDATA are indexed by these ids. }

{*** PRESENT VALUE SCREEN ***}
{ Single-payment grid (date/amount/value), periodic grid
        (from/through/times-per-yr/amount/COLA/value), and the bottom
        as-of/rate/yield/total block. }
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
{ Go port: internal/types/constants.go: FlagNotSaved/FlagNeedsCalc/
      FlagDidSometing/FlagCellChanged/FlagNeedsAll/FlagNeedsNothin (line 144).
      The AND-mask clear values (saved/computed/...) are DOS-specific and have
      no Go analogue (the port recomputes rather than tracking dirty bits). }
{ screenstatus bit flags.  screenstatus is a bitmask of pending work for the
      current screen.  The lower set are bits to TEST; the upper set are AND
      masks used to CLEAR a bit (e.g. screenstatus := screenstatus and computed
      clears needs_calc). }
    not_saved = 1;{ data changed since last save }
    needs_calc = 2;{ screen needs recalculation }
    did_something = 4;{ user made some edit }
    cell_changed = 8;{ a single cell was edited }
    needs_everything = 15;{ all of the above (full refresh needed) }
    settings_changed = 128; {for screenstatus}

    saved = 255; {not not_saved}{ mask to clear not_saved }
    computed = 254; {not needs_calc}{ mask to clear needs_calc }
    cell_recorded = 248; {not cell_changed}{ mask to clear cell_changed }
    needs_nothing = 0;
{$endif}

{empty = 0; already declared}
{ Go port: internal/types/constants.go: LineBlank/LineNeedsDeposit/
      LineNeedsTotal/LineNeedsBoth/LineMissing4/LineMissing3/LineMissing2/
      LineContainsUnknown/LineFullySpecified/LineOverDetermined (line 175).
      Set by the FirstPass classifiers (presentvalue/backward.go: FirstPass,
      amortization/firstpass.go: FirstPass). }
{ LINE-STATUS codes: the per-row classification stored in a record's
      "status" field by FirstPass and the ComputeLineValues procedures.
      The high values (250..255) form an ordered ladder used in comparisons
      like "status >= fully_specified".  Their meaning, roughly:
        missing_4 (250) - four required fields still missing
        missing_3 (251) - three missing
        missing_2 (252) / under_determined - two missing (cannot solve)
        contains_unknown (253) - exactly one unknown -> a BACKWARD-solve target
        fully_specified (254) - everything known -> usable for FORWARD calc
        over_determined (255) - too many fields given (user error)
      The low values (0..3) are CHR-screen specific (needs deposit/total). }
    blank_line = empty; {0}{ row is entirely blank }
    needs_deposit = 1;{ CHR: row needs a deposit value }
    needs_total = 2;{ CHR: row needs a total value }
    needs_both = 3;{ CHR: row needs both }
    missing_4 = 250;{ 4 fields missing }
    missing_3 = 251;{ 3 fields missing }
    missing_2 = 252;{ 2 fields missing }
    under_determined = missing_2;{ not enough data to solve }
    contains_unknown = 253;{ exactly one unknown -> backward-solve here }
    fully_specified = 254;{ all known -> forward-calculable }
    over_determined = 255;{ too much data supplied }

    maxprepay = 2;{ max prepayment series on AMZ screen }
    maxadj = maxlines;{ max rate/payment adjustments }
    maxballoon = maxlines;{ max balloon payments }
{$ifdef MAC}
    presvallines = 4;
{$else}
    presvallines = 3;
{$endif}

type
     str15=string[15];{ up to 15 chars (skip-month expression) }

{ Go port: internal/types/enums.go: BasisType + Basis365/Basis360/Basis365360
       (line 5); BasisType.String (line 14). }
{ basistype: day-count / interest-accrual convention.
         x365     - actual/365 (each year is 365 days)
         x360     - 30/360 (each month is 30 days, year is 360 days)
         x365_360 - actual/360 (actual days over a 360-day year) }
     basistype = (x365,x360,x365_360);

{ Go port: internal/types/enums.go: MethodType + MethodContinuous/MethodPerdcCont/
       MethodDaily/MethodPeriodic/MethodPmtToPmt/MethodSkipPmt/MethodUSRule/
       MethodPBeforeI (line 31); MethodType.String (line 58) + MethodNames. }
{ methodtype: amortization interest-application method (how interest is
       charged each period).  mCONTINUOUS (continuous compounding), mPERDC_CONT
       (periodic/continuous hybrid -- the default), mDAILY, mPERIODIC,
       mPMT_TO_PMT (payment-to-payment), mSKIP_PMT, mUS_RULE (US Rule: unpaid
       interest does not capitalize), mP_BEFORE_I (principal before interest). }
     methodtype=(mCONTINUOUS,mPERDC_CONT,mDAILY,mPERIODIC,mPMT_TO_PMT,mSKIP_PMT,mUS_RULE,mP_BEFORE_I);

{ Go port: internal/types/defaults.go: CompDefaults (line 6);
       DefaultCompDefaults (line 21) supplies the same initial values.  The
       amortization engine reads the same knobs via amortization/types.go:
       Settings (line 163). }
{ compdefaults: the computation-settings record (df.c).  These are the
       global financial defaults the user can change via the Settings screen
       and that govern every calculation:
         colamonth   - COLA step month 1..12, or CNT/ANN sentinel
         centurydiv  - 2-digit-year century pivot: years <= this are 20xx,
                       else 19xx (the Y2K split; default 50)
         peryr       - default payments per year (default 12)
         USARule     - apply the US Rule for interest capitalization
         basis       - day-count basis (see basistype)
         prepaid     - payments made at period start vs. end
         in_advance  - annuity-due (payment at start of period)
         plus_regular- prepayments are in addition to the regular payment
         exact       - use exact day-by-day summation instead of formulas
         r78         - Rule-of-78s interest allocation }
     compdefaults=record
         colamonth,centurydiv,peryr                             :byte;
         USARule                                                :boolean;
         basis                                                  :basistype;
         prepaid,in_advance,plus_regular,exact,r78              :boolean;
         end;

{ Go port: n/a -- display/memory/keyboard settings have no meaning in a
       stateless web port (some cosmetic knobs live in internal/types/defaults.go:
       AppDefaults, line 41). }
{ hardwaredefaults: display/memory/keyboard settings (df.h) -- presentation
       only, no financial meaning. }
     hardwaredefaults=record
             color                            :(default_color,force_color,force_bw);
             userMem                          :word;
             saveheader,commas,bypass,
             printzeros,use_ems,dummy         :boolean; {dummy preserves record length from v1.14 to 1.15}
             hot_key                          :word;
             swappath                         :str80;
             end;

{ Go port: n/a -- printer port/format settings; no analogue in the web port. }
{ printerdefaults: printer port/format settings (df.p) -- presentation
       only. }
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

{ Go port: n/a -- filesystem paths for a DOS install; not used by the port. }
{ directorydefaults: filesystem paths for saved screens, text, Lotus
       exports (df.d). }
     directorydefaults=record
         screenpath,textpath,lotuspath                  :str80;
         end;

{ Go port: partial -- only the financial-settings member (c :compdefaults)
       is ported, as internal/types/defaults.go: CompDefaults.  The full defrec
       (version/percent/header/hardware/printer/directory) is a DOS on-disk
       settings blob with no web analogue; the .PVL/.AMZ/.MTG loaders skip past
       the settings header rather than model defrec (internal/fileio/reader.go). }
{ defrec: the complete settings/defaults record persisted to the settings
       file.  df (the live instance) is consulted everywhere for computation
       settings (df.c), hardware (df.h), printer (df.p), directories (df.d).
       xsettings is dead padding kept solely to preserve the on-disk record
       size across versions. }
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
{ Screen identifiers used by thisrun and the per-screen proc arrays.
        iopen=opening menu, ipvl=Present Value, ivar/ichr=Chronological,
        imtg=Mortgage, iamz=Amortization; V_3 adds iinv=Investment, iarb/irbt=
        Rebate.  nscr = highest screen index. }
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
{ Go port: internal/types/records.go: DateRec (line 16) is the daterec analogue
    (day/month/year), with constructors NewDateRec (line 21), UnknownDate (32),
    EarliestDate (38), LatestDate (44) and predicate IsUnknown (26). }
       daterec0=daterec;{ alias of the (d,m,y) calendar-date record }
//       str12=string[12];
{ Go port: internal/types/records.go: InOut (line 51) is the int8 status alias;
    values are the constants in constants.go (InOutBad..InOutInput). }
       inout=shortint; {This integer accompanies every piece of data to tell where it came from.}
{ holds one of empty/outp/defp/inp/badp; see the inout ladder above }
{ Go port: internal/types/enums.go: Upto (line 90); consumed by
    internal/dateutil/dateutil.go: NumberOfInstallments (line 720) and
    Criterion (line 584). }
{ upto: rounding/inclusion mode for date-span counting (NumberOfInstallments):
         count installments strictly before / on-or-before / after / on-or-after a date. }
       upto=(before, on_or_before, after, on_or_after);
       bytepercol=array[1..allcols] of byte;{ one byte per screen column }
       colarray = bytepercol;{ synonym; e.g. startof/endof/coltype }
       blockarray=array[0..nblocks] of byte;{ one byte per block; e.g. nlines, ztop }
{ Go port: internal/types/enums.go: Destination (line 67) -- kept for parity;
    the web port returns JSON, so tolotus/toprinter/totext are unused. }
       destinations=(none,tolotus,toprinter,totext);{ output target for tables/reports }
{The fact that (tolotus<toprinter) and (tolotus<totext) is used in several places}
       midscreenblock=array[9..22] of screenline;{ saved middle-of-screen image }

       no_params=procedure;{ parameterless proc pointer }
       codeproctype=procedure(code :byte); {Called by screen Loading proc}{ action-code proc pointer }
       actarray=array[1..maxlines] of NOT_CONTINGENT..BOTH_LIVING;{ per-row actuarial contingency codes }

{ Go port: internal/types/screen.go: Placeholder (line 55) -- carried for
    parity, but the stateless web port never restores a DOS cursor. }
{ placeholder: saved cursor position (screen x/y, column, block) so the
    cursor can be restored when returning to a screen. }
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

{ Generic typed pointers used by the byte-level record-walking I/O code
      (PEDATA's ConvertDoubles/Reals) to read/write individual fields. }
    integerPtr = ^integer;
    realPtr = ^real;
    inoutptr = ^inout;
    statusptr = inoutptr;{ pointer to a status/inout byte }
    boolPtr = ^boolean;
    wordPtr = ^word;
    datePtr = ^daterec;
    ratePtr = ^raterec;
    methodPtr = ^methodtype;

    tabarray = array[1..20] of integer; {for tables}
    genericblockarray=array[1..maxlines] of pointer;{ untyped row-pointer array for any block }

{ Go port: internal/finance/mortgage/mortgage.go: MtgLine (line 44) -- same
      per-field value+status pairs; balloonstatus maps to BalloonStat of type
      internal/types/enums.go: BalloonStatus (line 80).  (A thinner
      internal/types/records.go: MortgageLine, line 74, mirrors it for I/O.) }
{ mortgageline: one row of the Mortgage screen.  Each user field has a
      paired <field>status (inout) byte.  Fields:
        price     - purchase price
        points    - up-front points charge (%)
        pct       - down-payment percentage
        cash      - cash required at settlement
        financed  - amount financed (loan principal)
        years     - mortgage term in years
        rate      - loan interest rate
        tax       - monthly tax+insurance escrow
        monthly   - total monthly payment (P&I + tax)
        when      - years from settlement to a balloon (optional)
        howmuch   - balloon payment amount (optional)
      pct/cash/financed are interdependent (solve any from the others); likewise
      price<->monthly.  balloonstatus tracks whether the balloon is known/
      unknown/blank. }
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
    mtgarray = mtgRecList;{ the Mortgage block's row-pointer array (global e) }

{ Go port: internal/finance/presentvalue/types.go: LumpSumPayment (line 38);
      helpers ZeroLumpSum / LumpSumIsEmpty (types.go:170/176). }
{ lumpsum: one row of the PV "Single Payments" block (a one-time cash flow).
      date   - when the payment occurs
      amt0   - face amount of the payment
      val0   - its present value as of the screen's as-of date
      act0   - actuarial contingency code (life-expectancy weighting)
      Exactly two of date, amt0, val0 specified -> solve the third.
      "status" encodes which field is blank/unknown/error (see old key below). }
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

{ Go port: internal/finance/presentvalue/types.go: PeriodicPayment (line 52);
      helpers ZeroPeriodic / PeriodicIsEmpty (types.go:184/193). }
{ periodic: one row of the PV "Periodic Payments" block (a level or COLA-
      escalating payment stream).
      fromdate/todate - first and last payment dates of the stream
      peryr           - payments per year
      amtn            - amount of each (base) payment
      cola            - annual cost-of-living escalation rate (decimal)
      valn            - present value of the whole stream as of the as-of date
      ninstallments   - number of payments (computed from the dates/peryr)
      actn            - actuarial contingency code
      The Summation() routines compute valn from amtn (or invert it). }
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

{ Go port: internal/types/records.go: RateRec (line 64) -- status/rate/peryr;
      the port's re-derivation logic lives in internal/finance/interest/rates.go
      (InterpretedRate / ReportedRate / YieldFromRate / RateFromYield). }
{ raterec: an interest rate together with provenance.  Because the same
      economic rate can be displayed as a True rate, a Loan/APR rate, or a
      Yield (depending on peryr/compounding), the record remembers the peryr in
      force when the user typed it so the other representations stay consistent
      if peryr later changes.
      status - provenance (fromrate/fromapr/fromyield/empty/...)
      rate   - the continuously-compounded "true" rate used internally
      peryr  - payments-per-year basis active when entered }
    raterec = record
        status: inout;  {for compatibility with other status inouts, though it's used a bit differently. }
        rate: real;
        peryr :byte; {This is the currently controlling value of peryr for i/o.
                      It needs to be stored because if peryr changes and it was the
                      loan rate that was originally entered, then the raterec needs
                      to be smart enough to correct its value based on what peryr
                      used to be.}
      end;

{ Go port: internal/finance/presentvalue/types.go: PresValLine (line 93);
      helpers ZeroPresValLine / PresValLineIsEmpty (types.go:204/210).  The
      forward/backward dispatch is presentvalue/backward.go: FirstPass (163). }
{ presval: one row of the PV bottom block -- the "answer" line.
      asof      - the date on which present value is reckoned
      r         - the discount rate (raterec)
      sumvalue  - total present value of all payments above, as of asof
      duration  - (obiter) computed Macaulay-style duration
      Field-presence here drives FORWARD (compute sumvalue) vs. BACKWARD (solve
      for rate or as-of date) calculation in PRESVALU. }
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

{ Go port: internal/finance/presentvalue/types.go: RateLine (line 124);
      consumed by presentvalue/variablerate.go (SortRateSchedule,
      VRDiscountFactor, forwardVariableRate). }
{ rateline: a (date, rate) pair used when the bottom block models a rate that
      changes over time (PVLfancy / ARM-style rate schedule).  Aliased onto the
      same storage as presval via cc[]. }
    rateline = record
        datestatus: inout;
        date: daterec;
        r: raterec;
        status :integer;
      end;
    ratelineptr = ^rateline;      

{ Go port: internal/types/records.go: XPresVal (line 162) mirrors the on-disk
      shape; loaded by internal/fileio/loader.go: readPresValLine and the PVLX
      path.  (No dedicated compute struct -- simple/xvalue are presentation.) }
{ xpresval: the fancy-PV "extra" block (PVLX builds).
      xasof  - alternate as-of date
      simple - true=simple interest, false=compound, for this extra value
      xvalue - the extra present value }
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
{ Go port: n/a -- V_3 (Investment screen) is never defined in the
      authoritative DOS build, so ilumpsum/iperiodic/irate are dead code and not
      ported.  See docs/dispatch_gaps.md on the V_3 blocks. }
{ ilumpsum / iperiodic: Investment-screen analogues of lumpsum/periodic, with
      an extra "constant" flag marking a constant-dollar (inflation-adjusted)
      amount rather than a nominal one. }
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
{ irate: Investment-screen rate block -- a real (irr) and inflation (iir)
      rate plus the date at which constant-dollar reckoning is anchored. }
    irate = record
       irr, iir :raterec;
       datestatus :inout;
       constdate :daterec;
       end;
    iLumpsumArray = array[1..maxlines] of ^ilumpsum;
    iPeriodicArray = array[1..maxlines] of ^iperiodic;
{$endif}

{ Go port: internal/finance/presentvalue/types.go: PVInput (line 130) holds
      Go slices ([]LumpSumPayment, []PeriodicPayment, []PresValLine, []RateLine)
      in place of these fixed pointer arrays; the web port has no maxlines cap. }
{ Row-pointer array types: one heap-allocated record per visible data row,
      indexed 1..nlines[block].  The matching globals are a/b/c/cc/... in PEDATA. }
    LumpsumArray = array[1..maxlines] of ^lumpsum;{ PV single-payment rows }
    PeriodicArray = array[1..maxlines] of ^periodic;{ PV periodic-payment rows }
    PresvalArray = array[1..presvallines] of ^presval;{ PV bottom block (few rows) }
    BigPresValArray = array[1..maxlines] of ^presval;
{this type is used for c_ in order to
         1) be able to access all fields of a PresValArray, and
         2) Not generate range-check errors when c_[i] is used for i>3 }
    RateArray = array[1..maxlines] of ^rateline;{ fancy-PV rate schedule rows }
    RateLineArray = Ratearray;


{ Go port: internal/types/records.go: CHRLine (line 175) mirrors the record.
      The Chronological screen has no REST endpoint yet, so there is no
      finance/chronolog compute package -- the struct exists for file I/O parity. }
{ CHRline: one row of the Chronological screen -- an account ledger line.
      Each line carries a date, principal change, rate, deposit/sum, computed
      interest, periodic deposit and per-year count.  The trailing fields
      (pdinterest, exprt, f, npayments) are scratch values the engine fills in. }
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
    CHRArray=CHRLineArray;{ Chronological screen row-pointer array (global g) }

{$ifdef V_3}
{ Go port: n/a -- Rebate screen is V_3-only dead code (never defined in the
      DOS build), so RBTstart/RBTline are not ported. }
{ RBTstart / RBTline: Rebate-screen (loan rebate / interest refund)
      records -- RBTstart holds the loan terms, RBTline one repayment-schedule
      line.  V_3 builds only. }
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

{ Go port: internal/finance/amortization/types.go: Loan (line 77) is the compute
      struct (amount/dates/rate/term/payment/points/apr with status bytes);
      loaded by internal/fileio/loader.go: readAMZLoan (line 227).  A parallel
      internal/types/records.go: AMZLoan (line 201) mirrors the on-disk layout. }
{ AMZLoan: the main loan-terms record of the Amortization screen (global h^).
      The interdependent core fields let the engine solve for whichever is left
      blank:
        amount    - loan principal
        loandate  - closing/settlement date
        loanrate  - nominal loan interest rate
        firstdate - date of first regular payment
        nperiods  - number of payment periods
        lastdate  - date of last payment
        peryr     - payments per year
        payamt    - regular payment amount
        points    - points charge (for APR)
        apr       - computed APR (output only)
      lastok is an obiter (scratch) flag.  Byte offsets in braces are the
      on-disk layout positions. }
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

{ Go port: internal/finance/amortization/types.go: BalloonPayment (line 104);
      ZeroBalloon / BalloonIsEmpty (types.go:256/262); read by
      loader.go: readBalloonPayment (line 253). }
{ balloonrec: an Advanced-Option BALLOON -- a one-time lump payment of
      'amount' applied on 'date'. }
    balloonrec = record
        datestatus: inout;
        date: daterec;
        amountstatus: inout;
        amount: real;
      end;
    balloonarray = array[1..maxballoon] of ^balloonrec;
    balloonptr = ^balloonrec;

{ Go port: internal/finance/amortization/types.go: RateAdjustment (line 113);
      ZeroAdjustment / AdjustmentIsEmpty (types.go:268/274); read by
      loader.go: readAdjustment (line 278). }

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

{ Go port: internal/finance/amortization/types.go: Prepayment (line 125);
      ZeroPrepayment / PrepaymentIsEmpty (types.go:282/292); read by
      loader.go: readPrepayment (line 262). }
{ prepaymentrec: a PREPAYMENT series -- extra payments of 'payment',
      'peryr' times per year, from startdate through stopdate (nn of them).
      nextdate (obiter) is the engine's running cursor to the next prepayment. }
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

{ Go port: internal/finance/amortization/types.go: Moratorium (line 141);
      ZeroMoratorium (types.go:302). }
{ moratoriumrec: a MORATORIUM -- pay interest only until first_repay, after
      which normal amortization (principal + interest) resumes. }
    moratoriumrec = record
        first_repaystatus: inout;
        first_repay: daterec;
      end;
    moratoriumptr = ^moratoriumrec;

{ Go port: internal/finance/amortization/types.go: Target (line 148);
      ZeroTarget (types.go:308). }
{ targetrec: a TARGET -- force each payment large enough that its principal
      portion is at least 'target' (minimum principal reduction). }
    targetrec = record
        targetstatus: inout;
        target: real;
      end;
    targetptr = ^targetrec;

{ Go port: internal/finance/amortization/types.go: SkipMonths (line 155);
      ZeroSkipMonths (types.go:314); the "6-8,12" expression is parsed by
      amortization/engine.go: MonthSetFromString (line 2018). }
{ skiprec: SKIP-MONTHS -- 'skipmonths' is a string expression listing the
      months in which no payment is made, e.g. "6-8,12". }
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
