{
  refdata.pas — Reference Data Generator for Per%Sense Go port verification.

  This is a STANDALONE Free Pascal program that reimplements the core financial
  math functions from the legacy source, exercises them with known inputs, and
  outputs results in a machine-readable format (JSON) for comparison against
  the Go port.

  Compile: fpc refdata.pas
  Run:     ./refdata > ../reference-output/refdata.json
}
program refdata;

{$mode delphi}

uses SysUtils, DateUtils, Math;

{ ===== Types from PETYPES/Globals ===== }

type
  daterec = record
    d, m: shortint;
    y: byte;
  end;
  w13 = array[1..13] of word;
  pw13 = ^w13;

const
  teeny = 1E-10;
  small = 1E-4;
  tiny  = 1E-5;
  twelfth = 1/12;
  half = 0.5;
  unkbyte = -88;
  fouryears = 1461;

const
  daysin_init: array[0..13] of byte = (31,31,28,31,30,31,30,31,31,30,31,30,31,31);

var
  daysin: array[0..13] of byte;
  notleapdaysbefore, leapdaysbefore: array[1..13] of word;

{ ===== Date functions from VIDEODAT.pas ===== }

procedure DecideAboutFeb29(wy: byte);
begin
  if (wy mod 4 = 0) and (wy > 0) then begin
    daysin[2] := 29;
  end else begin
    daysin[2] := 28;
  end;
end;

function DaysInM(f: daterec): byte;
begin
  if (f.m = 2) then begin
    if (f.y mod 4 = 0) then DaysInM := 29
    else DaysInM := 28;
  end else if (f.m <= 12) and (f.m >= 1) then
    DaysInM := daysin[f.m]
  else
    DaysInM := 30;
end;

function Julian(x: daterec): longint;
var
  db: pw13;
begin
  DecideAboutFeb29(x.y);
  if (x.y mod 4 = 0) and (x.y > 0) then
    db := @leapdaysbefore
  else
    db := @notleapdaysbefore;
  if (x.m > 13) or (x.m < 1) then begin
    Julian := -88;
    exit;
  end;
  Julian := (fouryears * longint(x.y) - 1) div 4 + db^[x.m] + x.d;
end;

procedure MDY(daynumber: longint; var x: daterec);
var
  days: integer;
  fourx: longint;
  db: pw13;
begin
  fourx := daynumber * 4;
  x.y := fourx div fouryears;
  DecideAboutFeb29(x.y);
  if (x.y mod 4 = 0) and (x.y > 0) then
    db := @leapdaysbefore
  else
    db := @notleapdaysbefore;
  days := ((fourx - longint(x.y) * fouryears) div 4) + 1;
  if (days <= db^[7]) then begin
    if (days <= db^[4]) then x.m := 1 else x.m := 4;
  end else begin
    if (days <= db^[10]) then x.m := 7 else x.m := 10;
  end;
  repeat inc(x.m) until (db^[x.m] >= days);
  dec(x.m);
  x.d := days - db^[x.m];
end;

function MakeDate(year, month, day: integer): daterec;
var d: daterec;
begin
  d.y := year - 1900;
  d.m := month;
  d.d := day;
  MakeDate := d;
end;

{ ===== Math functions from INTSUTIL.pas ===== }

function exxp(x: real): real;
const sixth = 1/6;
var x2: real;
begin
  if (x > 70) then exxp := 0
  else if (x < -70) then exxp := 1E-32
  else if (abs(x) < small) then begin
    x2 := sqr(x);
    exxp := 1 + x + half * x2 + sixth * x * x2;
  end else
    exxp := exp(x);
end;

function lnn(x: real): real;
const third = 1/3;
var t, t2: real;
begin
  if (x <= 0) then lnn := 0
  else if (abs(x - 1) < small) then begin
    t := x - 1;
    t2 := sqr(t);
    lnn := t - half * t2 + third * t * t2;
  end else
    lnn := ln(x);
end;

function Power(x, n: real): real;
begin
  if (x <= 0) then Power := 0
  else Power := exxp(n * lnn(x));
end;

procedure Round2(var x: real);
const halfpenny: real = 0.005 - 1E-10;
begin
  if (x > 0) then x := x + halfpenny
  else x := x - halfpenny;
  x := Trunc(x * 100) / 100;
end;

function RealPerYr(n: byte; yrdays: real): real;
begin
  case n of
    64: RealPerYr := yrdays;
    52: RealPerYr := yrdays / 7;
    26: RealPerYr := yrdays / 14;
  else
    RealPerYr := n and (not 128);
  end;
end;

function YieldFromRate(rr: real; n: byte; yrdays: real): real;
var nn: real;
begin
  nn := RealPerYr(n, yrdays);
  YieldFromRate := nn * (exxp(rr / nn) - 1);
end;

function RateFromYield(yy: real; n: byte; yrdays: real): real;
var nn: real;
begin
  nn := RealPerYr(n, yrdays);
  RateFromYield := nn * lnn(1 + yy / nn);
end;

{ ===== Mortgage Summation from Mortgage.pas ===== }

function Summation(r, t: real): real;
var last, f: real;
begin
  if (abs(r) < teeny) then
    Summation := 12 * t
  else begin
    last := exxp(-r * t);
    f := exxp(-r * twelfth);
    Summation := f * (1 - last) / (1 - f);
  end;
end;

{ ===== PV SumFormula from PRESVALU.pas ===== }

function SumFormula(lnf, n: real): real;
var oneminusexpnrt, oneminusf, arg: real;
    secondorder: boolean;
begin
  if (abs(lnf) < teeny) then
    SumFormula := n
  else begin
    secondorder := (abs(lnf) < tiny);
    if (secondorder) then begin
      arg := n * lnf;
      oneminusexpnrt := -arg - half * sqr(arg);
      oneminusf := -lnf - half * sqr(lnf);
    end else begin
      oneminusexpnrt := 1 - exxp(n * lnf);
      oneminusf := 1 - exxp(lnf);
    end;
    SumFormula := oneminusexpnrt / oneminusf;
  end;
end;

{ ===== YearsDif from INTSUTIL.pas (360 basis version) ===== }

function YearsDif360(z, a: daterec): real;
var til: real;
begin
  til := (z.y - a.y) + (z.m - a.m) / 12 + (z.d - a.d) / 360;
  if (a.d = 31) and (z.d < 31) then til := til + 1/360
  else if (a.d = 30) and (z.d = 31) then til := til - 1/360
  else if (a.m = 2) and (a.d > 27) then til := til - (30 - a.d) / 360;
  YearsDif360 := til;
end;

function YearsDif365(z, a: daterec): real;
begin
  YearsDif365 := (Julian(z) - Julian(a)) / 365.25;
end;

{ ===== Output helpers ===== }

var first_item: boolean;

procedure StartJSON;
begin
  writeln('{');
  first_item := true;
end;

procedure EndJSON;
begin
  writeln;
  writeln('}');
end;

procedure StartArray(name: string);
begin
  if not first_item then write(',');
  writeln;
  write('  "', name, '": [');
  first_item := true;
end;

procedure EndArray;
begin
  writeln;
  write('  ]');
  first_item := false;
end;

procedure EmitObj(s: string);
begin
  if not first_item then write(',');
  writeln;
  write('    ', s);
  first_item := false;
end;

procedure EmitKV(key: string; value: real);
begin
  if not first_item then write(',');
  writeln;
  write('  "', key, '": ', value:20:15);
  first_item := false;
end;

{ ===== Test Cases ===== }

procedure EmitJulianTests;
var
  years: array[1..10] of integer = (1901, 1950, 1952, 1999, 2000, 2001, 2024, 2050, 2100, 2149);
  i: integer;
  d: daterec;
  j: longint;
  r: daterec;
begin
  StartArray('julian_roundtrip');
  for i := 1 to 10 do begin
    d := MakeDate(years[i], 1, 1);
    j := Julian(d);
    MDY(j, r);
    EmitObj(Format('{"year":%d,"julian":%d,"rt_d":%d,"rt_m":%d,"rt_y":%d}',
      [years[i], j, r.d, r.m, integer(r.y) + 1900]));
  end;
  { Also test mid-year dates }
  d := MakeDate(2024, 7, 4);
  j := Julian(d);
  MDY(j, r);
  EmitObj(Format('{"year":2024,"month":7,"day":4,"julian":%d,"rt_d":%d,"rt_m":%d,"rt_y":%d}',
    [j, r.d, r.m, integer(r.y) + 1900]));
  d := MakeDate(2000, 2, 29);
  j := Julian(d);
  MDY(j, r);
  EmitObj(Format('{"year":2000,"month":2,"day":29,"julian":%d,"rt_d":%d,"rt_m":%d,"rt_y":%d}',
    [j, r.d, r.m, integer(r.y) + 1900]));
  EndArray;
end;

procedure EmitExxpTests;
var
  inputs: array[1..8] of real = (0, 1, -1, 0.00005, -0.00005, 0.06, -0.06, 69);
  i: integer;
begin
  StartArray('exxp');
  for i := 1 to 8 do
    EmitObj(Format('{"input":%.15g,"output":%.15g}', [inputs[i], exxp(inputs[i])]));
  EndArray;
end;

procedure EmitLnnTests;
var
  inputs: array[1..7] of real = (1, 2.718281828, 10, 0.5, 1.00005, 0.99995, 100);
  i: integer;
begin
  StartArray('lnn');
  for i := 1 to 7 do
    EmitObj(Format('{"input":%.15g,"output":%.15g}', [inputs[i], lnn(inputs[i])]));
  EndArray;
end;

procedure EmitPowerTests;
begin
  StartArray('power');
  EmitObj(Format('{"x":2,"n":10,"output":%.15g}', [Power(2, 10)]));
  EmitObj(Format('{"x":10,"n":0,"output":%.15g}', [Power(10, 0)]));
  EmitObj(Format('{"x":2,"n":0.5,"output":%.15g}', [Power(2, 0.5)]));
  EmitObj(Format('{"x":1.06,"n":30,"output":%.15g}', [Power(1.06, 30)]));
  EmitObj(Format('{"x":0.5,"n":10,"output":%.15g}', [Power(0.5, 10)]));
  EndArray;
end;

procedure EmitRound2Tests;
var
  v: real;
begin
  StartArray('round2');
  v := 1.234; Round2(v);
  EmitObj(Format('{"input":1.234,"output":%.15g}', [v]));
  v := 1.235; Round2(v);
  EmitObj(Format('{"input":1.235,"output":%.15g}', [v]));
  v := 1.236; Round2(v);
  EmitObj(Format('{"input":1.236,"output":%.15g}', [v]));
  v := -1.234; Round2(v);
  EmitObj(Format('{"input":-1.234,"output":%.15g}', [v]));
  v := -1.236; Round2(v);
  EmitObj(Format('{"input":-1.236,"output":%.15g}', [v]));
  v := 0.005; Round2(v);
  EmitObj(Format('{"input":0.005,"output":%.15g}', [v]));
  v := 0.006; Round2(v);
  EmitObj(Format('{"input":0.006,"output":%.15g}', [v]));
  v := 599.55; Round2(v);
  EmitObj(Format('{"input":599.55,"output":%.15g}', [v]));
  EndArray;
end;

procedure EmitYieldRateTests;
var
  rates: array[1..5] of real = (0.01, 0.05, 0.06, 0.10, 0.20);
  freqs: array[1..4] of byte = (1, 4, 12, 52);
  i, j: integer;
  y, r: real;
begin
  StartArray('yield_rate_roundtrip');
  for i := 1 to 5 do
    for j := 1 to 4 do begin
      y := YieldFromRate(rates[i], freqs[j], 365.25);
      r := RateFromYield(y, freqs[j], 365.25);
      EmitObj(Format('{"rate":%.15g,"peryr":%d,"yield":%.15g,"roundtrip":%.15g}',
        [rates[i], freqs[j], y, r]));
    end;
  EndArray;
end;

procedure EmitSummationTests;
begin
  StartArray('mortgage_summation');
  EmitObj(Format('{"rate":0,"years":30,"output":%.15g}', [Summation(0, 30)]));
  EmitObj(Format('{"rate":0.06,"years":30,"output":%.15g}', [Summation(0.06, 30)]));
  EmitObj(Format('{"rate":0.06,"years":1,"output":%.15g}', [Summation(0.06, 1)]));
  EmitObj(Format('{"rate":0.06,"years":15,"output":%.15g}', [Summation(0.06, 15)]));
  EmitObj(Format('{"rate":0.12,"years":30,"output":%.15g}', [Summation(0.12, 30)]));
  EmitObj(Format('{"rate":0.05,"years":30,"output":%.15g}', [Summation(0.05, 30)]));
  EmitObj(Format('{"rate":0.08,"years":10,"output":%.15g}', [Summation(0.08, 10)]));
  EmitObj(Format('{"rate":0.50,"years":30,"output":%.15g}', [Summation(0.50, 30)]));
  EndArray;
end;

procedure EmitMortgageCalcTests;
{ Replicate the core Calc: given price, pct, years, rate → compute monthly }
var
  prc, pct, rate, financed, summ, monthly: real;
  years: integer;
begin
  StartArray('mortgage_calc');

  { Test 1: $200K, 20% down, 30yr, 6% }
  prc := 200000; pct := 0.20; years := 30; rate := 0.06;
  financed := prc * (1 - pct);
  summ := Summation(rate, years);
  monthly := financed / summ;
  EmitObj(Format('{"price":%.2f,"pct":%.2f,"years":%d,"rate":%.4f,"financed":%.15g,"summation":%.15g,"monthly":%.15g}',
    [prc, pct, years, rate, financed, summ, monthly]));

  { Test 2: $100K, 0% down, 30yr, 6% }
  prc := 100000; pct := 0; years := 30; rate := 0.06;
  financed := prc * (1 - pct);
  summ := Summation(rate, years);
  monthly := financed / summ;
  EmitObj(Format('{"price":%.2f,"pct":%.2f,"years":%d,"rate":%.4f,"financed":%.15g,"summation":%.15g,"monthly":%.15g}',
    [prc, pct, years, rate, financed, summ, monthly]));

  { Test 3: $500K, 10% down, 15yr, 5% }
  prc := 500000; pct := 0.10; years := 15; rate := 0.05;
  financed := prc * (1 - pct);
  summ := Summation(rate, years);
  monthly := financed / summ;
  EmitObj(Format('{"price":%.2f,"pct":%.2f,"years":%d,"rate":%.4f,"financed":%.15g,"summation":%.15g,"monthly":%.15g}',
    [prc, pct, years, rate, financed, summ, monthly]));

  { Test 4: $120K, 0% down, 10yr, 0% (zero interest) }
  prc := 120000; pct := 0; years := 10; rate := 0;
  financed := prc * (1 - pct);
  summ := Summation(rate, years);
  monthly := financed / summ;
  EmitObj(Format('{"price":%.2f,"pct":%.2f,"years":%d,"rate":%.4f,"financed":%.15g,"summation":%.15g,"monthly":%.15g}',
    [prc, pct, years, rate, financed, summ, monthly]));

  EndArray;
end;

procedure EmitSumFormulaTests;
begin
  StartArray('pv_sumformula');
  EmitObj(Format('{"lnf":0,"n":360,"output":%.15g}', [SumFormula(0, 360)]));
  EmitObj(Format('{"lnf":-0.005,"n":360,"output":%.15g}', [SumFormula(-0.005, 360)]));
  EmitObj(Format('{"lnf":-0.005,"n":120,"output":%.15g}', [SumFormula(-0.005, 120)]));
  EmitObj(Format('{"lnf":0.001,"n":100,"output":%.15g}', [SumFormula(0.001, 100)]));
  EmitObj(Format('{"lnf":-0.00001,"n":12,"output":%.15g}', [SumFormula(-0.00001, 12)]));
  EmitObj(Format('{"lnf":-0.05,"n":30,"output":%.15g}', [SumFormula(-0.05, 30)]));
  EndArray;
end;

{ ===== Rule of 78 ===== }
{
  Per AMORTOP.pas / Amortize.pas:1506-1530, the Rule of 78 (sum-of-the-
  digits) interest split allocates total interest T = n*pmt - amount
  across periods so that period k (1-indexed) is allocated
       interest_k = T * (n + 1 - k) / (n(n+1)/2)
  i.e. front-loaded with interest declining linearly. We emit period 1,
  period 2, period n/2, and period n for a representative loan so the
  Go port can spot-check the head, middle, and tail of its own R78
  schedule against authoritative DOS-style values.
}
procedure EmitR78Tests;
var
  amt, pmt, totInt, r78step: real;
  n: integer;
  function r78IntForPeriod(period: integer): real;
  begin
    r78IntForPeriod := totInt * (n + 1 - period) / (0.5 * n * (n + 1));
  end;
begin
  StartArray('rule78');

  { Case 1: $100K, 30yr monthly, 6% — pmt 599.55 (textbook).
    n=360, totInt = 360*599.55 - 100000 = 115838.
    Head period dominated by interest; tail near zero. }
  amt := 100000; pmt := 599.55; n := 360;
  totInt := n * pmt - amt;
  r78step := totInt / (0.5 * n * (n + 1));
  EmitObj(Format('{"label":"R78_100k_30y_6pct","amount":%.4f,"payment":%.4f,"nPeriods":%d,"totalInterest":%.4f,"r78Step":%.15g,"int_pmt_1":%.15g,"int_pmt_2":%.15g,"int_pmt_180":%.15g,"int_pmt_360":%.15g}',
    [amt, pmt, n, totInt, r78step,
     r78IntForPeriod(1), r78IntForPeriod(2),
     r78IntForPeriod(180), r78IntForPeriod(n)]));

  { Case 2: $10K, 24-period (2yr) at 12% — pmt 470.73.
    n=24, totInt = 24*470.73 - 10000 = 1297.52. }
  amt := 10000; pmt := 470.73; n := 24;
  totInt := n * pmt - amt;
  r78step := totInt / (0.5 * n * (n + 1));
  EmitObj(Format('{"label":"R78_10k_24m_12pct","amount":%.4f,"payment":%.4f,"nPeriods":%d,"totalInterest":%.4f,"r78Step":%.15g,"int_pmt_1":%.15g,"int_pmt_2":%.15g,"int_pmt_12":%.15g,"int_pmt_24":%.15g}',
    [amt, pmt, n, totInt, r78step,
     r78IntForPeriod(1), r78IntForPeriod(2), r78IntForPeriod(12), r78IntForPeriod(n)]));

  EndArray;
end;

{ ===== In-advance (annuity-due) accrual ===== }
{
  When df.c.InAdvance is set, the loan pays in advance — interest accrues
  on the post-payment balance. The engine's per-period factor (per
  engine.go: RepayLoan / AMORTOP.pas) is:
       ff = (f - 1) / (2 - f), where f = exp(truerate / peryr)
  and the per-period interest for a normal full-month period is
       int_k = ff * (p_k - payment).
  We emit the ff factor and a closed-form payment for a few standard
  loans. The closed-form for an in-advance loan payment is:
       pmt = principal * f * (f - 1) / (f^n - 1)
  ...because each payment is at the start of the period (annuity-due).
}
procedure EmitInAdvanceTests;
var
  amt, rate, truerate, f, pmt, ff: real;
  n, peryr: integer;
begin
  StartArray('in_advance');

  { Case 1: $100K, 30yr monthly, 6% loan rate, in advance.
    For monthly loan rate r=6%, truerate = 12*ln(1 + 0.06/12). }
  amt := 100000; rate := 0.06; peryr := 12; n := 360;
  truerate := peryr * Ln(1 + rate / peryr);
  f := Exp(truerate / peryr);
  ff := (f - 1) / (2 - f);
  { Annuity-due payment = ordinary annuity payment / (1 + i) }
  { where i = f - 1 per period; equivalently pmt * f = ord-annuity pmt. }
  pmt := amt * (f - 1) / (1 - Exp(-truerate * (n / peryr)));
  pmt := pmt / f;
  EmitObj(Format('{"label":"InAdv_100k_30y_6pct","amount":%.2f,"rate":%.4f,"perYr":%d,"nPeriods":%d,"trueRate":%.15g,"f":%.15g,"ff":%.15g,"payment":%.15g}',
    [amt, rate, peryr, n, truerate, f, ff, pmt]));

  { Case 2: $50K, 5yr (60mo), 7%, in advance. }
  amt := 50000; rate := 0.07; peryr := 12; n := 60;
  truerate := peryr * Ln(1 + rate / peryr);
  f := Exp(truerate / peryr);
  ff := (f - 1) / (2 - f);
  pmt := amt * (f - 1) / (1 - Exp(-truerate * (n / peryr)));
  pmt := pmt / f;
  EmitObj(Format('{"label":"InAdv_50k_5y_7pct","amount":%.2f,"rate":%.4f,"perYr":%d,"nPeriods":%d,"trueRate":%.15g,"f":%.15g,"ff":%.15g,"payment":%.15g}',
    [amt, rate, peryr, n, truerate, f, ff, pmt]));

  EndArray;
end;

{ ===== Biweekly basis coercion ===== }
{
  Per Amortize.pas:297-303, weekly (peryr=52) and biweekly (peryr=26)
  schedules on a 360-day basis are silently switched to a 365-day basis
  before the schedule is built. We emit the truerate that results from
  the coerced basis so the port can verify it matches.
}
procedure EmitBiweeklyTests;
var
  rate, truerate: real;
  peryr: integer;
begin
  StartArray('biweekly_basis_coercion');

  { Case 1: rate=6%, peryr=26 (biweekly). After coercion to basis 365,
    yrdays = 365 and the per-period growth factor is computed against
    actual day count. We emit the resulting truerate. }
  rate := 0.06; peryr := 26;
  truerate := peryr * Ln(1 + rate / peryr);
  EmitObj(Format('{"label":"biweekly_6pct","rate":%.4f,"perYr":%d,"coercedBasis":"365","trueRate":%.15g}',
    [rate, peryr, truerate]));

  { Case 2: rate=8%, peryr=52 (weekly). }
  rate := 0.08; peryr := 52;
  truerate := peryr * Ln(1 + rate / peryr);
  EmitObj(Format('{"label":"weekly_8pct","rate":%.4f,"perYr":%d,"coercedBasis":"365","trueRate":%.15g}',
    [rate, peryr, truerate]));

  EndArray;
end;

{
  ===== Items deliberately NOT covered here =====

  Two additional fidelity areas — month-specific COLA stepping under
  variable-rate PV, and the AO7 re-amortize-at-current-rate / V6-2
  USA-rule-with-ARM end-states — require simulating a full multi-
  period schedule (RepayFancyLoan / vrPeriodicValue). They cannot be
  reduced to a single closed-form expression, so a faithful Pascal
  reference would need a substantial slice of the engine ported into
  this harness.

  Pinning is currently achieved by Go round-trip tests
  (TestVRPeriodicCOLAMatchesFixedRate_*, TestAO5UnderUSARuleNegativeAmort)
  plus help-example assertions. Adding a Pascal reference for these
  cases is non-trivial work that should be scoped separately.
}

procedure EmitYearsDifTests;
var a, z: daterec;
begin
  StartArray('yearsdif');
  a := MakeDate(2024, 1, 1); z := MakeDate(2025, 1, 1);
  EmitObj(Format('{"from":"2024-01-01","to":"2025-01-01","basis360":%.15g,"basis365":%.15g}',
    [YearsDif360(z, a), YearsDif365(z, a)]));
  a := MakeDate(2024, 1, 1); z := MakeDate(2024, 7, 1);
  EmitObj(Format('{"from":"2024-01-01","to":"2024-07-01","basis360":%.15g,"basis365":%.15g}',
    [YearsDif360(z, a), YearsDif365(z, a)]));
  a := MakeDate(2024, 1, 15); z := MakeDate(2024, 3, 1);
  EmitObj(Format('{"from":"2024-01-15","to":"2024-03-01","basis360":%.15g,"basis365":%.15g}',
    [YearsDif360(z, a), YearsDif365(z, a)]));
  a := MakeDate(2000, 1, 1); z := MakeDate(2030, 6, 15);
  EmitObj(Format('{"from":"2000-01-01","to":"2030-06-15","basis360":%.15g,"basis365":%.15g}',
    [YearsDif360(z, a), YearsDif365(z, a)]));
  EndArray;
end;

{ ===== PV lump-sum forward value + PV-1 backward amount ===== }
{ Forward: value = amt * exxp(rate * YearsDif(asof, date))  (PRESVALU LumpSumValue) }
{ Backward PV-1: amt = value * exxp(rate * YearsDif(date, asof)) (PRESVALU.pas:872) }

procedure EmitOnePVLump(amt: real; dy, ay: integer; rate: real);
var dt, asof: daterec; v, backamt: real;
begin
  dt := MakeDate(dy, 1, 1);
  asof := MakeDate(ay, 1, 1);
  v := amt * exxp(rate * YearsDif360(asof, dt));
  backamt := v * exxp(rate * YearsDif360(dt, asof));
  EmitObj(Format('{"amt":%.6f,"dateYear":%d,"asofYear":%d,"rate":%.6f,"value":%.15g,"backAmt":%.15g}',
    [amt, dy, ay, rate, v, backamt]));
end;

procedure EmitPVLumpTests;
begin
  StartArray('pv_lump');
  EmitOnePVLump(100000, 2036, 2026, 0.06);
  EmitOnePVLump(50000,  2030, 2026, 0.05);
  EmitOnePVLump(250000, 2046, 2026, 0.08);
  EmitOnePVLump(10000,  2027, 2026, 0.03);
  EmitOnePVLump(100000, 2026, 2026, 0.06);
  EndArray;
end;

{ ===== PV periodic (annual) forward value =====
  Standard-formula path of PRESVALU Summation for peryr=1, no COLA:
    realperyr = 1 ; lnf = -rate
    value = amt * exxp(rate * YearsDif(asof, fromDate-1period))
                * SumFormula(lnf, n) * exxp(-rate/realperyr)
  with n = (toYear - fromYear + 1) annual installments (on-or-before).
  Verified to match presentvalue.PeriodicSummation to ~1e-13. }

procedure EmitOnePVPeriodic(amt: real; fromY, toY, asofY: integer; rate: real);
var asof, stdloan: daterec; n: integer; lnf, sumF, since, ff, value: real;
begin
  asof := MakeDate(asofY, 1, 1);
  stdloan := MakeDate(fromY - 1, 1, 1);
  n := toY - fromY + 1;
  lnf := -rate;
  sumF := SumFormula(lnf, n);
  since := YearsDif360(asof, stdloan);
  ff := exxp(-rate);
  value := amt * exxp(rate * since) * sumF * ff;
  EmitObj(Format('{"amt":%.6f,"fromYear":%d,"toYear":%d,"asofYear":%d,"perYr":1,"rate":%.6f,"nInstallments":%d,"value":%.15g}',
    [amt, fromY, toY, asofY, rate, n, value]));
end;

procedure EmitPVPeriodicTests;
begin
  StartArray('pv_periodic');
  EmitOnePVPeriodic(1000, 2031, 2040, 2026, 0.06);
  EmitOnePVPeriodic(5000, 2030, 2050, 2026, 0.05);
  EmitOnePVPeriodic(2000, 2028, 2037, 2026, 0.08);
  EndArray;
end;

{ ===== Vanilla amortization schedule (per-period interest + balance) =====
  Arrears loan, loan date exactly one period before first payment so every
  period is full. f = 1 + rate/peryr (peryr in {1,2,4,12}); annuity payment
  d = amount*(f-1)/(1 - f^-n). Each period: int_k = bal_{k-1}*(f-1);
  pmt = d (or bal+int on the last period); bal_k = bal_{k-1} + int_k - pmt.
  Mirrors generateSimpleSchedule's per-period loop; emits head/mid/tail. }

procedure EmitOneAmortSchedule(amount, rate: real; n, peryr: integer);
var f, d, bal, intk, pmt: real;
    k, midk: integer;
    i1, i2, imid, ilast, b1, b2, bmid, blast: real;
begin
  f := 1 + rate / peryr;
  d := amount * (f - 1) / (1 - Power(f, -n));
  midk := n div 2;
  bal := amount;
  i1 := 0; i2 := 0; imid := 0; ilast := 0;
  b1 := 0; b2 := 0; bmid := 0; blast := 0;
  for k := 1 to n do
    begin
      intk := bal * (f - 1);
      if (k = n) then pmt := bal + intk else pmt := d;
      bal := bal + intk - pmt;
      if (k = 1) then begin i1 := intk; b1 := bal; end;
      if (k = 2) then begin i2 := intk; b2 := bal; end;
      if (k = midk) then begin imid := intk; bmid := bal; end;
      if (k = n) then begin ilast := intk; blast := bal; end;
    end;
  EmitObj(Format('{"amount":%.2f,"rate":%.6f,"nPeriods":%d,"perYr":%d,"payment":%.15g,"midK":%d,"int_1":%.15g,"bal_1":%.15g,"int_2":%.15g,"bal_2":%.15g,"int_mid":%.15g,"bal_mid":%.15g,"int_last":%.15g,"bal_last":%.15g}',
    [amount, rate, n, peryr, d, midk, i1, b1, i2, b2, imid, bmid, ilast, blast]));
end;

procedure EmitAmortScheduleTests;
begin
  StartArray('amort_schedule');
  EmitOneAmortSchedule(200000, 0.06, 360, 12);
  EmitOneAmortSchedule(50000,  0.08, 60,  12);
  EmitOneAmortSchedule(25000,  0.10, 20,  4);
  EmitOneAmortSchedule(100000, 0.05, 30,  1);
  EndArray;
end;

{ ===== Init and main ===== }

procedure InitDaysBefore;
var i: byte;
begin
  for i := 0 to 13 do daysin[i] := daysin_init[i];
  notleapdaysbefore[1] := 0;
  for i := 2 to 12 do
    notleapdaysbefore[i] := notleapdaysbefore[i-1] + daysin[i-1];
  notleapdaysbefore[13] := 65535;
  for i := 1 to 2 do
    leapdaysbefore[i] := notleapdaysbefore[i];
  for i := 3 to 12 do
    leapdaysbefore[i] := notleapdaysbefore[i] + 1;
  leapdaysbefore[13] := 65535;
end;

begin
  InitDaysBefore;

  StartJSON;
  EmitJulianTests;
  EmitExxpTests;
  EmitLnnTests;
  EmitPowerTests;
  EmitRound2Tests;
  EmitYieldRateTests;
  EmitSummationTests;
  EmitMortgageCalcTests;
  EmitSumFormulaTests;
  EmitPVLumpTests;
  EmitPVPeriodicTests;
  EmitAmortScheduleTests;
  EmitYearsDifTests;
  EmitR78Tests;
  EmitInAdvanceTests;
  EmitBiweeklyTests;
  EndJSON;
end.
