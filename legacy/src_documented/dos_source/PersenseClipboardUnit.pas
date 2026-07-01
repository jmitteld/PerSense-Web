{ ===========================================================================
  Unit:  PersenseClipboardUnit
  Role:  Clipboard abstraction that serialises grid cells to/from the Windows
         clipboard WHILE preserving Per%Sense-only per-cell metadata.

  Why an abstraction: the grids carry extra per-cell "hardness" info (whether a
  value is user-entered/locked vs computed) that has no place in the plain-text
  CF_TEXT clipboard. On Copy this class (1) builds a delimited text blob (cells
  joined by m_CellDelimiter, rows by m_RowDelimiter, RowWidth cells/row) and
  puts it on the real Windows clipboard, and (2) stashes a private shadow copy
  of the strings plus the CellInfo hardness list. On Paste it parses the
  clipboard text back into cells; if that text exactly matches the shadowed
  copy from the last Copy, it knows the data round-tripped from THIS app and
  also restores the hardness metadata; otherwise only the text is returned.

  Delimiters are configurable (see ClipboardSettingsDlgUnit) via the Set/Get
  CellDelimiter / RowDelimiter accessors.

  { Go port: n/a -- Windows clipboard abstraction (CF_TEXT + private hardness
    shadow); no financial logic. In the web port, grid copy/paste is handled by
    the browser clipboard API in cmd/persense/static/index.html; the cell
    "hardness" (input vs computed) is carried in the JSON response fields, not a
    parallel clipboard shadow. }
  =========================================================================== }
unit PersenseClipboardUnit;

interface

uses
  Classes, Clipbrd, Globals, SysUtils, Windows, LogUnit, peTypes;
  
type
  { This object handles the clipboard.  Reason we need an abstraction layer is that
    Persense has extra data about the hardness of data, which can't really get to the
    windows clipboard, but needs to be preserved for internal cutting and pasting.  So
    this object stores the extra data and recalls it when necessary }
  TPersenseClipboard = class
  public
    { Allocate the internal shadow string/info stores. }
    constructor Create();
    { Free the internal shadow stores. }
    destructor Destroy(); override;
    { Serialise cells to the Windows clipboard and stash hardness internally. }
    function Copy( Values: TStringList; Info: TList; RowWidth: integer ) : boolean;
    { Parse clipboard text into cells; restore hardness if it round-tripped. }
    function Paste( var Values: TStringList; var Info: TList; var RowWidth: integer ) : boolean;
    { Configure the between-cells delimiter. }
    procedure SetCellDelimiter( CellDelimiter: string );
    { Configure the between-rows delimiter. }
    procedure SetRowDelimiter( RowDelimiter: string );
    { Read the current cell delimiter. }
    function GetCellDelimiter(): string;
    { Read the current row delimiter. }
    function GetRowDelimiter(): string;
  protected
    { m_CellDelimiter = between-cells string; m_RowDelimiter = between-rows. }
    m_CellDelimiter: string;
    m_RowDelimiter: string;
  private
    { m_Strings/m_Info = shadow copy of last Copy (text + CellInfo hardness);
      m_RowWidth = cells-per-row of that copy (used to detect a round-trip). }
    m_Strings: TStringList;
    m_RowWidth: integer;
    m_Info: TList;
  end;

var
  { global clipboard helper instance }
  PersenseClipboard: TPersenseClipboard;

implementation

uses PersenseGrid;

{ Create - allocate the internal shadow stores used for hardness round-trip. }
constructor TPersenseClipboard.Create();
begin
  m_Strings := TStringList.Create();
  m_Info := TList.Create();
end;

{ Destroy - free the internal shadow stores. }
destructor TPersenseClipboard.Destroy();
begin
  m_Strings.Free();
  m_Info.Free();
end;

{ SetCellDelimiter - set the between-cells delimiter used by Copy/Paste. }
procedure TPersenseClipboard.SetCellDelimiter( CellDelimiter: string );
begin
  m_CellDelimiter := CellDelimiter;
end;

{ SetRowDelimiter - set the between-rows delimiter used by Copy/Paste. }
procedure TPersenseClipboard.SetRowDelimiter( RowDelimiter: string );
begin
  m_RowDelimiter := RowDelimiter;
end;

{ GetCellDelimiter - current between-cells delimiter. }
function TPersenseClipboard.GetCellDelimiter(): string;
begin
  GetCellDelimiter := m_CellDelimiter;
end;

{ GetRowDelimiter - current between-rows delimiter. }
function TPersenseClipboard.GetRowDelimiter(): string;
begin
  GetRowDelimiter := m_RowDelimiter;
end;

{ Copy
  Purpose: serialise a flat row-major list of cell strings into one delimited
           text blob, place it on the Windows clipboard, AND keep an internal
           shadow copy of the cells plus their hardness for a later same-app
           paste.
  Params:  Values=cell texts (row-major); Info=parallel ^CellInfo list;
           RowWidth=cells per row (marks row boundaries).
  Returns: false on error (RowWidth=0 would divide by zero), true on success.
  Side effects: clears+repopulates m_Strings/m_Info; allocates a temp PChar
                buffer (freed in finally); calls Clipboard.SetTextBuf.
  Layout: cell+m_CellDelimiter within a row; last cell of each row uses
          m_RowDelimiter; a trailing row delimiter is trimmed to #0. }
{ Copies to the clipboard.  Returns false on error }
function TPersenseClipboard.Copy( Values: TStringList; Info: TList; RowWidth: integer ) : boolean;
var
  pBuffer, ptr: PChar;
  i: integer;
  BufLength: integer;
  pCellInfo: ^CellInfo;
  pNewCellInfo: ^CellInfo;
begin
  if( RowWidth = 0 ) then begin
    MasterLog.Write( LVL_MED, 'Copy: Can''t divide by zero' );
    Copy := false;
    exit;
  end;
  pBuffer := nil; {just to remove useless warning }
  { erase internal stored data }
  m_Strings.Clear();
  m_Info.Clear();
  m_RowWidth := RowWidth;
  try
    { Find the length necessary for the character buffer }
    BufLength := 1;
    for i:=0 to Values.Count-1 do begin
      if( ((i+1) mod RowWidth) = 0 ) then
        BufLength := BufLength + Length(Values[i]) + length(m_RowDelimiter)
      else
        BufLength := BufLength + Length(Values[i]) + length(m_CellDelimiter);
      m_Strings.Add( Values[i] );
      pCellInfo := Info[i];
      new( pNewCellInfo );
      pNewCellInfo.Hardness := pCellInfo.Hardness;
      m_Info.Add( pNewCellInfo );
    end;
    GetMem( pBuffer, BufLength );
    pBuffer^ := #0;
    ptr := pBuffer;
    { Fill the buffer with the strings }
    for i:=0 to m_Strings.Count-1 do begin
      if( ((i+1) mod RowWidth) = 0 ) then
        ptr := StrEnd( StrPCopy(ptr, m_Strings[i]+m_RowDelimiter) )
      else
        ptr := StrEnd( StrPCopy(ptr, m_Strings[i]+m_CellDelimiter) );
    end;
    ptr := ptr-length(m_RowDelimiter);
    ptr^ := #0;
    Clipboard.SetTextBuf( pBuffer );
  finally
    FreeMem( pBuffer );
  end;
  Copy := true;
end;

{ Paste
  Purpose: read CF_TEXT from the Windows clipboard, split it back into cells
           using the configured delimiters, and - if it exactly matches the
           internally shadowed copy from the last Copy - also restore the
           per-cell hardness Info.
  Params (out): Values=parsed cell texts; Info=per-cell ^CellInfo, populated
                ONLY on an exact match with the shadow copy; RowWidth=inferred
                cells in the first row.
  Returns: false if the clipboard holds no text or can't be locked; else true.
  Side effects: locks/unlocks the global clipboard handle; allocates CellInfo
                records on a match.
  NOTE: Token is a fixed 256-char buffer; longer cells are truncated. }
function TPersenseClipboard.Paste( var Values: TStringList; var Info: TList; var RowWidth: integer ) : boolean;
var
  pBuffer, pRowDelimiter, pCellDelimiter, ptr, pEnd: PChar;
  Data: THandle;
  Token: array [0..255] of char;
  i: integer;
  Match: boolean;
  pCellInfo, pNewCellInfo: ^CellInfo;
  Width: integer;
begin
  if( not Clipboard.HasFormat(CF_TEXT) ) then begin
    MasterLog.Write( LVL_MED, 'Paste: Trying to paste non text data' );
    Paste := false;
    exit;
  end;
  Clipboard.Open();
  Data := GetClipboardData( CF_TEXT );
  try
    if Data<>0 then
      pBuffer := PChar(GlobalLock(Data))
    else
      pBuffer := nil
  finally
    if( Data<>0 )then
      GlobalUnlock(Data);
    ClipBoard.Close();
  end;
  if( pBuffer = nil ) then begin
    MasterLog.Write( LVL_MED, 'Paste: couldn''t lock Data' );
    Paste := false;
    exit;
  end;
  { Now that we have the buffer, let's go ahead and parse out the data }
  pRowDelimiter := pBuffer;             { initialize start }
  pEnd := StrScan( pBuffer, #0 );       { initialize end }
  Width := 0;
  RowWidth := 0;
  repeat
    ptr := pRowDelimiter;
    pRowDelimiter := StrPos( pBuffer, PChar(m_RowDelimiter) );
    if( pRowDelimiter=nil ) then
      pRowDelimiter := pEnd;
    while( ptr <> pRowDelimiter ) do begin
      pCellDelimiter := StrPos( pBuffer, PChar(m_CellDelimiter) );
      if( (pCellDelimiter>pRowDelimiter) or (pCellDelimiter=nil) ) then
        pCellDelimiter := pRowDelimiter;
      StrLCopy( Token, pBuffer, pCellDelimiter-pBuffer );
      { Token now contains the strings, let's paste it in }
      Values.Add( Token );
      ptr := pCellDelimiter;
      pBuffer := pCellDelimiter+length(m_CellDelimiter);
      Inc(Width);
    end;
    pBuffer := pBuffer+length(m_RowDelimiter)-length(m_CellDelimiter); {remove the row delimiter }
    if( RowWidth = 0 ) then
      RowWidth := Width;
  until pRowDelimiter=pEnd;
  { Okay, Values now has all the strings from the buffer.  Let's compare it against
    the stored values and if they're equal set up the info array }
  Match := true;
  if( (Values.Count = m_Strings.Count) and (m_RowWidth = RowWidth) ) then begin
    for i:=0 to Values.Count-1 do begin
      if( Values[i] <> m_Strings[i] ) then
        Match := false;
    end;
  end else
    Match := false;
  if( Match ) then begin
    for i:=0 to m_Info.Count-1 do begin
      pCellInfo := m_Info[i];
      new( pNewCellInfo );
      pNewCellInfo.Hardness := pCellInfo.Hardness;
      Info.Add( pNewCellInfo );
    end;
  end;
  Paste := true;
end;

end.
