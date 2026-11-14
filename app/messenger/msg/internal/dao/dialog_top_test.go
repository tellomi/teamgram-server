package dao

import "testing"

func TestRemainingDialogTopFallsBackToPrevious(t *testing.T) {
	newest := []dialogTopCandidate{{ID: 5, Date: 50}, {ID: 4, Date: 40}, {ID: 1, Date: 10}}
	id, date := remainingDialogTop(newest, []int32{5})
	if id != 4 || date != 40 {
		t.Fatalf("top %d date %d", id, date)
	}
	id, date = remainingDialogTop([]dialogTopCandidate{{ID: 12, Date: 12}, {ID: 10, Date: 10}}, []int32{12})
	if id != 10 || date != 10 {
		t.Fatalf("gapped top %d date %d", id, date)
	}
}

func TestRemainingDialogTopIgnoresOlderDelete(t *testing.T) {
	newest := []dialogTopCandidate{{ID: 5, Date: 50}, {ID: 4, Date: 40}, {ID: 3, Date: 30}}
	id, date := remainingDialogTop(newest, []int32{4})
	if id != 5 || date != 50 {
		t.Fatalf("top %d date %d", id, date)
	}
}

func TestRemainingDialogTopEmptyWhenEveryMessageGoes(t *testing.T) {
	newest := []dialogTopCandidate{{ID: 2, Date: 20}, {ID: 1, Date: 10}}
	id, date := remainingDialogTop(newest, []int32{2, 1})
	if id != 0 || date != 0 {
		t.Fatalf("top %d date %d", id, date)
	}
	if !deletedTouchesTop(2, []int32{2}) {
		t.Fatal("deleting the top should recompute")
	}
	if deletedTouchesTop(5, []int32{4, 1}) {
		t.Fatal("an older delete should leave the top")
	}
}
