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
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
  public
    constructor Create( AOwner: TComponent ); override;
    destructor Destroy(); override;
    function GetType(): TScreenType; override;
    procedure OnPrint( Header: TStringList ); override;
    function GetTableOptions( var SendToFile: boolean; var CommaSeperated: boolean; PerYr: integer ): boolean;
    procedure SetOutput( Output: TStringList );
    procedure SetHeaders( Line1: string; Line2: string );
    procedure DrawOutput();
  protected
    m_Output: TStringList;
  end;

implementation

uses peData, Printers, Globals, TableOptionsDlgUnit;

{$R *.dfm}
constructor TTableOut.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  m_Output := nil;
end;

destructor TTableOut.Destroy();
begin
  m_Output.Free();
  inherited Destroy();
end;

procedure TTableOut.SetOutput( Output: TStringList );
begin
  m_Output.Free();
  m_Output := Output;
end;

procedure TTableOut.SetHeaders( Line1: string; Line2: string );
begin
  HeaderLine1.Caption := Line1;
  HeaderLine2.Caption := Line2;
end;

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

function TTableOut.GetType(): TScreenType;
begin
  GetType := TableOutType;
end;

procedure TTableOut.OnPrint( Header: TStringList );
var
  yPos: integer;
  TextHeight: integer;
  RowsPrinted: integer;
  UsablePageWidth: integer;
  i: integer;
  HeaderCount: integer;
  yHeaderEnd: integer;
const
  Margin=80;
begin
  Printer.BeginDoc();
  Printer.Title := 'Persense Amortization Screen';
  Printer.Canvas.Font.Assign( OutputBox.Font );
  Printer.Canvas.Font.Pitch := fpFixed;
  Printer.Canvas.Brush.Color := clWhite;
  TextHeight := Printer.Canvas.TextHeight( 'Height' );
  UsablePageWidth := Printer.PageWidth - 2*Margin;
  RowsPrinted := 0;
  if( Header = nil ) then
    HeaderCount := 0
  else
    HeaderCount := Header.Count+1;
  PrintHeader( Printer, TextHeight, Header );
  yPos := 100 + (HeaderCount*TextHeight);
  yHeaderEnd := yPos;
  while( RowsPrinted < m_Output.Count ) do begin
    // draw headers
    Printer.Canvas.TextOut( Margin, yPos, HeaderLine1.Caption );
    yPos := yPos + TextHeight;
    Printer.Canvas.TextOut( Margin, yPos, HeaderLine2.Caption );
    yPos := yPos + TextHeight;
    // draw box
    Printer.Canvas.MoveTo( Margin, yHeaderEnd );
    Printer.Canvas.LineTo( UsablePageWidth, yHeaderEnd );
    Printer.Canvas.LineTo( UsablePageWidth, yPos );
    Printer.Canvas.LineTo( Margin, yPos );
    Printer.Canvas.LineTo( Margin, yHeaderEnd );
    yPos := yPos + TextHeight;
    for i:=RowsPrinted to m_Output.Count-1 do begin
      Printer.Canvas.TextOut( Margin, yPos, m_Output[i] );
      yPos := yPos + TextHeight;
      Inc( RowsPrinted );
      if( yPos > Printer.PageHeight - 2*Margin ) then break;
    end;
    if( yPos > Printer.PageHeight - 2*Margin ) then begin
      Printer.NewPage();
      yPos := 100 + (HeaderCount*TextHeight);
    end;
  end;
  Printer.EndDoc();
end;

procedure TTableOut.FormClose(Sender: TObject;
  var Action: TCloseAction);
begin
  OnFormClose( Action );
end;

end.