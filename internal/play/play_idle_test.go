package play

import (
	"math"
	"testing"

	"ttrack/internal/cast"
)

func TestClampIdleGaps(t *testing.T) {
	events := []cast.Event{
		{Time: 0.5, Type: "o", Data: "a"},
		{Time: 5.5, Type: "o", Data: "b"},  // 5.0s gap → should clamp to 2.0
		{Time: 5.6, Type: "o", Data: "c"},  // 0.1s gap → unchanged
		{Time: 10.6, Type: "o", Data: "d"}, // 5.0s gap → should clamp to 2.0
	}
	got := clampIdleGaps(events, 2.0)

	want := []float64{0.5, 2.5, 2.6, 4.6}
	for i, ev := range got {
		if math.Abs(ev.Time-want[i]) > 1e-9 {
			t.Errorf("event[%d]: got %.2f want %.2f", i, ev.Time, want[i])
		}
	}
	// originals must be untouched (clamp returns a copy)
	if events[1].Time != 5.5 {
		t.Error("clampIdleGaps mutated input slice")
	}
}

func TestClampIdleGapsNoop(t *testing.T) {
	events := []cast.Event{{Time: 1.0}, {Time: 2.0}}
	got := clampIdleGaps(events, 0) // maxIdle==0 → no-op
	if len(got) != 2 || got[0].Time != 1.0 || got[1].Time != 2.0 {
		t.Error("expected no-op when maxIdle==0")
	}
}
