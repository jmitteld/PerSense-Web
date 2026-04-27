unit MAIN;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, Menus, WelcomeScreenUnit,
  StdCtrls, Dialogs, Buttons, Messages, ExtCtrls, ComCtrls, StdActns,
  ActnList, ToolWin, ImgList, ShellCtrls, MortgageScreenUnit, CHILDWIN,
  LogUnit, PersenseClipboardUnit, AmortizationScreenUnit, PresentValueScreenUnit;

const
  // indexes for things in the ActionList1
  ACTION_MORTGAGE               = 15;
  ACTION_AMORTIZATION           = 28;
  ACTION_PRESENTVALUE           = 40;
  ACTION_REMOVE_SCREEN          = 22;
  AMZADVANCED_MENU_INDEX        = 2;
  PVLADVANCED_MENU_INDEX        = 2;
  
type
// This is the main application form, the MDI parents, the big screen
// that open's when you run Persense.  Praise to it.
  TMainForm = class(TForm)
    MainMenu1: TMainMenu;
    File1: TMenuItem;
    Window1: TMenuItem;
    Help1: TMenuItem;
    N1: TMenuItem;
    FileExitItem: TMenuItem;
    WindowCascadeItem: TMenuItem;
    WindowTileItem: TMenuItem;
    WindowArrangeItem: TMenuItem;
    HelpAboutItem: TMenuItem;
    OpenDialog1: TOpenDialog;
    FileSaveItem: TMenuItem;
    FileSaveAsItem: TMenuItem;
    Edit1: TMenuItem;
    CutItem: TMenuItem;
    CopyItem: TMenuItem;
    PasteItem: TMenuItem;
    WindowMinimizeItem: TMenuItem;
    StatusBar: TStatusBar;
    ActionList1: TActionList;
    FileNew1: TAction;
    FileSave1: TAction;
    FileExit1: TAction;
    FileOpen1: TAction;
    FileSaveAs1: TAction;
    WindowCascade1: TWindowCascade;
    WindowTileHorizontal1: TWindowTileHorizontal;
    WindowArrangeAll1: TWindowArrange;
    WindowMinimizeAll1: TWindowMinimizeAll;
    HelpAbout1: TAction;
    FileClose1: TWindowClose;
    WindowTileVertical1: TWindowTileVertical;
    WindowTileItem2: TMenuItem;
    ToolBar2: TToolBar;
    ToolButton2: TToolButton;
    ToolButton3: TToolButton;
    ToolButton4: TToolButton;
    ToolButton5: TToolButton;
    ToolButton6: TToolButton;
    ToolButton7: TToolButton;
    ToolButton8: TToolButton;
    ToolButton10: TToolButton;
    ToolButton11: TToolButton;
    ImageList1: TImageList;
    Settings: TMenuItem;
    SaveDialog1: TSaveDialog;
    Calculate1: TMenuItem;
    CalcCalculate1: TAction;
    Calculate2: TMenuItem;
    N2: TMenuItem;
    EditHarden1: TAction;
    HardenValue1: TMenuItem;
    EditUndo1: TAction;
    EditRedo1: TAction;
    EditDelete1: TAction;
    N3: TMenuItem;
    Undo1: TMenuItem;
    Redo1: TMenuItem;
    Delete1: TMenuItem;
    EditCut1: TAction;
    EditCopy1: TAction;
    EditPaste1: TAction;
    UIOptions1: TMenuItem;
    SettingsUIOptions1: TAction;
    SettingsClipboard1: TAction;
    Clipboard1: TMenuItem;
    HelpContents1: TAction;
    HelpExamples1: TAction;
    Contents1: TMenuItem;
    InlineExamples1: TMenuItem;
    N4: TMenuItem;
    N5: TMenuItem;
    CalcMtgCompareAPRs1: TAction;
    CalcMtgGenerateMtgRows1: TAction;
    FilePrint1: TAction;
    PrintDialog1: TPrintDialog;
    N6: TMenuItem;
    Print1: TMenuItem;
    ToolButton1: TToolButton;
    FileSaveAll1: TAction;
    FileSaveAllItem: TMenuItem;
    Close1: TMenuItem;
    CalcAmzToggleAdvancedOptions1: TAction;
    ScreenShowMortgage: TAction;
    FileOpen: TMenuItem;
    Screens1: TMenuItem;
    ScreensMortgage: TMenuItem;
    ScreenShowAmortization: TAction;
    ShowAmortization1: TMenuItem;
    RightClickMenu: TPopupMenu;
    ShowMortgage1: TMenuItem;
    ShowAmortization2: TMenuItem;
    CalcAMZTableOutput1: TAction;
    SettingsComputational1: TAction;
    Computational1: TMenuItem;
    CalcAMZAPRReport1: TAction;
    ScreenWelcomeScreen: TAction;
    WelcomeScreen1: TMenuItem;
    ScreenShowPresentValue: TAction;
    ShowPresentValue1: TMenuItem;
    CalcPVLToggleAdvancedOptions1: TAction;
    ShowPresentValue2: TMenuItem;
    HelpContextualHelp1: TAction;
    ContectualHelp1: TMenuItem;
    CalcPVLMakeTable1: TAction;
    procedure HelpAbout1Execute(Sender: TObject);
    procedure FileExit1Execute(Sender: TObject);
    procedure CalcCalculate1Execute(Sender: TObject);
    procedure EditHarden1Execute(Sender: TObject);
    procedure EditUndo1Execute(Sender: TObject);
    procedure EditRedo1Execute(Sender: TObject);
    procedure EditDelete1Execute(Sender: TObject);
    procedure EditCut1Execute(Sender: TObject);
    procedure EditCopy1Execute(Sender: TObject);
    procedure EditPaste1Execute(Sender: TObject);
    procedure SettingsUIOptions1Execute(Sender: TObject);
    procedure SettingsClipboard1Execute(Sender: TObject);
    procedure HelpContents1Execute(Sender: TObject);
    procedure HelpExamples1Execute(Sender: TObject);
    procedure CalcMtgCompareAPRs1Execute(Sender: TObject);
    procedure CalcMtgGenerateMtgRows1Execute(Sender: TObject);
    procedure FileSave1Execute(Sender: TObject);
    procedure FileSaveAs1Execute(Sender: TObject);
    procedure FilePrint1Execute(Sender: TObject);
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
    procedure FileClose1Execute(Sender: TObject);
    procedure CalcAmzToggleAdvancedOptions1Execute(Sender: TObject);
    procedure ScreenShowMortgageExecute(Sender: TObject);
    procedure FileOpen1Execute(Sender: TObject);
    procedure FileSaveAll1Execute(Sender: TObject);
    procedure ScreenShowAmortizationExecute(Sender: TObject);
    procedure CalcAMZTableOutput1Execute(Sender: TObject);
    procedure SettingsComputational1Execute(Sender: TObject);
    procedure CalcAMZAPRReport1Execute(Sender: TObject);
    procedure ScreenWelcomeScreenExecute(Sender: TObject);
    procedure ScreenShowPresentValueExecute(Sender: TObject);
    procedure CalcPVLToggleAdvancedOptions1Execute(Sender: TObject);
    procedure HelpContextualHelp1Execute(Sender: TObject);
    procedure CalcPVLMakeTable1Execute(Sender: TObject);
  public
    constructor Create(AOwner: TComponent); override;
    destructor Destroy(); override;
    procedure ChildActivating( Child: TObject );
    procedure ChildClosing( Child: TObject );
    procedure SetStatusBarText( TheText: string );
  protected
    m_CellFont: TFont;
    m_CellBackgroundColor: TColor;
    m_OutpCellFont: TFont;
    m_OutpCellBackgroundColor: TColor;
    m_SelectedCellColor: TColor;
    m_BasePath: string;
    procedure OnHelpNavigationComplete( CurrentFile: string );
    procedure OnHelpSystemClosed();
    procedure OnNotify( var AMsg: TMessage); message WM_NOTIFY;
    procedure RemoveCalcMenus();
    procedure CreateMortgageSubMenu();
    procedure CreateAmortizationSubMenu();
    procedure CreatePresentValueSubMenu();
    procedure AssureOpenMortgageScreen();
    procedure AssureOpenAmortizationScreen();
    procedure AssureOpenPresentValueScreen();
    procedure ReadRegistryInfo();
  end;

var
  MainForm: TMainForm;

implementation

{$R *.dfm}

uses about, WinProcs, UISettingsDlgUnit, ClipboardSettingsDlgUnit, peData,
     HelpSystemUnit, Globals, FileIOUnit, peTypes, ComputationalSettingsDlgUnit,
     APRReportScreenUnit, Registry;

constructor TMainForm.Create(AOwner: TComponent);
var
  i: integer;
begin
  inherited Create(AOwner);
  MasterLog := TLog.Create();
  MasterLog.Initialize( 'MasterLog.log', LVL_LOW );
  MasterLog.Write( LVL_HIGH, 'MasterLog object created' );
  MasterLog.Write( LVL_LOW, 'TMainForm.Create begining' );
  m_BasePath := Application.ExeName;
  for i:=Length( m_BasePath ) downto 0 do begin
    if( m_BasePath[i] = '\' ) then begin
      m_BasePath := copy( m_BasePath, 0, i );
      break;
    end;
  end;
  HelpSystem := THelpSystem.Create( Handle, m_BasePath );
  HelpSystem.OnNavigationComplete := OnHelpNavigationComplete;
  HelpSystem.OnHelpSystemClosed := OnHelpSystemClosed;
  PersenseClipboard := TPersenseClipboard.Create();
  PersenseClipboard.SetCellDelimiter( #9 );
  PersenseClipboard.SetRowDelimiter( #13+#10 );
  m_CellFont := TFont.Create();
  m_CellFont.Assign( Canvas.Font );
  m_CellBackgroundColor := clWhite;
  m_OutpCellFont := TFont.Create();
  m_OutpCellFont.Assign( Canvas.Font );
  m_OutpCellBackgroundColor := clMoneyGreen;
  m_SelectedCellColor := clYellow;
  Close1.Enabled := true;
  MortgageScreen := nil;
  AmortizationScreen := nil;
  PresentValueScreen := nil;
  ReadRegistryInfo();
  MasterLog.Write( LVL_LOW, 'TMainForm.Create finished' );
end;

destructor TMainForm.Destroy();
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.Destroy begining' );
  HelpSystem.Free();
  APRReportScreen.Free();
  MortgageScreen.Free();
  AmortizationScreen.Free();
  PresentValueScreen.Free();
  PersenseClipboard.Free();
  m_CellFont.Free();
  m_OutpCellFont.Free();
  inherited Destroy;
  MasterLog.Write( LVL_HIGH, 'MasterLog being destroyed.' );
  MasterLog.Free();
  MasterLog := nil;
end;

procedure TMainForm.ReadRegistryInfo();
var
  Registry: TRegistry;
  Styles: TFontStyles;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ReadRegistryInfo begin' );
  Registry := TRegistry.Create(KEY_READ);
  Registry.RootKey := HKEY_CURRENT_USER;
  if( Registry.OpenKey('SOFTWARE\PerSense\', FALSE)) then begin
    m_CellBackgroundColor := Registry.ReadInteger('CellColor' );
    m_SelectedCellColor := Registry.ReadInteger('SelectedCellColor' );
    m_OutpCellBackgroundColor := Registry.ReadInteger('OutpCellColor' );
    // cell font
    m_CellFont.Color := Registry.ReadInteger( 'CellFontColor' );
    m_CellFont.Charset := Registry.ReadInteger( 'CellFontCharset' );
    m_CellFont.Name := Registry.ReadString( 'CellFontName' );
    m_CellFont.Pitch := TFontPitch(Registry.ReadInteger( 'CellFontPitch' ));
    m_CellFont.Size := Registry.ReadInteger( 'CellFontSize' );
    Registry.ReadBinaryData( 'CellFontStyle', Styles, sizeof(TFontStyles) );
    m_CellFont.Style := Styles;
    m_CellFont.Height := Registry.ReadInteger( 'CellFontHeight' );
    // outp cell font
    m_OutpCellFont.Color := Registry.ReadInteger( 'OutpCellFontColor' );
    m_OutpCellFont.Charset := Registry.ReadInteger( 'OutpCellFontCharset' );
    m_OutpCellFont.Name := Registry.ReadString( 'OutpCellFontName' );
    m_OutpCellFont.Pitch := TFontPitch(Registry.ReadInteger( 'OutpCellFontPitch' ));
    m_OutpCellFont.Size := Registry.ReadInteger( 'OutpCellFontSize' );
    Registry.ReadBinaryData( 'OutpCellFontStyle', Styles, sizeof(TFontStyles) );
    m_OutpCellFont.Style := Styles;
    m_OutpCellFont.Height := Registry.ReadInteger( 'OutpCellFontHeight' );
  end;
  Registry.Free;
end;

procedure TMainForm.FormClose(Sender: TObject; var Action: TCloseAction);
var
  RetCode: integer;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FormClose begining' );
  OnHelpSystemClosed();
  if( MortgageScreen <> nil ) then begin
    if( MortgageScreen.HasUnsavedData() ) then begin
      MessageBoxYesNoCancel( 'Unsaved Mortgage data.  Would you like to save?', RetCode, DO_UnsavedMortgage );
      if( RetCode = DLG_YES ) then
        MortgageScreen.SaveFile( MortgageScreen.GetFileName(), MortgageScreen.Caption )
      else if( RetCode = DLG_CANCEL ) then begin
        Action := caNone;
        exit;
      end;
    end;
  end;
  if( AmortizationScreen <> nil ) then begin
    if( AmortizationScreen.HasUnsavedData() ) then begin
      MessageBoxYesNoCancel( 'Unsaved Amortization data.  Would you like to save?', RetCode, DO_UnsavedAmortization );
      if( RetCode = DLG_YES ) then
        AmortizationScreen.SaveFile( AmortizationScreen.GetFileName(), AmortizationScreen.Caption )
      else if( RetCode = DLG_CANCEL ) then begin
        Action := caNone;
        exit;
      end;
    end;
  end;
  if( PresentValueScreen <> nil ) then begin
    if( PresentValueScreen.HasUnsavedData() ) then begin
      MessageBoxYesNoCancel( 'Unsaved Present Value data.  Would you like to save?', RetCode, DO_UnsavedPresentValue );
      if( RetCode = DLG_YES ) then
        PresentValueScreen.SaveFile( PresentValueScreen.GetFileName(), PresentValueScreen.Caption )
      else if( RetCode = DLG_CANCEL ) then begin
        Action := caNone;
        exit;
      end;
    end;
  end;
  Action := caFree;
  MasterLog.Write( LVL_LOW, 'TMainForm.FormClose ending' );
end;

procedure TMainForm.HelpAbout1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpAbout1Execute' );
  AboutBox.ShowModal();
end;

procedure TMainForm.FileExit1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileExit1Execute' );
  Close();
end;

procedure TMainForm.ChildActivating( Child: TObject );
begin
  MasterLog.Write( LVL_LOW, 'TmainForm.ChildActivating' );
  case TMDIChild(Child).GetType() of
    MortgageType: CreateMortgageSubMenu();
    AmortizationType: CreateAmortizationSubMenu();
    PresentValueType: CreatePresentValueSubMenu();
  else
    RemoveCalcMenus();
  end;
end;

procedure TMainForm.ChildClosing( Child: TObject );
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ChildClosing' );
  RemoveCalcMenus();
end;

procedure TMainForm.RemoveCalcMenus();
begin
  while( Calculate1.Count > 2 ) do begin
    Calculate1.Delete( Calculate1.Count-1 );
  end;
end;

procedure TMainForm.CreateMortgageSubMenu();
var
  CompareAPRsMenu: TMenuItem;
  GenerateMtgRowsMenu: TMenuItem;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CreateMortgageSubMenu' );
  RemoveCalcMenus();
  CompareAPRsMenu := TMenuItem.Create( Self );
  CompareAPRsMenu.Action := TAction.Create( Self );
  CompareAPRsMenu.Action.OnExecute := CalcMtgCompareAPRs1Execute;
  CompareAprsMenu.Caption := 'Compare &APRs';
  CompareAprsMenu.ShortCut := ShortCut(Word('A'), [ssCtrl]);
  GenerateMtgRowsMenu := TMenuItem.Create( Self );
  GenerateMtgRowsMenu.Action := TAction.Create( Self );
  GenerateMtgRowsMenu.Action.OnExecute := CalcMtgGenerateMtgRows1Execute;
  GenerateMtgRowsMenu.Caption := '&Generate Mtg Rows';
  GenerateMtgRowsMenu.ShortCut := ShortCut(Word('T'), [ssCtrl]);
  Calculate1.Add( CompareAPRsMenu );
  Calculate1.Add( GenerateMtgRowsMenu );
end;

procedure TMainForm.CreateAmortizationSubMenu();
var
  AdvancedOptionsMenu: TMenuItem;
  TableOutputMenu: TMenuItem;
  APRReportMenu: TMenuItem;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CreateAmortizationSubMenu' );
  RemoveCalcMenus();
  AdvancedOptionsMenu := TMenuItem.Create( Self );
  AdvancedOptionsMenu.Action := TAction.Create( Self );
  AdvancedOptionsMenu.Action.OnExecute := CalcAmzToggleAdvancedOptions1Execute;
  AdvancedOptionsMenu.AutoCheck := true;
  AdvancedOptionsMenu.Caption := 'Advanced Options';
  AdvancedOptionsMenu.ShortCut := ShortCut(Word('X'), [ssAlt]);
  TableOutputMenu := TMenuItem.Create( Self );
  TableOutputMenu.Action := TAction.Create( Self );
  TableOutputMenu.Action.OnExecute := CalcAMZTableOutput1Execute;
  TableOutputMenu.Caption := 'Table Output';
  TableOutputMenu.ShortCut := ShortCut(Word('T'), [ssCtrl]);
  APRReportMenu := TMenuItem.Create( Self );
  APRReportMenu.Action := TAction.Create( Self );
  APRReportMenu.Action.OnExecute := CalcAMZAPRReport1Execute;
  APRReportMenu.Caption := 'APR Report';
  APRReportMenu.ShortCut := ShortCut( Word('A'), [ssCtrl] );
  Calculate1.Add( AdvancedOptionsMenu );
  Calculate1.Add( TableOutputMenu );
  Calculate1.Add( APRReportMenu );
  if( AmortizationScreen.AdvancedIsOn() ) then
    AdvancedOptionsMenu.Checked := true
  else
    AdvancedOptionsMenu.Checked := false;
end;

procedure TMainForm.CreatePresentValueSubMenu();
var
  AdvancedOptionsMenu: TMenuItem;
  TableOutputMenu: TMenuItem;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CreatePresentValueSubMenu' );
  RemoveCalcMenus();
  TableOutputMenu := TMenuItem.Create( Self );
  TableOutputMenu.Action := TAction.Create( Self );
  TableOutputMenu.Action.OnExecute := CalcPVLMakeTable1Execute;
  TableOutputMenu.Caption := 'Table Output';
  TableOutputMenu.ShortCut := ShortCut(Word('T'), [ssCtrl]);
  AdvancedOptionsMenu := TMenuItem.Create( Self );
  AdvancedOptionsMenu.Action := TAction.Create( Self );
  AdvancedOptionsMenu.Action.OnExecute := CalcPVLToggleAdvancedOptions1Execute;
  AdvancedOptionsMenu.AutoCheck := true;
  AdvancedOptionsMenu.Caption := 'Advanced Options';
  AdvancedOptionsMenu.ShortCut := ShortCut(Word('X'), [ssAlt]);
  Calculate1.Add( AdvancedOptionsMenu );
  Calculate1.Add( TableOutputMenu );
  if( PresentValueScreen.AdvancedIsOn() ) then
    AdvancedOptionsMenu.Checked := true
  else
    AdvancedOptionsMenu.Checked := false;
end;

procedure TMainForm.AssureOpenMortgageScreen();
begin
  if( MortgageScreen = nil ) then begin
    MortgageScreen := TMortgageScreen.Create( Self );
    MortgageScreen.OnActivate := ChildActivating;
    MortgageScreen.OnChildClosing := ChildClosing;
    MortgageScreen.SetUISettings( m_CellBackgroundColor, m_OutpCellBackgroundColor, m_SelectedCellColor, m_CellFont, m_OutpCellFont );
  end;
  MortgageScreen.BringToFront();
  MortgageScreen.ReallyShow();
  ChildActivating( MortgageScreen );
  MortgageScreen.WindowState := wsNormal;
end;

procedure TMainForm.AssureOpenAmortizationScreen();
begin
  if( AmortizationScreen = nil ) then begin
    AmortizationScreen := TAmortizationScreen.Create( Self );
    AmortizationScreen.OnActivate := ChildActivating;
    AmortizationScreen.OnChildClosing := ChildClosing;
    AmortizationScreen.SetUISettings( m_CellBackgroundColor, m_OutpCellBackgroundColor, m_SelectedCellColor, m_CellFont, m_OutpCellFont );
  end;
  AmortizationScreen.BringToFront();
  AmortizationScreen.ReallyShow();
  ChildActivating( AmortizationScreen );
  AmortizationScreen.WindowState := wsNormal;
end;

procedure TMainForm.AssureOpenPresentValueScreen();
begin
  if( PresentValueScreen = nil ) then begin
    PresentValueScreen := TPresentValueScreen.Create( Self );
    PresentValueScreen.OnActivate := ChildActivating;
    PresentValueScreen.OnChildClosing := ChildClosing;
    PresentValueScreen.SetUISettings( m_CellBackgroundColor, m_OutpCellBackgroundColor, m_SelectedCellColor, m_CellFont, m_OutpCellFont );
  end;
  PresentValueScreen.BringToFront();
  PresentValueScreen.ReallyShow();
  ChildActivating( PresentValueScreen );
  PresentValueScreen.WindowState := wsNormal;
end;

//
// Action functions and direct helpers
//

procedure TMainForm.FileClose1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileClose1Execute' );
  if( ActiveMDIChild <> nil ) then
    ActiveMDIChild.Close();
end;

procedure TMainForm.CalcCalculate1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcCalculate1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnCalculate();
end;

procedure TMainForm.EditHarden1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditHarden1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnHardenValue();
end;

procedure TMainForm.EditUndo1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditUndo1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnUndo();
end;

procedure TMainForm.EditRedo1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditRedo1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnRedo();
end;

procedure TMainForm.EditDelete1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditDelete1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnDelete();
end;

procedure TMainForm.EditCut1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditCut1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnCut();
end;

procedure TMainForm.EditCopy1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditCopy1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnCopy();
end;

procedure TMainForm.EditPaste1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditPaste1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnPaste();
end;

procedure TMainForm.SettingsUIOptions1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.SettingsUIOptions1Execute' );
  UISettingsDlg.SetCellBackgroundColor( m_CellBackgroundColor );
  UISettingsDlg.SetHighlightedCellBackgroundColor( m_SelectedCellColor );
  UISettingsDlg.SetSoftValueCellBackgroundColor( m_OutpCellBackgroundColor );
  UISettingsDlg.SetCellFont( m_CellFont );
  UISettingsDlg.SetOutpCellFont( m_OutpCellFont );
  UISettingsDlg.ShowModal();
  if( UISettingsDlg.ModalResult = mrOK ) then begin
    m_CellBackgroundColor := UISettingsDlg.GetCellBackgroundColor();
    m_SelectedCellColor := UISettingsDlg.GetHighlightedCellBackgroundColor();
    m_OutpCellBackgroundColor := UISettingsDlg.GetSoftValueCellBackgroundColor();
    m_CellFont.Free();
    m_OutpCellFont.Free();
    m_CellFont := TFont.Create();
    m_OutpCellFont := TFont.Create();
    UISettingsDlg.RetrieveCellFont( m_CellFont );
    UISettingsDlg.RetrieveOutpCellFont( m_OutpCellFont );
    if( MortgageScreen <> nil ) then
      MortgageScreen.SetUISettings( m_CellBackgroundColor, m_OutpCellBackgroundColor, m_SelectedCellColor, m_CellFont, m_OutpCellFont );
    if( AmortizationScreen <> nil ) then
      AmortizationScreen.SetUISettings( m_CellBackgroundColor, m_OutpCellBackgroundColor, m_SelectedCellColor, m_CellFont, m_OutpCellFont );
    if( PresentValueScreen <> nil ) then
      PresentValueScreen.SetUISettings( m_CellBackgroundColor, m_OutpCellBackgroundColor, m_SelectedCellColor, m_CellFont, m_OutpCellFont );
  end;
end;

procedure TMainForm.SettingsComputational1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.SettingsComputational1Execute' );
  WelcomeScreen.ReallyHide();
  ComputationalSettingsDlg.LoadSettings( df.c );
  ComputationalSettingsDlg.ShowModal();
  if( ComputationalSettingsDlg.ModalResult = mrOK ) then begin
    ComputationalSettingsDlg.GetSettings( df.c );
  end;
end;

procedure TMainForm.SettingsClipboard1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.SettingsClipboard1Execute' );
  if( PersenseClipboard.GetCellDelimiter() = #9 ) then
    ClipboardSettingsDlg.CellDelimiter.Text := '{tab}'
  else if( PersenseClipboard.GetCellDelimiter() = #13+#10 ) then
    ClipboardSettingsDlg.CellDelimiter.Text := '{CR+LF}'
  else
    ClipboardSettingsDlg.CellDelimiter.Text := PersenseClipboard.GetCellDelimiter();

  if( PersenseClipboard.GetRowDelimiter() = #9 ) then
    ClipboardSettingsDlg.RowDelimiter.Text := '{tab}'
  else if( PersenseClipboard.GetRowDelimiter() = #13+#10 ) then
    ClipboardSettingsDlg.RowDelimiter.Text := '{CR+LF}'
  else
    ClipboardSettingsDlg.RowDelimiter.Text := PersenseClipboard.GetRowDelimiter();

  ClipboardSettingsDlg.ShowModal();
  if( ClipboardSettingsDlg.ModalResult = mrOK ) then begin
    if( ClipboardSettingsDlg.CellDelimiter.Text = '{tab}' ) then
      PersenseClipboard.SetCellDelimiter( #9 )
    else if( ClipboardSettingsDlg.CellDelimiter.Text = '{CR+LF}' ) then
      PersenseClipboard.SetCellDelimiter( #13+#10 )
    else
      PersenseClipboard.SetCellDelimiter( ClipboardSettingsDlg.CellDelimiter.Text );

    if( ClipboardSettingsDlg.RowDelimiter.Text = '{tab}' ) then
      PersenseClipboard.SetRowDelimiter( #9 )
    else if( ClipboardSettingsDlg.RowDelimiter.Text = '{CR+LF}' ) then
      PersenseClipboard.SetRowDelimiter( #13+#10 )
    else
      PersenseClipboard.SetRowDelimiter( ClipboardSettingsDlg.RowDelimiter.Text );
  end;
end;

procedure TMainForm.HelpContents1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpContents1Execute' );
  WelcomeScreen.ReallyHide();
  HelpSystem.DisplayContents( '' );
end;

procedure TMainForm.HelpExamples1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpExamples1Execute' );
  WelcomeScreen.ReallyHide();
  HelpSystem.DisplayContents( 'Example_Overview.html' );
end;

procedure TMainForm.CalcMtgCompareAPRs1Execute(Sender: TObject);
var
  Child: TMDIChild;
  MortgageScreen : TMortgageScreen;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcMtgCompareAPRs1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child.GetType <> MortgageType ) then exit;
  MortgageScreen := TMortgageScreen(ActiveMDIChild);
  MortgageScreen.CompareAPRs();
end;

procedure TMainForm.CalcMtgGenerateMtgRows1Execute(Sender: TObject);
var
  Child: TMDIChild;
  MortgageScreen : TMortgageScreen;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcMtgGenerateMtgRows1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child.GetType <> MortgageType ) then exit;
  MortgageScreen := TMortgageScreen(ActiveMDIChild);
  MortgageScreen.GenerateMtgRows();
end;

procedure TMainForm.CalcAmzToggleAdvancedOptions1Execute(Sender: TObject);
var
  Child: TMDIChild;
  AmzScreen: TAmortizationScreen;
  AdvancedMenu: TMenuItem;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcAmzToggleAdvancedOptions1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child.GetType <> AmortizationType ) then exit;
  AmzScreen := TAmortizationScreen(ActiveMDIChild);
  AmzScreen.ToggleAdvanced();
  AdvancedMenu := Calculate1.Items[AMZADVANCED_MENU_INDEX];
  AdvancedMenu.Checked := AMZScreen.AdvancedIsOn();
end;

procedure TMainForm.CalcAMZTableOutput1Execute(Sender: TObject);
var
  Child: TMDIChild;
  AmzScreen: TAmortizationScreen;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcAMZTableOutput1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child.GetType <> AmortizationType ) then exit;
  AmzScreen := TAmortizationScreen(ActiveMDIChild);
  AMZScreen.TableOutput();
end;

procedure TMainForm.CalcAMZAPRReport1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcAMZAPRReport1Execute' );
  if( APRReportScreen = nil ) then begin
    APRReportScreen := TAPRReportScreen.Create( Self );
    APRReportScreen.OnActivate := ChildActivating;
    APRReportScreen.ReallyHide();
  end;
  if( not APRReportScreen.CreateReport() ) then exit;
  APRReportScreen.BringToFront();
  APRReportScreen.ReallyShow();
  ChildActivating( APRReportScreen );
end;

procedure TMainForm.CalcPVLToggleAdvancedOptions1Execute(Sender: TObject);
var
  Child: TMDIChild;
  PVLScreen: TPresentValueScreen;
  AdvancedMenu: TMenuItem;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcPVLToggleAdvancedOptions1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child.GetType <> PresentValueType ) then exit;
  PVLScreen := TPresentValueScreen(ActiveMDIChild);
  PVLScreen.ToggleAdvanced();
  AdvancedMenu := Calculate1.Items[PVLADVANCED_MENU_INDEX];
  AdvancedMenu.Checked := PVLScreen.AdvancedIsOn();
end;

procedure TMainForm.CalcPVLMakeTable1Execute(Sender: TObject);
var
  Child: TMDIChild;
  PVLScreen: TPresentValueScreen;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcPVLMakeTable1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child.GetType <> PresentValueType ) then exit;
  PVLScreen := TPresentValueScreen(ActiveMDIChild);
  PVLScreen.TableOutput();
end;

procedure TMainForm.FileSave1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileSave1Execute' );
  if( ActiveMDIChild <> nil ) then begin
    Child := TMDIChild( ActiveMDIChild );
    if( not Child.SaveFile( Child.GetFileName(), '' ) ) then exit;
    Child.Caption := Child.GetFileName();
  end;
end;

// exact copy of FileSaveExecute1 except I don't specify a file
// name when I call SaveFile
procedure TMainForm.FileSaveAs1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileSaveAs1Execute' );
  if( ActiveMDIChild <> nil ) then begin
    Child := TMDIChild( ActiveMDIChild );
    if( not Child.SaveFile( '', '' ) ) then exit;
    Child.Caption := Child.GetFileName();
  end;
end;

procedure TMainForm.FileSaveAll1Execute(Sender: TObject);
var
  i: integer;
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileSaveAll1Execute' );
  for i:=0 to MDIChildCount-1 do begin
    Child := TMDIChild(MDIChildren[i]);
    Child.SaveFile( Child.GetFileName(), '' );
    Child.Caption := Child.GetFileName();
  end;
end;

procedure TMainForm.FilePrint1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FilePrint1Execute' );
  if( ActiveMDIChild <> nil ) then begin
    if( not PrintDialog1.Execute() ) then exit;
    Child := TMDIChild( ActiveMDIChild );
    Child.OnPrint();
  end;
end;

procedure TMainForm.ScreenShowMortgageExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenShowMortgageExecute' );
  WelcomeScreen.ReallyHide();
  AssureOpenMortgageScreen();
end;

procedure TMainForm.ScreenShowAmortizationExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenShowAmortizationExecute' );
  WelcomeScreen.ReallyHide();
  AssureOpenAmortizationScreen();
end;

procedure TMainForm.ScreenShowPresentValueExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenShowPresentValueExecute' );
  WelcomeScreen.ReallyHide();
  AssureOpenPresentValueScreen();
end;

procedure TMainForm.FileOpen1Execute(Sender: TObject);
var
  TheFile: TFileIO;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileOpen1Execute' );
  OpenDialog1.Filter := 'Mortgage Files (*.mtg)|*.MTG|Amortization Files (*.amz)|*.AMZ|Present Value Files (*.pvl)|*.PVL|All Files(*)|*';
  if( not OpenDialog1.Execute() ) then
    exit;
  TheFile := TFileIO.Create();
  if( not TheFile.LoadFile( OpenDialog1.FileName ) ) then
    exit;
  case TheFile.GetIDByte() of
    MTGBlock: begin
      AssureOpenMortgageScreen();
      MortgageScreen.OpenFile( TheFile );
      MortgageScreen.Caption := MortgageScreen.GetFileName();
      end;
    AMZTopBlock: begin
      AssureOpenAmortizationScreen();
      AmortizationScreen.OpenFile( TheFile );
      AmortizationScreen.Caption := AmortizationScreen.GetFileName();
      end;
    PVLlumpsumblock: begin
      AssureOpenPresentValueScreen();
      PresentValueScreen.OpenFile( Thefile );
      PresentValueScreen.Caption := PresentValueScreen.GetFileName();
      end;
    else
      MessageBox( 'Error reading file: Unknown file type', DO_UnkownFileType );
  end;
  TheFile.Free();
  WelcomeScreen.ReallyHide();
end;

//
// help system
//
procedure TMainForm.SetStatusBarText( TheText: string );
begin
  StatusBar.SimpleText := TheText;
end;

// called for WM_NOTIFY messages.  Hook into these to catch help system
//  messages
procedure TMainForm.OnNotify( var AMsg: TMessage );
begin
  if( AMsg.WParam = NOTIFY_MESSAGE_ID ) then begin
    HelpSystem.CustomOnNotifyHandler( PHH_NOTIFY(AMsg.LParam) );
  end;
end;

{ called when someone navigates to a new page in the help system }
procedure TMainForm.OnHelpNavigationComplete( CurrentFile: string );
var
  TheFile: TFileIO;
  ScreenTypeString: string;
  HelpNumberString: string;
  HelpNumber: integer;
  HelpCode: integer;
  DotPosition: integer;
  HelpHTMLString: string;
  ValidNumber: boolean;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.OnHelpNavigationComplete' );
  TheFile := TFileIO.Create();
  // The CurrentFile must be a string of the form:
  // /??_Example*.html
  // where ?? is:
  //   MS = Mortgage screen example
  //   AM = Amortization screen example
  //   PV = Present Value screen example
  // and * is a the number of the example.
  // Based on this I can parse out the information to generate the
  // help code.
  ScreenTypeString := copy( CurrentFile, 2, 2 );
  DotPosition := Pos( '.', CurrentFile );
  if( DotPosition <> -1 ) then begin
    HelpNumberString := copy( CurrentFile, 12, length(CurrentFile)-DotPosition-3 );
    HelpCode := 0;
    if( ScreenTypeString = 'MS' ) then begin
      HelpCode := IM_MASK;
    end else if( ScreenTypeString = 'AM' ) then begin
      HelpCode := IA_MASK;
    end else if( ScreenTypeString = 'PV' ) then begin
      HelpCode := IP_MASK;
    end;
    try
      ValidNumber := true;
      HelpNumber := strtoint( HelpNumberString );
    except;
      ValidNumber := false;
    end;
    if( ValidNumber ) then begin
      HelpCode := HelpCode + HelpNumber;
      HelpHTMLString := HelpSystem.GetHelpString( HelpCode );
    end;
  end;
  if( HelpHTMLString <> '' ) then begin
    if( ScreenTypeString = 'MS' ) then begin
      AssureOpenMortgageScreen();
      if( not MortgageScreen.IsBackedUpForHelp() ) then MortgageScreen.BackupForHelpSystem();
      TheFile.LoadFile( m_BasePath + HelpHTMLString );
      MortgageScreen.OpenFile( TheFile );
    end else if( ScreenTypeString = 'AM' ) then begin
      AssureOpenAmortizationScreen();
      if( not AmortizationScreen.IsBackedUpForHelp() ) then AmortizationScreen.BackupForHelpSystem();
      TheFile.LoadFile( m_BasePath + HelpHTMLString );
      AmortizationScreen.OpenFile( TheFile );
    end else if( ScreenTypeString = 'PV' ) then begin
      AssureOpenPresentValueScreen();
      if( not PresentValueScreen.IsBackedUpForHelp() ) then PresentValueScreen.BackupForHelpSystem();
      TheFile.LoadFile( m_BasePath + HelpHTMLString );
      PresentValueScreen.OpenFile( TheFile );
    end;
  end else begin
    if( MortgageScreen <> nil ) then
      if( MortgageScreen.IsBackedUpForHelp() ) then MortgageScreen.RestoreFromHelpSystem();
    if( AmortizationScreen <> nil ) then
      if( AmortizationScreen.IsBackedUpForHelp() ) then AmortizationScreen.RestoreFromHelpSystem();
    if( PresentValueScreen <> nil ) then
      if( PresentValueScreen.IsBackedUpForHelp() ) then PresentValueScreen.RestoreFromHelpSystem();
  end;
  TheFile.Free();
end;

procedure TMainForm.OnHelpSystemClosed();
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.OnHelpSystemClosed' );
  if( MortgageScreen <> nil ) then
    if( MortgageScreen.IsBackedUpForHelp() ) then MortgageScreen.RestoreFromHelpSystem();
  if( AmortizationScreen <> nil ) then
    if( AmortizationScreen.IsBackedUpForHelp() ) then AmortizationScreen.RestoreFromHelpSystem();
  if( PresentValueScreen <> nil ) then
    if( PresentValueScreen.IsBackedUpForHelp() ) then PresentValueScreen.RestoreFromHelpSystem();
end;

procedure TMainForm.HelpContextualHelp1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpContextualHelp1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnContextualHelp();
end;

procedure TMainForm.ScreenWelcomeScreenExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenWelcomeScreenExecute' );
  WelcomeScreen.ReallyShow();
  WelcomeScreen.BringToFront();
  WelcomeScreen.WindowState := wsNormal;
end;

end.
