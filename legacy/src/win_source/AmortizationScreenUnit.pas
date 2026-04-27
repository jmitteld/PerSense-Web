unit AmortizationScreenUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, CHILDWIN, Grids, PersenseGrid, StdCtrls, Globals, FileIOUnit, Amortize,
  peTypes, AmortizationUndoBufferUnit, TableOutUnit;

const
  // AMZLoan Cols
  AMZAmountCol                  = 0;
  AMZLoanDateCol                = 1;
  AMZLoanRateCol                = 2;
  AMZFirstDateCol               = 3;
  AMZNPeriodsCol                = 4;
  AMZLastDateCol                = 5;
  AMZPerYrCol                   = 6;
  AMZPayAmtCol                  = 7;
  AMZPointsCol                  = 8;
  AMZAPRCol                     = 9;
  // Payoff Cols
  OFFDateCol                    = 0;
  OFFAmountCol                  = 1;
  // Adjustment Cols
  ADJDateCol                    = 0;
  ADJRateCol                    = 1;
  ADJAmountCol                  = 2;
  // balloon cols
  BALDateCol                    = 0;
  BALAmountCol                  = 1;
  // prepayment cols
  PREStartCol                   = 0;
  PRENNCol                      = 1;
  PREStopCol                    = 2;
  PREPerYrCol                   = 3;
  PREPaymentCol                 = 4;

type
  TAmortizationScreen = class(TMDIChild)
    AmortGrid: TPersenseGrid;
    GroupBox1: TGroupBox;
    AmountLabel: TLabel;
    LoanDateLabel: TLabel;
    LoanRateLabel: TLabel;
    FirstDateLabel: TLabel;
    NPeriodsLabel: TLabel;
    LastDateLabel: TLabel;
    PerYrLabel: TLabel;
    PayAmtLabel: TLabel;
    PointsLabel: TLabel;
    APRLabel: TLabel;
    AdvancedGroup: TGroupBox;
    BalloonGrid: TPersenseGrid;
    PrepaymentGrid: TPersenseGrid;
    AdjustmentGrid: TPersenseGrid;
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    Label4: TLabel;
    Label5: TLabel;
    Label6: TLabel;
    Label7: TLabel;
    Label8: TLabel;
    Label9: TLabel;
    MoratoriumGrid: TPersenseGrid;
    Label10: TLabel;
    TargetGrid: TPersenseGrid;
    SkipGrid: TPersenseGrid;
    Label11: TLabel;
    Label12: TLabel;
    PayoffBox: TGroupBox;
    PayoffGrid: TPersenseGrid;
    Label13: TLabel;
    Label14: TLabel;
    SaveDialog1: TSaveDialog;
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
    procedure FormResize(Sender: TObject);
    procedure AmortGridDownAfterGrid(Sender: TObject);
    procedure AmortGridUpBeforeGrid(Sender: TObject);
    procedure PrepaymentGridDownAfterGrid(Sender: TObject);
    procedure PrepaymentGridUpBeforeGrid(Sender: TObject);
    procedure BalloonGridUpBeforeGrid(Sender: TObject);
    procedure BalloonGridDownAfterGrid(Sender: TObject);
    procedure AdjustmentGridDownAfterGrid(Sender: TObject);
    procedure AdjustmentGridUpBeforeGrid(Sender: TObject);
    procedure MoratoriumGridUpBeforeGrid(Sender: TObject);
    procedure MoratoriumGridDownAfterGrid(Sender: TObject);
    procedure TargetGridDownAfterGrid(Sender: TObject);
    procedure TargetGridUpBeforeGrid(Sender: TObject);
    procedure PayoffGridUpBeforeGrid(Sender: TObject);
    procedure PrepaymentGridLeftBeforeGrid(Sender: TObject; var Default: boolean);
    procedure PrepaymentGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure AmortGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean );
    procedure PayoffGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure AdjustmentGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure BalloonGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure PrepaymentGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure TargetGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure MoratoriumGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure SkipGridCellAfterEdit(Sender: TObject; ACol, ARow: Integer; const Value: String; DataChanged: boolean);
    procedure AmortGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure PayoffGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure PrepaymentGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure BalloonGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure AdjustmentGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure MoratoriumGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure TargetGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure SkipGridEditEnterKeyPressed(Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
    procedure AmortGridEditCopy(Sender: TObject);
    procedure PayoffGridEditCopy(Sender: TObject);
    procedure PrepaymentGridEditCopy(Sender: TObject);
    procedure BalloonGridEditCopy(Sender: TObject);
    procedure AdjustmentGridEditCopy(Sender: TObject);
    procedure MoratoriumGridEditCopy(Sender: TObject);
    procedure TargetGridEditCopy(Sender: TObject);
    procedure SkipGridEditCopy(Sender: TObject);
    procedure AmortGridEditCut(Sender: TObject);
    procedure PayoffGridEditCut(Sender: TObject);
    procedure PrepaymentGridEditCut(Sender: TObject);
    procedure BalloonGridEditCut(Sender: TObject);
    procedure AdjustmentGridEditCut(Sender: TObject);
    procedure MoratoriumGridEditCut(Sender: TObject);
    procedure TargetGridEditCut(Sender: TObject);
    procedure SkipGridEditCut(Sender: TObject);
    procedure AmortGridEditPaste(Sender: TObject);
    procedure PayoffGridEditPaste(Sender: TObject);
    procedure PrepaymentGridEditPaste(Sender: TObject);
    procedure BalloonGridEditPaste(Sender: TObject);
    procedure AdjustmentGridEditPaste(Sender: TObject);
    procedure MoratoriumGridEditPaste(Sender: TObject);
    procedure TargetGridEditPaste(Sender: TObject);
    procedure SkipGridEditPaste(Sender: TObject);
    procedure BalloonGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure AdjustmentGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure MoratoriumGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure MoratoriumGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure TargetGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure TargetGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure SkipGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure SkipGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure PayoffGridLeftBeforeGrid(Sender: TObject; var Default: Boolean);
    procedure PayoffGridRightAfterGrid(Sender: TObject; var Default: Boolean);
    procedure AmortGridCellBeforeEdit(Sender: TObject; ACol, ARow: Integer; const Value: String);
    procedure AmortGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);
    procedure PrepaymentGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);
    procedure AdjustmentGridVerifyCellString(Sender: TObject; ACol, ARow: Integer; Value: String; var IsError: Boolean);
    procedure AmortGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure PrepaymentGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure BalloonGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure AdjustmentGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure MoratoriumGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure TargetGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure SkipGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
    procedure PayoffGridSelectCell(Sender: TObject; ACol, ARow: Integer; var CanSelect: Boolean);
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
    procedure OnPrint( Header: TStringList ); override;
    procedure OnContextualHelp(); override;
    procedure SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont ); override;
    function AdvancedIsOn(): boolean;
    procedure ToggleAdvanced();
    procedure TableOutput();
    function IsBackedUpForHelp(): boolean;
    procedure BackupForHelpSystem();
    procedure RestoreFromHelpSystem();
  protected
    m_UndoBuffer: TAmortizationUndoBuffer;
    m_TableOut: TTableOut;
    m_bBackedUpForHelp: boolean;
    m_BackupFileName: string;
    m_BackupCaption: string;
    m_bBackupUnsaved: boolean;
    m_HelpBackupAMZ: AMZLoan;
    m_HelpBackupPayoff: balloonrec;
    m_HelpBackupPrepayment: prepaymentarray;
    m_HelpBackupBalloon: balloonarray;
    m_HelpBackupAdjustment: adjarray;
    m_HelpBackupMoratorium: moratoriumrec;
    m_HelpBackupTarget: targetrec;
    m_HelpBackupSkip: skiprec;
    procedure DoCalculation();
    function GetFocusedGrid(): TPersenseGrid;
    procedure EmptyAllOutpCells();
    // harden cells
    procedure HardenAmortGrid();
    procedure HardenPayoffGrid();
    procedure HardenPrepaymentGrid();
    procedure HardenBalloonGrid();
    procedure HardenAdjustmentGrid();
    procedure HardenMoratoriumGrid();
    procedure HardenTargetGrid();
    procedure HardenSkipGrid();
    // cut
    procedure AmortGridCut();
    procedure PayoffGridCut();
    procedure PrepaymentGridCut();
    procedure BalloonGridCut();
    procedure AdjustmentGridCut();
    procedure MoratoriumGridCut();
    procedure TargetGridCut();
    procedure SkipGridCut();
    // paste
    procedure AmortGridPaste();
    procedure PayoffGridPaste();
    procedure PrepaymentGridPaste();
    procedure BalloonGridPaste();
    procedure AdjustmentGridPaste();
    procedure MoratoriumGridPaste();
    procedure TargetGridPaste();
    procedure SkipGridPaste();
    // delete
    procedure AmortGridDelete();
    procedure PayoffGridDelete();
    procedure PrepaymentGridDelete();
    procedure BalloonGridDelete();
    procedure AdjustmentGridDelete();
    procedure MoratoriumGridDelete();
    procedure TargetGridDelete();
    procedure SkipGridDelete();
    // moving data between the underlying object and the grids
    procedure AssignAMZLoanValues( AMZ: AMZPtr; ACol :integer; Value: string; Hardness: inout );
    procedure AMZValues2Grid( AMZ: AMZPtr; Grid: TPersenseGrid );
    procedure AssignPayoffValues( Payoff: balloonptr; ACol: integer; Value: string; Hardness: inout );
    procedure PayoffValues2Grid( Payoff: balloonptr; Grid: TPersenseGrid );
    procedure AssignAdjustmentValues( var ADJ: adjarray; ACol, ARow: integer; Value: string; Hardness: inout );
    procedure AdjustmentValues2Grid( ADJ: adjarray; Grid: TPersenseGrid );
    procedure AssignBalloonValues( var Balloon: balloonarray; ACol, ARow: integer; Value: string; Hardness: inout );
    procedure BalloonValues2Grid( Balloon: Balloonarray; Grid: TPersenseGrid );
    procedure AssignPrepaymentValues( var Prepayment: prepaymentarray; ACol, ARow: integer; Value: string; Hardness: inout );
    procedure PrepaymentValues2Grid( Prepayment: prepaymentarray; Grid: TPersenseGrid );
    procedure AssignTargetValue( Target: targetptr; Value: string; Hardness: inout );
    procedure TargetValue2Grid( Target: targetptr; Grid: TPersenseGrid );
    procedure AssignMoratoriumValue( Moratorium: moratoriumptr; Value: string; Hardness: inout );
    procedure MoratoriumValue2Grid( Moratorium: moratoriumptr; Grid: TPersenseGrid );
    procedure AssignSkipValue( Skip: skipptr; Value: string; Hardness: inout );
    procedure SkipValue2Grid( Skip: skipptr; Grid: TPersenseGrid );
    procedure AllAMZData2Grids();
  private
  end;

var
  AmortizationScreen: TAmortizationScreen;

implementation

uses peData, intsutil, LogUnit, Main, Printers, HelpSystemUnit;

{$R *.dfm}

constructor TAmortizationScreen.Create(AOwner: TComponent);
var
  i: integer;
  CanSelect: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.Create begining' );
  inherited Create( AOwner );
  // initialize global vars in peData
  GetMem( h, sizeof(AMZLoan) );                  // main AMZLoan object
  ZeroAMZLoan( h );
  GetMem( mor, sizeof(moratoriumrec) );          // moratorium pointer
  ZeroMoratorium( mor );
  GetMem( targ, sizeof(targetrec) );             // target pointer
  ZeroTarget( targ );
  GetMem( skp, sizeof(skiprec) );                // skip payment ptr
  ZeroSkip( skp );
  GetMem( w, sizeof(balloonrec) );               // Payoff ptr
  ZeroBalloon( balloonptr(w) );
  for i:=1 to maxprepay do begin
    GetMem( pre[i], sizeof(prepaymentrec) );     // prepayments
    ZeroPrepayment( prepaymentptr(pre[i]) );
    GetMem( m_HelpBackupPrepayment[i], sizeof(prepaymentrec) );
  end;
  for i:=1 to maxballoon do begin
    GetMem( balloon[i], sizeof(balloonrec) );    // balloons
    ZeroBalloon( balloonptr(balloon[i]) );
    GetMem( m_HelpBackupBalloon[i], sizeof(balloonrec) );
  end;
  for i:=1 to maxadj do begin
    GetMem( adj[i], sizeof(adjrec) );            // adjustments
    ZeroAdjustment( adjptr(adj[i]) );
    GetMem( m_HelpBackupAdjustment[i], sizeof(adjrec) );
  end;
  AmortGrid.SetupColumn( AMZAmountCol, 0.0, ColTypeDollar, 19 );
  AmortGrid.SetupColumn( AMZLoanDateCol, 0.14, ColTypeDate, DateCellLength );
  AmortGrid.SetupColumn( AMZLoanRateCol, 0.26, ColType4Real, 13 );
  AmortGrid.SetupColumn( AMZFirstDateCol, 0.33, ColTypeDate, DateCellLength );
  AmortGrid.SetupColumn( AMZNPeriodsCol, 0.44, ColTypeInt, 3 );
  AmortGrid.SetupColumn( AMZLastDateCol, 0.5, ColTypeDate, DateCellLength );
  AmortGrid.SetupColumn( AMZPerYrCol, 0.62, ColTypeInt, 2 );
  AmortGrid.SetupColumn( AMZPayAmtCol, 0.68, ColTypeDollar, 15 );
  AmortGrid.SetupColumn( AMZPointsCol, 0.82, ColType4Real, 10 );
  AmortGrid.SetupColumn( AMZAPRCol, 0.9, ColType4Real, 10 );
  AmortGrid.SetCellReadOnly( AMZAPRCol, 0 );
  fancy := false;
  AdvancedGroup.Visible := false;
  PrepaymentGrid.RowCount := maxprepay;
  PrepaymentGrid.SetupColumn( PREStartCol, 0.0, ColTypeDate, DateCellLength );
  PrepaymentGrid.SetupColumn( PRENNCol, 0.225, ColTypeInt, 3 );
  PrepaymentGrid.SetupColumn( PREStopCol, 0.345, ColTypeDate, DateCellLength );
  PrepaymentGrid.SetupColumn( PREPerYrCol, 0.59, ColTypeInt, 2 );
  PrepaymentGrid.SetupColumn( PREPaymentCol, 0.71, ColTypeDollar, 15 );
  BalloonGrid.RowCount := maxballoon;
  BalloonGrid.SetupColumn( BALDateCol, 0.0, ColTypeDate, DateCellLength );
  BalloonGrid.SetupColumn( BALAmountCol, 0.4, ColTypeDollar, 18 );
  AdjustmentGrid.RowCount := maxadj;
  AdjustmentGrid.SetupColumn( ADJDateCol, 0.0, ColTypeDate, DateCellLength );
  AdjustmentGrid.SetupColumn( ADJRateCol, 0.3, ColType4Real, 14 );
  AdjustmentGrid.SetupColumn( ADJAmountCol, 0.57, ColTypeDollar, 18 );
  MoratoriumGrid.SetupColumn( 0, 0.0, ColTypeDate, DateCellLength );
  TargetGrid.SetupColumn( 0, 0.0, ColType2Real, 14 );
  SkipGrid.SetupColumn( 0, 0.0, ColTypeString, 15 );
  PayoffGrid.SetupColumn( OFFDateCol, 0.0, ColTypeDate, DateCellLength );
  PayoffGrid.SetupColumn( OFFAmountCol, 0.4, ColTypeDollar, 22 );
  Width := 600;
  Height := 400;
  m_UndoBuffer := TAmortizationUndoBuffer.Create();
  // I have no idea what this thing is for, but it gets used in the
  // functions from arbitrar.pas so I need it to not be nil
  GetMem( q, sizeof(RBTstart) );
  m_bBackedUpForHelp := false;
  // must do this to get the status text going
  AmortGridSelectCell( Self, 0, 0, CanSelect );
  m_TableOut := nil;
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.Create ending' );
end;

destructor TAmortizationScreen.Destroy();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.Destroy begining' );
  m_TableOut.Free();
  m_UndoBuffer.Free();
  FreeMem( h );
  FreeMem( mor );
  FreeMem( targ );
  FreeMem( skp );
  FreeMem( w );
  for i:=1 to maxprepay do begin
    FreeMem( pre[i] );
  end;
  for i:=1 to maxballoon do begin
    FreeMem( balloon[i] );
  end;
  for i:=1 to maxadj do begin
    FreeMem( adj[i] );
  end;
  FreeMem( q );
  inherited Destroy();
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.Destroy ending' );
end;

function TAmortizationScreen.GetType(): TScreenType;
begin
  GetType := AmortizationType;
end;

procedure TAmortizationScreen.FormClose(Sender: TObject; var Action: TCloseAction);
begin
  OnFormClose( Action );
end;

function TAmortizationScreen.AdvancedIsOn(): boolean;
begin
  AdvancedIsOn := fancy;
end;

procedure TAmortizationScreen.ToggleAdvanced();
begin
  fancy := not fancy;
  AdvancedGroup.Visible := fancy;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.EmptyAllOutpCells();
var
  i: integer;
begin
  if( h.amountstatus = outp ) then
    h.amountstatus := empty;
  if( h.loandatestatus = outp ) then
    h.loandatestatus := empty;
  if( h.loanratestatus = outp ) then
    h.loanratestatus := empty;
  if( h.firststatus = outp ) then
    h.firststatus := empty;
  if( h.nstatus = outp ) then
    h.nstatus := empty;
  if( h.laststatus = outp ) then
    h.laststatus := empty;
  if( h.peryrstatus = outp ) then
    h.peryrstatus := empty;
  if( h.payamtstatus = outp ) then
    h.payamtstatus := empty;
  if( h.pointsstatus = outp ) then
    h.pointsstatus := empty;
  if( h.aprstatus = outp ) then
    h.aprstatus := empty;

  for i:=1 to maxprepay do begin
    if( pre[i].startdatestatus = outp ) then
      pre[i].startdatestatus := empty;
    if( pre[i].nnstatus = outp ) then
      pre[i].nnstatus := empty;
    if( pre[i].stopdatestatus = outp ) then
      pre[i].stopdatestatus := empty;
    if( pre[i].peryrstatus = outp ) then
      pre[i].peryrstatus := empty;
    if( pre[i].paymentstatus = outp ) then
      pre[i].paymentstatus := empty;
  end;

  for i:=1 to maxballoon do begin
    if( balloon[i].datestatus = outp ) then
      balloon[i].datestatus := empty;
    if( balloon[i].amountstatus = outp ) then
      balloon[i].amountstatus := empty;
  end;

  for i:=1 to maxadj do begin
    if( adj[i].datestatus = outp ) then
      adj[i].datestatus := empty;
    if( adj[i].loanratestatus = outp ) then
      adj[i].loanratestatus := empty;
    if( adj[i].amountstatus = outp ) then
      adj[i].amountstatus := empty;
  end;

  if( mor.first_repaystatus = outp ) then
    mor.first_repaystatus := empty;

  if( targ.targetstatus = outp ) then
    targ.targetstatus := empty;

  if( skp.skipstatus = outp ) then
    skp.skipstatus := empty;

  if( w.datestatus = outp ) then
    w.datestatus := empty;
  if( w.amountstatus = outp ) then
    w.amountstatus := empty;
end;

procedure TAmortizationScreen.TableOutput();
var
  Output: TStringList;
  Line1: string;
  Line2: string;
  bSendToFile: boolean;
  bCommaSeperated: boolean;
  FileName: string;
  Position: integer;
  hFile: integer;
  bCancelPressed: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.TableOutput' );
  // first check to see if there's enough data to make a table
  Enter( no_tab );
  if( errorflag ) then exit;
  if( not SufficientDataOnScreen() ) then begin
    MessageBox( 'Not enough data to create a table', DA_NotEnoughDataForTable );
    BringToFront();
    exit;
  end;
  if( m_TableOut = nil ) then begin
    m_TableOut := TTableOut.Create( Self );
    m_TableOut.ReallyHide();
    m_TableOut.OnActivate := OnActivate;
    MainForm.ChildActivating( m_TableOut );
  end;
  if( not m_TableOut.GetTableOptions( bSendToFile, bCommaSeperated, h.peryr ) ) then exit;
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
      MessageBoxWithCancel( 'File Exists, would you like to save over it?', bCancelPressed, DA_OverwriteFile );
      if( bCancelPressed ) then begin
        Output.Free();
        exit;
      end;
    end;
    Output.SaveToFile( FileName );
    Output.Free();
  end else begin
    if( Output.Count > 0 ) then begin
      m_TableOut.Width := 700;
      m_TableOut.Height := 500;
      if( fancy ) then begin
        Line1 := '               Payment      Interest     Principal     Principal       Interest';
        Line2 := '   Date         Amount   This Period   This Period   New Balance        To Date';
      end else begin
        Line1 := '                     Interest       Principal       Principal        Interest';
        Line2 := '  ##     Date     This Period     This Period     New Balance         To Date';
      end;
      m_TableOut.SetHeaders( Line1, Line2 );
      m_TableOut.SetOutput( Output );
      m_TableOut.DrawOutput();
      m_TableOut.ReallyShow();
    end else
      Output.Free();
  end;
end;

procedure TAmortizationScreen.OnCalculate();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnCalculate' );
  DoCalculation();
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
end;

procedure TAmortizationScreen.DoCalculation();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.DoCalculation' );
  EmptyAllOutpCells();
  nlines[AMZAdjBlock] := AdjustmentGrid.GetUsedRowCount();
  nlines[AMZballoonblock] := BalloonGrid.GetUsedRowCount();
  nlines[AMZpreblock] := PrepaymentGrid.GetUsedRowCount();
  nlines[AMZBalanceBlock] := PayoffGrid.GetUsedRowCount();
  Enter( no_tab );
  AllAMZData2Grids();
  SetUnsavedData( true );
end;

function TAmortizationScreen.GetFocusedGrid(): TPersenseGrid;
begin
  GetFocusedGrid := nil;
  if( AmortGrid.Focused() ) then
    GetFocusedGrid := AmortGrid
  else if( PayoffGrid.Focused() ) then
    GetFocusedGrid := PayoffGrid
  else if( AdvancedIsOn() ) then begin
    if( PrepaymentGrid.Focused() ) then
      GetFocusedGrid := PrepaymentGrid
    else if( BalloonGrid.Focused() ) then
      GetFocusedGrid := BalloonGrid
    else if( AdjustmentGrid.Focused() ) then
      GetFocusedGrid := AdjustmentGrid
    else if( MoratoriumGrid.Focused() ) then
      GetFocusedGrid := MoratoriumGrid
    else if( TargetGrid.Focused() ) then
      GetFocusedGrid := TargetGrid
    else if( SkipGrid.Focused() ) then
      GetFocusedGrid := SkipGrid
  end;
end;

procedure TAmortizationScreen.OnHardenValue();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnHardenValue' );
  if( AmortGrid.Focused() ) then
    HardenAmortGrid()
  else if( PayoffGrid.Focused() ) then
    HardenPayoffGrid()
  else if( AdvancedIsOn() ) then begin
    if( PrepaymentGrid.Focused() ) then
      HardenPrepaymentGrid()
    else if( BalloonGrid.Focused() ) then
      HardenBalloonGrid()
    else if( AdjustmentGrid.Focused() ) then
      HardenAdjustmentGrid()
    else if( MoratoriumGrid.Focused() ) then
      HardenMoratoriumGrid()
    else if( TargetGrid.Focused() ) then
      HardenTargetGrid()
    else if( SkipGrid.Focused() ) then
      HardenSkipGrid()
  end;
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.OnUndo();
var
  Success : boolean;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnUndo' );
  m_UndoBuffer.Undo( h, balloonptr(w), pre, balloon, adj, mor, targ, skp, Success );
  if( not Success ) then begin
    MasterLog.Write( LVL_LOW, 'Undo1Click failed to Undo' );
    exit;
  end;
  AllAMZData2Grids();
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.OnRedo();
var
  Success : boolean;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnRedo' );
  m_UndoBuffer.Redo( h, balloonptr(w), pre, balloon, adj, mor, targ, skp, Success );
  if( not Success ) then begin
    MasterLog.Write( LVL_LOW, 'Redo1Click failed to Redo' );
    exit;
  end;
  AllAMZData2Grids();
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.OnCut();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnCut' );
  if( AmortGrid.Focused() ) then
    AmortGridCut()
  else if( PayoffGrid.Focused() ) then
    PayoffGridCut()
  else if( AdvancedIsOn() ) then begin
    if( PrepaymentGrid.Focused() ) then
      PrepaymentGridCut()
    else if( BalloonGrid.Focused() ) then
      BalloonGridCut()
    else if( AdjustmentGrid.Focused() ) then
      AdjustmentGridCut()
    else if( MoratoriumGrid.Focused() ) then
      MoratoriumGridCut()
    else if( TargetGrid.Focused() ) then
      TargetGridCut()
    else if( SkipGrid.Focused() ) then
      SkipGridCut()
  end;
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.OnCopy();
var
  Grid: TPersenseGrid;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnCopy' );
  Grid := GetFocusedGrid();
  if( Grid <> nil ) then
    Grid.CopyToClipboard();
end;

procedure TAmortizationScreen.OnPaste();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnPaste' );
  if( AmortGrid.Focused() ) then
    AmortGridPaste()
  else if( PayoffGrid.Focused() ) then
    PayoffGridPaste()
  else if( AdvancedIsOn() ) then begin
    if( PrepaymentGrid.Focused() ) then
      PrepaymentGridPaste()
    else if( BalloonGrid.Focused() ) then
      BalloonGridPaste()
    else if( AdjustmentGrid.Focused() ) then
      AdjustmentGridPaste()
    else if( MoratoriumGrid.Focused() ) then
      MoratoriumGridPaste()
    else if( TargetGrid.Focused() ) then
      TargetGridPaste()
    else if( SkipGrid.Focused() ) then
      SkipGridPaste()
  end;
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.OnDelete();
var
  Grid: TPersenseGrid;
  DelStart, DelEnd: Integer;
  TheText, First, Second: string;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnDelete' );
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
    if( AmortGrid.Focused() ) then
      AmortGridDelete()
    else if( PayoffGrid.Focused() ) then
      PayoffGridDelete()
    else if( AdvancedIsOn() ) then begin
      if( PrepaymentGrid.Focused() ) then
        PrepaymentGridDelete()
      else if( BalloonGrid.Focused() ) then
        BalloonGridDelete()
      else if( AdjustmentGrid.Focused() ) then
        AdjustmentGridDelete()
      else if( MoratoriumGrid.Focused() ) then
        MoratoriumGridDelete()
      else if( TargetGrid.Focused() ) then
        TargetGridDelete()
      else if( SkipGrid.Focused() ) then
        SkipGridDelete()
    end;
  end;
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.OpenFile( var TheFile: TFileIO );
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OpenFile' );
  // clean up existing data
  AMortGrid.EmptyRow( 0 );
  for i:=1 to maxprepay do begin
    PrepaymentGrid.EmptyRow( i-1 );
    ZeroPrepayment( prepaymentptr(pre[i]) );
  end;
  for i:=1 to maxballoon do begin
    BalloonGrid.EmptyRow( i-1 );
    ZeroBalloon( balloonptr(balloon[i]) );
  end;
  for i:=1 to maxadj do begin
    AdjustmentGrid.EmptyRow( i-1 );
    ZeroAdjustment( adjptr(adj[i]) );
  end;
  MoratoriumGrid.EmptyRow( 0 );
  ZeroMoratorium( mor );
  TargetGrid.EmptyRow( 0 );
  ZeroTarget( targ );
  SkipGrid.EmptyRow( 0 );
  ZeroSkip( skp );
  PayoffGrid.EmptyRow( 0 );
  ZeroBalloon( balloonptr(w) );
  // now fill with new data
  TheFile.GetAmortizationData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  if( not m_bBackedUpForHelp ) then
    m_FileName := TheFile.GetFileName()
  else
    m_FileName := '';
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  // put the screen in the right mode
  if( fancy <> (TheFile.GetFancyByte()=1) ) then begin
    MainForm.CalcAmzToggleAdvancedOptions1Execute(Self);
  end;
  AllAMZData2Grids();
  SetUnsavedData( false );
end;

function TAmortizationScreen.SaveFile( FileName, ScreenName : string ): boolean;
var
  TheFile: TFileIO;
  hFile: integer;
  Position: integer;
  bWarnOnOverwrite: boolean;
  bCancelPressed: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.SaveFile' );
  bWarnOnOverwrite := false;
  if( ScreenName <> '' ) then
    SaveDialog1.Title := 'Save ' + ScreenName + ' As';
  SaveDialog1.Filter := 'Amortization Files (*.amz)|*.AMZ';
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
    FileName := FileName + '.amz';
  if( not FileExists( FileName ) ) then begin
    hFile := FileCreate( FileName );
    if( hFile = -1 ) then begin
      MasterLog.Write( LVL_MED, 'SaveFile failed to create new file for saving' );
      SaveFile := false;
      exit;
    end;
    FileClose( hFile );
  end else if( bWarnOnOverwrite ) then begin
    MessageBoxWithCancel( 'File Exists, would you like to save over it?', bCancelPressed, DA_OverwriteFile );
    if( bCancelPressed ) then begin
      SaveFile := false;
      exit;
    end;
  end;
  TheFile := TFileIO.Create();
  TheFile.SaveAmortization( FileName, h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  m_FileName := FileName;
  TheFile.Free();
  SetUnsavedData( false );
  SaveFile := true;
end;

procedure TAmortizationScreen.OnPrint( Header: TStringList );
var
  yPos: integer;
  RowsPrinted, ADJRowsPrinted: integer;
  PenWidth: integer;
  CurrentRow, ADJCurrentRow: integer;
  CellHeight, ADJCellHeight: integer;
  BoxTop: integer;
  MaxRowCount, ADJMAxRowCount: integer;
  i: integer;
  BalloonPosition: integer;
  TextHeight: integer;
  UsablePageWidth: integer;
  HeaderCount: integer;
  yHeaderEnd: integer;
const
  Margin = 80;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnPrint' );
  Printer.BeginDoc();
  Printer.Canvas.Font.Assign( Font );
  Printer.Title := 'Persense Amortization Screen';
  TextHeight := Printer.Canvas.TextHeight( 'Amount' );
  UsablePageWidth := Printer.PageWidth-(2*Margin);
  if( Header = nil ) then
    HeaderCount := 0
  else
    HeaderCount := Header.Count+1;
  PrintHeader( Printer, TextHeight, Header );
  yPos := 100 + (HeaderCount*TextHeight);
  yHeaderEnd := yPos;
  // top line, the AMZLoan grid.
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.03), yPos, 'Amount' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.15), yPos, 'Loan' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.27), yPos, 'Loan' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.35), yPos, 'First Pmt' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.47), yPos, '#of' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.53), yPos, 'Last Pmt' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.65), yPos, 'Pmts' );
  // second line, AMZ Loan grid.
  yPos := yPos + TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.03), yPos, 'Borrowed' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.15), yPos, 'Date' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.27), yPos, 'Rate%' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.35), yPos, 'Date' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.47), yPos, 'Pds' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.53), yPos, 'Date' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.65), yPos, '/Year' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.71), yPos, 'Payment' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.83), yPos, 'Points' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.93), yPos, 'APR %' );
  // box the field headers
  PenWidth := Printer.Canvas.Pen.Width;
  Printer.Canvas.Pen.Width := 6;
  Printer.Canvas.MoveTo( Margin, yHeaderEnd );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, yHeaderEnd );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, yPos + TextHeight );
  Printer.Canvas.LineTo( Margin, yPos + TextHeight );
  Printer.Canvas.LineTo( Margin, yHeaderEnd );
  // now the AmortGrid
  yPos := yPos + Trunc( 1.5*TextHeight);
  CellHeight := Printer.PageHeight-yPos-2*Margin;
  AmortGrid.Print( 0, yPos, Margin, UsablePageWidth, CellHeight, 1, RowsPrinted );
  if( fancy ) then begin
    yPos := yPos + CellHeight;
    // print the PrepaymentsGrid
    CellHeight := Printer.PageHeight-yPos-2*Margin;
    PrepaymentGrid.Print( 0, yPos, Trunc(UsablePageWidth*0.35), Trunc(UsablePageWidth*0.48), CellHeight, maxprepay, RowsPrinted );
    BalloonPosition := yPos + CellHeight + Trunc(1.5*TextHeight);
    yPos := yPos + Trunc( 0.5*TextHeight );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.08), yPos, 'Additional Periodic Payments:' );
    yPos := yPos + TextHeight;
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.08), yPos, '(0 for regular payments)' );
    MaxRowCount := 0;
    ADJMaxRowCount := 0;
    for i:=1 to maxballoon do begin
      if( not BalloonIsEmpty( balloonptr(balloon[i]) ) ) then
        MaxRowCount := i;
    end;
    for i:=1 to maxadj do begin
      if( not AdjustmentIsEmpty( adjptr(adj[i]) ) ) then
        ADJMaxRowCount := i;
    end;
    // now we know how many rows need to be printed for balloon and adj
    CurrentRow := 0;
    ADJCurrentRow := 0;
    yPos := BalloonPosition;
    while( (CurrentRow < MaxRowCount) or (ADJCurrentRow<ADJMaxRowCount) ) do begin
      Printer.Canvas.Brush.Color := clWhite;
      if( CurrentRow < MaxRowCount ) then Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.14), yPos, 'Balloon Payments' );
      if( ADJCurrentRow < ADJMaxRowCount ) then Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.62), yPos, 'Rate Changes and Changes in Payment' );
      yPos := yPos + Trunc( TextHeight);
      if( CurrentRow < MaxRowCount ) then Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.14), yPos, 'Date               Amount' );
      if( ADJCurrentRow < ADJMaxRowCount ) then Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.64), yPos, 'Date           Rate            Amount' );
      yPos := yPos + Trunc( 1.5*TextHeight);
      if( CurrentRow < MaxRowCount ) then begin
        CellHeight := Printer.PageHeight-yPos-2*Margin;
        BalloonGrid.Print( CurrentRow, yPos, Trunc(UsablePageWidth*0.1), Trunc(UsablePageWidth*0.25), CellHeight, MaxRowCount, RowsPrinted );
        CurrentRow := CurrentRow + RowsPrinted;
      end else
        CellHeight := 0;
      if( ADJCurrentRow < ADJMaxRowCount ) then begin
        ADJCellHeight := Printer.PageHeight-yPos-2*Margin;
        AdjustmentGrid.Print( ADJCurrentRow, yPos, Trunc(UsablePageWidth*0.6), Trunc(UsablePageWidth*0.30), ADJCellHeight, ADJMaxRowCount, ADJRowsPrinted );
        ADJCurrentRow := ADJCurrentRow + ADJRowsPrinted;
      end else
        ADJCellHeight := 0;
      // possibly set up for a new page
      if( (CurrentRow < MaxRowCount) or (ADJCurrentRow < ADJMaxRowCount) ) then begin
        Printer.NewPage();
        yPos := 100 + (HeaderCount*TextHeight);
      end else if( ADJCellHeight > CellHeight ) then
        yPos := yPos + ADJCellHeight
      else
        yPos := yPos + CellHeight;
    end;
    if( (yPos + 5*TextHeight) > Printer.PageHeight ) then begin
      // we may be at the end of the page with no room to put the last line
      Printer.NewPage();
      yPos := 100 + (HeaderCount*TextHeight);
    end else begin
      // or we just want to add the last row
      yPos := yPos + TextHeight;
    end;
    // still need to print the last 3 grids
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.12), yPos, 'Int only til' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.3), yPos, 'Princ target' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.47), yPos, 'No payment months' );
    yPos := yPos + TextHeight;
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.14), yPos, 'Date' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.31), yPos, 'Amount' );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.49), yPos, 'Skip months' );
    yPos := yPos + Trunc(1.5*TextHeight);
    CellHeight := Printer.PageHeight-yPos-2*Margin;
    MoratoriumGrid.Print( 0, yPos, Trunc( UsablePageWidth * 0.1), Trunc( UsablePageWidth * 0.12), CellHeight, 1, RowsPrinted );
    CellHeight := Printer.PageHeight-yPos-2*Margin;
    TargetGrid.Print( 0, yPos, Trunc( UsablePageWidth * 0.28), Trunc( UsablePageWidth * 0.12), CellHeight, 1, RowsPrinted );
    CellHeight := Printer.PageHeight-yPos-2*Margin;
    SkipGrid.Print( 0, yPos, Trunc( UsablePageWidth * 0.46), Trunc( UsablePageWidth * 0.12), CellHeight, 1, RowsPrinted );
    // put the yPos back for the Payoff grid
    yPos := yPos - Trunc(2.5*TextHeight);
  end else begin
    yPos := yPos + CellHeight + Trunc( 1.5*TextHeight);
  end;
  // the payoff grid
  BoxTop := yPos;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.65), yPos, 'Check payoff balance here' );
  yPos := yPos + TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.7), yPos, 'Date' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.85), yPos, 'Balance as of:' );
  yPos := yPos + Trunc(1.5*TextHeight);
  CellHeight := Printer.PageHeight-yPos-2*Margin;
  PayoffGrid.Print( 0, yPos, Trunc( UsablePageWidth * 0.65), UsablePageWidth-Trunc( UsablePageWidth * 0.65)+60, CellHeight, 1, RowsPrinted );
  // box the Payoff grid
  Printer.Canvas.MoveTo( Trunc( UsablePageWidth * 0.65-20), BoxTop );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, BoxTop );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, yPos + CellHeight+20 );
  Printer.Canvas.LineTo( Trunc( UsablePageWidth * 0.65-20), yPos + CellHeight+20 );
  Printer.Canvas.LineTo( Trunc( UsablePageWidth * 0.65-20), BoxTop );

  Printer.Canvas.Pen.Width := PenWidth;
  Printer.EndDoc();
end;

procedure TAmortizationScreen.SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont );
begin
  AmortGrid.CellBackgroundColor := CellColour;
  AmortGrid.OutpCellBackgroundColor := OutpCellColour;
  AmortGrid.SelectedCellColor := SelectedColour;
  AmortGrid.CellFont := CellFont;
  AmortGrid.OutpCellFont := OutpCellFont;
  AmortGrid.Repaint();
  PrepaymentGrid.CellBackgroundColor := CellColour;
  PrepaymentGrid.OutpCellBackgroundColor := OutpCellColour;
  PrepaymentGrid.SelectedCellColor := SelectedColour;
  PrepaymentGrid.CellFont := CellFont;
  PrepaymentGrid.OutpCellFont := OutpCellFont;
  PrepaymentGrid.Repaint();
  BalloonGrid.CellBackgroundColor := CellColour;
  BalloonGrid.OutpCellBackgroundColor := OutpCellColour;
  BalloonGrid.SelectedCellColor := SelectedColour;
  BalloonGrid.CellFont := CellFont;
  BalloonGrid.OutpCellFont := OutpCellFont;
  BalloonGrid.Repaint();
  AdjustmentGrid.CellBackgroundColor := CellColour;
  AdjustmentGrid.OutpCellBackgroundColor := OutpCellColour;
  AdjustmentGrid.SelectedCellColor := SelectedColour;
  AdjustmentGrid.CellFont := CellFont;
  AdjustmentGrid.OutpCellFont := OutpCellFont;
  AdjustmentGrid.Repaint();
  MoratoriumGrid.CellBackgroundColor := CellColour;
  MoratoriumGrid.OutpCellBackgroundColor := OutpCellColour;
  MoratoriumGrid.SelectedCellColor := SelectedColour;
  MoratoriumGrid.CellFont := CellFont;
  MoratoriumGrid.OutpCellFont := OutpCellFont;
  MoratoriumGrid.Repaint();
  TargetGrid.CellBackgroundColor := CellColour;
  TargetGrid.OutpCellBackgroundColor := OutpCellColour;
  TargetGrid.SelectedCellColor := SelectedColour;
  TargetGrid.CellFont := CellFont;
  TargetGrid.OutpCellFont := OutpCellFont;
  TargetGrid.Repaint();
  SkipGrid.CellBackgroundColor := CellColour;
  SkipGrid.OutpCellBackgroundColor := OutpCellColour;
  SkipGrid.SelectedCellColor := SelectedColour;
  SkipGrid.CellFont := CellFont;
  SkipGrid.OutpCellFont := OutpCellFont;
  SkipGrid.Repaint();
  PayoffGrid.CellBackgroundColor := CellColour;
  PayoffGrid.OutpCellBackgroundColor := OutpCellColour;
  PayoffGrid.SelectedCellColor := SelectedColour;
  PayoffGrid.CellFont := CellFont;
  PayoffGrid.OutpCellFont := OutpCellFont;
  PayoffGrid.Repaint();
  FormResize( Self );
end;

procedure TAmortizationScreen.FormResize(Sender: TObject);
const
  LabelMargin = 10;
  CellBorderHeight = 4;
var
  BottomRowY: integer;
begin
  // move labels
  GroupBox1.Width := Width-10;
  AmountLabel.Left := 0 + LabelMargin;
  LoanDateLabel.Left := Trunc( Width * 0.12 ) + LabelMargin+10;
  LoanRateLabel.Left := Trunc( Width * 0.24 ) + LabelMargin;
  FirstDateLabel.Left := Trunc( Width * 0.32 ) + LabelMargin+10;
  NPeriodsLabel.Left := Trunc( Width * 0.44 ) + LabelMargin;
  LastDateLabel.Left := Trunc( Width * 0.5 ) + LabelMargin+10;
  PerYrLabel.Left := Trunc( Width * 0.62 ) + LabelMargin-5;
  PayAmtLabel.Left := Trunc( Width * 0.68 ) + LabelMargin+5;
  PointsLabel.Left := Trunc( Width * 0.8 ) + LabelMargin+5;
  APRLabel.Left := Trunc( Width * 0.9 ) + LabelMargin+5;
  // grid
  AmortGrid.Width := Width-10;
  AmortGrid.Height := AmortGrid.DefaultRowHeight + CellBorderHeight;
  // advanced Stuff
  AdvancedGroup.Top := AmortGrid.Top + AmortGrid.Height;
  AdvancedGroup.Height := Height - (AdvancedGroup.Top) - 30;
  AdvancedGroup.Width := Width-8;
  PrepaymentGrid.Top := -2;
  PrepaymentGrid.Left := Trunc( Width * 0.325 );
  PrepaymentGrid.Width := Trunc( Width * 0.815 ) - PrepaymentGrid.Left;
  PrepaymentGrid.Height := PrepaymentGrid.DefaultRowHeight * 2 + CellBorderHeight + 1;
  Label4.Left := PrepaymentGrid.Left - 170;
  Label5.Left := PrepaymentGrid.Left - 160;
  // balloon and adjustment
  Label1.Top := PrepaymentGrid.Top + PrepaymentGrid.Height + 30;
  Label1.Left := Trunc(Width*0.06);
  Label6.Top := PrepaymentGrid.Top + PrepaymentGrid.Height + 30;
  Label6.Left := Trunc(Width*0.56);
  Label2.Top := Label1.Top + Label1.Height + 3;
  Label2.Left := Trunc(Width*0.08);
  Label3.Top := Label1.Top + Label1.Height + 3;
  Label7.Top := Label1.Top + Label1.Height + 3;
  Label7.Left := Trunc(Width*0.58);
  Label8.Top := Label1.Top + Label1.Height + 3;
  Label9.Top := Label1.Top + Label1.Height + 3;
  BalloonGrid.Left := Trunc(Width*0.05);
  BalloonGrid.Top := Label2.Top + Label2.Height + 3;
  BalloonGrid.Width := Trunc(Width*0.3);
  Label3.Left := Trunc(Width*0.08) + Trunc(BalloonGrid.Width*0.5);
  // BalloonGrid.Height is based on bottom row stuff, so defer.
  AdjustmentGrid.Left := Trunc(Width*0.55);
  AdjustmentGrid.Top := Label2.Top + Label2.Height + 3;
  AdjustmentGrid.Width := Trunc(Width*0.4);
  Label8.Left := Trunc(Width*0.58) + Trunc(AdjustmentGrid.Width*0.3);
  Label9.Left := Trunc(Width*0.58) + Trunc(AdjustmentGrid.Width*0.6);
  // AdjustmentGrid.Height is based on bottom row stuff, so defer.
  // bottom row
  BottomRowY := AdvancedGroup.Height - PayoffGrid.DefaultRowHeight - Label10.Height - 30;
  BalloonGrid.Height := BottomRowY - BalloonGrid.Top-10;
  AdjustmentGrid.Height := BottomRowY - AdjustmentGrid.Top-10;
  Label10.Top := BottomRowY;
  Label10.Left := Trunc(Width*0.07);
  MoratoriumGrid.Left := Trunc(Width*0.05);
  MoratoriumGrid.Top := BottomRowY + Label10.Height+8;
  MoratoriumGrid.Width := Trunc(Width*0.15);
  MoratoriumGrid.Height := MoratoriumGrid.DefaultRowHeight + CellBorderHeight;
  Label11.Top := BottomRowY;
  Label11.Left := Trunc(Width*0.25);
  TargetGrid.Left := Trunc(Width*0.23);
  TargetGrid.Top := BottomRowY + Label11.Height+8;
  TargetGrid.Width := Trunc(Width*0.15);
  TargetGrid.Height := TargetGrid.DefaultRowHeight + CellBorderHeight;
  Label12.Top := BottomRowY;
  Label12.Left := Trunc(Width*0.41);
  SkipGrid.Left := Trunc(Width*0.41);
  SkipGrid.Top := BottomRowY + Label12.Height+8;
  SkipGrid.Width := Trunc(Width*0.15);
  SkipGrid.Height := SkipGrid.DefaultRowHeight + CellBorderHeight;
  // payoff box
  PayoffBox.Left := Trunc( Width*0.60);
  PayoffBox.Top := BottomRowY + AdvancedGroup.Top;
  PayoffBox.Width := Width - PayoffBox.Left - 20;
  PayoffBox.Height := Height - PayoffBox.Top-37;
  PayoffGrid.Width := PayoffBox.Width-20;
  PayoffGrid.Height := PayoffGrid.DefaultRowHeight + CellBorderHeight;
  Label14.Left := Trunc(PayoffGrid.Width*0.55);
end;

//
// Arrow movement between grids.
//
procedure TAmortizationScreen.AmortGridDownAfterGrid(Sender: TObject);
var
  PeriodicSelect: TGridRect;
begin
  inherited;
  if( not fancy ) then
    PayoffGrid.SetFocus()
  else begin
    if( AmortGrid.Selection.Left < 3 ) then
      PeriodicSelect.Left := 0
    else if( (AmortGrid.Selection.Left >= 3) and (AmortGrid.Selection.Right <= 7 ) ) then
      PeriodicSelect.Left := AmortGrid.Selection.Left - 3
    else
      PeriodicSelect.Left := 4;
    PeriodicSelect.Right := PeriodicSelect.Left;
    PeriodicSelect.Top := 0;
    PeriodicSelect.Bottom := 0;
    PrepaymentGrid.Selection := PeriodicSelect;
    PrepaymentGrid.SetFocus();
  end;
end;

procedure TAmortizationScreen.AmortGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  PayoffGrid.SetFocus();
end;

procedure TAmortizationScreen.PrepaymentGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  BalloonGrid.SetFocus();
end;

procedure TAmortizationScreen.PrepaymentGridUpBeforeGrid(Sender: TObject);
var
  AmortSelect: TGridRect;
begin
  inherited;
  AmortSelect.Left := PrepaymentGrid.Selection.Left + 3;
  AmortSelect.Right := PrepaymentGrid.Selection.Right + 3;
  AmortSelect.Top := 0;
  AmortSelect.Bottom := 0;
  AmortGrid.Selection := AmortSelect;
  AmortGrid.SetFocus();
end;

procedure TAmortizationScreen.BalloonGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  AmortGrid.SetFocus();
end;

procedure TAmortizationScreen.BalloonGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

procedure TAmortizationScreen.BalloonGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  AdjustmentGrid.SetFocus();
end;

procedure TAmortizationScreen.AdjustmentGridDownAfterGrid( Sender: TObject);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

procedure TAmortizationScreen.AdjustmentGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  BalloonGrid.SetFocus();
end;

procedure TAmortizationScreen.AdjustmentGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  BalloonGrid.SetFocus();
end;

procedure TAmortizationScreen.MoratoriumGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  AdjustmentGrid.SetFocus();
end;

procedure TAmortizationScreen.MoratoriumGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  AmortGrid.SetFocus();
end;

procedure TAmortizationScreen.MoratoriumGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  TargetGrid.SetFocus();
end;

procedure TAmortizationScreen.MoratoriumGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  PayoffGrid.SetFocus();
end;

procedure TAmortizationScreen.TargetGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  AmortGrid.SetFocus();
end;

procedure TAmortizationScreen.TargetGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

procedure TAmortizationScreen.TargetGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

procedure TAmortizationScreen.TargetGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  SkipGrid.SetFocus();
end;

procedure TAmortizationScreen.SkipGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  PayoffGrid.SetFocus();
end;

procedure TAmortizationScreen.SkipGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  TargetGrid.SetFocus();
end;

procedure TAmortizationScreen.PayoffGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  if( fancy ) then
    AdjustmentGrid.SetFocus()
  else
    AmortGrid.SetFocus();
end;

procedure TAmortizationScreen.PayoffGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  if( fancy ) then
    SkipGrid.SetFocus();
end;

procedure TAmortizationScreen.PayoffGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  if( fancy ) then
    SkipGrid.SetFocus();
end;

procedure TAmortizationScreen.PrepaymentGridLeftBeforeGrid( Sender: TObject; var Default: boolean );
var
  AmortSelect: TGridRect;
begin
  inherited;
  AmortSelect.Left := 2;
  AmortSelect.Right := 2;
  AmortSelect.Top := 0;
  AmortSelect.Bottom := 0;
  AmortGrid.Selection := AmortSelect;
  AmortGrid.SetFocus();
  Default := false;
end;

procedure TAmortizationScreen.PrepaymentGridRightAfterGrid( Sender: TObject; var Default: Boolean);
var
  AmortSelect: TGridRect;
begin
  inherited;
  AmortSelect.Left := 8;
  AmortSelect.Right := 8;
  AmortSelect.Top := 0;
  AmortSelect.Bottom := 0;
  AmortGrid.Selection := AmortSelect;
  AmortGrid.SetFocus();
  Default := false;
end;

procedure TAmortizationScreen.AllAMZData2Grids();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.AllAMZData2Grids' );
  AMZValues2Grid( h, AmortGrid );
  PayoffValues2Grid( balloonptr(w), PayoffGrid );
  if( AdvancedIsOn ) then begin
    AdjustmentValues2Grid( adj, AdjustmentGrid );
    BalloonValues2Grid( balloon, BalloonGrid );
    PrepaymentValues2Grid( pre, PrepaymentGrid );
    TargetValue2Grid( targ, TargetGrid );
    MoratoriumValue2Grid( mor, MoratoriumGrid );
    SkipValue2Grid( skp, SkipGrid );
  end;
end;

procedure TAmortizationScreen.AmortGridCellBeforeEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String);
begin
  if( ACol <> AMZPerYrCol ) then begin
    if( h.peryrstatus = empty ) then begin
      h.peryr := df.c.peryr;
      h.peryrstatus := defp;
      AmortGrid.SetEditText( AMZPerYrCol, 0, IntToStr(df.c.peryr) );
    end;
  end;
  if( ACol = AMZLastDateCol ) then begin
    h.nstatus := empty;
    h.nperiods := 0;
    AmortGrid.SetCell( '', AMZNPeriodsCol, 0, empty );
  end else if( ACol = AMZNPeriodsCol ) then begin
    h.laststatus := empty;
    h.lastdate := unkdate;
    AmortGrid.SetCell( '', AMZLastDateCol, 0, empty );
  end;
  AssignAMZLoanValues( h, ACol, Value, inp );
end;

procedure TAmortizationScreen.AmortGridCellAfterEdit(Sender: TObject; ACol,
  ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignAMZLoanValues( h, ACol, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

procedure TAmortizationScreen.AssignAMZLoanValues( AMZ: AMZptr; ACol :integer; Value: string; Hardness: inout );
var
  IsError: boolean;
begin
  case ACol of
    AMZAmountCol: begin
      if( Value = '' ) then begin
        AMZ.amount := 0;
        AMZ.amountstatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.amount := StringFormat2Double( Value, IsError );
        AMZ.amountstatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZLoanDateCol: begin
      if( Value = '' ) then begin
        AMZ.loandate := unkdate;
        AMZ.loandatestatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.loandate := StringFormat2Date( Value, IsError );
        AMZ.loandatestatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZLoanRateCol: begin
      if( Value = '' ) then begin
        AMZ.loanrate := 0;
        AMZ.loanratestatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.loanrate := StringFormat2Double( Value, IsError )/100;
        AMZ.loanratestatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZFirstDateCol: begin
      if( Value = '' ) then begin
        AMZ.firstdate := unkdate;
        AMZ.firststatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.firstdate := StringFormat2Date( Value, IsError );
        AMZ.firststatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZNPeriodsCol: begin
      if( Value = '' ) then begin
        AMZ.nperiods := 0;
        AMZ.nstatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.nperiods := StringFormat2Int( Value, IsError );
        AMZ.nstatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZLastDateCol: begin
      if( Value = '' ) then begin
        AMZ.lastdate := unkdate;
        AMZ.laststatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.lastdate := StringFormat2Date( Value, IsError );
        AMZ.laststatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZPerYrCol: begin
      if( Value = '' ) then begin
        AMZ.peryr := 0;
        AMZ.peryrstatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.peryr := StringFormat2Int( Value, IsError );
        AMZ.peryrstatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZPayAmtCol: begin
      if( Value = '' ) then begin
        AMZ.payamt := 0;
        AMZ.payamtstatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.payamt := StringFormat2Double( Value, IsError );
        AMZ.payamtstatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZPointsCol: begin
      if( Value = '' ) then begin
        AMZ.points := 0;
        AMZ.pointsstatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.points := StringFormat2Double( Value, IsError )/100;
        AMZ.pointsstatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    AMZAPRCol: begin
      if( Value = '' ) then begin
        AMZ.apr := 0;
        AMZ.aprstatus := empty;
        AmortGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        AMZ.apr := StringFormat2Double( Value, IsError )/100.0;
        AMZ.aprstatus := Hardness;
        AmortGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AMZValues2Grid( AMZ: AMZPtr; Grid: TPersenseGrid );
var
  NumStr: string;
begin
  if( AMZ.amountstatus <> empty ) then begin
    NumStr := FloatToStr( AMZ.amount );
    AmortGrid.SetCell( NumStr, AMZAmountCol, 0, AMZ.amountstatus );
  end else
    AmortGrid.SetCell( '', AMZAmountCol, 0, empty );

  if( AMZ.loandatestatus <> empty ) then begin
    NumStr := DateToStr( AMZ.loandate );
    AmortGrid.SetCell( NumStr, AMZLoanDateCol, 0, AMZ.loandatestatus );
  end else
    AmortGrid.SetCell( '', AMZLoanDateCol, 0, empty );

  if( AMZ.loanratestatus <> empty ) then begin
    NumStr := FloatToStr( AMZ.loanrate*100 );
    AmortGrid.SetCell( NumStr, AMZLoanRateCol, 0, AMZ.loanratestatus );
  end else
    AmortGrid.SetCell( '', AMZLoanRateCol, 0, empty );

  if( AMZ.firststatus <> empty ) then begin
    NumStr := DateToStr( AMZ.firstdate );
    AmortGrid.SetCell( NumStr, AMZFirstDateCol, 0, AMZ.firststatus );
  end else
    AmortGrid.SetCell( '', AMZFirstDateCol, 0, empty );

  if( AMZ.nstatus <> empty ) then begin
    NumStr := IntToStr( AMZ.nperiods );
    AmortGrid.SetCell( NumStr, AMZNPeriodsCol, 0, AMZ.nstatus );
  end else
    AmortGrid.SetCell( '', AMZNPeriodsCol, 0, empty );

  if( AMZ.laststatus <> empty ) then begin
    NumStr := DateToStr( AMZ.lastdate );
    AmortGrid.SetCell( NumStr, AMZLastDateCol, 0, AMZ.laststatus );
  end else
    AmortGrid.SetCell( '', AMZLastDateCol, 0, empty );

  if( AMZ.peryrstatus <> empty ) then begin
    NumStr := IntToStr( AMZ.peryr );
    AmortGrid.SetCell( NumStr, AMZPerYrCol, 0, AMZ.peryrstatus );
  end else
    AmortGrid.SetCell( '', AMZPerYrCol, 0, empty );

  if( AMZ.payamtstatus <> empty ) then begin
    NumStr := FloatToStr( AMZ.payamt );
    AmortGrid.SetCell( NumStr, AMZPayAmtCol, 0, AMZ.payamtstatus );
  end else
    AmortGrid.SetCell( '', AMZPayAmtCol, 0, empty );

  if( AMZ.pointsstatus <> empty ) then begin
    NumStr := FloatToStr( AMZ.points*100 );
    AmortGrid.SetCell( NumStr, AMZPointsCol, 0, AMZ.pointsstatus );
  end else
    AmortGrid.SetCell( '', AMZPointsCol, 0, empty );

  if( AMZ.aprstatus <> empty ) then begin
    NumStr := FloatToStr( AMZ.apr*100 );
    AmortGrid.SetCell( NumStr, AMZAPRCol, 0, AMZ.aprstatus );
  end else
    AmortGrid.SetCell( '', AMZAPRCol, 0, empty );


end;

procedure TAmortizationScreen.PayoffGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignPayoffValues( balloonptr(w), ACol, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

procedure TAmortizationScreen.AssignPayoffValues( Payoff: balloonptr; ACol: integer; Value: string; Hardness: inout );
var
  IsError: boolean;
begin
  case ACol of
    OFFDateCol: begin
      if( Value = '' ) then begin
        Payoff.date := unkdate;
        Payoff.datestatus := empty;
        PayoffGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        Payoff.date := StringFormat2Date( Value, IsError );
        Payoff.datestatus := Hardness;
        PayoffGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
    OFFAmountCol: begin
      if( Value = '' ) then begin
        Payoff.amount := 0;
        Payoff.amountstatus := empty;
        PayoffGrid.SetCellHardness( ACol, 0, empty );
      end else begin
        Payoff.amount := StringFormat2Double( Value, IsError );
        Payoff.amountstatus := Hardness;
        PayoffGrid.SetCellHardness( ACol, 0, Hardness );
      end;
    end;
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PayoffValues2Grid( Payoff: balloonptr; Grid: TPersenseGrid );
var
  NumStr: string;
begin
  if( Payoff.datestatus <> empty ) then begin
    NumStr := DateToStr( Payoff.date );
    PayoffGrid.SetCell( NumStr, OFFDateCol, 0, Payoff.datestatus );
  end else
    PayoffGrid.SetCell( '', OFFDateCol, 0, empty );

  if( Payoff.amountstatus <> empty ) then begin
    NumStr := FloatToStr( Payoff.amount );
    PayoffGrid.SetCell( NumStr, OFFAmountCol, 0, Payoff.amountstatus );
  end else
    PayoffGrid.SetCell( '', OFFAmountCol, 0, empty );
end;

procedure TAmortizationScreen.AdjustmentGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignAdjustmentValues( adj, ACOl, ARow, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

// keep in mind that index 0 in the table is index 1 in the array
procedure TAmortizationScreen.AssignAdjustmentValues( var ADJ: adjarray; ACol, ARow: integer; Value: string; Hardness: inout );
var
  pADJ: adjptr;
  IsError: boolean;
begin
  // the ADJ array type has start index=1
  pADJ := adjptr(ADJ[ARow+1]);
  case ACol of
    ADJDateCol: begin
      if( Value = '' ) then begin
        pADJ.date := unkdate;
        pADJ.datestatus := empty;
        AdjustmentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pADJ.date := StringFormat2Date( Value, IsError );
        pADJ.datestatus := Hardness;
        AdjustmentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    ADJRateCol: begin
      if( Value = '' ) then begin
        pADJ.loanrate := 0;
        pADJ.loanratestatus := empty;
        AdjustmentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pADJ.loanrate := StringFormat2Double( Value, IsError )/100;
        pADJ.loanratestatus := Hardness;
        AdjustmentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    ADJAmountCol: begin
      if( Value = '' ) then begin
        pADJ.amount := 0;
        pADJ.amountstatus := empty;
        AdjustmentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pADJ.amount := StringFormat2Double( Value, IsError );
        pADJ.amountstatus := Hardness;
        AdjustmentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AdjustmentValues2Grid( ADJ: adjarray; Grid: TPersenseGrid );
var
  pADJ: adjptr;
  NumStr: string;
  i: integer;
begin
  for i:=1 to maxadj do begin
    pADJ := adjptr(ADJ[i]);
    if( not( (i > Grid.GetUsedRowCount()) and AdjustmentIsEmpty( pADJ )) ) then begin
    if( pADJ.datestatus <> empty ) then begin
      NumStr := DateToStr( pADJ.date );
      AdjustmentGrid.SetCell( NumStr, ADJDateCol, i-1, pADJ.datestatus );
    end else
      AdjustmentGrid.SetCell( '', ADJDateCol, i-1, empty );

    if( pADJ.loanratestatus <> empty ) then begin
      NumStr := FloatToStr( pADJ.loanrate*100 );
      AdjustmentGrid.SetCell( NumStr, ADJRateCol, i-1, pADJ.loanratestatus );
    end else
      AdjustmentGrid.SetCell( '', ADJRateCol, i-1, empty );

    if( pADJ.amountstatus <> empty ) then begin
      NumStr := FloatToStr( pADJ.amount );
      AdjustmentGrid.SetCell( NumStr, ADJAmountCol, i-1, pADJ.amountstatus );
    end else
      AdjustmentGrid.SetCell( '', ADJAmountCol, i-1, empty );
    end;
  end;
end;

procedure TAmortizationScreen.BalloonGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignBalloonValues( balloon, ACOl, ARow, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

procedure TAmortizationScreen.AssignBalloonValues( var Balloon: balloonarray; ACol, ARow: integer; Value: string; Hardness: inout );
var
  pBalloon: balloonptr;
  IsError: boolean;
begin
  // the Balloon array type has start index=1
  pBalloon := balloonptr(Balloon[ARow+1]);
  case ACol of
    BALDateCol: begin
      if( Value = '' ) then begin
        pBalloon.date := unkdate;
        pBalloon.datestatus := empty;
        BalloonGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pBalloon.date := StringFormat2Date( Value, IsError );
        pBalloon.datestatus := Hardness;
        BalloonGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    BALAmountCol: begin
      if( Value = '' ) then begin
        pBalloon.amount := 0;
        pBalloon.amountstatus := empty;
        BalloonGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pBalloon.amount := StringFormat2Double( Value, IsError );
        pBalloon.amountstatus := Hardness;
        BalloonGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.BalloonValues2Grid( Balloon: Balloonarray; Grid: TPersenseGrid );
var
  pBalloon: balloonptr;
  NumStr: string;
  i: integer;
begin
  for i:=1 to maxballoon do begin
    pBalloon := balloonptr(Balloon[i]);
    if( not((i > Grid.GetUsedRowCount()) and BalloonIsEmpty( pBalloon )) ) then begin;
    if( pBalloon.datestatus <> empty ) then begin
      NumStr := DateToStr( pBalloon.date );
      BalloonGrid.SetCell( NumStr, BALDateCol, i-1, pBalloon.datestatus );
    end else
      BalloonGrid.SetCell( '', BALDateCol, i-1, empty );

    if( pBalloon.amountstatus <> empty ) then begin
      NumStr := FloatToStr( pBalloon.amount );
      BalloonGrid.SetCell( NumStr, BALAmountCol, i-1, pBalloon.amountstatus );
    end else
      BalloonGrid.SetCell( '', BALAmountCol, i-1, empty );
    end;
  end;
end;

procedure TAmortizationScreen.PrepaymentGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignPrepaymentValues( pre, ACOl, ARow, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

procedure TAmortizationScreen.AssignPrepaymentValues( var Prepayment: prepaymentarray; ACol, ARow: integer; Value: string; Hardness: inout );
var
  pPre: prepaymentptr;
  IsError: boolean;
begin
  // the prepayment array type has start index=1
  pPre := prepaymentptr(Prepayment[ARow+1]);
  case ACol of
    PREStartCol: begin
      if( Value = '' ) then begin
        pPre.startdate := unkdate;
        pPre.startdatestatus := empty;
        PrepaymentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pPre.startdate := StringFormat2Date( Value, IsError );
        pPre.startdatestatus := Hardness;
        PrepaymentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    PRENNCol: begin
      if( Value = '' ) then begin
        pPre.nn := 0;
        pPre.nnstatus := empty;
        PrepaymentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pPre.nn := StringFormat2Int( Value, IsError );
        pPre.nnstatus := Hardness;
        PrepaymentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    PREStopCol: begin
      if( Value = '' ) then begin
        pPre.stopdate := unkdate;
        pPre.stopdatestatus := empty;
        PrepaymentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pPre.stopdate := StringFormat2Date( Value, IsError );
        pPre.stopdatestatus := Hardness;
        PrepaymentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    PREPerYrCol: begin
      if( Value = '' ) then begin
        pPre.peryr := 0;
        pPre.peryrstatus := empty;
        PrepaymentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pPre.peryr := StringFormat2Int( Value, IsError );
        pPre.peryrstatus := Hardness;
        PrepaymentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
    PREPaymentCol: begin
      if( Value = '' ) then begin
        pPre.payment := 0;
        pPre.paymentstatus := empty;
        PrepaymentGrid.SetCellHardness( ACol, ARow, empty );
      end else begin
        pPre.payment := StringFormat2Double( Value, IsError );
        pPre.paymentstatus := Hardness;
        PrepaymentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PrepaymentValues2Grid( Prepayment: prepaymentarray; Grid: TPersenseGrid );
var
  pPre: prepaymentptr;
  NumStr: string;
  i: integer;
begin
  for i:=1 to maxprepay do begin
    pPre := prepaymentptr(Prepayment[i]);
    if( not((i > Grid.GetUsedRowCount()) and PrepaymentIsEmpty( pPre )) ) then begin;
    if( pPre.startdatestatus <> empty ) then begin
      NumStr := DateToStr( pPre.startdate );
      PrepaymentGrid.SetCell( NumStr, PREStartCol, i-1, pPre.startdatestatus );
    end else
      PrepaymentGrid.SetCell( '', PREStartCol, i-1, empty );

    if( pPre.nnstatus <> empty ) then begin
      NumStr := IntToStr( pPre.nn );
      PrepaymentGrid.SetCell( NumStr, PRENNCol, i-1, pPre.nnstatus );
    end else
      PrepaymentGrid.SetCell( '', PRENNCol, i-1, empty );

    if( pPre.stopdatestatus <> empty ) then begin
      NumStr := DateToStr( pPre.stopdate );
      PrepaymentGrid.SetCell( NumStr, PREStopCol, i-1, pPre.stopdatestatus );
    end else
      PrepaymentGrid.SetCell( '', PREStopCol, i-1, empty );

    if( pPre.peryrstatus <> empty ) then begin
      NumStr := IntToStr( pPre.peryr );
      PrepaymentGrid.SetCell( NumStr, PREPerYrCol, i-1, pPre.peryrstatus );
    end else
      PrepaymentGrid.SetCell( '', PREPerYrCol, i-1, empty );

    if( pPre.paymentstatus <> empty ) then begin
      NumStr := FloatToStr( pPre.payment );
      PrepaymentGrid.SetCell( NumStr, PREPaymentCol, i-1, pPre.paymentstatus );
    end else
      PrepaymentGrid.SetCell( '', PREPaymentCol, i-1, empty );
    end;
  end;
end;

procedure TAmortizationScreen.TargetGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignTargetValue( targ, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

procedure TAmortizationScreen.AssignTargetValue( Target: targetptr; Value: string; Hardness: inout );
var
  IsError: boolean;
begin
  if( Value = '' ) then begin
    Target.target := 0;
    Target.targetstatus := empty;
    TargetGrid.SetCellHardness( 0, 0, empty );
  end else begin
    Target.target := StringFormat2Double( Value, IsError );
    Target.targetstatus := Hardness;
    TargetGrid.SetCellHardness( 0, 0, Hardness );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.TargetValue2Grid( Target: targetptr; Grid: TPersenseGrid );
var
  NumStr: string;
begin
  if( Target.targetstatus <> empty ) then begin
    NumStr := FloatToStr( Target.target );
    TargetGrid.SetCell( NumStr, 0, 0, Target.targetstatus );
  end else
    TargetGrid.SetCell( '', 0, 0, empty );
end;

procedure TAmortizationScreen.MoratoriumGridCellAfterEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignMoratoriumValue( mor, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

procedure TAmortizationScreen.AssignMoratoriumValue( Moratorium: moratoriumptr; Value: string; Hardness: inout );
var
  IsError: boolean;
begin
  if( Value = '' ) then begin
    Moratorium.first_repay := unkdate;
    Moratorium.first_repaystatus := empty;
    MoratoriumGrid.SetCellHardness( 0, 0, empty );
  end else begin
    Moratorium.first_repay := StringFormat2Date( Value, IsError );
    Moratorium.first_repaystatus := Hardness;
    MoratoriumGrid.SetCellHardness( 0, 0, Hardness );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.MoratoriumValue2Grid( Moratorium: moratoriumptr; Grid: TPersenseGrid );
var
  NumStr: string;
begin
  if( Moratorium.first_repaystatus <> empty ) then begin
    NumStr := DateToStr( Moratorium.first_repay );
    MoratoriumGrid.SetCell( NumStr, 0, 0, Moratorium.first_repaystatus );
  end else
    MoratoriumGrid.SetCell( '', 0, 0, empty );
end;

procedure TAmortizationScreen.SkipGridCellAfterEdit(Sender: TObject; ACol,
  ARow: Integer; const Value: String; DataChanged: boolean);
begin
  inherited;
  AssignSkipValue( skp, Value, inp );
  if( DataChanged ) then begin
    m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
    SetUnsavedData( true );
  end;
end;

procedure TAmortizationScreen.AssignSkipValue( Skip: skipptr; Value: string; Hardness: inout );
begin
  if( Value = '' ) then begin
    Skip.skipmonths := '';
    Skip.skipstatus := empty;
    SkipGrid.SetCellHardness( 0, 0, empty );
  end else begin
    Skip.skipmonths := Value;
    Skip.skipstatus := Hardness;
    SkipGrid.SetCellHardness( 0, 0, Hardness );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.SkipValue2Grid( Skip: skipptr; Grid: TPersenseGrid );
begin
  if( Skip.skipstatus <> empty ) then
    SkipGrid.SetCell( Skip.skipmonths, 0, 0, Skip.skipstatus )
  else
    SkipGrid.SetCell( '', 0, 0, empty );
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
end;

procedure TAmortizationScreen.AmortGridEditEnterKeyPressed(Sender: TObject;
  ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
begin
  DoCalculation();
  if( ACol = AMZLoanRateCol ) then begin
    AMortGrid.Col := AMZNPeriodsCol;
    DefaultAction := false;
  end else if( ACol = AMZNPeriodsCol ) then begin
    AMortGrid.Col := AMZPayAmtCol;
    DefaultAction := false;
  end;
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PayoffGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  DoCalculation();
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PrepaymentGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  DoCalculation();
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.BalloonGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  DoCalculation();
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
  if( ACol = 1 ) then begin
    DefaultAction := false;
    BalloonGrid.Col := 0;
    BalloonGrid.Row := ARow + 1;
  end;
end;

procedure TAmortizationScreen.AdjustmentGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  DoCalculation();
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.MoratoriumGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  DoCalculation();
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
  if( ACol = 2 ) then begin
    DefaultAction := false;
    AdjustmentGrid.Col := 0;
    AdjustmentGrid.Row := ARow+1;
  end;
end;

procedure TAmortizationScreen.TargetGridEditEnterKeyPressed(
  Sender: TObject; ACol, ARow: Integer; ValueChanged: Boolean;
  var DefaultAction: Boolean);
begin
  DoCalculation();
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.SkipGridEditEnterKeyPressed(Sender: TObject;
  ACol, ARow: Integer; ValueChanged: Boolean; var DefaultAction: Boolean);
begin
  DoCalculation();
  m_UndoBuffer.OverWriteData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenAmortGrid();
var
  ACol: integer;
begin
  for ACol:=AmortGrid.Selection.Left to AmortGrid.Selection.Right do begin
    case ACol of
      AMZAmountCol: h.amountstatus := inp;
      AMZLoanDateCol: h.loandatestatus := inp;
      AMZLoanRateCol: h.loanratestatus := inp;
      AMZFirstDateCol: h.firststatus := inp;
      AMZNPeriodsCol: h.nstatus := inp;
      AMZLastDateCol: h.laststatus := inp;
      AMZPerYrCol: h.peryrstatus := inp;
      AMZPayAmtCol: h.payamtstatus := inp;
      AMZPointsCol: h.pointsstatus := inp;
//      AMZAPRCol: h.aprstatus := inp;  This one is read only
    end;
  end;
  AMZValues2Grid( amzptr(h), AmortGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenPayoffGrid();
var
  ACol: integer;
begin
  for ACol:=PayoffGrid.Selection.Left to PayoffGrid.Selection.Right do begin
    case ACol of
      OFFDateCol: w.datestatus := inp;
      OFFAmountCol: w.amountstatus := inp;
    end;
  end;
  PayoffValues2Grid( balloonptr(w), PayoffGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenPrepaymentGrid();
var
  ACol, ARow: integer;
begin
  for ARow:=PrepaymentGrid.Selection.Top to PrepaymentGrid.Selection.Bottom do begin
    if( ARow <= PrepaymentGrid.GetUsedRowCount() ) then begin
      for ACol:=PrepaymentGrid.Selection.Left to PrepaymentGrid.Selection.Right do begin
        case ACol of
          PREStartCol: pre[ARow+1].startdatestatus := inp;
          PRENNCol: pre[ARow+1].nnstatus := inp;
          PREStopCol: pre[ARow+1].stopdatestatus := inp;
          PREPerYrCol: pre[ARow+1].peryrstatus := inp;
          PREPaymentCol: pre[ARow+1].paymentstatus := inp;
        end;
      end;
    end;
  end;
  PrepaymentValues2Grid( pre, PrepaymentGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenBalloonGrid();
var
  ACol, ARow: integer;
begin
  for ARow:=BalloonGrid.Selection.Top to BalloonGrid.Selection.Bottom do begin
    if( ARow <= BalloonGrid.GetUsedRowCount() ) then begin
      for ACol:=BalloonGrid.Selection.Left to BalloonGrid.Selection.Right do begin
        case ACol of
          BALDateCol:  balloon[ARow+1].datestatus := inp;
          BALAmountCol: balloon[ARow+1].amountstatus := inp;
        end;
      end;
    end;
  end;
  BalloonValues2Grid( balloon, BalloonGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenAdjustmentGrid();
var
  ACol, ARow: integer;
begin
  for ARow:=AdjustmentGrid.Selection.Top to AdjustmentGrid.Selection.Bottom do begin
    if( ARow <= AdjustmentGrid.GetUsedRowCount() ) then begin
      for ACol:=AdjustmentGrid.Selection.Left to AdjustmentGrid.Selection.Right do begin
        case ACol of
          ADJDateCol: adj[ARow+1].datestatus := inp;
          ADJRateCol: adj[ARow+1].loanratestatus := inp;
          ADJAmountCol: adj[ARow+1].amountstatus := inp;
        end;
      end;
    end;
  end;
  AdjustmentValues2Grid( adj, AdjustmentGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenMoratoriumGrid();
begin
  mor.first_repaystatus := inp;
  MoratoriumValue2Grid( mor, MoratoriumGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenTargetGrid();
begin
  targ.targetstatus := inp;
  TargetValue2Grid( targ, TargetGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.HardenSkipGrid();
begin
  skp.skipstatus := inp;
  SkipValue2Grid( skp, SkipGrid );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AmortGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.AmortGridCut();
var
  ACol: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := AmortGrid.Selection;
  AmortGrid.CutToClipboard();
  for ACol:=SelectedRect.Left to SelectedRect.Right do
    AssignAMZLoanValues( h, ACol, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PayoffGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.PayoffGridCut();
var
  ACol: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PayoffGrid.Selection;
  PayoffGrid.CutToClipboard();
  for ACol:=SelectedRect.Left to SelectedRect.Right do
    AssignPayoffValues( balloonptr(w), ACol, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PrepaymentGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.PrepaymentGridCut();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PrepaymentGrid.Selection;
  PrepaymentGrid.CutToClipboard();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignPrepaymentValues( pre, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.BalloonGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.BalloonGridCut();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := BalloonGrid.Selection;
  BalloonGrid.CutToClipboard();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignBalloonValues( balloon, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AdjustmentGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.AdjustmentGridCut();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := AdjustmentGrid.Selection;
  AdjustmentGrid.CutToClipboard();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignAdjustmentValues( adj, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.MoratoriumGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.MoratoriumGridCut();
begin
  MoratoriumGrid.CutToClipboard();
  AssignMoratoriumValue( mor, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.TargetGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.TargetGridCut();
begin
  TargetGrid.CutToClipboard();
  AssignTargetValue( targ, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.SkipGridEditCut(Sender: TObject);
begin OnCut(); end;

procedure TAmortizationScreen.SkipGridCut();
begin
  SkipGrid.CutToClipboard();
  AssignSkipValue( skp, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AmortGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.PayoffGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.PrepaymentGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.BalloonGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.AdjustmentGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.MoratoriumGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.TargetGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.SkipGridEditCopy(Sender: TObject);
begin OnCopy(); end;

procedure TAmortizationScreen.AmortGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.AmortGridPaste();
var
  Col: integer;
  PasteRect: TRect;
begin
  AmortGrid.PasteFromClipboard( PasteRect );
  for Col:=PasteRect.Left to PasteRect.Right do
    AssignAMZLoanValues( h, Col, AmortGrid.Cells[Col,0], AmortGrid.GetCellHardness(Col, 0) );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PayoffGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.PayoffGridPaste();
var
  Col: integer;
  PasteRect: TRect;
begin
  PayoffGrid.PasteFromClipboard( PasteRect );
  for Col:=PasteRect.Left to PasteRect.Right do
    AssignPayoffValues( balloonptr(w), Col, PayoffGrid.Cells[Col,0], PayoffGrid.GetCellHardness(Col, 0) );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PrepaymentGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.PrepaymentGridPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  PrepaymentGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignPrepaymentValues( pre, Col, Row, PrepaymentGrid.Cells[Col,Row], PrepaymentGrid.GetCellHardness(Col, Row) );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.BalloonGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.BalloonGridPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  BalloonGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignBalloonValues( balloon, Col, Row, BalloonGrid.Cells[Col,Row], BalloonGrid.GetCellHardness(Col, Row) );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AdjustmentGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.AdjustmentGridPaste();
var
  Col, Row: integer;
  PasteRect: TRect;
begin
  AdjustmentGrid.PasteFromClipboard( PasteRect );
  for Row:=PasteRect.Top to PasteRect.Bottom do begin
    for Col:=PasteRect.Left to PasteRect.Right do
      AssignAdjustmentValues( adj, Col, Row, AdjustmentGrid.Cells[Col,Row], AdjustmentGrid.GetCellHardness(Col, Row) );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.MoratoriumGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.MoratoriumGridPaste();
var
  PasteRect: TRect;
begin
  MoratoriumGrid.PasteFromClipboard( PasteRect );
  AssignMoratoriumValue( mor, MoratoriumGrid.Cells[0,0], MoratoriumGrid.GetCellHardness(0,0) );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.TargetGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.TargetGridPaste();
var
  PasteRect: TRect;
begin
  TargetGrid.PasteFromClipboard( PasteRect );
  AssignTargetValue( targ, TargetGrid.Cells[0,0], TargetGrid.GetCellHardness(0,0) );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.SkipGridEditPaste(Sender: TObject);
begin OnPaste(); end;

procedure TAmortizationScreen.SkipGridPaste();
var
  PasteRect: TRect;
begin
  SkipGrid.PasteFromClipboard( PasteRect );
  AssignSkipValue( skp, SkipGrid.Cells[0,0], SkipGrid.GetCellHardness(0,0) );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AmortGridDelete();
var
  Col: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := AmortGrid.Selection;
  AmortGrid.DeleteSelected();
  for Col:=SelectedRect.Left to SelectedRect.Right do
    AssignAMZLoanValues( h, Col, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PayoffGridDelete();
var
  ACol: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PayoffGrid.Selection;
  PayoffGrid.DeleteSelected();
  for ACol:=SelectedRect.Left to SelectedRect.Right do
    AssignPayoffValues( balloonptr(w), ACol, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.PrepaymentGridDelete();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := PrepaymentGrid.Selection;
  PrepaymentGrid.DeleteSelected();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignPrepaymentValues( pre, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.BalloonGridDelete();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := BalloonGrid.Selection;
  BalloonGrid.DeleteSelected();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignBalloonValues( balloon, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.AdjustmentGridDelete();
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  SelectedRect := AdjustmentGrid.Selection;
  AdjustmentGrid.DeleteSelected();
  for ARow:=SelectedRect.Top to SelectedRect.Bottom do begin
    for ACol:=SelectedRect.Left to SelectedRect.Right do
      AssignAdjustmentValues( adj, ACol, ARow, '', inp );
  end;
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.MoratoriumGridDelete();
begin
  MoratoriumGrid.DeleteSelected();
  AssignMoratoriumValue( mor, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.TargetGridDelete();
begin
  TargetGrid.DeleteSelected();
  AssignTargetValue( targ, '', inp );
  SetUnsavedData( true );
end;

procedure TAmortizationScreen.SkipGridDelete();
begin
  SkipGrid.DeleteSelected();
  AssignSkipValue( skp, '', inp );
  SetUnsavedData( true );
end;

function TAmortizationScreen.IsBackedUpForHelp(): boolean;
begin
  IsBackedUpForHelp := m_bBackedUpForHelp;
end;

procedure TAmortizationScreen.BackupForHelpSystem();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.BackupForHelpSystem' );
  m_bBackedUpForHelp := true;
  m_BackupFileName := m_FileName;
  m_BackupCaption := Caption;
  m_bBackupUnsaved := HasUnsavedData();
  m_HelpBackupAMZ := h^;
  m_HelpBackupPayoff := w^;
  for i:=1 to maxprepay do
    m_HelpBackupPrepayment[i]^ := pre[i]^;
  for i:=1 to maxballoon do
    m_HelpBackupBalloon[i]^ := Balloon[i]^;
  for i:=1 to maxadj do
    m_HelpBackupAdjustment[i]^ := ADJ[i]^;
  m_HelpBackupMoratorium := Mor^;
  m_HelpBackupTarget := Targ^;
  m_HelpBackupSkip := Skp^;
  m_FileName := '';
  Caption := 'Amortization Help';
  SetUnsavedData( false );
end;

procedure TAmortizationScreen.RestoreFromHelpSystem();
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.RestoreFromHelpSystem' );
  m_bBackedUpForHelp := false;
  h^ := m_HelpBackupAMZ;
  w^ := m_HelpBackupPayoff;
  for i:=1 to maxprepay do
    pre[i]^ := m_HelpBackupPrepayment[i]^;
  for i:=1 to maxballoon do
    balloon[i]^ := m_HelpBackupBalloon[i]^;
  for i:=1 to maxadj do
    ADJ[i]^ := m_HelpBackupAdjustment[i]^;
  Mor^ := m_HelpBackupMoratorium;
  Targ^ := m_HelpBackupTarget;
  Skp^ := m_HelpBackupSkip;
  AllAMZData2Grids();
  m_FileName := m_BackupFileName;
  Caption := m_BackupCaption;
  SetUnsavedData( m_bBackupUnsaved );
end;

procedure TAmortizationScreen.AmortGridVerifyCellString(Sender: TObject;
  ACol, ARow: Integer; Value: String; var IsError: Boolean);
var
  DoubleVal: double;
begin
  IsError := false;
  case ACol of
    AMZLoanRateCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=100.00) or (DoubleVal<=-100) ) then IsError := true;
    end;
    AMZNPeriodsCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( DoubleVal<0 ) then IsError := true;
    end;
    AMZPerYrCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( DoubleVal<0 ) then IsError := true;
    end;
    AMZPointsCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( DoubleVal >= 10 ) then IsError := true;
    end;
  end;
end;

procedure TAmortizationScreen.PrepaymentGridVerifyCellString(
  Sender: TObject; ACol, ARow: Integer; Value: String;
  var IsError: Boolean);
var
  DoubleVal: double;
begin
  IsError := false;
  case ACol of
    PRENNCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( DoubleVal<0 ) then IsError := true;
    end;
    PREPerYrCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( DoubleVal<0 ) then IsError := true;
    end;
  end;
end;

procedure TAmortizationScreen.AdjustmentGridVerifyCellString(
  Sender: TObject; ACol, ARow: Integer; Value: String;
  var IsError: Boolean);
var
  DoubleVal: double;
begin
  IsError := false;
  case ACol of
    ADJRateCol: begin
      DoubleVal := StringFormat2Double( Value, IsError );
      if( IsError ) then exit;
      if( (DoubleVal>=100.00) or (DoubleVal<=-100) ) then IsError := true;
    end;
  end;
end;

procedure TAmortizationScreen.OnContextualHelp();
begin
  HelpSystem.DisplayContents( 'AM_Overview.html' );
end;

procedure TAmortizationScreen.AmortGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    AMZAmountCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZAmountCol) );
    AMZLoanDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZLoanDateCol) );
    AMZLoanRateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZLoanRateCol) );
    AMZFirstDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZFirstDateCol) );
    AMZNPeriodsCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZNPeriodsCol) );
    AMZLastDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZLastDateCol) );
    AMZPerYrCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZPerYrCol) );
    AMZPayAmtCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZPayAmtCol) );
    AMZPointsCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZPointsCol) );
    AMZAPRCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_AMZAPRCol) );
  end;
end;

procedure TAmortizationScreen.PrepaymentGridSelectCell(Sender: TObject;
  ACol, ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    PREStartCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_PREStartCol) );
    PRENNCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_PRENNCol) );
    PREStopCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_PREStopCol) );
    PREPerYrCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_PREPerYrCol) );
    PREPaymentCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_PREPaymentCol) );
  end;
end;

procedure TAmortizationScreen.BalloonGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    BALDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_BALDateCol) );
    BALAmountCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_BALAmountCol) );
  end;
end;

procedure TAmortizationScreen.AdjustmentGridSelectCell(Sender: TObject;
  ACol, ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    ADJDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_ADJDateCol) );
    ADJRateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_ADJRateCol) );
    ADJAmountCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_ADJAmountCol) );
  end;
end;

procedure TAmortizationScreen.MoratoriumGridSelectCell(Sender: TObject;
  ACol, ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_Moratorium) );
end;

procedure TAmortizationScreen.TargetGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_Target) );
end;

procedure TAmortizationScreen.SkipGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_Skip) );
end;

procedure TAmortizationScreen.PayoffGridSelectCell(Sender: TObject; ACol,
  ARow: Integer; var CanSelect: Boolean);
begin
  inherited;
  case ACol of
    OFFDateCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_OFFDateCol) );
    OFFAmountCol: MainForm.SetStatusBarText( HelpSystem.GetHelpString(SA_OFFAmountCol) );
  end;
end;

end.
