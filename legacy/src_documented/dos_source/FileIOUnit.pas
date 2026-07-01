{ ============================================================================
  Unit:    FileIOUnit
  Role:    Persistent on-disk file format reader/writer for Per%Sense documents.

  Per%Sense stores three kinds of documents, each in its own binary file with a
  distinct extension and screen "block ID":
    - Mortgage           (*.MTG)  -> MTGBlock
    - Amortization       (*.AMZ)  -> AMZTopBlock (+ sub-blocks)
    - Present Value      (*.PVL)  -> PVLlumpsumblock (+ sub-blocks)

  All three share a COMMON HEADER written by SaveCommon / parsed by LoadFile:
      offset 0 : Word  m_Version   (little-endian; see version* constants below)
      offset 2 : Char  '%'         (magic/identifier byte; validates the file)
      offset 3 : Byte  m_FancyByte (advanced-options flag; AMZ "fancy", PVL "pvlfancy")
      offset 4 : CompDefaults (df.c) computational-settings blob (sizeof-copied)
      then     : GridHeaderArray = 8 x TGridHeader (one slot per on-screen grid block)

  After the header come the per-block data records.  Each scalar field is stored
  as [1-byte inout status][value].  The 'inout' status byte records data
  provenance: empty=0 (blank), outp/calculated=1, defp=2 (default), inp=3 (user
  input).  This status travels with every field so the field-presence dispatch
  can be reconstructed exactly on reload.

  BYTE/LAYOUT ASSUMPTIONS (load-bearing for byte-for-byte compatibility):
    - All 'real' (Pascal extended) values are serialized through a real48
      temporary: 6 bytes, the Turbo/Borland 48-bit "Real" type.  In-memory the
      app uses native 'real' (often 8-byte), so every read goes
      real48 -> assign -> real, and every write goes real -> real48 -> stream.
    - Integers (Years, When, peryr, nperiods, status, ninstallments) are written
      as fixed widths (2 bytes) matching the original DOS struct, NOT sizeof().
    - daterec is streamed with sizeof(daterec); see PETYPES daterec (d,m,y ints).
    - Files are little-endian (DOS/x86); no endian conversion is performed, so
      the format is implicitly x86-only.
    - Mortgage records have a trailing pad byte (value 72) after each line; the
      loader skips it with Seek(1, soFromCurrent).  Every file ends with the
      sentinel byte #254.

  VERSION HANDLING: version constants are kept for historical DOS releases, but
  on save the unit always stamps version31 (3.1).  m_Version is read back on
  load and exposed via GetVersion(), though the current loaders do not branch on
  it -- the record layout is assumed stable across the supported versions.
  NOTE: there is no explicit per-version layout switch here; older-version
  files are trusted to share the v3.1 layout.

  Ported reference: the Go port mirrors these layouts in internal/fileio.
  ============================================================================ }
unit FileIOUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  ExtCtrls, Globals, peTypes;

const
  { Go port: n/a -- the loaders in internal/fileio/reader.go read m_Version but
    do not branch on it (all supported files share the v3.1 layout), so these
    stamps are not carried as Go constants. }
  { File-format version stamps, encoded as (major shl 8) + minor.  Inherited
    from the DOS Per%Sense releases.  Files always SAVE as version31 (3.1);
    the others are retained for reading/recognizing older documents. }
  version11=1 shl 8 + 5; {Version 1.00f changed default structure}
  version20=2 shl 8;
  version30=3 shl 8;
  version22=2 shl 8 + 2;
  version31=3 shl 8 + 1;

type
  { Day-count basis selector (365 actual / 360 / 365-over-360).  Declared here
    for file context; the live computation reads basis from df.c. }
  basistype = (x365,x360,x365_360);

  { Go port: internal/fileio/reader.go: FileHeader (parsed in readFrom, line 96)
    holds the equivalent per-block table of contents; ReadFile / ReadBytes
    (lines 78/90) expose it.  There is no standalone Go TGridHeader struct --
    the GridID/ScrollPosition/LineCount triples are read inline. }
  { On-disk descriptor for one on-screen grid "block".  Eight of these form the
    file's table of contents (one per possible screen block, indexed by GridID).
      GridID         - which block this is (MTGBlock, AMZ* or PVL* constant)
      ScrollPosition - saved scroll offset (cosmetic; written 0 on save)
      LineCount      - number of populated data rows that follow for this block }
  TGridHeader = record
    GridID : byte;
    ScrollPosition : byte;
    LineCount : byte;
  end;
  { Fixed 8-slot header array (file "table of contents") written right after the
    common header.  Slot 0's GridID identifies the document type on load. }
  GridHeaderArray = array[0..7] of TGridHeader;

  // a generalized class used to store and load all the file
  // types from a file.
  { Go port: internal/fileio (package) is the analogue of this class.  The Go
    port is LOAD-only; the load entry points are internal/fileio/loader.go:
    LoadMortgageFile/LoadMortgageBytes (line 44/55),
    LoadAmortizationFile/LoadAmortizationBytes (line 107/116),
    LoadPresentValueFile/LoadPresentValueBytes (line 169/178), returning
    MortgageFile / AmortizationFile / PresentValueFile structs (no Get*
    accessor step).  The .PSN web-import path is internal/api/import_psn.go:
    HandleImportPSN (line 121).  The Save* methods are not ported. }
  { TFileIO -- one object handles read AND write for ALL three document types.
    Usage on LOAD:  Create -> LoadFile(name) -> GetIDByte to learn the type ->
      Get<Type>Data/Array to copy the parsed records out -> Free.
    Usage on SAVE:  Create -> Save<Type>(...) -> Free.
    On load, parsed records are held in the private m_* buffers until a
    Get* accessor copies them into the caller's structures. }
  TFileIO = class
  public
    constructor Create();
    destructor Destroy();
    { Open and parse a file: validate header, dispatch on block ID to the right
      Load* routine. Returns false (and shows a MessageBox) on bad/unknown file. }
    function LoadFile( Name: string ) : boolean;
    { Accessor: file-format version word parsed from the header. }
    function GetVersion() : word;
    { Accessor: document-type id (GridID of slot 0); tells caller MTG/AMZ/PVL. }
    function GetIDByte() : byte;
    { Accessor: path the file was loaded from. }
    function GetFileName(): string;
    { Accessor: the advanced-options flag byte (AMZ fancy / PVL pvlfancy). }
    function GetFancyByte(): byte;
    { Copy the parsed mortgage rows into the caller's array. (load side) }
    function GetMortgageArray( var Mortgages: mtgarray ): boolean;
    { Copy parsed amortization loan + all advanced-option sub-blocks out. (load) }
    function GetAmortizationData( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                                  var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                                  Target: targetptr; Skip: skipptr ): boolean;
    { Copy parsed present-value blocks (lumpsum, periodic, presval/rateline, extra) out. (load) }
    function GetPresentValueData( var Lump: LumpSumArray; var ThePeriodic: PeriodicArray;
                                  var Pres: PresValArray; var TheRateLine: Ratelinearray;
                                  XPres: xpresvalptr ): boolean;
    { Serialize a full mortgage screen to a .MTG file. }
    function SaveMortgage( FileName: string; Mortgages: mtgarray ): boolean;
    { Serialize an amortization screen (loan + advanced options) to a .AMZ file. }
    function SaveAmortization( FileName: string; AMZ:AMZPtr; Payoff: balloonptr;
                               Prepayment: prepaymentarray; Balloon: balloonarray;
                               ADJ: adjarray; Mor: Moratoriumptr; Target: targetptr;
                               Skip: skipptr ) : boolean;
    { Serialize a present-value screen to a .PVL file. }
    function SavePresentValue( FileName: string; Lump: LumpSumArray; ThePeriodic: PeriodicArray;
                               Pres: PresValArray; TheRateLine: Ratelinearray; XPres: xpresvalptr ): boolean;
  private
    { --- parsed-header scalars --- }
    m_Version: word;     { file-format version word from the header }
    m_IDByte: byte;      { document-type id (slot-0 GridID) }
    m_FancyByte: byte;   { advanced-options flag from the header }
    m_FileName: string;  { source path of the loaded file }
    // for mortgages
    m_Mortgages: mtgarray;          { parsed mortgage rows (allocated in Create) }
    // for amortization
    m_AMZ: AMZLoan;                 { core amortization loan record }
    m_Payoff: balloonrec;           { payoff/balance block }
    m_Prepayment: prepaymentarray;  { prepayment rows (AMZPreBlock) }
    m_Balloon: balloonarray;        { balloon rows (AMZBalloonBlock) }
    m_ADJ: adjarray;                { rate/payment adjustment rows (AMZRateChangeBlock) }
    m_Mor: Moratoriumrec;           { moratorium block }
    m_Target: targetrec;            { target-principal block }
    m_Skip: skiprec;                { skip-months block }
    // for Present Value
    m_LumpSum: lumpsumarray;        { PV lump-sum rows }
    m_Periodic: periodicarray;      { PV periodic-payment rows }
    m_PresVal: presvalarray;        { PV present-value rows (non-fancy) }
    m_RateLine: ratelinearray;      { PV rate-line rows (fancy mode) }
    m_XPresVal: xpresval;           { PV extra/fancy summary block }
    { Parse LineCount mortgage rows from the stream into m_Mortgages. }
    function LoadMortgageData( LineCount: integer; FileStream: TFileStream ): boolean;
    { Walk the 8 grid headers and parse each present amortization sub-block. }
    function LoadAmortizationData( GridArray: GridHeaderArray; FileStream: TFileStream ): boolean;
    { Walk the 8 grid headers and parse each present present-value sub-block. }
    function LoadPresentValueData( GridArray: GridHeaderArray; FileStream: TFileStream ): boolean;
    { Write the shared header (version, '%', fancy byte, df.c) at file start. }
    function SaveCommon( FileStream: TFileStream ): boolean;
  end;

implementation

uses Mortgage, peData, Amortize, Presvalu, HelpSystemUnit;

{ Constructor: pre-allocates and zeroes every pointer-backed record buffer for
  all three document types (mortgage lines, lump sums, periodics, rate lines,
  prepayments, balloons, adjustments, presvals) plus the xpresval block.  Because
  one TFileIO can be used for any file type, all buffers are allocated up front.
  Side effects: heap allocation via GetMem; matched in Destroy. }
constructor TFileIO.Create();
var
  i: integer;
begin
  m_Version := 0;
  m_IDByte := 0;
  m_FancyByte := 0;
  for i:=1 to maxlines do begin
    GetMem( m_Mortgages[i], sizeof(mtgline) );
    ZeroMortgage( mtgptr(m_Mortgages[i]) );
    GetMem( m_LumpSum[i], sizeof(lumpsum) );
    ZeroLumpSum( lumpsumptr(m_LumpSum[i]) );
    GetMem( m_Periodic[i], sizeof(periodic) );
    ZeroPeriodic( periodicptr(m_Periodic[i]) );
    GetMem( m_RateLine[i], sizeof(rateline) );
    ZeroRateLine( ratelineptr(m_RateLine[i]) );
  end;
  for i:=1 to maxprepay do begin
    GetMem( m_Prepayment[i], sizeof(prepaymentrec) );
    ZeroPrepayment( prepaymentptr(m_Prepayment[i]) );
  end;
  for i:=1 to maxballoon do begin
    GetMem( m_Balloon[i], sizeof(balloonrec) );
    ZeroBalloon( balloonptr(m_Balloon[i]) );
  end;
  for i:=1 to maxadj do begin
    GetMem( m_ADJ[i], sizeof(adjrec) );
    ZeroAdjustment( adjptr(m_ADJ[i]) );
  end;
  for i:=1 to presvallines do begin
    GetMem( m_PresVal[i], sizeof(presval) );
    ZeroPresVal( presvalptr(m_PresVal[i]) );
  end;
  ZeroXPresVal( @m_XPresVal );
end;

{ Destructor: frees the heap buffers allocated in Create.
  NOTE: only m_Mortgages, m_Prepayment, m_Balloon and m_ADJ are freed here;
  the lump-sum / periodic / rate-line / presval buffers allocated in Create
  are NOT freed -- an original DOS-era memory leak preserved verbatim. }
destructor TFileIO.Destroy();
var
  i: integer;
begin
  for i:=1 to maxlines do
    FreeMem( m_Mortgages[i] );
  for i:=1 to maxprepay do
    FreeMem( m_Prepayment[i] );
  for i:=1 to maxballoon do
    FreeMem( m_Balloon[i] );
  for i:=1 to maxadj do
    FreeMem( m_ADJ[i] );
end;

{ Returns the parsed document-type id (slot-0 GridID). Trivial accessor. }
function TFileIO.GetIDByte() : byte;
begin
  GetIDByte := m_IDByte;
end;

{ Returns the parsed file-format version word. Trivial accessor. }
function TFileIO.GetVersion() : word;
begin
  GetVersion := m_Version;
end;

{ Returns the source file path. Trivial accessor. }
function TFileIO.GetFileName(): string;
begin
  GetFileName := m_FileName;
end;

{ Returns the advanced-options flag byte from the header. Trivial accessor. }
function TFileIO.GetFancyByte(): byte;
begin
  GetFancyByte := m_FancyByte;
end;

{ Copies the parsed mortgage rows from the internal buffer into the caller's
  array (deep record copy through the pointers). Always returns true. }
function TFileIO.GetMortgageArray( var Mortgages: mtgarray ): boolean;
var
  i: integer;
begin
  for i:=1 to maxlines do
    Mortgages[i]^ := m_Mortgages[i]^;
  GetMortgageArray := true;
end;

{ Copies all parsed amortization sub-blocks (core loan, payoff, prepayments,
  balloons, adjustments, moratorium, target, skip) into the caller-supplied
  records/arrays. Always returns true. }
function TFileIO.GetAmortizationData( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                                      var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                                      Target: targetptr; Skip: skipptr ): boolean;
var
  i: integer;
begin
  AMZ^ := m_AMZ;
  Payoff^ := m_Payoff;
  for i:=1 to maxprepay do
    Prepayment[i]^ := m_Prepayment[i]^;
  for i:=1 to maxballoon do
    Balloon[i]^ := m_Balloon[i]^;
  for i:=1 to maxadj do
    ADJ[i]^ := m_ADJ[i]^;
  Mor^ := m_Mor;
  Target^ := m_Target;
  Skip^ := m_Skip;
  GetAmortizationData := true;
end;

{ Copies all parsed present-value blocks (lump sums, periodics, rate lines,
  presvals, and the extra/fancy block) into the caller's structures. Note both
  the presval and rateline arrays are copied regardless of fancy mode; the
  caller uses whichever matches GetFancyByte. Always returns true. }
function TFileIO.GetPresentValueData( var Lump: LumpSumArray; var ThePeriodic: PeriodicArray;
                                      var Pres: PresValArray; var TheRateLine: Ratelinearray;
                                      XPres: xpresvalptr ): boolean;
var
  i: integer;
begin
  for i:=1 to maxlines do begin
    Lump[i]^ := m_LumpSum[i]^;
    ThePeriodic[i]^ := m_Periodic[i]^;
    TheRateLine[i]^ := m_RateLine[i]^;
  end;
  for i:=1 to presvallines do begin
    Pres[i]^ := m_PresVal[i]^;
  end;
  XPres^ := m_XPresVal;
  GetPresentValueData := true;
end;

//
// all the loading functions are below
//
{ Go port: internal/fileio/reader.go: readFrom (line 96) parses the shared
  header (version word, '%' magic, fancy byte, df.c blob, 8-slot grid TOC) into
  a FileHeader; internal/fileio/loader.go dispatches on the slot-0 GridID in
  loadMortgageFromHeaderAndData / loadAmortizationFromHeaderAndData /
  loadPresentValueFromHeaderAndData (lines 63/124/186). }
{ LoadFile -- top-level loader/dispatcher.
  Params:  Name = path to open.
  Returns: true on a successfully parsed, recognized file; false otherwise
           (also pops a MessageBox describing the failure).
  Steps:   open read-only -> read version Word -> read+validate the '%' magic
           byte -> read fancy byte -> read df.c computational defaults ->
           read the 8-entry grid header -> dispatch on slot-0 GridID to the
           type-specific Load* routine. Frees the stream before returning.
  Side effects: sets m_FileName, m_Version, m_FancyByte, df.c, m_IDByte and the
           m_* record buffers. }
function TFileIO.LoadFile( Name: string ) : boolean;
var
  Identifier: char;
  ScreenFileHeader: GridHeaderArray;
  BytesRead: longint;
  RetVal: boolean;
  FileStream: TFileStream;
begin
  try
    FileStream := TFileStream.Create( Name, fmOpenRead );
  except
    on EFOpenError do begin
      MessageBox( 'Error opening specified file', DO_OpeningFile );
      LoadFile := false;
      exit;
    end;
  end;
  m_FileName := Name;
  BytesRead := FileStream.Read( m_Version, 2 );       { 2-byte version word }
  BytesRead := FileStream.Read( Identifier, 1 );      { magic byte must be '%' }
  if( (Identifier<>'%') or (BytesRead<>1) ) then begin
    MessageBox( 'Invalid file', DO_InvalidFile );     { wrong magic -> not ours }
    FileStream.Free();
    LoadFile := false;
    exit;
  end;
  BytesRead := FileStream.Read( m_FancyByte, 1 );             { advanced-options flag }
  BytesRead := FileStream.Read( df.c, sizeof(CompDefaults) ); { computational defaults blob }
  BytesRead := FileStream.Read( ScreenFileHeader, sizeof(TGridHeader)*8 ); { 8-block TOC }
  RetVal := false;
  m_IDByte := ScreenFileHeader[0].GridID;             { first block decides document type }
  case m_IDByte of
    MTGBlock : RetVal := LoadMortgageData( ScreenFileHeader[0].LineCount, FileStream );
    AMZTopBlock: RetVal := LoadAmortizationData( ScreenFileHeader, FileStream );
    PVLlumpsumblock: RetVal := LoadPresentValueData( ScreenFileHeader, FileStream );
  else
    MessageBox( 'Unknown File Type', DO_UnkownFileType );
  end;
  FileStream.Free();
  LoadFile := RetVal;
end;

{ Go port: internal/fileio/loader.go: loadMortgageFromHeaderAndData (line 63)
  reads the same per-row [status][value] fields; each real48 field goes through
  reader.go: readReal48 (line 151), Years/When through readInt16LE (line 187),
  and the trailing 72 pad byte is skipped positionally. }
{ LoadMortgageData -- read LineCount mortgage rows from the stream.
  Layout per row, repeated: each numeric field is a 1-byte inout status followed
  by its value.  'real' fields are stored as 6-byte real48 and widened on read;
  integer fields (Years, When) are 2 bytes.  After the 11 fields, one trailing
  pad byte (the 72 written on save) is skipped with Seek.  Returns true. }
function TFileIO.LoadMortgageData( LineCount: integer; FileStream: TFileStream ): boolean;
var
  i: integer;
  ShortReal: real48;   { 6-byte Borland Real scratch; each real field round-trips through this }
begin
  for i:=1 to LineCount do begin
    { each pair below = [1-byte status][6-byte real48], status preserved verbatim }
    FileStream.Read( m_Mortgages[i].PriceStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Price := ShortReal;
    FileStream.Read( m_Mortgages[i].PointsStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Points := ShortReal;
    FileStream.Read( m_Mortgages[i].PctStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Pct := ShortReal;
    FileStream.Read( m_Mortgages[i].CashStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Cash := ShortReal;
    FileStream.Read( m_Mortgages[i].FinancedStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Financed := ShortReal;
    FileStream.Read( m_Mortgages[i].YearsStatus, 1 );
    FileStream.Read( m_Mortgages[i].Years, 2 );
    FileStream.Read( m_Mortgages[i].RateStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Rate := ShortReal;
    FileStream.Read( m_Mortgages[i].TaxStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Tax := ShortReal;
    FileStream.Read( m_Mortgages[i].MonthlyStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].Monthly := ShortReal;
    FileStream.Read( m_Mortgages[i].WhenStatus, 1 );
    FileStream.Read( m_Mortgages[i].When, 2 );
    FileStream.Read( m_Mortgages[i].HowMuchStatus, 1 );
    FileStream.Read( ShortReal, 6 );
    m_Mortgages[i].HowMuch := ShortReal;
    FileStream.Seek( 1, soFromCurrent );   { skip the per-row trailing pad byte (72) }
  end;
  LoadMortgageData := true;
end;

{ Go port: internal/fileio/loader.go: loadAmortizationFromHeaderAndData (line
  124) walks the grid TOC and parses each sub-block via readAMZLoan (line 227),
  readBalloonPayment (253), readPrepayment (262), readAdjustment (278); daterec
  fields use reader.go: readDateRec (line 199). }
{ LoadAmortizationData -- read all populated amortization sub-blocks.
  Iterates the 8 grid-header slots and, per slot's GridID, parses that block:
    AMZTopBlock       core loan record (amount/dates/rate/term/payment/points/apr)
    AMZBalanceBlock   payoff date+amount
    AMZPreBlock       LineCount prepayment rows
    AMZBalloonBlock   LineCount balloon rows
    AMZRateChangeBlock LineCount rate/payment adjustment (ARM) rows
    AMZMoratoriumBlock interest-only deferment date
    AMZTargetBlock    minimum-principal target
    AMZSkipMonthBlock 15-byte skip-months string
  Same [status][value] encoding; daterec via sizeof; reals via real48. Returns true. }
function TFileIO.LoadAmortizationData( GridArray: GridHeaderArray; FileStream: TFileStream ): boolean;
var
  i, j: integer;
  ShortReal: real48;
begin
  for i:=0 to 7 do begin
    case GridArray[i].GridID of
      AMZTopBlock: begin
        FileStream.Read( m_AMZ.amountstatus, 1);
        FileStream.Read( ShortReal, 6 );
        m_AMZ.amount := ShortReal;
        FileStream.Read( m_AMZ.loandatestatus, 1);
        FileStream.Read( m_AMZ.loandate, sizeof(daterec) );
        FileStream.Read( m_AMZ.loanratestatus, 1);
        FileStream.Read( ShortReal, 6 );
        m_AMZ.loanrate := ShortReal;
        FileStream.Read( m_AMZ.firststatus, 1);
        FileStream.Read( m_AMZ.firstdate, sizeof(daterec) );
        FileStream.Read( m_AMZ.nstatus, 1);
        FileStream.Read( m_AMZ.nperiods, 2 );
        FileStream.Read( m_AMZ.laststatus, 1);
        FileStream.Read( m_AMZ.lastdate, sizeof(daterec) );
        FileStream.Read( m_AMZ.peryrstatus, 1);
        FileStream.Read( m_AMZ.peryr, 2 );
        FileStream.Read( m_AMZ.payamtstatus, 1);
        FileStream.Read( ShortReal, 6 );
        m_AMZ.payamt := ShortReal;
        FileStream.Read( m_AMZ.pointsstatus, 1);
        FileStream.Read( ShortReal, 6 );
        m_AMZ.points := ShortReal;
        FileStream.Read( m_AMZ.aprstatus, 1);
        FileStream.Read( ShortReal, 6 );
        m_AMZ.apr := ShortReal;
        FileStream.Read( m_AMZ.lastok, 1);
      end;
      AMZBalanceBlock: begin
        FileStream.Read( m_Payoff.datestatus, 1 );
        FileStream.Read( m_Payoff.date, sizeof(daterec) );
        FileStream.Read( m_Payoff.amountstatus, 1 );
        FileStream.Read( ShortReal, 6 );
        m_Payoff.amount := ShortReal;
      end;
      AMZPreBlock: begin
        for j:=1 to GridArray[i].LineCount do begin
          FileStream.Read( m_Prepayment[j].startdatestatus, 1 );
          FileStream.Read( m_Prepayment[j].startdate, sizeof(daterec) );
          FileStream.Read( m_Prepayment[j].nnstatus, 1 );
          FileStream.Read( m_Prepayment[j].nn, 2 );
          FileStream.Read( m_Prepayment[j].stopdatestatus, 1 );
          FileStream.Read( m_Prepayment[j].stopdate, sizeof(daterec) );
          FileStream.Read( m_Prepayment[j].peryrstatus, 1 );
          FileStream.Read( m_Prepayment[j].peryr, 2 );
          FileStream.Read( m_Prepayment[j].paymentstatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_Prepayment[j].payment := ShortReal;
          FileStream.Read( m_Prepayment[j].nextdate, sizeof(daterec) );
        end;
      end;
      AMZBalloonBlock: begin
        for j:=1 to GridArray[i].LineCount do begin
          FileStream.Read( m_Balloon[j].datestatus, 1 );
          FileStream.Read( m_Balloon[j].date, sizeof(daterec) );
          FileStream.Read( m_Balloon[j].amountstatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_Balloon[j].amount := ShortReal;
        end;
      end;
      AMZRateChangeBlock: begin
        for j:=1 to GridArray[i].LineCount do begin
          FileStream.Read( m_ADJ[j].datestatus, 1 );
          FileStream.Read( m_ADJ[j].date, sizeof(daterec) );
          FileStream.Read( m_ADJ[j].loanratestatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_ADJ[j].loanrate := ShortReal;
          FileStream.Read( m_ADJ[j].amountstatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_ADJ[j].amount := ShortReal;
          FileStream.Read( m_ADJ[j].amtok, 1 );
        end;
      end;
      AMZMoratoriumBlock: begin
        FileStream.Read( m_Mor.first_repaystatus, 1 );
        FileStream.Read( m_Mor.first_repay, sizeof(daterec) );
      end;
      AMZTargetBlock: begin
        FileStream.Read( m_Target.targetstatus, 1 );
        FileStream.Read( ShortReal, 6 );
        m_Target.target := ShortReal;
      end;
      AMZSkipMonthBlock: begin
        FileStream.Read( m_Skip.skipstatus, 1 );
        FileStream.Read( m_Skip.skipmonths, 15 );
      end;
    end;
  end;
  LoadAmortizationData := true;
end;

{ Go port: internal/fileio/loader.go: loadPresentValueFromHeaderAndData (line
  186) parses the lump-sum/periodic/third blocks via readLumpSum (line 290),
  readPeriodic (303) and readPresValLine (323).  The port mirrors the fancy-byte
  polymorphism of the third block (rateline vs. presval) and the trailing
  xpresval block. }
{ LoadPresentValueData -- read all populated present-value sub-blocks.
  Iterates the 8 grid-header slots, parsing by GridID:
    PVLlumpsumblock   LineCount lump-sum rows (date, amt0, val0, status, act0)
    PVLperiodicblock  LineCount periodic rows (from/to dates, peryr, amtn, cola,
                      valn, status, ninstallments, actn)
    PVLpresvalblock   the third block is POLYMORPHIC on m_FancyByte:
                        fancy (>0)  -> rate-line rows (date + raterec)
                        plain (0)   -> presval rows (asof, raterec, sumvalue,
                                       status, duration)
    PVLExtraBlock     present only when fancy: the xpresval summary block
  Same [status][value] encoding throughout. Returns true. }
function TFileIO.LoadPresentValueData( GridArray: GridHeaderArray; FileStream: TFileStream ): boolean;
var
  i, j: integer;
  ShortReal: real48;
begin
  for i:=0 to 7 do begin
    case GridArray[i].GridID of
      PVLlumpsumblock: begin
        for j:=1 to GridArray[i].LineCount do begin
          FileStream.Read( m_LumpSum[j].datestatus, 1 );
          FileStream.Read( m_LumpSum[j].date, sizeof(daterec) );
          FileStream.Read( m_LumpSum[j].amt0status, 1 );
          FileStream.Read( ShortReal, 6 );
          m_LumpSum[j].amt0 := ShortReal;
          FileStream.Read( m_LumpSum[j].val0status, 1 );
          FileStream.Read( ShortReal, 6 );
          m_LumpSum[j].val0 := ShortReal;
          FileStream.Read( m_LumpSum[j].status, 2 );
          FileStream.Read( m_LumpSum[j].act0, 1 );
        end;
      end;
      PVLperiodicblock: begin
        for j:=1 to GridArray[i].LineCount do begin
          FileStream.Read( m_Periodic[j].fromdatestatus, 1 );
          FileStream.Read( m_Periodic[j].fromdate, sizeof(daterec) );
          FileStream.Read( m_Periodic[j].todatestatus, 1 );
          FileStream.Read( m_Periodic[j].todate, sizeof(daterec) );
          FileStream.Read( m_Periodic[j].peryrstatus, 1 );
          FileStream.Read( m_Periodic[j].peryr, 2 );
          FileStream.Read( m_Periodic[j].amtnstatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_Periodic[j].amtn := ShortReal;
          FileStream.Read( m_Periodic[j].colastatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_Periodic[j].cola := ShortReal;
          FileStream.Read( m_Periodic[j].valnstatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_Periodic[j].valn := ShortReal;
          FileStream.Read( m_Periodic[j].status, 2 );
          FileStream.Read( m_Periodic[j].ninstallments, 2 );
          FileStream.Read( m_Periodic[j].actn, 1 );
        end;
      end;
      PVLpresvalblock: begin
        { Block 3 has two distinct on-disk layouts; the fancy byte selects which. }
        if( m_FancyByte>0 ) then begin
          { fancy: a schedule of rate-lines (effective rate changes over time) }
          for j:=1 to GridArray[i].LineCount do begin
            FileStream.Read( m_RateLine[j].datestatus, 1 );
            FileStream.Read( m_RateLine[j].date, sizeof(daterec) );
            FileStream.Read( m_RateLine[j].r.status, 1 );
            FileStream.Read( ShortReal, 6 );
            m_RateLine[j].r.rate := ShortReal;
            FileStream.Read( m_RateLine[j].r.peryr, 1 );
            FileStream.Read( m_RateLine[j].status, 2 );
          end;
        end else begin
          { plain: ordinary present-value query rows (as-of date, rate, sum, duration) }
          for j:=1 to GridArray[i].LineCount do begin
            FileStream.Read( m_PresVal[j].asofstatus, 1 );
            FileStream.Read( m_PresVal[j].asof, sizeof(daterec) );
            FileStream.Read( m_PresVal[j].r.status, 1 );
            FileStream.Read( ShortReal, 6 );
            m_PresVal[j].r.rate := ShortReal;
            FileStream.Read( m_PresVal[j].r.peryr, 1 );
            FileStream.Read( m_PresVal[j].sumvaluestatus, 1 );
            FileStream.Read( ShortReal, 6 );
            m_PresVal[j].sumvalue := ShortReal;
            FileStream.Read( m_PresVal[j].status, 2 );
            FileStream.Read( m_PresVal[j].durationstatus, 1 );
            FileStream.Read( ShortReal, 6 );
            m_PresVal[j].duration := ShortReal;
          end;
        end;
      end;
      PVLExtraBlock: begin
        if( m_FancyByte>0 ) then begin
          FileStream.Read( m_XPresVal.xasofstatus, 1 );
          FileStream.Read( m_XPresVal.xasof, sizeof(daterec) );
          FileStream.Read( m_XPresVal.simplestatus, 1 );
          FileStream.Read( m_XPresVal.simple, 1 );
          FileStream.Read( m_XPresVal.xvaluestatus, 1 );
          FileStream.Read( ShortReal, 6 );
          m_XPresVal.xvalue := ShortReal;
          FileStream.Read( m_XPresVal.status, 1 );
        end;
      end;
    end;
  end;
  LoadPresentValueData := true;
end;

//
// all the saving functions are below
//
{ Go port: n/a -- the web port is load-only; there is no file-write path.  The
  inverse daterec<->bytes helpers exist in internal/fileio/loader.go
  (dateRecToBytes line 341, newDateRecFromBytes line 353) and real48.go
  (Float64ToReal48 line 55) for round-trip/testing, but no Save* is wired up. }
{ SaveCommon -- write the shared file header at the start of every saved file:
  version word (always stamped version31), the '%' magic byte, the fancy byte
  (set by the caller before calling), and the df.c computational-defaults blob.
  Caller must set m_FancyByte first. Returns true. }
function TFileIO.SaveCommon( FileStream: TFileStream ): boolean;
var
  Version: Word;
  Identifier: char;
begin
  Version := version31;
  FileStream.Write( Version, 2 );
  Identifier := '%';
  FileStream.Write( Identifier, 1 );
  FileStream.Write( m_FancyByte, 1 );
  FileStream.Write( df.c, sizeof(CompDefaults) );
  SaveCommon := true;
end;

{ SaveMortgage -- serialize the mortgage screen to a .MTG file.
  Params:  FileName = target path; Mortgages = the in-memory rows.
  Writes:  common header (fancy byte forced 0) -> grid header with slot 0 =
           MTGBlock and LineCount = index of the last non-empty row -> each
           non-empty row as [status][value] fields (reals as real48, Years/When
           as 2-byte ints) each followed by the 72 pad byte -> #254 end sentinel.
  NOTE: result var SaveMortgage is set false at entry and never set true on the
  normal path, so this returns false even on success -- a latent bug kept as-is
  (SaveAmortization/SavePresentValue do set their result true). }
function TFileIO.SaveMortgage( FileName: string; Mortgages: mtgarray ): boolean;
var
  FileStream: TFileStream;
  ScreenFileHeader: array [0..7] of TGridHeader;
  i: integer;
  ShortReal: real48;
  ch: char;
  SeventyTwo: byte;       { the per-row pad/terminator byte written after each line }
  Count: integer;         { highest non-empty row index = rows to write }
begin
  SaveMortgage := false;
  FileStream := TFileStream.Create( FileName, fmOpenWrite );
  if( FileStream = nil ) then exit;
  m_FancyByte := 0;
  if( not SaveCommon( FileStream ) ) then exit;
  Count := 0;
  for i:=1 to maxlines do begin
    if( not MortgageIsEmpty( mtgptr(Mortgages[i]) ) ) then
      Count := i;
  end;
  ScreenFileHeader[0].GridID := MTGBlock;
  ScreenFileHeader[0].LineCount := Count;
  FileStream.Write( ScreenFileHeader, sizeof(TGridHeader)*8 );
  SeventyTwo := 72;
  for i:=1 to Count do begin
    if( not MortgageIsEmpty(mtgptr(Mortgages[i])) ) then begin
      FileStream.Write( Mortgages[i].PriceStatus, 1 );
      ShortReal := Mortgages[i].Price;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].PointsStatus, 1 );
      ShortReal := Mortgages[i].Points;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].PctStatus, 1 );
      ShortReal := Mortgages[i].Pct;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].CashStatus, 1 );
      ShortReal := Mortgages[i].Cash;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].FinancedStatus, 1 );
      ShortReal := Mortgages[i].Financed;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].YearsStatus, 1 );
      FileStream.Write( Mortgages[i].Years, 2 );
      FileStream.Write( Mortgages[i].RateStatus, 1 );
      ShortReal := Mortgages[i].Rate;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].TaxStatus, 1 );
      ShortReal := Mortgages[i].Tax;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].MonthlyStatus, 1 );
      ShortReal := Mortgages[i].Monthly;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Mortgages[i].WhenStatus, 1 );
      FileStream.Write( Mortgages[i].When, 2 );
      FileStream.Write( Mortgages[i].HowMuchStatus, 1 );
      ShortReal := Mortgages[i].HowMuch;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( SeventyTwo, 1 );
    end;
  end;
  ch := #254;
  FileStream.Write( ch, 1 );    { end-of-file sentinel }
  FileStream.Free();
end;

{ SaveAmortization -- serialize the amortization screen (core loan plus every
  advanced-option block) to a .AMZ file.
  Params:  FileName plus pointers/arrays for each block.
  Header:  fancy byte = byte(fancy); builds the 8-slot grid header for the AMZ
           blocks AMZTopBlock..AMZSkipMonthBlock, computing per-block LineCount
           (1 for the singleton blocks; the last non-empty index for the
           prepayment/balloon/adjustment arrays).
  Body:    writes the core loan, payoff, prepayments, balloons, adjustments,
           moratorium, target and skip blocks in that fixed order, [status][value]
           encoded, then the #254 sentinel. Returns true on success. }
function TFileIO.SaveAmortization( FileName: string; AMZ:AMZPtr; Payoff: balloonptr;
                                   Prepayment: prepaymentarray; Balloon: balloonarray;
                                   ADJ: adjarray; Mor: Moratoriumptr; Target: targetptr;
                                   Skip: skipptr ) : boolean;
var
  FileStream: TFileStream;
  ScreenFileHeader: array [0..7] of TGridHeader;
  i, j: integer;
  ShortReal: real48;
  ch: char;
  PrepayCount, ADJCount, BalloonCount: integer;
begin
  SaveAmortization := false;
  FileStream := TFileStream.Create( FileName, fmOpenWrite );
  if( FileStream = nil ) then exit;
  m_FancyByte := byte(fancy);
  if( not SaveCommon( FileStream ) ) then exit;
  for i:=AMZTopBlock to AMZSkipMonthBlock do begin
    ScreenFileHeader[i-AMZTopBlock].GridID := i;
    ScreenFileHeader[i-AMZTopBlock].ScrollPosition := 0;
    case i of
      AMZTopBlock: ScreenFileHeader[i-AMZTopBlock].LineCount := 1;
      AMZBalanceBlock: ScreenFileHeader[i-AMZTopBlock].LineCount := 1;
      AMZPreBlock: begin
        PrepayCount := 0;
        for j:=1 to maxprepay do begin
          if( not PrepaymentIsEmpty(prepaymentptr(Prepayment[j])) ) then
            PrepayCount := j;
        end;
        ScreenFileHeader[i-AMZTopBlock].LineCount := PrepayCount;
      end;
      AMZBalloonBlock: begin
        BalloonCount := 0;
        for j:=1 to maxballoon do begin
          if( not BalloonIsEmpty(balloonptr(Balloon[j])) ) then
            BalloonCount := j;
        end;
        ScreenFileHeader[i-AMZTopBlock].LineCount := BalloonCount;
      end;
      AMZRateChangeBlock: begin
        ADJCount := 0;
        for j:=1 to maxadj do begin
          if( not AdjustmentIsEmpty(adjptr(ADJ[j])) ) then
            ADJCount := j;
        end;
        ScreenFileHeader[i-AMZTopBlock].LineCount := ADJCount;
      end;
      AMZMoratoriumBlock: ScreenFileHeader[i-AMZTopBlock].LineCount := 1;
      AMZTargetBlock: ScreenFileHeader[i-AMZTopBlock].LineCount := 1;
      AMZSkipMonthBlock: ScreenFileHeader[i-AMZTopBlock].LineCount := 1;
    end;
  end;
  FileStream.Write( ScreenFileHeader, sizeof(TGridHeader)*8 );
  // AMZTopBlock
  FileStream.Write( AMZ.amountstatus, 1 );
  ShortReal := AMZ.amount;
  FileStream.Write( ShortReal, 6 );
  FileStream.Write( AMZ.loandatestatus, 1 );
  FileStream.Write( AMZ.loandate, sizeof(daterec) );
  FileStream.Write( AMZ.loanratestatus, 1 );
  ShortReal := AMZ.loanrate;
  FileStream.Write( ShortReal, 6 );
  FileStream.Write( AMZ.firststatus, 1 );
  FileStream.Write( AMZ.firstdate, sizeof(daterec) );
  FileStream.Write( AMZ.nstatus, 1 );
  FileStream.Write( AMZ.nperiods, 2 );
  FileStream.Write( AMZ.laststatus, 1 );
  FileStream.Write( AMZ.lastdate, sizeof(daterec) );
  FileStream.Write( AMZ.peryrstatus, 1 );
  FileStream.Write( AMZ.peryr, 2 );
  FileStream.Write( AMZ.payamtstatus, 1 );
  ShortReal := AMZ.payamt;
  FileStream.Write( ShortReal, 6 );
  FileStream.Write( AMZ.pointsstatus, 1 );
  ShortReal := AMZ.points;
  FileStream.Write( ShortReal, 6 );
  FileStream.Write( AMZ.aprstatus, 1 );
  ShortReal := AMZ.apr;
  FileStream.Write( ShortReal, 6 );
  FileStream.Write( AMZ.lastok, 1 );
  // AMZBalanceBlock
  FileStream.Write( Payoff.datestatus, 1 );
  FileStream.Write( Payoff.date, sizeof(daterec) );
  FileStream.Write( Payoff.amountstatus, 1 );
  ShortReal := Payoff.amount;
  FileStream.Write( ShortReal, 6 );
  // AMZPreBlock
  for i:=1 to PrepayCount do begin
    FileStream.Write( Prepayment[i].startdatestatus, 1 );
    FileStream.Write( Prepayment[i].startdate, sizeof(daterec) );
    FileStream.Write( Prepayment[i].nnstatus, 1 );
    FileStream.Write( Prepayment[i].nn, 2 );
    FileStream.Write( Prepayment[i].stopdatestatus, 1 );
    FileStream.Write( Prepayment[i].stopdate, sizeof(daterec) );
    FileStream.Write( Prepayment[i].peryrstatus, 1 );
    FileStream.Write( Prepayment[i].peryr, 2 );
    FileStream.Write( Prepayment[i].paymentstatus, 1 );
    ShortReal := Prepayment[i].payment;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( Prepayment[i].nextdate, sizeof(daterec) );
  end;
  // AMZBalloonBlock
  for i:=1 to BalloonCount do begin
    FileStream.Write( Balloon[i].datestatus, 1 );
    FileStream.Write( Balloon[i].date, sizeof(daterec) );
    FileStream.Write( Balloon[i].amountstatus, 1 );
    ShortReal := Balloon[i].amount;
    FileStream.Write( ShortReal, 6 );
  end;
  // AMZRateChangeBlock
  for i:=1 to ADJCount do begin
    FileStream.Write( ADJ[i].datestatus, 1 );
    FileStream.Write( ADJ[i].date, sizeof(daterec) );
    FileStream.Write( ADJ[i].loanratestatus, 1 );
    ShortReal := ADJ[i].loanrate;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( ADJ[i].amountstatus, 1 );
    ShortReal := ADJ[i].amount;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( ADJ[i].amtok, 1 );
  end;
  // AMZMoratoriumBlock
  FileStream.Write( Mor.first_repaystatus, 1 );
  FileStream.Write( Mor.first_repay, sizeof(daterec) );
  // AMZTargetBlock
  FileStream.Write( Target.targetstatus, 1 );
  ShortReal := Target.target;
  FileStream.Write( ShortReal, 6 );
  // AMZSkipMonthBlock
  FileStream.Write( Skip.skipstatus, 1 );
  FileStream.Write( Skip.skipmonths, 15 );
  // end tag
  ch := #254;
  FileStream.Write( ch, 1 );
  FileStream.Free();
  SaveAmortization := true;
end;

{ SavePresentValue -- serialize the present-value screen to a .PVL file.
  Params:  FileName plus the lump-sum, periodic, presval, rateline and xpres data.
  Header:  fancy byte = byte(pvlfancy); builds the 8-slot grid header for the PVL
           blocks PVLlumpsumblock..PVLExtraBlock with per-block LineCount.  The
           third block (PVLpresvalblock) is counted from the rateline array when
           pvlfancy, else from the presval array; PVLExtraBlock gets LineCount 1
           only when pvlfancy.
  Body:    writes lump-sum, periodic, then the polymorphic third block (rateline
           if pvlfancy else presval), then the xpresval block when pvlfancy, then
           the #254 sentinel. Returns true on success. }
function TFileIO.SavePresentValue( FileName: string; Lump: LumpSumArray; ThePeriodic: PeriodicArray;
                                   Pres: PresValArray; TheRateLine: Ratelinearray; XPres: xpresvalptr ): boolean;
var
  FileStream: TFileStream;
  ScreenFileHeader: array [0..7] of TGridHeader;
  i, j: integer;
  ShortReal: real48;
  ch: char;
  LumpSumCount, PeriodicCount, Block3Count: integer;
begin
  SavePresentValue := false;
  FileStream := TFileStream.Create( FileName, fmOpenWrite );
  if( FileStream = nil ) then exit;
  m_FancyByte := byte(pvlfancy);
  if( not SaveCommon( FileStream ) ) then exit;
  for i:=PVLlumpsumblock to PVLExtraBlock do begin
    ScreenFileHeader[i-PVLlumpsumblock].GridID := i;
    ScreenFileHeader[i-PVLlumpsumblock].ScrollPosition := 0;
    case i of
      PVLlumpsumblock: begin
        LumpSumCount := 0;
        for j:=1 to maxlines do begin
          if( not LumpSumIsEmpty(lumpsumptr(Lump[j])) ) then
            LumpSumCount := j;
        end;
        ScreenFileHeader[i-PVLlumpsumblock].LineCount := LumpSumCount;
      end;
      PVLperiodicblock: begin
        PeriodicCount := 0;
        for j:=1 to maxlines do begin
          if( not PeriodicIsEmpty(periodicptr(ThePeriodic[j])) ) then
            PeriodicCount := j;
        end;
        ScreenFileHeader[i-PVLlumpsumblock].LineCount := PeriodicCount;
      end;
      PVLpresvalblock: begin
        Block3Count := 0;
        if( pvlfancy ) then begin
          for j:=1 to maxlines do begin
            if( not RatelineIsEmpty(ratelineptr(TheRateLine[j])) ) then
              Block3Count := j;
          end;
        end else begin
          for j:=1 to presvallines do begin
            if( not PresvalIsEmpty(presvalptr(Pres[j])) ) then
              Block3Count := j;
          end;
        end;
        ScreenFileHeader[i-PVLlumpsumblock].LineCount := Block3Count;
      end;
      PVLExtraBlock: begin
        if( pvlfancy ) then
          ScreenFileHeader[i-PVLlumpsumblock].LineCount := 1
        else
          ScreenFileHeader[i-PVLlumpsumblock].LineCount := 0;
      end;
    end;
  end;
  FileStream.Write( ScreenFileHeader, sizeof(TGridHeader)*8 );
  // lump sum block
  for i:=1 to LumpSumCount do begin
    FileStream.Write( Lump[i].datestatus, 1 );
    FileStream.Write( Lump[i].date, sizeof(daterec) );
    FileStream.Write( Lump[i].amt0status, 1 );
    ShortReal := Lump[i].amt0;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( Lump[i].val0status, 1 );
    ShortReal := Lump[i].val0;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( Lump[i].status, 2 );
    FileStream.Write( Lump[i].act0, 1 );
  end;
  // Periodic block
  for i:=1 to PeriodicCount do begin
    FileStream.Write( ThePeriodic[i].fromdatestatus, 1 );
    FileStream.Write( ThePeriodic[i].fromdate, sizeof(daterec) );
    FileStream.Write( ThePeriodic[i].todatestatus, 1 );
    FileStream.Write( ThePeriodic[i].todate, sizeof(daterec) );
    FileStream.Write( ThePeriodic[i].peryrstatus, 1 );
    FileStream.Write( ThePeriodic[i].peryr, 2 );
    FileStream.Write( ThePeriodic[i].amtnstatus, 1 );
    ShortReal := ThePeriodic[i].amtn;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( ThePeriodic[i].colastatus, 1 );
    ShortReal := ThePeriodic[i].cola;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( ThePeriodic[i].valnstatus, 1 );
    ShortReal := ThePeriodic[i].valn;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( ThePeriodic[i].status, 2 );
    FileStream.Write( ThePeriodic[i].ninstallments, 2 );
    FileStream.Write( ThePeriodic[i].actn, 1 );
  end;
  // third block, either rateline or presval
  for i:=1 to Block3Count do begin
    if( pvlfancy ) then begin
      FileStream.Write( TheRateLine[i].datestatus, 1 );
      FileStream.Write( TheRateLine[i].date, sizeof(daterec) );
      FileStream.Write( TheRateLine[i].r.status, 1 );
      ShortReal := TheRateLine[i].r.rate;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( TheRateLine[i].r.peryr, 1 );
      FileStream.Write( TheRateLine[i].status, 2 );
    end else begin
      FileStream.Write( Pres[i].asofstatus, 1 );
      FileStream.Write( Pres[i].asof, sizeof(daterec) );
      FileStream.Write( Pres[i].r.status, 1 );
      ShortReal := Pres[i].r.rate;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Pres[i].r.peryr, 1 );
      FileStream.Write( Pres[i].sumvaluestatus, 1 );
      ShortReal := Pres[i].sumvalue;
      FileStream.Write( ShortReal, 6 );
      FileStream.Write( Pres[i].status, 2 );
      FileStream.Write( Pres[i].durationstatus, 1 );
      ShortReal := Pres[i].duration;
      FileStream.Write( ShortReal, 6 );
    end;
  end;
  // the xpresval block if it exists
  if( pvlfancy ) then begin
    FileStream.Write( XPres.xasofstatus, 1 );
    FileStream.Write( XPres.xasof, sizeof(daterec) );
    FileStream.Write( XPres.simplestatus, 1 );
    FileStream.Write( XPres.simple, 1 );
    FileStream.Write( XPres.xvaluestatus, 1 );
    ShortReal := XPres.xvalue;
    FileStream.Write( ShortReal, 6 );
    FileStream.Write( XPres.status, 1 );
  end;  
  // end tag
  ch := #254;
  FileStream.Write( ch, 1 );
  FileStream.Free();
  SavePresentValue := true;
end;

end.
