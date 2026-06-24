{ ===========================================================================
  Unit:  UISettingsDlgUnit
  Role:  Modal dialog for the user's grid appearance preferences (colours and
         fonts), persisted to the Windows registry.

  The grids in Per%Sense colour-code cells by role, and this dialog exposes
  three background colours plus two fonts:
    CellColor          -> normal/input cell background
    SelectedCellColor  -> currently highlighted/selected cell background
    OutpCellColor      -> "soft value" (computed output) cell background
    CellFontSample     -> font used for input cells (edited via a font dialog)
    OutpCellFontSample -> font used for output cells (forced bold on change)
  Two RichEdit "sample" controls double as both a live preview AND the storage
  for the chosen font attributes (read back via the Retrieve* accessors).

  Persistence: OKBtnClick writes every colour and both fonts under the registry
  key HKEY_CURRENT_USER\SOFTWARE\PerSense. The font sample buttons resize the
  dialog to accommodate larger fonts.
  ===========================================================================}
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
    { Pick a new output-cell font via the font dialog (and resize the form). }
    procedure OutpCellFontButtonClick(Sender: TObject);
    { Pick a new input-cell font via the font dialog (and resize the form). }
    procedure CellFontButtonClick(Sender: TObject);
    { Persist all colours and fonts to the registry. }
    procedure OKBtnClick(Sender: TObject);
  private
  public
    { Seed the preview samples with example numeric text. }
    constructor Create(AOwner: TComponent); override;
    { Set/get normal input-cell background colour. }
    procedure SetCellBackgroundColor( NewColor: TColor );
    function GetCellBackgroundColor() : TColor;
    { Set/get highlighted/selected-cell background colour. }
    procedure SetHighlightedCellBackgroundColor( NewColor: TColor );
    function GetHighlightedCellBackgroundColor() : TColor;
    { Set/get "soft value" (computed output) cell background colour. }
    procedure SetSoftValueCellBackgroundColor( NewColor: TColor );
    function GetSoftValueCellBackgroundColor() : TColor;
    { Push/pull the input-cell font into/out of the preview sample. }
    procedure SetCellFont( NewFont: TFont );
    procedure RetrieveCellFont( var Font: TFont );
    { Push/pull the output-cell font into/out of the preview sample. }
    procedure SetOutpCellFont( NewFont: TFont );
    procedure RetrieveOutpCellFont( var Font: TFont );
  end;

var
  UISettingsDlg: TUISettingsDlg;

implementation

uses Registry;
{$R *.dfm}

{ Create
  Purpose: build the dialog and put sample numeric text in both font previews
           so the user can see how each font renders. }
constructor TUISettingsDlg.Create(AOwner: TComponent);
begin
  inherited Create( AOwner );
  CellFontSample.Text := '1,234.00';
  OutpCellFontSample.Text := '1,234.00';
end;

{ SetCellBackgroundColor - set the normal-cell colour box selection. }
procedure TUISettingsDlg.SetCellBackgroundColor( NewColor: TColor );
begin
  CellColor.Selected := NewColor;
end;

{ GetCellBackgroundColor - read the normal-cell colour selection. }
function TUISettingsDlg.GetCellBackgroundColor() : TColor;
begin
  GetCellBackgroundColor := CellColor.Selected;
end;

{ SetHighlightedCellBackgroundColor - set the selected-cell colour box. }
procedure TUISettingsDlg.SetHighlightedCellBackgroundColor( NewColor: TColor );
begin
  SelectedCellColor.Selected := NewColor;
end;

{ GetHighlightedCellBackgroundColor - read the selected-cell colour. }
function TUISettingsDlg.GetHighlightedCellBackgroundColor() : TColor;
begin
  GetHighlightedCellBackgroundColor := SelectedCellColor.Selected;
end;

{ SetSoftValueCellBackgroundColor - set the computed-output cell colour box. }
procedure TUISettingsDlg.SetSoftValueCellBackgroundColor( NewColor: TColor );
begin
  OutpCellColor.Selected := NewColor;
end;

{ GetSoftValueCellBackgroundColor - read the computed-output cell colour. }
function TUISettingsDlg.GetSoftValueCellBackgroundColor() : TColor;
begin
  GetSoftValueCellBackgroundColor := OutpCellColor.Selected;
end;

{ SetCellFont
  Purpose: copy a TFont's attributes into the input-cell preview sample so the
           dialog reflects the current font. Attributes are copied field-by-
           field (RichEdit uses DefAttributes, not a TFont). }
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

{ RetrieveCellFont
  Purpose: copy the input-cell preview's attributes back out into a TFont so
           the caller can apply the chosen font to the actual grid.
  Param (out): Font - receives the selected attributes. }
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

{ SetOutpCellFont - copy a TFont into the output-cell preview sample. }
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

{ RetrieveOutpCellFont - copy the output-cell preview attributes into a TFont. }
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

{ CellFontButtonClick
  Purpose: let the user choose the input-cell font via the standard font dialog,
           copy the result into the preview, and grow the form/controls if the
           font got taller so the layout still fits.
  Side effects: runs TheFontDialog; updates CellFontSample attributes; on a size
                change, shifts dependent controls (output sample, labels,
                buttons) and the form height by the size delta.
  Triggered: by the input-cell "font" button click. }
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

{ OutpCellFontButtonClick
  Purpose: as CellFontButtonClick but for the output-cell font.
  NOTE: the chosen style is overridden to [fsBold] - output cells are always
        rendered bold regardless of what the user picked in the font dialog.
  Side effects: updates OutpCellFontSample; resizes the form on a size change.
  Triggered: by the output-cell "font" button click. }
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

{ OKBtnClick
  Purpose: persist every appearance preference to the registry so the settings
           survive between sessions.
  Storage: under HKEY_CURRENT_USER\SOFTWARE\PerSense - the three cell colours
           plus, for each of the two fonts, Color/Charset/Name/Pitch/Size/
           Style(binary)/Height.
  Side effects: creates the registry key (if absent) and writes; frees the
                TRegistry object.
  Triggered: by the OK button click.
  NOTE: font Style is a set, so it is written as raw binary (WriteBinaryData). }
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
