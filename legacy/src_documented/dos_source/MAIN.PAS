{ ============================================================================
  Unit:    MAIN
  Role:    The application shell -- the MDI parent form (TMainForm) that hosts
           every calculator screen and owns all global, app-wide wiring.

  Responsibilities:
    - Startup / shutdown: creates the master log, help system, clipboard helper,
      default cell fonts/colours, then loads user UI prefs from the registry
      (ReadRegistryInfo); tears all of that down in Destroy.
    - Screen orchestration: the three calculator screens (Mortgage, Amortization,
      Present Value) are lazily created singletons.  AssureOpen*Screen creates on
      first use and shows; FileOpen routes a just-loaded file to the matching
      screen by its block id.
    - Menu/toolbar dispatch: the static main menu + ActionList route generic
      commands (Calculate, Cut/Copy/Paste/Undo/Redo/Harden/Delete, Save/Print,
      Contextual Help) to whichever TMDIChild is active, via its virtual On*()
      methods.  ChildActivating swaps in a screen-specific "Calculate" submenu
      (CreateMortgageSubMenu / CreateAmortizationSubMenu / CreatePresentValueSubMenu).
    - Settings dialogs: UI options, clipboard delimiters, computational defaults.
    - Help-system glue: OnNotify catches WM_NOTIFY help messages; the help
      "examples" feature drives a screen to load an example file and later
      restores the user's real data (see Backup/RestoreFromHelpSystem on screens).

  Globals wired here (declared elsewhere, e.g. Globals/peData): MasterLog,
  HelpSystem, PersenseClipboard, and the screen singletons MortgageScreen,
  AmortizationScreen, PresentValueScreen, APRReportScreen, WelcomeScreen.

  { Go port: n/a (menu/MDI shell) -- no financial logic; this is the DOS/Windows
    application frame. The web port has no MDI shell: navigation and command
    routing live in cmd/persense/static/index.html, and each screen's Calculate
    posts to a stateless endpoint in internal/api/handlers.go:
      Mortgage      -> HandleMortgageCalc (478), CompareAPRs -> HandleMortgageCompare
                       (625), GenerateMtgRows -> HandleMortgageWhatIf (696)
      Amortization  -> HandleAmortizationCalc (752)
      Present Value -> HandlePVCalc (1317)
      FileOpen      -> HandleImportPSN (internal/api/import_psn.go:121)
    Settings dialogs map to internal/types/defaults.go (DefaultCompDefaults:21,
    DefaultAppDefaults:51) + the frontend settings panel. Endpoint-relevant
    dispatch procs carry their own cross-ref below; the rest (menu build,
    clipboard/UI/help glue, MDI activation) are n/a. }
  ============================================================================ }
unit MAIN;

interface

uses Windows, SysUtils, Classes, Graphics, Forms, Controls, Menus, WelcomeScreenUnit,
  StdCtrls, Dialogs, Buttons, Messages, ExtCtrls, ComCtrls, StdActns,
  ActnList, ToolWin, ImgList, ShellCtrls, MortgageScreenUnit, CHILDWIN,
  LogUnit, PersenseClipboardUnit, AmortizationScreenUnit, PresentValueScreenUnit;

const
  // indexes for things in the ActionList1
  { Fixed positional indexes of actions within ActionList1, and the position of
    the "Advanced Options" item inside the dynamically-built Calculate submenu
    for the AMZ and PVL screens (AMZADVANCED_MENU_INDEX / PVLADVANCED_MENU_INDEX). }
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
    { Build/tear-down the whole app shell (log, help, clipboard, fonts, prefs). }
    constructor Create(AOwner: TComponent); override;
    destructor Destroy(); override;
    { Called when an MDI child becomes active: install its Calculate submenu. }
    procedure ChildActivating( Child: TObject );
    { Called when an MDI child closes: strip the screen-specific Calculate menus. }
    procedure ChildClosing( Child: TObject );
    { Set the bottom status-bar text (used for per-cell help and messages). }
    procedure SetStatusBarText( TheText: string );
  protected
    { UI appearance state, shared out to every screen via SetUISettings: }
    m_CellFont: TFont;                  { font for normal (input) cells }
    m_CellBackgroundColor: TColor;      { background for normal cells }
    m_OutpCellFont: TFont;              { font for calculated-output cells }
    m_OutpCellBackgroundColor: TColor;  { background for output cells (money green) }
    m_SelectedCellColor: TColor;        { highlight colour for the selection }
    m_BasePath: string;                 { app directory (for help/example file paths) }
    { Help "example" navigated: load the example doc into the matching screen. }
    procedure OnHelpNavigationComplete( CurrentFile: string );
    { Help window closed: restore each screen's real (pre-example) data. }
    procedure OnHelpSystemClosed();
    { WM_NOTIFY handler -- forwards help-control notifications to HelpSystem. }
    procedure OnNotify( var AMsg: TMessage); message WM_NOTIFY;
    { Trim the dynamic Calculate submenu back to its 2 static items. }
    procedure RemoveCalcMenus();
    { Rebuild the Calculate submenu for the Mortgage screen (Compare APRs, Generate Rows). }
    procedure CreateMortgageSubMenu();
    { Rebuild the Calculate submenu for the Amortization screen (Advanced, Table, APR). }
    procedure CreateAmortizationSubMenu();
    { Rebuild the Calculate submenu for the Present Value screen (Advanced, Table). }
    procedure CreatePresentValueSubMenu();
    { Lazily create + show the Mortgage screen singleton. }
    procedure AssureOpenMortgageScreen();
    { Lazily create + show the Amortization screen singleton. }
    procedure AssureOpenAmortizationScreen();
    { Lazily create + show the Present Value screen singleton. }
    procedure AssureOpenPresentValueScreen();
    { Load saved UI colours/fonts from HKCU\SOFTWARE\PerSense at startup. }
    procedure ReadRegistryInfo();
  end;

var
  MainForm: TMainForm;

implementation

{$R *.dfm}

uses about, WinProcs, UISettingsDlgUnit, ClipboardSettingsDlgUnit, peData,
     HelpSystemUnit, Globals, FileIOUnit, peTypes, ComputationalSettingsDlgUnit,
     APRReportScreenUnit, Registry;

{ Constructor (app startup): creates and initializes the master log; derives
  m_BasePath from the EXE path (everything up to the last backslash); creates the
  help system and wires its navigation/close callbacks; creates the clipboard
  helper and sets tab/CRLF delimiters; seeds default cell fonts and colours;
  marks the three screen singletons nil (lazy creation); then overlays saved
  prefs from the registry. Triggered once when the main form is constructed. }
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
  { strip the executable filename, leaving the install directory (with trailing '\') }
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

{ Destructor (app shutdown): frees the help system, all screen singletons, the
  clipboard helper and the cell fonts, then finally the master log. Order matters
  only in that MasterLog is freed last so earlier frees can still log. }
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

{ ReadRegistryInfo: load persisted UI appearance (cell/selected/output colours
  and the full input + output cell fonts) from HKEY_CURRENT_USER\SOFTWARE\PerSense.
  If the key is absent the constructor's hard-coded defaults remain. Read-only;
  fonts are populated field-by-field (style read as raw binary). }
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

{ FormClose: app-exit guard. For each open screen with unsaved data, prompt
  Yes/No/Cancel to save. Cancel aborts the close (Action := caNone); otherwise the
  app frees on close (Action := caFree). Triggered by attempting to close the main
  window / File>Exit. }
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

{ Help>About action: show the modal About box. }
procedure TMainForm.HelpAbout1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpAbout1Execute' );
  AboutBox.ShowModal();
end;

{ File>Exit action: initiate the main-form close (runs FormClose's save guard). }
procedure TMainForm.FileExit1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileExit1Execute' );
  Close();
end;

{ ChildActivating: when a child screen gains focus, swap in the Calculate
  submenu appropriate to its type (or strip the dynamic items for other windows).
  Triggered via each screen's OnActivate (wired in AssureOpen*Screen). }
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

{ ChildClosing: a child screen is closing -> remove its dynamic Calculate items. }
procedure TMainForm.ChildClosing( Child: TObject );
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ChildClosing' );
  RemoveCalcMenus();
end;

{ RemoveCalcMenus: delete dynamically-added Calculate submenu items, leaving the
  first 2 static items (the menu is rebuilt per active screen type). }
procedure TMainForm.RemoveCalcMenus();
begin
  while( Calculate1.Count > 2 ) do begin
    Calculate1.Delete( Calculate1.Count-1 );
  end;
end;

{ CreateMortgageSubMenu: build the Mortgage-specific Calculate submenu at
  runtime -- "Compare APRs" (Ctrl+A) and "Generate Mtg Rows" (Ctrl+T), each
  backed by a new TAction whose OnExecute points at the matching handler. }
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

{ CreateAmortizationSubMenu: build the Amortization-specific Calculate submenu --
  "Advanced Options" (Alt+X, a checkable toggle reflecting AdvancedIsOn), "Table
  Output" (Ctrl+T) and "APR Report" (Ctrl+A). The advanced item's Checked state
  is synced to the screen's current advanced flag. }
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

{ CreatePresentValueSubMenu: build the Present-Value-specific Calculate submenu --
  "Table Output" (Ctrl+T) and a checkable "Advanced Options" (Alt+X) synced to
  the PV screen's advanced flag. }
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

{ AssureOpenMortgageScreen: create the Mortgage screen singleton on first call
  (wiring its activate/close callbacks and pushing current UI settings), then
  bring it forward, show and un-minimize it. Idempotent. }
{ Go port: n/a (screen activation) -- the Mortgage screen's solve is
  internal/api/handlers.go:478 (HandleMortgageCalc); in the web port the panel
  is always present in cmd/persense/static/index.html, no lazy singleton. }
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

{ AssureOpenAmortizationScreen: create the Amortization screen singleton on first
  call (callbacks + UI settings), then bring forward, show, un-minimize. Idempotent. }
{ Go port: n/a (screen activation) -- the Amortization screen's solve is
  internal/api/handlers.go:752 (HandleAmortizationCalc). }
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

{ AssureOpenPresentValueScreen: create the Present Value screen singleton on
  first call (callbacks + UI settings), then bring forward, show, un-minimize.
  Idempotent. }
{ Go port: n/a (screen activation) -- the Present Value screen's solve is
  internal/api/handlers.go:1317 (HandlePVCalc). }
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
// The generic Edit/Calculate actions below all follow the same pattern: grab
// the active MDI child (a TMDIChild) and call its corresponding virtual On*()
// method, letting each screen implement the command its own way. This is the
// polymorphic command-dispatch backbone of the app.
//

{ File>Close action: close the active child window (if any). }
procedure TMainForm.FileClose1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.FileClose1Execute' );
  if( ActiveMDIChild <> nil ) then
    ActiveMDIChild.Close();
end;

{ Calculate action: run the active screen's OnCalculate (the main solve). }
procedure TMainForm.CalcCalculate1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.CalcCalculate1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnCalculate();
end;

{ Harden action: mark the selected calculated value(s) as fixed user input. }
procedure TMainForm.EditHarden1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditHarden1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnHardenValue();
end;

{ Undo action: delegate to active screen's OnUndo. }
procedure TMainForm.EditUndo1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditUndo1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnUndo();
end;

{ Redo action: delegate to active screen's OnRedo. }
procedure TMainForm.EditRedo1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditRedo1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnRedo();
end;

{ Delete action: delegate to active screen's OnDelete. }
procedure TMainForm.EditDelete1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditDelete1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnDelete();
end;

{ Cut action: delegate to active screen's OnCut. }
procedure TMainForm.EditCut1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditCut1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnCut();
end;

{ Copy action: delegate to active screen's OnCopy. }
procedure TMainForm.EditCopy1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditCopy1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnCopy();
end;

{ Paste action: delegate to active screen's OnPaste. }
procedure TMainForm.EditPaste1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.EditPaste1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnPaste();
end;

{ Settings>UI Options action: show the UI-settings dialog seeded with current
  colours/fonts; on OK, copy the chosen values back into the m_* fields and push
  them to every open screen via SetUISettings. (Persisted to registry elsewhere.) }
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

{ Settings>Computational action: edit the global computational defaults (df.c) --
  day-count basis, rounding, etc. -- via its dialog; commit on OK. These defaults
  feed the financial engine and are saved into every document file. }
{ Go port: internal/types/defaults.go:21 (DefaultCompDefaults) supplies df.c's
  equivalent; the settings are passed per-request to the api handlers rather than
  held in a global df.c. Dialog UI itself -> ComputationalSettingsDlgUnit / the
  frontend settings panel in cmd/persense/static/index.html. }
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

{ Settings>Clipboard action: edit the cell/row delimiters used when copying grid
  data to/from the clipboard. The dialog shows friendly tokens (tab, CR+LF)
  for the control characters; on OK those map back to the actual delimiter chars. }
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

{ Help>Contents action: open the help system at its default contents page. }
procedure TMainForm.HelpContents1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpContents1Execute' );
  WelcomeScreen.ReallyHide();
  HelpSystem.DisplayContents( '' );
end;

{ Help>Inline Examples action: open the help system at the examples overview. }
procedure TMainForm.HelpExamples1Execute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpExamples1Execute' );
  WelcomeScreen.ReallyHide();
  HelpSystem.DisplayContents( 'Example_Overview.html' );
end;

{ Calculate>Compare APRs (Mortgage only): verify the active child is the Mortgage
  screen, then run its CompareAPRs. Dynamic-menu handler wired in CreateMortgageSubMenu. }
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

{ Calculate>Generate Mtg Rows (Mortgage only): verify active child is Mortgage,
  then run its GenerateMtgRows (scenario row generation). }
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

{ Calculate>Advanced Options (Amortization): toggle the AMZ advanced-options
  panel and re-sync the menu item's Checked state to the screen flag. }
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

{ Calculate>Table Output (Amortization): generate the amortization schedule table. }
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

{ Calculate>APR Report (Amortization): lazily create the APR report screen, build
  its report from the current amortization, and show it. Aborts if CreateReport fails. }
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

{ Calculate>Advanced Options (Present Value): toggle the PV advanced panel and
  re-sync the menu item's Checked state. }
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

{ Calculate>Table Output (Present Value): generate the PV table for the active screen. }
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

{ File>Save action: save the active screen to its existing file name (empty name
  triggers a Save dialog inside SaveFile); update the window caption to the path. }
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
{ File>Save As action: like Save but passes an empty filename, forcing SaveFile
  to prompt for a new name regardless of the current one. }
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

{ File>Save All action: save every open MDI child to its own file, updating captions. }
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

{ File>Print action: show the print dialog, then delegate to the active screen's
  OnPrint to render itself to the chosen printer. }
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

{ Screens>Mortgage action: hide the welcome screen and open/show the Mortgage screen. }
procedure TMainForm.ScreenShowMortgageExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenShowMortgageExecute' );
  WelcomeScreen.ReallyHide();
  AssureOpenMortgageScreen();
end;

{ Screens>Amortization action: hide welcome screen and open/show Amortization. }
procedure TMainForm.ScreenShowAmortizationExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenShowAmortizationExecute' );
  WelcomeScreen.ReallyHide();
  AssureOpenAmortizationScreen();
end;

{ Screens>Present Value action: hide welcome screen and open/show Present Value. }
procedure TMainForm.ScreenShowPresentValueExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenShowPresentValueExecute' );
  WelcomeScreen.ReallyHide();
  AssureOpenPresentValueScreen();
end;

{ File>Open action: prompt for a file, load it via TFileIO, then route it by its
  block id (GetIDByte) to the matching screen's OpenFile -- MTGBlock->Mortgage,
  AMZTopBlock->Amortization, PVLlumpsumblock->Present Value -- creating the screen
  if needed and setting its caption. Unknown ids show an error. }
{ Go port: internal/api/import_psn.go:121 (HandleImportPSN) dispatches by block
  id (MTG/AMZ/PVL) to the matching payload builder, mirroring this GetIDByte
  routing; the low-level load is internal/fileio/loader.go. }
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
{ SetStatusBarText: set the bottom status-bar caption (per-cell help / messages). }
procedure TMainForm.SetStatusBarText( TheText: string );
begin
  StatusBar.SimpleText := TheText;
end;

// called for WM_NOTIFY messages.  Hook into these to catch help system
//  messages
{ OnNotify: WM_NOTIFY message handler. When the notification carries the help
  system's id, forward the embedded HTML-Help notify struct to HelpSystem so it
  can react to navigation events (which feed OnHelpNavigationComplete). }
procedure TMainForm.OnNotify( var AMsg: TMessage );
begin
  if( AMsg.WParam = NOTIFY_MESSAGE_ID ) then begin
    HelpSystem.CustomOnNotifyHandler( PHH_NOTIFY(AMsg.LParam) );
  end;
end;

{ called when someone navigates to a new page in the help system }
{ OnHelpNavigationComplete: the heart of the "live examples" help feature.
  When the user navigates to a help page named like "/MS_Example3.html", this
  decodes the two-letter screen prefix (MS=Mortgage, AM=Amortization, PV=Present
  Value) and the trailing example number to build a help-code lookup. If that
  yields an example data file, it backs up the user's current screen data (once)
  and loads the example into the screen so the user sees a worked scenario. When
  navigation leaves the example pages (no matching code), it restores each
  screen's backed-up real data. Param: CurrentFile = the navigated help URL.
  NOTE: the example-number parse uses fixed copy() offsets (12, etc.) and a
  try/except around StrToInt; it assumes the strict "/??_Example<n>.html" shape. }
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
  ScreenTypeString := copy( CurrentFile, 2, 2 );   { the "MS"/"AM"/"PV" prefix }
  DotPosition := Pos( '.', CurrentFile );           { locate the ".html" }
  if( DotPosition <> -1 ) then begin
    { build the help code: screen-mask (IM/IA/IP) OR'd with the example number }
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
    { An example data file maps to this page -> back up the user's real data the
      first time, then load the example file into the appropriate screen. }
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
    { Not an example page -> restore any screen that currently holds example data. }
    if( MortgageScreen <> nil ) then
      if( MortgageScreen.IsBackedUpForHelp() ) then MortgageScreen.RestoreFromHelpSystem();
    if( AmortizationScreen <> nil ) then
      if( AmortizationScreen.IsBackedUpForHelp() ) then AmortizationScreen.RestoreFromHelpSystem();
    if( PresentValueScreen <> nil ) then
      if( PresentValueScreen.IsBackedUpForHelp() ) then PresentValueScreen.RestoreFromHelpSystem();
  end;
  TheFile.Free();
end;

{ OnHelpSystemClosed: when the help window closes, restore every screen that is
  still showing backed-up example data, so the user gets their real data back. }
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

{ Help>Contextual Help action: ask the active screen to open its own overview
  help page (each screen's OnContextualHelp names its topic). }
procedure TMainForm.HelpContextualHelp1Execute(Sender: TObject);
var
  Child: TMDIChild;
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.HelpContextualHelp1Execute' );
  Child := TMDIChild(ActiveMDIChild);
  if( Child <> nil ) then
    Child.OnContextualHelp();
end;

{ Screens>Welcome action: re-show and focus the welcome/splash screen. }
procedure TMainForm.ScreenWelcomeScreenExecute(Sender: TObject);
begin
  MasterLog.Write( LVL_LOW, 'TMainForm.ScreenWelcomeScreenExecute' );
  WelcomeScreen.ReallyShow();
  WelcomeScreen.BringToFront();
  WelcomeScreen.WindowState := wsNormal;
end;

end.
