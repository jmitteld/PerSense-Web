unit LogUnit;

interface

uses
  SysUtils, Classes;

const
  LVL_HIGH              = 100;
  LVL_MED               = 50;
  LVL_LOW               = 10;

type
  TLog = class
  private
    m_LogLevel : integer;
    m_FileName : string;
    m_FileStream : TFileStream;
    m_FileOpen : boolean;
    procedure AssureOpen();
  public
    constructor Create();
    destructor Destroy(); override;
    procedure Initialize( FileName: string; Level: integer );
    procedure Write( Level: integer; Output: string );
    function GetLevel(): integer;
  protected
  end;

var
  MasterLog: TLog;

implementation

constructor TLog.Create();
begin
  m_FileName := 'LogUnitOutput.txt';
  m_LogLevel := LVL_LOW;
  m_FileOpen := false;
end;

destructor TLog.Destroy();
begin
  if( m_FileOpen ) then
    m_FileStream.Free();
end;

procedure TLog.AssureOpen();
begin
  if( not m_FileOpen ) then
    Initialize( m_FileName, m_LogLevel );
end;

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

procedure TLog.Write( Level: integer; Output: string );
const
  LineFeed = #13+#10;
begin
  AssureOpen();
  if( (Level >= m_LogLevel) and m_FileOpen ) then begin
    m_FileStream.Write( PChar(Output)^, Length(Output) );
    m_FileStream.Write( LineFeed, 2 );
  end;
end;

function TLog.GetLevel(): integer;
begin
  GetLevel := m_LogLevel;
end;

{
I always seem to need the following text, so:
Format('#%2.2x%2.2x%2.2x', [Red,Green,Blue]);
}

end.
