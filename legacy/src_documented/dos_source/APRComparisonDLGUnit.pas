{ ===========================================================================
  Unit:  APRComparisonDLGUnit
  Role:  Read-only modal dialog that displays the result of comparing two
         loan scenarios by their effective APR.

  The mortgage screen lets the user pick two rows and compare them. The APR
  engine computes an effective APR for each scenario; this dialog presents the
  two APR figures (APR1Output, APR2Output) and a human-readable verdict
  (Comparison) explaining which option is cheaper. It has no inputs of its own
  and only an OK button to dismiss.

  The caller populates the three labels via the SetAPR1String / SetAPR2String /
  SetResultString helpers before invoking ShowModal.
  ===========================================================================}
unit APRComparisonDLGUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
     Buttons, ExtCtrls, Globals, LogUnit;

type
  TAPRComparisonDLG = class(TForm)
    OKBtn: TButton;
    Bevel1: TBevel;
    { displays the first scenario's effective APR }
    APR1Output: TLabel;
    { displays the second scenario's effective APR }
    APR2Output: TLabel;
    { displays the comparison verdict text }
    Comparison: TLabel;
  public
    { Set the caption showing scenario 1's APR (preformatted by the caller). }
    procedure SetAPR1String( APR1: string );
    { Set the caption showing scenario 2's APR (preformatted by the caller). }
    procedure SetAPR2String( APR2: string );
    { Set the comparison verdict caption and re-fit its width to the bevel. }
    procedure SetResultString( Result: string );
  end;

var
  APRComparisonDLG: TAPRComparisonDLG;

implementation

{$R *.dfm}

{ SetAPR1String
  Purpose: assign the first scenario's APR text into the APR1Output label.
  Param:   APR1 - preformatted APR string (e.g. "8.250%").
  Side effects: updates APR1Output.Caption. }
{ Go port: n/a -- DOS text UI; the APR figures come from internal/finance/mortgage/mortgage.go: CompareAPRs (line 505) and are rendered by internal/api/handlers.go: HandleMortgageCompare (line 625) + cmd/persense/static/index.html. }
procedure TAPRComparisonDLG.SetAPR1String( APR1: string );
begin
  APR1Output.Caption := APR1;
end;

{ SetAPR2String
  Purpose: assign the second scenario's APR text into the APR2Output label.
  Param:   APR2 - preformatted APR string.
  Side effects: updates APR2Output.Caption. }
{ Go port: n/a -- DOS text UI; superseded by internal/api/handlers.go: HandleMortgageCompare (line 625) + cmd/persense/static/index.html. }
procedure TAPRComparisonDLG.SetAPR2String( APR2: string );
begin
  APR2Output.Caption := APR2;
end;

{ SetResultString
  Purpose: assign the comparison verdict text and constrain the label width so
           the (potentially long) message wraps within the dialog's bevel.
  Param:   Result - verdict sentence describing which option is better.
  Side effects: updates Comparison.Caption and Comparison.Width (bevel width
                less a 30px margin). }
{ Go port: n/a -- DOS text UI; the verdict string is built from internal/finance/mortgage/mortgage.go: CompareAPRs (line 505) output in internal/api/handlers.go: HandleMortgageCompare (line 625) + cmd/persense/static/index.html. }
procedure TAPRComparisonDLG.SetResultString( Result: string );
begin
  Comparison.Caption := Result;
  Comparison.Width := Bevel1.Width - 30;
end;

end.
