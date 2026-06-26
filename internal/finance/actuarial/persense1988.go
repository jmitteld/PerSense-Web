package actuarial

// 1988 HHS mortality tables — the actuarial basis the original DOS Per%Sense
// shipped and computed against. Recovered from the original distribution files
// MALE.ACT and FEMALE.ACT (qx = probability of death within the year at exact
// age x; the files express values as "x.xxxE-3" meaning ×0.001). Source named
// in the sibling 88B&WM&F.ACT header: "USA Dept of HHS, 1988".
//
// These are the DOS-faithful tables: validating or running the actuarial engine
// against them reproduces the mortality assumptions of the original program,
// unlike the SSA-2021 table the web build also offers. See
// docs/actuarial_oracle_blocked.md for why the DOS actuarial *engine* itself
// still cannot be differentially tested (its ACTUARY unit source is absent);
// these tables close the data half of that gap.
//
// NOTE (verify against DOS): FEMALE.ACT in the original distribution begins at
// age 1 — it has no age-0 row. We preserve that exactly and leave qx[0]=0 for
// the female table. This is immaterial to any real contingency (a reference age
// of 0 never occurs in practice) but is flagged rather than silently filled.
// TODO: verify logic — confirm how the DOS engine reads the missing female age-0.

// Persense1988MaleQx is the age-indexed qx for the 1988 HHS male table,
// qx[age] for age 0..115. qx[115]=1.0 (table closes).
var Persense1988MaleQx = []float64{
	0.000377, 0.000377, 0.000377, 0.000377, 0.000377, 0.000377, 0.00035,
	0.000333, 0.000352, 0.000368, 0.000382, 0.000394, 0.000405, 0.000415,
	0.000425, 0.000435, 0.000446, 0.000458, 0.000472, 0.000488, 0.000505,
	0.000525, 0.000546, 0.00057, 0.000596, 0.000622, 0.00065, 0.000677,
	0.000704, 0.000731, 0.000759, 0.000786, 0.000814, 0.000843, 0.000876,
	0.000917, 0.000968, 0.001032, 0.001114, 0.001216, 0.001341, 0.001492,
	0.001673, 0.001886, 0.002129, 0.002399, 0.002693, 0.003009, 0.003343,
	0.003694, 0.004057, 0.004431, 0.004812, 0.005198, 0.005591, 0.005994,
	0.006409, 0.006839, 0.00729, 0.007782, 0.008338, 0.008983, 0.00974, 0.01063,
	0.011664, 0.012851, 0.014199, 0.015717, 0.017414, 0.019296, 0.021371,
	0.023647, 0.026131, 0.028835, 0.031794, 0.035046, 0.038631, 0.042587,
	0.046951, 0.051755, 0.057026, 0.062971, 0.069081, 0.075908, 0.08323,
	0.090987, 0.099122, 0.107577, 0.116316, 0.125394, 0.134887, 0.144873,
	0.155429, 0.166629, 0.178537, 0.191214, 0.204721, 0.21912, 0.234735,
	0.251889, 0.270906, 0.292111, 0.315826, 0.342377, 0.372086, 0.405278,
	0.442277, 0.483406, 0.528989, 0.579351, 0.634814, 0.695704, 0.762343,
	0.835056, 0.914167, 1.0}

// Persense1988FemaleQx is the age-indexed qx for the 1988 HHS female table,
// qx[age] for age 0..115. The source file omits age 0, so qx[0]=0 (see note
// above); qx[115]=1.0 (table closes).
var Persense1988FemaleQx = []float64{
	0, 0.000194, 0.000194, 0.000194, 0.000194, 0.000194, 0.00016, 0.000134,
	0.000134, 0.000136, 0.000141, 0.000147, 0.000155, 0.000165, 0.000175,
	0.000188, 0.000201, 0.000214, 0.000229, 0.000244, 0.00026, 0.000276,
	0.000293, 0.000311, 0.00033, 0.000349, 0.000368, 0.000387, 0.000405,
	0.000423, 0.000441, 0.00046, 0.000479, 0.000499, 0.000521, 0.000545,
	0.000574, 0.000607, 0.000646, 0.000691, 0.000742, 0.000801, 0.000867,
	0.000942, 0.001026, 0.001122, 0.001231, 0.001356, 0.001499, 0.001657,
	0.00183, 0.002016, 0.002215, 0.002426, 0.00265, 0.002891, 0.003151,
	0.003432, 0.003739, 0.004081, 0.004467, 0.004908, 0.005413, 0.00599,
	0.006633, 0.007336, 0.00809, 0.008888, 0.009731, 0.010653, 0.011697,
	0.012905, 0.014319, 0.01598, 0.017909, 0.020127, 0.022654, 0.025509,
	0.028717, 0.032328, 0.036395, 0.040975, 0.046121, 0.051889, 0.058336,
	0.065518, 0.073493, 0.082318, 0.092017, 0.102491, 0.113605, 0.125227,
	0.137222, 0.149462, 0.161834, 0.174228, 0.186535, 0.198646, 0.211102,
	0.224445, 0.239215, 0.255953, 0.275201, 0.2975, 0.32339, 0.353414, 0.388111,
	0.428023, 0.473692, 0.525658, 0.584462, 0.650646, 0.72475, 0.807316,
	0.898885, 1.0}

// Persense1988Male returns the original DOS Per%Sense male life table.
func Persense1988Male() *LifeTable {
	return NewLifeTableFromQx("Per%Sense 1988 HHS Male", Persense1988MaleQx)
}

// Persense1988Female returns the original DOS Per%Sense female life table.
func Persense1988Female() *LifeTable {
	return NewLifeTableFromQx("Per%Sense 1988 HHS Female", Persense1988FemaleQx)
}
