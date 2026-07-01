{ ===========================================================================
  Unit:  MessageDialogUnit
  Role:  Generic, reusable application message box with an expandable
         "Details" pane.

  This is Per%Sense's custom replacement for the stock MessageBox: it shows a
  short message (MessageText) and optional OK/Yes/No/Cancel buttons, plus a
  Details >> toggle that expands the form to reveal a memo (HelpMemo) holding
  a longer explanation. It is driven through the single ShowMessage entry
  point and is normally invoked from thin wrapper functions elsewhere.

  Button-set selection and the boolean return convention are described in the
  comment above ShowMessage's implementation.

  { Go port: n/a -- generic message-box/confirm UI; superseded by browser
    dialogs and inline status/advisory rendering in
    cmd/persense/static/index.html. The engine reports conditions via advisory
    records (e.g. internal/finance/*/advisories.go), not modal prompts. }
  ===========================================================================}
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
    { Toggles the expandable Details pane (grows/shrinks the form). }
    procedure DetailsBtnClick(Sender: TObject);
  private
    { the short/primary message shown in MessageText }
    m_HelpString: string;
    { the long detail text shown in HelpMemo }
    m_TextMessage: string;
  public
    { Construct collapsed (Details hidden, small height). }
    constructor Create( AOwner: TComponent ); override;
    { Configure, show modally, and report the user's choice (see impl note). }
    procedure ShowMessage( HelpString: string; TextMessage: string; var bUseYesNo: boolean; var bShowCancel: boolean );
  end;

var
  MessageDialog: TMessageDialog;

implementation

{$R *.dfm}

const
  { form height with the Details pane collapsed }
  SMALL_HEIGHT = 160;
  { form height with the Details pane expanded }
  LARGE_HEIGHT = 350;

{ Create
  Purpose: build the dialog in its collapsed state.
  Side effects: hides HelpMemo and sets the small height. }
constructor TMessageDialog.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  HelpMemo.Visible := false;
  Height := SMALL_HEIGHT;
end;

{ DetailsBtnClick
  Purpose: expand/collapse the detail memo when the Details button is clicked.
  Param:   Sender - the button (unused).
  Side effects: toggles HelpMemo.Visible, swaps the form Height between
                SMALL_HEIGHT and LARGE_HEIGHT, and flips the button caption
                between 'Details >>' and '<< Details'. }
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

{ ShowMessage
  Purpose: configure the buttons/layout, display the dialog modally, and
           translate the modal result into the in/out flags.
  Params:
    HelpString  - short message shown prominently (MessageText caption).
    TextMessage - long detail text loaded into HelpMemo; '' disables Details.
    bUseYesNo   - in: request Yes/No buttons instead of OK; out: see below.
    bShowCancel - in: also show a Cancel button; out: see below.
  Return convention (encoded in the two var flags on exit):
    OK / Yes    -> bUseYesNo=true,  bShowCancel=false
    No          -> bUseYesNo=false, bShowCancel=false
    Cancel      -> bUseYesNo=false, bShowCancel=true
  Side effects: sets control captions/visibility/positions, then ShowModal.
  See the original author's note below for the rationale. }
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
