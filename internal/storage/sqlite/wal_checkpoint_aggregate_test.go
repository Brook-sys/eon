package sqlite_test

import (
	"testing"
)

func TestIntPercentile(t *testing.T) {
	tests := []struct {
		name string
		vals []int64
		p    float64
		want int64
	}{
		{"empty", nil, 50, 0},
		{"single", []int64{42}, 50, 42},
		{"single_p95", []int64{42}, 95, 42},
		{"two_p50", []int64{10, 90}, 50, 10},
		{"three_p50", []int64{10, 50, 90}, 50, 50},
		{"three_p95", []int64{10, 50, 90}, 95, 90},
		{"unsorted_p50", []int64{90, 10, 50}, 50, 50},
		{"ten_p50", []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 50, 5},  // idx=4 (0-based)
		{"ten_p95", []int64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, 95, 10}, // nearest-rank ceil(.95*10)=10
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := intPercentile(tc.vals, tc.p)
			if got != tc.want {
				t.Fatalf("intPercentile(vals=%v, p=%.0f) = %d, want %d", tc.vals, tc.p, got, tc.want)
			}
		})
	}
}

func TestIntPercentileDoesNotMutate(t *testing.T) {
	original := []int64{5, 3, 1, 4, 2}
	backup := append([]int64(nil), original...)
	_ = intPercentile(original, 95)
	for i, v := range backup {
		if original[i] != v {
			t.Fatalf("intPercentile mutated input: original[%d]=%d, expected %d", i, original[i], v)
		}
	}
}

func TestFloatPercentile(t *testing.T) {
	tests := []struct {
		name string
		vals []float64
		p    float64
		want float64
	}{
		{"empty", nil, 50, 0},
		{"single", []float64{3.14}, 50, 3.14},
		{"three_p50", []float64{1.0, 2.0, 3.0}, 50, 2.0},
		{"three_p95", []float64{1.0, 2.0, 3.0}, 95, 3.0},
		{"unsorted", []float64{3.0, 1.0, 2.0}, 50, 2.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := floatPercentile(tc.vals, tc.p)
			if got != tc.want {
				t.Fatalf("floatPercentile(vals=%v, p=%.0f) = %f, want %f", tc.vals, tc.p, got, tc.want)
			}
		})
	}
}

func TestIntDistribution(t *testing.T) {
	vals := []int64{100, 200, 300, 400, 500}
	dist := intDistribution(vals)
	if dist.Min != 100 {
		t.Fatalf("Min = %d, want 100", dist.Min)
	}
	if dist.Max != 500 {
		t.Fatalf("Max = %d, want 500", dist.Max)
	}
	if dist.P50 != 300 {
		t.Fatalf("P50 = %d, want 300", dist.P50)
	}
	if dist.P95 != 500 {
		t.Fatalf("P95 = %d, want 500", dist.P95)
	}
	if len(dist.Values) != 5 {
		t.Fatalf("Values len = %d, want 5", len(dist.Values))
	}
	// Values should be sorted
	for i := 1; i < len(dist.Values); i++ {
		if dist.Values[i] < dist.Values[i-1] {
			t.Fatalf("Values not sorted at index %d", i)
		}
	}
}

func TestFloatDistribution(t *testing.T) {
	vals := []float64{2.0, 1.0, 3.0}
	dist := floatDistribution(vals)
	if dist.Min != 1.0 {
		t.Fatalf("Min = %f, want 1.0", dist.Min)
	}
	if dist.Max != 3.0 {
		t.Fatalf("Max = %f, want 3.0", dist.Max)
	}
	if dist.P50 != 2.0 {
		t.Fatalf("P50 = %f, want 2.0", dist.P50)
	}
	if len(dist.Values) != 3 {
		t.Fatalf("Values len = %d, want 3", len(dist.Values))
	}
	// Values should be sorted
	for i := 1; i < len(dist.Values); i++ {
		if dist.Values[i] < dist.Values[i-1] {
			t.Fatalf("Values not sorted at index %d", i)
		}
	}
}

func TestIntDistributionDoesNotMutateInput(t *testing.T) {
	original := []int64{5, 3, 1, 4, 2}
	backup := append([]int64(nil), original...)
	_ = intDistribution(original)
	for i, v := range backup {
		if original[i] != v {
			t.Fatalf("intDistribution mutated input: original[%d]=%d, expected %d", i, original[i], v)
		}
	}
}

func TestMinInt64(t *testing.T) {
	if minInt64(nil) != 0 {
		t.Fatal("minInt64(nil) should be 0")
	}
	if minInt64([]int64{5, 3, 7, 1, 9}) != 1 {
		t.Fatal("minInt64 unexpected")
	}
}

func TestMaxInt64(t *testing.T) {
	if maxInt64(nil) != 0 {
		t.Fatal("maxInt64(nil) should be 0")
	}
	if maxInt64([]int64{5, 3, 7, 1, 9}) != 9 {
		t.Fatal("maxInt64 unexpected")
	}
}

func TestMinFloat64(t *testing.T) {
	if minFloat64(nil) != 0 {
		t.Fatal("minFloat64(nil) should be 0")
	}
	if minFloat64([]float64{5.0, 3.0, 7.0}) != 3.0 {
		t.Fatal("minFloat64 unexpected")
	}
}

func TestMaxFloat64(t *testing.T) {
	if maxFloat64(nil) != 0 {
		t.Fatal("maxFloat64(nil) should be 0")
	}
	if maxFloat64([]float64{5.0, 3.0, 7.0}) != 7.0 {
		t.Fatal("maxFloat64 unexpected")
	}
}

func TestEnvBoundedInt(t *testing.T) {
	// With no env var set, should return default.
	if v := envBoundedInt(&testing.T{}, "MOTOR_AUTONOMO_TEST_NONEXISTENT_KEY_12345", 7, 1, 100); v != 7 {
		t.Fatalf("expected default 7, got %d", v)
	}
}

func TestBuildWalCheckpointAggregate(t *testing.T) {
	samples := map[walCheckpointPairKey]*walCheckpointSamples{
		{"FULL", 1000, 500}: {
			passive:  []int64{10, 20, 30},
			truncate: []int64{5, 10, 15},
		},
		{"NORMAL", -1, 2000}: {
			passive:  []int64{100, 200},
			truncate: []int64{50, 80},
		},
	}
	report := buildWalCheckpointAggregate(3, samples)
	if report.SchemaVersion != "motor-autonomo.sqlite-wal-checkpoint-aggregate.v1" {
		t.Fatalf("SchemaVersion = %s", report.SchemaVersion)
	}
	if report.RepeatCount != 3 {
		t.Fatalf("RepeatCount = %d, want 3", report.RepeatCount)
	}
	if len(report.Pairs) != 2 {
		t.Fatalf("Pairs len = %d, want 2", len(report.Pairs))
	}
	// Pairs should be sorted: FULL,1000,500 comes before NORMAL,-1,2000
	p0 := report.Pairs[0]
	if p0.Synchronous != "FULL" || p0.WalAutoCheckpoint != 1000 || p0.NumCommits != 500 {
		t.Fatalf("first pair = %+v", p0)
	}
	if p0.SampleCount != 3 {
		t.Fatalf("SampleCount = %d, want 3", p0.SampleCount)
	}
	// passive: 10,20,30 → p50=20, p95=30, min=10, max=30
	if p0.PassiveReopenMs.P50 != 20 {
		t.Fatalf("PassiveReopenMs.P50 = %d, want 20", p0.PassiveReopenMs.P50)
	}
	// truncate: 5,10,15 → p50=10
	if p0.TruncateReopenMs.P50 != 10 {
		t.Fatalf("TruncateReopenMs.P50 = %d, want 10", p0.TruncateReopenMs.P50)
	}
	// speedup: 10/5=2.0, 20/10=2.0, 30/15=2.0 → p50=2.0
	if p0.Speedup.P50 != 2.0 {
		t.Fatalf("Speedup.P50 = %f, want 2.0", p0.Speedup.P50)
	}
}
