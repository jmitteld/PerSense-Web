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

function TTableOptionsDlg.SendToFile(): boolean;
begin
  SendToFile := m_bSendToFile;
end;

function TTableOptionsDlg.CommaSeperated(): boolean;
begin
  CommaSeperated := m_bCommaSeperated;
end;

end.
