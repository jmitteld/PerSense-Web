unit MortgageRowGenerationDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls, Globals, LogUnit;

type
  NewMtgRowInfo = record
    Column: integer;
    Amount: double;
    Repetition: integer;
  end;

  TMortgageRowGenerationDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    GroupBox1: TGroupBox;
    Label1: TLabel;
    Variable1Combo: TComboBox;
    Label2: TLabel;
    Variable1Increment: TEdit;
    Label3: TLabel;
    Variable1Lines: TEdit;
    GroupBox2: TGroupBox;
    Label4: TLabel;
    Label5: TLabel;
    Label6: TLabel;
    Variable2Combo: TComboBox;
    Variable2Increment: TEdit;
    Variable2Lines: TEdit;
    GroupBox3: TGroupBox;
    Label7: TLabel;
    Label8: TLabel;
    Label9: TLabel;
    Variable3Combo: TComboBox;
    Variable3Increment: TEdit;
    Variable3Lines: TEdit;
    procedure Variable1ComboChange(Sender: TObject);
    procedure Variable2ComboChange(Sender: TObject);
    procedure Variable3ComboChange(Sender: TObject);
    constructor Create( AOwner: TComponent ); override;
  private
    procedure DisableSet2();
    procedure DisableSet3();
    procedure KeyPressForIncrement( Sender: TObject; var Key: Char );
    procedure KeyPressForLines( Sender: TObject; var Key: Char );
  public
    procedure Reset();
    procedure RetreiveInfo( var InfoArray: array of NewMtgRowInfo );
  end;

var
  MortgageRowGenerationDlg: TMortgageRowGenerationDlg;

implementation

{$R *.dfm}

constructor TMortgageRowGenerationDlg.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  Variable1Increment.OnKeyPress := KeyPressForIncrement;
  Variable2Increment.OnKeyPress := KeyPressForIncrement;
  Variable3Increment.OnKeyPress := KeyPressForIncrement;
  Variable1Lines.OnKeyPress := KeyPressForLines;
  Variable2Lines.OnKeyPress := KeyPressForLines;
  Variable3Lines.OnKeyPress := KeyPressForLines;
end;

procedure TMortgageRowGenerationDlg.Reset();
begin
  Variable1Combo.ItemIndex := 0;
  Variable1Increment.Text := '';
  Variable1Lines.Text := '';
  Variable2Combo.ItemIndex := 0;
  Variable2Increment.Text := '';
  Variable2Lines.Text := '';
  Variable3Combo.ItemIndex := 0;
  Variable3Increment.Text := '';
  Variable3Lines.Text := '';

  Variable1Increment.Enabled := false;
  Variable1Lines.Enabled := false;
  DisableSet2();
  DisableSet3();
end;

{ Key validator for the increment amount entry lines }
procedure TMortgageRowGenerationDlg.KeyPressForIncrement( Sender: TObject; var Key: Char );
begin
  { strip out letters and such.  Only things that should get through:
    letters 0-9
    comma
    period
    backspace (#8)
    Enter (#13)
    forward slash
    space bar
    }
  if( not (((Key>='0') and (Key<='9')) or (Key=',') or (Key='.') or
            (Key=#8) or (Key=#13) or (Key='/') or (Key=' ')) ) then begin
    Key := #0;
  end;
end;

{ Key validator for the line count entry lines }
procedure TMortgageRowGenerationDlg.KeyPressForLines( Sender: TObject; var Key: Char );
begin
  { strip out letters and such.  Only things that should get through:
    letters 0-9
    comma
    backspace (#8)
    Enter (#13)
    }
  if( not (((Key>='0') and (Key<='9')) or (Key=',') or (Key=#8) or (Key=#13)) ) then begin
    Key := #0;
  end;
end;

{ sort of like an accessor to see what the user selected.  The incoming InfoArray
  must be of size 3. }
procedure TMortgageRowGenerationDlg.RetreiveInfo( var InfoArray: array of NewMtgRowInfo );
var
  ErrorFlag1: boolean;
  ErrorFlag2: boolean;
begin
  if( High(InfoArray) < 2 ) then begin
    MasterLog.Write( LVL_MED, 'RetreiveInfo: InfoArray is not large enough' );
    exit;
  end;
  InfoArray[0].Column := -1;
  InfoArray[1].Column := -1;
  InfoArray[2].Column := -1;
  if( Variable1Combo.Items[Variable1Combo.ItemIndex] <> '{Not Used}' ) then begin
    InfoArray[0].Column := Variable1Combo.ItemIndex;
    InfoArray[0].Amount := StringFormat2Double( Variable1Increment.Text, ErrorFlag1 );
    InfoArray[0].Repetition := StringFormat2Int( Variable1Lines.Text, ErrorFlag2 );
    if( ErrorFlag1 or ErrorFlag2 ) then
      InfoArray[0].Column := -1
    else if( (Variable2Combo.Items[Variable2Combo.ItemIndex] <> '{Not Used}') and Variable2Lines.Enabled ) then begin
      InfoArray[1].Column := Variable2Combo.ItemIndex;
      InfoArray[1].Amount := StringFormat2Double( Variable2Increment.Text, ErrorFlag1 );
      InfoArray[1].Repetition := StringFormat2Int( Variable2Lines.Text, ErrorFlag2 );
      if( ErrorFlag1 or ErrorFlag2 ) then
        InfoArray[1].Column := -1
      else if( (Variable3Combo.Items[Variable3Combo.ItemIndex] <> '{Not Used}') and Variable3Lines.Enabled ) then begin
        InfoArray[2].Column := Variable3Combo.ItemIndex;
        InfoArray[2].Amount := StringFormat2Double( Variable3Increment.Text, ErrorFlag1 );
        InfoArray[2].Repetition := StringFormat2Int( Variable3Lines.Text, ErrorFlag2 );
        if( ErrorFlag1 or ErrorFlag2 ) then
          InfoArray[2].Column := -1;
      end;
    end;
  end;
end;

procedure TMortgageRowGenerationDlg.DisableSet2();
begin
  Variable2Combo.Enabled := false;
  Variable2Increment.Enabled := false;
  Variable2Lines.Enabled := false;
end;

procedure TMortgageRowGenerationDlg.DisableSet3();
begin
  Variable3Combo.Enabled := false;
  Variable3Increment.Enabled := false;
  Variable3Lines.Enabled := false;
end;

procedure TMortgageRowGenerationDlg.Variable1ComboChange(Sender: TObject);
begin
  if( Variable1Combo.Text = '{Not Used}' ) then begin
    Variable1Increment.Enabled := false;
    Variable1Lines.Enabled := false;
    DisableSet2();
    DisableSet3();
  end else begin
    Variable1Increment.Enabled := true;
    Variable1Lines.Enabled := true;
    Variable2Combo.Enabled := true;
  end;
end;

procedure TMortgageRowGenerationDlg.Variable2ComboChange(Sender: TObject);
begin
  if( Variable2Combo.Text = '{Not Used}' ) then begin
    Variable2Increment.Enabled := false;
    Variable2Lines.Enabled := false;
    DisableSet3();
  end else begin
    Variable2Increment.Enabled := true;
    Variable2Lines.Enabled := true;
    Variable3Combo.Enabled := true;
  end;
end;

procedure TMortgageRowGenerationDlg.Variable3ComboChange(Sender: TObject);
begin
  if( Variable3Combo.Text = '{Not Used}' ) then begin
    Variable3Increment.Enabled := false;
    Variable3Lines.Enabled := false;
  end else begin
    Variable3Increment.Enabled := true;
    Variable3Lines.Enabled := true;
  end
end;

end.
