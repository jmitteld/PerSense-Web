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
    procedure SummaryComboChange(Sender: TObject);
    procedure DetailsComboChange(Sender: TObject);
    procedure OKBtnClick(Sender: TObject);
  protected
    m_bSendToFile: boolean;
    m_bCommaSeperated: boolean;
    m_PerYr: integer;
  public
    constructor Create( AOwner: TComponent ); override;
    function SendToFile(): boolean;
    function CommaSeperated(): boolean;
    procedure SetPerYr( PerYr: integer );
  end;

var
  TableOptionsDlg: TTableOptionsDlg;

implementation

{$R *.dfm}

constructor TTableOptionsDlg.Create( AOwner: TComponent );
begin
  inherited Create( AOwner );
  DetailsCombo.ItemIndex := 0;
  OutputCombo.ItemIndex := 0;
  m_PerYr := df.c.peryr;
end;

procedure TTableOptionsDlg.SetPerYr( PerYr: integer );
begin
  m_PerYr := PerYr;
end;

procedure TTableOptionsDlg.SummaryComboChange(Sender: TObject);
var
  i: integer;
begin
  MonthsCombo.Clear();
  if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Annual' ) then begin
    MonthsCombo.Items.Add( 'January' );
    MonthsCombo.Items.Add( 'February' );
    MonthsCombo.Items.Add( 'March' );
    MonthsCombo.Items.Add( 'April' );
    MonthsCombo.Items.Add( 'May' );
    MonthsCombo.Items.Add( 'June' );
    MonthsCombo.Items.Add( 'July' );
    MonthsCombo.Items.Add( 'August' );
    MonthsCombo.Items.Add( 'September' );
    MonthsCombo.Items.Add( 'October' );
    MonthsCombo.Items.Add( 'November' );
    MonthsCombo.Items.Add( 'December' );
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Semi-Annual' ) then begin
    MonthsCombo.Items.Add( 'Jan, Jul' );
    MonthsCombo.Items.Add( 'Feb, Aug' );
    MonthsCombo.Items.Add( 'Mar, Sep' );
    MonthsCombo.Items.Add( 'Apr, Oct' );
    MonthsCombo.Items.Add( 'May, Nov' );
    MonthsCombo.Items.Add( 'Jun, Dec' );
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Quarterly' ) then begin
    MonthsCombo.Items.Add( 'Jan, Apr, Jul, Oct' );
    MonthsCombo.Items.Add( 'Feb, May, Aug, Nov' );
    MonthsCombo.Items.Add( 'Mar, Jun, Sep, Dec' );
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Monthly' ) then begin
    MonthsCombo.Items.Add( 'All' );
  end;
  MonthsCombo.ItemIndex := 0;
end;

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
    MonthsCombo.Items.Add( 'January' );
    MonthsCombo.Items.Add( 'February' );
    MonthsCombo.Items.Add( 'March' );
    MonthsCombo.Items.Add( 'April' );
    MonthsCombo.Items.Add( 'May' );
    MonthsCombo.Items.Add( 'June' );
    MonthsCombo.Items.Add( 'July' );
    MonthsCombo.Items.Add( 'August' );
    MonthsCombo.Items.Add( 'September' );
    MonthsCombo.Items.Add( 'October' );
    MonthsCombo.Items.Add( 'November' );
    MonthsCombo.Items.Add( 'December' );
    MonthsCombo.ItemIndex := 0;
  end;
end;

procedure TTableOptionsDlg.OKBtnClick(Sender: TObject);
var
  bUseDetail: boolean;
  Months: string;
  Month: string;
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
    cumset := [MonthsCombo.ItemIndex+1];
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Semi-Annual' ) then begin
    if( bUseDetail ) then
      cum := 'S'
    else
      cum := 's';
    cumset := [MonthsCombo.ItemIndex+1];
    cumset := cumset + [MonthsCombo.ItemIndex+7];
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Quarterly' ) then begin
    if( bUseDetail ) then
      cum := 'Q'
    else
      cum := 'q';
    cumset := [MonthsCombo.ItemIndex+1];
    cumset := cumset + [MonthsCombo.ItemIndex+4];
    cumset := cumset + [MonthsCombo.ItemIndex+7];
    cumset := cumset + [MonthsCombo.ItemIndex+10];
  end else if( SummaryCombo.Items[SummaryCombo.ItemIndex] = 'Monthly' ) then begin
    if( bUseDetail ) then
      cum := 'M'
    else
      cum := 'm';
    cumset := [1,2,3,4,5,6,7,8,9,10,11,12];
  end else begin
    // if the user doesn't choose anything then it's
    // monthly, deatilonly
    cum := 'm';
    cumset := [1,2,3,4,5,6,7,8,9,10,11,12];
  end;
end;

function TTableOptionsDlg.SendToFile(): boolean;
begin
  SendToFile := m_bSendToFile;
end;

function TTableOptionsDlg.CommaSeperated(): boolean;
begin
  CommaSeperated := m_bCommaSeperated;
end;

end.
