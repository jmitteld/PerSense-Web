unit UISettingsDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls, 
  Buttons, ExtCtrls, Dialogs, ComCtrls;

type
  TUISettingsDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    TheFontDialog: TFontDialog;
    OutpCellFontButton: TButton;
    Label4: TLabel;
    OutpCellFontSample: TRichEdit;
    Label5: TLabel;
    CellFontSample: TRichEdit;
    CellFontButton: TButton;
    CellColor: TColorBox;
    SelectedCellColor: TColorBox;
    OutpCellColor: TColorBox;
    procedure OutpCellFontButtonClick(Sender: TObject);
    procedure CellFontButtonClick(Sender: TObject);
    procedure OKBtnClick(Sender: TObject);
  private
  public
    constructor Create(AOwner: TComponent); override;
    procedure SetCellBackgroundColor( NewColor: TColor );
    function GetCellBackgroundColor() : TColor;
    procedure SetHighlightedCellBackgroundColor( NewColor: TColor );
    function GetHighlightedCellBackgroundColor() : TColor;
    procedure SetSoftValueCellBackgroundColor( NewColor: TColor );
    function GetSoftValueCellBackgroundColor() : TColor;
    procedure SetCellFont( NewFont: TFont );
    procedure RetrieveCellFont( var Font: TFont );
    procedure SetOutpCellFont( NewFont: TFont );
    procedure RetrieveOutpCellFont( var Font: TFont );
  end;

var
  UISettingsDlg: TUISettingsDlg;

implementation

uses Registry;
{$R *.dfm}

constructor TUISettingsDlg.Create(AOwner: TComponent);
begin
  inherited Create( AOwner );
  CellFontSample.Text := '1,234.00';
  OutpCellFontSample.Text := '1,234.00';
end;

procedure TUISettingsDlg.SetCellBackgroundColor( NewColor: TColor );
begin
  CellColor.Selected := NewColor;
end;

function TUISettingsDlg.GetCellBackgroundColor() : TColor;
begin
  GetCellBackgroundColor := CellColor.Selected;
end;

procedure TUISettingsDlg.SetHighlightedCellBackgroundColor( NewColor: TColor );
begin
  SelectedCellColor.Selected := NewColor;
end;

function TUISettingsDlg.GetHighlightedCellBackgroundColor() : TColor;
begin
  GetHighlightedCellBackgroundColor := SelectedCellColor.Selected;
end;

procedure TUISettingsDlg.SetSoftValueCellBackgroundColor( NewColor: TColor );
begin
  OutpCellColor.Selected := NewColor;
end;

function TUISettingsDlg.GetSoftValueCellBackgroundColor() : TColor;
begin
  GetSoftValueCellBackgroundColor := OutpCellColor.Selected;
end;

procedure TUISettingsDlg.SetCellFont( NewFont: TFont );
begin
  CellFontSample.DefAttributes.Color := NewFont.Color;
  CellFontSample.DefAttributes.Charset := NewFont.Charset;
  CellFontSample.DefAttributes.Name := NewFont.Name;
  CellFontSample.DefAttributes.Pitch := NewFont.Pitch;
  CellFontSample.DefAttributes.Size := NewFont.Size;
  CellFontSample.DefAttributes.Style := Newfont.Style;
  CellFontSample.DefAttributes.Height := NewFont.Height;
end;

procedure TUISettingsDlg.RetrieveCellFont( var Font: TFont );
begin
  Font.Color := CellFontSample.DefAttributes.Color;
  Font.Charset := CellFontSample.DefAttributes.Charset;
  Font.Name := CellFontSample.DefAttributes.Name;
  Font.Pitch := CellFontSample.DefAttributes.Pitch;
  Font.Size := CellFontSample.DefAttributes.Size;
  Font.Style := CellFontSample.DefAttributes.Style;
  Font.Height := CellFontSample.DefAttributes.Height;
end;

procedure TUISettingsDlg.SetOutpCellFont( NewFont: TFont );
begin
  OutpCellFontSample.DefAttributes.Color := NewFont.Color;
  OutpCellFontSample.DefAttributes.Charset := NewFont.Charset;
  OutpCellFontSample.DefAttributes.Name := NewFont.Name;
  OutpCellFontSample.DefAttributes.Pitch := NewFont.Pitch;
  OutpCellFontSample.DefAttributes.Size := NewFont.Size;
  OutpCellFontSample.DefAttributes.Style := Newfont.Style;
  OutpCellFontSample.DefAttributes.Height := NewFont.Height;
end;

procedure TUISettingsDlg.RetrieveOutpCellFont( var Font: TFont );
begin
  Font.Color := OutpCellFontSample.DefAttributes.Color;
  Font.Charset := OutpCellFontSample.DefAttributes.Charset;
  Font.Name := OutpCellFontSample.DefAttributes.Name;
  Font.Pitch := OutpCellFontSample.DefAttributes.Pitch;
  Font.Size := OutpCellFontSample.DefAttributes.Size;
  Font.Style := OutpCellFontSample.DefAttributes.Style;
  Font.Height := OutpCellFontSample.DefAttributes.Height;
end;

procedure TUISettingsDlg.CellFontButtonClick(Sender: TObject);
var
  Before, After: integer;
  Diff: integer;
begin
  Before := CellFontSample.DefAttributes.Size;
  if( TheFontDialog.Execute() ) then begin
    CellFontSample.DefAttributes.Color := TheFontDialog.Font.Color;
    CellFontSample.DefAttributes.Charset := TheFontDialog.Font.Charset;
    CellFontSample.DefAttributes.Name := TheFontDialog.Font.Name;
    CellFontSample.DefAttributes.Pitch := TheFontDialog.Font.Pitch;
    CellFontSample.DefAttributes.Size := TheFontDialog.Font.Size;
    CellFontSample.DefAttributes.Style := TheFontDialog.Font.Style;
    CellFontSample.DefAttributes.Height := TheFontDialog.Font.Height;
    After := CellFontSample.DefAttributes.Size;
    if( (Before > After) or (After > Before) ) then begin
      Diff := After-Before;
      CellFontSample.Height := CellFontSample.Height + Diff;
      Bevel1.Height := Bevel1.Height + Diff;
      Height := Height + Diff;
      OutpCellFontSample.Top := OutpCellFontSample.Top + Diff;
      Label4.Top := Label4.Top + Diff;
      OutpCellFontButton.Top := OutpCellFontButton.Top + Diff;
      OKBtn.Top := OKBtn.Top + Diff;
      CancelBtn.Top := CancelBtn.Top + Diff;
      CellFontSample.Repaint();
    end;
  end;
end;

procedure TUISettingsDlg.OutpCellFontButtonClick(Sender: TObject);
var
  Before, After: integer;
  Diff: integer;
begin
  Before := OutpCellFontSample.DefAttributes.Size;
  if( TheFontDialog.Execute() ) then begin
    OutpCellFontSample.DefAttributes.Color := TheFontDialog.Font.Color;
    OutpCellFontSample.DefAttributes.Charset := TheFontDialog.Font.Charset;
    OutpCellFontSample.DefAttributes.Name := TheFontDialog.Font.Name;
    OutpCellFontSample.DefAttributes.Pitch := TheFontDialog.Font.Pitch;
    OutpCellFontSample.DefAttributes.Size := TheFontDialog.Font.Size;
    OutpCellFontSample.DefAttributes.Style := [fsBold];
    OutpCellFontSample.DefAttributes.Height := TheFontDialog.Font.Height;
    After := OutpCellFontSample.DefAttributes.Size;
    if( (Before > After) or (After > Before) ) then begin
      Diff := After-Before;
      OutpCellFontSample.Height := OutpCellFontSample.Height + Diff;
      Bevel1.Height := Bevel1.Height + Diff;
      Height := Height + Diff;
      OKBtn.Top := OKBtn.Top + Diff;
      CancelBtn.Top := CancelBtn.Top + Diff;
    end;
  end;
end;

procedure TUISettingsDlg.OKBtnClick(Sender: TObject);
var
  Registry: TRegistry;
  Styles: TFontStyles;
begin
  // write these values to the registry
  Registry := TRegistry.Create();
  Registry.RootKey := HKEY_CURRENT_USER;
  if Registry.OpenKey('SOFTWARE\PerSense\', TRUE) then begin
    Registry.WriteInteger('CellColor', CellColor.Selected );
    Registry.WriteInteger('SelectedCellColor', SelectedCellColor.Selected );
    Registry.WriteInteger('OutpCellColor', OutpCellColor.Selected );
    // cell font
    Registry.WriteInteger( 'CellFontColor', CellFontSample.DefAttributes.Color );
    Registry.WriteInteger( 'CellFontCharset', CellFontSample.DefAttributes.Charset );
    Registry.WriteString( 'CellFontName', CellFontSample.DefAttributes.Name );
    Registry.WriteInteger( 'CellFontPitch', integer(CellFontSample.DefAttributes.Pitch) );
    Registry.WriteInteger( 'CellFontSize', CellFontSample.DefAttributes.Size );
    Styles := CellFontSample.DefAttributes.Style;
    Registry.WriteBinaryData( 'CellFontStyle', Styles, sizeof(TFontStyles) );
    Registry.WriteInteger( 'CellFontHeight', CellFontSample.DefAttributes.Height );
    // outp cell font
    Registry.WriteInteger( 'OutpCellFontColor', OutpCellFontSample.DefAttributes.Color );
    Registry.WriteInteger( 'OutpCellFontCharset', OutpCellFontSample.DefAttributes.Charset );
    Registry.WriteString( 'OutpCellFontName', OutpCellFontSample.DefAttributes.Name );
    Registry.WriteInteger( 'OutpCellFontPitch', integer(OutpCellFontSample.DefAttributes.Pitch) );
    Registry.WriteInteger( 'OutpCellFontSize', OutpCellFontSample.DefAttributes.Size );
    Styles := OutpCellFontSample.DefAttributes.Style;
    Registry.WriteBinaryData( 'OutpCellFontStyle', Styles, sizeof(TFontStyles) );
    Registry.WriteInteger( 'OutpCellFontHeight', OutpCellFontSample.DefAttributes.Height );
  end;
  Registry.Free;
end;

end.
