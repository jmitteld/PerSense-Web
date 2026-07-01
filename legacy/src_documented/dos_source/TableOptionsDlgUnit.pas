{ ===========================================================================
  Unit:  TableOptionsDlgUnit
  Role:  Modal dialog that configures how a table/schedule is laid out and
         exported before it is rendered or sent to a file.

  It offers four chained combo boxes:
    - DetailsCombo : 'Detail Only' vs 'Detail & Summary' (whether per-period
                     rows are accompanied by cumulative summary rows).
    - SummaryCombo : summary cadence - Annual / Semi-Annual / Quarterly /
                     Monthly (Monthly only offered when payments/yr > 12).
    - MonthsCombo  : which month(s) anchor each summary period (populated to
                     match the chosen cadence).
    - OutputCombo  : destination/format - on-screen, Spreadsheet, Comma
                     Separated File, or Raw Text File.
  The combos are dependent: changing Details rebuilds Summary, and changing
  Summary rebuilds Months.

  On OK the choices are translated into the GLOBAL cumulative-summary settings
  in Globals: the single 'cum' code character (its case = detail flag) and the
  'cumset' set of anchor months, plus the dialog's own SendToFile /
  CommaSeperated flags that the caller (TableOutUnit) reads back.

  The cum code letters (Y/S/Q/M, lowercase = summary-only, uppercase =
  detail+summary) mirror the legacy table engine's expectations.

  { Go port: n/a (presentation only) -- this configures the cumulative-summary
    aggregation (cum/cumset) and export format for the table report. Per the
    port's outstanding items, the yearly/quarterly summary aggregation (DOS
    V6-14) is presentation-grade and intentionally not fully ported; per-period
    numbers are unaffected. The detail schedule itself comes from
    internal/finance/amortization/engine.go (Amortize) via
    internal/api/handlers.go:752 (HandleAmortizationCalc); summary/format
    choices are handled client-side in cmd/persense/static/index.html. }
  ===========================================================================}
unit TableOptionsDlgUnit;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, StdCtrls, 
  Buttons, ExtCtrls, peData, peTypes, Globals;

type
  TTableOptionsDlg = class(TForm)
    OKBtn: TButton;
    CancelBtn: TButton;
    Bevel1: TBevel;
    Label1: TLabel;
    Label2: TLabel;
    Label3: TLabel;
    DetailsCombo: TComboBox;
    SummaryCombo: TComboBox;
    MonthsCombo: TComboBox;
    Label4: TLabel;
    OutputCombo: TComboBox;
    { Rebuilds MonthsCombo to match the chosen summary cadence. }
    procedure SummaryComboChange(Sender: TObject);
    { Rebuilds SummaryCombo (and Months) based on Detail-Only vs Detail+Summary }
    procedure DetailsComboChange(Sender: TObject);
    { Translates all selections into the global cum/cumset + export flags. }
    procedure OKBtnClick(Sender: TObject);
  protected
    { result: export to file rather than screen }
    m_bSendToFile: boolean;
    { result: use comma-separated formatting }
    m_bCommaSeperated: boolean;
    { payments-per-year (gates Monthly option) }
    m_PerYr: integer;
  public
    constructor Create( AOwner: TComponent ); override;
    { Accessor for the file-vs-screen result. }
    function SendToFile(): boolean;
    { Accessor for the comma-separated result. }
    function CommaSeperated(): boolean;
    { Supply the payments-per-year used to decide if Monthly is allowed. }
    procedure SetPerYr( PerYr: integer );
  end;

var
  TableOptionsDlg: TTableOptionsDlg;

implementation

{$R *.dfm}

{ Create
  Purpose: build the dialog with default selections.
  Side effects: selects the first Details/Output items and seeds m_PerYr from
                the global default payments-per-year (df.c.peryr). }
constructor TTableOptionsDlg.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  DetailsCombo.ItemIndex := 0;
  OutputCombo.ItemIndex := 0;
  m_PerYr := df.c.peryr;
end;

{ SetPerYr
  Purpose: override the payments-per-year used to decide whether the 'Monthly'
           summary cadence is offered.
  Param:   PerYr - payments per year for the table being exported. }
procedure TTableOptionsDlg.SetPerYr( PerYr: integer );
begin
  m_PerYr := PerYr;
end;

{ SummaryComboChange
  Purpose: when the summary cadence changes, repopulate MonthsCombo with the
           valid anchor-month choices for that cadence.
  Cadence -> choices:
    Annual      : single months 1..12
    Semi-Annual : pairs "n,n+6" for n=1..6
    Quarterly   : quads "n,n+3,n+6,n+9" for n=1..3
    Monthly     : single 'All' entry
  Side effects: clears/refills MonthsCombo and selects its first item.
  Triggered: by the SummaryCombo OnChange event. }
procedure TTableOptionsDlg.SummaryComboChange(Sender: TObject);
var
  i: integer;
begin
  MonthsCombo.Clear();
  if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Annual' ) then begin
    for i:=0 to 11 do
      MonthsCombo.Items.Add( inttostr( i+1 ) );
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Semi-Annual' ) then begin
    for i:=0 to 6 do
      MonthsCombo.Items.Add( inttostr( i+1 ) + ',' + inttostr( i+6 ) );
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Quarterly' ) then begin
    for i:=0 to 2 do
      MonthsCombo.Items.Add( inttostr( i+1 ) + ',' + inttostr( i+4 ) + ',' +
                       inttostr( i+7 ) + ',' + inttostr( i+10 ) );
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Monthly' ) then begin
    MonthsCombo.Items.Add( 'All' );
  end;
  MonthsCombo.ItemIndex := 0;
end;

{ DetailsComboChange
  Purpose: react to the Detail-Only vs Detail+Summary choice by enabling or
           disabling the summary controls.
  Behaviour:
    'Detail Only'    -> no summary, so clear Months (and leave Summary empty).
    otherwise        -> offer Annual/Semi-Annual/Quarterly cadences (plus
                        Monthly only when m_PerYr > 12), defaulting to Annual,
                        and seed Months with 1..12.
  Side effects: rebuilds SummaryCombo and MonthsCombo.
  Triggered: by the DetailsCombo OnChange event. }
procedure TTableOptionsDlg.DetailsComboChange(Sender: TObject);
var
  i: integer;
begin
  SummaryCombo.Clear();
  if( DetailsCombo.Items[DetailsCombo.ItemIndex] = 'Detail Only' ) then begin
    MonthsCombo.Clear();
  end else begin
    SummaryCombo.Items.Add( 'Annual' );
    SummaryCombo.Items.Add( 'Semi-Annual' );
    SummaryCombo.Items.Add( 'Quarterly' );
    if( m_PerYr > 12 ) then
      SummaryCombo.Items.Add( 'Monthly' );
    SummaryCombo.ItemIndex := 0;
    for i:=0 to 11 do
      MonthsCombo.Items.Add( inttostr( i+1 ) );
    MonthsCombo.ItemIndex := 0;
  end;
end;

{ OKBtnClick
  Purpose: translate the four combo selections into the global summary
           configuration consumed by the table-output engine, plus this
           dialog's export-format result flags.
  Globals written:
    cum    - one letter encoding cadence (Y/S/Q/M); UPPERCASE when detail rows
             are also wanted ('Detail & Summary'), lowercase for summary-only.
    cumset - set of month numbers that anchor the summary periods.
  Members written:
    m_bSendToFile     - true for 'Comma Seperated File' or 'Raw Text File'.
    m_bCommaSeperated - true for 'Comma Seperated File' or 'Spreadsheet'.
  Side effects: parses the MonthsCombo string (which may be a comma list) into
                cumset; for Annual/Monthly cumset is set directly.
  Triggered: by the OK button click (before the modal closes with mrOK). }
procedure TTableOptionsDlg.OKBtnClick(Sender: TObject);
var
  { true => include detail rows (upper-case cum code) }
  bUseDetail: boolean;
  { remaining comma-list of anchor months to parse }
  Months: string;
  { one month token split off of Months }
  Month: string;
  { position of next comma while tokenising }
  Pos1: integer;
begin
  bUseDetail := (DetailsCombo.Items[DetailsCombo.ItemIndex] = 'Detail & Summary');
  Months := '';
  cumset := [];
  m_bSendToFile := (OutputCombo.Items[OutputCombo.ItemIndex] = 'Comma Seperated File') or
                   (OutputCombo.Items[OutputCombo.ItemIndex] = 'Raw Text File');
  m_bCommaSeperated := (OutputCombo.Items[OutputCombo.ItemIndex] = 'Comma Seperated File') or
                       (OutputCombo.Items[OutputCombo.ItemIndex] = 'Spreadsheet');
  if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Annual' ) then begin
    if( bUseDetail ) then
      cum := 'Y'
    else
      cum := 'y';
    cumset := [strtoint( MonthsCombo.Items[MonthsCombo.ItemIndex] )];
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Semi-Annual' ) then begin
    if( bUseDetail ) then
      cum := 'S'
    else
      cum := 's';
    Months := MonthsCombo.Items[MonthsCombo.ItemIndex];
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Quarterly' ) then begin
    if( bUseDetail ) then
      cum := 'Q'
    else
      cum := 'q';
    Months := MonthsCombo.Items[MonthsCombo.ItemIndex];
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Monthly' ) then begin
    if( bUseDetail ) then
      cum := 'M'
    else
      cum := 'm';
    cumset := [1,2,3,4,5,6,7,8,9,10,11,12];
  end;
  { Tokenise the comma-separated anchor-month list (Semi-Annual/Quarterly)
    into individual integers and add each to cumset. }
  while( Months <> '' ) do begin
    Pos1 := Pos( ',', Months );
    if( Pos1 <> 0 ) then begin
      Month := Copy( Months, 0, Pos1-1 );
      Months := Copy( Months, Pos1+1, Length(Months)-Pos1 );
    end else begin
      Month := Months;
      Months := '';
    end;
    cumset := cumset + [strtoint(Month)];
  end;
end;

{ SendToFile - true if the user chose a file-export output option. }
function TTableOptionsDlg.SendToFile(): boolean;
begin
  SendToFile := m_bSendToFile;
end;

{ CommaSeperated - true if the user chose a comma-separated/spreadsheet format }
function TTableOptionsDlg.CommaSeperated(): boolean;
begin
  CommaSeperated := m_bCommaSeperated;
end;

end.
