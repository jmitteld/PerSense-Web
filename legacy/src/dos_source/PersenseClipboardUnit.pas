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
    constructor Create();
    destructor Destroy(); override;
    function Copy( Values: TStringList; Info: TList; RowWidth: integer ) : boolean;
    function Paste( var Values: TStringList; var Info: TList; var RowWidth: integer ) : boolean;
    procedure SetCellDelimiter( CellDelimiter: string );
    procedure SetRowDelimiter( RowDelimiter: string );
    function GetCellDelimiter(): string;
    function GetRowDelimiter(): string;
  protected
    m_CellDelimiter: string;
    m_RowDelimiter: string;
  private
    m_Strings: TStringList;
    m_RowWidth: integer;
    m_Info: TList;
  end;

var
  PersenseClipboard: TPersenseClipboard;

implementation

uses PersenseGrid;

constructor TPersenseClipboard.Create();
begin
  m_Strings := TStringList.Create();
  m_Info := TList.Create();
end;

destructor TPersenseClipboard.Destroy();
begin
  m_Strings.Free();
  m_Info.Free();
end;

procedure TPersenseClipboard.SetCellDelimiter( CellDelimiter: string );
begin
  m_CellDelimiter := CellDelimiter;
end;

procedure TPersenseClipboard.SetRowDelimiter( RowDelimiter: string );
begin
  m_RowDelimiter := RowDelimiter;
end;

function TPersenseClipboard.GetCellDelimiter(): string;
begin
  GetCellDelimiter := m_CellDelimiter;
end;

function TPersenseClipboard.GetRowDelimiter(): string;
begin
  GetRowDelimiter := m_RowDelimiter;
end;

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
