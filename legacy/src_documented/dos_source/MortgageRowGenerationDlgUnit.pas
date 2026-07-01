{ ===========================================================================
  Unit:  MortgageRowGenerationDlgUnit
  Role:  Dialog for bulk-generating mortgage comparison rows by varying up to
         three columns in arithmetic steps.

  Rather than typing many similar mortgage scenarios by hand, the user defines
  up to three "variables": each picks a grid column, an increment amount, and a
  number of lines. The three variable groups are nested/dependent - variable 2
  is only enabled once variable 1 is in use, and variable 3 once variable 2 is.
  On OK, RetreiveInfo returns the selections as an array of NewMtgRowInfo so the
  Mortgage screen can expand them into actual rows.

  The edit fields are guarded by key-press validators (digits + a few symbols
  only). The special "Not Used" combo entry means that variable is inactive.
  ===========================================================================}
unit MortgageRowGenerationDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls, Globals, LogUnit;

type
  { Describes one variable's row-generation request. }
  NewMtgRowInfo = record
    { target grid column index, or -1 if unused/invalid }
    Column: integer;
    { per-step increment applied to that column }
    Amount: double;
    { number of generated lines for this variable }
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
    { OnChange for variable 1's column combo: enable/disable dependent fields. }
    procedure Variable1ComboChange(Sender: TObject);
    { OnChange for variable 2's column combo. }
    procedure Variable2ComboChange(Sender: TObject);
    { OnChange for variable 3's column combo. }
    procedure Variable3ComboChange(Sender: TObject);
    { Wire up the key-press validators for the edit fields. }
    constructor Create( AOwner: TComponent ); override;
  private
    { Disable variable-2's controls (column/increment/lines). }
    procedure DisableSet2();
    { Disable variable-3's controls. }
    procedure DisableSet3();
    { Key filter for increment fields (digits + , . / space). }
    procedure KeyPressForIncrement( Sender: TObject; var Key: Char );
    { Key filter for line-count fields (digits + comma). }
    procedure KeyPressForLines( Sender: TObject; var Key: Char );
  public
    { Clear all fields and disable the dependent variable groups. }
    procedure Reset();
    { Read the user's three variable definitions into InfoArray (size >= 3). }
    procedure RetreiveInfo( var InfoArray: array of NewMtgRowInfo );
  end;

var
  MortgageRowGenerationDlg: TMortgageRowGenerationDlg;

implementation

{$R *.dfm}

{ Create
  Purpose: construct the dialog and attach the numeric key-press validators to
           all six edit fields (increments and line counts).
  Side effects: assigns OnKeyPress handlers. }
{ Go port: n/a -- DOS/VCL dialog construction; superseded by web frontend cmd/persense/static/index.html + internal/api/handlers.go: HandleMortgageWhatIf (line 696). }
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

{ Reset
  Purpose: return the dialog to its initial empty state before each use.
  Side effects: selects the first ("Not Used") combo item and blanks all edit
                fields; disables variable 1's fields and all of sets 2 and 3.
  Triggered: by the caller before ShowModal. }
{ Go port: n/a -- DOS text UI; superseded by web frontend cmd/persense/static/index.html + internal/api/handlers.go. }
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

{ Go port: n/a -- keystroke input filter; browser <input> validation in cmd/persense/static/index.html. }
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

{ Go port: n/a -- keystroke input filter; browser <input> validation in cmd/persense/static/index.html. }
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

{ RetreiveInfo
  Purpose: harvest the three variable definitions into the caller's array.
           Acts as an accessor for what the user selected; the incoming
           InfoArray must hold at least 3 elements.
  Param (out): InfoArray - filled with up to three NewMtgRowInfo entries; an
               entry's Column is set to -1 when the variable is unused or its
               numeric fields fail to parse.
  Side effects: logs and aborts if InfoArray is too small.
  NOTE: the variables are evaluated in a nested fashion - variable 2 is only
        considered if variable 1 parsed and is enabled, and variable 3 only if
        variable 2 did, mirroring the on-screen enablement chain. }
{ Go port: internal/api/handlers.go: HandleMortgageWhatIf (line 696) -- harvests the (column, increment, repetition) tuples from the request; the nested vary loop is internal/finance/mortgage/rowgen.go: GenerateGrid (line 156) via bumpField (line 111). }
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

{ Go port: n/a -- DOS/VCL control enablement; superseded by web frontend cmd/persense/static/index.html. }
{ DisableSet2 - greys out variable 2's column/increment/lines controls. }
procedure TMortgageRowGenerationDlg.DisableSet2();
begin
  Variable2Combo.Enabled := false;
  Variable2Increment.Enabled := false;
  Variable2Lines.Enabled := false;
end;

{ Go port: n/a -- DOS/VCL control enablement; superseded by web frontend cmd/persense/static/index.html. }
{ DisableSet3 - greys out variable 3's column/increment/lines controls. }
procedure TMortgageRowGenerationDlg.DisableSet3();
begin
  Variable3Combo.Enabled := false;
  Variable3Increment.Enabled := false;
  Variable3Lines.Enabled := false;
end;

{ Variable1ComboChange
  Purpose: enable/disable downstream controls based on whether variable 1 is in
           use. If "Not Used", disable variable 1's fields and all of sets 2
           and 3; otherwise enable variable 1's fields and unlock set 2's combo.
  Triggered: by the Variable1Combo OnChange event. }
{ Go port: n/a -- DOS/VCL dependent-field enablement; the web frontend cmd/persense/static/index.html handles the nested vary-column UI. }
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

{ Variable2ComboChange
  Purpose: as Variable1ComboChange but for variable 2 - toggles its own fields
           and unlocks/locks set 3 accordingly.
  Triggered: by the Variable2Combo OnChange event. }
{ Go port: n/a -- DOS/VCL dependent-field enablement; superseded by web frontend cmd/persense/static/index.html. }
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

{ Variable3ComboChange
  Purpose: toggle variable 3's increment/lines fields based on its combo (the
           last variable, so it has no further dependent set).
  Triggered: by the Variable3Combo OnChange event. }
{ Go port: n/a -- DOS/VCL dependent-field enablement; superseded by web frontend cmd/persense/static/index.html. }
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
