package core

// createChatFloodWait is how many seconds messages.createChat must still
// wait. createChatFlood is the gap after the creator's previous group.
// Zero means the gap has already passed.
//
// The old code returned the seconds already elapsed. A create 0 seconds
// after the last one answered FLOOD_WAIT_0, and each retry was told to wait
// only the time it had already spent, so four client retries were still
// inside the 10 second gap and the last answer was FLOOD_WAIT_7.
func createChatFloodWait(now, last int64) int32 {
	elapsed := now - last
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed >= createChatFlood {
		return 0
	}
	remain := createChatFlood - elapsed
	if remain < 1 {
		remain = 1
	}
	return int32(remain)
}
