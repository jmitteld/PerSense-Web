unit MessageDialogUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls;

type
  TMessageDialog = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    DetailsBtn: TButton;
    HelpMemo: TMemo;
    MessageText: TLabel;
    NoBtn: TButton;
    procedure DetailsBtnClick(Sender: TObject);
  private
    m_HelpString: string;
    m_TextMessage: string;
  public
    constructor Create( AOwner: TComponent ); override;
    procedure ShowMessage( HelpString: string; TextMessage: string; var bUseYesNo: boolean; var bShowCancel: boolean );
  end;

var
  MessageDialog: TMessageDialog;

implementation

{$R *.dfm}

const
  SMALL_HEIGHT = 160;
  LARGE_HEIGHT = 350;

constructor TMessageDialog.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  HelpMemo.Visible := false;
  Height := SMALL_HEIGHT;
end;

procedure TMessageDialog.DetailsBtnClick(Sender: TObject);
begin
  if( HelpMemo.Visible ) then begin
    HelpMemo.Visible := false;
    Height := SMALL_HEIGHT;
    DetailsBtn.Caption := 'Details >>';
  end else begin
    HelpMemo.Visible := true;
    Height := LARGE_HEIGHT;
    DetailsBtn.Caption := '<< Details';
  end;
end;

// sort of an over complicated call, but it's only called from wrapper
// functions so it can be a little zany.  If you specify bUseYesNo
// then you will get yes and no buttons.  You can also turn cancel on.
// when you get a return the yesno will be true for yes or false for now.
// however if the person pressed cancel then it will be false for yesno and
// true for cancel.
procedure TMessageDialog.ShowMessage( HelpString: string; TextMessage: string; var bUseYesNo: boolean; var bShowCancel: boolean );
var
  ShowResult: integer;
begin
  m_HelpString := HelpString;
  m_TextMessage := TextMessage;
  MessageText.Width := 289;
  MessageText.Height := 73;
  MessageText.Caption := HelpString;
  HelpMemo.Lines[0] := m_TextMessage;
  // figure out which buttons to show and their layout
  if( bUseYesNo ) then begin
    OkBtn.Caption := 'Yes';
    NoBtn.Visible := true;
  end else begin
    OkBtn.Caption := 'OK';
    NoBtn.Visible := false;
  end;
  if( bShowCancel ) then begin
    if( bUseYesNo ) then begin
      OkBtn.Left := 7;
      NoBtn.Left := 88;
      CancelBtn.Left := 167;
    end else begin
      OkBtn.Left := 88;
      CancelBtn.Left := 167;
    end;
    CancelBtn.Visible := true;
  end else begin
    if( bUseYesNo ) then begin
      OkBtn.Left := 88;
      NoBtn.Left := 167;
    end else begin
      OkBtn.Left := 167;
    end;
    CancelBtn.Visible := false;
  end;
  // make sure we should enable the Details button
  if( m_TextMessage = '' ) then
    DetailsBtn.Enabled := false
  else
    DetailsBtn.Enabled := true;
  // now show the thing
  ShowResult := showModal();
  // now figure out the return value
  if( ShowResult = mrCancel ) then begin
    bShowCancel := true;
    bUseYesNo := false;
  end else if( ShowResult = mrNo ) then begin
    bUseYesNo := false;
    bShowCancel := false;
  end else begin
    bUseYesNo := true;
    bShowCancel := false;
  end;
end;

end.
