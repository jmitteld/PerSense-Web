{ ===========================================================================
  Unit:  SelectAPRDlgUnit
  Role:  Modal picker that asks the user which other mortgage row to compare
         against, as part of the APR comparison workflow.

  After the user selects one row (Row1) to compare, this dialog lists all the
  other eligible rows in a list box and lets the user pick exactly one. The
  caller supplies the candidate row numbers via SetInputs and retrieves the
  chosen row number via GetSelectedRow.
  ===========================================================================}
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
    { lists candidate row numbers to compare against }
    ListBox1: TListBox;
  private
    { Private declarations }
  public
    { Populate the list with comparable row numbers; Row1 is the fixed row. }
    procedure SetInputs( Row1: integer; RowOptions: array of integer );
    { Return the row number currently highlighted in the list box. }
    function GetSelectedRow() : integer;
  end;

var
  SelectAPRDlg: TSelectAPRDlg;

implementation

{$R *.dfm}

{ SetInputs
  Purpose: fill the list box with the row numbers that may be compared against
           the user's fixed selection.  These are the valid rows that can be
           compared to whatever the user selected.
  Params:  Row1       - the already-selected ("Compare Row") number, shown in
                         the Label1 caption (not added to the list).
           RowOptions - open array of candidate row numbers to list.
  Side effects: clears and repopulates ListBox1, sets Label1.Caption, and
                pre-selects the first list item.
  Triggered: by the caller before ShowModal. }
{ Go port: n/a -- DOS text UI; the "which two rows to compare" choice is made in the web frontend cmd/persense/static/index.html, which posts the two rows to internal/api/handlers.go: HandleMortgageCompare (line 625). }
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

{ GetSelectedRow
  Purpose: return the row number the user highlighted in the list box.
  Returns: the selected row number as an integer (the list text is parsed via
           StrToFloat then truncated to drop any formatting).
  Triggered: by the caller after ShowModal returns mrOK. }
{ Go port: n/a -- DOS text UI; row selection is client-side in cmd/persense/static/index.html. }
function TSelectAPRDlg.GetSelectedRow() : integer;
var
  StringVal: string;
begin
  StringVal := ListBox1.items[ListBox1.ItemIndex];
  GetSelectedRow := Trunc(StrToFloat( StringVal ));
end;

end.
