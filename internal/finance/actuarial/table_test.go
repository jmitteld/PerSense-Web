package actuarial

import (
	"math"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Simple test life table: qx increases linearly, max age 5
// qx = [0.0, 0.1, 0.2, 0.4, 0.6, 1.0]
// lx = [100000, 100000, 90000, 72000, 43200, 17280, 0]
var testQx = []float64{0.0, 0.1, 0.2, 0.4, 0.6, 1.0}

func TestNewLifeTableFromQx(t *testing.T) {
	lt := NewLifeTableFromQx("test", testQx)

	if lt.MaxAge() != 6 {
		t.Errorf("MaxAge = %d, want 6", lt.MaxAge())
	}
	if lt.Lx[0] != 100000 {
		t.Errorf("Lx[0] = %f, want 100000", lt.Lx[0])
	}
	// lx[1] = 100000 * (1 - 0.0) = 100000
	if lt.Lx[1] != 100000 {
		t.Errorf("Lx[1] = %f, want 100000", lt.Lx[1])
	}
	// lx[2] = 100000 * (1 - 0.1) = 90000
	if math.Abs(lt.Lx[2]-90000) > 0.01 {
		t.Errorf("Lx[2] = %f, want 90000", lt.Lx[2])
	}
	// lx[3] = 90000 * (1 - 0.2) = 72000
	if math.Abs(lt.Lx[3]-72000) > 0.01 {
		t.Errorf("Lx[3] = %f, want 72000", lt.Lx[3])
	}
	// lx[6] = 17280 * (1 - 1.0) = 0
	if lt.Lx[6] != 0 {
		t.Errorf("Lx[6] = %f, want 0", lt.Lx[6])
	}
}

func TestSurvivalProb(t *testing.T) {
	lt := NewLifeTableFromQx("test", testQx)

	tests := []struct {
		age  float64
		want float64
		tol  float64
	}{
		{0, 1.0, 0.001},
		{1, 1.0, 0.001},              // qx[0]=0, so all survive year 0
		{2, 0.9, 0.001},              // 90000/100000
		{3, 0.72, 0.001},             // 72000/100000
		{4, 0.432, 0.001},            // 43200/100000
		{5, 0.1728, 0.001},           // 17280/100000
		{6, 0.0, 0.001},              // 0/100000
		{1.5, 0.95, 0.001},           // interpolated: (100000+90000)/2 / 100000
		{-1, 1.0, 0.001},             // before birth
		{100, 0.0, 0.001},            // way past max age
	}
	for _, tt := range tests {
		got := lt.SurvivalProb(tt.age)
		if math.Abs(got-tt.want) > tt.tol {
			t.Errorf("SurvivalProb(%v) = %f, want %f", tt.age, got, tt.want)
		}
	}
}

func TestConditionalSurvival(t *testing.T) {
	lt := NewLifeTableFromQx("test", testQx)

	// P(survive to 3 | alive at 2) = lx[3]/lx[2] = 72000/90000 = 0.8
	got := lt.ConditionalSurvival(2, 3)
	if math.Abs(got-0.8) > 0.001 {
		t.Errorf("ConditionalSurvival(2,3) = %f, want 0.8", got)
	}

	// P(survive to 2 | alive at 2) = 1.0 (already there)
	got = lt.ConditionalSurvival(2, 2)
	if math.Abs(got-1.0) > 0.001 {
		t.Errorf("ConditionalSurvival(2,2) = %f, want 1.0", got)
	}

	// P(survive to 2 | alive at 0) = 0.9
	got = lt.ConditionalSurvival(0, 2)
	if math.Abs(got-0.9) > 0.001 {
		t.Errorf("ConditionalSurvival(0,2) = %f, want 0.9", got)
	}
}

func TestContingencyProbs(t *testing.T) {
	lt := NewLifeTableFromQx("test", testQx)

	dob := types.NewDateRec(1980, 1, 1)
	now := types.NewDateRec(2020, 1, 1) // age 40 at "now"
	// Use a simple date where person is alive at now
	// For our test table, max age is 6, so everyone at age 40 is dead.
	// Use dob so person is age 3 at "now":
	dob = types.NewDateRec(2017, 1, 1) // age 3 at 2020
	payDate := types.NewDateRec(2022, 1, 1) // age 5 at payment

	cfg := &ActuarialConfig{
		Table1: lt,
		DOB1:   dob,
		Table2: lt,
		DOB2:   dob,
		Now:    now,
	}

	// P(alive at age 5 | alive at age 3) = lx[5]/lx[3] = 17280/72000 = 0.24
	pLiving := cfg.LifeProb(payDate, Living)
	if math.Abs(pLiving-0.24) > 0.01 {
		t.Errorf("LifeProb Living = %f, want 0.24", pLiving)
	}

	// Dead = 1 - Living
	pDead := cfg.LifeProb(payDate, Dead)
	if math.Abs(pDead-0.76) > 0.01 {
		t.Errorf("LifeProb Dead = %f, want 0.76", pDead)
	}

	// Living + Dead should sum to 1
	if math.Abs(pLiving+pDead-1.0) > 0.001 {
		t.Errorf("Living + Dead = %f, want 1.0", pLiving+pDead)
	}

	// NotContingent always 1.0
	pNone := cfg.LifeProb(payDate, NotContingent)
	if pNone != 1.0 {
		t.Errorf("LifeProb NotContingent = %f, want 1.0", pNone)
	}

	// BothLiving = s1 * s2 (same table/dob) = 0.24^2 = 0.0576
	pBoth := cfg.LifeProb(payDate, BothLiving)
	if math.Abs(pBoth-0.0576) > 0.01 {
		t.Errorf("LifeProb BothLiving = %f, want 0.0576", pBoth)
	}

	// EitherLiving = 1 - (1-s1)*(1-s2) = 1 - 0.76^2 = 1 - 0.5776 = 0.4224
	pEither := cfg.LifeProb(payDate, EitherLiving)
	if math.Abs(pEither-0.4224) > 0.01 {
		t.Errorf("LifeProb EitherLiving = %f, want 0.4224", pEither)
	}
}

func TestParseCSV(t *testing.T) {
	csvData := `age,qx
0,0.0
1,0.1
2,0.2
3,0.4
4,0.6
5,1.0`

	lt, err := ParseCSV("test", strings.NewReader(csvData), "qx")
	if err != nil {
		t.Fatalf("ParseCSV error: %v", err)
	}
	if lt.MaxAge() != 6 { // 6 lx entries for ages 0-5 qx
		t.Errorf("MaxAge = %d, want 6", lt.MaxAge())
	}
	if math.Abs(lt.SurvivalProb(2)-0.9) > 0.001 {
		t.Errorf("SurvivalProb(2) = %f, want 0.9", lt.SurvivalProb(2))
	}
}

func TestParseJSON(t *testing.T) {
	jsonData := `[[0,0.0],[1,0.1],[2,0.2],[3,0.4],[4,0.6],[5,1.0]]`

	lt, err := ParseJSON("test", []byte(jsonData))
	if err != nil {
		t.Fatalf("ParseJSON error: %v", err)
	}
	if math.Abs(lt.SurvivalProb(2)-0.9) > 0.001 {
		t.Errorf("SurvivalProb(2) = %f, want 0.9", lt.SurvivalProb(2))
	}
}

func TestLifeExpectancy(t *testing.T) {
	lt := NewLifeTableFromQx("test", testQx)

	// Life expectancy at birth = sum of P(survive to k) for k=1..6
	// = 1.0 + 0.9 + 0.72 + 0.432 + 0.1728 + 0 = 3.2248
	le := lt.LifeExpectancy(0)
	if math.Abs(le-3.2248) > 0.01 {
		t.Errorf("LifeExpectancy(0) = %f, want ~3.22", le)
	}
}
