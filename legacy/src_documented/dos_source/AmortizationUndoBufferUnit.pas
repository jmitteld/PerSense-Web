{ ===========================================================================
  Unit:  AmortizationUndoBufferUnit
  Role:  Undo/redo history for the Amortization screen (loan + advanced options)

  Same circular-ring snapshot model as MortgageUndoBufferUnit (see that unit's
  header for CurrentIndex/UndoLimit/RedoLimit/RefCount and the transactional
  Begin/End + OverWriteData fold-in semantics).

  Captured state (bundled per snapshot into AMZData) is the amortization
  worksheet plus every Advanced Option list: core loan, payoff/balloon,
  prepayment plans, balloon lumps, rate/payment adjustments (ARMs), moratorium
  (interest-only deferment), minimum-principal target, and skipped months.
  Fixed-size sub-arrays (Prepayment/Balloon/Adjustment) are pre-allocated in
  Create; scalar records are copied by value.
  =========================================================================== }
unit AmortizationUndoBufferUnit;

interface

uses
  Globals, LogUnit, Grids, peTypes;

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
  { One full snapshot of the Amortization worksheet + advanced options:
      AMZ=core loan; Payoff=payoff/final-balloon; Prepayment=extra-payment plans;
      Balloon=one-off lumps; Adjustment=ARM rate/payment changes;
      Moratorium=interest-only deferment; Target=min principal-reduction;
      Skip=skipped-payment months. }
  AMZData = record
    AMZ: AMZLoan;
    Payoff: balloonrec;
    Prepayment: prepaymentarray;
    Balloon: balloonarray;
    Adjustment: adjarray;
    Moratorium: moratoriumrec;
    Target: targetrec;
    Skip: skiprec;
  end;

  TAmortizationUndoBuffer = class
  public
    { snapshot ring }
    m_AMZArray: array [0..UndoBufferSize-1] of AMZData;
    { Allocate and zero all snapshot slots' sub-arrays. }
    constructor Create();
    { Free all pre-allocated sub-array memory. }
    destructor Destroy(); override;
    { Open a (possibly nested) transactional snapshot. }
    procedure BeginSnapshot();
    { Record one standalone undo step (self-wrapping Begin/End). }
    procedure StoreData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                         Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                         Target: targetptr; Skip: skipptr );
    { Re-write the current slot in place (fold into the existing undo step). }
    procedure OverWriteData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                             Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                             Target: targetptr; Skip: skipptr );
    { Close a transactional snapshot; commit on the outermost close. }
    procedure EndSnapshot();
    { Step back one snapshot, restoring loan + all options; false at floor. }
    procedure Undo( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                    var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                    Target: targetptr; Skip: skipptr; var Success : boolean );
    { Step forward one snapshot; false if no redo available. }
    procedure Redo( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                    var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                    Target: targetptr; Skip: skipptr; var Success : boolean );
  private
    { m_RefCount=nesting depth; m_UndoLimit=undo floor; m_RedoLimit=newest redo
      slot or InvalidRedo; m_CurrentIndex=live slot. }
    m_RefCount: integer;
    m_UndoLimit: integer;
    m_RedoLimit: integer;
    m_CurrentIndex: integer;
    { Deep-copy loan + all option groups into the current slot. }
    procedure InternalStoreData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                                 Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                                 Target: targetptr; Skip: skipptr );
  end;

implementation

uses Amortize;

{ TMortgageUndoBuffer }
{ NOTE: this stray banner is copy-pasted; the class is TAmortizationUndoBuffer. }
{ Create
  Purpose: initialise ring pointers and pre-allocate the fixed-size option
           sub-arrays (Prepayment/Balloon/Adjustment) for every slot.
  NOTE: AMZ/Payoff/Moratorium/Target/Skip are value fields, so they need no
        GetMem (copied by value in InternalStoreData). }
constructor TAmortizationUndoBuffer.Create();
var
  i, k: integer;
begin;
  m_UndoLimit := 0;
  m_RedoLimit := InvalidRedo;
  m_RefCount := 0;
  m_CurrentIndex := 0;
  for i:=0 to UndoBufferSize-1 do begin
    for k:=1 to maxprepay do begin
      GetMem( m_AMZArray[i].Prepayment[k], sizeof(prepaymentrec) );
      ZeroPrepayment( prepaymentptr(m_AMZArray[i].Prepayment[k]) );
    end;
    for k:=1 to maxballoon do begin
      GetMem( m_AMZArray[i].balloon[k], sizeof(balloonrec) );
      ZeroBalloon( balloonptr(m_AMZArray[i].balloon[k]) );
    end;
    for k:=1 to maxadj do begin
      GetMem( m_AMZArray[i].Adjustment[k], sizeof(adjrec) );
      ZeroAdjustment( adjptr(m_AMZArray[i].Adjustment[k]) );
    end;
  end;
end;

{ Destroy
  Purpose: release the pre-allocated option sub-array memory in every slot. }
destructor TAmortizationUndoBuffer.Destroy();
var
  i, k: integer;
begin;
  for i:=0 to UndoBufferSize-1 do begin
    for k:=1 to maxprepay do
      FreeMem( m_AMZArray[i].Prepayment[k] );
    for k:=1 to maxballoon do
      FreeMem( m_AMZArray[i].balloon[k] );
    for k:=1 to maxadj do
      FreeMem( m_AMZArray[i].Adjustment[k] );
  end;
end;

// Call BeginSnapshot to start storing data.  If you try to begin and you are
// already in a snapshot (ie for multiple row actions) then this will do nothing. 
procedure TAmortizationUndoBuffer.BeginSnapshot();
begin
  if( m_RefCount = 0 ) then
    m_CurrentIndex := (m_CurrentIndex+1) mod UndoBufferSize;
  Inc( m_RefCount );
end;

// Call this when you are done storing data.  If this is a multiple line snapshot
// then this will do nothing
procedure TAmortizationUndoBuffer.EndSnapshot();
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

procedure TAmortizationUndoBuffer.StoreData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                                             Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                                             Target: targetptr; Skip: skipptr );
begin
  // calling BeginSnapshot at the begining has no effect if this is part of a
  // transactional snapshot thing.  If this is a one off then doing this will
  // allow the outside to call StoreData without the begin and end around it
  // when it just wants to store one action as an undo/redo
  BeginSnapshot();
  InternalStoreData( AMZ, Payoff, Prepayment, Balloon, ADJ, Mor, Target, Skip );
  // since we started this with a begin, we need to end with an end :)
  EndSnapshot();
end;

procedure TAmortizationUndoBuffer.OverWriteData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                                             Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                                             Target: targetptr; Skip: skipptr );
begin
  InternalStoreData( AMZ, Payoff, Prepayment, Balloon, ADJ, Mor, Target, Skip );
end;

{ InternalStoreData
  Purpose: deep-copy the loan and every advanced-option group into the current
           ring slot (the actual snapshot write). }
procedure TAmortizationUndoBuffer.InternalStoreData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                                                 Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                                                 Target: targetptr; Skip: skipptr );
var
  i: integer;
begin
  m_AMZArray[m_CurrentIndex].AMZ := AMZ^;
  m_AMZArray[m_CurrentIndex].Payoff := Payoff^;
  for i:=1 to maxprepay do
    m_AMZArray[m_CurrentIndex].Prepayment[i]^ := Prepayment[i]^;
  for i:=1 to maxballoon do
    m_AMZArray[m_CurrentIndex].Balloon[i]^ := Balloon[i]^;
  for i:=1 to maxadj do
    m_AMZArray[m_CurrentIndex].Adjustment[i]^ := ADJ[i]^;
  m_AMZArray[m_CurrentIndex].Moratorium := Mor^;
  m_AMZArray[m_CurrentIndex].Target := Target^;
  m_AMZArray[m_CurrentIndex].Skip := Skip^;
end;

{ Undo
  Purpose: step one snapshot back, restoring the loan and all advanced options.
  Side effects: decrements m_CurrentIndex (wrapping); seeds m_RedoLimit on the
                first undo. Success=false if already at the undo floor. }
procedure TAmortizationUndoBuffer.Undo( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                                        var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                                        Target: targetptr; Skip: skipptr; var Success : boolean );
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
  AMZ^ := m_AMZArray[m_CurrentIndex].AMZ;
  Payoff^ := m_AMZArray[m_CurrentIndex].Payoff;
  for i:=1 to maxprepay do
    Prepayment[i]^ := m_AMZArray[m_CurrentIndex].Prepayment[i]^;
  for i:=1 to maxballoon do
    Balloon[i]^ := m_AMZArray[m_CurrentIndex].Balloon[i]^;
  for i:=1 to maxadj do
    ADJ[i]^ := m_AMZArray[m_CurrentIndex].Adjustment[i]^;
  Mor^ := m_AMZArray[m_CurrentIndex].Moratorium;
  Target^ := m_AMZArray[m_CurrentIndex].Target;
  Skip^ := m_AMZArray[m_CurrentIndex].Skip;
end;

{ Redo
  Purpose: step one snapshot forward, restoring the loan and all options.
  Side effects: advances m_CurrentIndex (wrapping). Success=false if no redo. }
procedure TAmortizationUndoBuffer.Redo( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                                        var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                                        Target: targetptr; Skip: skipptr; var Success : boolean  );
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
  AMZ^ := m_AMZArray[m_CurrentIndex].AMZ;
  Payoff^ := m_AMZArray[m_CurrentIndex].Payoff;
  for i:=1 to maxprepay do
    Prepayment[i]^ := m_AMZArray[m_CurrentIndex].Prepayment[i]^;
  for i:=1 to maxballoon do
    Balloon[i]^ := m_AMZArray[m_CurrentIndex].Balloon[i]^;
  for i:=1 to maxadj do
    ADJ[i]^ := m_AMZArray[m_CurrentIndex].Adjustment[i]^;
  Mor^ := m_AMZArray[m_CurrentIndex].Moratorium;
  Target^ := m_AMZArray[m_CurrentIndex].Target;
  Skip^ := m_AMZArray[m_CurrentIndex].Skip;
end;

end.