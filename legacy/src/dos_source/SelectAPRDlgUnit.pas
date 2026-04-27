unit SelectAPRDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls;

type
  TSelectAPRDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    Label1: TLabel;
    Label2: TLabel;
    ListBox1: TListBox;
  private
    { Private declarations }
  public
    procedure SetInputs( Row1: integer; RowOptions: array of integer );
    function GetSelectedRow() : integer;
  end;

var
  SelectAPRDlg: TSelectAPRDlg;

implementation

{$R *.dfm}

{ sets the row numbers that will appear in the list box.  These are the
  valid rows that can be compared to whatever the user selected }
procedure TSelectAPRDlg.SetInputs( Row1: integer; RowOptions: array of integer );
var
  i: integer;
  RowString: string;
begin
  ListBox1.Items.Clear();
  Label1.Caption := Format('Compare Row: %2d', [Row1]);
  for i:=0 to High(RowOptions) do begin
    RowString := Format( '%2d', [RowOptions[i]] );
    ListBox1.Items.Add( RowString );
  end;
  ListBox1.ItemIndex := 0;
end;

function TSelectAPRDlg.GetSelectedRow() : integer;
var
  StringVal: string;
begin
  StringVal := ListBox1.items[ListBox1.ItemIndex];
  GetSelectedRow := Trunc(StrToFloat( StringVal ));
end;

end.
