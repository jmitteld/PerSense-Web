unit ClipboardSettingsDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls, 
  Buttons, ExtCtrls;

type
  TClipboardSettingsDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    Label1: TLabel;
    Label2: TLabel;
    CellDelimiter: TComboBox;
    RowDelimiter: TComboBox;
  private
  public
  end;

var
  ClipboardSettingsDlg: TClipboardSettingsDlg;

implementation

{$R *.dfm}

end.
