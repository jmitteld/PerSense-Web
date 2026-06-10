package main

import (
	"math/rand"
	"testing"
)

// TestRigSelfClean: the engine compared against itself must never
// disagree — this pins that the generator, evaluation, and comparison
// plumbing are sound (a non-zero count here means the harness, not the
// engine, is broken).
func TestRigSelfClean(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	checked := 0
	for i := 0; i < 1500; i++ {
		w := genWorksheet(rng)
		g, gerr := goEval(w)
		o, oerr := goEval(w)
		if gerr != nil || oerr != nil {
			continue
		}
		checked++
		if ms := mismatches(g, o); len(ms) != 0 {
			t.Fatalf("self-diff mismatch on %+v: %v", w, ms)
		}
	}
	if checked < 500 {
		t.Fatalf("only %d comparable worksheets", checked)
	}
}

// TestRigCatchesAndShrinks: against a deliberately-broken oracle the rig
// must (a) find disagreements and (b) shrink one to a smaller failing
// worksheet — proving it would surface and minimize a real engine-vs-
// authority divergence.
func TestRigCatchesAndShrinks(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var firstFail Worksheet
	found := false
	for i := 0; i < 1500 && !found; i++ {
		w := genWorksheet(rng)
		if fails(w, mutantOracle) {
			firstFail, found = w, true
		}
	}
	if !found {
		t.Fatal("rig found no mismatch against the broken oracle — detection is not working")
	}
	min := shrink(firstFail, mutantOracle)
	if !fails(min, mutantOracle) {
		t.Fatalf("shrunk worksheet no longer fails: %+v", min)
	}
	// The shrink should not have grown the problem.
	if min.NPeriods > firstFail.NPeriods || min.Amount > firstFail.Amount {
		t.Errorf("shrink did not reduce: from %+v to %+v", firstFail, min)
	}
}
