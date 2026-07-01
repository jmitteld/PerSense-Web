{ ===========================================================================
  Unit:  CHILDWIN
  Role:  Abstract base class for every MDI child window in Per%Sense.

  All of the application's document windows (Welcome, Mortgage, Amortization,
  Present Value, table-output report, APR report) descend from TMDIChild. It
  centralises the cross-cutting behaviour the MAIN form needs to drive
  uniformly through a common interface:
    - identity            (GetType - each subclass declares its kind)
    - edit/command verbs  (OnCalculate, OnUndo/OnRedo, OnCut/Copy/Paste/Delete,
                           OnHardenValue, OnPrint, OnContextualHelp)
    - file persistence    (OpenFile / SaveFile, m_FileName)
    - UI theming          (SetUISettings - colours and fonts pushed from prefs)
    - dirty tracking       (HasUnsavedData / SetUnsavedData and the
                           save-prompt-on-close flow CheckForUnsavedDataAndFree)
    - lifecycle plumbing   (OnFormClose hides rather than destroys; the actual
                           free happens via CheckForUnsavedDataAndFree, with
                           ReallyShow/ReallyHide toggling true visibility)
  Most command methods are empty virtual no-ops here; concrete screens override
  only the ones they support. The OnChildClosing event lets MAIN react when a
  child requests to close.

  { Go port: n/a -- abstract MDI-child framework (window lifecycle, dirty
    tracking, command-verb dispatch); no financial logic. The web port has no
    MDI shell: each calculation screen is a panel in
    cmd/persense/static/index.html and command verbs (calculate/undo/copy/print)
    are handled client-side or by a stateless POST to the api endpoints. The
    OpenFile/SaveFile verbs correspond to the .psn import path in
    internal/api/import_psn.go:121 (HandleImportPSN) + internal/fileio. }
  ===========================================================================}
unit CHILDWIN;

interface

uses Windows, Classes, Controls, StdCtrls, Forms, Graphics, FileIOUnit;

type
// Types for the different screens
  { Enumerates every concrete MDI child kind; returned by GetType so MAIN can
    branch on which screen is active (menu state, dispatch, etc.). }
  TScreenType = ( MortgageType, AmortizationType, PresentValueType, WelcomeType,
                  TableOutType, APRReportType );

  TMDIChild = class( TForm )
  private
    { fired when this child wants to close }
    FOnChildClosing: TNotifyEvent;
    { dirty flag - true if edits unsaved }
    m_bUnsavedData: boolean;
  public
    constructor Create(AOwner: TComponent); override;
    { Subclass MUST report its screen kind. }
    function GetType(): TScreenType; virtual; abstract;
    { Command-verb hooks: base versions are no-ops; screens override as needed }
    { run this screen's calculation }
    procedure OnCalculate(); virtual;
    { commit a computed value as input }
    procedure OnHardenValue(); virtual;
    { undo last edit }
    procedure OnUndo(); virtual;
    { redo last undone edit }
    procedure OnRedo(); virtual;
    { clipboard cut }
    procedure OnCut(); virtual;
    { clipboard copy }
    procedure OnCopy(); virtual;
    { clipboard paste }
    procedure OnPaste(); virtual;
    { delete current selection }
    procedure OnDelete(); virtual;
    { load doc }
    procedure OpenFile( var TheFile: TFileIO ); virtual;
    { save }
    function SaveFile( FileName, ScreenName : string ): boolean; virtual;
    { print this screen's output }
    procedure OnPrint(); virtual;
    procedure OnContextualHelp(); virtual;{ show F1/context help }
    { Apply user-preference colours and fonts to this screen's grid/output. }
    procedure SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont ); virtual;
    { True if the document has unsaved changes. }
    function HasUnsavedData(): boolean;
    { Prompt to save if dirty, then free the window; returns false if the user
      cancelled (window kept open). }
    function CheckForUnsavedDataAndFree(): boolean;
    { Current document file name (empty if never saved). }
    function GetFileName(): string;
    { Force the native window visible/hidden (bypasses VCL Show/Hide). }
    procedure ReallyShow();
    procedure ReallyHide();
  protected
    { backing store for the document file name }
    m_FileName: string;
    { Standard FormClose handler logic shared by all children. }
    procedure OnFormClose(var Action: TCloseAction);
    { Set/clear the dirty flag. }
    procedure SetUnsavedData( Unsaved: boolean );
  published
    { Notify the owner (MAIN) that this child is closing. }
    property OnChildClosing : TNotifyEvent read FOnChildClosing write FOnChildClosing;
  end;

implementation

{$R *.dfm}

uses Dialogs, Globals;

{ Create
  Purpose: base construction; start with a clean (not-dirty) document.
  Side effects: m_bUnsavedData := false. }
constructor TMDIChild.Create(AOwner: TComponent);
begin
  inherited Create( AOwner );
  m_bUnsavedData := false;
end;

{ OnFormClose
  Purpose: shared close handler - do NOT destroy on close; instead hide the
           window and notify the owner so MAIN can run the save/free flow.
  Param:   Action - set to caNone so the VCL keeps the form alive.
  Side effects: hides the window; raises OnChildClosing if assigned.
  NOTE: actual destruction is deferred to CheckForUnsavedDataAndFree. }
procedure TMDIChild.OnFormClose(var Action: TCloseAction);
begin
  ReallyHide();
  Action := caNone;
  if( Assigned(FOnChildClosing) ) then
    OnChildClosing( Self );
end;

{ HasUnsavedData - returns the current dirty flag. }
function TMDIChild.HasUnsavedData(): boolean;
begin
  HasUnsavedData := m_bUnsavedData;
end;

{ SetUnsavedData - set/clear the dirty flag (Unsaved=true marks edits pending) }
procedure TMDIChild.SetUnsavedData( Unsaved: boolean );
begin
  m_bUnsavedData := Unsaved;
end;

{ CheckForUnsavedDataAndFree
  Purpose: safely close the window, prompting to save first if dirty.
  Returns: true if the window was freed; false if the user cancelled.
  Flow:    if dirty, ask Yes/No/Cancel. Cancel aborts (keep window). Yes saves
           (and aborts if the save itself fails). Otherwise/No proceeds to Free.
  Side effects: may show a dialog, call SaveFile, and Free() this instance. }
function TMDIChild.CheckForUnsavedDataAndFree(): boolean;
var
  bCanFree: boolean;
  RetVal: integer;
begin
  bCanFree := true;
  if( HasUnsavedData() ) then begin
    RetVal := MessageDlg( 'Would you like to save?', mtInformation, [mbYes, mbNo, mbCancel], 0 );
    if( RetVal = DLG_CANCEL ) then
      bCanFree := false
    else if( RetVal = DLG_YES ) then
      if( not SaveFile( m_FileName, '' ) ) then
        bCanFree := false;
  end;
  if( bCanFree ) then
    Free();
  CheckForUnsavedDataAndFree := bCanFree;
end;

{ GetFileName - return the document's file name (empty until first saved). }
function TMDIChild.GetFileName(): string;
begin
  GetFileName := m_FileName;
end;

{ ReallyShow - force native visibility via the Win32 API (bypasses VCL). }
procedure TMDIChild.ReallyShow();
begin
  ShowWindow( Handle, SW_SHOW );
end;

{ ReallyHide - force native hidden state via the Win32 API (bypasses VCL). }
procedure TMDIChild.ReallyHide();
begin
  ShowWindow( Handle, SW_HIDE );
end;

{ The following command hooks are intentionally empty no-ops in the base class;
  concrete screens override only the verbs they actually implement. }

{ OnCalculate - base no-op (override to run the screen's calculation). }
procedure TMDIChild.OnCalculate();
begin
end;

procedure TMDIChild.OnHardenValue();
begin
end;

procedure TMDIChild.OnUndo();
begin
end;

procedure TMDIChild.OnRedo();
begin
end;

procedure TMDIChild.OnCut();
begin
end;

procedure TMDIChild.OnCopy();
begin
end;

procedure TMDIChild.OnPaste();
begin
end;

procedure TMDIChild.OnDelete();
begin
end;

procedure TMDIChild.OpenFile( var TheFile: TFileIO );
begin
end;

{ SaveFile - base no-op that reports success (true); screens that persist data
  override this to actually write the document. }
function TMDIChild.SaveFile( FileName, ScreenName : string ): boolean;
begin
  SaveFile := true;
end;

procedure TMDIChild.OnPrint();
begin
end;

procedure TMDIChild.SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont );
begin
end;

procedure TMDIChild.OnContextualHelp();
begin
end;

end.
