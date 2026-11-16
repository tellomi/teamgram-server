package mysql_dao

import (
	"math"
	"testing"
)

func TestSavedDeleteUpperBoundTreatsZeroAsAll(t *testing.T) {
	if savedDeleteUpperBound(0) != math.MaxInt32 || savedDeleteUpperBound(-1) != math.MaxInt32 {
		t.Fatal("max_id 0 must delete the whole saved dialog")
	}
	if savedDeleteUpperBound(18) != 18 {
		t.Fatal("positive max_id is the inclusive cap")
	}
}
