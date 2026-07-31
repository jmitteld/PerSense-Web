unit OracleBits;
{ Raw-float64 emission for the headless DOS oracles.

  WHY THIS EXISTS
  ---------------
  Every oracle driver reports its results with Pascal's fixed-point Write
  formatting (`x:0:6`). That formatting is LOSSY in a way that matters here:
  FPC converts a real to ~16 significant digits and only THEN rounds to the
  requested decimals, so it double-rounds. A value whose exact expansion
  continues ...4999x just past the cut is first pulled up to ...5000 and then
  rounded half-up, landing one unit-in-the-last-printed-place above the
  correctly rounded result Go's strconv produces. Measured over 200,000 random
  doubles in the 1e2..1e6 band, FPC's `:0:6` disagrees with Go's `%.6f` on 15
  of them (0.0075%).

  Two consequences, both of which cost real investigation time:

    1. A one-in-the-last-place difference against an oracle is NOT by itself
       evidence of an engine divergence. Two such PV cases were logged as
       unexplained noise before being shown bit-identical.

    2. More importantly, a differential built on formatted decimals can never
       certify bit fidelity, and the existing tolerances (max(0.01, 1e-7|x|)
       for totals, 0.0051 + 1e-9|x| for table lines) hide anything below about
       1e-6. That is exactly how the COLA yield->continuous conversion defect
       survived every sweep to date: it was a systematic last-bits offset on a
       third of all COLA inputs, and no formatted-decimal comparison could see
       it. See docs/discrepancies.md sec 48.

  This unit lets a driver additionally emit the RAW 64-bit pattern of any value
  it prints, so a Go harness can assert the two engines produced the SAME
  double rather than the same rounded text.

  SAFETY CONTRACT (read before changing this)
  -------------------------------------------
  Emission is OFF unless the environment variable PERSENSE_ORACLE_RAWBITS is
  set to a non-empty value. With it unset the drivers' stdout is BYTE-IDENTICAL
  to what it was before this unit existed. That matters because roughly sixty
  Go exec sites parse these binaries and none of them share a parser: several
  use exact field counts, five mortgage parsers walk the whole flattened output
  in stride-2 key/value pairs, three dateutil/interest parsers require the
  ENTIRE stdout to be one number, and the Playwright harness run_mtg.js takes
  the LAST line of stdout. An unconditional extra token or line would break
  some of them.

  When enabled, output is appended as complete new lines of the form

      RAWBITS name=<16 hex>|name=<16 hex>

  chosen to be inert to every parser in the corpus: exactly two whitespace
  tokens (so the stride-2 pair walkers keep their parity), no token equal to
  any scanned key name (the '=' and '|' guarantee it), no leading ERR or
  ENGINE, not the bare word 'end', and not prefixed 'T|'. Call RawBitsFlush
  only AFTER the driver's own output for that mode, and never in `intutil`
  mode, whose callers require stdout to be a bare number. }

interface

uses
  SysUtils;

{ RawBitsOn reports whether emission is enabled for this process. Cheap; the
  environment is read once at unit initialization. }
function RawBitsOn: boolean;

{ RawBitsAdd records one named value. No-op when emission is off, so call
  sites need no guard of their own. }
procedure RawBitsAdd(const name: string; v: double);

{ RawBitsFlush writes the accumulated values as a single RAWBITS line and
  clears the buffer. No-op when emission is off or nothing was recorded.
  Safe to call more than once (a driver with several output phases may flush
  per phase). }
procedure RawBitsFlush;

implementation

type
  TBitCast = record
    case boolean of
      true:  (d: double);
      false: (q: qword);
  end;

var
  enabled: boolean;
  buf: string;
  count: integer;

function RawBitsOn: boolean;
begin
  RawBitsOn := enabled;
end;

procedure RawBitsAdd(const name: string; v: double);
var
  c: TBitCast;
begin
  if not enabled then exit;
  c.d := v;
  if count > 0 then buf := buf + '|';
  buf := buf + name + '=' + HexStr(c.q, 16);
  inc(count);
end;

procedure RawBitsFlush;
begin
  if (not enabled) or (count = 0) then exit;
  { Two whitespace tokens exactly: the tag and the packed payload. }
  Writeln('RAWBITS ', buf);
  buf := '';
  count := 0;
end;

initialization
  enabled := GetEnvironmentVariable('PERSENSE_ORACLE_RAWBITS') <> '';
  buf := '';
  count := 0;

end.
