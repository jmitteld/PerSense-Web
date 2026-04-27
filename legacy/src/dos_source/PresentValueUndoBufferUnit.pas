unit PresentValueUndoBufferUnit;

interface

uses
  Globals, LogUnit, Grids, peTypes;

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
  PreValData = record
    LumpSum: lumpsumarray;
    Periodic: periodicarray;
    PresVal: presvalarray;
    RateLine: ratelinearray;
    XPresVal: xpresval;
  end;

  TPresentValueUndoBuffer = class
  public
    m_PValArray: array [0..UndoBufferSize-1] of PreValData;
    constructor Create();
    destructor Destroy(); override;
    procedure BeginSnapshot();
    procedure StoreData( LumpSum: lumpsumarray; Periodic: periodicarray; PresVal: presvalarray;
                         RateLine: ratelinearray; XPresVal: xpresvalptr );
    procedure OverWriteData( LumpSum: lumpsumarray; Periodic: periodicarray; PresVal: presvalarray;
                             RateLine: ratelinearray; XPresVal: xpresvalptr );
    procedure EndSnapshot();
    procedure Undo( var LumpSum: lumpsumarray; var Periodic: periodicarray;
                    var PresVal: presvalarray; var RateLine: ratelinearray;
                    XPresVal: xpresvalptr; var Success : boolean );
    procedure Redo( var LumpSum: lumpsumarray; var Periodic: periodicarray;
                    var PresVal: presvalarray; var RateLine: ratelinearray;
                    XPresVal: xpresvalptr; var Success : boolean );
  private
    m_RefCount: integer;
    m_UndoLimit: integer;
    m_RedoLimit: integer;
    m_CurrentIndex: integer;
    procedure InternalStoreData( LumpSum: lumpsumarray; Periodic: periodicarray; PresVal: presvalarray;
                                 RateLine: ratelinearray; XPresVal: xpresvalptr );
  end;

implementation

uses Presvalu;

{ TMortgageUndoBuffer }
constructor TPresentValueUndoBuffer.Create();
var
  i, k: integer;
begin;
  m_UndoLimit := 0;
  m_RedoLimit := InvalidRedo;
  m_RefCount := 0;
  m_CurrentIndex := 0;
  for i:=0 to UndoBufferSize-1 do begin
    for k:=1 to maxlines do begin
      GetMem( m_PValArray[i].LumpSum[k], sizeof(lumpsum) );
      ZeroLumpSum( lumpsumptr(m_PValArray[i].LumpSum[k]) );
      GetMem( m_PValArray[i].Periodic[k], sizeof(periodic) );
      ZeroPeriodic( periodicptr(m_PValArray[i].Periodic[k]) );
      GetMem( m_PValArray[i].RateLine[k], sizeof(RateLine) );
      ZeroRateLine( RateLineptr(m_PValArray[i].RateLine[k]) );
    end;
    for k:=1 to presvallines do begin
      GetMem( m_PValArray[i].PresVal[k], sizeof(PresVal) );
      ZeroPresVal( presvalptr(m_PValArray[i].PresVal[k]) );
    end;
    ZeroXPresVal( @(m_PValArray[i].XPresVal) );
  end;
end;

destructor TPresentValueUndoBuffer.Destroy();
var
  i, k: integer;
begin;
  for i:=0 to UndoBufferSize-1 do begin
    for k:=1 to maxlines do begin
      FreeMem( m_PValArray[i].LumpSum[k] );
      FreeMem( m_PValArray[i].Periodic[k] );
      FreeMem( m_PValArray[i].RateLine[k] );
    end;
    for k:=1 to presvallines do
      FreeMem( m_PValArray[i].PresVal[k] );
  end;
end;

// Call BeginSnapshot to start storing data.  If you try to begin and you are
// already in a snapshot (ie for multiple row actions) then this will do nothing. 
procedure TPresentValueUndoBuffer.BeginSnapshot();
begin
  if( m_RefCount = 0 ) then
    m_CurrentIndex := (m_CurrentIndex+1) mod UndoBufferSize;
  Inc( m_RefCount );
end;

// Call this when you are done storing data.  If this is a multiple line snapshot
// then this will do nothing
procedure TPresentValueUndoBuffer.EndSnapshot();
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

procedure TPresentValueUndoBuffer.StoreData( LumpSum: lumpsumarray; Periodic: periodicarray; PresVal: presvalarray;
                                             RateLine: ratelinearray; XPresVal: xpresvalptr );
begin
  // calling BeginSnapshot at the begining has no effect if this is part of a
  // transactional snapshot thing.  If this is a one off then doing this will
  // allow the outside to call StoreData without the begin and end around it
  // when it just wants to store one action as an undo/redo
  BeginSnapshot();
  InternalStoreData( LumpSum, Periodic, PresVal, RateLine, XPresVal );
  // since we started this with a begin, we need to end with an end :)
  EndSnapshot();
end;

procedure TPresentValueUndoBuffer.OverWriteData( LumpSum: lumpsumarray; Periodic: periodicarray; PresVal: presvalarray;
                                                 RateLine: ratelinearray; XPresVal: xpresvalptr );
begin
  InternalStoreData( LumpSum, Periodic, PresVal, RateLine, XPresVal );
end;

procedure TPresentValueUndoBuffer.InternalStoreData( LumpSum: lumpsumarray; Periodic: periodicarray; PresVal: presvalarray;
                                                     RateLine: ratelinearray; XPresVal: xpresvalptr );
var
  i: integer;
begin
  for i:=1 to maxlines do begin
    m_PValArray[m_CurrentIndex].LumpSum[i]^ := LumpSum[i]^;
    m_PValArray[m_CurrentIndex].Periodic[i]^ := Periodic[i]^;
    m_PValArray[m_CurrentIndex].RateLine[i]^ := RateLine[i]^;
  end;
  for i:=1 to presvallines do
    m_PValArray[m_CurrentIndex].PresVal[i]^ := PresVal[i]^;
  m_PValArray[m_CurrentIndex].XPresVal := XPresVal^;
end;

procedure TPresentValueUndoBuffer.Undo( var LumpSum: lumpsumarray; var Periodic: periodicarray;
                                        var PresVal: presvalarray; var RateLine: ratelinearray;
                                        XPresVal: xpresvalptr; var Success : boolean );
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
  // now that we found the correct one, do the copy
  for i:=1 to maxlines do begin
    LumpSum[i]^ := m_PValArray[m_CurrentIndex].LumpSum[i]^;
    Periodic[i]^ := m_PValArray[m_CurrentIndex].Periodic[i]^;
    RateLine[i]^ := m_PValArray[m_CurrentIndex].RateLine[i]^;
  end;
  for i:=1 to presvallines do
    PresVal[i]^ := m_PValArray[m_CurrentIndex].PresVal[i]^;
  XPresVal^ := m_PValArray[m_CurrentIndex].XPresVal;
end;

procedure TPresentValueUndoBuffer.Redo( var LumpSum: lumpsumarray; var Periodic: periodicarray;
                                        var PresVal: presvalarray; var RateLine: ratelinearray;
                                        XPresVal: xpresvalptr; var Success : boolean  );
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
  // now that we found the correct one, do the copy
  for i:=1 to maxlines do begin
    LumpSum[i]^ := m_PValArray[m_CurrentIndex].LumpSum[i]^;
    Periodic[i]^ := m_PValArray[m_CurrentIndex].Periodic[i]^;
    RateLine[i]^ := m_PValArray[m_CurrentIndex].RateLine[i]^;
  end;
  for i:=1 to presvallines do
    PresVal[i]^ := m_PValArray[m_CurrentIndex].PresVal[i]^;
  XPresVal^ := m_PValArray[m_CurrentIndex].XPresVal;
end;

end.