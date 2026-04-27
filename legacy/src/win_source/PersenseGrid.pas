unit PersenseGrid;

interface

uses
  Windows, Messages, SysUtils, Classes, Graphics, Controls, Grids, Dialogs,
  Menus, PersenseClipboardUnit, Printers, Globals, peTypes, StdCtrls;

const
  ColTypeNone           = 0; { No formatting }
  ColType4Real          = 1; { Of the form: xx,xxx.xxxx }
  ColType2Real          = 2; { Of the form: xx,xxx.xx }
  ColTypeDollar         = 3; { Of the form: xx,xxx.xx }
  ColTypeInt            = 4; { Of the form: xx,xxx }
  ColTypeDate           = 5; { of the form: DD/MM/YY }
  ColTypeString         = 6; { Will not be tampered with }
  ColTypeStringList     = 7; { combo box, will not be tampered with }
  TopCellMargin         = 2; { margin for cell drawn to screen }
  RightCellMargin       = 3;
  LeftCellMargin        = 3;
  MAX_CELL_HEIGHT       = 25;

type
  ColumnType = shortint;

  { A record of column information }
  ColInfo = record
    ColType: ColumnType;
    LeftPos: real;
    Strings: TStringList;
    MaxLength: integer;
  end;

  { information about each cell }
  CellInfo = record
    Hardness : InOut;
    bReadOnly: boolean;
  end;

  // Type of procedure for the OnEditEnterKeyPressed event
  TCellEditEnterKeyPressed = procedure( Sender: TObject; ACol, ARow: integer; ValueChanged: boolean; var DefaultAction: boolean ) of object;

  // type of procedure for OnCellAfterEdit
  TCellAfterEdit = procedure( Sender: TObject; ACol, ARow: integer; const Value: string; DataChanged: boolean ) of object;

  // for when the arrow key moves past of pre the grid
  TArrowKeyMovementEvent = procedure( Sender: TObject; var Default: boolean ) of object;

  // for verification of doubles
  TVerifyCellStringEvent = procedure( Sender: TObject; ACol, ARow: integer; Value: string; var IsError: boolean ) of object;

  // A child of the THintWindow class.   This is because the base THintWindow
  // doesn't vertically center the text, and I need it to
  TPersenseHintWindow = class(THintWindow)
  public
    procedure PaintWindow( DC: HDC ); override;
  end;

  // The actual TPersenseGrid Component
  TPersenseGrid = class(TStringGrid)
  private
    m_ColArray: array of ColInfo;
    m_CellInfo: array of array of CellInfo;
    m_EditedRow: integer;
    m_EditedCol: integer;
    m_CellStringBefore: string;
    m_TriggerOnCellAfterEdit: boolean;
    m_EnterKeyPressed: boolean;
    m_WideCellTextWindow: TPersenseHintWindow;
    m_MouseX, m_MouseY: integer;
    m_ContextCut: TMenuItem;
    m_ContextCopy: TMenuItem;
    m_ContextPaste: TMenuItem;
    m_bRowMouseDrag: boolean;
    m_CurrentMouseDragRow: integer;
    m_UsedRowCount: integer;
    m_ComboBox: TComboBox;
    m_ComboBoxEditorMode: boolean;
    m_MouseDownRow: integer;
    m_MouseDownCol: integer;
    { custom properties }
    FCellFont: TFont;
    FCellBackgroundColor: TColor;
    FOutpCellFont: TFont;
    FOutpCellBackgroundColor: TColor;
    FSelectedCellColor: TColor;
    FIsExpandable: boolean;
    { custome events }
    FOnCellAfterEdit : TCellAfterEdit;
    FOnCellBeforeEdit : TSetEditEvent;
    FOnAddNewRows : TNotifyEvent;
    FOnEditEnterKeyPressed : TCellEditEnterKeyPressed;
    FOnDownAfterGrid: TNotifyEvent;
    FOnUpBeforeGrid: TNotifyEvent;
    FOnRightAfterGrid: TArrowKeyMovementEvent;
    FOnLeftBeforeGrid: TArrowKeyMovementEvent;
    FOnVerifyCellString: TVerifyCellStringEvent;
    { Only so many hot keys can be used, this is for extra key combos }
    FOnEditCut : TNotifyEvent;
    FOnEditCopy : TNotifyEvent;
    FOnEditPaste : TNotifyEvent;
  protected
    // override type things
    procedure CMMouseLeave( var AMsg: TMessage ); message CM_MOUSELEAVE;
//    function CreateEditor(): TInplaceEdit; override;
    // combo box things
    procedure EnableComboBox( ACol, ARow: integer );
    procedure OnComboBoxExit( Sender: TObject );
    procedure OnComboBoxKeyDown( Sender: TObject; var Key: Word; Shift: TShiftState );
     { useful things }
    function ConvertString( ColType: ColumnType; const value: string; var IsError: boolean ) : string;
    procedure MoveSelectedRight();
    procedure MoveSelectedLeft();
    function ShouldDisplayLongTextWindow( ACol, ARow: integer ): boolean;
    procedure DisplayLongTextWindow( ACol, ARow: integer );
    procedure HideLongTextWindow();
    procedure ShrinkTextToFit( Width: integer; var TheString: string; TheCanvas: TCanvas );
    procedure SetUsedRowCount( Value: integer );
    procedure AdjustUsedRowCount();
    function CellIsValid( ACol, ARow: integer ): boolean;
    { accessors to published properties }
    procedure SetColCount( NewCount: Longint );
    procedure SetRowCount( NewCount: Longint );
    procedure SetCellFont( const NewFont: TFont );
    procedure SetOutpCellFont( const NewFont: TFont );
    procedure SetSelection( NewSelection: TGridRect );
    { events }
    procedure Resize(); override;
    procedure DrawCell( ACol, ARow: Integer; Rect: TRect; State: TGridDrawState ); override;
    procedure DrawCellToCanvas( ACol, ARow: Integer; Rect: TRect; LeftMargin, RightMargin: integer; TheCanvas: TCanvas; IsPrinting: boolean );
    function SelectCell( ACol,ARow:Longint ): Boolean; override;
    function GetEditText( ACol:integer;ARow:Integer ):string; override;
    function GetEditMask( ACol, ARow: integer ): string; override;
    procedure CustomKeyDown( Sender: TObject; var Key: Word; Shift: TShiftState );
    procedure CustomKeyUp( Sender: TObject; var Key: Word; Shift: TShiftState );
    procedure CustomKeyPress( Sender: TObject; var Key: Char );
    procedure MouseDown( Button: TMouseButton; Shift: TShiftState; X, Y: integer ); override;
    procedure MouseUp( Button: TMouseButton; Shift: TShiftState; X, Y: integer ); override;
    procedure MouseMove( Shift: TShiftState; X, Y: integer ); override;
    procedure ContextMenuPopupEvent( Sender: TObject );
//    procedure DoExit(); override;
  public
    property InplaceEditor;
    procedure SetEditText( ACol,ARow:Longint; const Value: string ); override;
    constructor Create( AOwner: TComponent ); override;
    destructor Destroy(); override;
    procedure SetupColumn( Index: integer; LeftPos: real; ColType: ColumnType; MaxLength: integer );
    procedure SetColumnStringList( Index: integer; Strings: TStringList );
    procedure SetCellHardness( ACol, ARow: integer; Hardness: inout );
    function GetCellHardness( ACol, ARow: integer ): inout;
    procedure SetCell( Input: string; ACol, ARow: integer; Hardness: inout );
    procedure SetCellReadOnly( ACol, ARow: integer );
    procedure CutToClipboard();
    procedure CopyToClipboard();
    procedure PasteFromClipboard( var PasteRect: TRect );
    procedure DeleteSelected();
    procedure CopyRow( Source, Destination: integer );
    procedure EmptyRow( TheRow: integer );
    function RowIsEmpty( ARow: integer ): boolean;
    procedure Print( FirstRow, Top, Left, Width: integer; var Height: integer; RowsToPrint: integer; var RowsPrinted: integer );
    function GetUsedRowCount(): integer;
    function Focused(): boolean; override;
    procedure DisableComboBox();
    function CurrentCellIsDateType(): boolean;
  published
    { custom properties }
    property CellFont : TFont read FCellFont write SetCellFont;
    property CellBackgroundColor : TColor read FCellBackgroundColor write FCellBackgroundColor;
    property OutpCellFont : TFont read FOutpCellFont write SetOutpCellFont;
    property OutpCellBackgroundColor : TColor read FOutpCellBackgroundColor write FOutpCellBackgroundColor;
    property SelectedCellColor : TColor read FSelectedCellColor write FSelectedCellColor;
    property IsExpandable : boolean read FIsExpandable write FIsExpandable;
    { override properties }
    property ColCount write SetColCount;
    property RowCount write SetRowCount;
    property Selection write SetSelection;
    { custom events }
    property OnCellBeforeEdit : TSetEditEvent read FOnCellBeforeEdit write FOnCellBeforeEdit;
    property OnCellAfterEdit : TCellAfterEdit read FOnCellAfterEdit write FOnCellAfterEdit;
    property OnAddNewRows : TNotifyEvent read FOnAddNewRows write FOnAddNewRows;
    property OnEditEnterKeyPressed : TCellEditEnterKeyPressed read FOnEditEnterKeyPressed write FOnEditEnterKeyPressed;
    property OnEditCut: TNotifyEvent read FOnEditCut write FOnEditCut;
    property OnEditCopy: TNotifyEvent read FOnEditCopy write FOnEditCopy;
    property OnEditPaste: TNotifyEvent read FOnEditPaste write FOnEditPaste;
    property OnDownAfterGrid: TNotifyEvent read FOnDownAfterGrid write FOnDownAfterGrid;
    property OnUpBeforeGrid: TNotifyEvent read FOnUpBeforeGrid write FOnUpBeforeGrid;
    property OnRightAfterGrid: TArrowKeyMovementEvent read FOnRightAfterGrid write FOnRightAfterGrid;
    property OnLeftBeforeGrid: TArrowKeyMovementEvent read FOnLeftBeforeGrid write FOnLeftBeforeGrid;
    property OnVerifyCellString: TVerifyCellStringEvent read FOnVerifyCellString write FOnVerifyCellString;
  end;
{
  // a custom InPlaceEdit object so I can do some stuff
  // I wouldn't otherwise be able to do
  TPersenseInplaceEdit = class(TInplaceEdit)
  public
    constructor Create( AOwner: TComponent ); override;
  end;
}
procedure Register;

implementation

uses
  LogUnit;

const
  DefaultMaxColWidth = 256;

// the hint window object
procedure TPersenseHintWindow.PaintWindow( DC: HDC );
var
  TextHeight: integer;
begin
  TextHeight := Canvas.TextHeight( Caption );
  Canvas.TextOut( 2, Trunc(Height/2)-Trunc(TextHeight/2), Caption );
end;

{
// the inplaceedit object
// just a place holder for now.  May be good enough to
// use the existing one.  But just in case....
constructor TPersenseInplaceEdit.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
end;
}
{ override the constructor.  This sets up whatever stuff that needs to get set
  up in the begining.  Delphi visual setup does some crappy stuff, like set this
  grid to be 5x5 before it assignes the values I specify in ObjectInspector.  So
  This does the safe stuff, hoping that more correct numbers will come later. }
constructor TPersenseGrid.Create( AOwner: TComponent );
var
  ARow, ACol : integer;
begin;
//  MasterLog.Write( LVL_LOW, 'TPersenseGrid.Create begin' );
  inherited Create( AOwner );
  m_EnterKeyPressed := false;
  m_WideCellTextWindow := TPersenseHintWindow.Create( Self );
  m_MouseX := 0;
  m_MouseY := 0;
  m_bRowMouseDrag := false;
  m_UsedRowCount := 0;
  FCellFont := TFont.Create();
  FCellFont.Assign( Canvas.Font );
  FOutpCellFont := TFont.Create();
  FOutpCellFont.Assign( Canvas.Font );
  FOutpCellFont.Style := [fsBold];
  Options := Options + [goEditing];
  Options := Options + [goTabs];
  OnKeyDown := CustomKeyDown;    // See explenation above CustomKeyDown
  OnKeyUp := CustomKeyUp;
  OnKeyPress := CustomKeyPress;
  // Set up the array of column information
  if( ColCount = 0 ) then begin
    MasterLog.Write( LVL_HIGH, 'PersenseGrid constructor ColCount is zero, changing to 1 to avoid crash');
    ColCount := 1;
  end;
  SetLength( m_ColArray, ColCount );
  for ACol:=0 to ColCount-1 do begin
    m_ColArray[ACol].ColType := ColTypeNone;
    m_ColArray[ACol].LeftPos := ACol/ColCount;
    m_ColArray[ACol].Strings := nil;
    m_ColArray[ACol].MaxLength := DefaultMaxColWidth;
  end;
  // Set up the array of cell info
  SetLength( m_CellInfo, ColCount, RowCount );
  for ARow:=0 to RowCount-1 do begin
    for ACol:=0 to ColCount-1 do begin
      m_CellInfo[ACol,ARow].Hardness := empty;
      m_CellInfo[ACol,ARow].bReadOnly := false;
    end;
  end;
  // Stupid FixedCols and FixedRows which are set on the 'Object Inspector'
  // get the value 1 at this point in the code.  I have no idea when they get the
  // real values.
  FixedRows := 0;
  if( FixedCols > 1 ) then
    FixedCols := 1;
  // create the context menu
  PopupMenu := TPopupMenu.Create( Self );
  PopupMenu.AutoPopup := true;
  m_ContextCut := TMenuItem.Create( Self );
  m_ContextCut.Caption := 'Cut';
  m_ContextCut.OnClick := ContextMenuPopupEvent;
  PopupMenu.Items.Add( m_ContextCut );
  m_ContextCopy := TMenuItem.Create( Self );
  m_ContextCopy.Caption := 'Copy';
  m_ContextCopy.OnClick := ContextMenuPopupEvent;
  PopupMenu.Items.Add( m_ContextCopy );
  m_ContextPaste := TMenuItem.Create( Self );
  m_ContextPaste.Caption := 'Paste';
  m_ContextPaste.OnClick := ContextMenuPopupEvent;
  PopupMenu.Items.Add( m_ContextPaste );
  m_ComboBox := TComboBox.Create( Self );
  m_ComboBox.Style := csDropDownList;
  m_ComboBox.OnExit := OnComboBoxExit;
  m_ComboBox.OnKeyDown := OnComboBoxKeyDown;
  m_ComboBox.Parent := Self;
  m_ComboBox.Visible := false;
  m_ComboBoxEditorMode := false;
//  MasterLog.Write( LVL_LOW, 'TPersenseGrid.Create end' );
end;

destructor TPersenseGrid.Destroy();
begin
//  MasterLog.Write( LVL_LOW, 'TPersenseGrid.Destroy begin' );
  m_WideCellTextWindow.Free();
  FCellFont.Free();
  FOutpCellFont.Free();
  PopupMenu.Free();
  m_ComboBox.Free();
  inherited Destroy();
//  MasterLog.Write( LVL_LOW, 'TPersenseGrid.Destroy end' );
end;

function TPersenseGrid.Focused(): boolean;
begin
  Focused := inherited Focused();
  if( InplaceEditor <> nil ) then begin
    if( InplaceEditor.Focused() ) then
      Focused := true
  end;
end;

{
// this is how you make your own InplaceEditor
function TPersenseGrid.CreateEditor(): TInplaceEdit;
begin
  Result := TPersenseInplaceEdit.Create( Self );
//  TPersenseInplaceEdit(Result).SetPersenseGrid( @self );
end;
}
{ When someone sets the column count we need to add more info to the m_ColArray
  and the m_CellInfo structure.  This goes ahead and does that. This sets up the
  safe answers.  You should still call SetupColumn to get what you want. }
procedure TPersenseGrid.SetColCount( NewCount: LongInt );
var
  ACol, ARow: longint;
begin;
//  MasterLog.Write( LVL_LOW, 'TPersenseGrid.SetColCount' );
  // Set up the array of column information
  if( NewCount = 0 ) then begin
    MasterLog.Write( LVL_HIGH, 'SetColCount: NewCount is zero, changing to 1 to avoid crash');
    NewCount := 1;
  end;
  SetLength( m_ColArray, NewCount );
  for ACol:=0 to NewCount-1 do begin
    m_ColArray[ACol].ColType := ColTypeNone;
    m_ColArray[ACol].LeftPos := ACol/NewCount;
    m_ColArray[ACol].Strings := nil;
    m_ColArray[ACol].MaxLength := DefaultMaxColWidth;
  end;
  // Set up the array of cell info
  SetLength( m_CellInfo, NewCount, RowCount );
  for ARow:=0 to RowCount-1 do begin
    for ACol:=ColCount to NewCount-1 do begin
      m_CellInfo[ACol,ARow].Hardness := Empty;
    end;
  end;
  // Now actually update the value ofColCount
  TStringGrid(Self).ColCount := NewCount;
end;

{ When someone sets the row count we need to add more info to the
  m_CellInfo structure.  This goes ahead and does that. }
procedure TPersenseGrid.SetRowCount( NewCount: Longint );
var
  ACol, ARow: longint;
begin;
//  MasterLog.Write( LVL_LOW, 'TPersenseGrid.SetRowCount' );
  // Set up the array of cell info
  SetLength( m_CellInfo, ColCount, NewCount );
  for ARow:=RowCount to NewCount-1 do begin
    for ACol:=0 to ColCount-1 do begin
      m_CellInfo[ACol,ARow].Hardness := Empty;
    end;
  end;
  if( FixedCols > 0 ) then begin;
    for ARow:=RowCount to NewCount-1 do
      Cells[0, ARow] := IntToStr( ARow+1 );
  end;
  // Now actually update the value of RowCount
  TStringGrid(Self).RowCount := NewCount;
  if( Assigned(OnAddNewRows) ) then
    OnAddNewRows( Self );
end;

// the OnSelectCell call triggers the update of the status bar.
// however without this overload if the program manually sets
// the selection OnSelectCell will not be called, and the status
// bar will not update (example is on an undo/redo if the selected
// field is changed).  This fixes that problem.
procedure TPersenseGrid.SetSelection( NewSelection: TGridRect );
var
  CanSelect: boolean;
begin
  TStringGrid(Self).Selection := NewSelection;
  if( (NewSelection.Left=NewSelection.Right) and (NewSelection.Top=NewSelection.Bottom) ) then begin
    if( Assigned(OnSelectCell) ) then
      OnSelectCell( Self, NewSelection.Left, NewSelection.Top, CanSelect );
  end;
end;

{ use to set the font of the normal (inp) cell }
procedure TPersenseGrid.SetCellFont( const NewFont: TFont );
var
  CellFontHeight, OutpCellFontHeight: integer;
begin
  FCellFont.Assign( NewFont );
  // the Height property has different meaning depending on if it's positive or
  // negative.  If positive it includes leading.
  if( NewFont.Height < 0 ) then
    CellFontHeight := abs(NewFont.Height)
  else
    CellFontHeight := NewFont.Height - Trunc(NewFont.Height/5);
  if( OutpCellFont.Height < 0 ) then
    OutpCellFontHeight := abs(OutpCellFont.Height)
  else
    OutpCellFontHeight := OutpCellFont.Height - Trunc(OutpCellFont.Height/5);
  // we have the real font heights, let's change the row height.
  if( CellFontHeight > OutpCellFontHeight ) then begin
    if( CellFontHeight < MAX_CELL_HEIGHT ) then
      DefaultRowHeight := CellFontHeight+10
    else
      DefaultRowHeight := MAX_CELL_HEIGHT+10;
  end else begin
    if( OutpCellFontHeight < MAX_CELL_HEIGHT ) then
      DefaultRowHeight := OutpCellFontHeight+10
    else
      DefaultRowHeight := MAX_CELL_HEIGHT+10;
  end;
end;

{ use to set the font of the outp cell }
procedure TPersenseGrid.SetOutpCellFont( const NewFont: TFont );
var
  CellFontHeight, OutpCellFontHeight: integer;
begin
  FOutpCellFont.Assign( NewFont );
  FOutpCellFont.Style := [fsBold];
  // the Height property has different meaning depending on if it's positive or
  // negative.  If positive it includes leading.
  if( NewFont.Height < 0 ) then
    OutpCellFontHeight := abs(NewFont.Height)
  else
    OutpCellFontHeight := NewFont.Height - Trunc(NewFont.Height/5);
  if( CellFont.Height < 0 ) then
    CellFontHeight := abs(CellFont.Height)
  else
    CellFontHeight := CellFont.Height - Trunc(CellFont.Height/5);
  // we have the real font heights, let's change the row height.
  if( CellFontHeight > OutpCellFontHeight ) then begin
    if( CellFontHeight < MAX_CELL_HEIGHT ) then
      DefaultRowHeight := CellFontHeight+10
    else
      DefaultRowHeight := MAX_CELL_HEIGHT+10;
  end else begin
    if( OutpCellFontHeight < MAX_CELL_HEIGHT ) then
      DefaultRowHeight := OutpCellFontHeight+10
    else
      DefaultRowHeight := MAX_CELL_HEIGHT+10;
  end;
end;

{ Use this to set up information about a column. }
procedure TPersenseGrid.SetupColumn( Index: integer; LeftPos: real; ColType: ColumnType; MaxLength: integer );
begin;
  if( (Index > High(m_ColArray)) or (Index < 0) ) then begin
    MasterLog.Write( LVL_MED, 'SetupColumn: tried to access out of bounds Index' );
    exit;
  end;
  m_ColArray[Index].LeftPos := LeftPos;
  m_ColArray[Index].ColType := ColType;
  m_ColArray[Index].MaxLength := MaxLength;
end;

procedure TPersenseGrid.SetColumnStringList( Index: integer; Strings: TStringList );
begin
  if( (Index > High(m_ColArray)) or (Index < 0) ) then begin
    MasterLog.Write( LVL_MED, 'SetColumnStringList: tried to access out of bounds Index' );
    exit;
  end;
  if( m_ColArray[Index].Strings = nil ) then
    m_ColArray[Index].Strings := TStringList.Create();
  m_ColArray[Index].Strings.Assign( Strings );
end;

{ use to set the value and hardness of a cell }
procedure TPersenseGrid.SetCell( Input: string; ACol, ARow: integer; Hardness: inout );
var
  ConvertedString: string;
  ErrorFlag: boolean;
begin
  if( (ACol > High(m_ColArray)) or (ACol<0) ) then begin
    MasterLog.Write( LVL_MED, 'SetCell: tried to access out of range Col' );
    exit;
  end;
  ConvertedString := ConvertString( m_ColArray[ACol].ColType, Input, ErrorFlag );
  m_CellInfo[ACol, ARow].Hardness := Hardness;
  Cells[ACol,ARow] := ConvertedString;
  if( ARow >= m_UsedRowCount ) then
    SetUsedRowCount( ARow+1 )
  else if( (ARow = m_UsedRowCount-1) and (Input='') ) then
    AdjustUsedRowCount();
end;

// Sets the specified Cell to the specified hardness
// this is the one that can be seen and used from the outside.
// This will make sure that read only cells do not get changed
procedure TPersenseGrid.SetCellHardness( ACol, ARow: integer; Hardness: inout );
begin;
  if( (ACol>High(m_CellInfo)) or (ACol<0) or (ARow>High(m_CellInfo[0])) or (ARow<0) ) then begin
    MasterLog.Write( LVL_MED, 'SetCellHardness: tried to access invalid ACol or ARow' );
    exit;
  end;
  if( m_CellInfo[ACol,ARow].bReadOnly ) then exit;
  m_CellInfo[ACol,ARow].Hardness := Hardness;
end;

procedure TPersenseGrid.SetCellReadOnly( ACol, ARow: integer );
begin
  if( (ACol>High(m_CellInfo)) or (ACol<0) or (ARow>High(m_CellInfo[0])) or (ARow<0) ) then begin
    MasterLog.Write( LVL_MED, 'SetCellReadOnly: tried to access invalid ACol or ARow' );
    exit;
  end;
  m_CellInfo[ACol,ARow].bReadOnly := true;
end;

{ return info about the cell hardness }
function TPersenseGrid.GetCellHardness( ACol, ARow: integer ): inout;
begin
  if( (ACol>High(m_CellInfo)) or (ACol<0) or (ARow>High(m_CellInfo[0])) or (ARow<0) ) then begin
    MasterLog.Write( LVL_MED, 'GetCellHardness: tried to access invalid ACol or ARow' );
    GetCellHardness := badp;
    exit;
  end;
  GetCellHardness := m_CellInfo[ACol,ARow].Hardness;
end;

{ copies a row from the Source to the Destination }
procedure TPersenseGrid.CopyRow( Source, Destination: integer );
var
  i: integer;
begin
  if( ((ColCount-1)>High(m_CellInfo)) or (Source>High(m_CellInfo[0])) or (Source<0) ) then begin
    MasterLog.Write( LVL_MED, 'CopyRow: out of bounds' );
    exit;
  end;
  for i:=FixedCols to ColCount-1 do
    SetCell( Cells[i,Source], i, Destination, m_CellInfo[i,Source].Hardness );
  if( Destination >= m_UsedRowCount ) then
    SetUsedRowCount( Destination+1 );
end;

{ empties out a row }
procedure TPersenseGrid.EmptyRow( TheRow: integer );
var
  i: integer;
begin
  if( (TheRow<0) or (TheRow>High(m_CellInfo[0])) or ((ColCount-1)>High(m_CellInfo)) ) then begin
    MasterLog.Write( LVL_MED, 'EmptyRow: out of bounds' );
    exit;
  end;
  for i:=FixedCols to ColCount-1 do begin
    Cells[i,TheRow] := '';
    m_CellInfo[i,TheRow].Hardness := empty;
  end;
  if( TheRow=m_UsedRowCount-1 ) then
    AdjustUsedRowCount();
end;

function TPersenseGrid.RowIsEmpty( ARow: integer ): boolean;
var
  i: integer;
begin
  RowIsEmpty := false;
  for i:=FixedCols to ColCount-1 do
    if( Cells[i,ARow] <> '' ) then exit;
  RowIsEmpty := true;
end;

procedure TPersenseGrid.SetUsedRowCount( Value: integer );
begin
  m_UsedRowCount := Value;
  Repaint();
  if( IsExpandable and (m_UsedRowCount=RowCount) ) then begin
    RowCount := RowCount+1;
    if( Assigned(FOnAddNewRows) ) then
      OnAddNewRows( Self );
  end;
end;

procedure TPersenseGrid.AdjustUsedRowCount();
var
  i: integer;
  NewCount: integer;
begin
  NewCount := m_UsedRowCount;
  for i:=m_UsedRowCount downto 0 do begin
    if( RowIsEmpty(i) ) then
      NewCount := i
    else begin
      if( m_UsedRowCount <> NewCount ) then begin
        m_UsedRowCount := NewCount;
        Repaint();
      end;
      exit;
    end;
  end;
  if( m_UsedRowCount <> NewCount ) then begin
    m_UsedRowCount := NewCount;
    Repaint();
  end;
end;

function TPersenseGrid.GetUsedRowCount(): integer;
begin
  GetUsedRowCount := m_UsedRowCount;
end;

{ Columns have ColTypes associated with them.  Depending on the type a column will
  have to display that info a certain way.  This converts it from whatever the user
  types in to the standard representation }
function TPersenseGrid.ConvertString( ColType: ColumnType; const value: string; var IsError: boolean ) : string;
var
  StringValue: double;
  TheDate: daterec;
begin;
  IsError := false;
  if( (ColType=ColTypeNone) or (ColType=ColTypeString) or (ColType=ColTypeStringList) ) then begin
    Result := value;
    exit;
  end;
  if( ColType = ColTypeDate ) then begin
    if( Value = '  /  /    ' ) then
      Result := ''
    else begin
      TheDate := StringFormat2Date( Value, IsError );
      if( IsError ) then begin
        MasterLog.Write( LVL_MED, 'ConvertString: StringFormat2Date failed' );
        Result := '';
        exit;
      end;
      Result := DateToStr( TheDate );
    end;
    exit;
  end;
  if( Value = '' ) then begin
    Result := '';
    exit;
  end;
  StringValue := StringFormat2Double( Value, IsError );
  if( IsError ) then begin
    MasterLog.Write( LVL_MED, 'ConvertString: StringFormat2Double failed' );
    Result := '';
    exit;
  end;
  case ColType of
    ColType4Real: Result := Double2StringFormat( StringValue, DoubleDotFour );
    ColType2Real: Result := Double2StringFormat( StringValue, DoubleDotTwo );
    ColTypeDollar: Result := Double2StringFormat( StringValue, DoubleDotTwo );
    ColTypeInt: Result := Double2StringFormat( StringValue, Int );
  end;
end;

{ Just a chunk of code I was using a bunch of times.  Moves the cell
  selection one cell to the right, with wrapping }
procedure TPersenseGrid.MoveSelectedRight();
var
  DefaultHandler: boolean;
begin
  DefaultHandler := true;
  if( Col+1 = ColCount ) then begin
    if( assigned( FOnRightAfterGrid ) ) then
      OnRightAfterGrid( Self, DefaultHandler );
  end;
  if( DefaultHandler ) then begin
    if( (Col+1) < ColCount ) then
      Col := Col+1
    else
      Col := FixedCols;
  end;
end;

{ same idea, but to the left }
procedure TPersenseGrid.MoveSelectedLeft();
var
  DefaultHandler: boolean;
begin
  DefaultHandler := true;
  if( Col = FixedCols ) then begin
    if( assigned( FOnLeftBeforeGrid ) ) then
      OnLeftBeforeGrid( Self, DefaultHandler );
  end;
  if( DefaultHandler ) then begin
    if( Col > FixedCols ) then
      Col := Col-1
    else
      Col := ColCount-1;
  end;
end;

{ Handle the resizing of the grid. }
procedure TPersenseGrid.Resize();
var
  i: integer;
  AdditionalRows: integer;
  TotalWidth: integer;
begin;
  inherited Resize();
  if( FIsExpandable ) then begin
    if( (DefaultRowHeight*RowCount) < Height ) then begin
      AdditionalRows := Trunc((Height-(DefaultRowHeight*RowCount))/DefaultRowHeight);
      AdditionalRows := AdditionalRows + 1;
      RowCount := RowCount + AdditionalRows;
    end;
  end;

  // assign all the widths except the last one
  For i:=0 to ColCount-2 do
    ColWidths[i] := Trunc((m_ColArray[i+1].LeftPos-m_ColArray[i].LeftPos)*Width);
  // find the total widths of all the columns
  TotalWidth := 0;
  For i:=0 to ColCount-2 do
    TotalWidth := TotalWidth+ColWidths[i]+GridLineWidth;
  if( ScrollBars = ssNone ) then
    ColWidths[ColCount-1] := Width-TotalWidth-4    // 4 is for the width of the 3D border
  else
    ColWidths[ColCount-1] := Width-TotalWidth-21;   // 21 is for the width of the border and the scroll bar
end;

{ We have to do our own special drawing to enable custom colours and right
  justification of cells.  Also note that if a cell's colour is specified as
  clNone then it will be drawn using the current CellBackground or CellFont Color. }
procedure TPersenseGrid.DrawCell( ACol, ARow: Integer; Rect: TRect; State: TGridDrawState );
begin
  if( (ACol<0) or (ARow<0) or (ACol>High(m_CellInfo)) or (ARow>High(m_CellInfo[0])) ) then begin
    MasterLog.Write( LVL_MED, 'DrawCell: range error' );
    exit;
  end;
  DrawCellToCanvas( ACol, ARow, Rect, LeftCellMargin, RightCellMargin, Canvas, false );
end;

{ I made a special one because this can be called both from DrawCell (to do the screen
  drawing) and Print (to do the printing). }
procedure TPersenseGrid.DrawCellToCanvas( ACol, ARow: Integer; Rect: TRect; LeftMargin, RightMargin: integer; TheCanvas: TCanvas; IsPrinting: boolean );
var
  TextXPos: Integer;
  PrintableString: string;
  TopMargin: integer;
begin
  TheCanvas.Font := Font;
  if( ACol >= FixedCols ) then begin
    // assign font info
    if( m_CellInfo[ACol,ARow].Hardness = outp ) then begin
      TheCanvas.Font.Assign( OutpCellFont );
    end else
      TheCanvas.Font.Assign( CellFont );
    // assign background colour info
    if( ARow > m_UsedRowCount ) then
      TheCanvas.Brush.Color := clSilver
    else if( m_CellInfo[ACol,ARow].Hardness = outp ) then
      TheCanvas.Brush.Color := OutpCellBackgroundColor
    else
      TheCanvas.Brush.Color := CellBackgroundColor;
  end;
  if( not IsPrinting ) then begin
    // If the cell is selected...
    if( (Selection.Left<=ACol) and (Selection.Right>=ACol) and
        (Selection.Top<=ARow) and (Selection.Bottom>=ARow ) and Focused() ) then
      TheCanvas.Brush.Color := SelectedCellColor;
  end;
  // now do the actual painting
  TheCanvas.FillRect( Rect );
  PrintableString := Cells[ACol,ARow];
  ShrinkTextToFit( (Rect.Right-Rect.Left-LeftMargin-RightMargin), PrintableString, TheCanvas );
  TopMargin := Trunc( ((Rect.Bottom - Rect.Top) - TheCanvas.TextHeight( PrintableString ))/2.0 );
  if( TheCanvas.TextWidth( PrintableString ) > (Rect.Right-Rect.Left) ) then begin
    TheCanvas.TextRect( Rect, Rect.Left+LeftMargin, Rect.Top+TopMargin, PrintableString );
  end else begin
    TextXPos := Rect.Right - TheCanvas.TextWidth( PrintableString ) - RightMargin;
    TheCanvas.TextOut( TextXPos, Rect.Top+TopMargin, PrintableString );
  end;
end;

{ This procedure is called to make sure that the text will fit within the borders
  of the cell.  If the text is too wide this will start by removing the commas.  If
  it is still too wide it will shrink the font until it gets as small as 8 point.  If it
  is still to wide then it will strip zeros from the right side of the decimal }
procedure TPersenseGrid.ShrinkTextToFit( Width: integer; var TheString: string; TheCanvas: TCanvas );
var
  Position: integer;
begin
  if( TheCanvas.TextWidth( TheString ) > Width ) then begin
    { remove commas }
    StripCommas( TheString );
    { shrink font }
    while( (TheCanvas.TextWidth(TheString)>Width) and ( TheCanvas.Font.Size<-8 ) ) do
      TheCanvas.Font.Size := TheCanvas.Font.Size+1;
    Position := Pos( '.', TheString );
    if( Position<>0 ) then begin
      { remove trailing zero }
      while( (TheCanvas.TextWidth(TheString)>Width) and (TheString[length(TheString)]='0')) do
        TheString := Copy( TheString, 0, Length(TheString)-1 );
      { if I just happened to remove to the point of a decimal then remove that too }
      if( TheString[length(TheString)] = '.' ) then
        TheString := Copy( TheString, 0, Length(TheString)-1 );
    end;
  end;
end;

{ The following three functions are for handling post cell editing
  events.  They took me forever to figure out, so they're worth
  explaining.  The idea is that just before a cell gets edited
  I save the value of the cell.  Then while I'm editing it I save
  the current string value.  Then when someone changes to another
  cell I check to see if the string has changed. }

{ This is the part where I store initial values }
function TPersenseGrid.GetEditText( ACol:integer; ARow:Integer ):string;
var
  DefaultHandling: boolean;
begin;
  HideLongTextWindow();
  if( (Acol<0) or (ARow<0) ) then exit;
  // This gets called as the InplaceEditor is starting up with EditorMode=true
  // If this is a read only cell then shut down the EditorMode.
  if( EditorMode ) then begin
    if( m_CellInfo[ACol,ARow].bReadOnly ) then begin
      EditorMode := false;
      exit;
    end;
  end;
  // We want to strike off the Calculation even if they press enter
  // before editing a cell (ie if they fill in all the data and then
  // go to any new cell and press enter).
{
  Maybe we do, but it sucks.  Need to come up
  with something better.  Pressing enter here will do the calc,
  but then put them in the cell, causing the outputted cell to become
  input.  
  if( Assigned(FOnEditEnterKeyPressed) and m_EnterKeyPressed ) then begin
    DefaultHandling := true;
    m_EnterKeyPressed := false;
    OnEditEnterKeyPressed( Self, m_EditedCol, m_EditedRow, false, DefaultHandling );
  end;
  }
  m_CellStringBefore:= inherited GetEditText(Acol, Arow);
  m_EditedCol := ACol;
  m_EditedRow := ARow;
  m_TriggerOnCellAfterEdit := true;
  Result := m_CellStringBefore;
  if( EditorMode ) then begin
    if( Assigned(FOnCellBeforeEdit) ) then begin
      OnCellBeforeEdit( Self, m_EditedCol, m_EditedRow, m_CellStringBefore );
    end;
  end;
end;

{ This is the part where I store current values }
procedure TPersenseGrid.SetEditText( ACol,ARow:Longint; const Value: string );
var
  NewCellString: string;
  DefaultHandling: boolean;
  IsError: boolean;
  DataChanged: boolean;
begin;
  HideLongTextWindow();
  if( m_CellInfo[ACol,ARow].bReadOnly ) then exit;
  NewCellString := ConvertString( m_ColArray[ACol].ColType, Value, IsError );
  if( m_TriggerOnCellAfterEdit and (not (EditorMode or m_ComboBoxEditorMode)) ) then begin;
    m_TriggerOnCellAfterEdit := false;
    if( Assigned(OnVerifyCellString) ) then begin
      // the case where a user enters logically bad data and then clicks a
      // different grid is handles here.
      IsError := false;
      OnVerifyCellString( Self, ACol, ARow, NewCellString, IsError );
      if( IsError ) then begin
        SetEditText( ACol, ARow, '' );
        exit;
      end;
    end;
    Inherited SetEditText( ACol, ARow, NewCellString );
    if( Assigned(FOnCellAfterEdit)) then begin
      DataChanged := true;
      if( m_CellStringBefore=NewCellString ) then begin
        if( m_CellInfo[m_EditedCol,m_EditedRow].Hardness<>inp) then
          DataChanged := true
        else
          DataChanged := false;
      end;
      OnCellAfterEdit( Self, m_EditedCol, m_EditedRow, NewCellString, DataChanged );
    end;
  end else
    Inherited SetEditText( ACol, ARow, NewCellString );
  { this flag is set to true only when a user presses the EnterKey to end
    an editing session }
  if( Assigned(FOnEditEnterKeyPressed) and m_EnterKeyPressed ) then begin
    DefaultHandling := true;
    m_EnterKeyPressed := false;
    OnEditEnterKeyPressed( Self, m_EditedCol, m_EditedRow, m_CellStringBefore<>NewCellString, DefaultHandling );
    if( DefaultHandling ) then begin
      { If this is true then move the selected cell over by one, wrapping if necessary }
      MoveSelectedRight();
    end;
  end;
  m_EnterKeyPressed := false;
  if( (ARow >= m_UsedRowCount) and (NewCellString<>'') ) then
    SetUsedRowCount( ARow+1 )
  else if( (ARow=m_UsedRowCount-1) and (NewCellString='') ) then
    AdjustUsedRowCount();
end;

{ This is the part where I check for changes and fire an event }
function TPersenseGrid.SelectCell( ACol,ARow:Longint ): Boolean;
var
  NewCellString: string;
  Rect: TRect;
  i: integer;
  IsError: boolean;
  DataChanged: boolean;
begin;
  if( ARow > m_UsedRowCount ) then begin
    SelectCell := false;
    exit;
  end;
  NewCellString := Cells[m_EditedCol,m_EditedRow];
  HideLongTextWindow();
  if( (EditorMode or m_ComboBoxEditorMode) and m_TriggerOnCellAfterEdit) then begin;
    NewCellString := ConvertString( m_ColArray[m_EditedCol].ColType, NewCellString, IsError );
    m_TriggerOnCellAfterEdit := false;
    { This next line has implications.  If I do not set EditorMode to false then
      assigning a value to Cells will result in a call to GetEditText, which I
      don't want.  So I set EditorMode to false, which works, EXCEPT doing the
      assignment will result in a call to SetEditText, which is fine as long as
      m_TriggerOnCellAfterEdit has been set to false already. }
    EditorMode := false;
    if( m_ComboBoxEditorMode ) then
      DisableComboBox();
    Cells[m_EditedCol,m_EditedRow] := NewCellString;
    if( Assigned( FOnCellAfterEdit )) then begin
      DataChanged := true;
      if( m_CellStringBefore=NewCellString ) then begin
        if( m_CellInfo[m_EditedCol,m_EditedRow].Hardness<>inp) then
          DataChanged := true
        else
          DataChanged := false;
      end;
      OnCellAfterEdit( Self, m_EditedCol, m_EditedRow, NewCellString, DataChanged );
    end;
  end else begin
    if( ShouldDisplayLongTextWindow( ACol,ARow ) ) then begin
      DisplayLongTextWindow( ACol, ARow );
    end;
  end;
  if( m_ColArray[ACol].ColType = ColTypeStringList ) then begin
    Rect := CellRect( ACol, ARow );
    for i:=0 to m_ColArray[ACol].Strings.Count-1 do
      m_ComboBox.Items.Add( m_ColArray[ACol].Strings[i] );
    m_ComboBox.SetBounds( Rect.Left+1, Rect.Top+2, Rect.Right-Rect.Left, Rect.Bottom-Rect.Top+50 );
    EnableComboBox( ACol, ARow );
  end;
  Result := inherited SelectCell(ACol,ARow);
end;

// When a user puts bad data in a cell they should not be allowed to leave
// that cell unless the data becomes good or they erase it.
// This function is the test to see if the data is good.
// In this case 'good' means no real=12.34.56 or date=12/45/75
function TPersenseGrid.CellIsValid( ACol, ARow: integer ): boolean;
var
  IsError: boolean;
  StringValue: string;
begin
  CellIsValid := true;
  if( not EditorMode ) then exit;
  IsError := false;
  StringValue := ConvertString( m_ColArray[ACol].ColType, InplaceEditor.EditText, IsError );
  if( IsError ) then begin
    CellIsValid := false;
    exit;
  end;
  if( Assigned(OnVerifyCellString) ) then begin
    IsError := false;
    OnVerifyCellString( Self, ACol, ARow, StringValue, IsError );
    if( IsError ) then begin
      CellIsValid := false;
      exit;
    end;
  end;
end;

function TPersenseGrid.CurrentCellIsDateType(): boolean;
begin
  CurrentCellIsDateType := (m_ColArray[Col].ColType=ColTypeDate);
end;

{ default handler for any clicks to the context menu }
procedure TPersenseGrid.ContextMenuPopupEvent( Sender: TObject );
begin
  if( Sender = m_ContextCut ) then begin
    if( assigned( OnEditCut ) ) then
      OnEditCut( Self );
  end else if( Sender = m_ContextCopy ) then begin
    if( assigned( OnEditCopy ) ) then
      OnEditCopy( Self );
  end else if( Sender = m_ContextPaste ) then begin
    if( assigned( OnEditPaste ) ) then
      OnEditPaste( Self );
  end;
end;

{ copies the selected cells to the clipboard }
procedure TPersenseGrid.CopyToClipboard();
var
  Values: TStringList;
  Info: TList;
  ARow, ACol: integer;
  pCellInfo: ^CellInfo;
  Left, Right: integer;
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.CopyToClipboard' );
  Values := TStringList.Create();
  Info := TList.Create();
  Left := Selection.Left;
  Right := Selection.Right;
  if( (Left<0) or (Right<0) ) then exit;
  if( Right>High(m_CellInfo) ) then
    Right := High( m_CellInfo );
  for ARow:=Selection.Top to Selection.Bottom do begin
    for ACol:=Left to Right do begin
      if( ACol<=High(m_CellInfo) ) then begin
        if( ARow<0 ) then exit;
        Values.Add( Cells[ACol,ARow] );
        new( pCellInfo );
        pCellInfo.Hardness := m_CellInfo[ACol,ARow].Hardness;
        Info.Add( pCellInfo );
      end;
    end;
  end;
  { all the values are now stored in Values and Info }
  PersenseClipboard.Copy( Values, Info, Right-Left+1 );
  Values.Free();
  Info.Free();
end;

{ just like CopyToClipboard, only it then goes and deletes everything that was copied }
procedure TPersenseGrid.CutToClipboard();
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.CutToClipboard' );
  CopyToClipboard();
  DeleteSelected();
end;

{ you guessed it... }
procedure TPersenseGrid.PasteFromClipboard( var PasteRect: TRect );
var
  PasteRowCount: integer;
  MultiplePasteCount: integer;
  ACol, ARow: integer;
  i: integer;
  Values: TStringList;
  Info: TList;
  RowWidth: integer;
  pCellInfo: ^CellInfo;
  IsError: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.PasteFromClipboard' );
  Values := TStringList.Create();
  Info := TList.Create();
  PersenseClipboard.Paste( Values, Info, RowWidth );
  if( RowWidth = 0 ) then begin
    MasterLog.Write( LVL_MED, 'PasteFromClipboard: can''t paste with zero RowWidth' );
    exit;
  end;
  ACol := Selection.Left;
  ARow := Selection.Top;
  if( (ACol<0) or (ARow<0) ) then exit;
  PasteRowCount := Values.Count div RowWidth;
  { if someone's pasting 10 rows where there are only 5 left, then we need to add rows }
  if( (PasteRowCount+ARow) > RowCount ) then begin
    if( IsExpandable ) then
      SetRowCount( PasteRowCount+ARow )
    else
      PasteRowCount := RowCount-ARow;
  end;
  for i:=0 to Values.Count-1 do begin
    if( ((ACol+(i mod RowWidth))<=High(m_CellInfo)) and ((ARow+(i div RowWidth))<=High(m_CellInfo[0])) ) then begin
      if( not m_CellInfo[ACol+(i mod RowWidth),ARow+(i div RowWidth)].bReadOnly ) then begin
        Cells[ACol+(i mod RowWidth),ARow+(i div RowWidth)] := ConvertString( m_ColArray[ACol+(i mod RowWidth)].ColType, Values[i], IsError );
        if( Info.Count <> 0 ) then begin
          pCellInfo := Info[i];
          m_CellInfo[ACol+(i mod RowWidth), ARow+(i div RowWidth)].Hardness := pCellInfo.Hardness;
        end else
          m_CellInfo[ACol+(i mod RowWidth), ARow+(i div RowWidth)].Hardness := outp;
      end;
    end;
  end;
  { if a multiple of the pasted row is selected then paste multiple times }
  MultiplePasteCount := (Selection.Bottom-Selection.Top+1) div PasteRowCount;
  PasteRect.Left := Selection.Left;
  PasteRect.Top := Selection.Top;
  PasteRect.Right := Selection.Left + RowWidth - 1;
  PasteRect.Bottom := Selection.Top + PasteRowcount - 1;
  if( MultiplePasteCount>1 ) then begin
    for i:=1 to MultiplePasteCount-1 do begin
      for ARow:=PasteRect.Top+(i*PasteRowCount) to PasteRect.Top+((i+1)*PasteRowCount)-1 do begin
        if( ARow<=High(m_CellInfo[0]) ) then begin
          for ACol:=PasteRect.Left to PasteRect.Right do begin
            if( ACol<=High(m_CellInfo) ) then begin
              if( not m_CellInfo[ACol,ARow].bReadOnly ) then begin
                Cells[ACol,ARow] := Cells[ACol,ARow-PasteRowCount];
                m_CellInfo[ACol,ARow].Hardness := m_CellInfo[ACol, ARow-PasteRowCount].Hardness;
              end;
            end;
          end;
          Inc( PasteRect.Bottom );
        end;
      end;
    end;
  end;
  Values.Free();
  Info.Free();
  if( PasteRect.bottom >= m_UsedRowCount ) then
    SetUsedRowCount( PasteRect.bottom+1 );
end;

{ deletes all the selected cells }
procedure TPersenseGrid.DeleteSelected();
var
  ACol, ARow: integer;
  NewRect: TGridRect;
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.DeleteSelected' );
  for ARow:=Selection.Top to Selection.Bottom do begin
    for ACol:=Selection.Left to Selection.Right do begin
      if( (ACol<0) or (ARow<0) ) then exit;
      Cells[ACol,ARow] := '';
      if( (ACol<=High(m_CellInfo)) and (ARow<=High(m_CellInfo[0])) ) then
        m_CellInfo[ACol,ARow].Hardness := empty;
    end;
  end;
  NewRect.Left := Selection.Left;
  NewRect.Right := Selection.Left;
  NewRect.Top := Selection.Top;
  NewRect.Bottom := Selection.Top;
  Selection := NewRect;
  AdjustUsedRowCount();
end;

{ mouse handling.  For left button we have to enable dragging for multiple cell
  selection.  For right button we have to enable context menu popup and moving
  around input accordingly }
procedure TPersenseGrid.MouseDown( Button: TMouseButton; Shift: TShiftState; X, Y: integer );
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
  DebugString: string;
begin
  MouseToCell( X, Y, ACol, ARow );
  if( MasterLog.GetLevel() <= LVL_LOW ) then begin
    DebugString := Format( 'TPersenseGrid.MouseDown on %d, %d', [ACol,ARow] );
    MasterLog.Write( LVL_LOW, DebugString );
  end;
  if( ARow > m_UsedRowCount ) then exit;
  if( Button = mbLeft ) then begin
    if( EditorMode ) then begin
      if( not CellIsValid( Col, Row ) ) then exit;
    end;
    if( ACol = FixedCols-1 ) then begin
      m_bRowMouseDrag := true;
      SelectedRect.Left := FixedCols;
      SelectedRect.Top := ARow;
      SelectedRect.Right := ColCount-1;
      SelectedRect.Bottom := ARow;
      Selection := SelectedRect;
      m_CurrentMouseDragRow := ARow;
    end else begin
      if( (Selection.Left=ACol) and (Selection.Right=ACol) and
          (Selection.Top=ARow) and (Selection.Bottom=ARow) ) then begin
        m_MouseDownRow := ARow;
        m_MouseDownCol := ACol;
      end else begin
        // if it's not a single cell selection this will effectively
        // disable selecting.
        m_MouseDownRow := -1;
        m_MouseDownCol := -1;
      end;
      Options := Options - [goEditing] + [goRangeSelect];
    end;
  end else if( Button = mbRight ) then begin
    { from word processor experimentation it seems that if a range is selected
      when the right button is hit then the input point is unchanged.  However
      if no range is selected then change the input to be the currently clicked
      cell.  So check to see if the selected area is only one cell, in which case
      change the selected cell to be the ones that the user just clicked on. }
    if( (Selection.Left=Selection.Right) and (Selection.Top=Selection.Bottom) ) then begin
      SelectedRect.Left := ACol;
      SelectedRect.Top := ARow;
      SelectedRect.Right := ACol;
      SelectedRect.Bottom := ARow;
      Selection := SelectedRect;
    end;
  end;

  inherited MouseDown( Button, Shift, X, Y );
end;

{ just turns back on single cell selection. }
procedure TPersenseGrid.MouseUp( Button: TMouseButton; Shift: TShiftState; X, Y: integer );
var
  ACol, ARow: integer;
  DebugString: string;
begin
  MouseToCell( X, Y, ACol, ARow );
  if( MasterLog.GetLevel() <= LVL_LOW ) then begin
    DebugString := Format( 'TPersenseGrid.MouseUp on %d, %d', [ACol,ARow] );
    MasterLog.Write( LVL_LOW, DebugString );
  end;
  m_bRowMouseDrag := false;
  if( Button = mbLeft ) then
    Options := Options + [goEditing] - [goRangeSelect];
  if( (ACol = m_MouseDownCol) and (ARow = m_MouseDownRow) ) then
    EditorMode := true;
  inherited MouseUp( Button, Shift, X, Y );
  // date fields where putting the cursor at
  // the END of the field.  All other fields
  // would put them at the begining.
  if( InPlaceEditor <> nil ) then begin
    if( InPlaceEditor.Text = '  /  /    ' ) then
      InPlaceEditor.SelStart := 0;
      InPlaceEditor.SelLength := 1;
  end;
end;

{ does the multiple cell select stuff if the left button is held down }
procedure TPersenseGrid.MouseMove( Shift: TShiftState; X, Y: integer );
var
  ACol, ARow: integer;
  SelectedRect: TGridRect;
begin
  MouseToCell( X, Y, ACol, ARow );
  if( (ACol=-1) or (ARow=-1) ) then begin
    HideLongTextWindow();
    exit;
  end;
  inherited MouseMove( Shift, X, Y );
  if( m_bRowMouseDrag ) then begin
    SelectedRect := Selection;
    if( ARow > m_CurrentMouseDragRow ) then begin
      if( ARow > SelectedRect.Bottom ) then
        SelectedRect.Bottom := ARow
      else if( ARow > SelectedRect.Top ) then
        SelectedRect.Top := ARow
    end else if( ARow < m_CurrentMouseDragRow ) then begin
      if( ARow < SelectedRect.Top ) then
        SelectedRect.Top := ARow
      else if( ARow < SelectedRect.Bottom ) then
        SelectedRect.Bottom := ARow
    end;
    m_CurrentMouseDragRow := ARow;
    Selection := SelectedRect;
    exit;
  end;
  if( ShouldDisplayLongTextWindow( ACol, ARow ) ) then begin
    { Sometimes you get this even when the mouse hasn't moved. }
    if( ((m_MouseX <> X) or (m_MouseY <> Y)) and not EditorMode ) then begin
      DisplayLongTextWindow( ACol, ARow );
    end;
  end else begin
    if( ((m_MouseX <> X) or (m_MouseY <> Y)) and not EditorMode ) then begin
      HideLongTextWindow();
    end;
  end;
  m_MouseX := X;
  m_MouseY := Y;
end;

{ fires when the mouse leaves the grid.  }
procedure TPersenseGrid.CMMouseLeave( var AMsg: TMessage );
begin
  HideLongTextWindow();
end;

{ Helper function to figure out whether or not we need to see the long text window }
function TPersenseGrid.ShouldDisplayLongTextWindow( ACol, ARow: integer ): boolean;
var
  BackupFont: TFont;
  TheCellRect: TRect;
begin
  BackupFont := TFont.Create();
  BackupFont.Assign( Canvas.Font );
  if( m_CellInfo[ACol,ARow].Hardness = outp ) then
    Canvas.Font.Assign( OutpCellFont )
  else
    Canvas.Font.Assign( CellFont );
  TheCellRect := CellRect( ACol, ARow );
  if( (TheCellRect.Right-TheCellRect.Left) < Canvas.TextWidth(Cells[ACol, ARow]) ) then
    ShouldDisplayLongTextWindow := true
  else
    ShouldDisplayLongTextWindow := false;
  Canvas.Font.Assign( BackupFont );
  BackupFont.Free();
end;

{ displays the long text window }
procedure TPersenseGrid.DisplayLongTextWindow( ACol, ARow: integer );
var
  TheCellRect: TRect;
  ClientPoint: TPoint;
  ScreenPoint: TPoint;
begin
  if( m_CellInfo[ACol,ARow].Hardness = outp ) then
    m_WideCellTextWindow.Canvas.Font.Assign( OutpCellFont )
  else
    m_WideCellTextWindow.Canvas.Font.Assign( CellFont );
  TheCellRect := CellRect( ACol, ARow );
  ClientPoint.X := TheCellRect.Left;
  ClientPoint.Y := TheCellRect.Top;
  ScreenPoint := ClientToScreen( ClientPoint );
  TheCellRect.Bottom := TheCellRect.Bottom - TheCellRect.Top;
  TheCellRect.Left := ScreenPoint.X;
  TheCellRect.Top := ScreenPoint.Y;
  TheCellRect.Right := TheCellRect.Left + m_WideCellTextWindow.Canvas.TextWidth(Cells[ACol,ARow]) + RightCellMargin + LeftCellMargin;
  TheCellRect.Bottom := TheCellRect.Bottom + TheCellRect.Top - 4;
  m_WideCellTextWindow.ActivateHint( TheCellRect, Cells[ACol,ARow] );
end;

{ just an abstraction in case we need it }
procedure TPersenseGrid.HideLongTextWindow();
begin
  m_WideCellTextWindow.ReleaseHandle();
end;

{
  I specifically did this the way you're NOT supposed to do it, because the way
  you are supposed to do it doesn't work.  If I override KeyDown directly then
  I don't get the messages while editing is happening (I'm guessing all the messages
  are going to the InPlaceEditor which isn't forwarding them to the root KeyDown).
  So what I'm doing is overriding OnKeyDown, and if I need to I'll export some other
  Form of OnKeyDown event.
}
procedure TPersenseGrid.CustomKeyDown( Sender: TObject; var Key: Word; Shift: TShiftState );
var
  TheString: string;
  DebugString: string;
begin
        inherited;
  if( MasterLog.GetLevel() <= LVL_LOW ) then begin
    DebugString := Format( 'TPersenseGrid.CustomKeyDown with %d', [Key] );
    MasterLog.Write( LVL_LOW, DebugString );
  end;
  // The first set is keys read with Editor Mode on
  if( EditorMode = true ) then begin;
    if( Key = VK_RIGHT ) then begin;
      TheString := InplaceEditor.EditText;
      if( InplaceEditor.SelStart = Length(TheString) ) then begin;
        if( not CellIsValid( Col, Row ) ) then begin
          Key := 0;
          exit;
        end;
        MoveSelectedRight();
      end;
    end else if( (Key=VK_TAB) and not (ssShift in Shift) ) then begin;
      Key := 0;
      if( not CellIsValid( Col, Row ) ) then exit;
      MoveSelectedRight();
    end else if( (Key = VK_LEFT) and not (ssShift in Shift) ) then begin;
      TheString := InplaceEditor.EditText;
      if( InplaceEditor.SelStart = 0 ) then begin
        if( (InplaceEditor.SelLength=0) or
            ((m_ColArray[Col].ColType=ColTypeDate) and (InplaceEditor.SelLength=1) ) ) then begin
          if( not CellIsValid( Col, Row ) ) then exit;
          MoveSelectedLeft();
          Key := 0;
        end else begin
          // the default is to highlight everything when you press enter on the
          //  cell.  This is fine, but then if you press left you should be able to
          //  edit, not switch to the next call to the left
          InplaceEditor.SelLength := 0;
        end;
      end;
    end else if( (Key = VK_TAB) and (ssShift in Shift) ) then begin;
      Key := 0;
      if( not CellIsValid( Col, Row ) ) then exit;
      MoveSelectedLeft();
    end;
  end else begin;
    // This next set of keys are read when the editor mode is off
    if( (Key = VK_RIGHT) or ((Key = VK_TAB) and not (ssShift in Shift)) ) then begin;
      if( Col+1 = ColCount ) then begin;
        MoveSelectedRight();
        Key := 0; //disabled default handling, which would screw this up
      end;
    end else if( (Key = VK_LEFT) or ((Key = VK_TAB) and (ssShift in Shift)) ) then begin;
      if( Col = FixedCols ) then begin;
        MoveSelectedLeft();
        Key := 0; //disabled default handling, which would screw this up
      end;
    end else if( Key = VK_SHIFT ) then begin
      // Shift key means the user wants to select a bunch of rows.
      Options := Options - [goEditing] + [goRangeSelect];
    end else if( KEY=VK_INSERT ) then begin
      if( ssCtrl in Shift ) then begin
        if( Assigned(FOnEditCopy) ) then
          OnEditCopy( Self );
      end else if( ssShift in Shift ) then begin
        if( Assigned(FOnEditPaste) ) then
          OnEditPaste( Self );
      end;
    end else if( (Key=VK_DELETE) and (ssShift in Shift) ) then begin
      if( Assigned(FOnEditCut) ) then
        OnEditCut( Self );
    end;
  end;
  // This set of keys are read regardless of EditorMode
  if( Key = VK_DOWN ) then begin
    if( not CellIsValid( Col, Row ) ) then begin
      Key := 0;
      exit;
    end;
    if( (Row>=m_UsedRowCount)or(Row+1>=RowCount) ) then begin
      Row := 0;
      Key := 0; // disabled default handling, which would screw this up
      if( assigned(FOnDownAfterGrid) ) then
        OnDownAfterGrid( Self );
    end;
  end else if( Key = VK_UP ) then begin
    if( not CellIsValid( Col, Row ) ) then begin
      Key := 0;
      exit;
    end;
    if( (Row = 0) and assigned(FOnUpBeforeGrid) ) then begin
      OnUpBeforeGrid( Self );
    end;
  end else if( (Key = VK_PRIOR) and (ssCtrl in Shift) ) then begin
    if( not CellIsValid( Col, Row ) ) then exit;
    Row := 0;
  end else if( (Key = VK_NEXT) and (ssCtrl in Shift) ) then begin
    if( not CellIsValid( Col, Row ) ) then exit;
    Row := RowCount-1;
  end;
  // slightly different because it needs to be either or
  if( Key = VK_RETURN ) then begin
    // Set this flag so that the OnSetEditText function will know that it
    // was called by pressing the Enter Key.
    // Cell validity is checked in OnKeyPressed
    m_EnterKeyPressed := true;
  end else
    m_EnterKeyPressed := false;
end;

procedure TPersenseGrid.CustomKeyPress( Sender: TObject; var Key: Char );
var
  DebugString: string;
begin
  if( MasterLog.GetLevel() <= LVL_LOW ) then begin
    DebugString := Format( 'TPersenseGrid.CustomKeyPress with %s', [Key] );
    MasterLog.Write( LVL_LOW, DebugString );
  end;
  // special case, if it's a date and the user presses 't' then fill
  // in today's date.
  if( (Key='t') and (m_ColArray[Col].ColType=ColTypeDate) ) then begin
    Cells[Col,Row] := SysUtils.datetostr( now );
    Key := #0;
    exit;
  end;
  // another special case.  Some people use the space bar to
  // delete contents of a cell.  So turn the space into a backspace
  if( (Key=' ') and (InplaceEditor.SelLength = length(Cells[Col,Row])) ) then
    Key := #8;
  // strip out letters and such.  Only things that should get through:
  //  letters 0-9
  //  if it's a date then the letter t=today
  //  comma
  //  period
  //  backspace (#8)
  //  Enter (#13)
  //  forward slash
  //  space bar
  //  negative sign
  if( not (((Key>='0') and (Key<='9')) or (Key=',') or (Key='.') or
            (Key=#8) or (Key=#13) or (Key='/') or (Key=' ') or (Key='-')) ) then begin
    Key := #0;
    exit;
  end;
  if( ((Key<>#13)and(Key<>#8)) and (length(Cells[Col,Row]) >= m_ColArray[Col].MaxLength ) and (InplaceEditor.SelLength=0) ) then begin
    Key := #0;
    exit;
  end;
  if( (m_ColArray[Col].ColType=ColTypeDate) and (Key='/') ) then begin
    if( InplaceEditor.SelStart = 3 ) then begin
      Key := #0;
      exit;
    end;
  end;
  if( EditorMode and (Key=#13) ) then begin
    if( not CellIsValid(Col, Row) ) then begin
      Key := #0;
      m_EnterKeyPressed := false;
    end;
  end;
end;

procedure TPersenseGrid.CustomKeyUp( Sender: TObject; var Key: Word; Shift: TShiftState );
var
  DebugString: string;
begin
  if( MasterLog.GetLevel() <= LVL_LOW ) then begin
    DebugString := Format( 'TPersenseGrid.CustomKeyUp with %d', [Key] );
    MasterLog.Write( LVL_LOW, DebugString );
  end;
  if( not EditorMode ) then begin
    if( Key = VK_SHIFT ) then
      Options := Options + [goEditing] - [goRangeSelect];
  end;
end;

function TPersenseGrid.GetEditMask( ACol, ARow: longint ): string;
begin
  if( m_ColArray[ACol].ColType = ColTypeDate ) then
    GetEditMask := '00/00/0000;1; '
  else
    GetEditMask := '';
end;

procedure TPersenseGrid.Print( FirstRow, Top, Left, Width: integer; var Height: integer; RowsToPrint: integer; var RowsPrinted: integer );
var
  BoundingRect: TRect;
  CellRect: TRect;
  yPos: integer;
  CellHeight, NewHeight: integer;
  ARow, ACol: integer;
  i: integer;
const
  LeftMargin = 10;
  RightMargin = 10;
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.Print' );
  RowsPrinted := 0;
  // find the cell height
  Printer.Canvas.Font.Assign( OutpCellFont );
  CellHeight := Printer.Canvas.TextHeight( '1234567890' );
  Printer.Canvas.Font.Assign( CellFont );
  NewHeight := Printer.Canvas.TextHeight( '1234567890' );
  if( NewHeight > CellHeight ) then
    CellHeight := NewHeight;
  Printer.Canvas.Font.Assign( Font );
  NewHeight := Printer.Canvas.TextHeight( '1234567890' );
  if( NewHeight > CellHeight ) then
    CellHeight := NewHeight;
  CellHeight := CellHeight + 30;
  BoundingRect.Top := Top;
  BoundingRect.Left := Left;
  BoundingRect.Right := Left+Width;
  BoundingRect.Bottom := Top;
  // fill grid
  yPos := Top;
  for ARow:=FirstRow to RowsToPrint-1 do begin
    if( yPos > (Height+Top) ) then
      break;
    Printer.Canvas.MoveTo( Left, yPos );
    Printer.Canvas.LineTo( Left+Width, yPos );
    CellRect.Top := yPos+1;
    CellRect.Bottom := CellRect.Top + CellHeight;
    for ACol:=0 to ColCount-1 do begin
      CellRect.Left := Left+Trunc(m_ColArray[ACol].LeftPos*Width);
      if( ACol = ColCount-1 ) then
        CellRect.Right := Left+Width
      else
        CellRect.Right := Left+Trunc(m_ColArray[ACol+1].LeftPos*Width);
      DrawCellToCanvas( ACol, ARow, CellRect, LeftMargin, RightMargin, Printer.Canvas, true );
    end;
    BoundingRect.Bottom := BoundingRect.Bottom + CellHeight;
    yPos := yPos + CellHeight;
    Inc( RowsPrinted );
  end;
  Printer.Canvas.MoveTo( Left, yPos );
  Printer.Canvas.LineTo( Left+Width, yPos );
  // draw vertical bars
  for i:=0 to ColCount-1 do begin
    Printer.Canvas.MoveTo( Left+Trunc(m_ColArray[i].LeftPos*Width), Top );
    Printer.Canvas.LineTo( Left+Trunc(m_ColArray[i].LeftPos*Width), BoundingRect.Bottom );
  end;
  Printer.Canvas.MoveTo( Left+Width, Top );
  Printer.Canvas.LineTo( Left+Width, BoundingRect.Bottom );
  Height := BoundingRect.Bottom - BoundingRect.Top;
  Printer.Canvas.Brush.Color := clWhite;
end;

procedure TPersenseGrid.EnableComboBox( ACol, ARow: integer );
var
  CurrentString: string;
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.EnableComboBox');
  m_ComboBox.Visible := true;
  m_ComboBoxEditorMode := true;
  m_ComboBox.SetFocus();
  m_ComboBox.BringToFront();
  // must call it this way (as opposed to just reading cells[ACol,ARow])
  // in order to go through the usual lines of events
  CurrentString := GetEditText( ACol, ARow );
  m_ComboBox.ItemIndex := m_ComboBox.Items.IndexOf( CurrentString );
end;

procedure TPersenseGrid.DisableComboBox();
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.DisableComboBox');
  m_ComboBox.Visible := false;
  m_ComboBox.Clear();
  m_ComboBoxEditorMode := false;
  SetFocus();
end;

procedure TPersenseGrid.OnComboBoxExit( Sender: TObject );
var
  TheString: string;
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.OnComboBoxExit');
  with sender as TCombobox do begin
    // Store the values before because disabling the combo box
    // will wipe out the values we need
    if( ItemIndex >= 0 ) then
      TheString := Items[ItemIndex];
    DisableComboBox();
    if TheString <> '' then
      // use SetEditText instead of just doing cells[Col,Row] so that
      // the correct events will be fired
      SetEditText( Col, Row, TheString );
  end;
end;

procedure TPersenseGrid.OnComboBoxKeyDown( Sender: TObject; var Key: Word; Shift: TShiftState );
var
  GridRect: TGridRect;
begin
  MasterLog.Write( LVL_LOW, 'TPersenseGrid.OnComboBoxKeyDown');
  GridRect := Selection;
  if( Key=VK_RIGHT ) then begin;
    DisableComboBox();
    if( not (ssShift in Shift) ) then
      Inc( GridRect.Left );
    Inc( GridRect.Right );
    Selection := GridRect;
    Key := 0;
  end else if( Key=VK_LEFT ) then begin;
    DisableComboBox();
    Dec( GridRect.Left );
    if( not (ssShift in Shift) ) then
      Dec( GridRect.Right );
    Selection := GridRect;
  end else if( Key=VK_RETURN ) then begin
    m_EnterKeyPressed := true;
    DisableComboBox();
  end;
  inherited;
end;

procedure Register;
begin
  RegisterComponents('MyComponents', [TPersenseGrid]);
end;

end.
