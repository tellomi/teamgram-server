package dataobject

// DialogTopicDraftDO is one forum-topic draft.
// top_msg_id is never 0: the dialog draft stays on dialogs.draft_message_data.
type DialogTopicDraftDO struct {
	Id               int64  `db:"id" json:"id"`
	UserId           int64  `db:"user_id" json:"user_id"`
	PeerType         int32  `db:"peer_type" json:"peer_type"`
	PeerId           int64  `db:"peer_id" json:"peer_id"`
	TopMsgId         int32  `db:"top_msg_id" json:"top_msg_id"`
	DraftType        int32  `db:"draft_type" json:"draft_type"`
	DraftMessageData string `db:"draft_message_data" json:"draft_message_data"`
	Date2            int64  `db:"date2" json:"date2"`
}
