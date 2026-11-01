package message

// SavedHistoryDeleteList is an internal MessageGetSavedHistoryMessages limit.
// It lists up to 100 saved messages for messages.deleteSavedHistory and does
// not delete them. A client limit is never negative.
const SavedHistoryDeleteList int32 = -3
