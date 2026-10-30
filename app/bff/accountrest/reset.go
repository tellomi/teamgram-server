package accountrest

// Reset clears in-process account state. Tests call it.
func Reset() {
	resetPasskeys()
	resetBrowsers()
	resetTakeouts()
	resetUsernames()
	resetPaid()
}
