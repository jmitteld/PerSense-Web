unit APRReportScreenUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, StdCtrls, ExtCtrls, Childwin, FileIOUnit;

type
  TAPRReportScreen = class(TMDIChild)
    Panel1: TPanel;
    Panel2: TPanel;
    Panel3: TPanel;
    Panel4: TPanel;
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    Label4: TLabel;
    Label5: TLabel;
    Label6: TLabel;
    Label7: TLabel;
    Label8: TLabel;
    Label9: TLabel;
    Label10: TLabel;
    Label11: TLabel;
    PaymentCountLabel: TLabel;
    PaymentAmountLabel: TLabel;
    PaymentDateLabel: TLabel;
    APRLabel: TLabel;
    FinanceLabel: TLabel;
    AmountLabel: TLabel;
    TotalLabel: TLabel;
    ExtraLabel1: TLabel;
    ExtraLabel2: TLabel;
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
    procedure FormActivate(Sender: TObject);
  protected
    ccosts :real;
    pcosts :real;
    savpayamt: real;
    local_d: real;
    procedure ModifyLoanData();
    procedure RestoreLoanData();
  public
    constructor Create( AOwner: TComponent ); override;
    function GetType(): TScreenType; override;
    procedure OnPrint(); override;
    function CreateReport(): boolean;
  end;

var
  APRReportScreen: TAPRReportScreen;

implementation

uses amortize, amortop, peData, peTypes, Globals, videodat, intsutil,
     APROptionsDlgUnit, Printers, HelpSystemUnit;

{$R *.dfm}
constructor TAPRReportScreen.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  ccosts := 0;
  pcosts := 0;
end;

function TAPRReportScreen.GetType(): TScreenType;
begin
  GetType := APRReportType;
end;

procedure TAPRReportScreen.OnPrint();
var
  TextHeight: integer;
  UsablePageWidth: integer;
  yPos: integer;
  FontSize: integer;
const
  Margin=80;
begin
  Printer.BeginDoc();
  Printer.Canvas.Font.Assign( Font );
  Printer.Canvas.Brush.Color := clWhite;
  Printer.Title := 'Persense APR Report Screen';
  TextHeight := Printer.Canvas.TextHeight( 'Amount' );
  UsablePageWidth := Printer.PageWidth-(2*Margin);
  yPos := 100;
  FontSize := Printer.Canvas.Font.Size;
  // titles across the top
  Printer.Canvas.Font.Size := 12;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.05), yPos, '  Annual' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.3), yPos, 'Finance' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.55), yPos, ' Amount' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.8), yPos, ' Total Of' );
  yPos := yPos + Printer.Canvas.TextHeight( 'Height' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.05), yPos, 'Percentage' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.3), yPos, 'Charge' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.55), yPos, 'Financed' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.8), yPos, 'Payments' );
  yPos := yPos + Printer.Canvas.TextHeight( 'Height' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.05), yPos, '   Rate' );
  Printer.Canvas.Font.Size := FontSize;
  yPos := yPos + 2*TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.03), yPos, 'The cost of your credit as' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.28), yPos, 'The dollar amount the credit' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.53), yPos, 'The amount of credit provided' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.78), yPos, 'The amount you will have paid' );
  yPos := yPos + TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.03), yPos, 'a yearly rate' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.28), yPos, 'will cost you' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.53), yPos, 'to you or on your behalf' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.78), yPos, 'after you have made all' );
  yPos := yPos + TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.78), yPos, 'payments as scheduled' );
  yPos := yPos + 2*TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.03), yPos, APRLabel.Caption );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.28), yPos, FinanceLabel.Caption );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.53), yPos, AmountLabel.Caption );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.78), yPos, TotalLabel.Caption );
  yPos := yPos + 2*TextHeight;
  // the boxes that surround
  Printer.Canvas.Pen.Width := 6;
  Printer.Canvas.MoveTo( Margin, Margin );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, Margin );
  Printer.Canvas.LineTo( Printer.PageWidth-Margin, yPos );
  Printer.Canvas.LineTo( Margin, yPos );
  Printer.Canvas.LineTo( Margin, Margin );
  Printer.Canvas.MoveTo( Trunc( UsablePageWidth * 0.25), Margin );
  Printer.Canvas.LineTo( Trunc( UsablePageWidth * 0.25), yPos );
  Printer.Canvas.MoveTo( Trunc( UsablePageWidth * 0.5), Margin );
  Printer.Canvas.LineTo( Trunc( UsablePageWidth * 0.5), yPos );
  Printer.Canvas.MoveTo( Trunc( UsablePageWidth * 0.75), Margin );
  Printer.Canvas.LineTo( Trunc( UsablePageWidth * 0.75), yPos );
  yPos := yPos + TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.1), yPos, 'Number of regular payments: ' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.35), yPos, PaymentCountLabel.Caption );
  yPos := yPos + TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.1), yPos, 'Amount of regular payments: ' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.35), yPos, PaymentAmountLabel.Caption );
  yPos := yPos + TextHeight;
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.1), yPos, 'When payments are due: ' );
  Printer.Canvas.TextOut( Trunc( UsablePageWidth * 0.35), yPos, PaymentDateLabel.Caption );
  Printer.EndDoc();
end;

//
// The following functions where lifted from APReport.pas
// They seem pretty simple, so I'm figuring no need to include the
// original source.
procedure TAPRReportScreen.ModifyLoanData();
var
  save_balloon :saved_balloon_state;
begin
  h^.points:=h^.points+ccosts/h^.amount;
  h^.pointsstatus:=inp;
  savpayamt:=h^.payamt;
  h^.payamt:=h^.payamt+pcosts;
  local_d := h^.payamt;
  if ((screenstatus and needs_calc)>0) then AMORTIZE.Enter(0);
  if (fancy) then begin
    v_rate:=0; aprvalue:=0;
    save_balloon.Save;
    RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, value_calc, 0);
    cumint:=aprvalue-h^.amount*(1-h^.points);
    save_balloon.Restore;
  end else
    cumint:=local_d*h^.nperiods-h^.amount*(1-h^.points);
end;

procedure TAPRReportScreen.RestoreLoanData();
begin
  h^.points:=h^.points-ccosts/h^.amount;
  h^.payamt:=savpayamt;
  local_d:=h^.payamt;
end;

function TAPRReportScreen.CreateReport(): boolean;
var
  Hold: string;
begin
  CreateReport := false;
  if (h^.amount<=tiny) then begin
    MessageBox('Loan amount must be positive to compute APR.', DO_LoanAmountMustBePositive);
    exit;
  end;
  Enter( no_tab );
  if( errorflag ) then exit;
  if( not SufficientDataOnScreen() ) then begin
    InsufficientDataMessage( 'APR', DO_InsufficentDataForAPR );
    exit;
  end;
  if (h^.pointsstatus<defp) then begin
    h^.pointsstatus:=defp;
    h^.points:=0;
  end;
  APROptionsDlg.Init( h^.points*100, ccosts, pcosts );
  APROptionsDlg.ShowModal();
  if( APROptionsDlg.ModalResult <> mrOK ) then exit;
  APROptionsDlg.GetResults( h^.points, ccosts, pcosts );
  ModifyLoanData;
  if (not EstimateAndRefineAPRWithPoints) then begin
    MessageBox('APR computation did not converge.', DO_APRDidNotConverge);
    RestoreLoanData;
    exit;
  end;
  aprpmtsum:=cumint+h^.amount*(1-h^.points);
  // output time
  APRLabel.caption := ftoa4(h^.apr*100,7);
  FinanceLabel.caption := ftoa2(cumint,10,df.h.commas);
  AmountLabel.caption := ftoa2(h^.amount*(1-h^.points),10,df.h.commas);
  TotalLabel.caption := ftoa2(aprpmtsum,10,df.h.commas);

  PaymentCountLabel.caption := strb(h^.nperiods,0);
  Hold := ftoa2(h^.payamt,10,df.h.commas);
  StripSpaces( Hold );
  PaymentAmountLabel.caption := Hold;
  PaymentDateLabel.caption := PerYrString(h^.peryr,0)+' beginning '+Datestr(h^.firstdate);

  ExtraLabel1.Caption := '';
  ExtraLabel2.Caption := '';
  if (fancy) then begin
    if (mor^.first_repaystatus>empty) or (targ^.targetstatus>empty) or (nlines[AMZChangesBlock]>0) then
      ExtraLabel1.Caption := 'Payment amounts described in attached schedule.';
    if (nballoons>0) or (npre>0) then
      ExtraLabel2.Caption := 'Extra payments as described in attached schedule.';
  end;
  RestoreLoanData();
  CreateReport := true;
end;

procedure TAPRReportScreen.FormClose(Sender: TObject;
  var Action: TCloseAction);
begin
  inherited;
  OnFormClose( Action );
end;

procedure TAPRReportScreen.FormActivate(Sender: TObject);
begin
  inherited;
  Width := 522;
  Height := 320;
end;

end.
