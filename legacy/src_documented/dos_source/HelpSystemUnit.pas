{ ==========================================================================
  HelpSystemUnit.pas

  PURPOSE / ROLE
    The application's context-help subsystem for the Windows port. It does two
    related jobs:
      1. Loads a flat help-text database (Help/Help.txt) keyed by 32-bit help
         codes, and answers GetHelpString(code) lookups used throughout the UI
         (status-bar text, dialog help, inline examples).
      2. Drives the compiled HTML Help (.chm) viewer via HHCTRL.OCX, including
         a sneaky window-proc subclass so it can detect when the externally
         owned help window is closed and fire navigation/close callbacks.

  HELP-CODE SCHEME (see the long const list below)
    Codes are $SSTTNNNN-style. The high byte selects help KIND
      $00 = status-bar text, $01 = inline example HTML,
      $02 = dialog/message-box help HTML, $04 = contextual help HTML.
    The next byte selects the SCREEN
      $00 Mortgage, $01 Amortization, $02 Present Value, $04 Other.
    The low bytes enumerate the specific field/dialog. $00000000 is reserved.

  KEY TYPES
    THelpSystem      - the live help object (singleton `HelpSystem`).
    HelpStrings(Ptr) - one code->string record stored in m_HelpStrings.
    HH_WINTYPE/HH_NOTIFY/NMHDR - HTML Help API structures.
    THtmlHelpA       - imported HtmlHelpA function pointer from HHCTRL.OCX.

  { Go port: n/a -- context-help database + Windows HTML-Help (.chm) viewer
    subsystem; no financial logic. In the web port, help/example text is
    rendered inline in cmd/persense/static/index.html (status-bar tooltips,
    example presets, contextual panels) and the legacy Help/*.html files remain
    a READ-ONLY reference. The HHCTRL.OCX subclassing and code->string lookup
    have no server-side equivalent. Applies to every method below. }
  ========================================================================== }
unit HelpSystemUnit;

interface

uses
  Windows, HHCTRLLib_TLB, LogUnit, Globals, SysUtils, Messages, Classes;

const
  // the following is a LONG list of all the help codes used in Persense.
  // these codes are assigned in the Help/Help.txt file.  The file has a
  // simple format:
  // $00000000help message
  // where the first 9 chars are the help code and the rest of the text until
  // the <CRLF> is the help message.
  // I assign some special codes to the first byte:
  // 0x00 = Status bar help message
  // 0x01 = Inline example HTML string for help system
  // 0x02 = Dialog or Message box help HTML string for help system
  // 0x04 = Contextual help HTML string for help system
  // second byte specifies which screen
  // 0x00 = Mortgage screen
  // 0x01 = Amortization screen
  // 0x02 = Present Value screen
  // 0x04 = Other screen
  //
  // THE CODE $00000000 IS RESERVED!  DO NOT ASSIGN!
  //
  HELP_NULL                     = $00000000;
  // Status bar help messages
  // mortgage screen status bar messages
  SM_PriceCol                   = $00000001;
  SM_PointsCol                  = $00000002;
  SM_PercentDownCol             = $00000003;
  SM_CashRequiredCol            = $00000004;
  SM_AmountBorrowedCol          = $00000005;
  SM_YearsCol                   = $00000006;
  SM_LoanRateCol                = $00000007;
  SM_MonthlyTaxCol              = $00000008;
  SM_MonthlyTotalCol            = $00000009;
  SM_BalloonYearsCol            = $0000000a;
  SM_BalloonAmountCol           = $0000000b;
  // Amortization screen status bar messages
  SA_AMZAmountCol               = $00010001;
  SA_AMZLoanDateCol             = $00010002;
  SA_AMZLoanRateCol             = $00010003;
  SA_AMZFirstDateCol            = $00010004;
  SA_AMZNPeriodsCol             = $00010005;
  SA_AMZLastDateCol             = $00010006;
  SA_AMZPerYrCol                = $00010007;
  SA_AMZPayAmtCol               = $00010008;
  SA_AMZPointsCol               = $00010009;
  SA_AMZAPRCol                  = $0001000a;
  SA_PREStartCol                = $0001000b;
  SA_PRENNCol                   = $0001000c;
  SA_PREStopCol                 = $0001000d;
  SA_PREPerYrCol                = $0001000e;
  SA_PREPaymentCol              = $0001000f;
  SA_BALDateCol                 = $00010010;
  SA_BALAmountCol               = $00010011;
  SA_ADJDateCol                 = $00010012;
  SA_ADJRateCol                 = $00010013;
  SA_ADJAmountCol               = $00010014;
  SA_Moratorium                 = $00010015;
  SA_Target                     = $00010016;
  SA_Skip                       = $00010017;
  SA_OFFDateCol                 = $00010018;
  SA_OFFAmountCol               = $00010019;
  // Present value status bar text
  SP_LSMDateCol                 = $00020001;
  SP_LSMAmountCol               = $00020002;
  SP_LSMValueCol                = $00020003;
  SP_PERFromDateCol             = $00020004;
  SP_PERToDateCol               = $00020005;
  SP_PERPerYrCol                = $00020006;
  SP_PERAmountCol               = $00020007;
  SP_PERCOLACol                 = $00020008;
  SP_PERValueCol                = $00020009;
  SP_PRVAsOfCol                 = $0002000a;
  SP_PRVTrueRateCol             = $0002000b;
  SP_PRVLoanRateCol             = $0002000c;
  SP_PRVYieldCol                = $0002000d;
  SP_PRVValueCol                = $0002000e;
  SP_RTLDateCol                 = $0002000f;
  SP_RTLTrueRateCol             = $00020011;
  SP_RTLLoanRateCol             = $00020012;
  SP_RTLYieldCol                = $00020013;
  SP_XPRAsOfCol                 = $00020014;
  SP_XPRComputationCol          = $00020015;
  SP_XPRValueCol                = $00020016;
  // inline help HTML strings
  // last 3 bytes is the example number that's being loaded
  // Mortgage HTML strings
  IM_MASK                       = $01000000;
  // Amortization HTML strings
  IA_MASK                       = $01010000;
  // Present Value HTML strings
  IP_MASK                       = $01020000;
  // dialog box help strings
  // mortgage
  DM_YearsNegative              = $02000001;
  DM_SpecifyBalloonPayment      = $02000002;
  DM_AmountBorrowedExceedsPrice = $02000003;
  DM_PriceTooSmall              = $02000004;
  DM_LeaveSomeBlank             = $02000005;
  DM_FillPercentOrCash          = $02000006;
  DM_APRDidNotConverge          = $02000007;
  DM_InsufficientDataIn2nd      = $02000008;
  DM_CrossoverDidNotConverge    = $02000009;
  DM_1OrMore4APR                = $0200000a;
  DA_FillLinesForComparison     = $0200000b;
  DM_NotNEoughDataForAPR        = $0200000c;
  DA_FillRowForComparison       = $0200000d;
  DM_CalcErrorsForGenerate      = $0200000e;
  DM_RowCountExceeded           = $0200000f;
  DM_GeneratedRowError          = $02000010;
  // amortization
  DA_LastPayBeforeFirst         = $02010001;
  DA_ChangeTo365                = $02010002;
  DA_InterestTooSmall           = $02010003;
  DA_ZeroRateLoan               = $02010004;
  DA_DurationIsNegative         = $02010005;
  DA_RateTooSmall               = $02010006;
  DA_TerminatingBalloonChanged  = $02010007;
  DA_LoanBalanceBeforeDate      = $02010008;
  DA_NotEnoughDataForTable      = $02010009;
  DA_2PaymentsMin               = $0201000a;
  DA_DateOutOfOrder             = $0201000b;
  DA_BalloonPrecedesRepay       = $0201000c;
  DA_PrincPrecedesFirstPay      = $0201000d;
  DA_NoAdvanceMsg               = $0201000e;
  DA_PrincPrecedeLastPay        = $0201000f;
  DA_PrincipalReductionTooHigh  = $02010010;
  DA_BorrowedUsingReduction     = $02010011;
  DA_APRNoConverge              = $02010012;
  DA_BalloonPrecedeFirstPay     = $02010013;
  DA_2RateAdjustsPerDay         = $02010014;
  DA_RateChangePrecedeDate      = $02010015;
  DA_RateChangeAfterPay         = $02010016;
  DA_InternalError              = $02010017;
  DA_PaymentTooSmall            = $02010018;
  DA_PayOrInterestNoConverge    = $02010019;
  DA_UnusuallyHighRate          = $0201001a;
  DA_SetBalloonIncludesToNo     = $0201001b;
  DA_SetBalloonIncPmt           = $0201001c;
  DA_OverwriteFile              = $0201001d;
  // present value
  DP_OutOfMemory                = $02020001;
  DP_NoMemoryForTable           = $02020002;
  DP_DateAmountNoValue          = $02020003;
  DP_1Line2Unknowns             = $02020004;
  DP_TooMuchInPaymentBlock      = $02020005;
  DP_PaymentInfinite            = $02020006;
  DP_1Line2UnknownsUpperRight   = $02020007;
  DP_Only2Of3InPayment          = $02020008;
  DP_DatesOutOfOrder            = $02020009;
  DP_RateNotDetermind           = $0202000a;
  DP_RateDidNotConverge         = $0202000b;
  DP_InterestTooSmall           = $0202000c;
  DP_ComputationNoConvergeBy    = $0202000d;
  DP_PositiveNegativeMessage    = $0202000e;
  DP_DateNotConvergeBy          = $0202000f;
  DP_DateOrAmountInSingle       = $02020010;
  DP_DateOrAmountInPeriodic     = $02020011;
  DP_InsufficientDataForTable   = $02020012;
  DP_TooManyUnknowns            = $02020013;
  DP_FromLaterThenThrough       = $02020014;
  DP_1Line2UnknownsUpperBlock   = $02020015;
  DP_1Line2UnknownsTopRight     = $02020016;
  DP_OutOfOrderRateList         = $02020017;
  DP_RedeterminedValue          = $02020018;
  DP_ReDeterminedData           = $02020019;
  DP_OverwriteFile              = $0202001a;
  // other
  DO_OutOfMemory                = $02040001;
  DO_TimeTooLong                = $02040002;
  DO_FindBlockNotFound          = $02040003;
  DO_SqrrtTiny                  = $02040004;
  DO_ExxpOverflow               = $02040005;
  DO_LnnNegative                = $02040006;
  DO_OpeningFile                = $02040007;
  DO_InvalidFile                = $02040008;
  DO_UnkownFileType             = $02040009;
  DO_LoanAmountMustBePositive   = $0204000a;
  DO_InsufficentDataForAPR      = $0204000b;
  DO_APRDidNotConverge          = $0204000c;
  DO_OverwriteFile              = $0204000d;
  DO_UnsavedMortgage            = $0204000e;
  DO_UnsavedAmortization        = $0204000f;
  DO_UnsavedPresentValue        = $02040010;
  // end of help codes

  // HTML Help (HHCTRL.OCX) command codes passed as the uCommand to HtmlHelpA.
  HH_INITIALIZE                 = $001C;  // start the help system, returns a cookie
  HH_UNINITIALIZE               = $001D;  // shut the help system down
  HH_DISPLAY_TOC                = $0001;  // open the .chm at its table of contents
  HH_GET_WIN_TYPE               = $0005;  // retrieve the window-type struct (to set idNotify)
  HH_CLOSE_ALL                  = $0012;  // close all open help windows

  // HTML Help notification codes (negative WM_NOTIFY codes from the viewer).
  HHN_FIRST                     = (0-860);
  HHN_LAST                      = (0-879);
  HHN_NAVCOMPLETE               = (HHN_FIRST-0);  // user navigated to a new topic
  HHN_TRACK                     = (HHN_FIRST-1);
  HHN_WINDOW_CREATE             = (HHN_FIRST-2);

  NOTIFY_MESSAGE_ID             = $6969;  // our chosen idNotify so the viewer routes WM_NOTIFY to us

type
  { necessary types for taking control of the message loop }
  WParameter = LongInt;   // alias for a WPARAM-sized message argument
  LParameter = LongInt;   // alias for an LPARAM-sized message argument

  { procedure type for HTMLHelp calls }
  // Signature of the HtmlHelpA entry point imported at run time from HHCTRL.OCX.
  THtmlHelpA = function(hwndCaller: THandle; pszFile: pchar; uCommand: cardinal; dwData: longint): THandle; stdCall;

  // Callback fired after the viewer finishes navigating (carries the topic file).
  TNavigationComplete = procedure( CurrentFile: string ) of object;
  // Callback fired when the help window is detected closing.
  THelpSystemClosed = procedure() of object;

  { used for getting the WinType, which is needed to enable the notification system }
  HH_WINTYPE = record
    cbStruct                    : integer;
    fUniCodeStrings             : boolean;
    pszType                     : pchar;
    fsValidMembers              : DWORD;
    fsWinProperties             : DWORD;
    pszCaption                  : pchar;
    dwStyles                    : dword;
    dwExStyles                  : dword;
    rcWindowPos                 : TRect;
    nShowState                  : integer;
    hwndHelp                    : HWND;
    hwndCaller                  : HWND;
    paInfoTypes                 : ^DWORD;
    hwndToolBar                 : HWND;
    hwndNavigation              : HWND;
    hwndHTML                    : HWND;
    iNavWidth                   : integer;
    rcHTML                      : TRect;
    pszToc                      : pchar;
    pszIndex                    : pchar;
    pszFile                     : pchar;
    pszHome                     : pchar;
    fsToolBarFlags              : DWORD;
    fNotExpanded                : boolean;
    curNavType                  : integer;
    TabPosition                 : integer;
    idNotify                    : integer;
    TabOrder                    : packed array [0..19] of byte;
    cHistory                    : integer;
    pszJump1                    : pchar;
    pszJump2                    : pchar;
    pszUrlJump1                 : pchar;
    pszUrlJump2                 : pchar;
    rcMinSize                   : TRect;
    cbInfoType                  : integer;
    pszCustomTabs               : pchar;
  end;

  { standard structure sent in WM_NOTIFY messages.  Used
    for notifications sent from the help system }
  NMHDR = record
    hwndFrom: HWND;
    idFrom: integer;
    code: integer;
  end;

  { structure for notify messages }
  HH_NOTIFY = record
    Header: NMHDR;
    pszURL: pchar;
  end;
  PHH_NOTIFY = ^HH_NOTIFY;

  // a record of codes to strings
  // One entry in the in-memory help database: a numeric code and its text.
  HelpStrings = record
    HelpCode: integer;
    HelpString: string;
  end;
  HelpStringsPtr = ^HelpStrings;

  { the actual usable class }
  // THelpSystem: owns the help-string table and the HTML Help viewer.
  // Public:  Create/Destroy lifecycle, DisplayContents (open .chm),
  //          the two custom notify/close handlers, and GetHelpString lookup.
  // Private: imported HtmlHelpA, OCX module handle, parent window, init cookie,
  //          base path, the TList of HelpStringsPtr records, and the file-parsing
  //          helpers. Published properties expose the navigation/close callbacks.
  THelpSystem = class
  public
    // Construct: load Help.txt and initialize the HTML Help OCX.
    constructor Create( WinHandle: HWND; BasePath: string );
    // Destroy: close/uninit the OCX, free the library, free all help records.
    destructor Destroy(); override;
    // Open the .chm viewer (optionally at a given topic Location).
    procedure DisplayContents( Location: string );
    // Handle a WM_NOTIFY originating from the help viewer (navigation events).
    procedure CustomOnNotifyHandler( pInfo: PHH_NOTIFY );
    // Handle the subclassed WM_CLOSE for the help window.
    procedure CustomOnCloseHandler();
    // Look up the help text for a given code ('' if not found).
    function GetHelpString( HelpCode: integer ): string;
  private
    HtmlHelpA: THtmlHelpA;          // imported viewer entry point
    m_hHTMLHelpSystem: THandle;     // loaded HHCTRL.OCX module handle
    m_hParent: HWND;                // owner window for help calls
    m_Cookie: DWORD;                // init cookie returned by HH_INITIALIZE
    m_BasePath: string;             // app base path (used to locate the .chm)
    m_HelpStrings: TList;           // list of HelpStringsPtr (the code->text DB)
    FOnNavigationComplete: TNavigationComplete;
    FOnHelpSystemClosed: THelpSystemClosed;
    // Extract the topic file name following "::" in a help URL.
    function GetFilePart( URL: string ): string;
    // Load and parse the whole Help.txt file into m_HelpStrings.
    procedure ReadHelpStrings( FileName: string );
    // Read one CR-terminated line from the help-text stream.
    function ReadLineFrom( InputFile: TFileStream ): string;
    // Split a raw help line into its 9-char numeric code and message text.
    procedure ParseHelpLine( InputString: string; var HelpCode: integer; var HelpString: string );
  published
    property OnNavigationComplete: TNavigationComplete read FOnNavigationComplete write FOnNavigationComplete;
    property OnHelpSystemClosed: THelpSystemClosed read FOnHelpSystemClosed write FOnHelpSystemClosed;
  end;

var
  HelpSystem: THelpSystem;     // application-wide singleton instance
  OldHelpWindowProc: Pointer;  // saved original window proc of the help window (for subclassing)

implementation

uses Forms, ExtCtrls, Dialogs;

{ NewHelpWindowProc
  PURPOSE: replacement window procedure installed over the help viewer window so
           we can observe its WM_CLOSE (the viewer never notifies us otherwise).
  PARAMS:  standard Win32 wndproc params.
  RETURNS: result of chaining to the original proc (CallWindowProc).
  SIDE EFFECTS: on WM_CLOSE, invokes HelpSystem.CustomOnCloseHandler.
  INTENT: subclassing hook; always forwards the message to OldHelpWindowProc. }
{ when the help system window opens I don't have direct control over it.  It
  sends me messages for Navigation stuff, but it doesn't send me any message
  when it closes.  So I take over its window proc and watch for the WM_CLOSE event }
function NewHelpWindowProc( WindowHandle : hWnd; TheMessage : WParameter; ParamW : WParameter; ParamL : LParameter) : LongInt stdcall;
begin
  if( TheMessage = WM_CLOSE ) then
    HelpSystem.CustomOnCloseHandler();

  { pass the message on to the original windows proc }
  NewHelpWindowProc := CallWindowProc(OldHelpWindowProc, WindowHandle, TheMessage, ParamW, ParamL);
end;

{ THelpSystem.Create
  PURPOSE: construct the help system - load the help-string DB and bring up the
           HTML Help OCX.
  PARAMS:  WinHandle - owner window; BasePath - app directory (to find .chm).
  SIDE EFFECTS: reads 'Help/Help.txt'; LoadLibrary('HHCTRL.OCX'); resolves and
           calls HtmlHelpA(HH_INITIALIZE). On any failure, shows an error dialog,
           logs it, and leaves the help system disabled (HtmlHelpA stays nil). }
constructor THelpSystem.Create( WinHandle: HWND; BasePath: string );
begin
  m_BasePath := BasePath;
  m_hParent := WinHandle;
  m_HelpStrings := TList.Create();
  ReadHelpStrings( 'Help/Help.txt' );
  try
    HtmlHelpA := nil;
    m_hHTMLHelpSystem := LoadLibrary('HHCTRL.OCX');
    if (m_hHTMLHelpSystem <> 0) then begin
      HtmlHelpA := GetProcAddress(m_hHTMLHelpSystem, 'HtmlHelpA');
      if( Assigned(HTMLHelpA) ) then begin
        HtmlHelpA( 0, nil, HH_INITIALIZE, dword(@m_Cookie) );
      end else begin
        MessageDlg( 'HTMLHelp function not found in hhctrl.ocx, help system disabled', mtError, [mbOK], 0 );
        MasterLog.Write( LVL_MED, 'THelpSystem::Create could not find HtmlHelpA in ocx');
      end;
    end else begin
      MessageDlg( 'hhctrl.ocx not found, help system disabled', mtError, [mbOK], 0 );
      MasterLog.Write( LVL_MED, 'THelpSystem::Create could not find hhctrl.ocx');
    end;
  except
  end;
  OldHelpWindowProc := nil;
end;

{ THelpSystem.Destroy
  PURPOSE: tear down the help system.
  SIDE EFFECTS: clears the navigation callback; if the OCX loaded, closes all
           help windows and uninitializes; frees the OCX library; disposes every
           HelpStringsPtr record held in m_HelpStrings.
  NOTE: m_HelpStrings (the TList itself) is not explicitly Freed here. }
destructor THelpSystem.Destroy();
var
  i: integer;
  DeadRecord: HelpStringsPtr;
begin
  FOnNavigationComplete := nil;
  if( assigned( HTMLHelpA ) ) then begin
    HtmlHelpA( 0, nil, HH_CLOSE_ALL, 0 );
    HtmlHelpA( 0, nil, HH_UNINITIALIZE, dword(m_Cookie) );
  end;
  FreeLibrary( m_hHTMLHelpSystem );
  for i:=0 to m_HelpStrings.Count-1 do begin
    DeadRecord := m_HelpStrings.Items[i];
    Dispose( DeadRecord );
  end;
end;

{ THelpSystem.ReadHelpStrings
  PURPOSE: load the help-text database file into memory.
  PARAMS:  FileName - path to the help text (e.g. 'Help/Help.txt').
  SIDE EFFECTS: opens the file read-only; for each non-blank, non-'#'-comment
           line, allocates a HelpStrings record, parses it, and Adds it to
           m_HelpStrings. Stops at the first empty line / end of file. }
procedure THelpSystem.ReadHelpStrings( FileName: string );
var
  StringFile: TFileStream;
  OneLine: string;
  OneRecord: HelpStringsPtr;
begin
  OneLine := 'empty';
  StringFile := TFileStream.Create( FileName, fmOpenRead );
  while( OneLine <> '' ) do begin
    OneLine := ReadLineFrom( StringFile );
    if( OneLine <> '' ) then begin
      if( copy( OneLine, 0, 1 ) <> '#' ) then begin   // parse out the comments
        OneRecord := new( HelpStringsPtr );
        ParseHelpLine( OneLine, OneRecord.HelpCode, OneRecord.HelpString );
        m_HelpStrings.Add( OneRecord );
      end;
    end;
  end;
  StringFile.Free();
end;

{ THelpSystem.ReadLineFrom
  PURPOSE: read a single CR-terminated line from a byte stream.
  PARAMS:  InputFile - the open file stream.
  RETURNS: the line text without the terminator ('' at end of file).
  SIDE EFFECTS: advances the stream; consumes the trailing #10 (LF) after the
           #13 so the next read starts cleanly on the following line. }
function THelpSystem.ReadLineFrom( InputFile: TFileStream ): string;
var
  OneLetter: char;
  RetString: string;
begin
  ReadLineFrom := '';
  while( OneLetter <> #13 ) do begin
    if( InputFile.Read( OneLetter, 1 ) <> 1 ) then
      exit;
    if( OneLetter <> #13 ) then
      RetString := RetString + OneLetter;
  end;
  InputFile.Read( OneLetter, 1 ); // read out the #A
  ReadLineFrom := RetString;
end;

{ THelpSystem.ParseHelpLine
  PURPOSE: split one raw help-file line into its numeric code and message text.
  PARAMS:  InputString - the raw line; HelpCode (var) - parsed numeric code;
           HelpString (var) - the remaining message text.
  NOTE: the code is the first 9 characters; the message follows. The length
        math here (length-5) is a legacy off-by-some adjustment carried over
        from the original line format - preserved as-is. }
procedure THelpSystem.ParseHelpLine( InputString: string; var HelpCode: integer; var HelpString: string );
var
  CodeString: string;
begin
  CodeString := copy( InputString, 0, 9 );
  HelpCode := StrToInt( CodeString );
  HelpString := copy( InputString, 10, length(InputString)-5 );
end;

{ THelpSystem.GetHelpString
  PURPOSE: look up the help/status text registered for a help code.
  PARAMS:  HelpCode - the code to find.
  RETURNS: the matching help string, or '' if no record matches.
  NOTE: linear scan over m_HelpStrings. }
function THelpSystem.GetHelpString( HelpCode: integer ): string;
var
  i: integer;
  TheRecord: HelpStringsPtr;
begin
  GetHelpString := '';
  for i:=0 to m_HelpStrings.Count-1 do begin
    TheRecord := HelpStringsPtr(m_HelpStrings.Items[i]);
    if( TheRecord.HelpCode = HelpCode ) then begin
      GetHelpString := TheRecord.HelpString;
      exit;
    end;
  end;
end;

{ THelpSystem.CustomOnNotifyHandler
  PURPOSE: react to help-viewer WM_NOTIFY messages.
  PARAMS:  pInfo - pointer to the HH_NOTIFY payload.
  SIDE EFFECTS: on HHN_NAVCOMPLETE, extracts the topic file from the URL and
           fires OnNavigationComplete (if assigned). Used to sync interactive
           help examples with the topic currently shown. }
{ called when the controlling window gets a WM_NOTIFY that originated
  from the Help System }
procedure THelpSystem.CustomOnNotifyHandler( pInfo: PHH_NOTIFY );
var
  TheFile: string;
begin
  if( pInfo.Header.code = HHN_NAVCOMPLETE ) then begin
    Thefile := GetFilePart( pInfo.pszURL );
    if( assigned(FOnNavigationComplete) ) then
      OnNavigationComplete( TheFile );
  end;
end;

{ THelpSystem.CustomOnCloseHandler
  PURPOSE: notify the app that the help window has closed.
  SIDE EFFECTS: fires OnHelpSystemClosed (if assigned). Invoked from the
           subclassed NewHelpWindowProc on WM_CLOSE. }
{ Since I don't have direct access to the Help System window I
  took over its window proc (sneaky of me) and watch for the WM_CLOSE
  event.  When that happens this will get called }
procedure THelpSystem.CustomOnCloseHandler();
begin
  if( assigned(FOnHelpSystemClosed) ) then
    OnHelpSystemClosed();
end;

{ THelpSystem.DisplayContents
  PURPOSE: open the .chm help viewer at its table of contents (optionally at a
           specific topic), and arm the notification/subclass plumbing.
  PARAMS:  Location - topic file to show ('' = default home/TOC).
  SIDE EFFECTS: no-op (logged) if HtmlHelpA is unavailable. Otherwise calls
           HH_GET_WIN_TYPE to obtain the window-type struct, sets its idNotify
           to NOTIFY_MESSAGE_ID (and pszFile if a Location was given), opens
           the viewer via HH_DISPLAY_TOC, and on first open subclasses the help
           window with NewHelpWindowProc (saving the old proc in
           OldHelpWindowProc). Logs and bails on any HHCTRL failure. }
{ displays the contents page.  Also sets up callback stuff so we can do
  interactive example stuff }
procedure THelpSystem.DisplayContents( Location: string );
var
  pWinType: ^HH_WINTYPE;
  RetVal: THandle;
  hHelpWindow: HWND;
begin
  if( not assigned(HTMLHelpA) ) then begin
    MasterLog.Write( LVL_MED, 'DisplayContents: HTMLHelpA is not assigned, can not show help' );
    exit;
  end;
  RetVal := HTMLHelpA( m_hParent, PChar( m_BasePath + '\Help\PersenseHelp.chm>MainWin' ), HH_GET_WIN_TYPE, integer(@pWinType) );
  if( RetVal = $ffffffff ) then begin
    MasterLog.Write( LVL_MED, 'DisplayContents: Couldn''t call HH_GET_WIN_TYPE' );
    exit;
  end;
  pWinType.idNotify := NOTIFY_MESSAGE_ID;
  if( Location <> '' ) then
    pWinType.pszFile := pchar(Location);
  hHelpWindow := HTMLHelpA( m_hParent, PChar( m_BasePath + '\Help\PersenseHelp.chm>MainWin' ), HH_DISPLAY_TOC, 0 );
  if( hHelpWindow = 0 ) then begin
    MasterLog.Write( LVL_MED, 'DisplayContents: Couldn''t call HH_DISPLAY_TOC' );
    exit;
  end;
  if( OldHelpWindowProc = nil ) then
    OldHelpWindowProc := Pointer(SetWindowLong(hHelpWindow, GWL_WNDPROC, LongInt(@NewHelpWindowProc) ));
end;

{ THelpSystem.GetFilePart
  PURPOSE: pull the topic file name out of a help viewer URL.
  PARAMS:  URL - the full help URL (e.g. "...PersenseHelp.chm::/topic.htm").
  RETURNS: the substring after "::" ('' if no "::" present). }
{ The URL specified in the help system contains all sorts of wacky
  stuff.  The file I want is past the :: substring }
function THelpSystem.GetFilePart( URL: string ): string;
var
  Position: integer;
begin
  Position := Pos( '::', URL );
  if( Position = 0 ) then begin
    GetFilePart := '';
    exit;
  end;
  GetFilePart := copy( URL, Position+2, Length(URL)-Position );
end;

end.

