unit CHILDWIN;

interface

uses Windows, Classes, Controls, StdCtrls, Forms, Graphics, FileIOUnit, Printers;

type
// Types for the different screens
  TScreenType = ( MortgageType, AmortizationType, PresentValueType, WelcomeType,
                  TableOutType, APRReportType );

  TMDIChild = class( TForm )
  private
    FOnChildClosing: TNotifyEvent;
    m_bUnsavedData: boolean;
  public
    constructor Create(AOwner: TComponent); override;
    function GetType(): TScreenType; virtual; abstract;
    procedure OnCalculate(); virtual;
    procedure OnHardenValue(); virtual;
    procedure OnUndo(); virtual;
    procedure OnRedo(); virtual;
    procedure OnCut(); virtual;
    procedure OnCopy(); virtual;
    procedure OnPaste(); virtual;
    procedure OnDelete(); virtual;
    procedure OpenFile( var TheFile: TFileIO ); virtual;
    function SaveFile( FileName, ScreenName : string ): boolean; virtual;
    procedure OnPrint( Header: TStringList ); virtual;
    procedure OnContextualHelp(); virtual;
    procedure SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont ); virtual;
    function HasUnsavedData(): boolean;
    function CheckForUnsavedDataAndFree(): boolean;
    function GetFileName(): string;
    procedure ReallyShow();
    procedure ReallyHide();
  protected
    m_FileName: string;
    m_RestoreState: TWindowState;
    procedure OnFormClose(var Action: TCloseAction);
    procedure SetUnsavedData( Unsaved: boolean );
    procedure PrintHeader( ThePrinter: TPrinter; TextHeight: integer; Header: TStringList );
  published
    property OnChildClosing : TNotifyEvent read FOnChildClosing write FOnChildClosing;
  end;

implementation

{$R *.dfm}

uses Dialogs, Globals;

constructor TMDIChild.Create(AOwner: TComponent);
begin
  inherited Create( AOwner );
  m_bUnsavedData := false;
end;

procedure TMDIChild.OnFormClose(var Action: TCloseAction);
begin
  ReallyHide();
  Action := caNone;
  if( Assigned(FOnChildClosing) ) then
    OnChildClosing( Self );
end;

function TMDIChild.HasUnsavedData(): boolean;
begin
  HasUnsavedData := m_bUnsavedData;
end;

procedure TMDIChild.SetUnsavedData( Unsaved: boolean );
begin
  m_bUnsavedData := Unsaved;
end;

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

function TMDIChild.GetFileName(): string;
begin
  GetFileName := m_FileName;
end;

procedure TMDIChild.ReallyShow();
begin
  ShowWindow( Handle, SW_SHOW );
  WindowState := m_RestoreState;
end;

procedure TMDIChild.ReallyHide();
begin
  m_RestoreState := WindowState;
  if( WindowState = wsMaximized ) then
    WindowState := wsMinimized;
  ShowWindow( Handle, SW_HIDE );
end;

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

function TMDIChild.SaveFile( FileName, ScreenName : string ): boolean;
begin
  SaveFile := true;
end;

procedure TMDIChild.OnPrint( Header: TStringList );
begin
end;

procedure TMDIChild.PrintHeader( ThePrinter: TPrinter; TextHeight: integer; Header: TStringList );
var
  i: integer;
  yPos: integer;
const
  Margin = 80;
begin
  if( Header = nil ) then exit;
  yPos := 100;
  for i:=0 to Header.Count-1 do begin
    ThePrinter.Canvas.TextOut( Margin, yPos, Header[i] );
    yPos := yPos + TextHeight;
  end;
end;

procedure TMDIChild.SetUISettings( CellColour, OutpCellColour, SelectedColour: TColor; CellFont, OutpCellFont: TFont );
begin
end;

procedure TMDIChild.OnContextualHelp();
begin
end;

end.
