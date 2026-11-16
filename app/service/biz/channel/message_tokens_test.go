package channel

import "testing"

func TestMessageHasTokensIgnoresOrder(t *testing.T) {
	if !messageHasTokens("Quick brown Fox", "fox quick") {
		t.Fatal("order")
	}
	if messageHasTokens("Quick brown Fox", "fox weather") {
		t.Fatal("missing word")
	}
	if !messageHasTokens("anything", "  ") {
		t.Fatal("blank query")
	}
}
