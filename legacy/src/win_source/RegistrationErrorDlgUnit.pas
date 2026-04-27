unit RegistrationErrorDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls, 
  Buttons, ExtCtrls;

type
  TRegistrationErrorDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    Label1: TLabel;
    Label2: TLabel;
    procedure OKBtnClick(Sender: TObject);
  private
    { Private declarations }
  public
    { Public declarations }
  end;

var
  RegistrationErrorDlg: TRegistrationErrorDlg;

implementation

{$R *.dfm}

uses RegistrationDlgUnit, Main;

procedure TRegistrationErrorDlg.OKBtnClick(Sender: TObject);
begin
  MainForm.ShowRegistration();
end;

end.
