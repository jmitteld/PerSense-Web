{ ===========================================================================
  Unit:  WelcomeScreenUnit
  Role:  The landing / launcher MDI child window shown when Per%Sense starts.

  Per%Sense is an MDI application (see MAIN / CHILDWIN). The Welcome screen is
  the first MDI child the user sees; it is a simple menu of large buttons that
  open the three calculation modules (Mortgage, Amortization, Present Value),
  plus Examples and Help shortcuts. It derives from TMDIChild (ChildWin) so it
  participates in the standard child-window activation/close bookkeeping.

  GetType reports WelcomeType so MAIN can recognise which kind of child is
  active (e.g. to enable/disable the right menus).

  { Go port: n/a -- MDI launcher/landing child; no financial logic. The three
    module buttons are superseded by the tab/panel navigation in
    cmd/persense/static/index.html, which posts to the corresponding endpoints
    (/api/mortgage/calc, /api/amortization/calc, /api/presentvalue/calc). The
    FormActivate/FormClose/GetType methods are DOS MDI bookkeeping with no web
    equivalent. }
  ===========================================================================}
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
    { OnActivate: re-asserts the form's fixed size/position and notifies MAIN. }
    procedure FormActivate(Sender: TObject);
    { OnClose: routes through the inherited MDIChild close handling. }
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
  private
    { Private declarations }
  public
    { Construct the welcome child and caption its launcher buttons. }
    constructor Create( AOwner:TComponent ); override;
    { Identify this child as the WelcomeType screen (for MAIN's dispatch). }
    function GetType(): TScreenType; override;
  end;

var
  WelcomeScreen: TWelcomeScreen;

implementation

uses Globals, MAIN;

{$R *.dfm}

{ Create
  Purpose: build the welcome child window and set the visible captions on its
           five launcher buttons.
  Param:   AOwner - the owning component (the MDI MainForm).
  Side effects: assigns button captions.
  NOTE: PresentValueButton.Caption is assigned twice (lines are duplicated);
        harmless, just redundant. }
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

{ GetType
  Purpose: report this child's screen kind to MAIN's MDI bookkeeping.
  Returns: WelcomeType. }
function TWelcomeScreen.GetType(): TScreenType;
begin
  GetType := WelcomeType;
end;

{ FormActivate
  Purpose: on activation, force the window to its designed size and centre it
           over the MDI parent, then tell MainForm which child is now active.
  Param:   Sender - the activating control (unused).
  Side effects: sets Width/Height/Left/Top; calls MainForm.ChildActivating.
  NOTE: the explicit size/position is a workaround so the form honours the
        dimensions set in the Object Inspector rather than the MDI default. }
procedure TWelcomeScreen.FormActivate(Sender: TObject);
begin
  // makes the form actually set to the size specified by the Object Inspector
  Width := 396;
  Height := 424;
  Left := trunc(MainForm.Width/2)-210;
  Top := trunc(MainForm.Height/2)-250;
  MainForm.ChildActivating( Self );
end;

{ FormClose
  Purpose: delegate close handling to the inherited TMDIChild logic.
  Params:  Sender - closing form (unused); Action - close action (in/out),
           may be adjusted by OnFormClose.
  Side effects: whatever OnFormClose does (e.g. caFree to release the child). }
procedure TWelcomeScreen.FormClose(Sender: TObject;
  var Action: TCloseAction);
begin
  OnFormClose( Action );
end;

end.
