{ ============================================================================
  Unit:    MortgageScreenUnit
  Role:    The Mortgage calculator MDI child screen (TMortgageScreen).

  This is the UI/glue layer that sits on top of the pure financial engine in
  unit Mortgage. It owns a spreadsheet-like grid (TPersenseGrid) where each row
  is one mortgage scenario and each column is one mortgage field. The screen's
  job is to:
    - map grid cells <-> the underlying mortgage record array `e` (the global
      mtgarray of mtgline pointers), preserving each field's "hardness" status
      (empty / inp / outp) so the engine knows which fields are inputs;
    - run calculations by delegating to the engine (CalculateRows / Calc) and
      writing the solved values back into the grid;
    - implement the editing model: cut/copy/paste, undo/redo (via
      TMortgageUndoBuffer), delete, and "harden" (promote a calculated value to
      a fixed input);
    - host the higher-level features: Compare APRs and Generate Mtg Rows;
    - load/save .MTG files (via TFileIO) and print the grid;
    - support the help system's live-example backup/restore.

  FIELD-PRESENCE DISPATCH (the program's defining behavior): the user leaves the
  unknown field(s) blank and the engine solves for them. Three of the columns --
  %Down, Cash Required, Amount Borrowed -- are mutually exclusive (they encode
  the same down-payment fact three ways), so editing one clears the other two.
  The per-field status byte (inout: empty/inp/outp) flows from grid edits into
  the mortgage record and is what the engine reads to choose its solve path.

  COORDINATE CONVENTION: grid rows are 0-based (ARow); the record array `e` is
  1-based, so the matching record is always e[ARow+1]. Columns use the *Col
  constants below (1..11); column 0 is the row-number gutter.

  Engine reference: unit Mortgage (Calc, CalculateRows, EnoughDataForAPR,
  ReportAPR, ReportComparisonOfAPRs, EnoughDataForRowGeneration). Rates are
  stored internally as effective rates and shown as nominal-monthly via
  RateFromYield / YieldFromRate.
  ============================================================================ }
unit MortgageScreenUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, ChildWin, StdCtrls, Grids, PersenseGrid, peTypes, peData,
  MortgageUndoBufferUnit, Globals, LogUnit, Menus, FileIOUnit;

const
  { *Pos constants are horizontal layout fractions (0..1 of the form/page width)
    used to position the column header labels in FormResize and OnPrint.
    *Col constants are the 1-based grid column indexes for each mortgage field
    (column 0 is the row-number gutter). The two sets are paired by field. }
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
  WA_ACTIVATE                   = 1;   { Windows activation flag (window-message constant) }

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
    { Grid/form event handlers (wired in the .dfm). Each is detailed at its impl. }
    procedure FormClose(Sender: TObject; var Action: TCloseAction);                                                            { close-confirm }
    procedure FormResize(Sender: TObject);                                                                                     { layout }
    procedure MortgageGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean );     { store edited cell into record }
    procedure MortgageGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean); { Enter = calc + move cursor }
    procedure MortgageGridEditCopy(Sender: TObject);                                                                           { context-menu Copy }
    procedure MortgageGridEditCut(Sender: TObject);                                                                            { context-menu Cut }
    procedure MortgageGridEditPaste(Sender: TObject);                                                                          { context-menu Paste }
    procedure MortgageGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);        { range validation }
    procedure MortgageGridCellBeforeEdit(Sender: TObject; ACol, ARow: Integer; const Value: String);                          { enforce mutually-exclusive cols }
    procedure MortgageGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);                           { update status-bar help }
  private
    { Private declarations }
  public
    constructor Create(AOwner: TComponent); override;            { allocate row records, set up columns }
    destructor Destroy(); override;                              { free row records + undo buffer }
    function GetType(): TScreenType; override;                   { returns MortgageType }
    procedure OnCalculate(); override;                           { solve the current row }
    procedure OnHardenValue(); override;                         { promote selected calc'd value(s) to input }
    procedure OnUndo(); override;                                { restore previous state from undo buffer }
    procedure OnRedo(); override;                                { re-apply an undone state }
    procedure OnCut(); override;                                 { copy selection then clear it }
    procedure OnCopy(); override;                                { copy selection to clipboard }
    procedure OnPaste(); override;                               { paste clipboard into grid + records }
    procedure OnDelete(); override;                              { delete char (in edit) or clear selection }
    procedure OpenFile( var TheFile: TFileIO ); override;        { load mortgage rows from a parsed file }
    function SaveFile( FileName, ScreenName: string ): boolean; override; { save rows to a .MTG file }
    procedure OnPrint(); override;                               { print the grid with headers/boxes }
    procedure OnContextualHelp(); override;                      { open the mortgage overview help page }
    procedure SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont ); override; { apply colours/fonts }
    procedure CompareAPRs();                                     { compare APRs of two mortgage rows }
    procedure GenerateMtgRows();                                 { fan one row into many via increments }
    procedure BackupForHelpSystem();                            { save real data before loading a help example }
    procedure RestoreFromHelpSystem();                         { restore real data after a help example }
    function IsBackedUpForHelp(): boolean;                       { is example data currently loaded? }
  protected
    m_UndoBuffer : TMortgageUndoBuffer;   { ring of saved grid states for undo/redo }
    m_UndoRedoHolder: mtgRecList;         { scratch buffer used when applying an undo/redo state }
    m_HelpBackup: mtgarray;               { snapshot of the user's rows while a help example is shown }
    m_bBackedUpForHelp: boolean;          { true while example data is displayed }
    m_BackupFileName: string;             { saved file name during help example }
    m_BackupCaption: string;              { saved window caption during help example }
    m_bBackupUnsaved: boolean;            { saved unsaved-data flag during help example }
    procedure AssignMortgageValues( ACol, ARow: Integer; const Value: String );                       { write cell -> record as input }
    procedure AssignMortgageValuesEx( ACol, ARow: Integer; const Value: String; const Hardness: InOut ); { write cell -> record with explicit status }
    procedure DoCalculation( Row: integer );                                                          { solve one row + push results to grid }
    procedure TMortgage2Grid( Mortgage: mtgptr; Grid: TPersenseGrid; Row: integer );                  { render a record into a grid row }
    procedure MoveRow( Source, Destination: integer );                                                { move a row (record + grid) }
    procedure CopyRowWithIncrement( Source, Destination, Column: integer; Increment: double );        { copy row, bump one column, recalc }
  end;

var
  MortgageScreen: TMortgageScreen;

implementation

{$R *.dfm}
uses MortgageRowGenerationDlgUnit, INTSUTIL, Mortgage, MAIN,
     Printers, APRComparisonDLGUnit, SelectAPRDlgUnit, HelpSystemUnit;

{ Constructor: build the mortgage screen. Allocates and zeroes the per-row
  record buffers -- the global display array e[], the undo scratch
  m_UndoRedoHolder[], and the help-backup m_HelpBackup[] -- one mtgline each up
  to maxlines, and labels the grid's row-number gutter. Configures each grid
  column (index, layout position, data type/format, width). Creates the undo
  buffer. Triggered once when the screen is first opened. }
{ Go port: n/a -- DOS text UI; superseded by web frontend cmd/persense/static/index.html + internal/api/handlers.go. }
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

{ Destructor: free the per-row record buffers (e[] and the undo scratch) and the
  undo buffer object.
  NOTE: m_HelpBackup[] rows allocated in Create are not freed here -- a
  pre-existing leak preserved as-is. }
{ Go port: n/a -- DOS text UI; superseded by web frontend cmd/persense/static/index.html + internal/api/handlers.go. }
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

{ GetType: identifies this MDI child as the Mortgage screen (used by MAIN to
  pick the right menus and file routing). }
{ Go port: n/a -- DOS/VCL screen-type tag; the web port routes by URL, no screen-type enum. }
function TMortgageScreen.GetType(): TScreenType;
begin
  GetType := MortgageType;
end;

{ FormClose: forward the window-close to the shared TMDIChild.OnFormClose, which
  handles the unsaved-data save prompt. }
{ Go port: n/a -- DOS/VCL window lifecycle; no equivalent in the stateless web port. }
procedure TMortgageScreen.FormClose(Sender: TObject; var Action: TCloseAction);
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.FormClose' );
  OnFormClose( Action );
end;

{ FormResize: re-lay-out the header group boxes, the column header labels (each
  positioned by its *Pos fraction of the form width) and the grid. Pure UI
  geometry; runs on every resize and after SetUISettings. }
{ Go port: n/a -- DOS/VCL grid layout; CSS handles responsive layout in cmd/persense/static/index.html. }
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
{ MortgageGridCellAfterEdit: fired after a cell edit commits. Persists the new
  string into the matching record field (AssignMortgageValues), with two special
  cases: entering a Balloon-Years value clears any blank-derived Monthly total so
  it gets recomputed; and an unusually high Loan Rate triggers a warning. If the
  data actually changed, snapshots state for undo. Params ACol/ARow = 0-based
  cell; Value = committed text; DataChanged = whether the text changed. }
{ Go port: n/a -- DOS/VCL grid edit event; the store-typed-value step is JSON->MtgLine in internal/api/handlers.go: mtgLineFromInput (line 581). }
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

{ MortgageGridCellBeforeEdit: fired just before a cell becomes editable. Enforces
  the mutual exclusivity of the three down-payment columns (%Down, Cash Required,
  Amount Borrowed): editing any one of them blanks the other two (grid + record),
  since they express the same fact and only one may be a user input at a time. }
{ Go port: n/a -- DOS/VCL grid edit event; complementary-cell clearing is client-side in cmd/persense/static/index.html. }
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

{ AssignMortgageValues: convenience wrapper -- store a cell value treating it as
  user INPUT (hardness = inp). Most edits go through here. }
{ Go port: internal/api/handlers.go: mtgLineFromInput (line 581) -- JSON request fields (with their status) become an MtgLine; the per-field unit conversions and empty->StatusEmpty logic live there. }
procedure TMortgageScreen.AssignMortgageValues( ACol, ARow: Integer; const Value: String );
begin
  AssignMortgageValuesEx( ACol, ARow, Value, inp );
end;

{ useful function for assigning a string value at a given row and column
  to the underlying TMortgage object }
{ AssignMortgageValuesEx: parse Value and store it into the e[ARow+1] mortgage
  record field selected by ACol, tagging that field's status with Hardness
  (inp for typed input, or the pasted cell's original status). Empty Value sets
  the field to 0 and status=empty (the engine then treats it as a blank to solve
  for). Also mirrors the hardness onto the grid cell colouring. Per-field unit
  conversions: Points and %Down are entered as percents and stored /100; Loan
  Rate is entered as a nominal monthly yield and stored as an effective rate via
  RateFromYield(rate,12). Params: ACol/ARow 0-based cell; Hardness = inout status. }
{ Go port: internal/api/handlers.go: mtgLineFromInput (line 581) -- same field-by-field storage + the Points/%Down /100 and Loan-Rate RateFromYield(rate,12) conversions; a missing JSON pointer field maps to StatusEmpty (the "blank to solve for"). }
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
        { entered value is a nominal annual rate; /100 then convert the implied
          monthly compounding to the effective rate the engine stores }
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
{ OnCalculate: the Calculate command for the active row. Forces any in-progress
  cell edit to commit (turning off EditorMode fires CellAfterEdit), solves the
  current row via DoCalculation, then snapshots the result for undo. Triggered
  by the Calculate menu/toolbar/Enter. }
{ Go port: internal/api/handlers.go: HandleMortgageCalc (line 478) -- one POST solves the active row; the "commit in-progress edit" and undo-snapshot steps are client-side in cmd/persense/static/index.html. }
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
{ DoCalculation: solve a single mortgage row. Updates the engine's used-row count
  (nlines[MTGBlock]), calls the engine CalculateRows on just this row (which reads
  the per-field statuses to decide what to solve for -- field-presence dispatch),
  then writes the solved record back to the grid. Param: Row = 0-based grid row. }
{ Go port: internal/finance/mortgage/mortgage.go: Calc (line 184) -- the field-presence solve for one row; internal/api/handlers.go: HandleMortgageCalc (line 478) is the caller. }
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
{ TMortgage2Grid: render one mortgage record into grid row `Row`, field by field.
  For each field: if its status is non-empty, format the value (reversing the
  storage conversions -- Points/%Down x100, Rate via YieldFromRate x100) and set
  the cell with its status (which drives input vs calculated-output colouring);
  if empty, clear the cell. Early-out if the row is past the used range and the
  record is empty. This is the inverse of AssignMortgageValuesEx. }
{ Go port: internal/api/handlers.go: HandleMortgageCalc (line 478) -- the CalcResult is serialized to JSON (reversing storage conversions: Points/%Down x100, Rate via YieldFromRate x100); cmd/persense/static/index.html paints it, using status for input-vs-output cell colouring. }
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

{ MortgageGridEditEnterKeyPressed: Enter pressed while editing a cell. Solves the
  row immediately, then implements smart cursor movement: after entering %Down or
  Cash Required, if the other down-payment fields came out as calculated outputs,
  skip the cursor past them (setting DefaultAction := false to suppress the grid's
  normal advance). Finally OverWrites the undo snapshot so the calculation folds
  into the same undo step as the edit. }
{ Go port: n/a -- DOS/VCL keyboard + smart-cursor movement; the recalc-on-Enter behavior is client-side in cmd/persense/static/index.html (it calls internal/api/handlers.go: HandleMortgageCalc line 478). }
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

{ OnUndo: pull the previous saved state from the undo buffer into m_UndoRedoHolder;
  if successful, zero the live records, copy the restored records in, repaint every
  grid row from them, and restore the saved selection. No-op if nothing to undo. }
{ Go port: n/a -- undo handled client-side in cmd/persense/static/index.html. }
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

{ OnRedo: mirror of OnUndo -- re-apply the next state from the undo buffer's redo
  side, rebuilding the records, grid and selection. No-op if nothing to redo. }
{ Go port: n/a -- undo/redo handled client-side in cmd/persense/static/index.html. }
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
{ OnHardenValue: "harden" the selected cells -- promote each selected field from
  a calculated output (or whatever) to a fixed INPUT (status := inp), so the next
  calculation treats it as given rather than re-solving it. For the down-payment
  trio, hardening one column also blanks the other two (preserving their mutual
  exclusivity). Repaints affected rows and snapshots for undo. Triggered by the
  Edit>Harden menu/shortcut. }
{ Go port: n/a -- DOS/VCL UI action (flip a computed outp cell into a locked inp); done client-side in cmd/persense/static/index.html by re-posting that field as an input to internal/api/handlers.go: HandleMortgageCalc (line 478). }
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

{ OnCut: copy the selection to the clipboard, then clear every selected cell in
  both the grid and the backing records (AssignMortgageValues with ''). Snapshots
  for undo. }
{ Go port: n/a -- clipboard cut handled client-side in cmd/persense/static/index.html. }
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
{ Go port: n/a -- clipboard handled client-side in cmd/persense/static/index.html. }
{ Grid context-menu Cut -> delegate to OnCut. }
procedure TMortgageScreen.MortgageGridEditCut(Sender: TObject);
begin
  OnCut();
end;

{ Go port: n/a -- clipboard copy handled client-side in cmd/persense/static/index.html. }
{ OnCopy: copy the current grid selection to the clipboard (records unchanged). }
procedure TMortgageScreen.OnCopy();
begin
  MasterLog.Write( LVL_LOW, 'TMortgageScreen.OnCopy' );
  MortgageGrid.CopyToClipboard();
end;

// hook in from PersenseGrid Context menu
{ Go port: n/a -- clipboard handled client-side in cmd/persense/static/index.html. }
{ Grid context-menu Copy -> delegate to OnCopy. }
procedure TMortgageScreen.MortgageGridEditCopy(Sender: TObject);
begin
  OnCopy();
end;

{ OnPaste: paste clipboard cells into the grid (PasteFromClipboard returns the
  filled rectangle), then mirror each pasted cell into the records using its
  pasted hardness (AssignMortgageValuesEx with the cell's GetCellHardness), so
  pasted input/output status is preserved. Snapshots for undo. }
{ Go port: n/a -- clipboard paste handled client-side in cmd/persense/static/index.html; pasted cells become inputs re-posted to internal/api/handlers.go: HandleMortgageCalc (line 478). }
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
{ Go port: n/a -- clipboard handled client-side in cmd/persense/static/index.html. }
{ Grid context-menu Paste -> delegate to OnPaste. }
procedure TMortgageScreen.MortgageGridEditPaste(Sender: TObject);
begin
  OnPaste();
end;

{ OnDelete: context-sensitive Delete. While editing a cell, delete the character
  to the right of the caret (or the selected substring) within the in-place
  editor's text. Otherwise (cell-selection mode), clear every selected cell in the
  grid and records and snapshot for undo. }
{ Go port: n/a -- DOS/VCL grid row/cell delete; handled client-side in cmd/persense/static/index.html. }
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

{ SetUISettings: apply the app-wide colour/font preferences to the grid (normal
  cell colour/font, calculated-output cell colour/font, selection colour), then
  re-layout and repaint. Called by MAIN on creation and whenever UI options change. }
{ Go port: n/a -- DOS/VCL colour/font settings; styling is CSS in cmd/persense/static/index.html. }
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
{ CompareAPRs: the Compare-APRs feature. Counts how many rows contain valid data
  and branches:
    0 rows  -> prompt the user to enter data.
    1 row   -> just report that one row's APR (ReportAPR).
    2 rows  -> calc both, and if both have enough data, show the side-by-side APR
               comparison dialog (ReportComparisonOfAPRs).
    3+ rows -> use the selection: the top selected row is the first mortgage; if
               exactly two rows are selected, the second is the partner; otherwise
               pop a chooser (SelectAPRDlg) listing the other valid rows, or fall
               back to reporting just the first row. Then do the 2-row comparison.
  Reads the global row array e; uses the engine's Calc/EnoughDataForAPR/Report*
  routines; aborts on errorflag. Triggered by Calculate>Compare APRs (Ctrl+A). }
{ Go port: internal/finance/mortgage/mortgage.go: CompareAPRs (line 505) -- the two-loan APR comparison; internal/api/handlers.go: HandleMortgageCompare (line 625) is the entry. The row-count/selection branching and single-row ReportAPR fallback are UI orchestration done in cmd/persense/static/index.html. }
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

{ GenerateMtgRows: scenario generator. Starting from the selected (seed) row,
  expand it into a family of rows by varying up to THREE columns, each by a
  repetition count and a per-step increment (entered via MortgageRowGenerationDlg
  -> InfoArray). The total new rows = product of (repetition+1) over the active
  varying columns, minus one (the seed itself stays). Steps:
    1. Validate the seed has enough data and calculates cleanly.
    2. Ask the dialog for the up-to-3 (column, repetition, amount) specs.
    3. Compute NewRowCount; bail if zero or if it would overflow the grid.
    4. Shift existing rows below the seed down by NewRowCount to make room.
    5. Order the active specs into Set1/Set2/Set3 (outer..inner) and run nested
       loops, each new row produced by CopyRowWithIncrement (copy previous row,
       bump the chosen column, recalc). Abort cleanly on any per-row error.
  Triggered by Calculate>Generate Mtg Rows (Ctrl+T). Mutates e[] and the grid. }
{ Go port: internal/finance/mortgage/rowgen.go: GenerateGrid (line 156) -- the nested up-to-3-column vary-by-increment expansion (via GenerateRows line 58 / bumpField line 111); internal/api/handlers.go: HandleMortgageWhatIf (line 696) is the entry. Grid row-shifting/repaint is client-side in cmd/persense/static/index.html. }
procedure TMortgageScreen.GenerateMtgRows();
var
  Row: integer;
  MaxRowInNeedOfMove: integer;
  InfoArray: array [0..2] of NewMtgRowInfo;   { up-to-3 varying-column specs from the dialog }
  i: integer;
  NewRowCount: integer;                       { number of NEW rows to create (excludes the seed) }
  CreateNewRows: boolean;
  Set1Index, Set2Index, Set3Index: integer;   { which InfoArray entry drives each nested loop level }
  Set1, Set2, Set3: integer;
  Set1Source, Set2Source: integer;            { row to copy from when restarting an inner loop }
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
  { Map the populated InfoArray entries (Column <> -1) onto loop levels
    Set1 (outermost) .. Set3 (innermost); -1 means that level is unused. }
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
{ MoveRow: relocate a full row from Source to Destination (0-based grid rows).
  Copies the record (e[Source+1] -> e[Destination+1]) and the grid cells, then
  empties/zeros the source. Range-checked; logs and exits on bad indices. Used by
  GenerateMtgRows to clear space for generated rows. }
{ Go port: n/a -- DOS/VCL grid row move; handled client-side in cmd/persense/static/index.html. }
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
{ CopyRowWithIncrement: the workhorse of row generation. Copies Source's record
  and grid cells to Destination, then bumps the one field named by Column by
  Increment (reading the just-copied cell text, adding the increment, and applying
  the same per-field unit conversions as data entry -- Points/%Down /100, Rate via
  RateFromYield, Years/When truncated to int), and recalculates the Destination
  row. Range-checked. Params 0-based Source/Destination; Column = a *Col constant. }
{ Go port: internal/finance/mortgage/rowgen.go: bumpField (line 111) -- copy row then bump the chosen field by the increment (same per-field unit conversions), then Calc; driven by GenerateRows (line 58) / GenerateGrid (line 156). }
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

{ BackupForHelpSystem: stash the user's current rows, file name, caption and
  unsaved flag (into m_HelpBackup/m_Backup*), then blank the file name and set a
  "Mortgage Help" caption so a help EXAMPLE file can be loaded over the top without
  losing or appearing to dirty the user's real work. Sets m_bBackedUpForHelp. }
{ Go port: n/a -- help-example scratch-buffer swap; handled client-side in cmd/persense/static/index.html. }
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

{ RestoreFromHelpSystem: undo BackupForHelpSystem -- copy the saved rows back into
  e[], repaint the grid, and restore the file name, caption and unsaved flag, so
  the user's real data reappears once they leave the help example. }
{ Go port: n/a -- help-example scratch-buffer restore; handled client-side in cmd/persense/static/index.html. }
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

{ IsBackedUpForHelp: true while a help example is being shown over the user's
  real data (so MAIN knows to restore). }
{ Go port: n/a -- help-example state flag; handled client-side in cmd/persense/static/index.html. }
function TMortgageScreen.IsBackedUpForHelp(): boolean;
begin
  IsBackedUpForHelp := m_bBackedUpForHelp;
end;

{ OpenFile: load mortgage rows from an already-parsed TFileIO. Clears the grid and
  records, pulls the rows out (GetMortgageArray), repaints every row, sets the file
  name (blank if this is a help-example load), clears the unsaved flag, seeds the
  undo buffer, and selects the first data cell. Param: TheFile = loaded document. }
{ Go port: internal/fileio/loader.go: LoadMortgageFile (line 44) parses the legacy .psn document into records; internal/api/import_psn.go: HandleImportPSN (line 121) is the entry that returns them to cmd/persense/static/index.html for display. The grid-population/selection steps are UI, n/a. }
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

{ SaveFile: save the mortgage rows to a .MTG file.
  Params:  FileName = target path (empty -> prompt via SaveDialog1); ScreenName =
           optional label for the dialog title.
  Returns: true on success, false if the user cancels or creation fails.
  Behavior: if no name given, run the Save dialog and arm the overwrite warning;
  default the .mtg extension; create the file if new, else confirm overwrite; then
  serialize via TFileIO.SaveMortgage, record the file name and clear the unsaved
  flag. Triggered by File>Save / Save As. }
{ Go port: n/a -- the port imports legacy .psn files (internal/fileio/loader.go: LoadMortgageFile line 44 via internal/api/import_psn.go: HandleImportPSN line 121) but does not write them back; save/export is client-side in cmd/persense/static/index.html. }
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

{ OnPrint: print the whole mortgage grid. Finds the last non-empty row, then loops
  page by page: draws the two-line column header (positioned by the *Pos fractions
  of the usable page width), prints as many grid rows as fit (MortgageGrid.Print
  returns how many were printed), draws the surrounding box, the header separator
  and the balloon-section divider, and starts a new page if rows remain. Called via
  File>Print after the print dialog. Margin is a fixed page inset. }
{ Go port: n/a -- printer canvas layout; the web frontend cmd/persense/static/index.html handles print/export of the grid. }
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

{ MortgageGridVerifyCellString: per-column input validation hook, called by the
  grid before accepting a typed value. Parses the value and range-checks the
  bounded columns (Points 0..10, %Down -9..100, Years 0..100, Loan Rate -100..100,
  Balloon Years 0..99); on violation sets IsError and shows the limit in the status
  bar. Columns not listed are unchecked. Out-param IsError tells the grid to reject. }
{ Go port: n/a -- per-cell input validation; browser <input> validation in cmd/persense/static/index.html, with server-side parse/validate in internal/api/handlers.go: HandleMortgageCalc (line 478). }
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

{ Go port: n/a -- DOS/VCL help navigation; help content served statically, linked from cmd/persense/static/index.html. }
{ OnContextualHelp: open this screen's overview help page (Help>Contextual Help). }
procedure TMortgageScreen.OnContextualHelp();
begin
  HelpSystem.DisplayContents( 'MS_Overview.html' );
end;

{ MortgageGridSelectCell: fired when the grid selection moves to a new cell.
  Updates the status bar with the help string for the selected column (per-field
  guidance), so the bottom of the screen describes whatever cell is focused.
  CanSelect is left as inherited (selection always allowed). }
{ Go port: n/a -- DOS/VCL grid selection event; handled client-side in cmd/persense/static/index.html. }
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
