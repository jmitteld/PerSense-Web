package types

// Column identifiers map data fields to their column positions.
// Each constant identifies a specific data field across all screens.
//
// Ported from legacy/source/PETYPES.PAS

// --- Present Value screen columns ---

const (
	ColDate     byte = 1  // datecol: date of a one-time payment
	ColAmount   byte = 2  // amountcol: amount of a one-time payment
	ColValue    byte = 3  // valuecol: present value of this payment
	ColFrom     byte = 4  // fromcol: starting date of periodic payments
	ColTo       byte = 5  // tocol/thrucol: ending date of periodic payments
	ColTimes    byte = 6  // timescol: number of times per year
	ColPAmount  byte = 7  // pamountcol: amount of periodic payments
	ColCOLA     byte = 8  // colacol: cost of living adjustment %
	ColPValue   byte = 9  // pvaluecol: present value of periodic payments
	ColAsOf     byte = 10 // asofcol: date on which present value is computed
	ColTrueRate byte = 11 // tratecol: true interest rate
	ColLoanRate byte = 12 // lratecol: loan interest rate (with compounding)
	ColYield    byte = 13 // yieldcol: 1-year yield
	ColSumValue byte = 14 // sumvaluecol: total value of all payments
	ColXAsOf    byte = 15 // xasofcol: extended present value as-of date
	ColSimple   byte = 16 // simplecol: simple vs compound toggle
	ColXValue   byte = 17 // xvaluecol: extended present value (PVLX only)
)

// --- Mortgage screen columns ---

const (
	ColPrice    byte = 20 // pricecol: purchase price of property
	ColPoints   byte = 21 // pointscol: points charged by bank
	ColPct      byte = 22 // pctcol: downpayment percentage
	ColCash     byte = 23 // cashcol: cash required at settlement
	ColFinanced byte = 24 // financedcol: amount financed
	ColYears    byte = 25 // yearscol: life of mortgage in years
	ColMRate    byte = 26 // mratecol: loan rate charged by lender
	ColTax      byte = 27 // taxcol: monthly tax + insurance
	ColMonthly  byte = 28 // monthlycol: total monthly payment
	ColWhen     byte = 29 // whencol: years to balloon payment
	ColHowMuch  byte = 30 // howmuchcol: balloon payment amount
	ColAPRX     byte = 31 // aprxcol: not a real column, used in script files
)

// --- Life expectancy window columns ---

const (
	ColLineSelect  byte = 32 // lineselectcol
	ColDOB1        byte = 33 // dob1col
	ColFileSelect1 byte = 34 // fileselect1col
	ColDOB2        byte = 35 // dob2col
	ColFileSelect2 byte = 36 // fileselect2col
	ColNow         byte = 37 // nowcol
	ColPOD         byte = 38 // podcol
	ColTerm        byte = 39 // termcol / deathcol
)

// --- Chronological screen columns ---

const (
	ColVDate      byte = 42 // vdatecol
	ColVPrincipal byte = 43 // vprincipalcol
	ColVRate      byte = 44 // vratecol
	ColVAPR       byte = 45 // vaprcol
	ColVSum       byte = 46 // vsumcol
	ColVInterest  byte = 47 // vinterestcol
	ColVDeposit   byte = 48 // vdepositcol
	ColVPerYr     byte = 49 // vperyrcol
)

// --- Amortization screen columns ---

const (
	ColAAmount   byte = 50 // aamountcol: loan amount
	ColLoanDate  byte = 51 // loandatecol: date of closing
	ColARate     byte = 52 // aratecol: loan interest rate
	ColFirstDate byte = 53 // firstdatecol: date of first regular payment
	ColPdNum     byte = 54 // pdnumcol: number of regular periods
	ColLastDate  byte = 55 // lastdatecol: date of last regular payment
	ColAPerYr    byte = 56 // aperyrcol: payments per year
	ColPayment   byte = 57 // paymentcol: regular payment amount
	ColAPoints   byte = 58 // apointscol: points for APR computation
	ColAAPR      byte = 59 // aaprcol: computed APR (output only)
)

// --- Amortization as-of / balance columns ---

const (
	ColAAsOf    byte = 60 // aasofcol
	ColABalance byte = 61 // abalancecol / balancecol
)

// --- Amortization prepayment columns ---

const (
	ColPreFirstDate byte = 64 // prefirstdatecol
	ColPrePdNum     byte = 65 // prepdnumcol
	ColPreLastDate  byte = 66 // prelastdatecol
	ColPrePerYr     byte = 67 // preperyrcol
	ColPrePayment   byte = 68 // prepaymentcol
)

// --- Amortization balloon columns ---

const (
	ColBalloonDate byte = 69 // balloondatecol
	ColBalloonAmt  byte = 70 // balloonamtcol
)

// --- Amortization adjustment columns ---

const (
	ColAdjDate byte = 71 // adjdatecol
	ColAdjRate byte = 72 // adjratecol / adjaprcol
	ColAdjAmt  byte = 73 // adjamtcol
)

// --- Amortization misc columns ---

const (
	ColSumPmt    byte = 74 // sumpmtcol: not a real column, used in scripts
	ColMoratoriu byte = 77 // moratoriumcol / int_only_tilcol
	ColTarget    byte = 78 // targetcol
	ColSkipMonth byte = 79 // skipmonthcol
)

// --- Block identifiers ---
// Blocks group related data on each screen.
// Ported from legacy/source/PETYPES.PAS

const (
	BlockPVLLumpSum  byte = 1  // PVLlumpsumblock
	BlockPVLPeriodic byte = 2  // PVLperiodicblock
	BlockPVLPresVal  byte = 3  // PVLpresvalblock / PVLRatesBlock
	BlockPVLExtra    byte = 4  // PVLExtraBlock / PVLXBlock
	BlockMTG         byte = 5  // MTGblock
	BlockActuarial   byte = 6  // ActuarialBlock / SpecialBlock
	BlockCHR         byte = 7  // CHRblock
	BlockAMZTop      byte = 8  // AMZTopBlock
	BlockAMZBalance  byte = 9  // AMZBalanceBlock / AMZAsofBlock
	BlockAMZPre      byte = 10 // AMZPreBlock
	BlockAMZBalloon  byte = 11 // AMZBalloonBlock
	BlockAMZChanges  byte = 12 // AMZRateChangeBlock / AMZChangesBlock / AMZAdjBlock
	BlockAMZMorator  byte = 13 // AMZMoratoriumBlock
	BlockAMZTarget   byte = 14 // AMZTargetBlock
	BlockAMZSkip     byte = 15 // AMZSkipMonthBlock
)
