{ ===========================================================================
  Unit:  TableOutUnit
  Role:  MDI child window that displays a finished, fixed-pitch text "table"
         report (e.g. an amortization schedule) and prints it.

  After a calculation screen produces its tabular result, the lines are pushed
  here as a TStringList (SetOutput) along with two header lines (SetHeaders).
  DrawOutput renders them into a read-only memo; OnPrint paginates and prints
  the same content to the Windows printer, repeating the header block and a
  surrounding box on every page. GetTableOptions pops the Table Options dialog
  to let the user choose file-vs-screen and comma-separated formatting before
  export.

  Derives from TMDIChild and reports GetType = TableOutType. It OWNS the
  TStringList it is given (frees the old one on each SetOutput and in Destroy).

  { Go port: n/a -- report-display MDI child + Windows printer pagination; no
    financial logic (it only renders pre-formatted rows). In the web port the
    schedule is rendered as an HTML table in cmd/persense/static/index.html
    from the /api/amortization/calc JSON, and printing is the browser's job. }
  ===========================================================================}
unit TableOutUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, childwin, FileIOUnit, StdCtrls;

type
  TTableOut = class(TMDIChild)
    OutputBox: TMemo;
    GroupBox1: TGroupBox;
    HeaderLine1: TLabel;
    HeaderLine2: TLabel;
    { OnClose: routes through inherited MDIChild close handling. }
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
  public
    { Construct with no table data yet (m_Output = nil). }
    constructor Create( AOwner: TComponent ); override;
    { Free the owned output list, then inherited cleanup. }
    destructor Destroy(); override;
    { Identify this child as TableOutType. }
    function GetType(): TScreenType; override;
    { Print the table (with repeated headers/box) to the Windows printer. }
    procedure OnPrint(); override;
    { Show the Table Options dialog; return user choices and true on OK. }
    function GetTableOptions( var SendToFile: boolean; var CommaSeperated: boolean; PerYr: integer ): boolean;
    { Take ownership of the table lines (frees any previous list). }
    procedure SetOutput( Output: TStringList );
    { Set the two header caption lines shown above the table. }
    procedure SetHeaders( Line1: string; Line2: string );
    { Render the current m_Output lines into the on-screen memo. }
    procedure DrawOutput();
  protected
    { owned list of formatted table rows }
    m_Output: TStringList;
  end;

implementation

uses peData, Printers, Globals, TableOptionsDlgUnit;

{$R *.dfm}

{ Create
  Purpose: construct the report window with no data attached yet.
  Side effects: m_Output := nil. }
constructor TTableOut.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  m_Output := nil;
end;

{ Destroy
  Purpose: release the owned output list before the inherited teardown.
  Side effects: frees m_Output (nil-safe). }
destructor TTableOut.Destroy();
begin
  m_Output.Free();
  inherited Destroy();
end;

{ SetOutput
  Purpose: hand the window a new set of table rows, taking ownership.
  Param:   Output - the TStringList of formatted rows to display/print.
  Side effects: frees the previously held list, then stores Output. }
procedure TTableOut.SetOutput( Output: TStringList );
begin
  m_Output.Free();
  m_Output := Output;
end;

{ SetHeaders
  Purpose: set the two header caption lines drawn above the table.
  Params:  Line1, Line2 - header text (e.g. column titles / underline rule).
  Side effects: updates HeaderLine1/HeaderLine2 captions. }
procedure TTableOut.SetHeaders( Line1: string; Line2: string );
begin
  HeaderLine1.Caption := Line1;
  HeaderLine2.Caption := Line2;
end;

{ GetTableOptions
  Purpose: prompt the user for export options via the Table Options dialog.
  Params (all out except PerYr):
    SendToFile     - true to export to a file, false for clipboard/screen.
    CommaSeperated - true for comma-separated formatting (CSV-style).
    PerYr          - in: payments-per-year, passed to the dialog for display.
  Returns: true if the user clicked OK (out-params valid); false if cancelled.
  Side effects: creates, shows, and frees a TTableOptionsDlg. }
function TTableOut.GetTableOptions( var SendToFile: boolean; var CommaSeperated: boolean; PerYr: integer ): boolean;
var
  OptionsDlg: TTableOptionsDlg;
begin
  GetTableOptions := false;
  OptionsDlg := TTableOptionsDlg.Create( Self );
  OptionsDlg.SetPerYr( PerYr );
  OptionsDlg.ShowModal();
  if( OptionsDlg.ModalResult<>MrOK ) then exit;
  SendToFile := OptionsDlg.SendToFile();
  CommaSeperated := OptionsDlg.CommaSeperated();
  OptionsDlg.Free();
  GetTableOptions := true;
end;

{ DrawOutput
  Purpose: (re)render the held table rows into the on-screen memo.
  Side effects: clears the memo, resizes it to fill below the header group box,
                then inserts each row.
  NOTE: rows are inserted at index 0 from last to first, which preserves the
        original top-to-bottom order while avoiding an append reflow. }
procedure TTableOut.DrawOutput();
var
  i: integer;
begin
  OutputBox.Clear();
  OutputBox.Height := Height-GroupBox1.Height-30;
  for i:=m_Output.Count-1 downto 0 do begin
    OutputBox.Lines.insert( 0, m_Output[i] );
  end;
end;

{ GetType
  Purpose: identify this MDI child's kind.
  Returns: TableOutType. }
function TTableOut.GetType(): TScreenType;
begin
  GetType := TableOutType;
end;

{ OnPrint
  Purpose: paginate and print the whole table to the default Windows printer.
  Side effects: drives the Printer object (BeginDoc..EndDoc), drawing on each
                page the two header lines, a bounding box, then as many data
                rows as fit before NewPage. Uses the memo's font for metrics.
  Triggered: by the File > Print command routed through the MDI child. }
procedure TTableOut.OnPrint();
var
  yPos: integer;
  TextHeight: integer;
  RowsPrinted: integer;
  UsablePageWidth: integer;
  i: integer;
const
  Margin=80;
begin
  Printer.BeginDoc();
  Printer.Title := 'Persense Amortization Screen';
  Printer.Canvas.Font.Assign( OutputBox.Font );
  Printer.Canvas.Brush.Color := clWhite;
  TextHeight := Printer.Canvas.TextHeight( 'Height' );
  UsablePageWidth := Printer.PageWidth - 2*Margin;
  RowsPrinted := 0;
  yPos := 100;
  { Outer loop = one page per iteration; continues until every row is printed }
  while( RowsPrinted < m_Output.Count ) do begin
    // draw headers
    Printer.Canvas.TextOut( Margin, yPos, HeaderLine1.Caption );
    yPos := yPos + TextHeight;
    Printer.Canvas.TextOut( Margin, yPos, HeaderLine2.Caption );
    yPos := yPos + TextHeight;
    // draw box
    Printer.Canvas.MoveTo( Margin, Margin );
    Printer.Canvas.LineTo( UsablePageWidth, Margin );
    Printer.Canvas.LineTo( UsablePageWidth, yPos );
    Printer.Canvas.LineTo( Margin, yPos );
    Printer.Canvas.LineTo( Margin, Margin );
    yPos := yPos + TextHeight;
    { Emit rows until the page is full (then break to start a new page) }
    for i:=RowsPrinted to m_Output.Count-1 do begin
      Printer.Canvas.TextOut( Margin, yPos, m_Output[i] );
      yPos := yPos + TextHeight;
      Inc( RowsPrinted );
      if( yPos > Printer.PageHeight - 2*Margin ) then break;
    end;
    { If we stopped because the page filled, eject and reset for next page }
    if( yPos > Printer.PageHeight - 2*Margin ) then begin
      yPos := 100;
      Printer.NewPage();
    end;
  end;
  Printer.EndDoc();
end;

{ FormClose
  Purpose: route window close through the inherited MDIChild handling.
  Params:  Sender (unused); Action - close action, possibly modified. }
procedure TTableOut.FormClose(Sender: TObject;
  var Action: TCloseAction);
begin
  OnFormClose( Action );
end;

end.