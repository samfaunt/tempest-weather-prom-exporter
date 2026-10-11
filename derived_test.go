package main

import (
	"math"
	"testing"
)

func f(v float64) *float64 { return &v }

func TestFeelsLike(t *testing.T) {
	for _, tc := range []struct {
		name           string
		temp, rh, wind *float64
		want           *float64
	}{
		// NWS heat index chart: 90 °F at 60% feels like 100 °F.
		{"heat index", f(32.2222), f(60), f(1), f(37.6)},
		// Environment Canada table: 0 °C with 20 km/h wind feels like -5.
		{"wind chill", f(0), f(80), f(20 / 3.6), f(-5.24)},
		{"calm cold", f(0), f(80), f(1), f(0)},
		{"cold no wind reading", f(0), f(80), nil, f(0)},
		{"mild", f(20), f(50), f(10), f(20)},
		{"no humidity", f(30), nil, f(1), nil},
		{"no temperature", nil, f(50), f(1), nil},
	} {
		got := feelsLike(tc.temp, tc.rh, tc.wind)
		if (got == nil) != (tc.want == nil) || (got != nil && math.Abs(*got-*tc.want) > 0.05) {
			t.Errorf("%s: got %v, want %v", tc.name, deref(got), deref(tc.want))
		}
	}
}

func TestSeaLevelPressure(t *testing.T) {
	if got := *seaLevelPressure(f(1000), 100); math.Abs(got-1011.94) > 0.01 {
		t.Errorf("1000 mb at 100 m: got %.2f, want 1011.94", got)
	}
	if got := *seaLevelPressure(f(1017.57), 0); math.Abs(got-1017.57) > 1e-9 {
		t.Errorf("at sea level: got %v, want station pressure", got)
	}
	if seaLevelPressure(nil, 100) != nil {
		t.Error("nil pressure: want nil")
	}
}

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
