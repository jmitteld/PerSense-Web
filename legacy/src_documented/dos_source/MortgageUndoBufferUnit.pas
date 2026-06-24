{ ===========================================================================
  Unit:  MortgageUndoBufferUnit
  Role:  Undo/redo history for the Mortgage screen's grid.

  Snapshot model (shared by all three Per%Sense undo-buffer units):
    * A fixed-size CIRCULAR ring of UndoBufferSize snapshots. Each snapshot is
      a full deep copy of the screen's editable state - here the entire
      mortgage row array (mtgarray) plus the current grid selection
      (TGridRect). It snapshots whole-screen state, not deltas.
    * m_CurrentIndex points at the slot holding the live state. Taking a new
      snapshot advances CurrentIndex (mod size) and writes into the new slot.
    * m_UndoLimit is the oldest reachable slot; when CurrentIndex catches it
      the oldest history is overwritten.
    * m_RedoLimit marks the newest slot reachable by Redo; it is InvalidRedo
      until the first Undo, and is reset whenever new data is stored (a fresh
      edit destroys the redo branch).
    * m_RefCount supports TRANSACTIONAL snapshots: BeginSnapshot/EndSnapshot
      nest, so several StoreData calls between an outer Begin/End collapse into
      a single undo step. StoreData wraps one Begin/End (a standalone step);
      OverWriteData re-writes the current slot without advancing (folds a
      follow-on action, e.g. a recalc, into the same undo level as the edit).
  All record memory is pre-allocated in Create and freed in Destroy; snapshots
  copy by value into the pre-allocated slots.
  =========================================================================== }
unit MortgageUndoBufferUnit;

interface

uses
  Globals, LogUnit, Grids, Mortgage, peTypes;

const
  { UndoBufferSize = ring depth; InvalidRedo = "no redo branch" sentinel. }
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
    { ring of mortgage-row snapshots }
    m_MortgageArrays: array [0..UndoBufferSize-1] of mtgarray;
    { ring of grid-selection snapshots (parallel to m_MortgageArrays) }
    m_SelectionArray: array [0..UndoBufferSize-1] of TGridRect;
    { Allocate and zero all snapshot slots. }
    constructor Create();
    { Free all snapshot row memory. }
    destructor Destroy(); override;
    { Open a (possibly nested) transactional snapshot; advances slot if outer. }
    procedure BeginSnapshot();
    { Record one standalone undo step (self-wrapping Begin/End). }
    procedure StoreData( MortgageArray: mtgarray; Selection: TGridRect );
    { Re-write the current slot in place (fold into the existing undo step). }
    procedure OverWriteData( MortgageArray: mtgarray; Selection: TGridRect );
    { Close a transactional snapshot; on the outermost close, commit the step. }
    procedure EndSnapshot();
    { Step back one snapshot; returns it via out-params, Success=false at limit }
    procedure Undo( var MortgageArray: mtgarray; var Selection: TGridRect; var Success : boolean );
    { Step forward one snapshot; Success=false if no redo available. }
    procedure Redo( var MortgageArray: mtgarray; var Selection: TGridRect; var Success : boolean );
  private
    { m_RefCount = snapshot nesting depth; m_UndoLimit = undo floor slot;
      m_RedoLimit = newest redo-reachable slot (or InvalidRedo);
      m_CurrentIndex = slot holding the live state. }
    m_RefCount: integer;
    m_UndoLimit: integer;
    m_RedoLimit: integer;
    m_CurrentIndex: integer;
    { Deep-copy the supplied state into the current slot. }
    procedure InternalStoreData( MortgageArray: mtgarray; Selection: TGridRect );
  end;

implementation

{ TMortgageUndoBuffer }
{ Create
  Purpose: initialise ring pointers and pre-allocate every snapshot slot.
  Side effects: GetMem + ZeroMortgage for each mortgage line in every slot;
                resets limits/index to empty-history state. }
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

{ Destroy
  Purpose: release all pre-allocated snapshot row memory.
  Side effects: FreeMem for every line in every slot. }
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

{ InternalStoreData
  Purpose: deep-copy the given mortgage rows and selection into the current
           ring slot (the actual snapshot write).
  Side effects: overwrites m_MortgageArrays[m_CurrentIndex] and selection slot. }
procedure TMortgageUndoBuffer.InternalStoreData( MortgageArray: mtgarray; Selection: TGridRect );
var
  i: integer;
  InternalMax: integer;
begin
  for i:=1 to maxlines do
    m_MortgageArrays[m_CurrentIndex][i]^ := MortgageArray[i]^;
  m_SelectionArray[m_CurrentIndex] := Selection;
end;

{ Undo
  Purpose: move one snapshot back in history and return that prior state.
  Params (out): MortgageArray, Selection - restored state; Success - false if
                already at the undo floor.
  Side effects: decrements m_CurrentIndex (wrapping); initialises m_RedoLimit on
                the first undo so Redo becomes possible. }
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

{ Redo
  Purpose: move one snapshot forward (reapply an undone change).
  Params (out): MortgageArray, Selection - restored state; Success - false if no
                undo has occurred or already at the newest redo slot.
  Side effects: advances m_CurrentIndex (wrapping). }
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