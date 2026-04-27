unit WelcomeScreenUnit;

interface

uses
  Windows, Messages, SysUtils, Variants, Classes, Graphics, Controls, Forms,
  Dialogs, ChildWin, StdCtrls, FileIOUnit;

type
  TWelcomeScreen = class(TMDIChild)
    Label1: TLabel;
    GroupBox1: TGroupBox;
    MortgageButton: TButton;
    AmortizationButton: TButton;
    PresentValueButton: TButton;
    HelpButton: TButton;
    ExamplesButton: TButton;
    Label3: TLabel;
    Label4: TLabel;
    Label5: TLabel;
    Label7: TLabel;
    GroupBox2: TGroupBox;
    Label2: TLabel;
    procedure FormActivate(Sender: TObject);
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
  private
    { Private declarations }
  public
    constructor Create( AOwner:TComponent ); override;
    function GetType(): TScreenType; override;
  end;

var
  WelcomeScreen: TWelcomeScreen;

implementation

uses Globals, MAIN;

{$R *.dfm}

constructor TWelcomeScreen.Create( AOwner:TComponent );
begin
  inherited Create( AOwner );
  MortgageButton.Caption := 'Mortgage';
  AmortizationButton.Caption := 'Amortization';
  PresentValueButton.Caption := 'Present Value';
  ExamplesButton.Caption := 'Examples';
  PresentValueButton.Caption := 'Present Value';
  HelpButton.Caption := 'Help';
end;

function TWelcomeScreen.GetType(): TScreenType;
begin
  GetType := WelcomeType;
end;

procedure TWelcomeScreen.FormActivate(Sender: TObject);
begin
  // makes the form actually set to the size specified by the Object Inspector
  Width := 396;
  Height := 424;
  Left := trunc(MainForm.Width/2)-210;
  Top := trunc(MainForm.Height/2)-250;
  MainForm.ChildActivating( Self );
end;

procedure TWelcomeScreen.FormClose(Sender: TObject;
  var Action: TCloseAction);
begin
  OnFormClose( Action );
end;

end.
