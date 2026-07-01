{ ===========================================================================
  Unit:  ClipboardSettingsDlgUnit
  Role:  Modal dialog for configuring how grid data is serialized to the
         Windows clipboard.

  When the user copies a table (mortgage / amortization / PV grid) to the
  clipboard, the rows and cells must be joined into a single text blob. This
  dialog lets the user choose the delimiter inserted *between cells* of a row
  (CellDelimiter, e.g. Tab vs comma) and the delimiter inserted *between rows*
  (RowDelimiter, e.g. CR/LF vs newline). Those choices map to the global
  clipboard-format config consumed by PersenseClipboardUnit when building the
  outgoing text.

  This is a layout-only VCL form (controls defined in the .dfm). The selected
  combo-box values are read back by the calling screen on OK.

  { Go port: n/a -- clipboard-format configuration for a native copy feature;
    no financial logic. In the web port, copy/export is handled client-side in
    cmd/persense/static/index.html (browser clipboard + table serialization). }
  ===========================================================================}
unit ClipboardSettingsDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls, 
  Buttons, ExtCtrls;

type
  TClipboardSettingsDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    { caption for the cell-delimiter selector }
    Label1: TLabel;
    { caption for the row-delimiter selector }
    Label2: TLabel;
    { delimiter inserted between cells within a row }
    CellDelimiter: TComboBox;
    { delimiter inserted between successive rows }
    RowDelimiter: TComboBox;
  private
  public
  end;

var
  ClipboardSettingsDlg: TClipboardSettingsDlg;

implementation

{$R *.dfm}

end.
