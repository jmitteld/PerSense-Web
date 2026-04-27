unit FileIOUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  ExtCtrls, Globals, peTypes;

const
  { versions from DOS Persense }
  version11=1 shl 8 + 5; {Version 1.00f changed default structure}
  version20=2 shl 8;
  version30=3 shl 8;
  version22=2 shl 8 + 2;
  version31=3 shl 8 + 1;

type
  basistype = (x365,x360,x365_360);

  TGridHeader = record
    GridID : byte;
    ScrollPosition : byte;
    LineCount : byte;
  end;
  GridHeaderArray = array[0..7] of TGridHeader;

  // a generalized class used to store and load all the file
  // types from a file.
  TFileIO = class
  public
    constructor Create();
    destructor Destroy();
    function LoadFile( Name: string ) : boolean;
    function GetVersion() : word;
    function GetIDByte() : byte;
    function GetFileName(): string;
    function GetFancyByte(): byte;
    function GetMortgageArray( var Mortgages: mtgarray ): boolean;
    function GetAmortizationData( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                                  var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                                  Target: targetptr; Skip: skipptr ): boolean;
    function GetPresentValueData( var Lump: LumpSumArray; var ThePeriodic: PeriodicArray;
                                  var Pres: PresValArray; var TheRateLine: Ratelinearray;
                                  XPres: xpresvalptr ): boolean;
    function SaveMortgage( FileName: string; Mortgages: mtgarray ): boolean;
    function SaveAmortization( FileName: string; AMZ:AMZPtr; Payoff: balloonptr;
                               Prepayment: prepaymentarray; Balloon: balloonarray;
                               ADJ: adjarray; Mor: Moratoriumptr; Target: targetptr;
                               Skip: skipptr ) : boolean;
    function SavePresentValue( FileName: string; Lump: LumpSumArray; ThePeriodic: PeriodicArray;
                               Pres: PresValArray; TheRateLine: Ratelinearray; XPres: xpresvalptr ): boolean;
  private
    m_Version: word;
    m_IDByte: byte;
    m_FancyByte: byte;
    m_FileName: string;
    // for mortgages
    m_Mortgages: mtgarray;
    // for amortization
    m_AMZ: AMZLoan;
    m_Payoff: balloonrec;
    m_Prepayment: prepaymentarray;
    m_Balloon: balloonarray;
    m_ADJ: adjarray;
    m_Mor: Moratoriumrec;
    m_Target: targetrec;
    m_Skip: skiprec;
    // for Present Value
    m_LumpSum: lumpsumarray;
    m_Periodic: periodicarray;
    m_PresVal: presvalarray;
    m_RateLine: ratelinearray;
    m_XPresVal: xpresval;
    function LoadMortgageData( LineCount: integer; FileStream: TFileStream ): boolean;
    function LoadAmortizationData( GridArray: GridHeaderArray; FileStream: TFileStream ): boolean;
    function LoadPresentValueData( GridArray: GridHeaderArray; FileStream: TFileStream ): boolean;
    function SaveCommon( FileStream: TFileStream ): boolean;
  end;

implementation

uses Mortgage, peData, Amortize, Presvalu, HelpSystemUnit;

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

function TFileIO.GetIDByte() : byte;
begin
  GetIDByte := m_IDByte;
end;

function TFileIO.GetVersion() : word;
begin
  GetVersion := m_Version;
end;

function TFileIO.GetFileName(): string;
begin
  GetFileName := m_FileName;
end;

function TFileIO.GetFancyByte(): byte;
begin
  GetFancyByte := m_FancyByte;
end;

function TFileIO.GetMortgageArray( var Mortgages: mtgarray ): boolean;
var
  i: integer;
begin
  for i:=1 to maxlines do
    Mortgages[i]^ := m_Mortgages[i]^;
  GetMortgageArray := true;
end;

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
  BytesRead := FileStream.Read( m_Version, 2 );
  BytesRead := FileStream.Read( Identifier, 1 );
  if( (Identifier<>'%') or (BytesRead<>1) ) then begin
    MessageBox( 'Invalid file', DO_InvalidFile );
    FileStream.Free();
    LoadFile := false;
    exit;
  end;
  BytesRead := FileStream.Read( m_FancyByte, 1 );
  BytesRead := FileStream.Read( df.c, sizeof(CompDefaults) );
  BytesRead := FileStream.Read( ScreenFileHeader, sizeof(TGridHeader)*8 );
  RetVal := false;
  m_IDByte := ScreenFileHeader[0].GridID;
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

function TFileIO.LoadMortgageData( LineCount: integer; FileStream: TFileStream ): boolean;
var
  i: integer;
  ShortReal: real48;
begin
  for i:=1 to LineCount do begin
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
    FileStream.Seek( 1, soFromCurrent );
  end;
  LoadMortgageData := true;
end;

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
        if( m_FancyByte>0 ) then begin
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

function TFileIO.SaveMortgage( FileName: string; Mortgages: mtgarray ): boolean;
var
  FileStream: TFileStream;
  ScreenFileHeader: array [0..7] of TGridHeader;
  i: integer;
  ShortReal: real48;
  ch: char;
  SeventyTwo: byte;
  Count: integer;
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
  FileStream.Write( ch, 1 );
  FileStream.Free();
end;

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
