package dialog

import "testing"

func TestFilterOrderStepsPlacesDefault(t *testing.T) {
	const now int64 = 1_700_000_000
	steps := FilterOrderSteps([]int32{2, DialogFilterDefaultID, 3}, now)
	if len(steps) != 3 {
		t.Fatalf("steps %d", len(steps))
	}
	base := now << 32
	wantID := []int32{2, 0, 3}
	wantUpsert := []bool{false, true, false}
	for i, step := range steps {
		if step.ID != wantID[i] || step.UpsertDefault != wantUpsert[i] {
			t.Fatalf("step %d %+v", i, step)
		}
		if step.OrderValue != base-int64(i) {
			t.Fatalf("order %d = %d", i, step.OrderValue)
		}
	}
	if steps[0].OrderValue <= steps[1].OrderValue || steps[1].OrderValue <= steps[2].OrderValue {
		t.Fatal("order does not descend from the front of the vector")
	}
}

func TestFilterOrderStepsEmpty(t *testing.T) {
	if FilterOrderSteps(nil, 10) != nil {
		t.Fatal("nil order")
	}
	if len(FilterOrderSteps([]int32{}, 10)) != 0 {
		t.Fatal("empty order")
	}
}
