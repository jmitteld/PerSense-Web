// Package actuarial implements life table calculations for mortality-contingent
// present value computations.
//
// This feature was present in the DOS version of Per%Sense (compiled under
// {$ifdef ACTU}) but was never ported to the Windows version. The ACTUARY unit
// source is missing; this implementation is reconstructed from integration
// points in PRESVALU.pas, PVLXSCRN.pas, pvltable.pas, PEDATA.pas, and
// PETYPES.PAS, combined with standard actuarial mathematics.
//
// A life table provides age-indexed mortality rates (qx = probability of dying
// within the year at age x). From these, we derive survival probabilities that
// are used to weight present value calculations — e.g., the present value of a
// pension payment is reduced by the probability that the recipient has died
// before the payment date.
package actuarial

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// LifeTable holds mortality data indexed by integer age.
// qx[i] is the probability of dying within the year for a person aged i.
// lx[i] is the number of survivors to exact age i from a cohort of 100,000.
type LifeTable struct {
	Name string    // descriptive name, e.g. "SSA 2021 Male"
	Qx   []float64 // qx[age] = probability of death within year at age
	Lx   []float64 // lx[age] = survivors to age from 100,000 birth cohort
}

// MaxAge returns the highest age in the table.
func (t *LifeTable) MaxAge() int {
	return len(t.Lx) - 1
}

// SurvivalProb returns the probability of surviving from birth to the given
// fractional age. For non-integer ages, linear interpolation is used between
// adjacent lx values (uniform distribution of deaths assumption).
func (t *LifeTable) SurvivalProb(age float64) float64 {
	if age <= 0 {
		return 1.0
	}
	maxAge := t.MaxAge()
	if age >= float64(maxAge) {
		return 0.0
	}
	intAge := int(age)
	frac := age - float64(intAge)
	if intAge >= maxAge {
		return 0.0
	}
	// Linear interpolation: lx(age) = lx[intAge] * (1 - frac) + lx[intAge+1] * frac
	// Normalize by lx[0] (should be 100,000 but normalize to be safe)
	l0 := t.Lx[0]
	if l0 == 0 {
		return 0.0
	}
	lAge := t.Lx[intAge]*(1-frac) + t.Lx[intAge+1]*frac
	return lAge / l0
}

// ConditionalSurvival returns the probability of surviving from currentAge to
// futureAge, given that the person is alive at currentAge.
// P(survive to futureAge | alive at currentAge) = lx(futureAge) / lx(currentAge)
func (t *LifeTable) ConditionalSurvival(currentAge, futureAge float64) float64 {
	if futureAge <= currentAge {
		return 1.0
	}
	sCurrent := t.SurvivalProb(currentAge)
	if sCurrent <= 0 {
		return 0.0
	}
	sFuture := t.SurvivalProb(futureAge)
	return sFuture / sCurrent
}

// LifeExpectancy returns the expected remaining years of life for a person
// at the given age, computed by summing survival probabilities.
// e(x) = sum_{k=0}^{max} P(survive k more years | alive at x)
func (t *LifeTable) LifeExpectancy(age float64) float64 {
	if age < 0 {
		age = 0
	}
	maxAge := t.MaxAge()
	sum := 0.0
	for k := 1; float64(k)+age <= float64(maxAge); k++ {
		p := t.ConditionalSurvival(age, age+float64(k))
		sum += p
	}
	return sum
}

// NewLifeTableFromQx creates a LifeTable from age-indexed qx values.
// qx[i] = probability of dying within the year at exact age i.
// lx is computed as: lx[0] = 100,000; lx[i+1] = lx[i] * (1 - qx[i])
func NewLifeTableFromQx(name string, qx []float64) *LifeTable {
	lx := make([]float64, len(qx)+1)
	lx[0] = 100000.0
	for i := 0; i < len(qx); i++ {
		lx[i+1] = lx[i] * (1 - qx[i])
		if lx[i+1] < 0 {
			lx[i+1] = 0
		}
	}
	return &LifeTable{Name: name, Qx: qx, Lx: lx}
}

// NewLifeTableFromLx creates a LifeTable from age-indexed lx values.
// lx[i] = survivors to exact age i from a radix (typically 100,000).
// qx is derived as: qx[i] = 1 - lx[i+1]/lx[i]
func NewLifeTableFromLx(name string, lx []float64) *LifeTable {
	if len(lx) < 2 {
		return &LifeTable{Name: name, Lx: lx}
	}
	qx := make([]float64, len(lx)-1)
	for i := 0; i < len(qx); i++ {
		if lx[i] > 0 {
			qx[i] = 1 - lx[i+1]/lx[i]
		} else {
			qx[i] = 1.0
		}
		qx[i] = math.Max(0, math.Min(1, qx[i]))
	}
	return &LifeTable{Name: name, Qx: qx, Lx: lx}
}

// ParseCSV parses a life table from CSV data.
// Expected format: rows of "age,value" where value is either qx or lx.
// The format parameter should be "qx" or "lx".
// Lines starting with # are treated as comments. Header rows are skipped.
func ParseCSV(name string, r io.Reader, format string) (*LifeTable, error) {
	reader := csv.NewReader(r)
	reader.Comment = '#'
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV parse error: %w", err)
	}

	type ageVal struct {
		age int
		val float64
	}
	var entries []ageVal

	for _, row := range records {
		if len(row) < 2 {
			continue
		}
		age, err := strconv.Atoi(strings.TrimSpace(row[0]))
		if err != nil {
			continue // skip header or non-numeric rows
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			continue
		}
		entries = append(entries, ageVal{age: age, val: val})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid data rows found in CSV")
	}

	// Find max age
	maxAge := 0
	for _, e := range entries {
		if e.age > maxAge {
			maxAge = e.age
		}
	}

	switch strings.ToLower(format) {
	case "qx":
		qx := make([]float64, maxAge+1)
		for _, e := range entries {
			if e.age <= maxAge {
				qx[e.age] = e.val
			}
		}
		return NewLifeTableFromQx(name, qx), nil
	case "lx":
		lx := make([]float64, maxAge+1)
		for _, e := range entries {
			if e.age <= maxAge {
				lx[e.age] = e.val
			}
		}
		return NewLifeTableFromLx(name, lx), nil
	default:
		return nil, fmt.Errorf("unknown format %q: use \"qx\" or \"lx\"", format)
	}
}

// ParseJSON parses a life table from JSON data.
// Expected format: [[age, value], [age, value], ...] where value is qx.
func ParseJSON(name string, data []byte) (*LifeTable, error) {
	var rows [][]float64
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("no data rows in JSON")
	}

	maxAge := 0
	for _, row := range rows {
		if len(row) >= 2 && int(row[0]) > maxAge {
			maxAge = int(row[0])
		}
	}

	qx := make([]float64, maxAge+1)
	for _, row := range rows {
		if len(row) >= 2 {
			age := int(row[0])
			if age >= 0 && age <= maxAge {
				qx[age] = row[1]
			}
		}
	}
	return NewLifeTableFromQx(name, qx), nil
}
