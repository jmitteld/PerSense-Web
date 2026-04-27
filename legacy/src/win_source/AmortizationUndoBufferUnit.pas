unit AmortizationUndoBufferUnit;

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
    m_AMZArray: array [0..UndoBufferSize-1] of AMZData;
    constructor Create();
    destructor Destroy(); override;
    procedure BeginSnapshot();
    procedure StoreData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                         Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                         Target: targetptr; Skip: skipptr );
    procedure OverWriteData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                             Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                             Target: targetptr; Skip: skipptr );
    procedure EndSnapshot();
    procedure Undo( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                    var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                    Target: targetptr; Skip: skipptr; var Success : boolean );
    procedure Redo( AMZ:AMZPtr; Payoff: balloonptr; var Prepayment: prepaymentarray;
                    var Balloon: balloonarray; var ADJ: adjarray; Mor: Moratoriumptr;
                    Target: targetptr; Skip: skipptr; var Success : boolean );
  private
    m_RefCount: integer;
    m_UndoLimit: integer;
    m_RedoLimit: integer;
    m_CurrentIndex: integer;
    procedure InternalStoreData( AMZ:AMZPtr; Payoff: balloonptr; Prepayment: prepaymentarray;
                                 Balloon: balloonarray; ADJ: adjarray; Mor: Moratoriumptr;
                                 Target: targetptr; Skip: skipptr );
  end;

implementation

uses Amortize;

{ TMortgageUndoBuffer }
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