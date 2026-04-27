unit PresentValueScreenUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, ChildWin, StdCtrls, Grids, PersenseGrid, Presvalu, FileIOUnit,
  peData, peTypes, PresentValueUndoBufferUnit, TableOutUnit;

const
  // LumpSum cols
  LSMDateCol                    = 0;
  LSMAmountCol                  = 1;
  LSMValueCol                   = 2;
  // periodic cols
  PERFromDateCol                = 0;
  PERToDateCol                  = 1;
  PERPerYrCol                   = 2;
  PERAmountCol                  = 3;
  PERCOLACol                    = 4;
  PERValueCol                   = 5;
  // presentval cols
  PRVAsOfCol                    = 0;
  PRVTrueRateCol                = 1;
  PRVLoanRateCol                = 2;
  PRVYieldCol                   = 3;
  PRVValueCol                   = 4;
  // rateline cols
  RTLDateCol                    = 0;
  RTLTrueRateCol                = 1;
  RTLLoanRateCol                = 2;
  RTLYieldCol                   = 3;
  // xpresval cols
  XPRAsOfCol                    = 0;
  XPRComputationCol             = 1;
  XPRValueCol                   = 2;

type
  TPresentValueScreen = class(TMDIChild)
    LumpSumGrid: TPersenseGrid;
    PeriodicGrid: TPersenseGrid;
    PresentValueGrid: TPersenseGrid;
    GroupBox1: TGroupBox;
    GroupBox2: TGroupBox;
    GroupBox3: TGroupBox;
    SingleDateLabel: TLabel;
    SingleAmountLabel: TLabel;
    SingleValueLabel: TLabel;
    PeriodicFromLabel: TLabel;
    PeriodicThroughLabel: TLabel;
    PeriodicPerYrLabel: TLabel;
    PeriodicAmountLabel: TLabel;                                                    
    PeriodicColaLabel: TLabel;
    PeriodicValueLabel: TLabel;
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    Label4: TLabel;
    Label5: TLabel;
    PlainGroup: TGroupBox;
    AdvancedGroup: TGroupBox;
    GroupBox4: TGroupBox;
    RatelineGrid: TPersenseGrid;
    GroupBox5: TGroupBox;
    XPresValGrid: TPersenseGrid;
    Label6: TLabel;
    Label7: TLabel;
    Label8: TLabel;
    Label9: TLabel;
    Label10: TLabel;
    Label11: TLabel;
    Label12: TLabel;
    Label13: TLabel;
    SaveDialog1: TSaveDialog;
    procedure LumpSumGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure LumpSumGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure PeriodicGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure PeriodicGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure LumpSumGridDownAfterGrid(Sender: TObject);
    procedure PeriodicGridDownAfterGrid(Sender: TObject);
    procedure PresentValueGridUpBeforeGrid(Sender: TObject);
    procedure LumpSumGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure PeriodicGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure PresentValueGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure RatelineGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure RatelineGridUpBeforeGrid(Sender: TObject);
    procedure XPresValGridUpBeforeGrid(Sender: TObject);
    procedure XPresValGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure RatelineGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure LumpSumGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure PeriodicGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure RatelineGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure PresentValueGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure XPresValGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
    procedure RatelineGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure XPresValGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure LumpSumGridEditCut(Sender: TObject);
    procedure PeriodicGridEditCut(Sender: TObject);
    procedure PresentValueGridEditCut(Sender: TObject);
    procedure RatelineGridEditCut(Sender: TObject);
    procedure XPresValGridEditCut(Sender: TObject);
    procedure LumpSumGridEditCopy(Sender: TObject);
    procedure PeriodicGridEditCopy(Sender: TObject);
    procedure PresentValueGridEditCopy(Sender: TObject);
    procedure RatelineGridEditCopy(Sender: TObject);
    procedure XPresValGridEditCopy(Sender: TObject);
    procedure LumpSumGridEditPaste(Sender: TObject);
    procedure PeriodicGridEditPaste(Sender: TObject);
    procedure PresentValueGridEditPaste(Sender: TObject);
    procedure RatelineGridEditPaste(Sender: TObject);
    procedure XPresValGridEditPaste(Sender: TObject);
    procedure PeriodicGridCellBeforeEdit(Sender: TObject; ACol, ARow: Integer; const Value: String);
    procedure PeriodicGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);
    procedure PresentValueGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);
    procedure RatelineGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);
    procedure FormResize(Sender: TObject);
    procedure LumpSumGridCellBeforeEdit(Sender: TObject; ACol, ARow: Integer; const Value: String);
    procedure LumpSumGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure PeriodicGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure PresentValueGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure RatelineGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure XPresValGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
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
    function SaveFile( FileName, ScreenName : string ): boolean; override;
    procedure OnPrint(); override;
    procedure OnContextualHelp(); override;
    procedure SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont ); override;
    procedure ToggleAdvanced();
    function AdvancedIsOn(): boolean;
    function IsBackedUpForHelp(): boolean;
    procedure BackupForHelpSystem();
    procedure RestoreFromHelpSystem();
    procedure TableOutput();
    procedure FixRates( NewPerYr: byte );
  protected
    m_UndoBuffer: TPresentValueUndoBuffer;
    m_bBackedUpForHelp: boolean;
    m_HelpBackupLumpSum : lumpsumarray;
    m_HelpBackupPeriodic : periodicarray;
    m_HelpBackupRateLine : ratelinearray;
    m_HelpBackupPresVal : presvalarray;
    m_HelpBackupXPresVal : xpresval;
    m_BackupFileName: string;
    m_BackupCaption: string;
    m_bBackupUnsaved: boolean;
    m_TableOut: TTableOut;
    procedure DoCalculation();
    // getting data to and from the grids
    procedure AssignLumpSumValues( Data: lumpsumarray; ACol, ARow: integer; Value: string; Status: inout );
    procedure LumpSumValues2Grid( Data: LumpSumArray; Grid: TPersenseGrid );
    procedure AssignPeriodicValues( Data: periodicarray; ACol, ARow: integer; Value: string; Status: inout );
    procedure PeriodicValues2Grid( Data: periodicarray; Grid: TPersenseGrid );
    procedure AssignPresentValueValues( Data: presvalarray; ACol, ARow: integer; Value: string; Status: inout );
    procedure PresentValueValues2Grid( Data: presvalarray; Grid: TPersenseGrid );
    procedure AssignRateLineValues( Data: ratelinearray; ACol, ARow: integer; Value: string; Status: inout );
    procedure RateLineValues2Grid( Data: ratelinearray; Grid: TPersenseGrid );
    procedure AssignXPresValValues( Data: xpresvalptr; ACol: integer; Value: string; Status: inout );
    procedure XPresValValues2Grid( Data: xpresvalptr; Grid: TPersenseGrid );
    procedure AllPresentValueData2Grids();
    // per grid functions
    function GetFocusedGrid(): TPersenseGrid;
    // cut
    procedure LumpSumGridCut();
    procedure PeriodicGridCut();
    procedure RateLineGridCut();
    procedure XPresValGridCut();
    procedure PresentValueGridCut();
    // paste
    procedure LumpSumGridPaste();
    procedure PeriodicGridPaste();
    procedure RateLineGridPaste();
    procedure XPresValGridPaste();
    procedure PresentValueGridPaste();
    // delete
    procedure LumpSumGridDelete();
    procedure PeriodicGridDelete();
    procedure RateLineGridDelete();
    procedure XPresValGridDelete();
    procedure PresentValueGridDelete();
    // harden
    procedure HardenLumpSumGrid();
    procedure HardenPeriodicGrid();
    procedure HardenRateLineGrid();
    procedure HardenXPresValGrid();
    procedure HardenPresentValueGrid();
  end;

var
  PresentValueScreen: TPresentValueScreen;

implementation

uses Globals, intsutil, LogUnit, MAIN, Printers, HelpSystemUnit;

{$R *.dfm}

constructor TPresentValueScreen.Create(AOwner: TComponent);
var
  i: integer;
  Strings: TStringList;
  CanSelect: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.Create begin' );
  inherited Create( AOwner );
  for i:=1 to maxlines do begin
    GetMem( a[i], sizeof(lumpsum) );
    ZeroLumpSum( lumpsumptr(a[i]) );
    GetMem( b[i], sizeof(periodic) );
    ZeroPeriodic( periodicptr(b[i]) );
    GetMem( cc[i], sizeof(rateline) );
    ZeroRateLine( ratelineptr(cc[i]) );
    GetMem( m_HelpBackupLumpSum[i], sizeof(lumpsum) );
    GetMem( m_HelpBackupPeriodic[i], sizeof(periodic) );
    GetMem( m_HelpBackupRateLine[i], sizeof(rateline) );
  end;
  for i:=1 to presvallines do begin
    GetMem( c[i], sizeof(presval) );
    ZeroPresVal( presvalptr(c[i]) );
    GetMem( m_HelpBackupPresVal[i], sizeof(presval) );
  end;
  GetMem( d, sizeof(xpresval) );
  ZeroXPresVal( xpresvalptr(d) );
  LumpSumGrid.RowCount := maxlines;
  PeriodicGrid.RowCount := maxlines;
  PresentValueGrid.RowCount := presvallines;
  RateLineGrid.RowCount := maxlines;
  LumpSumGrid.SetupColumn( 0, 0.0, ColTypeDate, DateCellLength );
  LumpSumGrid.SetupColumn( 1, 0.30, ColTypeDollar, 15 );
  LumpSumGrid.SetupColumn( 2, 0.60, ColTypeDollar, 15 );
  PeriodicGrid.SetupColumn( 0, 0.0, ColTypeDate, DateCellLength );
  PeriodicGrid.SetupColumn( 1, 0.18, ColTypeDate, DateCellLength );
  PeriodicGrid.SetupColumn( 2, 0.36, ColTypeInt, 2 );
  PeriodicGrid.SetupColumn( 3, 0.42, ColTypeDollar, 14 );
  PeriodicGrid.SetupColumn( 4, 0.62, ColType4Real, 9 );
  PeriodicGrid.SetupColumn( 5, 0.74, ColTypeDollar, 15 );
  PresentValueGrid.SetupColumn( 0, 0.0, ColTypeDate, DateCellLength );
  PresentValueGrid.SetupColumn( 1, 0.2, ColType4Real, 13 );
  PresentValueGrid.SetupColumn( 2, 0.4, ColType4Real, 13 );
  PresentValueGrid.SetupColumn( 3, 0.6, ColType4Real, 13 );
  PresentValueGrid.SetupColumn( 4, 0.8, ColTypeDollar, 16 );
  RatelineGrid.SetupColumn( 0, 0.0, ColTypeDate, DateCellLength );
  RatelineGrid.SetupColumn( 1, 0.24, ColType4Real, 14 );
  RatelineGrid.SetupColumn( 2, 0.48, ColType4Real, 13 );
  RatelineGrid.SetupColumn( 3, 0.73, ColType4Real, 13 );
  RatelineGrid.SetCellReadOnly( 0, 0 );
  XPresValGrid.SetupColumn( 0, 0.0, ColTypeDate, DateCellLength );
  XPresValGrid.SetupColumn( 1, 0.33, ColTypeStringList, 8 );
  XPresValGrid.SetupColumn( 2, 0.66, ColTypeDollar, 18 );
  Strings := TStringList.Create();
  Strings.Add( 'Simple' );
  Strings.Add( 'Compound' );
  XPresValGrid.SetColumnStringList( 1, Strings );
  XPresValGrid.SetCell( 'Simple', XPRComputationCol, 0, inp );
  Strings.Free();
  m_bBackedUpForHelp := false;
  Width := 620;
  Height := 354;
  m_UndoBuffer := TPresentValueUndoBuffer.Create();
  // must do this to get the status text going
  LumpSumGridSelectCell( Self, 0, 0, CanSelect );
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.Create end' );
end;

destructor TPresentValueScreen.Destroy();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.Destroy begin' );
  m_UndoBuffer.Destroy();
  for i:=1 to maxlines do begin
    FreeMem( a[i] );
    FreeMem( b[i] );
    FreeMem( cc[i] );
    FreeMem( m_HelpBackupLumpSum[i] );
    FreeMem( m_HelpBackupPeriodic[i] );
    FreeMem( m_HelpBackupRateLine[i] );
  end;
  for i:=1 to presvallines do begin
    FreeMem( c[i] );
    FreeMem( m_HelpBackupPresVal[i] );
  end;
  FreeMem( d );
  m_TableOut.Free();
  inherited Destroy();
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.Destroy end' );
end;

function TPresentValueScreen.GetType(): TScreenType;
begin
  GetType := PresentValueType;
end;

procedure TPresentValueScreen.OnCalculate();
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnCalculate' );
  DoCalculation();
  m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
end;

procedure TPresentValueScreen.DoCalculation();
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.DoCalculation' );
  nlines[PVLLumpSumBlock] := LumpSumGrid.GetUsedRowCount();
  nlines[PVLPeriodicBlock] := PeriodicGrid.GetUsedRowCount();
  if( pvlfancy ) then begin
    nlines[PVLPresValBlock] := RateLineGrid.GetUsedRowCount();
    nlines[PVLXBlock] := XPresValGrid.GetUsedRowCount();
    cc[1].date := earliest;    // not sure why, but these are locked in like this
    cc[1].datestatus := defp;
  end else
    nlines[PVLPresValBlock] := PresentValueGrid.GetUsedRowCount();
  Enter( no_tab );
  AllPresentValueData2Grids();
end;

function TPresentValueScreen.AdvancedIsOn(): boolean;
begin
  AdvancedIsOn := pvlfancy;
end;

procedure TPresentValueScreen.ToggleAdvanced();
begin
  pvlfancy := not pvlfancy;
  if( not pvlfancy ) then
    XPresValGrid.DisableComboBox();
  AdvancedGroup.Visible := pvlfancy;
  PlainGroup.Visible := not pvlfancy;
end;

procedure TPresentValueScreen.OnHardenValue();
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnHardenValue' );
  if( LumpSumGrid.Focused() ) then
    HardenLumpSumGrid()
  else if( PeriodicGrid.Focused() ) then
    HardenPeriodicGrid();
  if( AdvancedIsOn() ) then begin
    if( RateLineGrid.Focused() ) then
      HardenRateLineGrid()
    else if( XPresValGrid.Focused() ) then
      HardenXPresValGrid();
  end else begin
    if( PresentValueGrid.Focused() ) then
      HardenPresentValueGrid();
  end;
  m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.OnUndo();
var
  Success : boolean;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnUndo' );
  m_UndoBuffer.Undo( a, b, c, cc, xpresvalptr(d), Success );
  if( not Success ) then begin
    MasterLog.Write( LVL_LOW, 'Undo1Click failed to Undo' );
    exit;
  end;
  AllPresentValueData2Grids();
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.OnRedo();
var
  Success : boolean;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnRedo' );
  m_UndoBuffer.Redo( a, b, c, cc, xpresvalptr(d), Success );
  if( not Success ) then begin
    MasterLog.Write( LVL_LOW, 'Redo1Click failed to Redo' );
    exit;
  end;
  AllPresentValueData2Grids();
  SetUnsavedData( true );
end;

function TPresentValueScreen.GetFocusedGrid(): TPersenseGrid;
begin
  GetFocusedGrid := nil;
  if( LumpSumGrid.Focused() ) then
    GetFocusedGrid := LumpSumGrid
  else if( PeriodicGrid.Focused() ) then
    GetFocusedGrid := PeriodicGrid;
  if( AdvancedIsOn() ) then begin
    if( RateLineGrid.Focused() ) then
      GetFocusedGrid := RateLineGrid
    else if( XPresValGrid.Focused() ) then
      GetFocusedGrid := XPresValGrid;
  end else begin
    if( PresentValueGrid.Focused() ) then
      GetFocusedGrid := PresentValueGrid;
  end;
end;

procedure TPresentValueScreen.OnCut();
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnCut' );
  if( LumpSumGrid.Focused() ) then
    LumpSumGridCut()
  else if( PeriodicGrid.Focused() ) then
    PeriodicGridCut();
  if( AdvancedIsOn() ) then begin
    if( RateLineGrid.Focused() ) then
      RateLineGridCut()
    else if( XPresValGrid.Focused() ) then
      XPresValGridCut();
  end else begin
    if( PresentValueGrid.Focused() ) then
      PresentValueGridCut();
  end;
  m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.OnCopy();
var
  Grid: TPersenseGrid;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnCopy' );
  Grid := GetFocusedGrid();
  if( Grid <> nil ) then
    Grid.CopyToClipboard();
end;

procedure TPresentValueScreen.OnPaste();
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnPaste' );
  if( LumpSumGrid.Focused() ) then
    LumpSumGridPaste()
  else if( PeriodicGrid.Focused() ) then
    PeriodicGridPaste();
  if( AdvancedIsOn() ) then begin
    if( RateLineGrid.Focused() ) then
      RateLineGridPaste()
    else if( XPresValGrid.Focused() ) then
      XPresValGridPaste();
  end else begin
    if( PresentValueGrid.Focused() ) then
      PresentValueGridPaste();
  end;
  m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.OnDelete();
var
  Grid: TPersenseGrid;
  DelStart, DelEnd: Integer;
  TheText, First, Second: string;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnDelete' );
  Grid := GetFocusedGrid();
  if( Grid = nil ) then exit;
  if( Grid.EditorMode ) then begin
    TheText := Grid.InplaceEditor.Text;
    DelStart := Grid.InplaceEditor.SelStart;
    DelEnd := DelStart + Grid.InplaceEditor.SelLength;
    if( DelStart = DelEnd ) then begin
      First := copy( TheText, 0, DelStart );
      Second := copy( TheText, DelStart+2, length(TheText)-DelStart );
    end else begin
      First := copy( TheText, 0, DelStart );
      Second := copy( TheText, DelEnd+1, length(TheText)-(DelEnd-DelStart) );
    end;
    Grid.InplaceEditor.Text := First + Second;
    Grid.SetEditText( Grid.Col, Grid.Row, Grid.InplaceEditor.Text );
    Grid.InplaceEditor.SelStart := DelStart;
  end else begin
    if( LumpSumGrid.Focused() ) then
      LumpSumGridDelete()
    else if( PeriodicGrid.Focused() ) then
      PeriodicGridDelete();
    if( AdvancedIsOn() ) then begin
      if( RateLineGrid.Focused() ) then
        RateLineGridDelete()
      else if( XPresValGrid.Focused() ) then
        XPresValGridDelete();
    end else begin
      if( PresentValueGrid.Focused() ) then
        PresentValueGridDelete();
    end;
  end;
  m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.OpenFile( var TheFile: TFileIO );
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OpenFile' );
  for i:=1 to maxlines do begin
    LumpSumGrid.EmptyRow( i-1 );
    ZeroLumpSum( lumpsumptr(a[i]) );
    PeriodicGrid.EmptyRow( i-1 );
    ZeroPeriodic( periodicptr(b[i]) );
    RatelineGrid.EmptyRow( i-1 );
    ZeroRateLine( ratelineptr(cc[i]) );
  end;
  for i:=1 to presvallines do begin
    PresentValueGrid.EmptyRow( i-1 );
    ZeroPresVal( presvalptr(c[i]) );
  end;
  XPresValGrid.EmptyRow( 0 );
  ZeroXPresVal( xpresvalptr(d) );
  // now fill with new data
  TheFile.GetPresentValueData( a, b, c, cc, xpresvalptr(d) );
  if( not m_bBackedUpForHelp ) then
    m_FileName := TheFile.GetFileName()
  else
    m_FileName := '';
  m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
  // put the screen in the right mode
  if( pvlfancy <> (TheFile.GetFancyByte()=1) ) then begin
    MainForm.CalcPvlToggleAdvancedOptions1Execute(Self);
  end;
  AllPresentValueData2Grids();
  SetUnsavedData( false );
end;

function TPresentValueScreen.SaveFile( FileName, ScreenName : string ): boolean;
var
  TheFile: TFileIO;
  hFile: integer;
  Position: integer;
  bWarnOnOverwrite: boolean;
  bCancelPressed: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.SaveFile' );
  bWarnOnOverwrite := false;
  if( ScreenName <> '' ) then
    SaveDialog1.Title := 'Save ' + ScreenName + ' As';
  SaveDialog1.Filter := 'Present Value Files (*.pvl)|*.PVL';
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
    FileName := FileName + '.pvl';
  if( not FileExists( FileName ) ) then begin
    hFile := FileCreate( FileName );
    if( hFile = -1 ) then begin
      MasterLog.Write( LVL_MED, 'SaveFile failed to create new file for saving' );
      SaveFile := false;
      exit;
    end;
    FileClose( hFile );
  end else if( bWarnOnOverwrite ) then begin
    MessageBoxWithCancel( 'File Exists, would you like to save over it?', bCancelPressed, DO_OverwriteFile );
    if( bCancelPressed ) then begin
      SaveFile := false;
      exit;
    end;
  end;
  TheFile := TFileIO.Create();
  TheFile.SavePresentValue( FileName, a, b, c, cc, xpresvalptr(d) );
  m_FileName := FileName;
  TheFile.Free();
  SetUnsavedData( false );
  SaveFile := true;
end;

procedure TPresentValueScreen.OnPrint();
var
  yPos: integer;
  PenWidth: integer;
  LumpRowsPrinted, PeriodicRowsPrinted, ThirdRowsPrinted: integer;
  CurrentLumpRow, CurrentPeriodicRow, CurrentThirdRow: integer;
  LumpCellHeight, PeriodicCellHeight, ThirdCellHeight: integer;
  MaxLumpRowCount, MaxPeriodicRowCount, MaxThirdRowCount: integer;
  BoxTop: integer;
  i: integer;
  TextHeight: integer;
  UsablePageWidth: integer;
const
  Margin = 80;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.OnPrint' );
  Printer.BeginDoc();
  Printer.Canvas.Font.Assign( Font );
  Printer.Title := 'Persense Present Value Screen';
  TextHeight := Printer.Canvas.TextHeight( 'Amount' );
  UsablePageWidth := Printer.PageWidth-(2*Margin);
  // the LumpSum and Periodic rows
  MaxLumpRowCount := 0;
  MaxPeriodicRowCount := 0;
  for i:=1 to maxlines do begin
    if( not LumpSumIsEmpty( lumpsumptr(a[i]) ) ) then
      MaxLumpRowCount := i;
    if( not PeriodicIsEmpty( periodicptr(b[i]) ) ) then
      MaxPeriodicRowCount := i;
  end;
  CurrentLumpRow := 0;
  CurrentPeriodicRow := 0;
  yPos := 100;
  while( (CurrentLumpRow<MaxLumpRowCount) or (CurrentPeriodicRow<MaxPeriodicRowCount) ) do begin
    Printer.Canvas.Brush.Color := clWhite;
    if( CurrentLumpRow < MaxLumpRowCount ) then begin
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.05), yPos, 'Single Payments' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.05), yPos+Trunc( TextHeight), 'Date' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.15), yPos+Trunc( TextHeight), 'Amount' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.25), yPos+Trunc( TextHeight), 'Value' );
    end;
    if( CurrentPeriodicRow < MaxPeriodicRowCount ) then begin
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.45), yPos, 'Periodic Payments' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.45), yPos+Trunc( TextHeight), 'From' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.55), yPos+Trunc( TextHeight), 'Through' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.63), yPos+Trunc( TextHeight), 'PerYr' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.7), yPos+Trunc( TextHeight), 'Amount' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.8), yPos+Trunc( TextHeight), 'COLA%' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.9), yPos+Trunc( TextHeight), 'Value' );
    end;
    yPos := yPos + Trunc(2.5*TextHeight);
    if( CurrentLumpRow < MaxLumpRowCount ) then begin
      LumpCellHeight := Printer.PageHeight-yPos-2*Margin;
      LumpSumGrid.Print( CurrentLumpRow, yPos, Trunc(UsablePageWidth*0.01), Trunc(UsablePageWidth*0.35), LumpCellHeight, MaxLumpRowCount, LumpRowsPrinted );
      CurrentLumpRow := CurrentLumpRow + LumpRowsPrinted;
    end else
      LumpCellHeight := 0;
    if( CurrentPeriodicRow < MaxPeriodicRowCount ) then begin
      PeriodicCellHeight := Printer.PageHeight-yPos-2*Margin;
      PeriodicGrid.Print( CurrentPeriodicRow, yPos, Trunc(UsablePageWidth*0.41), Trunc(UsablePageWidth*0.6), PeriodicCellHeight, MaxPeriodicRowCount, PeriodicRowsPrinted );
      CurrentPeriodicRow := CurrentPeriodicRow + PeriodicRowsPrinted;
    end else
      PeriodicCellHeight := 0;
    // possibly set up for new page
    if( (CurrentLumpRow < MaxLumpRowCount) or (CurrentPeriodicRow < MaxPeriodicRowCount) ) then begin
      Printer.NewPage();
      yPos := 100;
    end else if( PeriodicCellHeight > LumpCellHeight ) then
      yPos := yPos + PeriodicCellHeight
    else
      yPos := yPos + LumpCellHeight;
  end;
  // put in a page break if we need one
  if( (Printer.PageHeight-yPos) < 5*TextHeight ) then begin
    Printer.NewPage();
    yPos := 100;
  end else
    yPos := yPos + TextHeight;
  // now print out the rest
  if( pvlfancy ) then begin
    // the XPresVal stuff
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.65), yPos, 'Value computed with rate table at left:' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.7), yPos+TextHeight, 'Interest' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.55), yPos+2*TextHeight, 'As of' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.7), yPos+2*TextHeight, 'Computation' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.9), yPos+2*TextHeight, 'Total Value' );
    XPresValGrid.Print( 0, yPos+3*TextHeight, Trunc(UsablePageWidth*0.52), Trunc(UsablePageWidth*0.48), TextHeight, 1, PeriodicRowsPrinted );
    // now the RateLine Grid
    MaxThirdRowCount := 0;
    for i:=1 to maxlines do begin
      if( not RateLineIsEmpty( ratelineptr(cc[i]) ) ) then
        MaxThirdRowCount := i;
    end;
    CurrentThirdRow := 0;
    while( CurrentThirdRow < MaxThirdRowCount ) do begin
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.05), yPos, 'Effective' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.2), yPos, 'True' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.3), yPos, 'Loan' );
      yPos := yPos + TextHeight;
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.05), yPos, 'Date' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.2), yPos, 'Rate %' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.3), yPos, 'Rate %' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.4), yPos, 'Yield %' );
      yPos := yPos + TextHeight;
      ThirdCellHeight := Printer.PageHeight-yPos-2*Margin;
      RateLineGrid.Print( CurrentThirdRow, yPos, Trunc(UsablePageWidth*0.01), Trunc(UsablePageWidth*0.48), ThirdCellHeight, MaxThirdRowCount, ThirdRowsPrinted );
      CurrentThirdRow := CurrentThirdRow + ThirdRowsPrinted;
      if( CurrentThirdRow < MaxThirdRowCount ) then begin
        Printer.NewPage();
        yPos := 100;
      end;
    end;
  end else begin
    // now the PresVal Grid
    MaxThirdRowCount := 0;
    for i:=1 to presvallines do begin
      if( not PresValIsEmpty( presvalptr(c[i]) ) ) then
        MaxThirdRowCount := i;
    end;
    CurrentThirdRow := 0;
    while( CurrentThirdRow < MaxThirdRowCount ) do begin
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.3), yPos, 'Present Value:' );
      yPos := yPos + TextHeight;
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.4), yPos, 'True' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.5), yPos, 'Loan' );
      yPos := yPos + TextHeight;
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.3), yPos, 'As Of' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.4), yPos, 'Rate %' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.5), yPos, 'Rate %' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.6), yPos, 'Yield %' );
      Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.7), yPos, 'Value' );
      yPos := yPos + Trunc(1.5*TextHeight);
      ThirdCellHeight := Printer.PageHeight-yPos-2*Margin;
      PresentValueGrid.Print( CurrentThirdRow, yPos, Trunc(UsablePageWidth*0.25), Trunc(UsablePageWidth*0.5), ThirdCellHeight, MaxThirdRowCount, ThirdRowsPrinted );
      CurrentThirdRow := CurrentThirdRow + ThirdRowsPrinted;
      if( CurrentThirdRow < MaxThirdRowCount ) then begin
        Printer.NewPage();
        yPos := 100;
      end;
    end;
  end;
  Printer.EndDoc();
end;

procedure TPresentValueScreen.SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont );
begin
  LumpSumGrid.CellBackgroundColor := CellColour;
  LumpSumGrid.OutpCellBackgroundColor := OutpCellColour;
  LumpSumGrid.SelectedCellColor := SelectedColour;
  LumpSumGrid.CellFont := CellFont;
  LumpSumGrid.OutpCellFont := OutpCellFont;
  LumpSumGrid.Repaint();
  PeriodicGrid.CellBackgroundColor := CellColour;
  PeriodicGrid.OutpCellBackgroundColor := OutpCellColour;
  PeriodicGrid.SelectedCellColor := SelectedColour;
  PeriodicGrid.CellFont := CellFont;
  PeriodicGrid.OutpCellFont := OutpCellFont;
  PeriodicGrid.Repaint();
  PresentValueGrid.CellBackgroundColor := CellColour;
  PresentValueGrid.OutpCellBackgroundColor := OutpCellColour;
  PresentValueGrid.SelectedCellColor := SelectedColour;
  PresentValueGrid.CellFont := CellFont;
  PresentValueGrid.OutpCellFont := OutpCellFont;
  PresentValueGrid.Repaint();
  RatelineGrid.CellBackgroundColor := CellColour;
  RatelineGrid.OutpCellBackgroundColor := OutpCellColour;
  RatelineGrid.SelectedCellColor := SelectedColour;
  RatelineGrid.CellFont := CellFont;
  RatelineGrid.OutpCellFont := OutpCellFont;
  RatelineGrid.Repaint();
  XPresValGrid.CellBackgroundColor := CellColour;
  XPresValGrid.OutpCellBackgroundColor := OutpCellColour;
  XPresValGrid.SelectedCellColor := SelectedColour;
  XPresValGrid.CellFont := CellFont;
  XPresValGrid.OutpCellFont := OutpCellFont;
  XPresValGrid.Repaint();
  FormResize( Self );
end;

procedure TPresentValueScreen.FormResize(Sender: TObject);
const
  LabelMargin = 10;
  CellBorderHeight = 4;
var
  BottomGroupY: integer;
begin
  // Label boxes
  GroupBox1.Left := 0;
  GroupBox1.Width := Trunc(Width*0.4);
  SingleDateLabel.Left := Trunc(GroupBox1.Width*0.05);
  SingleAmountLabel.Left := Trunc(GroupBox1.Width*0.33);
  SingleValueLabel.Left := Trunc(GroupBox1.Width*0.63);
  GroupBox2.left := Trunc(Width*0.41);
  GroupBox2.Width := Width-GroupBox2.left-10;
  PeriodicFromLabel.Left := Trunc(GroupBox2.Width*0.05);
  PeriodicThroughLabel.Left := Trunc(GroupBox2.Width*0.20);
  PeriodicPerYrLabel.Left := Trunc(GroupBox2.Width*0.36);
  PeriodicAmountLabel.Left := Trunc(GroupBox2.Width*0.46);
  PeriodicColaLabel.Left := Trunc(GroupBox2.Width*0.64);
  PeriodicValueLabel.Left := Trunc(GroupBox2.Width*0.77);
  // top grids
  LumpSumGrid.Left := 0;
  LumpSumGrid.Width := Trunc(Width*0.4);
  PeriodicGrid.Left := Trunc(Width*0.41);
  PeriodicGrid.Width := Width-PeriodicGrid.Left-10;
  BottomGroupY := Height - (PresentValueGrid.DefaultRowHeight*presvallines) - GroupBox3.Height-110;
  LumpSumGrid.Height := BottomGroupY - (GroupBox1.Top+GroupBox1.Height) - 10;
  PeriodicGrid.Height := BottomGroupY - (GroupBox1.Top+GroupBox1.Height) - 10;
  // plain group
  PlainGroup.Left := Trunc(Width*0.2);
  PlainGroup.Top := BottomGroupY+10;
  PlainGroup.Width := Width-Trunc(Width*0.4);
  PlainGroup.Height := Height-BottomGroupY-70;
  GroupBox3.Left := 10;
  GroupBox3.Width := PlainGroup.Width-20;
  PresentValueGrid.Left := 10;
  PresentValueGrid.Width := PlainGroup.Width-20;
  PresentValueGrid.Height := (PresentValueGrid.DefaultRowHeight*presvallines) + CellBorderHeight;
  Label1.Left := Trunc(PresentValueGrid.Width*0.05);
  Label2.Left := Trunc(PresentValueGrid.Width*0.25);
  Label3.Left := Trunc(PresentValueGrid.Width*0.45);
  Label4.Left := Trunc(PresentValueGrid.Width*0.65);
  Label5.Left := Trunc(PresentValueGrid.Width*0.85);
  // AdvancedGroup
  AdvancedGroup.Left := 5;
  AdvancedGroup.Top := BottomGroupY;
  AdvancedGroup.Width := Width-20;
  AdvancedGroup.Height := Height-BottomGroupY-30;
  GroupBox4.Left := 10;
  GroupBox4.Width := Trunc(AdvancedGroup.Width*0.5)-GroupBox4.Left;
  Label6.Left := Trunc(GroupBox4.Width*0.05);
  Label7.Left := Trunc(GroupBox4.Width*0.3);
  Label8.Left := Trunc(GroupBox4.Width*0.55);
  Label9.Left := Trunc(GroupBox4.Width*0.8);
  GroupBox5.left := Trunc(AdvancedGroup.Width*0.52);
  GroupBox5.Width := AdvancedGroup.Width - GroupBox5.Left-10;
  Label11.Left := Trunc(GroupBox5.Width*0.05);
  Label12.Left := Trunc(GroupBox5.Width*0.38);
  Label13.Left := Trunc(GroupBox5.Width*0.7);
  RateLineGrid.Left := 10;
  RateLineGrid.Width := Trunc(AdvancedGroup.Width*0.5)-RateLineGrid.Left;
  RateLineGrid.Height := AdvancedGroup.Height - RateLineGrid.Top - 10;
  XPresValGrid.Left := Trunc(AdvancedGroup.Width*0.52);
  XPresValGrid.Width := AdvancedGroup.Width - XPresValGrid.Left-10;
  XPresValGrid.Height := XPresValGrid.DefaultRowHeight + CellBorderHeight;
end;

function TPresentValueScreen.IsBackedUpForHelp(): boolean;
begin
  IsBackedUpForHelp := m_bBackedUpForHelp;
end;

procedure TPresentValueScreen.BackupForHelpSystem();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.BackupForHelpSystem' );
  m_bBackedUpForHelp := true;
  m_BackupFileName := m_FileName;
  m_BackupCaption := Caption;
  m_bBackupUnsaved := HasUnsavedData();
  for i:=1 to maxlines do begin
    m_HelpBackupLumpSum[i]^ := a[i]^;
    m_HelpBackupPeriodic[i]^ := b[i]^;
    m_HelpBackupRateLine[i]^ := cc[i]^;
  end;
  for i:=1 to presvallines do
    m_HelpBackupPresVal[i]^ := c[i]^;
  m_HelpBackupXPresVal := d^;
  m_FileName := '';
  Caption := 'Present Value Help';
  SetUnsavedData( false );
end;

procedure TPresentValueScreen.RestoreFromHelpSystem();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.RestoreFromHelpSystem' );
  m_bBackedUpForHelp := false;
  for i:=1 to maxlines do begin
    a[i]^ := m_HelpBackupLumpSum[i]^;
    b[i]^ := m_HelpBackupPeriodic[i]^;
    cc[i]^ := m_HelpBackupRateLine[i]^;
  end;
  for i:=1 to presvallines do
    c[i]^ := m_HelpBackupPresVal[i]^;
  d^ := m_HelpBackupXPresVal;
  AllPresentValueData2Grids();
  m_FileName := m_BackupFileName;
  Caption := m_BackupCaption;
  SetUnsavedData( m_bBackupUnsaved );
end;

procedure TPresentValueScreen.LumpSumGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
var
  SelectedRect: TGridRect;
begin
  inherited;
  PeriodicGrid.SetFocus();
  SelectedRect := PeriodicGrid.Selection;
  SelectedRect.Left := 5;
  SelectedRect.Right := 5;
  PeriodicGrid.Selection := SelectedRect;
end;

procedure TPresentValueScreen.LumpSumGridRightAfterGrid(Sender: TObject; var Default: Boolean);
var
  SelectedRect: TGridRect;
begin
  inherited;
  PeriodicGrid.SetFocus();
  SelectedRect := PeriodicGrid.Selection;
  SelectedRect.Left := 0;
  SelectedRect.Right := 0;
  PeriodicGrid.Selection := SelectedRect;
end;

procedure TPresentValueScreen.PeriodicGridRightAfterGrid(Sender: TObject; var Default: Boolean);
var
  SelectedRect: TGridRect;
begin
  inherited;
  LumpSumGrid.SetFocus();
  SelectedRect := LumpSumGrid.Selection;
  SelectedRect.Left := 0;
  SelectedRect.Right := 0;
  LumpSumGrid.Selection := SelectedRect;
end;

procedure TPresentValueScreen.PeriodicGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
var
  SelectedRect: TGridRect;
begin
  inherited;
  LumpSumGrid.SetFocus();
  SelectedRect := LumpSumGrid.Selection;
  SelectedRect.Left := 2;
  SelectedRect.Right := 2;
  LumpSumGrid.Selection := SelectedRect;
end;

procedure TPresentValueScreen.LumpSumGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  if( AdvancedIsOn() ) then
    RateLineGrid.SetFocus()
  else
    PresentValueGrid.SetFocus();
end;

procedure TPresentValueScreen.PeriodicGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  if( AdvancedIsOn() ) then
    RateLineGrid.SetFocus()
  else
    PresentValueGrid.SetFocus();
end;

procedure TPresentValueScreen.PresentValueGridUpBeforeGrid( Sender: TObject);
begin
  inherited;
  PeriodicGrid.SetFocus();
end;

procedure TPresentValueScreen.RatelineGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
var
  SelectedRect: TGridRect;
begin
  inherited;
  XPresValGrid.SetFocus();
  SelectedRect := XPresValGrid.Selection;
  SelectedRect.Left := 2;
  SelectedRect.Right := 2;
  XPresValGrid.Selection := SelectedRect;
end;

procedure TPresentValueScreen.RatelineGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  PeriodicGrid.SetFocus();
end;

procedure TPresentValueScreen.RatelineGridRightAfterGrid(Sender: TObject; var Default: Boolean);
var
  SelectedRect: TGridRect;
begin
  inherited;
  XPresValGrid.SetFocus();
  SelectedRect := XPresValGrid.Selection;
  SelectedRect.Left := 0;
  SelectedRect.Right := 0;
  XPresValGrid.Selection := SelectedRect;
end;

procedure TPresentValueScreen.XPresValGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  PeriodicGrid.SetFocus();
end;

procedure TPresentValueScreen.XPresValGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
var
  SelectedRect: TGridRect;
begin
  inherited;
  RateLineGrid.SetFocus();
  SelectedRect := RateLineGrid.Selection;
  SelectedRect.Left := 3;
  SelectedRect.Right := 3;
  RateLineGrid.Selection := SelectedRect;
end;

procedure TPresentValueScreen.LumpSumGridCellBeforeEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String);
begin
  inherited;
  if( ACol = LSMAmountCol ) then begin
    if( a[ARow+1].val0status <> empty ) then
      LumpSumGrid.SetCell( '', LSMValueCol, ARow, empty );
  end else if( ACol = LSMValueCol ) then begin
    if( a[ARow+1].amt0status <> empty ) then
      LumpSumGrid.SetCell( '', LSMAmountCol, ARow, empty );
  end;
end;

procedure TPresentValueScreen.LumpSumGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignLumpSumValues( a, ACol, ARow, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
    SetUnsavedData( true );
  end;
end;

procedure TPresentValueScreen.AssignLumpSumValues( Data: lumpsumarray; ACol, ARow: integer; Value: string; Status: inout );
var
  IsError: boolean;
begin
  case ACol of
    LSMDateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].date := unkdate;
        Data[ARow+1].datestatus := empty;
        LumpSumGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].date := StringFormat2Date( Value, IsError );
        Data[ARow+1].datestatus := Status;
        LumpSumGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    LSMAmountCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].amt0 := 0;
        Data[ARow+1].amt0status := empty;
        LumpSumGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].amt0 := StringFormat2Double( Value, IsError );
        Data[ARow+1].amt0status := Status;
        LumpSumGrid.SetCellHardness( ACol, ARow, Status );
        Data[ARow+1].val0 := 0;
        Data[ARow+1].val0Status := empty;
        LumpSumGrid.SetCell( '', LSMValueCol, ARow, empty );
      end;
    end;
    LSMValueCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].val0 := 0;
        Data[ARow+1].val0status := empty;
        LumpSumGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].val0 := StringFormat2Double( Value, IsError );
        Data[ARow+1].val0status := Status;
        LumpSumGrid.SetCellHardness( ACol, ARow, Status );
        Data[ARow+1].amt0 := 0;
        Data[ARow+1].amt0Status := empty;
        LumpSumGrid.SetCell( '', LSMAmountCol, ARow, empty );
      end;
    end;
  end;
end;

procedure TPresentValueScreen.LumpSumValues2Grid( Data: LumpSumArray; Grid: TPersenseGrid );
var
  pLumpSum: lumpsumptr;
  i: integer;
  NumStr: string;
begin
  for i:=1 to maxlines do begin
    pLumpSum := lumpsumptr(Data[i]);
    if( not( (i > Grid.GetUsedRowCount()) and LumpSumIsEmpty( pLumpSum )) ) then begin
      if( pLumpSum.datestatus <> empty ) then begin
        NumStr := DateToStr( pLumpSum.date );
        LumpSumGrid.SetCell( NumStr, LSMDateCol, i-1, pLumpSum.datestatus );
      end else
        LumpSumGrid.SetCell( '', LSMDateCol, i-1, empty );

      if( pLumpSum.amt0status <> empty ) then begin
        NumStr := FloatToStr( pLumpSum.amt0 );
        LumpSumGrid.SetCell( NumStr, LSMAmountCol, i-1, pLumpSum.amt0status );
      end else
        LumpSumGrid.SetCell( '', LSMAmountCol, i-1, empty );

      if( pLumpSum.val0status <> empty ) then begin
        NumStr := FloatToStr( pLumpSum.val0 );
        LumpSumGrid.SetCell( NumStr, LSMValueCol, i-1, pLumpSum.val0status );
      end else
        LumpSumGrid.SetCell( '', LSMValueCol, i-1, empty );
    end;
  end;
end;

procedure TPresentValueScreen.PeriodicGridCellBeforeEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String);
begin
  inherited;
  if( ACol <> PERPerYrCol ) then begin
    if( b[ARow+1].peryrstatus = empty ) then begin
      b[ARow+1].peryrstatus := defp;
      b[ARow+1].peryr := df.c.peryr;
      PeriodicGrid.SetEditText( PERPerYrCol, ARow, IntToStr( df.c.peryr) );
    end;
  end;
end;

procedure TPresentValueScreen.PeriodicGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignPeriodicValues( b, ACol, ARow, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
    SetUnsavedData( true );
  end;
end;

procedure TPresentValueScreen.AssignPeriodicValues( Data: periodicarray; ACol, ARow: integer; Value: string; Status: inout );
var
  IsError: boolean;
begin
  case ACol of
    PERFromDateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].fromdate := unkdate;
        Data[ARow+1].fromdatestatus := empty;
        PeriodicGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].fromdate := StringFormat2Date( Value, IsError );
        Data[ARow+1].fromdatestatus := Status;
        PeriodicGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    PERToDateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].todate := unkdate;
        Data[ARow+1].todatestatus := empty;
        PeriodicGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].todate := StringFormat2Date( Value, IsError );
        Data[ARow+1].todatestatus := Status;
        PeriodicGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    PERPerYrCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].peryr := 0;
        Data[ARow+1].peryrstatus := empty;
        PeriodicGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].peryr := StringFormat2Int( Value, IsError );
        Data[ARow+1].peryrstatus := Status;
        PeriodicGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    PERAmountCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].amtn := 0;
        Data[ARow+1].amtnstatus := empty;
        PeriodicGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].amtn := StringFormat2Double( Value, IsError );
        Data[ARow+1].amtnstatus := Status;
        PeriodicGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    PERCOLACol: begin
      if( Value = '' ) then begin
        Data[ARow+1].cola := 0;
        Data[ARow+1].colastatus := empty;
        PeriodicGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].cola := (StringFormat2Double( Value, IsError )/100.0);
        Data[ARow+1].colastatus := Status;
        PeriodicGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    PERValueCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].valn := 0;
        Data[ARow+1].valnstatus := empty;
        PeriodicGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].valn := StringFormat2Double( Value, IsError );
        Data[ARow+1].valnstatus := Status;
        PeriodicGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
  end;
end;

procedure TPresentValueScreen.PeriodicValues2Grid( Data: periodicarray; Grid: TPersenseGrid );
var
  pPeriodic: periodicptr;
  i: integer;
  NumStr: string;
begin
  for i:=1 to maxlines do begin
    pPeriodic := periodicptr(Data[i]);
    if( not( (i > Grid.GetUsedRowCount()) and PeriodicIsEmpty( pPeriodic )) ) then begin
      if( pPeriodic.fromdatestatus <> empty ) then begin
        NumStr := DateToStr( pPeriodic.fromdate );
        PeriodicGrid.SetCell( NumStr, PERFromDateCol, i-1, pPeriodic.fromdatestatus );
      end else
        PeriodicGrid.SetCell( '', PERFromDateCol, i-1, empty );

      if( pPeriodic.todatestatus <> empty ) then begin
        NumStr := DateToStr( pPeriodic.todate );
        PeriodicGrid.SetCell( NumStr, PERToDateCol, i-1, pPeriodic.todatestatus );
      end else
        PeriodicGrid.SetCell( '', PERToDateCol, i-1, empty );

      if( pPeriodic.peryrstatus <> empty ) then begin
        NumStr := IntToStr( pPeriodic.peryr );
        PeriodicGrid.SetCell( NumStr, PERPerYrCol, i-1, pPeriodic.peryrstatus );
      end else
        PeriodicGrid.SetCell( '', PERPerYrCol, i-1, empty );

      if( pPeriodic.amtnstatus <> empty ) then begin
        NumStr := FloatToStr( pPeriodic.amtn );
        PeriodicGrid.SetCell( NumStr, PERAmountCol, i-1, pPeriodic.amtnstatus );
      end else
        PeriodicGrid.SetCell( '', PERAmountCol, i-1, empty );

      if( pPeriodic.colastatus <> empty ) then begin
        NumStr := FloatToStr( pPeriodic.cola*100 );
        PeriodicGrid.SetCell( NumStr, PERCOLACol, i-1, pPeriodic.colastatus );
      end else
        PeriodicGrid.SetCell( '', PERCOLACol, i-1, empty );

      if( pPeriodic.valnstatus <> empty ) then begin
        NumStr := FloatToStr( pPeriodic.valn );
        PeriodicGrid.SetCell( NumStr, PERValueCol, i-1, pPeriodic.valnstatus );
      end else
        PeriodicGrid.SetCell( '', PERValueCol, i-1, empty );
    end;
  end;
end;

procedure TPresentValueScreen.PresentValueGridCellAfterEdit(
  Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignPresentValueValues( c, ACol, ARow, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
    SetUnsavedData( true );
  end;
end;

procedure TPresentValueScreen.AssignPresentValueValues( Data: presvalarray; ACol, ARow: integer; Value: string; Status: inout );
var
  IsError: boolean;
  Hold: double;
  NumStr: string;
begin
  case ACol of
    PRVAsOfCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].asof := unkdate;
        Data[ARow+1].asofstatus := empty;
        PresentValueGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].asof := StringFormat2Date( Value, IsError );
        Data[ARow+1].asofstatus := Status;
        PresentValueGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    PRVTrueRateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].r.rate := 0;
        Data[ARow+1].r.status := empty;
        PresentValueGrid.SetCell( '', PRVTrueRateCol, ARow, empty );
        PresentValueGrid.SetCell( '', PRVLoanRateCol, ARow, empty );
        PresentValueGrid.SetCell( '', PRVYieldCol, ARow, empty );
      end else begin
        Data[ARow+1].r.rate := StringFormat2Double( Value, IsError )/100;
        Data[ARow+1].r.status := fromrate;
        Data[ARow+1].r.peryr := df.c.peryr;
        PresentValueGrid.SetCellHardness( ACol, ARow, inp );
        Hold := YieldFromRate( Data[ARow+1].r.rate, df.c.peryr );
        NumStr := FloatToStr( Hold*100 );
        PresentValueGrid.SetCell( NumStr, PRVLoanRateCol, ARow, outp );
        Hold := YieldFromRate( Data[ARow+1].r.rate, 1 );
        NumStr := FloatToStr( Hold*100 );
        PresentValueGrid.SetCell( NumStr, PRVYieldCol, ARow, outp );
      end;
    end;
    PRVLoanRateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].r.rate := 0;
        Data[ARow+1].r.status := empty;
        PresentValueGrid.SetCell( '', PRVTrueRateCol, ARow, empty );
        PresentValueGrid.SetCell( '', PRVLoanRateCol, ARow, empty );
        PresentValueGrid.SetCell( '', PRVYieldCol, ARow, empty );
      end else begin
        Hold := StringFormat2Double( Value, IsError )/100;
        Data[ARow+1].r.rate := RateFromYield( Hold, df.c.peryr );
        Data[ARow+1].r.status := fromapr;
        Data[ARow+1].r.peryr := df.c.peryr;
        PresentValueGrid.SetCellHardness( ACol, ARow, inp );
        Hold := Data[ARow+1].r.rate;
        NumStr := FloatToStr( Hold*100 );
        PresentValueGrid.SetCell( NumStr, PRVTrueRateCol, ARow, outp );
        Hold := YieldFromRate( Data[ARow+1].r.rate, 1 );
        NumStr := FloatToStr( Hold*100 );
        PresentValueGrid.SetCell( NumStr, PRVYieldCol, ARow, outp );
      end;
    end;
    PRVYieldCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].r.rate := 0;
        Data[ARow+1].r.status := empty;
        PresentValueGrid.SetCell( '', PRVTrueRateCol, ARow, empty );
        PresentValueGrid.SetCell( '', PRVLoanRateCol, ARow, empty );
        PresentValueGrid.SetCell( '', PRVYieldCol, ARow, empty );
      end else begin
        Hold := StringFormat2Double( Value, IsError )/100;
        Data[ARow+1].r.rate := RateFromYield( Hold, 1 );
        Data[ARow+1].r.status := fromyield;
        Data[ARow+1].r.peryr := df.c.peryr;
        PresentValueGrid.SetCellHardness( ACol, ARow, inp );
        Hold := Data[ARow+1].r.rate;
        NumStr := FloatToStr( Hold*100 );
        PresentValueGrid.SetCell( NumStr, PRVTrueRateCol, ARow, outp );
        Hold := YieldFromRate( Data[ARow+1].r.rate, df.c.peryr );
        NumStr := FloatToStr( Hold*100 );
        PresentValueGrid.SetCell( NumStr, PRVLoanRateCol, ARow, outp );
      end;
    end;
    PRVValueCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].sumvalue := 0;
        Data[ARow+1].sumvaluestatus := empty;
        PresentValueGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].sumvalue := StringFormat2Double( Value, IsError );
        Data[ARow+1].sumvaluestatus := Status;
        PresentValueGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
  end;
end;

procedure TPresentValueScreen.PresentValueValues2Grid( Data: presvalarray; Grid: TPersenseGrid );
var
  pPresVal: presvalptr;
  i: integer;
  NumStr: string;
begin
  for i:=1 to presvallines do begin
    pPresVal := presvalptr(Data[i]);
    if( not( (i > Grid.GetUsedRowCount()) and PresValIsEmpty( pPresVal )) ) then begin
      if( pPresVal.asofstatus <> empty ) then begin
        NumStr := DateToStr( pPresVal.asof );
        PresentValueGrid.SetCell( NumStr, PRVAsOfCol, i-1, pPresVal.asofstatus );
      end else
        PresentValueGrid.SetCell( '', PRVAsOfCol, i-1, empty );
      if( pPresVal.r.status <> empty ) then begin
        NumStr := FloatToStr( pPresVal.r.rate*100 );
        if( pPresVal.r.status = fromrate ) then
          PresentValueGrid.SetCell( NumStr, PRVTrueRateCol, i-1, inp )
        else
          PresentValueGrid.SetCell( NumStr, PRVTrueRateCol, i-1, outp );
        NumStr := FloatToStr( YieldFromRate( pPresVal.r.rate, df.c.peryr )*100 );
        if( pPresVal.r.status = fromapr ) then
          PresentValueGrid.SetCell( NumStr, PRVLoanRateCol, i-1, inp )
        else
          PresentValueGrid.SetCell( NumStr, PRVLoanRateCol, i-1, outp );
        NumStr := FloatToStr( YieldFromRate( pPresVal.r.rate, 1 )*100 );
        if( pPresVal.r.status = fromyield ) then
          PresentValueGrid.SetCell( NumStr, PRVYieldCol, i-1, inp )
        else
          PresentValueGrid.SetCell( NumStr, PRVYieldCol, i-1, outp );
      end else begin
        PresentValueGrid.SetCell( '', PRVTrueRateCol, i-1, empty );
        PresentValueGrid.SetCell( '', PRVLoanRateCol, i-1, empty );
        PresentValueGrid.SetCell( '', PRVYieldCol, i-1, empty );
      end;
      if( pPresVal.sumvaluestatus <> empty ) then begin
        NumStr := FloatToStr( pPresVal.sumvalue );
        PresentValueGrid.SetCell( NumStr, PRVValueCol, i-1, pPresVal.sumvaluestatus );
      end else
        PresentValueGrid.SetCell( '', PRVValueCol, i-1, empty );
    end;
  end;
end;

procedure TPresentValueScreen.RatelineGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignRateLineValues( cc, ACol, ARow, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
    SetUnsavedData( true );
  end;
end;

procedure TPresentValueScreen.AssignRateLineValues( Data: ratelinearray; ACol, ARow: integer; Value: string; Status: inout );
var
  IsError: boolean;
  Hold: double;
  NumStr: string;
begin
  case ACol of
    RTLDateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].date := unkdate;
        Data[ARow+1].datestatus := empty;
        RateLineGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        Data[ARow+1].date := StringFormat2Date( Value, IsError );
        Data[ARow+1].datestatus := Status;
        RateLineGrid.SetCellHardness( ACol, ARow, Status );
      end;
    end;
    RTLTrueRateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].r.rate := 0;
        Data[ARow+1].r.status := empty;
        RateLineGrid.SetCell( '', RTLTrueRateCol, ARow, empty );
        RateLineGrid.SetCell( '', RTLLoanRateCol, ARow, empty );
        RateLineGrid.SetCell( '', RTLYieldCol, ARow, empty );
      end else begin
        Data[ARow+1].r.rate := StringFormat2Double( Value, IsError )/100;
        Data[ARow+1].r.status := fromrate;
        Data[ARow+1].r.peryr := df.c.peryr;
        RateLineGrid.SetCellHardness( ACol, ARow, inp );
        if( d.simple = true ) then begin
          RateLineGrid.SetCell( Value, RTLLoanRateCol, ARow, inp );
          RateLineGrid.SetCell( Value, RTLYieldCol, ARow, inp );
        end else begin
          Hold := YieldFromRate( Data[ARow+1].r.rate, df.c.peryr );
          NumStr := FloatToStr( Hold*100 );
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, ARow, outp );
          Hold := YieldFromRate( Data[ARow+1].r.rate, 1 );
          NumStr := FloatToStr( Hold*100 );
          RateLineGrid.SetCell( NumStr, RTLYieldCol, ARow, outp );
        end;
      end;
    end;
    RTLLoanRateCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].r.rate := 0;
        Data[ARow+1].r.status := empty;
        RateLineGrid.SetCell( '', RTLTrueRateCol, ARow, empty );
        RateLineGrid.SetCell( '', RTLLoanRateCol, ARow, empty );
        RateLineGrid.SetCell( '', RTLYieldCol, ARow, empty );
      end else begin
        Hold := StringFormat2Double( Value, IsError )/100;
        if( d.simple = true ) then
          Data[ARow+1].r.rate := StringFormat2Double( Value, IsError )/100
        else
          Data[ARow+1].r.rate := RateFromYield( Hold, df.c.peryr );
        Data[ARow+1].r.status := fromapr;
        Data[ARow+1].r.peryr := df.c.peryr;
        RateLineGrid.SetCellHardness( ACol, ARow, inp );
        if( d.simple = true ) then begin
          RateLineGrid.SetCell( Value, RTLTrueRateCol, ARow, inp );
          RateLineGrid.SetCell( Value, RTLYieldCol, ARow, inp );
        end else begin
          Hold := Data[ARow+1].r.rate;
          NumStr := FloatToStr( Hold*100 );
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, ARow, outp );
          Hold := YieldFromRate( Data[ARow+1].r.rate, 1 );
          NumStr := FloatToStr( Hold*100 );
          RateLineGrid.SetCell( NumStr, RTLYieldCol, ARow, outp );
        end;
      end;
    end;
    RTLYieldCol: begin
      if( Value = '' ) then begin
        Data[ARow+1].r.rate := 0;
        Data[ARow+1].r.status := empty;
        RateLineGrid.SetCell( '', RTLTrueRateCol, ARow, empty );
        RateLineGrid.SetCell( '', RTLLoanRateCol, ARow, empty );
        RateLineGrid.SetCell( '', RTLYieldCol, ARow, empty );
      end else begin
        Hold := StringFormat2Double( Value, IsError )/100;
        if( d.simple = true ) then
          Data[ARow+1].r.rate := StringFormat2Double( Value, IsError )/100
        else
          Data[ARow+1].r.rate := RateFromYield( Hold, 1 );
        Data[ARow+1].r.status := fromyield;
        Data[ARow+1].r.peryr := df.c.peryr;
        RateLineGrid.SetCellHardness( ACol, ARow, inp );
        if( d.simple = true ) then begin
          RateLineGrid.SetCell( Value, RTLTrueRateCol, ARow, inp );
          RateLineGrid.SetCell( Value, RTLLoanRateCol, ARow, inp );
        end else begin
          Hold := Data[ARow+1].r.rate;
          NumStr := FloatToStr( Hold*100 );
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, ARow, outp );
          Hold := YieldFromRate( Data[ARow+1].r.rate, df.c.peryr );
          NumStr := FloatToStr( Hold*100 );
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, ARow, outp );
        end;
      end;
    end;
  end;
end;

procedure TPresentValueScreen.RateLineValues2Grid( Data: ratelinearray; Grid: TPersenseGrid );
var
  pRateLine: ratelineptr;
  i: integer;
  NumStr: string;
  Origstr: string;
begin
  for i:=1 to maxlines do begin
    pRateLine := ratelineptr(Data[i]);
    if( not( (i > Grid.GetUsedRowCount()) and RateLineIsEmpty( pRateLine )) ) then begin
      if( (pRateLine.datestatus <> empty) and (i<>1) ) then begin
        NumStr := DateToStr( pRateLine.date );
        RateLineGrid.SetCell( NumStr, RTLDateCol, i-1, pRateLine.datestatus );
      end else
        RateLineGrid.SetCell( '', RTLDateCol, i-1, empty );

      if( pRateLine.r.status <> empty ) then begin
        NumStr := FloatToStr( pRateLine.r.rate*100 );
        OrigStr := NumStr;
        if( pRateLine.r.status = fromrate ) then
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, i-1, inp )
        else if( d.simple = true ) then
          RateLineGrid.SetCell( OrigStr, RTLTrueRateCol, i-1, inp )
        else
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, i-1, outp );
        NumStr := FloatToStr( YieldFromRate( pRateLine.r.rate, df.c.peryr )*100 );
        if( (pRateLine.r.status = fromapr) and (d.simple=false) ) then
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, i-1, inp )
        else if( d.simple = true ) then
          RateLineGrid.SetCell( OrigStr, RTLLoanRateCol, i-1, inp )
        else
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, i-1, outp );
        NumStr := FloatToStr( YieldFromRate( pRateLine.r.rate, 1 )*100 );
        if( (pRateLine.r.status = fromyield) and (d.simple=false) ) then
          RateLineGrid.SetCell( NumStr, RTLYieldCol, i-1, inp )
        else if( d.simple = true ) then
          RateLineGrid.SetCell( OrigStr, RTLYieldCol, i-1, inp )
        else
          RateLineGrid.SetCell( NumStr, RTLYieldCol, i-1, outp );
      end else begin
        RateLineGrid.SetCell( '', RTLTrueRateCol, i-1, empty );
        RateLineGrid.SetCell( '', RTLLoanRateCol, i-1, empty );
        RateLineGrid.SetCell( '', RTLYieldCol, i-1, empty );
      end;
    end;
  end;
end;

procedure TPresentValueScreen.XPresValGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignXPresValValues( xpresvalptr(d), ACol, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( a, b, c, cc, xpresvalptr(d) );
    SetUnsavedData( true );
  end;
end;

procedure TPresentValueScreen.AssignXPresValValues( Data: xpresvalptr; ACol: integer; Value: string; Status: inout );
var
  IsError: boolean;
begin
  case ACol of
    XPRAsOfCol: begin
      if( Value = '' ) then begin
        Data.xasof := unkdate;
        Data.xasofstatus := empty;
        XPresValGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        Data.xasof := StringFormat2Date( Value, IsError );
        Data.xasofstatus := Status;
        XPresValGrid.SetCellHardness( ACol, 0, Status );
      end;
    end;
    XPRComputationCol: begin
      if( Value = '' ) then begin
        Data.simplestatus := empty;
        XPresValGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        if( Value = 'Simple' ) then begin
          Data.simple := true;
        end else begin
          Data.simple := false;
        end;
        Data.simplestatus := Status;
        XPresValGrid.SetCellHardness( ACol, 0, Status );
        // since changing the simple value will change the shown rates in
        // the rateline grid, call XPresValValues2Grid to handle the
        // canging values.
        XPresValValues2Grid( Data, XPresValGrid );
      end;
    end;
    XPRValueCol: begin
      if( Value = '' ) then begin
        Data.xvalue := 0;
        Data.xvaluestatus := empty;
        XPresValGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        Data.xvalue := StringFormat2Double( Value, IsError );
        Data.xvaluestatus := Status;
        XPresValGrid.SetCellHardness( ACol, 0, Status );
      end;
    end;
  end;
end;

procedure TPresentValueScreen.XPresValValues2Grid( Data: xpresvalptr; Grid: TPersenseGrid );
var
  NumStr: string;
  i: integer;
begin
  if( Data.xasofstatus <> empty ) then begin
    NumStr := DateToStr( Data.xasof );
    XPresValGrid.SetCell( NumStr, XPRAsOfCol, 0, Data.xasofstatus );
  end else
    XPresValGrid.SetCell( '', XPRAsOfCol, 0, empty );
  if( Data.simplestatus <> empty ) then begin
    for i:=0 to RateLineGrid.GetUsedRowCount()-1 do begin
      if( Data.simple ) then begin
        XPresValGrid.SetCell( 'Simple', XPRComputationCol, 0, Data.simplestatus );
        if( cc[i+1].r.status = fromrate ) then begin
          NumStr := FloatToStr( cc[i+1].r.rate*100 );
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, i, inp );
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, i, inp );
          RateLineGrid.SetCell( NumStr, RTLYieldCol, i, inp );
        end else if( cc[i+1].r.status = fromapr ) then begin
          NumStr := FloatToStr( YieldFromRate( cc[i+1].r.rate, df.c.peryr )*100 );
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, i, inp );
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, i, inp );
          RateLineGrid.SetCell( NumStr, RTLYieldCol, i, inp );
        end else if( cc[i+1].r.status = fromyield ) then begin
          NumStr := FloatToStr( YieldFromRate( cc[i+1].r.rate, 1 )*100 );
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, i, inp );
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, i, inp );
          RateLineGrid.SetCell( NumStr, RTLYieldCol, i, inp );
        end;
      end else begin
        XPresValGrid.SetCell( 'Compound', XPRComputationCol, 0, Data.simplestatus );
        if( cc[i+1].r.status = fromrate ) then begin
          NumStr := FloatToStr( YieldFromRate( cc[i+1].r.rate, df.c.peryr )*100 );
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, i, outp );
          NumStr := FloatToStr( YieldFromRate( cc[i+1].r.rate, 1 )*100 );
          RateLineGrid.SetCell( NumStr, RTLYieldCol, i, outp );
        end else if( cc[i+1].r.status = fromapr ) then begin
          NumStr := FloatToStr( cc[i+1].r.rate*100 );
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, i, outp );
          NumStr := FloatToStr( YieldFromRate( cc[i+1].r.rate, 1 )*100 );
          RateLineGrid.SetCell( NumStr, RTLYieldCol, i, outp );
        end else if( cc[i+1].r.status = fromyield ) then begin
          NumStr := FloatToStr( cc[i+1].r.rate*100 );
          RateLineGrid.SetCell( NumStr, RTLTrueRateCol, i, outp );
          NumStr := FloatToStr( YieldFromRate( cc[i+1].r.rate, df.c.peryr )*100 );
          RateLineGrid.SetCell( NumStr, RTLLoanRateCol, i, outp );
        end;
      end;
    end;
  end else
    XPresValGrid.SetCell( '', XPRComputationCol, 0, empty );
  if( Data.xvaluestatus <> empty ) then begin
    NumStr := FloatToStr( Data.xvalue );
    XPresValGrid.SetCell( NumStr, XPRValueCol, 0, Data.xvaluestatus );
  end else
    XPresValGrid.SetCell( '', XPRValueCol, 0, empty );
end;

procedure TPresentValueScreen.AllPresentValueData2Grids();
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.AllPresentValueData2Grid' );
  LumpSumValues2Grid( a, LumpSumGrid );
  PeriodicValues2Grid( b, PeriodicGrid );
  if( pvlfancy ) then begin
    RateLineValues2Grid( cc, RateLineGrid );
    XPresValValues2Grid( xpresvalptr(d), XPresValGrid );
  end else
    PresentValueValues2Grid( c, PresentValueGrid );
end;

procedure TPresentValueScreen.LumpSumGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  inherited;
  DoCalculation();
  m_UndoBuffer.OverWriteData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PeriodicGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  inherited;
  DoCalculation();
  m_UndoBuffer.OverWriteData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.RatelineGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  inherited;
  DoCalculation();
  m_UndoBuffer.OverWriteData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PresentValueGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  inherited;
  DoCalculation();
  m_UndoBuffer.OverWriteData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.XPresValGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  inherited;
  DoCalculation();
  m_UndoBuffer.OverWriteData( a, b, c, cc, xpresvalptr(d) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.FormClose(Sender: TObject; var Action: TCloseAction);
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.FormClose' );
  inherited;
  OnFormClose( Action );
end;

procedure TPresentValueScreen.LumpSumGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TPresentValueScreen.LumpSumGridCut();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := LumpSumGrid.Selection;
  LumpSumGrid.CutToClipboard();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignLumpSumValues( a, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PeriodicGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TPresentValueScreen.PeriodicGridCut();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PeriodicGrid.Selection;
  PeriodicGrid.CutToClipboard();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignPeriodicValues( b, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PresentValueGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TPresentValueScreen.PresentValueGridCut();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PresentValueGrid.Selection;
  PresentValueGrid.CutToClipboard();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do begin
      if( (ACol<0) or (ARow<0) ) then exit;
      AssignPresentValueValues( c, ACol, ARow, '', inp );
    end;
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.RatelineGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TPresentValueScreen.RateLineGridCut();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := RateLineGrid.Selection;
  RateLineGrid.CutToClipboard();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignRateLineValues( cc, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.XPresValGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TPresentValueScreen.XPresValGridCut();
var
  ACol: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := XPresValGrid.Selection;
  XPresValGrid.CutToClipboard();
  for ACol:=SelectedRect.Left to SelectedRect.Right do
    AssignXPresValValues( xpresvalptr(d), ACol, '', inp );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.LumpSumGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TPresentValueScreen.PeriodicGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TPresentValueScreen.PresentValueGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TPresentValueScreen.RatelineGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TPresentValueScreen.XPresValGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TPresentValueScreen.LumpSumGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TPresentValueScreen.LumpSumGridPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  LumpSumGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignLumpSumValues( a, Col, Row, LumpSumGrid.Cells[Col,Row], LumpSumGrid.GetCellHardness(Col, Row) );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PeriodicGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TPresentValueScreen.PeriodicGridPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  PeriodicGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignPeriodicValues( b, Col, Row, PeriodicGrid.Cells[Col,Row], PeriodicGrid.GetCellHardness(Col, Row) );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PresentValueGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TPresentValueScreen.PresentValueGridPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  PresentValueGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignPresentValueValues( c, Col, Row, PresentValueGrid.Cells[Col,Row], PresentValueGrid.GetCellHardness(Col, Row) );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.RatelineGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TPresentValueScreen.RateLineGridPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  RateLineGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignRateLineValues( cc, Col, Row, RateLineGrid.Cells[Col,Row], RateLineGrid.GetCellHardness(Col, Row) );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.XPresValGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TPresentValueScreen.XPresValGridPaste();
var
  Col: integer;
  PasteRect: TRect;
begin
  XPresValGrid.PasteFromClipboard( PasteRect );
  for Col:=PasteRect.Left to PasteRect.Right do
    AssignXPresValValues( xpresvalptr(d), Col, XPresValGrid.Cells[Col,0], XPresValGrid.GetCellHardness(Col, 0) );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.LumpSumGridDelete();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := LumpSumGrid.Selection;
  LumpSumGrid.DeleteSelected();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignLumpSumValues( a, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PeriodicGridDelete();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PeriodicGrid.Selection;
  PeriodicGrid.DeleteSelected();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignPeriodicValues( b, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PresentValueGridDelete();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PresentValueGrid.Selection;
  PresentValueGrid.DeleteSelected();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignPresentValueValues( c, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.RateLineGridDelete();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := RateLineGrid.Selection;
  RateLineGrid.DeleteSelected();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignRateLineValues( cc, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.XPresValGridDelete();
var
  ACol: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := XPresValGrid.Selection;
  XPresValGrid.DeleteSelected();
  for ACol:=SelectedRect.Left to SelectedRect.Right do
    AssignXPresValValues( xpresvalptr(d), ACol, '', inp );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.HardenLumpSumGrid();
var
  ACol, ARow: integer;
begin
  for ARow:=LumpSumGrid.Selection.Top to LumpSumGrid.Selection.Bottom do begin
    if( ARow <= LumpSumGrid.GetUsedRowCount() ) then begin
      for ACol:=LumpSumGrid.Selection.Left to LumpSumGrid.Selection.Right do begin
        case ACol of
          LSMDateCol: a[ARow+1].datestatus := inp;
          LSMAmountCol: a[ARow+1].amt0status := inp;
          LSMValueCol: a[ARow+1].val0status := inp;
        end;
      end;
    end;
  end;
  LumpSumValues2Grid( a, LumpSumGrid );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.HardenPeriodicGrid();
var
  ACol, ARow: integer;
begin
  for ARow:=PeriodicGrid.Selection.Top to PeriodicGrid.Selection.Bottom do begin
    if( ARow <= PeriodicGrid.GetUsedRowCount() ) then begin
      for ACol:=PeriodicGrid.Selection.Left to PeriodicGrid.Selection.Right do begin
        case ACol of
          PERFromDateCol: b[ARow+1].fromdatestatus := inp;
          PERToDateCol: b[ARow+1].todatestatus := inp;
          PERPerYrCol: b[ARow+1].peryrstatus := inp;
          PERAmountCol: b[ARow+1].amtnstatus := inp;
          PERCOLACol: b[ARow+1].colastatus := inp;
          PERValueCol: b[ARow+1].valnstatus := inp;
        end;
      end;
    end;
  end;
  PeriodicValues2Grid( b, PeriodicGrid );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.HardenRateLineGrid();
var
  ACol, ARow: integer;
begin
  for ARow:=RateLineGrid.Selection.Top to RateLineGrid.Selection.Bottom do begin
    if( ARow <= RateLineGrid.GetUsedRowCount() ) then begin
      for ACol:=RateLineGrid.Selection.Left to RateLineGrid.Selection.Right do begin
        case ACol of
          RTLDateCol: cc[ARow+1].datestatus := inp;
          RTLTrueRateCol: cc[ARow+1].r.status := fromrate;
          RTLLoanRateCol: cc[ARow+1].r.status := fromapr;
          RTLYieldCol: cc[ARow+1].r.status := fromyield;
        end;
      end;
    end;
  end;
  RateLineValues2Grid( cc, RateLineGrid );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.HardenXPresValGrid();
var
  ACol: integer;
begin
  for ACol:=XPresValGrid.Selection.Left to XPresValGrid.Selection.Right do begin
    case ACol of
      XPRAsOfCol: d.xasofstatus := inp;
      XPRComputationCol: d.simplestatus := inp;
      XPRValueCol: d.xvaluestatus := inp;
    end;
  end;
  XPresValValues2Grid( xpresvalptr(d), XPresValGrid );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.HardenPresentValueGrid();
var
  ACol, ARow: integer;
begin
  for ARow:=PresentValueGrid.Selection.Top to PresentValueGrid.Selection.Bottom do begin
    if( ARow <= PresentValueGrid.GetUsedRowCount() ) then begin
      for ACol:=PresentValueGrid.Selection.Left to PresentValueGrid.Selection.Right do begin
        case ACol of
          PRVAsOfCol: c[ARow+1].asofstatus := inp;
          PRVTrueRateCol: c[ARow+1].r.status := fromrate;
          PRVLoanRateCol: c[ARow+1].r.status := fromapr;
          PRVYieldCol: c[ARow+1].r.status := fromyield;
          PRVValueCol: c[ARow+1].sumvaluestatus := inp;
        end;
      end;
    end;
  end;
  PresentValueValues2Grid( c, PresentValueGrid );
  SetUnsavedData( true );
end;

procedure TPresentValueScreen.PeriodicGridVerifyCellString(Sender: TObject;
  ACol, ARow: Integer; Value: String; var IsError: Boolean);
var
  DoubleVal: double;
begin
  IsError := false;
  case ACol of
    PERPerYrCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( DoubleVal<=0 ) then IsError := true;
    end;
    PERCOLACol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=100.00) or (DoubleVal<=-100) ) then IsError := true;
    end;
  end;
end;

procedure TPresentValueScreen.PresentValueGridVerifyCellString(
  Sender: TObject; ACol, ARow: Integer; Value: String;
  var IsError: Boolean);
var
  DoubleVal: double;
begin
  IsError := false;
  if( (ACol=PRVTrueRateCol) or (ACol=PRVLoanRateCol) or (ACol=PRVYieldCol) ) then begin
    DoubleVal := StringFormat2Double( Value, IsError );
    if( IsError ) then exit;
    if( (DoubleVal>=100.00) or (DoubleVal<=-100) ) then IsError := true;
  end;
end;

procedure TPresentValueScreen.RatelineGridVerifyCellString(Sender: TObject;
  ACol, ARow: Integer; Value: String; var IsError: Boolean);
var
  DoubleVal: double;
begin
  IsError := false;
  if( (ACol=RTLTrueRateCol) or (ACol=RTLLoanRateCol) or (ACol=RTLYieldCol) ) then begin
    DoubleVal := StringFormat2Double( Value, IsError );
    if( IsError ) then exit;
    if( (DoubleVal>=100.00) or (DoubleVal<=-100) ) then IsError := true;
  end;
end;

procedure TPresentValueScreen.TableOutput();
var
  Output: TStringList;
  Line1, Line2: string;
  bSendToFile: boolean;
  bCommaSeperated: boolean;
  FileName: string;
  Position: integer;
  hFile: integer;
  bCancelPressed: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.TableOutput' );
  // the make table function needs the nlines variables
  nlines[PVLLumpSumBlock] := LumpSumGrid.GetUsedRowCount();
  nlines[PVLPeriodicBlock] := PeriodicGrid.GetUsedRowCount();
  if( pvlfancy ) then begin
    nlines[PVLPresValBlock] := RateLineGrid.GetUsedRowCount();
    nlines[PVLXBlock] := XPresValGrid.GetUsedRowCount();
    cc[1].date := earliest;    // not sure why, but these are locked in like this
    cc[1].datestatus := defp;
  end else
    nlines[PVLPresValBlock] := PresentValueGrid.GetUsedRowCount();
  // first check to see if there's enough data to make a table
  Enter( no_tab );
  if( errorflag ) then exit;
  if (not pvlfancy) and (c[1]^.status<contains_unknown) then begin
    InsufficientDataMessage('table', DP_InsufficientDataForTable);
    exit;
  end;
  if( m_TableOut = nil ) then begin
    m_TableOut := TTableOut.Create( Self );
    m_TableOut.ReallyHide();
    m_TableOut.OnActivate := OnActivate;
    MainForm.ChildActivating( m_TableOut );
  end;
  if( not m_TableOut.GetTableOptions( bSendToFile, bCommaSeperated, df.c.peryr ) ) then exit;
  Output := TStringList.Create();
  MakeTable( Output, bCommaSeperated );
  if( bSendToFile ) then begin
    SaveDialog1.Title := 'Save table output as';
    if( bCommaSeperated ) then
      SaveDialog1.Filter := 'Comma Seperated File (*.csv)|*.CSV'
    else
      SaveDialog1.Filter := 'Text File (*.txt)|*.TXT';
    if( not SaveDialog1.Execute() ) then begin
      Output.Free();
      exit;
    end;
    FileName := SaveDialog1.FileName;
    Position := Pos( '.', FileName );
    if( Position = 0 ) then begin
      if( bCommaSeperated ) then
        FileName := FileName + '.csv'
      else
        FileName := FileName + '.txt';
    end;
    if( not FileExists( FileName ) ) then begin
      hFile := FileCreate( FileName );
      if( hFile = -1 ) then begin
        MasterLog.Write( LVL_MED, 'SaveFile failed to create new file for csv saving' );
        Output.Free();
        exit;
      end;
      FileClose( hFile );
    end else begin
      MessageBoxWithCancel( 'File Exists, would you like to save over it?', bCancelPressed, DP_OverwriteFile );
      if( bCancelPressed ) then begin
        Output.Free();
        exit;
      end;
    end;
    Output.SaveToFile( FileName );
    Output.Free();
  end else begin
    if( Output.Count > 0 ) then begin
      if( m_TableOut = nil ) then begin
        m_TableOut := TTableOut.Create( Self );
        m_TableOut.OnActivate := OnActivate;
        MainForm.ChildActivating( m_TableOut );
      end;
      m_TableOut.Width := 600;
      m_TableOut.Height := 500;
      Line1 := '';
      Line2 := '        Date        Payment          Value    Cumulative Value';
      m_TableOut.SetHeaders( Line1, Line2 );
      m_TableOut.SetOutput( Output );
      m_TableOut.DrawOutput();
      m_TableOut.ReallyShow();
    end else
      Output.Free();
  end;
end;

procedure TPresentValueScreen.OnContextualHelp();
begin
  HelpSystem.DisplayContents( 'PV_Overview.html' );
end;

procedure TPresentValueScreen.FixRates( NewPerYr: byte );
var
  i: integer;
  HoldRate: real;
  OldPerYr: integer;
begin
  MasterLog.Write( LVL_LOW, 'TPresentValueScreen.FixRates' );
  // tricky note.  We have to set df.c.peryr because calls to
  // PresentValueValues2Grid will use it and it is not
  // changed at this point.  So we will set it.
  OldPerYr := df.c.peryr;
  df.c.peryr := NewPerYr;
  for i:=1 to PresentValueGrid.GetUsedRowCount() do begin
    if( c[i].r.status <> empty ) then begin
      if( c[i].r.status = fromapr ) then begin
        HoldRate := YieldFromRate( c[i].r.rate, OldPerYr );
        c[i].r.rate := RateFromYield( HoldRate, NewPerYr );
        c[i].r.peryr := NewPeryr;
      end;
    end;
  end;
  PresentValueValues2Grid( c, PresentValueGrid );
  for i:=1 to RatelineGrid.GetUsedRowCount() do begin
    if( cc[i].r.status <> empty ) then begin
      if( cc[i].r.status = fromapr ) then begin
        HoldRate := YieldFromRate( cc[i].r.rate, OldPerYr );
        cc[i].r.rate := RateFromYield( HoldRate, NewPerYr );
        cc[i].r.peryr := NewPeryr;
      end;
    end;
  end;
  RateLineValues2Grid( cc, RatelineGrid );
end;

procedure TPresentValueScreen.LumpSumGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    LSMDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_LSMDateCol) );
    LSMAmountCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_LSMAmountCol) );
    LSMValueCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_LSMValueCol) );
  end;
end;

procedure TPresentValueScreen.PeriodicGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    PERFromDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PERFromDateCol) );
    PERToDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PERToDateCol) );
    PERPerYrCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PERPerYrCol) );
    PERAmountCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PERAmountCol) );
    PERCOLACol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PERCOLACol) );
    PERValueCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PERValueCol) );
  end;
end;

procedure TPresentValueScreen.PresentValueGridSelectCell(Sender: TObject;
  ACol, ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    PRVAsOfCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PRVAsOfCol) );
    PRVTrueRateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PRVTrueRateCol) );
    PRVLoanRateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PRVLoanRateCol) );
    PRVYieldCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PRVYieldCol) );
    PRVValueCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_PRVValueCol) );
  end;
end;

procedure TPresentValueScreen.RatelineGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    RTLDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_RTLDateCol) );
    RTLTrueRateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_RTLTrueRateCol) );
    RTLLoanRateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_RTLLoanRateCol) );
    RTLYieldCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_RTLYieldCol) );
  end;
end;

procedure TPresentValueScreen.XPresValGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    XPRAsOfCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_XPRAsOfCol) );
    XPRComputationCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_XPRComputationCol) );
    XPRValueCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SP_XPRValueCol) );
  end;
end;

end.

