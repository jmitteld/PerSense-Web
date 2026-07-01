{ ============================================================================
  AmortizationScreenUnit  --  Per%Sense Amortization screen (UI form)

  PURPOSE
    This unit implements the windowed UI for the Amortization calculator.  It
    is the on-screen front end that lets the user enter a loan and produce an
    amortization schedule.  All the actual financial math lives in the
    Amortize / AMORTOP engine units; this unit is purely the form: it owns the
    grids, marshals values between those grids and the underlying loan-model
    records, triggers a recalculation, and renders the resulting table.

  WHAT THE USER DOES HERE
    The top grid (AmortGrid) is the main "AMZLoan" row: Amount borrowed, Loan
    date, Rate, First payment date, # of periods, Last payment date,
    payments/year, payment amount, points, and APR (read-only output).  The
    user types whatever they know and leaves the unknown blank -- this is the
    "field-presence dispatch" pattern of Per%Sense: the engine solves for the
    blank field (e.g. leave Payment blank to solve the payment; leave Amount
    blank to solve the loan amount; leave Rate blank to solve the rate).

    Pressing Enter / Calculate pushes the grid contents into the model records
    and calls the engine through Enter(no_tab) (see DoCalculation).  TableOutput
    then asks the engine for the period-by-period schedule and shows it in a
    TTableOut child window (or saves it to .txt / .csv).

  ADVANCED ("fancy") OPTIONS
    When Advanced mode is on (the global "fancy" flag, toggled via
    ToggleAdvanced), an extra panel (AdvancedGroup) exposes the irregular-loan
    options, each backed by its own grid and model record/array:
      - Prepayments  (PrepaymentGrid / pre[]) : extra periodic payments between
                       a start and stop date at a given frequency.
      - Balloons     (BalloonGrid / balloon[]): one-time lump payments on dates.
      - Adjustments  (AdjustmentGrid / adj[]) : ARM-style rate and/or payment
                       changes on specific dates.
      - Moratorium   (MoratoriumGrid / mor)   : interest-only deferment until a
                       given first-repay date.
      - Target       (TargetGrid / targ)      : minimum principal reduction per
                       payment.
      - Skip months  (SkipGrid / skp)         : months in which payments are
                       suppressed (string like "6-8,12").
    The Payoff grid (PayoffGrid / w) is always visible and asks the engine for
    the outstanding balance as of a given date.

  GLOBAL MODEL STATE (declared in peData, allocated in Create)
    h        : AMZLoan      -- the main loan row.
    w        : balloonrec   -- the payoff query (reuses the balloon record).
    pre[]    : prepaymentarray
    balloon[]: balloonarray
    adj[]    : adjarray
    mor      : moratoriumrec
    targ     : targetrec
    skp      : skiprec
    fancy    : boolean      -- advanced options on/off.
    nlines[] : per-block used-row counts handed to the engine.
    These are module-level globals (peData) rather than fields because the
    arbitrar.pas / Amortize engine reads them directly.

  HARDNESS (the "inout" status on every field)
    Every value carries a status of empty / defp / inp (see peTypes):
      empty = blank (the field to solve for),
      defp  = a defaulted value (e.g. payments/year filled in for the user),
      inp   = a hard, user-entered value.
    "Hardening" promotes a defaulted cell to inp so the engine treats it as a
    fixed constraint instead of something it may overwrite.

  UNDO / HELP
    m_UndoBuffer snapshots the whole model after every edit for Undo/Redo.
    BackupForHelpSystem / RestoreFromHelpSystem stash and restore the entire
    model so the contextual Help system can load a demo file into this same
    screen without destroying the user's work.

  { Go port: MIXED. This is the DOS/Windows Amortization form; the bulk of its
    150 methods -- grid navigation (*GridUp/Down/Left/Right), cut/copy/paste/
    delete, harden-value, select-cell, resize/print/UI-theming, undo/help backup
    -- are n/a (superseded by the web frontend cmd/persense/static/index.html;
    schedule display is an HTML table, undo/copy are client-side, printing is
    the browser's). The load-bearing bridge to the financial engine is:
      DoCalculation/OnCalculate  -> internal/api/handlers.go:752
                                    (HandleAmortizationCalc); derive-only ->
                                    handleAmortizationDeriveOnly (1241)
      TableOutput                -> the schedule from
                                    internal/finance/amortization/engine.go:139
                                    (Amortize)
      Assign*Values marshalling  -> the pointer-field request binding in
                                    HandleAmortizationCalc (blank field => solve)
      AssignSkipValue / skip     -> internal/finance/amortization/engine.go:2018
                                    (MonthSetFromString) parses the "6-8,12" form
      OpenFile / SaveFile        -> internal/api/import_psn.go:121
                                    (HandleImportPSN) + internal/fileio
    The "hardness" (empty/defp/inp) status is the API's pointer-field presence
    dispatch (omitted JSON field => StatusEmpty). Load-bearing procs carry their
    own cross-ref below; the rest are covered by this note. }
  ============================================================================ }
unit AmortizationScreenUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, CHILDWIN, Grids, PersenseGrid, StdCtrls, Globals, FileIOUnit, Amortize,
  peTypes, AmortizationUndoBufferUnit, TableOutUnit;

{ Column index constants for each grid.  Each grid column maps to one field of
  the corresponding model record; the Assign*/...*2Grid routines below switch on
  these to move a single cell to/from the model. }
const
  // AMZLoan Cols -- columns of the main top loan row (AmortGrid)
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
  // Payoff Cols -- "balance as of" query grid (date, amount-output)
  OFFDateCol                    = 0;
  OFFAmountCol                  = 1;
  // Adjustment Cols -- ARM rate/payment changes (date, new rate, new payment)
  ADJDateCol                    = 0;
  ADJRateCol                    = 1;
  ADJAmountCol                  = 2;
  // balloon cols -- one-time lump payment (date, amount)
  BALDateCol                    = 0;
  BALAmountCol                  = 1;
  // prepayment cols -- recurring extra payments (start, count, stop, /yr, amount)
  PREStartCol                   = 0;
  PRENNCol                      = 1;
  PREStopCol                    = 2;
  PREPerYrCol                   = 3;
  PREPaymentCol                 = 4;

type
  { The amortization screen, an MDI child window.  Published fields below are
    the visual controls auto-wired from the .dfm; the methods are split into
    overrides of the TMDIChild contract (OnCalculate, OnUndo, OpenFile, ...) and
    a set of per-grid helpers that move data between grids and the model. }
  TAmortizationScreen = class(TMDIChild)
    AmortGrid: TPersenseGrid;        // main top loan row (the AMZLoan, model "h")
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
    AdvancedGroup: TGroupBox;        // panel holding all advanced-option grids; shown only when fancy=true
    BalloonGrid: TPersenseGrid;      // one-time lump payments (model "balloon[]")
    PrepaymentGrid: TPersenseGrid;   // recurring extra payments (model "pre[]")
    AdjustmentGrid: TPersenseGrid;   // ARM rate/payment changes (model "adj[]")
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    Label4: TLabel;
    Label5: TLabel;
    Label6: TLabel;
    Label7: TLabel;
    Label8: TLabel;
    Label9: TLabel;
    MoratoriumGrid: TPersenseGrid;   // interest-only-until date (model "mor")
    Label10: TLabel;
    TargetGrid: TPersenseGrid;       // minimum principal reduction/payment (model "targ")
    SkipGrid: TPersenseGrid;         // skip-payment months string (model "skp")
    Label11: TLabel;
    Label12: TLabel;
    PayoffBox: TGroupBox;
    PayoffGrid: TPersenseGrid;       // "balance as of" query (model "w")
    Label13: TLabel;
    Label14: TLabel;
    SaveDialog1: TSaveDialog;        // shared save dialog for .amz files and table .txt/.csv export
    { --- Published .dfm event handlers ---------------------------------------
      The blocks below are grouped by purpose; the implementations carry the
      detailed comments.  In brief:
        *DownAfterGrid / *UpBeforeGrid / *LeftBeforeGrid / *RightAfterGrid :
            arrow-key navigation that hops focus between grids.
        *CellAfterEdit / *CellBeforeEdit : push a just-edited cell into the model.
        *EditEnterKeyPressed : Enter in a cell -> recalculate.
        *EditCopy / *EditCut / *EditPaste : clipboard plumbing per grid.
        *VerifyCellString : per-cell input validation.
        *SelectCell : update the status-bar help text for the focused column. }
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
    constructor Create(AOwner: TComponent); override;        // allocate model records, configure grid columns
    destructor Destroy(); override;                           // free model records and helper objects
    function GetType(): TScreenType; override;                // identifies this as the Amortization screen
    procedure OnCalculate(); override;                        // user pressed Calculate: run engine + snapshot undo
    procedure OnHardenValue(); override;                      // promote focused cells from defaulted to hard input
    procedure OnUndo(); override;                             // restore previous model snapshot
    procedure OnRedo(); override;                             // re-apply an undone snapshot
    procedure OnCut(); override;                              // cut focused grid selection to clipboard + clear model
    procedure OnCopy(); override;                             // copy focused grid selection to clipboard
    procedure OnPaste(); override;                            // paste clipboard into focused grid + into model
    procedure OnDelete(); override;                           // delete focused selection (char in editor, else cells)
    procedure OpenFile( var TheFile: TFileIO ); override;     // load an .amz file into the model and grids
    function SaveFile( FileName, ScreenName : string ): boolean; override;  // save model to an .amz file
    procedure OnPrint(); override;                            // render the screen (loan + advanced + payoff) to printer
    procedure OnContextualHelp(); override;                   // open the amortization help page
    procedure SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont ); override;  // apply theme colors/fonts to every grid
    function AdvancedIsOn(): boolean;                         // true when advanced ("fancy") options are showing
    procedure ToggleAdvanced();                               // flip fancy mode and show/hide the advanced panel
    procedure TableOutput();                                  // build the amortization schedule and show/export it
    function IsBackedUpForHelp(): boolean;                    // true while the model is stashed for the help demo
    procedure BackupForHelpSystem();                          // stash the whole model so help can load a demo file
    procedure RestoreFromHelpSystem();                        // restore the user's model after the help demo
  protected
    m_UndoBuffer: TAmortizationUndoBuffer;   // ring of model snapshots for undo/redo
    m_TableOut: TTableOut;                    // lazily-created child window that displays the schedule table
    m_bBackedUpForHelp: boolean;              // true while help-system backup (below) is live
    m_BackupFileName: string;                 // saved m_FileName during help backup
    m_BackupCaption: string;                  // saved window caption during help backup
    m_bBackupUnsaved: boolean;                // saved unsaved-data flag during help backup
    m_HelpBackupAMZ: AMZLoan;                 // saved main loan row during help backup
    m_HelpBackupPayoff: balloonrec;           // saved payoff query during help backup
    m_HelpBackupPrepayment: prepaymentarray;  // saved prepayments during help backup
    m_HelpBackupBalloon: balloonarray;        // saved balloons during help backup
    m_HelpBackupAdjustment: adjarray;         // saved adjustments during help backup
    m_HelpBackupMoratorium: moratoriumrec;    // saved moratorium during help backup
    m_HelpBackupTarget: targetrec;            // saved target during help backup
    m_HelpBackupSkip: skiprec;                // saved skip-months during help backup
    procedure DoCalculation();                // marshal grids->model, call the engine via Enter(no_tab)
    function GetFocusedGrid(): TPersenseGrid; // returns whichever grid currently has focus (nil if none)
    // harden cells -- promote the focused selection in one grid to status inp
    procedure HardenAmortGrid();
    procedure HardenPayoffGrid();
    procedure HardenPrepaymentGrid();
    procedure HardenBalloonGrid();
    procedure HardenAdjustmentGrid();
    procedure HardenMoratoriumGrid();
    procedure HardenTargetGrid();
    procedure HardenSkipGrid();
    // cut -- copy the focused grid selection to clipboard, then clear it in the model
    procedure AmortGridCut();
    procedure PayoffGridCut();
    procedure PrepaymentGridCut();
    procedure BalloonGridCut();
    procedure AdjustmentGridCut();
    procedure MoratoriumGridCut();
    procedure TargetGridCut();
    procedure SkipGridCut();
    // paste -- paste clipboard cells into the grid, then mirror them into the model
    procedure AmortGridPaste();
    procedure PayoffGridPaste();
    procedure PrepaymentGridPaste();
    procedure BalloonGridPaste();
    procedure AdjustmentGridPaste();
    procedure MoratoriumGridPaste();
    procedure TargetGridPaste();
    procedure SkipGridPaste();
    // delete -- clear the focused grid selection (cells and model), no clipboard
    procedure AmortGridDelete();
    procedure PayoffGridDelete();
    procedure PrepaymentGridDelete();
    procedure BalloonGridDelete();
    procedure AdjustmentGridDelete();
    procedure MoratoriumGridDelete();
    procedure TargetGridDelete();
    procedure SkipGridDelete();
    { Moving data between the underlying model records and the grids.
      Each option type has a symmetric pair:
        Assign*Values  : grid cell (string) -> model field, setting hardness/status.
        *Values2Grid   : model fields -> grid cells, formatting and status colors.
      "Hardness" is the inout status (empty/defp/inp) to stamp on the field;
      an empty Value string clears the field to status "empty" (= solve-for-this). }
    procedure AssignAMZLoanValues( AMZ: AMZPtr; ACol :integer; Value: string; Hardness: inout );  // top loan row: write one column
    procedure AMZValues2Grid( AMZ: AMZPtr; Grid: TPersenseGrid );                                  // top loan row: render all columns
    procedure AssignPayoffValues( Payoff: balloonptr; ACol: integer; Value: string; Hardness: inout );  // payoff query: write one column
    procedure PayoffValues2Grid( Payoff: balloonptr; Grid: TPersenseGrid );                              // payoff query: render columns
    procedure AssignAdjustmentValues( var ADJ: adjarray; ACol, ARow: integer; Value: string; Hardness: inout );  // ARM row: write one cell (ARow+1 -> array)
    procedure AdjustmentValues2Grid( ADJ: adjarray; Grid: TPersenseGrid );                               // ARM rows: render all
    procedure AssignBalloonValues( var Balloon: balloonarray; ACol, ARow: integer; Value: string; Hardness: inout );  // balloon row: write one cell
    procedure BalloonValues2Grid( Balloon: Balloonarray; Grid: TPersenseGrid );                          // balloon rows: render all
    procedure AssignPrepaymentValues( var Prepayment: prepaymentarray; ACol, ARow: integer; Value: string; Hardness: inout );  // prepayment row: write one cell
    procedure PrepaymentValues2Grid( Prepayment: prepaymentarray; Grid: TPersenseGrid );                 // prepayment rows: render all
    procedure AssignTargetValue( Target: targetptr; Value: string; Hardness: inout );                    // target: write the single value
    procedure TargetValue2Grid( Target: targetptr; Grid: TPersenseGrid );                                // target: render
    procedure AssignMoratoriumValue( Moratorium: moratoriumptr; Value: string; Hardness: inout );        // moratorium: write the single date
    procedure MoratoriumValue2Grid( Moratorium: moratoriumptr; Grid: TPersenseGrid );                    // moratorium: render
    procedure AssignSkipValue( Skip: skipptr; Value: string; Hardness: inout );                          // skip-months: write the string
    procedure SkipValue2Grid( Skip: skipptr; Grid: TPersenseGrid );                                      // skip-months: render
    procedure AllAMZData2Grids();                                                                         // push the entire model into every grid (used after load/undo/calc)
  private
  end;

var
  AmortizationScreen: TAmortizationScreen;

implementation

uses peData, intsutil, LogUnit, Main, Printers, HelpSystemUnit;

{$R *.dfm}

{ Create -- constructs the amortization screen.
  Side effects: allocates (GetMem) and zeroes every model record/array in the
  peData globals (h, mor, targ, skp, w, pre[], balloon[], adj[]) plus the
  parallel help-backup arrays, configures the column type/width/position of
  every grid (SetupColumn), starts in non-advanced mode (fancy:=false, advanced
  panel hidden), creates the undo buffer, and primes the status-bar help by
  faking a cell selection.  q is an opaque RBTstart buffer the arbitrar.pas
  engine routines require to be non-nil.  Triggered when the user opens a new
  or existing amortization screen. }
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
  // Configure AmortGrid columns: (col index, left fraction, value type, width).
  // Types drive parsing/formatting/right-justification; APR column is output-only.
  AmortGrid.SetupColumn( AMZAmountCol, 0.0, ColTypeDollar, 19 );
  AmortGrid.SetupColumn( AMZLoanDateCol, 0.12, ColTypeDate, DateCellLength );
  AmortGrid.SetupColumn( AMZLoanRateCol, 0.24, ColType4Real, 13 );
  AmortGrid.SetupColumn( AMZFirstDateCol, 0.32, ColTypeDate, DateCellLength );
  AmortGrid.SetupColumn( AMZNPeriodsCol, 0.44, ColTypeInt, 3 );
  AmortGrid.SetupColumn( AMZLastDateCol, 0.5, ColTypeDate, DateCellLength );
  AmortGrid.SetupColumn( AMZPerYrCol, 0.62, ColTypeInt, 2 );
  AmortGrid.SetupColumn( AMZPayAmtCol, 0.68, ColTypeDollar, 15 );
  AmortGrid.SetupColumn( AMZPointsCol, 0.8, ColType4Real, 10 );
  AmortGrid.SetupColumn( AMZAPRCol, 0.9, ColType4Real, 10 );
  AmortGrid.SetCellReadOnly( AMZAPRCol, 0 );   // APR is computed, never typed
  fancy := false;                              // start in basic (non-advanced) mode
  AdvancedGroup.Visible := false;
  PrepaymentGrid.RowCount := maxprepay;        // size each advanced grid to its model array
  PrepaymentGrid.SetupColumn( PREStartCol, 0.0, ColTypeDate, DateCellLength );
  PrepaymentGrid.SetupColumn( PRENNCol, 0.245, ColTypeInt, 3 );
  PrepaymentGrid.SetupColumn( PREStopCol, 0.365, ColTypeDate, DateCellLength );
  PrepaymentGrid.SetupColumn( PREPerYrCol, 0.615, ColTypeInt, 2 );
  PrepaymentGrid.SetupColumn( PREPaymentCol, 0.735, ColTypeDollar, 15 );
  BalloonGrid.RowCount := maxballoon;
  BalloonGrid.SetupColumn( BALDateCol, 0.0, ColTypeDate, DateCellLength );
  BalloonGrid.SetupColumn( BALAmountCol, 0.5, ColTypeDollar, 18 );
  AdjustmentGrid.RowCount := maxadj;
  AdjustmentGrid.SetupColumn( ADJDateCol, 0.0, ColTypeDate, DateCellLength );
  AdjustmentGrid.SetupColumn( ADJRateCol, 0.33, ColType4Real, 14 );
  AdjustmentGrid.SetupColumn( ADJAmountCol, 0.66, ColTypeDollar, 18 );
  MoratoriumGrid.SetupColumn( 0, 0.0, ColTypeDate, DateCellLength );
  TargetGrid.SetupColumn( 0, 0.0, ColType2Real, 14 );
  SkipGrid.SetupColumn( 0, 0.0, ColTypeString, 15 );
  PayoffGrid.SetupColumn( OFFDateCol, 0.0, ColTypeDate, DateCellLength );
  PayoffGrid.SetupColumn( OFFAmountCol, 0.5, ColTypeDollar, 22 );
  Width := 600;
  Height := 400;
  m_UndoBuffer := TAmortizationUndoBuffer.Create();
  // I have no idea what this thing is for, but it gets used in the
  // functions from arbitrar.pas so I need it to not be nil
  GetMem( q, sizeof(RBTstart) );
  m_bBackedUpForHelp := false;
  // must do this to get the status text going
  AmortGridSelectCell( Self, 0, 0, CanSelect );
  m_TableOut := nil;   // table-output window is created lazily on first TableOutput
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.Create ending' );
end;

{ Destroy -- releases everything Create allocated: the table window, undo buffer,
  and every GetMem'd model record/array.  Triggered when the screen is closed. }
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

{ GetType -- identifies this MDI child as the Amortization screen so the main
  form can route screen-type-specific menu actions here. }
function TAmortizationScreen.GetType(): TScreenType;
begin
  GetType := AmortizationType;
end;

{ FormClose -- delegates to the inherited close handler (unsaved-data prompt etc.). }
procedure TAmortizationScreen.FormClose(Sender: TObject; var Action: TCloseAction);
begin
  OnFormClose( Action );
end;

{ AdvancedIsOn -- whether advanced ("fancy") options are currently active.
  This is the single source of truth used throughout to decide whether the
  advanced grids participate in focus, calculation, save, and rendering. }
function TAmortizationScreen.AdvancedIsOn(): boolean;
begin
  AdvancedIsOn := fancy;
end;

{ ToggleAdvanced -- flips between basic and advanced mode, showing/hiding the
  advanced-options panel, and marks the document dirty.  Triggered from the
  main-form menu (Toggle Advanced Options). }
procedure TAmortizationScreen.ToggleAdvanced();
begin
  fancy := not fancy;
  AdvancedGroup.Visible := fancy;
  SetUnsavedData( true );
end;

{ TableOutput -- produces the full period-by-period amortization schedule.
  Flow:
    1. Enter(no_tab) runs the engine on the current model (no table requested
       yet) and sets errorflag; bails on error.
    2. SufficientDataOnScreen() guards against an under-specified loan.
    3. GetTableOptions asks the user: to file or on-screen, comma-separated or
       fixed-width, using h.peryr for the period frequency.
    4. MakeTable fills a TStringList with the rendered rows.
    5. Either saves to .csv/.txt (with extension-fixup and overwrite prompt) or
       hands the lines to the lazily-created TTableOut child window with the
       appropriate column headers (the "fancy" header set has an extra Payment
       Amount column because advanced loans have variable payments).
  Triggered by the user's "Make Table" action. }
{ Go port: internal/finance/amortization/engine.go:139 (Amortize) produces the
  period-by-period schedule that this renders; delivered to the web frontend as
  the JSON rows from internal/api/handlers.go:752 (HandleAmortizationCalc) and
  rendered as an HTML table in cmd/persense/static/index.html. The fancy header's
  extra Payment-Amount column corresponds to per-row payment in the fancy
  schedule (variable under advanced options). }
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
  Enter( no_tab );             // run engine on current model (errorflag set on failure)
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

{ OnCalculate -- the user pressed Calculate.  Runs the calculation and then
  snapshots the (now solved) model into the undo buffer. }
{ Go port: internal/api/handlers.go:752 (HandleAmortizationCalc) -- the web
  Calculate posts the model and returns the solved schedule; the undo snapshot
  is client-side in cmd/persense/static/index.html. }
procedure TAmortizationScreen.OnCalculate();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnCalculate' );
  DoCalculation();
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
end;

{ DoCalculation -- the core grid->engine->grid round-trip.
  Records how many rows of each advanced block are in use (nlines[]) so the
  engine knows how many prepayments/balloons/adjustments/payoff rows to read,
  invokes the engine via Enter(no_tab) which solves for the blank fields in
  place, then AllAMZData2Grids writes the solved values (including the formerly
  blank one) back into the grids.  Marks the document dirty. }
{ Go port: internal/api/handlers.go:752 (HandleAmortizationCalc), which calls
  internal/finance/amortization/engine.go:139 (Amortize) to solve the blank
  field(s) in place, then returns the solved fields + schedule. The nlines[]
  used-row counts correspond to the lengths of the prepayment/balloon/adjustment
  slices in the request. Derive-only (no schedule) ->
  handleAmortizationDeriveOnly (handlers.go:1241). }
procedure TAmortizationScreen.DoCalculation();
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.DoCalculation' );
  // tell the engine how many used rows are in each multi-row block
  nlines[AMZAdjBlock] := AdjustmentGrid.GetUsedRowCount();
  nlines[AMZballoonblock] := BalloonGrid.GetUsedRowCount();
  nlines[AMZpreblock] := PrepaymentGrid.GetUsedRowCount();
  nlines[AMZBalanceBlock] := PayoffGrid.GetUsedRowCount();
  Enter( no_tab );        // run engine; solves blank fields in place
  AllAMZData2Grids();     // reflect solved values back into the grids
  SetUnsavedData( true );
end;

{ GetFocusedGrid -- returns whichever editable grid currently has keyboard
  focus, or nil.  Advanced-only grids are considered only when fancy is on.
  Used by Copy/Cut/Paste/Harden to target the active grid. }
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

{ OnHardenValue -- "harden" the focused grid's selection: promote those cells'
  status from a soft/default value (defp) to a hard user input (inp) so the
  engine treats them as fixed constraints instead of values it may recompute.
  Dispatches to the per-grid Harden* helper for whichever grid has focus, then
  snapshots undo and marks dirty. }
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

{ OnUndo -- restore the previous model snapshot from the undo buffer into the
  peData globals, then repaint all grids.  No-op (just logs) if there is nothing
  to undo. }
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

{ OnRedo -- re-apply a previously undone snapshot, then repaint all grids. }
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

{ OnCut -- routes a Cut to the focused grid's cut helper (clipboard + clear),
  then snapshots undo and marks dirty. }
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

{ OnCopy -- copies the focused grid's selection to the clipboard.  Read-only,
  so no undo snapshot or dirty flag. }
procedure TAmortizationScreen.OnCopy();
var
  Grid: TPersenseGrid;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnCopy' );
  Grid := GetFocusedGrid();
  if( Grid <> nil ) then
    Grid.CopyToClipboard();
end;

{ OnPaste -- routes a Paste to the focused grid's paste helper (clipboard ->
  grid -> model), then snapshots undo and marks dirty. }
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

{ OnDelete -- the Delete key.  Two modes:
    - If a cell is open in its in-place editor, delete the selected text (or the
      single character to the right of the caret) inside the editor only.
    - Otherwise delete the whole selected grid region via the per-grid Delete
      helper (clears cells and the underlying model fields).
  Either way, snapshots undo and marks dirty. }
procedure TAmortizationScreen.OnDelete();
var
  Grid: TPersenseGrid;
  DelStart, DelEnd: Integer;
  TheText, First, Second: string;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnDelete' );
  Grid := GetFocusedGrid();
  if( Grid = nil ) then exit;
  if( Grid.EditorMode ) then begin    // editing a cell: act on the editor text, not the model
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

{ OpenFile -- loads a saved .amz file into this screen.
  First wipes every grid and zeroes every model record/array, then reads the
  whole model from TheFile (GetAmortizationData), seeds the undo buffer, and --
  if the file's advanced flag disagrees with the current mode -- toggles
  advanced options so the panel matches the file.  Finally renders everything
  and clears the dirty flag.  When the load is part of the help system,
  m_FileName is left blank so the demo isn't mistaken for the user's document.
  Triggered by File>Open. }
{ Go port: internal/api/import_psn.go:121 (HandleImportPSN) + the .amz reader
  internal/fileio/loader.go:107 (LoadAmortizationFile); the web port returns the
  parsed loan+advanced options as a JSON payload for the frontend to populate,
  rather than pushing into grids. }
procedure TAmortizationScreen.OpenFile( var TheFile: TFileIO );
var
  i: integer;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OpenFile' );
  // clean up existing data: blank every grid and zero every model record
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
  // now fill with new data read from the file
  TheFile.GetAmortizationData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  if( not m_bBackedUpForHelp ) then
    m_FileName := TheFile.GetFileName()
  else
    m_FileName := '';
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
  // put the screen in the right mode: match the file's advanced/basic flag
  if( fancy <> (TheFile.GetFancyByte()=1) ) then begin
    MainForm.CalcAmzToggleAdvancedOptions1Execute(Self);
  end;
  AllAMZData2Grids();
  SetUnsavedData( false );
end;

{ SaveFile -- writes the current model to an .amz file.
  If FileName is blank, prompts via SaveDialog1 (and then warns on overwrite);
  otherwise saves silently to the given path.  Ensures the .amz extension,
  creates the file if needed, then SaveAmortization serializes the whole model.
  Returns false if the user cancels or the file can't be created.  Clears the
  dirty flag and remembers the path on success. }
{ Go port: n/a (no writer) -- the web port is import-only; internal/fileio reads
  legacy .amz/.mtg/.pvl files (loader.go) but there is no .amz serializer, so
  this save path has no server-side equivalent. Persistence in the web app is
  the browser's (the frontend can round-trip its own JSON state). }
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

{ OnPrint -- renders the input screen (not the schedule table) to the printer:
  a boxed header row of field captions, the AmortGrid loan row, and -- when in
  advanced mode -- the prepayments, balloon and adjustment grids (paginated
  together side-by-side), then the moratorium/target/skip row, and finally the
  boxed Payoff grid.  Pure layout math in printer units; coordinates are all
  fractions of UsablePageWidth.  Triggered by File>Print. }
procedure TAmortizationScreen.OnPrint();
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
const
  Margin = 80;
begin
  MasterLog.Write( LVL_LOW, 'TAmortizationScreen.OnPrint' );
  Printer.BeginDoc();
  Printer.Canvas.Font.Assign( Font );
  Printer.Title := 'Persense Amortization Screen';
  TextHeight := Printer.Canvas.TextHeight( 'Amount' );
  UsablePageWidth := Printer.PageWidth-(2*Margin);
  // top line, the AMZLoan grid.
  yPos := 100;
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
  Printer.Canvas.MoveTo( Margin, Margin );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, Margin );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, yPos + TextHeight );
  Printer.Canvas.LineTo( Margin, yPos + TextHeight );
  Printer.Canvas.LineTo( Margin, Margin );
  // now the AmortGrid
  yPos := yPos + Trunc( 1.5*TextHeight);
  CellHeight := Printer.PageHeight-yPos-2*Margin;
  AmortGrid.Print( 0, yPos, Margin, UsablePageWidth, CellHeight, 1, RowsPrinted );
  if( fancy ) then begin
    yPos := yPos + CellHeight;
    // print the PrepaymentsGrid
    CellHeight := Printer.PageHeight-yPos-2*Margin;
    PrepaymentGrid.Print( 0, yPos, Trunc(UsablePageWidth*0.337), Trunc(UsablePageWidth*0.48), CellHeight, maxprepay, RowsPrinted );
    BalloonPosition := yPos + CellHeight + Trunc(1.5*TextHeight);
    yPos := yPos + Trunc( 0.5*TextHeight );
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.08), yPos, 'Additional Periodic Payments:' );
    yPos := yPos + TextHeight;
    Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.08), yPos, '(0 for regular payments)' );
    // find the last non-empty row in the balloon and adjustment arrays so we
    // only print the used portion of each
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
    // now we know how many rows need to be printed for balloon and adj.
    // Walk both side by side, paginating when either overflows the page.
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
        yPos := 100;
      end else if( ADJCellHeight > CellHeight ) then
        yPos := yPos + ADJCellHeight
      else
        yPos := yPos + CellHeight;
    end;
    if( (yPos + 5*TextHeight) > Printer.PageHeight ) then begin
      // we may be at the end of the page with no room to put the last line
      Printer.NewPage();
      yPos := 100;
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

{ SetUISettings -- applies the user's chosen theme (input-cell color, output-cell
  color, selection color, and the two fonts) uniformly to every grid on the
  screen, repaints them, and re-lays-out via FormResize.  Triggered when display
  preferences change. }
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

{ FormResize -- repositions and resizes every label, grid, and group box as a
  function of the form's Width/Height.  All placements are fractions of Width so
  the layout scales with the window: the loan row spans the top, the advanced
  panel fills the middle (prepayments top, balloon+adjustment columns, then the
  moratorium/target/skip bottom row), and the payoff box sits at lower right.
  Triggered on every resize and after theme/visibility changes. }
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
  PrepaymentGrid.Left := Trunc( Width * 0.32 );
  PrepaymentGrid.Width := Trunc( Width * 0.8 ) - PrepaymentGrid.Left;
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
// Each PersenseGrid fires these when the caret tries to leave the grid via an
// arrow key (Down past the last row, Up before the first, Left/Right past an
// edge).  The handlers move keyboard focus to the logically adjacent grid so
// the cluster of grids behaves like one tab-stop surface.  Several handlers
// also translate the column index across grids whose columns line up visually
// (e.g. AmortGrid cols 3..7 align with PrepaymentGrid cols 0..4).
//
{ AmortGridDownAfterGrid -- Down from the top loan row.  In basic mode go to the
  Payoff grid.  In advanced mode go to the Prepayment grid, carrying the column
  across: AmortGrid's periodic columns (First date..Payment, cols 3..7) map onto
  PrepaymentGrid cols 0..4; anything left of that lands in col 0, anything right
  lands in the last prepayment column. }
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

{ AmortGridUpBeforeGrid -- Up from the top row wraps to the Payoff grid. }
procedure TAmortizationScreen.AmortGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  PayoffGrid.SetFocus();
end;

{ PrepaymentGridDownAfterGrid -- Down from prepayments goes to the balloon grid. }
procedure TAmortizationScreen.PrepaymentGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  BalloonGrid.SetFocus();
end;

{ PrepaymentGridUpBeforeGrid -- Up from prepayments returns to the loan row,
  mapping prepayment cols 0..4 back onto AmortGrid cols 3..7 (the inverse of
  AmortGridDownAfterGrid). }
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

{ BalloonGridUpBeforeGrid -- Up from balloons goes to the loan row. }
procedure TAmortizationScreen.BalloonGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  AmortGrid.SetFocus();
end;

{ BalloonGridDownAfterGrid -- Down from balloons goes to the moratorium grid. }
procedure TAmortizationScreen.BalloonGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

{ BalloonGridRightAfterGrid -- Right from balloons crosses to the adjustment grid. }
procedure TAmortizationScreen.BalloonGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  AdjustmentGrid.SetFocus();
end;

{ AdjustmentGridDownAfterGrid -- Down from adjustments goes to the moratorium grid. }
procedure TAmortizationScreen.AdjustmentGridDownAfterGrid( Sender: TObject);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

{ AdjustmentGridUpBeforeGrid -- Up from adjustments goes to the balloon grid. }
procedure TAmortizationScreen.AdjustmentGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  BalloonGrid.SetFocus();
end;

{ AdjustmentGridLeftBeforeGrid -- Left from adjustments crosses back to balloons. }
procedure TAmortizationScreen.AdjustmentGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  BalloonGrid.SetFocus();
end;

{ MoratoriumGridUpBeforeGrid -- Up from moratorium goes to the adjustment grid. }
procedure TAmortizationScreen.MoratoriumGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  AdjustmentGrid.SetFocus();
end;

{ MoratoriumGridDownAfterGrid -- Down from the bottom row wraps up to the loan row. }
procedure TAmortizationScreen.MoratoriumGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  AmortGrid.SetFocus();
end;

{ MoratoriumGridRightAfterGrid -- Right along the bottom row goes to target. }
procedure TAmortizationScreen.MoratoriumGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  TargetGrid.SetFocus();
end;

{ MoratoriumGridLeftBeforeGrid -- Left from moratorium wraps to the payoff grid. }
procedure TAmortizationScreen.MoratoriumGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  PayoffGrid.SetFocus();
end;

{ TargetGridDownAfterGrid -- Down from target wraps up to the loan row. }
procedure TAmortizationScreen.TargetGridDownAfterGrid(Sender: TObject);
begin
  inherited;
  AmortGrid.SetFocus();
end;

{ TargetGridUpBeforeGrid -- Up from target goes to the moratorium grid. }
procedure TAmortizationScreen.TargetGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

{ TargetGridLeftBeforeGrid -- Left from target goes to the moratorium grid. }
procedure TAmortizationScreen.TargetGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  MoratoriumGrid.SetFocus();
end;

{ TargetGridRightAfterGrid -- Right from target goes to the skip-months grid. }
procedure TAmortizationScreen.TargetGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  SkipGrid.SetFocus();
end;

{ SkipGridRightAfterGrid -- Right from skip-months wraps to the payoff grid. }
procedure TAmortizationScreen.SkipGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  PayoffGrid.SetFocus();
end;

{ SkipGridLeftBeforeGrid -- Left from skip-months goes back to target. }
procedure TAmortizationScreen.SkipGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  TargetGrid.SetFocus();
end;

{ PayoffGridUpBeforeGrid -- Up from payoff: to the last advanced grid
  (adjustments) when advanced, otherwise straight to the loan row. }
procedure TAmortizationScreen.PayoffGridUpBeforeGrid(Sender: TObject);
begin
  inherited;
  if( fancy ) then
    AdjustmentGrid.SetFocus()
  else
    AmortGrid.SetFocus();
end;

{ PayoffGridLeftBeforeGrid -- Left from payoff wraps to skip-months (advanced only). }
procedure TAmortizationScreen.PayoffGridLeftBeforeGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  if( fancy ) then
    SkipGrid.SetFocus();
end;

{ PayoffGridRightAfterGrid -- Right from payoff wraps to skip-months (advanced only). }
procedure TAmortizationScreen.PayoffGridRightAfterGrid(Sender: TObject;
  var Default: Boolean);
begin
  inherited;
  if( fancy ) then
    SkipGrid.SetFocus();
end;

{ PrepaymentGridLeftBeforeGrid -- Left off the prepayment grid jumps to the
  AmortGrid Rate column (col 2) and suppresses the grid's default move. }
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

{ PrepaymentGridRightAfterGrid -- Right off the prepayment grid jumps to the
  AmortGrid Points column (col 8) and suppresses the grid's default move. }
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

{ AllAMZData2Grids -- repaints every grid from the current model.  Always
  refreshes the loan and payoff grids; the advanced grids only when advanced
  mode is on.  Called after any operation that changes the model wholesale
  (calculate, undo/redo, file open, help restore). }
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

{ AmortGridCellBeforeEdit -- fires as the user begins editing a loan-row cell.
  Two pieces of model-keeping happen before the new value is stored:
   1. If the user is editing any column other than payments/year and per-year is
      still blank, auto-fill it with the configured default (df.c.peryr) at the
      soft "defp" hardness so the loan has a frequency to work with.
   2. The "# of periods" and "last payment date" fields are mutually exclusive
      ways of stating term length: editing one clears the other (so the engine
      doesn't see a conflicting over-determined term).
  Finally stamps the new value into the model at hard "inp" status. }
procedure TAmortizationScreen.AmortGridCellBeforeEdit(Sender: TObject;
  ACol, ARow: Integer; const Value: String);
begin
  if( ACol <> AMZPerYrCol ) then begin
    if( h.peryrstatus = empty ) then begin
      h.peryr := df.c.peryr;             // default payments/year (soft)
      h.peryrstatus := defp;
      AmortGrid.SetEditText( AMZPerYrCol, 0, IntToStr(df.c.peryr) );
    end;
  end;
  if( ACol = AMZLastDateCol ) then begin
    h.nstatus := empty;                  // editing last-date clears #periods
    h.nperiods := 0;
    AmortGrid.SetCell( '', AMZNPeriodsCol, 0, empty );
  end else if( ACol = AMZNPeriodsCol ) then begin
    h.laststatus := empty;               // editing #periods clears last-date
    h.lastdate := unkdate;
    AmortGrid.SetCell( '', AMZLastDateCol, 0, empty );
  end;
  AssignAMZLoanValues( h, ACol, Value, inp );
end;

{ AmortGridCellAfterEdit -- fires when a loan-row cell edit is committed.  Writes
  the value into the model (hard input), and if the data actually changed,
  snapshots undo and marks dirty. }
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

{ AssignAMZLoanValues -- writes a single loan-row column from its grid-cell
  string into the AMZLoan model record.  For every column: an empty string sets
  the field to its zero/unknown value with status "empty" (= the field to solve
  for); a non-empty string is parsed (StringFormat2Double/Date/Int) and stamped
  with the given Hardness.  Note the rate and points columns are entered as
  percentages and divided by 100 to store the fraction the engine expects.
  IsError from the parsers is intentionally ignored here -- cell-level
  validation already happened in the Verify*CellString handlers. }
{ Go port: this per-field marshalling is the pointer-field binding in
  internal/api/handlers.go:752 (HandleAmortizationCalc): an omitted JSON field
  => StatusEmpty (the field to solve), a present value => a hard input. The
  rate/points percent-to-fraction /100 conversion happens at the same API
  boundary. Field-presence dispatch is documented in CLAUDE.md. }
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

{ AMZValues2Grid -- renders the whole AMZLoan record into the loan-row grid.
  Each field: if its status is non-empty, format the value (percentages are
  multiplied back by 100 for display) and write it with its status (which the
  grid uses to color input vs. output/solved cells); otherwise blank the cell. }
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

{ PayoffGridCellAfterEdit -- commit an edit in the "balance as of" grid into the
  payoff record (model w); on real change, snapshot undo and mark dirty. }
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

{ AssignPayoffValues -- write one payoff-grid column (the as-of date, or the
  balance-output amount) into the payoff record, same empty/parse/stamp pattern
  as AssignAMZLoanValues.  The payoff query reuses the balloonrec layout. }
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

{ PayoffValues2Grid -- render the payoff record (date + computed balance) into
  its grid; blank cells whose status is empty. }
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

{ AdjustmentGridCellAfterEdit -- commit an ARM adjustment-row cell into adj[];
  snapshot undo and mark dirty on real change. }
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

{ AssignAdjustmentValues -- write one cell of one ARM adjustment row (date, new
  rate, or new payment) into the adj[] array.  An adjustment makes the schedule
  re-amortize from that date at the new rate and/or new payment.  Rate is a
  percentage stored as a fraction (/100). }
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

{ AdjustmentValues2Grid -- render every adjustment row into the grid.  Array is
  1-based, grid is 0-based, so row i maps to grid row i-1.  The guard skips
  rows that are both beyond the used-row count AND empty, so it doesn't paint
  blank trailing rows but still refreshes any row that holds data. }
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

{ BalloonGridCellAfterEdit -- commit a balloon-row cell into balloon[]; snapshot
  undo and mark dirty on real change. }
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

{ AssignBalloonValues -- write one cell of one balloon row (date or lump amount)
  into balloon[].  A balloon is a one-time extra payment applied on its date.
  Array is 1-based (ARow+1). }
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

{ BalloonValues2Grid -- render every balloon row (i -> grid row i-1), skipping
  trailing rows that are both unused and empty. }
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

{ PrepaymentGridCellAfterEdit -- commit a prepayment-row cell into pre[];
  snapshot undo and mark dirty on real change. }
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

{ AssignPrepaymentValues -- write one cell of one prepayment row into pre[].
  A prepayment row describes recurring extra payments: start date, count (nn),
  stop date, payments/year, and the extra-payment amount.  Array is 1-based
  (ARow+1).  NOTE: the payment column is parsed with StringFormat2Int, so extra
  payments are taken as whole-dollar (integer) amounts. }
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
        pPre.payment := StringFormat2Int( Value, IsError );
        pPre.paymentstatus := Hardness;
        PrepaymentGrid.SetCellHardness( ACol, ARow, Hardness );
      end;
    end;
  end;
  SetUnsavedData( true );
end;

{ PrepaymentValues2Grid -- render every prepayment row (i -> grid row i-1),
  skipping trailing rows that are both unused and empty. }
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

{ TargetGridCellAfterEdit -- commit the single target value into targ; snapshot
  undo and mark dirty on real change. }
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

{ AssignTargetValue -- write the single Target value (minimum principal that
  each payment must retire) into the targetrec, or clear it when blank. }
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

{ TargetValue2Grid -- render the target value into its single-cell grid. }
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

{ MoratoriumGridCellAfterEdit -- commit the moratorium date into mor; snapshot
  undo and mark dirty on real change. }
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

{ AssignMoratoriumValue -- write the moratorium's first-repayment date into mor,
  or clear it.  Until this date the loan is interest-only (payments cover
  interest only, principal is deferred). }
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

{ MoratoriumValue2Grid -- render the moratorium date into its single-cell grid. }
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

{ SkipGridCellAfterEdit -- commit the skip-months string into skp; snapshot undo
  and mark dirty on real change. }
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

{ AssignSkipValue -- store the raw skip-months string verbatim (e.g. "6-8,12")
  into skp; the engine parses the ranges.  No numeric conversion happens here. }
{ Go port: internal/finance/amortization/engine.go:2018 (MonthSetFromString) is
  the range/list parser for the "6-8,12" skip-months string; the raw string is
  carried through the api request in internal/api/handlers.go:752
  (HandleAmortizationCalc). }
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

{ SkipValue2Grid -- render the skip-months string into its single-cell grid.
  Also snapshots undo (this is the last grid AllAMZData2Grids touches). }
procedure TAmortizationScreen.SkipValue2Grid( Skip: skipptr; Grid: TPersenseGrid );
begin
  if( Skip.skipstatus <> empty ) then
    SkipGrid.SetCell( Skip.skipmonths, 0, 0, Skip.skipstatus )
  else
    SkipGrid.SetCell( '', 0, 0, empty );
  m_UndoBuffer.StoreData( h, balloonptr(w), pre, balloon, adj, mor, targ, skp );
end;

{ *EditEnterKeyPressed handlers -- Enter inside any grid cell triggers a full
  recalculation (DoCalculation) and overwrites the latest undo snapshot (so the
  pre- and post-calc states aren't two separate undo steps).
  AmortGridEditEnterKeyPressed additionally implements a guided tab order on the
  loan row: after Rate, jump to # of periods; after # of periods, jump to
  Payment -- suppressing the default Enter behavior in those two cases. }
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

{ Harden* helpers -- for each selected cell in the focused grid, set the
  corresponding model field's status to inp (hard user input).  This locks a
  value the engine had defaulted or solved so a subsequent calculation treats it
  as a fixed constraint.  Each then re-renders its grid.  HardenAmortGrid skips
  the APR column because it is output-only. }
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

{ HardenPrepaymentGrid -- like HardenAmortGrid but over a 2-D selection; only
  rows within the used-row count are hardened (so blank trailing rows are left
  alone).  pre[] is 1-based, hence ARow+1. }
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

{ Clipboard plumbing.  Each grid has a thin *EditCut/*EditCopy/*EditPaste event
  handler (the published .dfm hook) that just forwards to the shared OnCut/
  OnCopy/OnPaste dispatcher; the dispatcher then calls the matching *Cut/*Paste
  worker below.  The workers move data both to/from the clipboard AND keep the
  model in sync: Cut copies then clears the model cells via Assign*(...,''),
  Paste pastes then mirrors the pasted cells back into the model. }
procedure TAmortizationScreen.AmortGridEditCut(Sender: TObject);
begin OnCut(); end;

{ AmortGridCut -- copy the loan-row selection to the clipboard, then blank those
  columns in the model. }
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

// Copy hooks: all forward to OnCopy (which copies the focused grid's selection).
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

// Paste hooks: all forward to OnPaste.
procedure TAmortizationScreen.AmortGridEditPaste(Sender: TObject);
begin OnPaste(); end;

{ AmortGridPaste -- paste clipboard cells into the loan row, then read each
  pasted cell (with the hardness the grid assigned it) back into the model so
  the model and grid stay consistent.  Same shape for every grid's *Paste. }
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

{ *Delete workers -- clear the selected region without touching the clipboard:
  DeleteSelected blanks the grid cells, then Assign*(...,'') clears the matching
  model fields (setting them back to status empty).  Same shape for every grid. }
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

{ IsBackedUpForHelp -- true while the user's model is stashed because the help
  system has loaded a demo into this screen. }
function TAmortizationScreen.IsBackedUpForHelp(): boolean;
begin
  IsBackedUpForHelp := m_bBackedUpForHelp;
end;

{ BackupForHelpSystem -- deep-copies the entire model (loan, payoff, all advanced
  arrays/records) plus the filename/caption/dirty state into the m_HelpBackup*
  fields, then clears filename and retitles the window "Amortization Help".  This
  lets the contextual help system load an example .amz into this same live screen
  without losing the user's work; RestoreFromHelpSystem puts it all back. }
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

{ RestoreFromHelpSystem -- the inverse of BackupForHelpSystem: copies every
  m_HelpBackup* value back into the live model, repaints all grids, and restores
  the saved filename/caption/dirty flag.  Called when the help demo is dismissed. }
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

{ *VerifyCellString handlers -- per-cell input validation called by the grid as
  the user types/commits, setting IsError to reject the value before it reaches
  the model.  Rules:
    AmortGrid : rate in (-100,100)%, periods and payments/year non-negative,
                points < 10%.
    Prepayment: count (nn) and payments/year non-negative.
    Adjustment: new rate in (-100,100)%.
  A parse failure (non-numeric) also flags IsError. }
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

{ OnContextualHelp -- opens the amortization overview help page in the help
  viewer.  Triggered by F1 / Help on this screen. }
procedure TAmortizationScreen.OnContextualHelp();
begin
  HelpSystem.DisplayContents( 'AM_Overview.html' );
end;

{ *SelectCell handlers -- fire whenever the focused cell changes; they look up
  the help string for the focused column and show it in the main window's status
  bar, giving the user a one-line description of the field they are on.  The
  single-column advanced grids (moratorium/target/skip) have one fixed string. }
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
