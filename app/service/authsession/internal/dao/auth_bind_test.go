package dao

import "testing"

func TestAuthBindConflicts(t *testing.T) {
	if authBindConflicts(nil, 7) {
		t.Fatal("no active owner is not a conflict")
	}
	if authBindConflicts([]int64{7}, 7) {
		t.Fatal("the same active owner is not a conflict")
	}
	if authBindConflicts([]int64{7, 7}, 7) {
		t.Fatal("repeated same owner is not a conflict")
	}
	if !authBindConflicts([]int64{8}, 7) {
		t.Fatal("a different active owner is a conflict")
	}
	if !authBindConflicts([]int64{7, 8}, 7) {
		t.Fatal("another active owner beside the requested user is a conflict")
	}
}

func TestAuthDataCacheKeyCarriesGeneration(t *testing.T) {
	if genAuthDataCacheKey(42, 0) == genAuthDataCacheKey(42, 1) {
		t.Fatal("generation 0 and 1 share a cache slot")
	}
	if genAuthBindGenKey(42) == genAuthDataCacheKey(42, 0) {
		t.Fatal("generation counter and cache slot use the same redis key")
	}
}
