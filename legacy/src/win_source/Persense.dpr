program Persense;

uses
  Forms,
  MAIN in 'MAIN.PAS' {MainForm},
  CHILDWIN in 'CHILDWIN.PAS' {MDIChild},
  about in 'about.pas' {AboutBox},
  MortgageScreenUnit in 'MortgageScreenUnit.pas' {Form1},
  UISettingsDlgUnit in 'UISettingsDlgUnit.pas' {UISettingsDlg},
  SelectAPRDlgUnit in 'SelectAPRDlgUnit.pas' {SelectAPRDlg},
  APRComparisonDLGUnit in 'APRComparisonDLGUnit.pas' {APRComparisonDLG},
  MortgageRowGenerationDlgUnit in 'MortgageRowGenerationDlgUnit.pas' {MortgageRowGenerationDlg},
  ClipboardSettingsDlgUnit in 'ClipboardSettingsDlgUnit.pas' {ClipboardSettingsDlg},
  WelcomeScreenUnit in 'WelcomeScreenUnit.pas' {WelcomeScreen},
  PresentValueScreenUnit in 'PresentValueScreenUnit.pas' {PresentValueScreen},
  ComputationalSettingsDlgUnit in 'ComputationalSettingsDlgUnit.pas' {ComputationalSettingsDLG},
  APRReportScreenUnit in 'APRReportScreenUnit.pas' {APRReportScreen},
  APROptionsDlgUnit in 'APROptionsDlgUnit.pas' {APROptionsDlg},
  TableOptionsDlgUnit in 'TableOptionsDlgUnit.pas' {TableOptionsDlg},
  MessageDialogUnit in 'MessageDialogUnit.pas' {MessageDialog},
  RegistrationDlgUnit in 'RegistrationDlgUnit.pas' {RegistrationDialog},
  RegistrationErrorDlgUnit in 'RegistrationErrorDlgUnit.pas' {RegistrationErrorDlg};

{$R *.RES}

begin
  Application.Initialize;
  Application.Title := 'Persense';
  Application.CreateForm(TMainForm, MainForm);
  Application.CreateForm(TAboutBox, AboutBox);
  Application.CreateForm(TUISettingsDlg, UISettingsDlg);
  Application.CreateForm(TClipboardSettingsDlg, ClipboardSettingsDlg);
  Application.CreateForm(TSelectAPRDlg, SelectAPRDlg);
  Application.CreateForm(TAPRComparisonDLG, APRComparisonDLG);
  Application.CreateForm(TMortgageRowGenerationDlg, MortgageRowGenerationDlg);
  Application.CreateForm(TWelcomeScreen, WelcomeScreen);
  Application.CreateForm(TComputationalSettingsDLG, ComputationalSettingsDLG);
  Application.CreateForm(TAPROptionsDlg, APROptionsDlg);
  Application.CreateForm(TTableOptionsDlg, TableOptionsDlg);
  Application.CreateForm(TMessageDialog, MessageDialog);
  Application.CreateForm(TRegistrationDialog, RegistrationDialog);
  Application.CreateForm(TRegistrationErrorDlg, RegistrationErrorDlg);
  Application.Run;
end.
