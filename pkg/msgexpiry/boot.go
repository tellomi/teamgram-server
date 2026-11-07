package msgexpiry

func init() {
	// Process start only. A test that sets the sweep switch later does not
	// enter this loop. The loop walks the in-memory ledger and does not
	// query MySQL, Redis, or backup storage.
	Guard(Default())
}
