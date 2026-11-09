package core

// nonNegativeSeq is the seq written into updates.State. Zero is a real
// starting value. A missing counter used to be stored as -1, and that value
// leaked into messages.getPeerDialogs.
func nonNegativeSeq(seq int32) int32 {
	if seq < 0 {
		return 0
	}
	return seq
}
