{ ===========================================================================
  Unit:  ComputationalSettingsDlgUnit
  Role:  Modal dialog for the application-wide computational/financial defaults
         (the compdefaults record) that govern how every screen computes.

  Each control maps to one field of the compdefaults record:
    CenturyDivEdit -> centurydiv   : the Y2K pivot year. Entered 2-digit years
                                     below it are 21st-century, above it 20th.
    PerYrCombo     -> peryr         : compounding periods per year, OR the
                                     special CANADIAN / DAILY codes (these two
                                     affect all screens; numeric values mainly
                                     affect Present Value APR<->Rate).
    COLAMonthBox   -> colamonth     : when COLA increments apply - ANN
                                     (anniversary), CNT (continuous), or a
                                     specific calendar month (1..12).
    USARuleBox     -> USARule       : Actuarial (false) vs USA rule (true) for
                                     handling payments that don't cover interest
    BasisBox       -> basis         : day-count basis x360 / x365 / x365_360.
    PrepaidBox     -> prepaid       : is first partial period's interest prepaid
                                     at settlement (Yes) or levelled in (No).
    InAdvanceBox   -> in_advance    : interest paid in Advance vs Arrears.
    PlusRegularBox -> plus_regular  : whether a balloon on a payment date is
                                     IN ADDITION to the regular payment.
                                     NOTE inverted mapping: 'No' => true.
    ExactBox       -> exact         : exact day calculations (requires 365).
    Rule78Box      -> r78           : Rule-of-78 amortization (Amort screen only)

  Settings flow IN via LoadSettings (record -> controls) and OUT via OKBtnClick
  (controls -> m_Settings) + GetSettings (m_Settings -> caller's record).
  Hovering or focusing any control shows its explanatory text in HelpTextLabel
  via the matching *MouseEnter / *Enter handlers and the *Help string constants.

  { Go port: internal/types/defaults.go:21 (DefaultCompDefaults) supplies the
    compdefaults equivalent (CompDefaults); internal/finance/interest/rates.go:21
    (NewCalcContext) consumes basis/peryr into the calc context; the dialog UI
    itself is superseded by the settings panel in cmd/persense/static/index.html.
    The MouseEnter/Enter help-blurb handlers are n/a -- DOS contextual-help
    layer with no web equivalent (tooltips live in the frontend). }
  ===========================================================================}
unit ComputationalSettingsDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls,
  Buttons, ExtCtrls, peTypes;

const
  { Per-setting help blurbs shown in HelpTextLabel on hover/focus. }
  DivCenturyHelp =  'In entered dates, years below this number are interpreted ' +
    'as being in the 21st century, while years above this are considered to be in the ' +
    '20th century.';
  PerYrHelp = 'In the Present Value screen, this number controls the way APR is ' +
    'related to Rate.  The Canadian and Daily options affect all screens.';
  COLAMonthHelp = 'In the Present Value screen, in which month are payments with ' +
    'COLA incremented?  Anniversary month: taken from first payment date. ' +
    'Continuous: increment is gradual, a little bit with each payment.';
  USARuleHelp = 'Actuarial standard: interest and principal are on an equal ' +
    'footing.  USA = American rule.  If any regular payment doesn''t cover interest ' +
    'for the preceding period, seperate tally is kept of shortfall, and no interest ' +
    'accrues on that.';
  BasisHelp = 'Loans are usually computed on a 360-day basis.  All months are considered ' +
    'to have 30 days, and years are 360 days.  365, used in savings accounts, is close ' +
    'to a true calendar.  365/360 means all 365 days are counted, over a denominator of 360.';
  PrepaidHelp = 'In most loans, interest for the first partial interest period is prepaid ' +
    'at settlement.  If "No" is specified, all payments will be made equal, including ' +
    'the first partial period.';
  InAdvanceHelp = 'In most loans, interest is paid for the prior interest period (arrears), ' +
    'but sometimes a loan is structured with interest payments for the coming interest period (advance).';
  PlusRegularHelp = 'In the Amortization screen (not the Mortgage screen), when balloon ' +
    'payments come due on regular payment dates, does the spceified balloon amount include ' +
    'the regular payment, or is the regular payment added on?';
  ExactHelp = 'if "No" then a standard approximation will be used in Chronological, ' +
    'Present Value, and Amortization screens.  If "Yes" results will be "exact."  365 ' +
    'DAY MUST ALSO BE SELECTED.  Note: Exact results are non-standard.';
  Rule78Help = 'Set this option to "Yes" for Rule of 78 Amortizations.  This is available ' +
    'only in the Amortization Screen, and with Advanced Options turned OFF.  (Rule of ' +
    '78 modifies the way each payment is divided between interest and principal.)';
  { fixed width applied to HelpTextLabel before each blurb }
  HelpLabelWidth = 281;

type
  TComputationalSettingsDLG = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    CenturyDivLabel: TLabel;
    PerYrLabel: TLabel;
    COLAMonthLabel: TLabel;
    USARuleLabel: TLabel;
    CenturyDivEdit: TEdit;
    BasisLabel: TLabel;
    PrepaidLabel: TLabel;
    InAdvanceLabel: TLabel;
    PlusRegularLabel: TLabel;
    ExactLabel: TLabel;
    Rule78Label: TLabel;
    PerYrCombo: TComboBox;
    COLAMonthBox: TComboBox;
    USARuleBox: TComboBox;
    BasisBox: TComboBox;
    PrepaidBox: TComboBox;
    InAdvanceBox: TComboBox;
    PlusRegularBox: TComboBox;
    ExactBox: TComboBox;
    Rule78Box: TComboBox;
    GroupBox1: TGroupBox;
    { shows the contextual help blurb for the focused control }
    HelpTextLabel: TLabel;
    { The following *MouseEnter (label hover) and *Enter (control focus)
      handlers all do the same thing: load the matching *Help constant into
      HelpTextLabel so the user sees an explanation for whatever they point at
      or tab into. Each is named for the setting it documents. }
    procedure CenturyDivLabelMouseEnter(Sender: TObject);
    procedure PerYrLabelMouseEnter(Sender: TObject);
    procedure COLAMonthLabelMouseEnter(Sender: TObject);
    procedure USARuleLabelMouseEnter(Sender: TObject);
    procedure BasisLabelMouseEnter(Sender: TObject);
    procedure PrepaidLabelMouseEnter(Sender: TObject);
    procedure InAdvanceLabelMouseEnter(Sender: TObject);
    procedure PlusRegularLabelMouseEnter(Sender: TObject);
    procedure ExactLabelMouseEnter(Sender: TObject);
    procedure Rule78LabelMouseEnter(Sender: TObject);
    procedure CenturyDivEditEnter(Sender: TObject);
    procedure PerYrComboEnter(Sender: TObject);
    procedure COLAMonthBoxEnter(Sender: TObject);
    procedure USARuleBoxEnter(Sender: TObject);
    procedure BasisBoxEnter(Sender: TObject);
    procedure PrepaidBoxEnter(Sender: TObject);
    procedure InAdvanceBoxEnter(Sender: TObject);
    procedure PlusRegularBoxEnter(Sender: TObject);
    procedure ExactBoxEnter(Sender: TObject);
    procedure Rule78BoxEnter(Sender: TObject);
    { Commit all control selections back into m_Settings. }
    procedure OKBtnClick(Sender: TObject);
  public
    { Populate the controls from an existing settings record. }
    procedure LoadSettings( Settings: compdefaults );
    { Copy the committed settings out to the caller. }
    procedure GetSettings( var Settings: compdefaults );
  protected
    { working copy of the settings being edited }
    m_Settings: compdefaults;
    { Map month number 1..12 to its English name (for COLAMonthBox). }
    function IntToMonthString( Month: integer ): string;
    { Map an English month name back to its number 1..12. }
    function MonthStringToInt( Month: string ): integer;
  end;

var
  ComputationalSettingsDLG: TComputationalSettingsDLG;

implementation

uses PresentValueScreenUnit;

{$R *.dfm}


{ LoadSettings
  Purpose: populate every control from an incoming compdefaults record so the
           dialog reflects the current configuration.
  Param:   Settings - the current defaults to display (also cached in
           m_Settings for OKBtnClick to update).
  Side effects: sets each combo's ItemIndex / edit text; stores m_Settings.
  NOTE: peryr is decoded specially - the CanadianOrDaily bit distinguishes the
        two named modes from a plain numeric periods-per-year; colamonth is
        decoded as ANN/CNT/specific-month.
  Triggered: by the caller before ShowModal. }
{ Go port: internal/types/defaults.go:21 (DefaultCompDefaults) -- record->UI
  hydration; in the web port the frontend settings panel is seeded from the
  CompDefaults JSON rather than combo ItemIndex assignment. }
procedure TComputationalSettingsDlg.LoadSettings( Settings: compdefaults );
var
  Index: integer;
begin
  CenturyDivEdit.Text := IntToStr( Settings.centurydiv );
  if( (Settings.peryr and CanadianOrDaily)>0 ) then begin
    if( Settings.peryr = DAILY ) then
      PerYrCombo.ItemIndex := PerYrCombo.Items.IndexOf( 'Daily' )
    else
      PerYrCombo.ItemIndex := PerYrCombo.Items.IndexOf( 'Canadian' )
  end else
    PerYrCombo.ItemIndex := PerYrCombo.Items.IndexOf( IntToStr( Settings.peryr ) );
  if( Settings.colamonth = ANN ) then
    COLAMonthBox.ItemIndex := COLAMonthBox.Items.IndexOf( 'Anniversary' )
  else if( Settings.colamonth = CNT ) then
    COLAMonthBox.ItemIndex := COLAMonthBox.Items.IndexOf( 'Continuous' )
  else
    COLAMonthBox.ItemIndex := COLAMonthBox.Items.IndexOf( IntToMonthString( Settings.colamonth ) );
  if( Settings.USARule ) then
    USARuleBox.ItemIndex := USARuleBox.Items.IndexOf( 'USA' )
  else
    USARuleBox.ItemIndex := USARuleBox.Items.IndexOf( 'Actuarial' );
  case Settings.basis of
    x360: BasisBox.ItemIndex := BasisBox.Items.IndexOf( '360' );
    x365: BasisBox.ItemIndex := BasisBox.Items.IndexOf( '365' );
    x365_360: BasisBox.ItemIndex := BasisBox.Items.IndexOf( '365/360' );
  end;
  if( Settings.prepaid ) then
    PrepaidBox.ItemIndex := PrepaidBox.Items.IndexOf( 'Yes' )
  else
    PrepaidBox.ItemIndex := PrepaidBox.Items.IndexOf( 'No' );
  if( Settings.in_advance ) then
    InAdvanceBox.ItemIndex := InAdvanceBox.Items.IndexOf( 'Advance' )
  else
    InAdvanceBox.ItemIndex := InAdvanceBox.Items.IndexOf( 'Arrears' );
  if( Settings.plus_regular ) then
    PlusRegularBox.ItemIndex := PlusRegularBox.Items.IndexOf( 'No' )
  else
    PlusRegularBox.ItemIndex := PlusRegularBox.Items.IndexOf( 'Yes' );
  if( Settings.exact ) then
    ExactBox.ItemIndex := ExactBox.Items.IndexOf( 'Yes' )
  else
    ExactBox.ItemIndex := ExactBox.Items.IndexOf( 'No' );
  if( Settings.r78 ) then
    Rule78Box.ItemIndex := Rule78Box.Items.IndexOf( 'Yes' )
  else
    Rule78Box.ItemIndex := Rule78Box.Items.IndexOf( 'No' );
  m_Settings := Settings;
end;

{ OKBtnClick
  Purpose: read every control selection back into m_Settings, encoding the
           combo strings into the compdefaults field values.
  Side effects: updates all m_Settings fields. If peryr changed, every stored
                rate (derived from the old peryr) is now inconsistent, so the
                Present Value screen's FixRates is invoked to recompute them.
  NOTE: plus_regular uses an inverted mapping ('No' => true). Settings are NOT
        applied globally here - the caller pulls them via GetSettings after the
        modal returns mrOK.
  Triggered: by the OK button click. }
{ Go port: internal/types/defaults.go:21 (DefaultCompDefaults fields) +
  internal/finance/interest/rates.go:21 (NewCalcContext consumes basis/peryr) --
  UI->record encoding. The peryr-changed FixRates recompute has no server
  analogue: the Go engine is stateless, so every PV call recomputes rates from
  the supplied peryr on each request. }
procedure TComputationalSettingsDLG.OKBtnClick(Sender: TObject);
var
  NewPeryr: byte;
begin
  m_Settings.centurydiv := strToInt(CenturyDivEdit.Text);
  if( PerYrCombo.Items.Strings[ PerYrCombo.ItemIndex ] = 'Canadian' ) then
    NewPeryr := CANADIAN
  else if( PerYrCombo.Items.Strings[ PerYrCombo.ItemIndex ] = 'Daily' ) then
    NewPeryr := DAILY
  else
    NewPeryr := StrToInt( PerYrCombo.Items.Strings[ PerYrCombo.ItemIndex ] );
  if( NewPeryr <> m_settings.peryr ) then begin
    // if peryr has changed then all the rates that are stored could be wrong
    // as they are calculated out of the default peryr.  Go through
    // and fix them all.
    if( PresentValueScreen <> nil ) then
      PresentValueScreen.FixRates( NewPerYr );
    m_Settings.peryr := NewPeryr;
  end;
  if( COLAMonthBox.Items.Strings[ COLAMonthBox.ItemIndex ] = 'Anniversary' ) then
    m_Settings.colamonth := ANN
  else if( COLAMonthBox.Items.Strings[ COLAMonthBox.ItemIndex ] = 'Continuous' ) then
    m_Settings.colamonth := CNT
  else
    m_Settings.colamonth := MonthStringToInt(COLAMonthBox.Items.Strings[COLAMonthBox.ItemIndex] );
  m_Settings.USARule := (USARuleBox.Items.Strings[ USARuleBox.ItemIndex ] = 'USA');
  if( BasisBox.Items.Strings[ BasisBox.ItemIndex ] = '360' ) then m_Settings.basis := x360
  else if( BasisBox.Items.Strings[ BasisBox.ItemIndex ] = '365' ) then m_Settings.basis := x365
  else if( BasisBox.Items.Strings[ BasisBox.ItemIndex ] = '365/360' ) then m_Settings.basis := x365_360;
  m_Settings.prepaid := (PrepaidBox.Items.Strings[ PrepaidBox.ItemIndex ] = 'Yes');
  m_Settings.in_advance := (InAdvanceBox.Items.Strings[ InAdvanceBox.ItemIndex ] = 'Advance');
  m_Settings.plus_regular := (PlusRegularBox.Items.Strings[ PlusRegularBox.ItemIndex ] = 'No');
  m_Settings.exact := (ExactBox.Items.Strings[ ExactBox.ItemIndex ] = 'Yes');
  m_Settings.r78 := (Rule78Box.Items.Strings[ Rule78Box.ItemIndex ] = 'Yes');
end;

{ IntToMonthString - map a month number (1..12) to its English name. }
function TComputationalSettingsDLG.IntToMonthString( Month: integer ): string;
begin
  case Month of
    1: IntToMonthString := 'January';
    2: IntToMonthString := 'February';
    3: IntToMonthString := 'March';
    4: IntToMonthString := 'April';
    5: IntToMonthString := 'May';
    6: IntToMonthString := 'June';
    7: IntToMonthString := 'July';
    8: IntToMonthString := 'August';
    9: IntToMonthString := 'September';
    10: IntToMonthString := 'October';
    11: IntToMonthString := 'November';
    12: IntToMonthString := 'December';
  end;
end;

{ MonthStringToInt - inverse of IntToMonthString (English name -> 1..12). }
function TComputationalSettingsDLG.MonthStringToInt( Month: string ): integer;
begin
  if( Month = 'January' ) then MonthStringToInt := 1
  else if( Month = 'February' ) then MonthStringToInt := 2
  else if( Month = 'March' ) then MonthStringToInt := 3
  else if( Month = 'April' ) then MonthStringToInt := 4
  else if( Month = 'May' ) then MonthStringToInt := 5
  else if( Month = 'June' ) then MonthStringToInt := 6
  else if( Month = 'July' ) then MonthStringToInt := 7
  else if( Month = 'August' ) then MonthStringToInt := 8
  else if( Month = 'September' ) then MonthStringToInt := 9
  else if( Month = 'October' ) then MonthStringToInt := 10
  else if( Month = 'November' ) then MonthStringToInt := 11
  else if( Month = 'December' ) then MonthStringToInt := 12;
end;

{ GetSettings
  Purpose: hand the caller the committed settings record after OK.
  Param (out): Settings - receives m_Settings.
  Triggered: by the caller after ShowModal returns mrOK. }
{ Go port: internal/types/defaults.go:21 (CompDefaults value hand-off) -- in the
  web port settings are passed per-request in the JSON body, not via a modal's
  out-param. }
procedure TComputationalSettingsDLG.GetSettings( var Settings: compdefaults );
begin
  Settings := m_Settings;
end;

{ ---------------------------------------------------------------------------
  Contextual-help handlers: each *MouseEnter (label hover) and *Enter (control
  focus) below simply sets HelpTextLabel to the corresponding *Help blurb.
  They are intentionally uniform; only the help constant differs per setting.
  --------------------------------------------------------------------------- }
procedure TComputationalSettingsDLG.CenturyDivLabelMouseEnter(
  Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := DivCenturyHelp;
end;

procedure TComputationalSettingsDLG.PerYrLabelMouseEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := PerYrHelp;
end;

procedure TComputationalSettingsDLG.COLAMonthLabelMouseEnter(
  Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := COLAMonthHelp;
end;

procedure TComputationalSettingsDLG.USARuleLabelMouseEnter(
  Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := USARuleHelp;
end;

procedure TComputationalSettingsDLG.BasisLabelMouseEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := BasisHelp;
end;

procedure TComputationalSettingsDLG.PrepaidLabelMouseEnter(
  Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := PrepaidHelp;
end;

procedure TComputationalSettingsDLG.InAdvanceLabelMouseEnter(
  Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := InAdvanceHelp;
end;

procedure TComputationalSettingsDLG.PlusRegularLabelMouseEnter(
  Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := PlusRegularHelp;
end;

procedure TComputationalSettingsDLG.ExactLabelMouseEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := ExactHelp;
end;

procedure TComputationalSettingsDLG.Rule78LabelMouseEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := Rule78Help;
end;

procedure TComputationalSettingsDLG.CenturyDivEditEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := DivCenturyHelp;
end;

procedure TComputationalSettingsDLG.PerYrComboEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := PerYrHelp;
end;

procedure TComputationalSettingsDLG.COLAMonthBoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := COLAMonthHelp;
end;

procedure TComputationalSettingsDLG.USARuleBoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := USARuleHelp;
end;

procedure TComputationalSettingsDLG.BasisBoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := BasisHelp;
end;

procedure TComputationalSettingsDLG.PrepaidBoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := PrepaidHelp;
end;

procedure TComputationalSettingsDLG.InAdvanceBoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := InAdvanceHelp;
end;

procedure TComputationalSettingsDLG.PlusRegularBoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := PlusRegularHelp;
end;

procedure TComputationalSettingsDLG.ExactBoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := ExactHelp;
end;

procedure TComputationalSettingsDLG.Rule78BoxEnter(Sender: TObject);
begin
  HelpTextLabel.Width := HelpLabelWidth;
  HelpTextLabel.Caption := Rule78Help;
end;

end.
