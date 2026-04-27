unit MortgageUndoBufferUnit;

interface

uses
  Globals, LogUnit, Grids, Mortgage, peTypes;

const
  UndoBufferSize                = 20;  { setting this to zero would SUCK }
  InvalidRedo                   = -1;

type
  { an undo buffer class.  Handles storing actions for later undo and redo.
    There are 3 ways to use, differentiated by which buffer they write to
    1 overwrite existing - Store data this way if you know that an action has
      been performed that has written data to an undo buffer, and you know that
      the buffer isn't complete.  For example you press enter on a cell and the data
      entered will create one undo buffer and then the calculation will create a second
      undo buffer.  You want both to be the same undo buffer.
    2 Normal mode - storing data will create one level of undo
    3 Transactional mode - you want the next 5 actions to all be part of one
      buffer.  Call BeginSnapshot followed by as many storedata as you want
      followed by EndSnapshot }
  TMortgageUndoBuffer = class
  public
    m_MortgageArrays: array [0..UndoBufferSize-1] of mtgarray;
    m_SelectionArray: array [0..UndoBufferSize-1] of TGridRect;
    constructor Create();
    destructor Destroy(); override;
    procedure BeginSnapshot();
    procedure StoreData( MortgageArray: mtgarray; Selection: TGridRect );
    procedure OverWriteData( MortgageArray: mtgarray; Selection: TGridRect );
    procedure EndSnapshot();
    procedure Undo( var MortgageArray: mtgarray; var Selection: TGridRect; var Success : boolean );
    procedure Redo( var MortgageArray: mtgarray; var Selection: TGridRect; var Success : boolean );
  private
    m_RefCount: integer;
    m_UndoLimit: integer;
    m_RedoLimit: integer;
    m_CurrentIndex: integer;
    procedure InternalStoreData( MortgageArray: mtgarray; Selection: TGridRect );
  end;

implementation

{ TMortgageUndoBuffer }
constructor TMortgageUndoBuffer.Create();
var
  i, j: integer;
begin;
  m_UndoLimit := 0;
  m_RedoLimit := InvalidRedo;
  m_RefCount := 0;
  m_CurrentIndex := 0;
  for i:=0 to UndoBufferSize-1 do begin
    for j:=1 to maxlines do begin
      GetMem( m_MortgageArrays[i][j], sizeof(mtgline) );
      ZeroMortgage( mtgptr(m_MortgageArrays[i][j]) );
    end;
  end;
end;

destructor TMortgageUndoBuffer.Destroy();
var
  i, j: integer;
begin;
  for i:=0 to UndoBufferSize-1 do begin
    for j:=1 to maxlines do
      FreeMem( m_MortgageArrays[i][j] );
  end;
end;

{ Call BeginSnapshot to start storing data.  If you try to begin and you are
  already in a snapshot (ie for multiple row actions) then this will do nothing. }
procedure TMortgageUndoBuffer.BeginSnapshot();
begin
  if( m_RefCount = 0 ) then
    m_CurrentIndex := (m_CurrentIndex+1) mod UndoBufferSize;
  Inc( m_RefCount );
end;

{ Call this when you are done storing data.  If this is a multiple line snapshot
  then this will do nothing }
procedure TMortgageUndoBuffer.EndSnapshot();
begin
  Dec( m_RefCount );
  if( m_RefCount = 0 ) then begin
    { if CurrentIndex equals the UndoLimit then we have to
      advance the UndoLimit along.  The UndoLimit should always
      be one step ahead of the CurrentIndex, except when
      the amount of undos have been depleted }
    if( m_CurrentIndex = m_UndoLimit ) then
      m_UndoLimit := (m_UndoLimit+1) mod UndoBufferSize;
    { Entering data will destroy the redo set }
    m_RedoLimit := InvalidRedo;
  end;
end;

procedure TMortgageUndoBuffer.StoreData( MortgageArray: mtgarray; Selection: TGridRect );
begin
  // calling BeginSnapshot at the begining has no effect if this is part of a
  // transactional snapshot thing.  If this is a one off then doing this will
  // allow the outside to call StoreData without the begin and end around it
  // when it just wants to store one action as an undo/redo
  BeginSnapshot();
  InternalStoreData( MortgageArray, Selection );
  // since we started this with a begin, we need to end with an end :)
  EndSnapshot();
end;

procedure TMortgageUndoBuffer.OverWriteData( MortgageArray: mtgarray; Selection: TGridRect );
begin
  InternalStoreData( MortgageArray, Selection );
end;

procedure TMortgageUndoBuffer.InternalStoreData( MortgageArray: mtgarray; Selection: TGridRect );
var
  i: integer;
  InternalMax: integer;
begin
  for i:=1 to maxlines do
    m_MortgageArrays[m_CurrentIndex][i]^ := MortgageArray[i]^;
  m_SelectionArray[m_CurrentIndex] := Selection;
end;

procedure TMortgageUndoBuffer.Undo( var MortgageArray: mtgarray; var Selection: TGridRect; var Success : boolean );
var
  i: integer;
begin
  Success := true;
  { If CurrentIndex equals the m_UndoLimit then
    we've undone as much as possible so just return
    the current index, no undo }
  if( m_CurrentIndex = m_UndoLimit ) then begin
    MasterLog.Write( LVL_LOW, 'Undo: Reached Undo limit' );
    Success := false;
    exit;
  end;
  { If this is our first undo then we need to start up
    the redo system by setting the redo limit }
  if( m_RedoLimit = InvalidRedo ) then
    m_RedoLimit := m_CurrentIndex;
  if( m_CurrentIndex > 0 ) then
    m_CurrentIndex := m_CurrentIndex - 1
  else
    m_CurrentIndex := UndoBufferSize-1;
  for i:=1 to maxlines do
    MortgageArray[i]^ := m_MortgageArrays[m_CurrentIndex][i]^;
  Selection := m_SelectionArray[m_CurrentIndex];
end;

procedure TMortgageUndoBuffer.Redo( var MortgageArray: mtgarray; var Selection: TGridRect; var Success : boolean );
var
  i: integer;
begin
  Success := true;
  { If their either trying to do a redo without doing an undo,
    of they're done all the redos they can just return the current index }
  if( (m_RedoLimit = InvalidRedo) or (m_CurrentIndex = m_RedoLimit) ) then begin
    MasterLog.Write( LVL_LOW, 'Redo: redo limit reached' );
    Success := false;
    exit;
  end;
  m_CurrentIndex := (m_CurrentIndex+1) mod UndoBufferSize;
  for i:=1 to maxlines do
    MortgageArray[i]^ := m_MortgageArrays[m_CurrentIndex][i]^;
  Selection := m_SelectionArray[m_CurrentIndex];
end;

end.