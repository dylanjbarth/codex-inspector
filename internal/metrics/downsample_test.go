package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestDownsampleCapacityPointsPreservesEndpointsAndSignificantChange(t *testing.T) {
	points := make([]CapacityPoint, 100)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := range points {
		points[i] = CapacityPoint{
			ObservedAt:  start.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			UsedPercent: 20,
		}
	}
	points[51].UsedPercent = 95

	got := downsampleCapacityPoints(points, 10)
	if len(got) != 10 {
		t.Fatalf("points=%d, want 10", len(got))
	}
	if got[0] != points[0] || got[len(got)-1] != points[len(points)-1] {
		t.Fatalf("endpoints were not preserved: first=%+v last=%+v", got[0], got[len(got)-1])
	}
	foundSpike := false
	for i, point := range got {
		if i > 0 && point.ObservedAt <= got[i-1].ObservedAt {
			t.Fatalf("sample is not ordered at %d: %+v", i, got)
		}
		foundSpike = foundSpike || point.UsedPercent == 95
	}
	if !foundSpike {
		t.Fatalf("significant change was not preserved: %+v", got)
	}
	if again := downsampleCapacityPoints(points, 10); !reflect.DeepEqual(got, again) {
		t.Fatalf("sampling is not deterministic: first=%+v second=%+v", got, again)
	}
}

func TestDownsampleCapacityPointsHonorsSmallLimits(t *testing.T) {
	points := []CapacityPoint{{ObservedAt: "first"}, {ObservedAt: "middle"}, {ObservedAt: "last"}}
	if got := downsampleCapacityPoints(points, 0); got != nil {
		t.Fatalf("zero limit=%+v", got)
	}
	if got := downsampleCapacityPoints(points, 1); len(got) != 1 || got[0] != points[2] {
		t.Fatalf("one point=%+v", got)
	}
	if got := downsampleCapacityPoints(points, 2); !reflect.DeepEqual(got, []CapacityPoint{points[0], points[2]}) {
		t.Fatalf("two points=%+v", got)
	}
}
