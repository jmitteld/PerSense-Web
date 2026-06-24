package actuarial

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// dr is a small DateRec constructor helper for these tests.
func dr(y int, m time.Month, d int) types.DateRec { return types.NewDateRec(y, m, d) }

func TestRequiresSecondLife_AllCases(t *testing.T) {
	twoLife := []byte{Only1Living, Only2Living, EitherLiving, BothLiving}
	for _, c := range twoLife {
		if !RequiresSecondLife(c) {
			t.Errorf("RequiresSecondLife(%d) = false, want true", c)
		}
	}
	oneLife := []byte{NotContingent, Living, Dead, 99}
	for _, c := range oneLife {
		if RequiresSecondLife(c) {
			t.Errorf("RequiresSecondLife(%d) = true, want false", c)
		}
	}
}

func TestContingencyLabel_AllCases(t *testing.T) {
	cases := map[byte]string{
		NotContingent: "None",
		Living:        "Living",
		Dead:          "Deceased",
		Only1Living:   "Only 1 Living",
		Only2Living:   "Only 2 Living",
		EitherLiving:  "Either Living",
		BothLiving:    "Both Living",
		99:            "Unknown",
	}
	for c, want := range cases {
		if got := ContingencyLabel(c); got != want {
			t.Errorf("ContingencyLabel(%d) = %q, want %q", c, got, want)
		}
	}
}

func TestContingencyFromCode_AllCases(t *testing.T) {
	cases := map[string]byte{
		"L": Living,
		"D": Dead,
		"1": Only1Living,
		"2": Only2Living,
		"E": EitherLiving,
		"B": BothLiving,
		"N": NotContingent, // default
		"X": NotContingent, // default
		"":  NotContingent, // default
	}
	for code, want := range cases {
		if got := ContingencyFromCode(code); got != want {
			t.Errorf("ContingencyFromCode(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestLifeProb_DefaultAndTwoLife(t *testing.T) {
	lt := NewLifeTableFromQx("t", testQx)
	c := &ActuarialConfig{
		Table1: lt, DOB1: dr(2000, time.January, 1),
		Table2: lt, DOB2: dr(2000, time.January, 1),
		Now: dr(2002, time.January, 1),
	}
	date := dr(2004, time.January, 1)
	// Unknown contingency code -> default branch returns 1.0
	if got := c.LifeProb(date, 99); got != 1.0 {
		t.Errorf("LifeProb default = %v, want 1.0", got)
	}
	// Exercise each two-life branch (probabilities must be in [0,1]).
	for _, cont := range []byte{Living, Dead, Only1Living, Only2Living, EitherLiving, BothLiving} {
		p := c.LifeProb(date, cont)
		if p < -1e-9 || p > 1+1e-9 {
			t.Errorf("LifeProb(%d) = %v out of range", cont, p)
		}
	}
	// NotContingent short-circuit.
	if got := c.LifeProb(date, NotContingent); got != 1.0 {
		t.Errorf("LifeProb NotContingent = %v, want 1.0", got)
	}
}

func TestSurvivalProb1_NilTable(t *testing.T) {
	c := &ActuarialConfig{} // Table1 nil
	if got := c.survivalProb1(dr(2004, time.January, 1)); got != 1.0 {
		t.Errorf("survivalProb1 nil table = %v, want 1.0", got)
	}
}

func TestSurvivalProb_EdgeBranches(t *testing.T) {
	lt := NewLifeTableFromQx("t", testQx)
	if got := lt.SurvivalProb(0); got != 1.0 {
		t.Errorf("SurvivalProb(0) = %v, want 1.0", got)
	}
	if got := lt.SurvivalProb(-3); got != 1.0 {
		t.Errorf("SurvivalProb(-3) = %v, want 1.0", got)
	}
	if got := lt.SurvivalProb(100); got != 0.0 {
		t.Errorf("SurvivalProb(100) = %v, want 0.0", got)
	}
	// l0 == 0 branch.
	zero := &LifeTable{Name: "z", Lx: []float64{0, 50, 0}}
	if got := zero.SurvivalProb(1); got != 0.0 {
		t.Errorf("SurvivalProb with l0=0 = %v, want 0.0", got)
	}
}

func TestConditionalSurvival_EdgeBranches(t *testing.T) {
	lt := NewLifeTableFromQx("t", testQx)
	if got := lt.ConditionalSurvival(5, 3); got != 1.0 {
		t.Errorf("ConditionalSurvival(future<=current) = %v, want 1.0", got)
	}
	// sCurrent <= 0 branch: current age past max age.
	if got := lt.ConditionalSurvival(100, 101); got != 0.0 {
		t.Errorf("ConditionalSurvival(sCurrent=0) = %v, want 0.0", got)
	}
}

func TestLifeExpectancy_NegativeAge(t *testing.T) {
	lt := NewLifeTableFromQx("t", testQx)
	got := lt.LifeExpectancy(-5)
	want := lt.LifeExpectancy(0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("LifeExpectancy(-5) = %v, want = LifeExpectancy(0) = %v", got, want)
	}
}

func TestNewLifeTableFromQx_ClampNegative(t *testing.T) {
	// qx > 1 drives lx negative; constructor must clamp to 0.
	lt := NewLifeTableFromQx("t", []float64{1.5})
	if lt.Lx[1] != 0 {
		t.Errorf("Lx[1] = %v, want clamped 0", lt.Lx[1])
	}
}

func TestNewLifeTableFromLx_Branches(t *testing.T) {
	// len(lx) < 2 short-circuit.
	lt := NewLifeTableFromLx("short", []float64{100000})
	if lt.Qx != nil {
		t.Errorf("expected nil Qx for short lx, got %v", lt.Qx)
	}
	// lx[i] <= 0 -> qx forced to 1.0.
	lt2 := NewLifeTableFromLx("zeros", []float64{100000, 0, 0})
	if lt2.Qx[1] != 1.0 {
		t.Errorf("Qx[1] = %v, want 1.0 for zero survivors", lt2.Qx[1])
	}
}

func TestParseCSV_ErrorAndSkipPaths(t *testing.T) {
	// ReadAll parse error (unterminated quote).
	if _, err := ParseCSV("e", strings.NewReader("\"a,b\n"), "qx"); err == nil {
		t.Error("expected ReadAll error, got nil")
	}
	// All single-column rows: each skipped (len<2), then no entries.
	if _, err := ParseCSV("e", strings.NewReader("65\n66\n67\n"), "qx"); err == nil {
		t.Error("expected no-usable-rows error, got nil")
	}
	// Non-numeric age and non-numeric value rows are skipped; one good row remains.
	lt, err := ParseCSV("ok", strings.NewReader("abc,0.1\n65,xyz\n65,0.0123\n"), "qx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lt.MaxAge() != 66 {
		t.Errorf("MaxAge = %d, want 66", lt.MaxAge())
	}
	// lx format path.
	if _, err := ParseCSV("lx", strings.NewReader("0,100000\n1,99000\n"), "lx"); err != nil {
		t.Errorf("lx parse failed: %v", err)
	}
	// Unknown format -> error (default branch).
	if _, err := ParseCSV("bad", strings.NewReader("65,0.1\n"), "zz"); err == nil {
		t.Error("expected unknown-format error, got nil")
	}
}

func TestPODValueFunc_EdgeBranches(t *testing.T) {
	lt := NewLifeTableFromQx("t", testQx)
	// POD == 0 -> 0.
	c0 := &ActuarialConfig{Table1: lt, POD: 0}
	if got := c0.PODValueFunc(dr(2002, time.January, 1), func(float64) float64 { return 1 }); got != 0 {
		t.Errorf("PODValueFunc POD=0 = %v, want 0", got)
	}
	// Table1 == nil with POD != 0 -> 0.
	cNil := &ActuarialConfig{POD: 1000}
	if got := cNil.PODValueFunc(dr(2002, time.January, 1), func(float64) float64 { return 1 }); got != 0 {
		t.Errorf("PODValueFunc nil table = %v, want 0", got)
	}
	// ageNow >= maxAge -> 0 (born long before Now, beyond table max age).
	cOld := &ActuarialConfig{Table1: lt, POD: 1000,
		DOB1: dr(1900, time.January, 1), Now: dr(2002, time.January, 1)}
	if got := cOld.PODValueFunc(dr(2002, time.January, 1), func(float64) float64 { return 1 }); got != 0 {
		t.Errorf("PODValueFunc ageNow>=maxAge = %v, want 0", got)
	}
	// Normal path: hits pDeathInYear<=0 continue (qx[0]=0 -> flat first year)
	// and the accumulating sum branch.
	c := &ActuarialConfig{Table1: lt, POD: 1000,
		DOB1: dr(2000, time.January, 1), Now: dr(2000, time.January, 1)}
	got := c.PODValueFunc(dr(2000, time.January, 1), func(y float64) float64 { return math.Exp(-0.05 * y) })
	if got <= 0 {
		t.Errorf("PODValueFunc normal = %v, want > 0", got)
	}
	// PODValue wrapper (constant rate).
	if pv := c.PODValue(dr(2000, time.January, 1), 0.05); pv <= 0 {
		t.Errorf("PODValue = %v, want > 0", pv)
	}
}
