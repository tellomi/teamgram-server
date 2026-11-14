package core

// privateInboxDelivered is false when the recipient has blocked the sender.
// The sender's outbox is still written, so their client shows the message as
// sent. The blocker does not get an inbox copy. This is the official private
// chat behavior and is already what the send and edit handlers do.
func privateInboxDelivered(recipientBlockedSender bool) bool {
	return !recipientBlockedSender
}
