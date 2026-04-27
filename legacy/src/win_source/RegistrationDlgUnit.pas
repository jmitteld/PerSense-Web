unit RegistrationDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls;

type
  TRegistrationDialog = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    Edit1: TEdit;
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    procedure Edit1KeyUp(Sender: TObject; var Key: Word; Shift: TShiftState);
  private
    m_IsRegistration: boolean;
  public
    procedure SetForName();
    procedure SetForRegistrationKey( RegistrationCode: string );
    function GetText(): string;
  end;

var
  RegistrationDialog: TRegistrationDialog;

implementation

{$R *.dfm}

function TRegistrationDialog.GetText(): string;
begin
  GetText := Edit1.Text;
end;

procedure TRegistrationDialog.SetForName();
begin
  Label1.Width := 200;
  Label2.Width := 200;
  Label1.Caption := 'Welcome to Per%Sense';
  Label2.Caption := 'Please fill in your name below:';
  Label3.Caption := '';
  OkBtn.Enabled := false;
  m_IsRegistration := false;
end;

procedure TRegistrationDialog.SetForRegistrationKey( RegistrationCode: string );
begin
  Label1.Width := 200;
  Label2.Width := 200;
  Label3.Width := 200;
  Label1.Caption := 'To Register Per%Sense please go to ';
  Label1.Caption := Label1.Caption + 'http://www.persense.org';
  Label2.Caption := 'and register with the following code: ';
  Label2.Caption := Label2.Caption + RegistrationCode;
  Label3.Caption := 'Fill in the Key it gives you below';
  m_IsRegistration := true;
end;

procedure TRegistrationDialog.Edit1KeyUp(Sender: TObject; var Key: Word; Shift: TShiftState);
var
  UsableCount: integer;
  i: integer;
begin
  if( Length( Edit1.Text ) = 0 ) then exit;
  if( m_IsRegistration ) then exit;
  UsableCount := 0;
  for i:=0 to length( Edit1.Text ) do begin
    if( ((Edit1.Text[i] >= 'a') and (Edit1.Text[i] <= 'z')) or
        ((Edit1.Text[i] >= 'A') and (Edit1.Text[i] <= 'Z')) ) then
      UsableCount := UsableCount+1;
  end;
  if( UsableCount > 5 ) then
    OkBtn.Enabled := true
  else
    OkBtn.Enabled := false;
end;

end.

