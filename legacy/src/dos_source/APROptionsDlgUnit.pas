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
    PointsEdit: TEdit;
    CostsEdit: TEdit;
    PaymentEdit: TEdit;
  private
  public
    procedure Init( Points, Costs, Payments: real );
    procedure GetResults( var Points: real; var Costs: real; var Payments: real );
  end;

var
  APROptionsDlg: TAPROptionsDlg;

implementation

uses peData, Globals;

{$R *.dfm}
procedure TAPROptionsDlg.Init( Points, Costs, Payments: real );
begin
  PointsEdit.Text := ftoa4( Points, 10 );
  CostsEdit.Text := ftoa2( Costs, 10, df.h.commas );
  PaymentEdit.Text := ftoa2( Payments, 10, df.h.commas );
end;

procedure TAPROptionsDlg.GetResults( var Points: real; var Costs: real; var Payments: real );
var
  IsError: boolean;
begin
  Points := StringFormat2Double( PointsEdit.Text, IsError );
  Costs := StringFormat2Double( CostsEdit.Text, IsError );
  Payments := StringFormat2Double( PaymentEdit.Text, IsError );
end;

end.
