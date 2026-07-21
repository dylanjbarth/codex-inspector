package metrics

import (
	"fmt"
	"testing"
)

func TestCapacitySeriesShareResponsePointBudget(t *testing.T) {
	series := make([]CapacitySeries, 2)
	for i := range series {
		series[i].ResetBoundary = fmt.Sprintf("reset-%d", i)
		for point := 0; point < 1500; point++ {
			series[i].Points = append(series[i].Points, CapacityPoint{ObservedAt: fmt.Sprintf("2026-07-%02dT00:%02d:00Z", i+1, point%60), UsedPercent: float64(point % 100)})
		}
	}

	bounded := downsampleCapacitySeries(series, MaxCapacityPoints)
	total := 0
	for i, item := range bounded {
		total += len(item.Points)
		if item.Points[0] != series[i].Points[0] || item.Points[len(item.Points)-1] != series[i].Points[len(series[i].Points)-1] {
			t.Fatalf("series %d did not preserve endpoints", i)
		}
	}
	if total != MaxCapacityPoints {
		t.Fatalf("capacity points=%d, want %d", total, MaxCapacityPoints)
	}
}
