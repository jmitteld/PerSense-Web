{ ===========================================================================
  Unit:  LogUnit
  Role:  Minimal severity-filtered text logger written to a file.

  Provides a single TLog class plus a global MasterLog instance used across
  the app for diagnostic tracing. Each call to Write carries a severity Level;
  the message is emitted only if Level >= the configured threshold
  (m_LogLevel). The log is lazily opened on first write (AssureOpen), which
  truncates/recreates the target file (fmCreate).

  Severity constants below are ordered LOW < MED < HIGH; setting the threshold
  higher suppresses lower-severity chatter.

  { Go port: n/a -- diagnostic file logger; the Go port uses the standard
    library's log/net/http request logging rather than a bespoke severity
    logger. No financial logic. }
  ===========================================================================}
unit LogUnit;

interface

uses
  SysUtils, Classes;

const
  { highest severity / most important messages }
  LVL_HIGH              = 100;
  { medium severity }
  LVL_MED               = 50;
  { low severity / verbose (default threshold) }
  LVL_LOW               = 10;

type
  TLog = class
  private
    { threshold: messages below this are dropped }
    m_LogLevel : integer;
    { target log file path }
    m_FileName : string;
    { open stream (valid only when m_FileOpen) }
    m_FileStream : TFileStream;
    { true once the file stream is open }
    m_FileOpen : boolean;
    { Lazily open the log file on first use, using current name/level. }
    procedure AssureOpen();
  public
    { Construct with defaults: file 'LogUnitOutput.txt', level LVL_LOW, closed. }
    constructor Create();
    { Release the file stream if it was opened. }
    destructor Destroy(); override;
    { (Re)configure target file and threshold and (re)create the file. }
    procedure Initialize( FileName: string; Level: integer );
    { Append a line if Level passes the threshold and the file is open. }
    procedure Write( Level: integer; Output: string );
    { Return the current severity threshold. }
    function GetLevel(): integer;
  protected
  end;

var
  { global application-wide logger instance }
  MasterLog: TLog;

implementation

{ Create
  Purpose: initialise a logger with safe defaults; no file is opened yet.
  Side effects: sets default file name, LVL_LOW threshold, m_FileOpen=false. }
constructor TLog.Create();
begin
  m_FileName := 'LogUnitOutput.txt';
  m_LogLevel := LVL_LOW;
  m_FileOpen := false;
end;

{ Destroy
  Purpose: free the underlying file stream if one was opened.
  Side effects: closes/frees m_FileStream. }
destructor TLog.Destroy();
begin
  if( m_FileOpen ) then
    m_FileStream.Free();
end;

{ AssureOpen
  Purpose: lazy-open guard - opens the file on first write if not yet open.
  Side effects: may call Initialize, which (re)creates the file. }
procedure TLog.AssureOpen();
begin
  if( not m_FileOpen ) then
    Initialize( m_FileName, m_LogLevel );
end;

{ Initialize
  Purpose: set the file name and threshold, then create/truncate the file.
  Params:  FileName - log path; Level - new severity threshold.
  Side effects: assigns config fields; creates m_FileStream (fmCreate, which
                overwrites any existing file). On open failure (EFOpenError)
                m_FileOpen is reset to false so later writes are no-ops. }
procedure TLog.Initialize( FileName: string; Level: integer );
begin
  m_LogLevel := Level;
  m_FileName := FileName;
  m_FileOpen := true;
  try
    m_FileStream := TFileStream.Create( FileName, fmCreate );
  except
    on EFOpenError do m_FileOpen := false;
  end;
end;

{ Write
  Purpose: emit one log line (followed by CR/LF) when it meets the threshold.
  Params:  Level  - severity of this message; Output - the text to log.
  Side effects: ensures the file is open, then writes the raw bytes plus a
                CR/LF terminator to the stream. Messages with Level below the
                threshold, or when the file failed to open, are silently
                dropped. }
procedure TLog.Write( Level: integer; Output: string );
const
  { CR+LF line terminator written after each message }
  LineFeed = #13+#10;
begin
  AssureOpen();
  if( (Level >= m_LogLevel) and m_FileOpen ) then begin
    m_FileStream.Write( PChar(Output)^, Length(Output) );
    m_FileStream.Write( LineFeed, 2 );
  end;
end;

{ GetLevel
  Purpose: expose the current severity threshold.
  Returns: m_LogLevel. }
function TLog.GetLevel(): integer;
begin
  GetLevel := m_LogLevel;
end;

{
I always seem to need the following text, so:
Format('#%2.2x%2.2x%2.2x', [Red,Green,Blue]);
}

end.
