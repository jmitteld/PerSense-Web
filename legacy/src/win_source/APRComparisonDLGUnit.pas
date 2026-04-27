unit APRComparisonDLGUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
     Buttons, ExtCtrls, Globals, LogUnit;

type
  TAPRComparisonDLG = class(TForm)
    OKBtn: TButton;
    Bevel1: TBevel;
    APR1Output: TLabel;
    APR2Output: TLabel;
    Comparison: TLabel;
  public
    procedure SetAPR1String( APR1: string );
    procedure SetAPR2String( APR2: string );
    procedure SetResultString( Result: string );
  end;

var
  APRComparisonDLG: TAPRComparisonDLG;

implementation

{$R *.dfm}

procedure TAPRComparisonDLG.SetAPR1String( APR1: string );
begin
  APR1Output.Caption := APR1;
end;

procedure TAPRComparisonDLG.SetAPR2String( APR2: string );
begin
  APR2Output.Caption := APR2;
end;

procedure TAPRComparisonDLG.SetResultString( Result: string );
begin
  Comparison.Width := Bevel1.Width - 30;
  Comparison.Caption := Result;
end;

end.
