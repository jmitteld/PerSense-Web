{ ===========================================================================
  Unit:  APROptionsDlgUnit
  Role:  Modal dialog for entering the extra cost inputs that feed an APR
         calculation/comparison.

  Effective APR depends not only on the note rate but also on up-front loan
  charges. This dialog collects three such inputs:
    - Points   : discount/origination points (percentage of loan amount)
    - Costs     : fixed closing costs / fees (currency)
    - Payments  : the periodic payment amount (currency)
  These are pushed in via Init (formatted for display) and read back via
  GetResults (parsed from the edit fields) by the APR comparison flow.

  Numeric formatting/parsing is delegated to peData helpers (ftoa4/ftoa2 and
  StringFormat2Double); the comma/thousands display setting comes from the
  global df.h.commas config in Globals.
  ===========================================================================}
unit APROptionsDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls, 
  Buttons, ExtCtrls;

type
  TAPROptionsDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    { points input field }
    PointsEdit: TEdit;
    { closing-costs input field }
    CostsEdit: TEdit;
    { periodic-payment input field }
    PaymentEdit: TEdit;
  private
  public
    { Pre-populate the three edit fields from numeric values (formatted). }
    procedure Init( Points, Costs, Payments: real );
    { Parse the three edit fields back into numeric out-params. }
    procedure GetResults( var Points: real; var Costs: real; var Payments: real );
  end;

var
  APROptionsDlg: TAPROptionsDlg;

implementation

uses peData, Globals;

{$R *.dfm}

{ Init
  Purpose: load the dialog's edit fields with formatted starting values.
  Params:  Points  - discount/origination points (shown with 4 decimals).
           Costs    - up-front costs (2 decimals, commas per df.h.commas).
           Payments - periodic payment (2 decimals, commas per df.h.commas).
  Side effects: sets the .Text of all three edit controls.
  Triggered: by the caller just before ShowModal. }
{ Go port: n/a -- DOS text UI; these APR inputs (points/costs/payment) arrive as JSON to internal/api/handlers.go: HandleMortgageCompare (line 625), which builds MtgLines via mtgLineFromInput (line 581); web form in cmd/persense/static/index.html. }
procedure TAPROptionsDlg.Init( Points, Costs, Payments: real );
begin
  PointsEdit.Text := ftoa4( Points, 10 );
  CostsEdit.Text := ftoa2( Costs, 10, df.h.commas );
  PaymentEdit.Text := ftoa2( Payments, 10, df.h.commas );
end;

{ GetResults
  Purpose: read the three edit fields back into numeric out-parameters.
  Params (all var/out): Points, Costs, Payments - parsed numeric values.
  Side effects: none on the form.
  NOTE: the IsError flag from StringFormat2Double is ignored here; malformed
        text yields whatever the parser returns (typically 0) with no warning.
  Triggered: by the caller after ShowModal returns mrOK. }
{ Go port: n/a -- DOS text UI; the parse-back-into-numbers step is JSON decoding in internal/api/handlers.go: HandleMortgageCompare (line 625) + mtgLineFromInput (line 581). }
procedure TAPROptionsDlg.GetResults( var Points: real; var Costs: real; var Payments: real );
var
  IsError: boolean;
begin
  Points := StringFormat2Double( PointsEdit.Text, IsError );
  Costs := StringFormat2Double( CostsEdit.Text, IsError );
  Payments := StringFormat2Double( PaymentEdit.Text, IsError );
end;

end.
