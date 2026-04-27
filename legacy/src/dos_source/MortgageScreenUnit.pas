unit MortgageScreenUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, ChildWin, StdCtrls, Grids, PersenseGrid, peTypes, peData,
  MortgageUndoBufferUnit, Globals, LogUnit, Menus, FileIOUnit;

const
  HeaderBoxWidthPercent         = 0.83;
  PricePos                      = 0.04;
  PointsPos                     = 0.15;
  PercentDownPos                = 0.20;
  CashRequiredPos               = 0.25;
  AmountBorrowedPos             = 0.37;
  YearsPos                      = 0.49;
  LoanRatePos                   = 0.53;
  MonthlyTaxPos                 = 0.62;
  MonthlyTotalPos               = 0.72;
  BalloonYearsPos               = 0.83;
  BalloonAmountPos              = 0.87;
  PriceCol                      = 1;
  PointsCol                     = 2;
  PercentDownCol                = 3;
  CashRequiredCol               = 4;
  AmountBorrowedCol             = 5;
  YearsCol                      = 6;
  LoanRateCol                   = 7;
  MonthlyTaxCol                 = 8;
  MonthlyTotalCol               = 9;
  BalloonYearsCol               = 10;
  BalloonAmountCol              = 11;
  WA_ACTIVATE                   = 1;

type
  TMortgageScreen = class(TMDIChild)
    MortgageGrid: TPersenseGrid;
    HeaderBox: TGroupBox;
    PriceLabel: TLabel;
    PointsLabel: TLabel;
    PercentLabel: TLabel;
    CashLabel: TLabel;
    AmountLabel: TLabel;
    YearsLabel: TLabel;
    LoanLabel: TLabel;
    MonthlyLabel: TLabel;
    TotalLabel: TLabel;
    BalloonBox: TGroupBox;
    BalloonYearsLabel: TLabel;
    BalloonAmountLabel: TLabel;
    SaveDialog1: TSaveDialog;
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
    procedure FormResize(Sender: TObject);
    procedure MortgageGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean );
    procedure MortgageGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure MortgageGridEditCopy(Sender: TObject);
    procedure MortgageGridEditCut(Sender: TObject);
    procedure MortgageGridEditPaste(Sender: TObject);
    procedure MortgageGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);
    procedure MortgageGridCellBeforeEdit(Sender: TObject; ACol, ARow: Integer; const Value: String);
    procedure MortgageGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
  private
    { Private declarations }
  public
    constructor Create(AOwner: TComponent); override;
    destructor Destroy(); override;
    function GetType(): TScreenType; override;
    procedure OnCalculate(); override;
    procedure OnHardenValue(); override;
    procedure OnUndo(); override;
    procedure OnRedo(); override;
    procedure OnCut(); override;
    procedure OnCopy(); override;
    procedure OnPaste(); override;
    procedure OnDelete(); override;
    procedure OpenFile( var TheFile: TFileIO ); override;
    function SaveFile( FileName, ScreenName: string ): boolean; override;
    procedure OnPrint(); override;
    procedure OnContextualHelp(); override;
    procedure SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont ); override;
    procedure CompareAPRs();
    procedure GenerateMtgRows();
    procedure BackupForHelpSystem();
    procedure RestoreFromHelpSystem();
    function IsBackedUpForHelp(): boolean;
  protected
    m_UndoBuffer : TMortgageUndoBuffer;
    m_UndoRedoHolder: mtgRecList;
    m_HelpBackup: mtgarray;
    m_bBackedUpForHelp: boolean;
    m_BackupFileName: string;
    m_BackupCaption: string;
    m_bBackupUnsaved: boolean;
    procedure AssignMortgageValues( ACol, ARow: Integer; const Value: String );
    procedure AssignMortgageValuesEx( ACol, ARow: Integer; const Value: String; const Hardness: InOut );
    procedure DoCalculation( Row: integer );
    procedure TMortgage2Grid( Mortgage: mtgptr; Grid: TPersenseGrid; Row: integer );
    procedure MoveRow( Source, Destination: integer );
    procedure CopyRowWithIncrement( Source, Destination, Column: integer; Increment: double );
  end;

var
  MortgageScreen: TMortgageScreen;

implementation

{$R *.dfm}
uses MortgageRowGenerationDlgUnit, INTSUTIL, Mortgage, MAIN,
     Printers, APRComparisonDLGUnit, SelectAPRDlgUnit, HelpSystemUnit;

constructor TMortgageScreen.Create(AOwner: TComponent);
var
  i: integer;
  CanSelect: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.Create begin' );
  inherited Create(AOwner);
  m_bBackupUnsaved := false;
  MortgageGrid.RowCount := maxlines;
  MortgageGrid.IsExpandable := false;
  for i:=1 to maxlines do begin
    GetMem( e[i], sizeof(mtgline) );
    ZeroMortgage( mtgptr(e[i]) );
    GetMem( m_UndoRedoHolder[i], sizeof(mtgline) );
    ZeroMortgage( mtgptr(m_UndoRedoHolder[i]) );
    GetMem( m_HelpBackup[i], sizeof(mtgline) );
    ZeroMortgage( mtgptr(m_HelpBackup[i]) );
    MortgageGrid.Cells[0,i-1] := IntToStr( i-1 );
  end;
  m_bBackedUpForHelp := false;
  { set up the columns }
  MortgageGrid.SetupColumn( 0, 0.0, ColTypeNone, 0 );
  MortgageGrid.SetupColumn( PriceCol, PricePos, ColTypeDollar, 15 );
  MortgageGrid.SetupColumn( PointsCol, PointsPos, ColType2Real, 7 );
  MortgageGrid.SetupColumn( PercentDownCol, PercentDownPos, ColType2real, 7 );
  MortgageGrid.SetupColumn( CashRequiredCol, CashRequiredPos, ColTypeDollar, 14 );
  MortgageGrid.SetupColumn( AmountBorrowedCol, AmountBorrowedPos, ColTypeDollar, 14 );
  MortgageGrid.SetupColumn( YearsCol, YearsPos, ColTypeInt, 2 );
  MortgageGrid.SetupColumn( LoanRateCol, LoanRatePos, ColType4Real, 12 );
  MortgageGrid.SetupColumn( MonthlyTaxCol, MonthlyTaxPos, ColTypeDollar, 13 );
  MortgageGrid.SetupColumn( MonthlyTotalCol, MonthlyTotalPos, ColTypeDollar, 14 );
  MortgageGrid.SetupColumn( BalloonYearsCol, BalloonYearsPos, ColTypeInt, 2 );
  MortgageGrid.SetupColumn( BalloonAmountCol, BalloonAmountPos, ColTypeDollar, 11 );
  m_UndoBuffer := TMortgageUndoBuffer.Create();
  Width := 652;
  Visible := false;
  // must do this to get the status text going
  MortgageGridSelectCell( Self, 1, 0, CanSelect );
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.Create end' );
end;

destructor TMortgageScreen.Destroy();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.Destroy begin' );
  for i:=1 to maxlines do begin
    FreeMem( e[i] );
    FreeMem( m_UndoRedoHolder[i] );
  end;
  m_UndoBuffer.Free();
  inherited Destroy;
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.Destroy end' );
end;

function TMortgageScreen.GetType(): TScreenType;
begin
  GetType := MortgageType;
end;

procedure TMortgageScreen.FormClose(Sender: TObject; var Action: TCloseAction);
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.FormClose' );
  OnFormClose( Action );
end;

procedure TMortgageScreen.FormResize(Sender: TObject);
begin
  { Size and position the Header Box }
  HeaderBox.Top := 0;
  HeaderBox.Left := 0;
  HeaderBox.Width := Trunc( Width * HeaderBoxWidthPercent );
  { Size and position the Balloon Header Box }
  BalloonBox.Top := 0;
  BalloonBox.Left := HeaderBox.Width;
  BalloonBox.Width := Width - HeaderBox.Width-8;
  { Position the Header Box Labels }
  PriceLabel.Left := Trunc( Width * PricePos )+10;
  PointsLabel.Left := Trunc( Width * PointsPos );
  PercentLabel.Left := Trunc( Width * PercentDownPos );
  CashLabel.Left := Trunc( Width * CashRequiredPos )+10;
  AmountLabel.Left := Trunc( Width * AmountBorrowedPos )+10;
  YearsLabel.Left := Trunc( Width * YearsPos );
  LoanLabel.Left := Trunc( Width * LoanRatePos )+10;
  MonthlyLabel.Left := Trunc( Width * MonthlyTaxPos )+10;
  TotalLabel.Left := Trunc( Width * MonthlyTotalPos )+10;
  { Position the Balloon Header Box Labels }
  BalloonYearsLabel.Left := Trunc( BalloonBox.Width * (BalloonYearsPos-HeaderBoxWidthPercent) )+10;
  BalloonAmountLabel.Left := Trunc( BalloonBox.Width * (BalloonAmountPos-HeaderBoxWidthPercent) )+30;
  { Size Grid }
  MortgageGrid.Left := 0;
  MortgageGrid.Top := HeaderBox.Height;
  MortgageGrid.Width := Width - 8;
  MortgageGrid.Height := Height - HeaderBox.Height - 30;
end;

{ Store the value just entered in the m_Mortgages array.  If the value
  was zeroed out then reset it in the m_Mortgages array }
procedure TMortgageScreen.MortgageGridCellAfterEdit(Sender: TObject; ACol,
  ARow: Integer; const Value: String; DataChanged: boolean );
begin
  SetUnsavedData( true );
  { balloon filling has effects on things }
  if( (ACol = BalloonYearsCol) and (e[ARow+1].HowMuchStatus=empty) and
      (e[ARow+1].MonthlyStatus<>inp) ) then begin
    MortgageGrid.Cells[MonthlyTotalCol,ARow] := '';
    AssignMortgageValues( MonthlyTotalCol, ARow, '' );
  end;
  AssignMortgageValues( ACol, ARow, Value );
  if( (ACol = LoanRateCol) and (e[ARow+1].Rate>0.19835162342) ) then begin
    MessageBox( 'You entered an unusually high Loan Rate.', DA_UnusuallyHighRate );
  end;
  if( DataChanged ) then
    // store for undo.
    m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
end;

procedure TMortgageScreen.MortgageGridCellBeforeEdit(Sender: TObject; ACol,
  ARow: Integer; const Value: String);
begin
  { Only one of %Down, Cash Required, and Amount Borrowed can be filled in }
  if( ACol = PercentDownCol ) then begin;
    MortgageGrid.Cells[CashRequiredCol,ARow] := '';
    MortgageGrid.Cells[AmountBorrowedCol,ARow] := '';
    AssignMortgageValues( CashRequiredCol, ARow, '' );
    AssignMortgageValues( AmountBorrowedCol, ARow, '' );
  end else if( ACol = CashRequiredCol ) then begin;
    MortgageGrid.Cells[PercentDownCol,ARow] := '';
    MortgageGrid.Cells[AmountBorrowedCol,ARow] := '';
    AssignMortgageValues( PercentDownCol, ARow, '' );
    AssignMortgageValues( AmountBorrowedCol, ARow, '' );
  end else if( ACol = AmountBorrowedCol ) then begin;
    MortgageGrid.Cells[PercentDownCol,ARow] := '';
    MortgageGrid.Cells[CashRequiredCol,ARow] := '';
    AssignMortgageValues( PercentDownCol, ARow, '' );
    AssignMortgageValues( CashRequiredCol, ARow, '' );
  end;
end;

procedure TMortgageScreen.AssignMortgageValues( ACol, ARow: Integer; const Value: String );
begin
  AssignMortgageValuesEx( ACol, ARow, Value, inp );
end;

{ useful function for assigning a string value at a given row and column
  to the underlying TMortgage object }
procedure TMortgageScreen.AssignMortgageValuesEx( ACol, ARow: Integer; const Value: String; const Hardness: InOut );
var
  MonthlyRate: double;
  IsError: Boolean;
begin;
  SetUnsavedData( true );
  case ACol of
    PriceCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Price := 0;
        e[ARow+1].PriceStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Price := StringFormat2Double( Value, IsError );
        e[ARow+1].PriceStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    PointsCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Points := 0;
        e[ARow+1].PointsStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Points := StringFormat2Double( Value, IsError )/100.0;
        e[ARow+1].PointsStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    PercentDownCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Pct := 0;
        e[ARow+1].PctStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Pct := StringFormat2Double( Value, IsError )/100.0;
        e[ARow+1].PctStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    CashRequiredCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Cash := 0;
        e[ARow+1].CashStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Cash := StringFormat2Double( Value, IsError );
        e[ARow+1].CashStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    AmountBorrowedCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Financed := 0;
        e[ARow+1].FinancedStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Financed := StringFormat2Double( Value, IsError );
        e[ARow+1].FinancedStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    YearsCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Years := 0;
        e[ARow+1].YearsStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Years := StringFormat2Int( Value, IsError );
        e[ARow+1].YearsStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    LoanRateCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Rate := 0;
        e[ARow+1].RateStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        MonthlyRate := StringFormat2Double( Value, IsError )/100;
        e[ARow+1].Rate := RateFromYield( MonthlyRate, 12 );
        e[ARow+1].RateStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    MonthlyTaxCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Tax := 0;
        e[ARow+1].TaxStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Tax := StringFormat2Double( Value, IsError );
        e[ARow+1].TaxStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    MonthlyTotalCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].Monthly := 0;
        e[ARow+1].MonthlyStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].Monthly := StringFormat2Double( Value, IsError );
        e[ARow+1].MonthlyStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    BalloonYearsCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].When := 0;
        e[ARow+1].WhenStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].When := StringFormat2Int( Value, IsError );
        e[ARow+1].WhenStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    BalloonAmountCol: begin;
      if( Value = '' ) then begin
        e[ARow+1].HowMuch := 0;
        e[ARow+1].HowMuchStatus := empty;
        MortgageGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        e[ARow+1].HowMuch := StringFormat2Double( Value, IsError );
        e[ARow+1].HowMuchStatus := Hardness;
        MortgageGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
  end;
  if( IsError ) then
    MasterLog.Write( LVL_MED, 'AssignMortgageValueEx IsError set to true' );
end;

{ The big one.  Read in all the fields, store them, and calculate stuff
  then output the answers back to the MortgageGrid. }
procedure TMortgageScreen.OnCalculate();
var
  i: integer;
begin;
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnCalculate' );
  i := MortgageGrid.Row;
  { This next part triggers an OnAfterCellEdit event if
    the EditorMode is set to true }
  if( MortgageGrid.EditorMode ) then
    MortgageGrid.EditorMode := false;
  DoCalculation( i );
  { Save Current Set for possible undo }
  m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
end;

{ does the actual calculation }
procedure TMortgageScreen.DoCalculation( Row: integer );
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.DoCalculation' );
  SetUnsavedData( true );
  nlines[MTGBlock] := MortgageGrid.GetUsedRowCount();
  CalculateRows( Row+1, Row+1 );
  { Now output whatever came out }
  TMortgage2Grid( mtgptr(e[Row+1]), MortgageGrid, Row );
end;

{ handy function to copy the contents of a TMortgage to a PersenseGrid }
procedure TMortgageScreen.TMortgage2Grid( Mortgage: mtgptr; Grid: TPersenseGrid; Row: integer );
var
  NumStr: string;
  Holder: double;
begin
  if( (Row+1 > Grid.GetUsedRowCount()) and MortgageIsEmpty( Mortgage ) ) then exit;
  if( Mortgage.PriceStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Price );
    MortgageGrid.SetCell( NumStr, PriceCol, Row, Mortgage.PriceStatus );
  end else
    MortgageGrid.SetCell( '', PriceCol, Row, empty );

  if( Mortgage.PointsStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Points * 100 );
    MortgageGrid.SetCell( NumStr, PointsCol, Row, Mortgage.PointsStatus );
  end else
    MortgageGrid.SetCell( '', PointsCol, Row, empty );

  if( Mortgage.PctStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Pct * 100 );
    MortgageGrid.SetCell( NumStr, PercentDownCol, Row, Mortgage.PctStatus );
  end else
    MortgageGrid.SetCell( '', PercentDownCol, Row, empty );

  if( Mortgage.CashStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Cash );
    MortgageGrid.SetCell( NumStr, CashRequiredCol, Row, Mortgage.CashStatus );
  end else
    MortgageGrid.SetCell( '', CashRequiredCol, Row, empty );

  if( Mortgage.FinancedStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Financed );
    MortgageGrid.SetCell( NumStr, AmountBorrowedCol, Row, Mortgage.FinancedStatus );
  end else
    MortgageGrid.SetCell( '', AmountBorrowedCol, Row, empty );

  if( Mortgage.YearsStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Years );
    MortgageGrid.SetCell( NumStr, YearsCol, Row, Mortgage.YearsStatus );
  end else
    MortgageGrid.SetCell( '', YearsCol, Row, empty );

  if( Mortgage.RateStatus <> empty ) then begin;
    Holder := YieldFromRate( Mortgage.Rate, 12 );
    NumStr := FloatToStr( Holder*100 );
    MortgageGrid.SetCell( NumStr, LoanRateCol, Row, Mortgage.RateStatus );
  end else
    MortgageGrid.SetCell( '', LoanRateCol, Row, empty );

  if( Mortgage.TaxStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Tax );
    MortgageGrid.SetCell( NumStr, MonthlyTaxCol, Row, Mortgage.TaxStatus );
  end else
    MortgageGrid.SetCell( '', MonthlyTaxCol, Row, empty );

  if( Mortgage.MonthlyStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.Monthly );
    MortgageGrid.SetCell( NumStr, MonthlyTotalCol, Row, Mortgage.MonthlyStatus );
  end else
    MortgageGrid.SetCell( '', MonthlyTotalCol, Row, empty );

  if( Mortgage.WhenStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.When );
    MortgageGrid.SetCell( NumStr, BalloonYearsCol, Row, Mortgage.WhenStatus );
  end else
    MortgageGrid.SetCell( '', BalloonYearsCol, Row, empty );

  if( Mortgage.HowMuchStatus <> empty ) then begin;
    NumStr := FloatToStr( Mortgage.HowMuch );
    MortgageGrid.SetCell( NumStr, BalloonAmountCol, Row, Mortgage.HowMuchStatus );
  end else
    MortgageGrid.SetCell( '', BalloonAmountCol, Row, empty );
  SetUnsavedData( true );
end;

procedure TMortgageScreen.MortgageGridEditEnterKeyPressed(Sender: TObject;
  ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
begin
  SetUnsavedData( true );
  DoCalculation( ARow );
  if( errorflag ) then
    MasterLog.Write( LVL_MED, 'MortgageGridEditEnterKeyPressed Row DoCalculation failed' );
  // handles entry grid cell movement
  if( ACol = PercentDownCol ) then begin
    if( (e[ARow+1].CashStatus = outp) and (e[ARow+1].FinancedStatus=outp) ) then begin
      MortgageGrid.Col := MortgageGrid.Col + 3;
      DefaultAction := false;
    end;
  end else if( ACol = CashRequiredCol ) then begin
    if( (e[ARow+1].PctStatus = outp) and (e[ARow+1].FinancedStatus=outp) ) then begin
      MortgageGrid.Col := MortgageGrid.Col + 2;
      DefaultAction := false;
    end;
  end;
  // since I know this is called after MortgageGridCellAfterEdit then I will use
  // the OverWrite form so that the calculation will be part of the last cell edit
  m_UndoBuffer.OverWriteData( e, MortgageGrid.Selection );
end;

procedure TMortgageScreen.OnUndo();
var
  i : integer;
  Success : boolean;
  Selection: TGridRect;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnUndo' );
  m_UndoBuffer.Undo( m_UndoRedoHolder, Selection, Success );
  if( not Success ) then begin
    MasterLog.Write( LVL_LOW, 'Undo1Click failed to Undo' );
    exit;
  end;
  SetUnsavedData( true );
  for i:=1 to maxlines do
    ZeroMortgage( mtgptr(e[i]) );
  for i:=1 to maxlines do
    e[i]^ := m_UndoRedoHolder[i]^;
  for i:=1 to maxlines do
    TMortgage2Grid( mtgptr(e[i]), MortgageGrid, i-1 );
  MortgageGrid.Selection := Selection;
end;

procedure TMortgageScreen.OnRedo();
var
  i : integer;
  Success : boolean;
  Selection: TGridRect;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnRedo' );
  m_UndoBuffer.Redo( m_UndoRedoHolder, Selection, Success );
  if( not Success ) then begin
    MasterLog.Write( LVL_LOW, 'Redo1Click failed to Undo' );
    exit;
  end;
  SetUnsavedData( true );
  for i:=1 to maxlines do
    ZeroMortgage( mtgptr(e[i]) );
  for i:=1 to maxlines do
    e[i]^ := m_UndoRedoHolder[i]^;
  for i:=1 to maxlines do
    TMortgage2Grid( mtgptr(e[i]), MortgageGrid, i-1 );
  MortgageGrid.Selection := Selection;
end;

{ response to Harden option in menu }
procedure TMortgageScreen.OnHardenValue();
var
  ARow, ACol: integer;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnHardenValue' );
  SetUnsavedData( true );
  for ARow:=MortgageGrid.Selection.Top to MortgageGrid.Selection.Bottom do begin
    if( ARow <= MortgageGrid.GetUsedRowCount() ) then begin
      for ACol:=MortgageGrid.Selection.Left to MortgageGrid.Selection.Right do begin
        case ACol of
          PriceCol: e[ARow+1].PriceStatus := inp;
          PointsCol: e[ARow+1].PointsStatus := inp;
          PercentDownCol: begin;
            e[ARow+1].PctStatus := inp;
            e[ARow+1].CashStatus := empty;
            e[ARow+1].FinancedStatus := empty;
          end;
          CashRequiredCol: begin;
            e[ARow+1].PctStatus := empty;
            e[ARow+1].CashStatus := inp;
            e[ARow+1].FinancedStatus := empty;
          end;
          AmountBorrowedCol: begin;
            e[ARow+1].PctStatus := empty;
            e[ARow+1].CashStatus := empty;
            e[ARow+1].FinancedStatus := inp;
          end;
          YearsCol: e[ARow+1].YearsStatus := inp;
          LoanRateCol: e[ARow+1].RateStatus := inp;
          MonthlyTaxCol: e[ARow+1].TaxStatus := inp;
          MonthlyTotalCol: begin;
            e[ARow+1].MonthlyStatus := inp;
          end;
          BalloonYearsCol: e[ARow+1].WhenStatus := inp;
          BalloonAmountCol: e[ARow+1].HowMuchStatus := inp;
        end;
      end;
      TMortgage2Grid( mtgptr(e[ARow+1]), MortgageGrid, ARow );
    end;
  end;
  m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
end;

procedure TMortgageScreen.OnCut();
var
  Col, Row: integer;
  SelectedRect: TGridRect;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnCut' );
  SetUnsavedData( true );
  SelectedRect := MortgageGrid.Selection;
  MortgageGrid.CutToClipboard();
  for Row:=SelectedRect.Top to SelectedRect.Bottom do begin
    for Col:=SelectedRect.Left to SelectedRect.Right do begin
      AssignMortgageValues( Col, Row, '' );
    end;
  end;
  m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
end;

// hook in from PersenseGrid Context menu
procedure TMortgageScreen.MortgageGridEditCut(Sender: TObject);
begin
  OnCut();
end;

procedure TMortgageScreen.OnCopy();
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnCopy' );
  MortgageGrid.CopyToClipboard();
end;

// hook in from PersenseGrid Context menu
procedure TMortgageScreen.MortgageGridEditCopy(Sender: TObject);
begin
  OnCopy();
end;

procedure TMortgageScreen.OnPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnPaste' );
  SetUnsavedData( true );
  MortgageGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignMortgageValuesEx( Col, Row, MortgageGrid.Cells[Col,Row], MortgageGrid.GetCellHardness(Col, Row));
  end;
  m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
end;

// hook in from PersenseGrid Context menu
procedure TMortgageScreen.MortgageGridEditPaste(Sender: TObject);
begin
  OnPaste();
end;

procedure TMortgageScreen.OnDelete();
var
  DelStart, DelEnd: Integer;
  TheText, First, Second: string;
  SelectedRect: TGridRect;
  Col, Row: integer;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnDelete' );
  SetUnsavedData( true );
  if( MortgageGrid.EditorMode ) then begin
    TheText := MortgageGrid.InplaceEditor.Text;
    DelStart := MortgageGrid.InplaceEditor.SelStart;
    DelEnd := DelStart + MortgageGrid.InplaceEditor.SelLength;
    if( DelStart = DelEnd ) then begin
      First := copy( TheText, 0, DelStart );
      Second := copy( TheText, DelStart+2, length(TheText)-DelStart );
    end else begin
      First := copy( TheText, 0, DelStart );
      Second := copy( TheText, DelEnd+1, length(TheText)-(DelEnd-DelStart) );
    end;
    MortgageGrid.InplaceEditor.Text := First + Second;
    MortgageGrid.SetEditText( MortgageGrid.Col, MortgageGrid.Row, MortgageGrid.InplaceEditor.Text );
    MortgageGrid.InplaceEditor.SelStart := DelStart;
  end else begin
    SelectedRect := MortgageGrid.Selection;
    MortgageGrid.DeleteSelected();
    for Row:=SelectedRect.Top to SelectedRect.Bottom do begin
      for Col:=SelectedRect.Left to SelectedRect.Right do begin
        AssignMortgageValues( Col, Row, '' );
      end;
    end;
    m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
  end;
end;

procedure TMortgageScreen.SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont );
begin
  MortgageGrid.CellBackgroundColor := CellColour;
  MortgageGrid.OutpCellBackgroundColor := OutpCellColour;
  MortgageGrid.SelectedCellColor := SelectedColour;
  MortgageGrid.CellFont := CellFont;
  MortgageGrid.OutpCellFont := OutpCellFont;
  FormResize( Self );
  MortgageGrid.Repaint();
end;

// This function is a re-write of the same function in the Mortgage unit.
// The original was too heavily tangled into the old output mechanism.
// I'm trying my best to stick to the form of the old function.
procedure TMortgageScreen.CompareAPRs();
var
  FirstValidRow: byte;
  LastValidRow: byte;
  ValidRowCount: byte;
  e1, e2: mtgline;
  Result1, Result2, FinalResult: string;
  Row1, Row2: integer;
  i, j: integer;
  ValidRows: array of integer;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.CompareAPRs' );
  ValidRowCount := NumberOfValidRowsInArray( e, FirstValidRow, LastValidRow );
  if (errorflag) then exit;
  case ValidRowCount of
    0 : MessageBox('Enter data for one or more mortgages before requesting APR calculation.', DM_1OrMore4APR);
    1 : begin
      e1:=e[FirstValidRow]^;
      ReportAPR(e1);
    end;
    2 : begin
      Calc(FirstValidRow);
      Calc(LastValidRow);
      e1:=e[FirstValidRow]^;
      e2:=e[LastValidRow]^;
      if (EnoughDataForAPR(e1)) and (EnoughDataForAPR(e2)) then begin
        ReportComparisonOfAPRs(FirstValidRow,LastValidRow, Result1, Result2, FinalResult );
        APRComparisonDLG.SetAPR1String( Result1 );
        APRComparisonDLG.SetAPR2String( Result2 );
        APRComparisonDLG.SetResultString( FinalResult );
        APRComparisonDLG.ShowModal();
      end else
        MessageBox('Fill out both lines completely before we can compare these two mortgages.', DA_FillLinesForComparison);
    end;
    else begin
      Row1 := MortgageGrid.Selection.Top;
      if( not EnoughDataForAPR(e[Row1+1]^) ) then begin
        MessageBox( 'The first row you selected doesn''t contain enough data', DM_NotNEoughDataForAPR );
        exit;
      end;
      if( MortgageGrid.Selection.Bottom = Row1+1 ) then begin
        // The user has selected 2 rows specifically.
        Row2 := MortgageGrid.Selection.Bottom;
      end else begin
        // The user has only selected one row.  Let them choose from the other
        // valid rows.  Or they can just see the APR of that one row.
        j := 0;
        for i:=FirstValidRow to LastValidRow do begin
          if( i <> Row1+1 ) then begin
            if( EnoughDataForAPR( e[i]^ ) ) then begin
              inc( j );
              SetLength( ValidRows, j );
              ValidRows[j-1] := i-1;
            end;
          end;
        end;
        SelectAPRDlg.SetInputs( Row1, ValidRows );
        SelectAPRDlg.ShowModal();
        if( SelectAPRDlg.ModalResult = mrOK ) then
          Row2 := SelectAPRDlg.GetSelectedRow
        else begin
          ReportAPR(e[Row1+1]^);
          exit;
        end;
      end;
      // if we get this far then the user has 2 rows that they want to compare
      // so repeate case 2 from above
      Calc(Row1+1);
      Calc(Row2+1);
      e1:=e[Row1+1]^;
      e2:=e[Row2+1]^;
      if (EnoughDataForAPR(e1)) and (EnoughDataForAPR(e2)) then begin
        ReportComparisonOfAPRs( Row1+1, Row2+1, Result1, Result2, FinalResult );
        APRComparisonDLG.SetAPR1String( Result1 );
        APRComparisonDLG.SetAPR2String( Result2 );
        APRComparisonDLG.SetResultString( FinalResult );
        APRComparisonDLG.ShowModal();
      end else
        MessageBox('Fill out both lines completely before we can compare these two mortgages.', DA_FillLinesForComparison);
    end;
  end;
end;

procedure TMortgageScreen.GenerateMtgRows();
var
  Row: integer;
  MaxRowInNeedOfMove: integer;
  InfoArray: array [0..2] of NewMtgRowInfo;
  i: integer;
  NewRowCount: integer;
  CreateNewRows: boolean;
  Set1Index, Set2Index, Set3Index: integer;
  Set1, Set2, Set3: integer;
  Set1Source, Set2Source: integer;
  Source, Destination: integer;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.GenerateMtgRows' );
  Row := MortgageGrid.Selection.Top;
  if( not EnoughDataForRowGeneration( e[Row+1]^ ) ) then begin
    MessageBox( 'Selected Row does not contain enough data.  Make sure row is ready for calculation and then try again', DA_FillRowForComparison );
    exit;
  end;
  CalculateRows( Row+1, Row+1 );
  if( errorflag ) then begin
    MessageBox( 'Selected row has calculate errors.  Can not generate rows from it', DM_CalcErrorsForGenerate );
    MasterLog.Write( LVL_MED, 'GenerateMtgRows1Click: Tried to generate bad row (Calc)' );
    exit;
  end;
  MortgageRowGenerationDlg.Reset();
  MortgageRowGenerationDlg.ShowModal();
  if( MortgageRowGenerationDlg.ModalResult <> mrOK ) then exit;
  MortgageRowGenerationDlg.RetreiveInfo( InfoArray );
  NewRowcount := 1;
  CreateNewRows := false;
  for i:=0 to 2 do begin
    if( InfoArray[i].Column <> -1 ) then begin
      NewRowCount := NewRowCount * (InfoArray[i].Repetition+1);
      CreateNewRows := true;
    end;
  end;
  NewRowCount := NewRowCount-1;
  if( not CreateNewRows ) then exit;
  { If we get this far then we need to do the thing, and NewRowCount contains
    the number of rows we need to create }
  MaxRowInNeedOfMove := Row;
  for i:=Row+1 to maxlines-1 do begin
    if( not MortgageIsEmpty( mtgptr(e[i+1]) ) ) then
      MaxRowInNeedOfMove := i;
  end;
  if( (MaxRowInNeedOfMove+NewRowCount+1) > MortgageGrid.RowCount ) then begin
    MessageBox( 'This operation would create too many rows.', DM_RowCountExceeded );
    exit;
  end;
  { Okay, now we have all the room in the grid allocated, let's move the
    rows that need to be moved, if there are any. }
  SetUnsavedData( true );
  for i:=MaxRowInNeedOfMove downto Row+1 do begin
    if( not MortgageIsEmpty( mtgptr(e[i+1] ) ) ) then
      MoveRow( i, i+NewRowCount );
  end;
  { All rows have been moved out of the way, go ahead and do the
   actual new row creation.  Unfortunately the order is backwards from
    how I'd like it, so we have to do some bizarre index stuff }
  if( InfoArray[2].Column <> -1 ) then begin
    Set1Index := 2;
    Set2Index := 1;
    Set3Index := 0;
  end else if( InfoArray[1].Column <> -1 ) then begin
    Set1Index := 1;
    Set2Index := 0;
    Set3Index := -1;
  end else begin
    Set1Index := 0;
    Set2Index := -1;
    Set3Index := -1;
  end;
  { Okay, all the sets are in order, let's start going through them }
  Source := Row;
  Destination := Source+1;
  for Set1:=0 to InfoArray[Set1Index].Repetition do begin
    if( Set2Index <> -1 ) then begin
      Set1Source := Source;
      for Set2:=0 to InfoArray[Set2Index].Repetition do begin
        if( Set3Index <> -1 ) then begin
          Set2Source := Source;
          for Set3:=1 to InfoArray[Set3Index].Repetition do begin
            CopyRowWithIncrement( Source, Destination, InfoArray[Set3Index].Column, InfoArray[Set3Index].Amount );
            if( errorflag ) then begin
              MessageBox( 'Error with generated row, aborting row generation', DM_GeneratedRowError );
              ZeroMortgage( mtgptr(e[Destination+1]) );
              TMortgage2Grid( mtgptr(e[Destination+1]), MortgageGrid, Destination );
              exit;
            end;
            Source := Destination;
            Inc( Destination );
          end;
          Source := Set2Source;
        end;
        if( Set2 <> InfoArray[Set2Index].Repetition ) then begin
          CopyRowWithIncrement( Source, Destination, InfoArray[Set2Index].Column, InfoArray[Set2Index].Amount );
          if( errorflag ) then begin
            MessageBox( 'Error with generated row, aborting row generation', DM_GeneratedRowError );
            ZeroMortgage( mtgptr(e[Destination+1]) );
            TMortgage2Grid( mtgptr(e[Destination+1]), MortgageGrid, Destination );
            exit;
          end;
          Source := Destination;
          Inc( Destination );
        end;
      end;
      Source := Set1Source;
    end;
    if( Set1 <> InfoArray[Set1Index].Repetition ) then begin
      CopyRowWithIncrement( Source, Destination, InfoArray[Set1Index].Column, InfoArray[Set1Index].Amount );
      if( errorflag ) then begin
        MessageBox( 'Error with generated row, aborting row generation', DM_GeneratedRowError );
        ZeroMortgage( mtgptr(e[Destination+1]) );
        TMortgage2Grid( mtgptr(e[Destination+1]), MortgageGrid, Destination );
        exit;
      end;
      Source := Destination;
      Inc( Destination );
    end;
  end;
  m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
end;

// source and destination are 0 based.
procedure TMortgageScreen.MoveRow( Source, Destination: integer );
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.MoveRow' );
  if( (Source>maxlines) or (Source<0) or
      (Destination>maxlines) or (Destination<0) ) then begin
    MasterLog.Write( LVL_MED, 'MoveRow destination or source out of range' );
    exit;
  end;
  SetUnsavedData( true );
  e[Destination+1]^ := e[Source+1]^;
  MortgageGrid.CopyRow( Source, Destination );
  MortgageGrid.EmptyRow( Source );
  ZeroMortgage( mtgptr(e[Source+1]) );
end;

// row numbers are specified on the screen, ie 0 based.  Add 1 to get into e
procedure TMortgageScreen.CopyRowWithIncrement( Source, Destination, Column: integer; Increment: double );
var
  DoubleVal: double;
  IntVal: integer;
  StringVal: string;
  IsError: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.CopyRowWithIncrement' );
  if( (Source>maxlines) or (Source<0) or
      (Destination>maxlines) or (Destination<0) ) then begin
    MasterLog.Write( LVL_MED, 'CopyRowWithIncrement destination or source out of range' );
    exit;
  end;
  SetUnsavedData( true );
  e[Destination+1]^ := e[Source+1]^;
  MortgageGrid.CopyRow( Source, Destination );
  StringVal := MortgageGrid.Cells[Column, Destination];
  case Column of
    PriceCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].Price := DoubleVal;
    end;
    PointsCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].Points := DoubleVal/100;
    end;
    PercentDownCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].Pct := DoubleVal/100;
    end;
    CashRequiredCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].Cash := DoubleVal;
    end;
    AmountBorrowedCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].Financed := DoubleVal;
    end;
    YearsCol: begin
      IntVal := StringFormat2Int( StringVal, IsError );
      IntVal := IntVal + trunc(Increment);
      e[Destination+1].Years := IntVal;
    end;
    LoanRateCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := (DoubleVal + Increment)/100;
      e[Destination+1].Rate := RateFromYield( DoubleVal, 12 );
    end;
    MonthlyTaxCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].Tax := DoubleVal;
    end;
    MonthlyTotalCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].Monthly := DoubleVal;
    end;
    BalloonYearsCol: begin
      IntVal := StringFormat2Int( StringVal, IsError );
      IntVal := IntVal + trunc(Increment);
      e[Destination+1].When := IntVal;
    end;
    BalloonAmountCol: begin
      DoubleVal := StringFormat2Double( StringVal, IsError );
      DoubleVal := DoubleVal + Increment;
      e[Destination+1].HowMuch := DoubleVal;
    end;
  end;
  DoCalculation( Destination );
end;

procedure TMortgageScreen.BackupForHelpSystem();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.BackupForHelpSystem' );
  m_BackupFileName := m_FileName;
  m_BackupCaption := Caption;
  m_bBackedUpForHelp := true;
  m_bBackupUnsaved := HasUnsavedData();
  for i:=1 to maxlines do begin
    m_HelpBackup[i]^ := e[i]^;
  end;
  m_FileName := '';
  Caption := 'Mortgage Help';
  SetUnsavedData( false );
end;

procedure TMortgageScreen.RestoreFromHelpSystem();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.RestoreFromHelpSystem' );
  for i:=1 to maxlines do begin
    e[i]^ := m_HelpBackup[i]^;
    TMortgage2Grid( mtgptr(e[i]), MortgageGrid, i-1 );
  end;
  m_bBackedUpForHelp := false;
  m_FileName := m_BackupFileName;
  Caption := m_BackupCaption;
  SetUnsavedData( m_bBackupUnsaved );
end;

function TMortgageScreen.IsBackedUpForHelp(): boolean;
begin
  IsBackedUpForHelp := m_bBackedUpForHelp;
end;

procedure TMortgageScreen.OpenFile( var TheFile: TFileIO );
var
  i: integer;
  Selected: TGridRect;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OpenFile' );
  for i:=1 to maxlines do begin
    MortgageGrid.EmptyRow( i-1 );
    ZeroMortgage( mtgptr(e[i]) );
  end;
  TheFile.GetMortgageArray( e );
  for i:=1 to maxlines do
    TMortgage2Grid( mtgptr(e[i]), MortgageGrid, i-1 );
  if( not m_bBackedUpForHelp ) then
    m_FileName := TheFile.GetFileName()
  else
    m_FileName := '';
  SetUnsavedData( false );
  m_UndoBuffer.StoreData( e, MortgageGrid.Selection );
  Selected.Left := 1;
  Selected.Right := 1;
  Selected.Top := 0;
  Selected.Bottom := 0;
  MortgageGrid.Selection := Selected;
end;

function TMortgageScreen.SaveFile( FileName, ScreenName: string ): boolean;
var
  TheFile: TFileIO;
  hFile: integer;
  Position: integer;
  bWarnOnOverwrite: boolean;
  bCancelPressed: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.SaveFile' );
  bWarnOnOverwrite := false;
  if( ScreenName <> '' ) then
    SaveDialog1.Title := 'Save ' + ScreenName + ' As';
  SaveDialog1.Filter := 'Mortgage Files (*.mtg)|*.MTG';
  if( FileName = '' ) then begin
    if( not SaveDialog1.Execute() ) then begin
      SaveFile := false;
      exit;
    end;
    bWarnOnOverwrite := true;
    FileName := SaveDialog1.FileName;
  end;
  Position := Pos( '.', FileName );
  if( Position = 0 ) then
    FileName := FileName + '.mtg';
  if( not FileExists( FileName ) ) then begin
    hFile := FileCreate( FileName );
    if( hFile = -1 ) then begin
      MasterLog.Write( LVL_MED, 'SaveFile failed to create new file for saving' );
      SaveFile := false;
      exit;
    end;
    FileClose( hFile );
  end else if( bWarnOnOverwrite ) then begin
    MessageBoxWithCancel( 'File Exists, would you like to save over it?', bCancelPressed, DP_OverwriteFile );
    if( bCancelPressed ) then begin
      SaveFile := false;
      exit;
    end;
  end;
  TheFile := TFileIO.Create();
  TheFile.SaveMortgage( FileName, e );
  m_FileName := FileName;
  TheFile.Free();
  SetUnsavedData( false );
  SaveFile := true;
end;

procedure TMortgageScreen.OnPrint();
var
  yPos: integer;
  i: integer;
  MaxRowCount: integer;
  RowsPrinted: integer;
  CurrentRow: integer;
  GridHeight: integer;
  PenWidth: integer;
  UsablePageWidth: integer;
const
  Margin = 80;
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnPrint' );
  MaxRowCount := 0;
  for i:=1 to maxlines do begin
    if( not MortgageIsEmpty( mtgptr(e[i]) ) ) then
      MaxRowCount := i;
  end;
  UsablePageWidth := Printer.PageWidth-(2*Margin);
  Printer.BeginDoc();
  Printer.Canvas.Brush.Color := clWhite;
  Printer.Canvas.Font.Assign( Font );
  Printer.Title := 'Persense Mortgage Screen';
  // first line
  CurrentRow := 0;
  while( CurrentRow < MaxRowcount ) do begin
    Printer.Canvas.Brush.Color := clWhite;
    yPos := 100;
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * PercentDownPos)+Margin+40, yPos, '%' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * CashRequiredPos )+50+Margin, yPos, 'Cash' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * AmountBorrowedPos )+50+Margin, yPos, 'Amount' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * LoanRatePos )+50+Margin, yPos, 'Loan' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * MonthlyTaxPos )+50+Margin, yPos, 'Monthly' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * MonthlyTotalPos )+50+Margin, yPos, 'Monthly' );
    { Position the Balloon Header Box Labels }
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * BalloonYearsPos )+20+Margin, yPos, 'Balloon' );
    // second line
    yPos := yPos + Printer.Canvas.TextHeight( 'Monthly' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * PricePos )+100+Margin, yPos, 'Price' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * PointsPos)+Margin, yPos, 'Points' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * PercentDownPos)+Margin, yPos, 'Down' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * CashRequiredPos )+50+Margin, yPos, 'Required' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * AmountBorrowedPos )+50+Margin, yPos, 'Borrowed' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * YearsPos )+Margin, yPos, 'Years' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * LoanRatePos )+50+Margin, yPos, 'Rate' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * MonthlyTaxPos )+50+Margin, yPos, 'Tax+Ins' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * MonthlyTotalPos )+50+Margin, yPos, 'Total' );
    { Position the Balloon Header Box Labels }
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * BalloonYearsPos )+20+Margin, yPos, 'Yrs' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * BalloonAmountPos )+30+Margin, yPos, 'Amount' );
    // now actually print the grid
    yPos := yPos + Trunc( 1.5*Printer.Canvas.TextHeight( 'Monthly' ));
    GridHeight := Printer.PageHeight-yPos-2*Margin;
    MortgageGrid.Print( CurrentRow, yPos, Margin, UsablePageWidth, GridHeight, MaxRowCount, RowsPrinted );
    CurrentRow := CurrentRow + RowsPrinted;
    // draw boxes around things
    PenWidth := Printer.Canvas.Pen.Width;
    Printer.Canvas.Pen.Width := 6;
    Printer.Canvas.MoveTo( Margin, Margin );
    Printer.Canvas.LineTo( Printer.PageWidth-Margin, Margin );
    Printer.Canvas.LineTo( Printer.PageWidth-Margin, yPos+GridHeight );
    Printer.Canvas.LineTo( Margin, yPos+GridHeight );
    Printer.Canvas.LineTo( Margin, Margin );
    // seperate header from grid
    Printer.Canvas.MoveTo( Margin, yPos );
    Printer.Canvas.LineTo( Printer.PageWidth-Margin, yPos );
    // seperate Balloon fields from the rest
    Printer.Canvas.MoveTo( Trunc( UsablePageWidth * BalloonYearsPos )+Margin, Margin );
    Printer.Canvas.LineTo( Trunc( UsablePageWidth * BalloonYearsPos )+Margin, yPos );
    Printer.Canvas.Pen.Width := PenWidth;
    // possibly set up for a new page
    if( CurrentRow < MaxRowCount ) then
      Printer.NewPage();
  end;
  Printer.EndDoc();
end;

procedure TMortgageScreen.MortgageGridVerifyCellString(Sender: TObject;
  ACol, ARow: Integer; Value: String; var IsError: Boolean);
var
  DoubleVal: double;
begin
  isError := false;
  case ACol of
    PointsCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=10) or (DoubleVal<0) ) then begin
        IsError := true;
        MainForm.SetStatusBarText( 'Points must be between 0 and 10' );
      end;
    end;
    PercentDownCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=100) or (DoubleVal<-9) ) then begin
        IsError := true;
        MainForm.SetStatusBarText( 'Percent down must be between -9 and 100' );
      end;
    end;
    YearsCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=100) or (DoubleVal<0) ) then begin
        IsError := true;
        MainForm.SetStatusBarText( 'Years must be between 0 and 100' );
      end;
    end;
    LoanRateCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=100) or (DoubleVal<=-100) ) then begin
        IsError := true;
        MainForm.SetStatusBarText( 'The loan rate must be between -100 and 100' );
      end;
    end;
    BalloonYearsCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=99) or (DoubleVal<0) ) then begin
        IsError := true;
        MainForm.SetStatusBarText( 'Balloon Years must be between 0 and 100' );
      end;
    end;
  end;
end;

procedure TMortgageScreen.OnContextualHelp();
begin
  HelpSystem.DisplayContents( 'MS_Overview.html' );
end;

procedure TMortgageScreen.MortgageGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    PriceCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_PriceCol) );
    PointsCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_PointsCol) );
    PercentDownCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_PercentDownCol) );
    CashRequiredCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_CashRequiredCol) );
    AmountBorrowedCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_AmountBorrowedCol) );
    YearsCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_YearsCol) );
    LoanRateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_LoanRateCol) );
    MonthlyTaxCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_MonthlyTaxCol) );
    MonthlyTotalCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_MonthlyTotalCol) );
    BalloonYearsCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_BalloonYearsCol) );
    BalloonAmountCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SM_BalloonAmountCol) );
  end;
end;

end.
