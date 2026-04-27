unit about;

interface

uses Windows, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls;

type
  TAboutBox = class(TForm)
    Panel1: TPanel;
    OKButton: TButton;
    ProgramIcon: TImage;
    Version: TLabel;
    Copyright: TLabel;
    RegistrationInfo: TLabel;
    Label1: TLabel;
    RegistrationButton: TButton;
  private
    { Private declarations }
  public
    procedure SetRegistered();
    procedure SetNotRegistered();
  end;

var
  AboutBox: TAboutBox;

implementation

{$R *.dfm}
procedure TAboutBox.SetRegistered();
begin
  RegistrationInfo.Visible := false;
  RegistrationButton.Visible := false;
  Height := 273;
  Panel1.Height := 185;
  OKButton.Top := 200;
end;

procedure TAboutBox.SetNotRegistered();
begin
  RegistrationInfo.Visible := true;
  RegistrationButton.Visible := true;
  Height := 341;
  Panel1.Height := 257;
  OKButton.Top := 274;
end;

end.
 
