package secretchat

import (
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Gap is the qts feed for one user after the cursor the client already applied.
func (s *Store) Gap(userID int64, after, limit int32) (msgs []*mtproto.EncryptedMessage, others []*mtproto.Update, last int32, more bool) {
	if s == nil || s.be == nil || userID == 0 {
		return nil, nil, 0, false
	}
	return s.be.Since(userID, after, limit)
}

// Current is the user's latest qts. updates.getState returns it.
func Current(userID int64) int32 {
	return Default().be.Current(userID)
}

// ApplyDifference merges this user's qts queue into a pts difference.
// updateNewEncryptedMessage is both a new_encrypted_message and an
// other_update, because the qts cursor lives on the update. An empty pts
// difference becomes a real difference when any encrypted update is waiting,
// so an offline client is not told there is nothing to catch up.
// https://core.telegram.org/api/updates
func ApplyDifference(userID int64, clientQts, limit int32, diff *mtproto.Updates_Difference) *mtproto.Updates_Difference {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	msgs, others, last, more := Default().Gap(userID, clientQts, limit)
	cur := Current(userID)
	if diff == nil {
		diff = mtproto.MakeTLUpdatesDifferenceEmpty(&mtproto.Updates_Difference{
			Date: int32(time.Now().Unix()),
		}).To_Updates_Difference()
	}
	if len(msgs) == 0 && len(others) == 0 {
		stampState(diff, cur)
		return diff
	}
	qts := last
	if !more && cur > qts {
		qts = cur
	}
	switch diff.GetPredicateName() {
	case mtproto.Predicate_updates_differenceEmpty:
		state := mtproto.MakeTLUpdatesState(&mtproto.Updates_State{
			Date: diff.GetDate(),
			Seq:  diff.GetSeq(),
			Qts:  qts,
		}).To_Updates_State()
		if more {
			return mtproto.MakeTLUpdatesDifferenceSlice(&mtproto.Updates_Difference{
				NewEncryptedMessages: msgs,
				OtherUpdates:         others,
				IntermediateState:    state,
			}).To_Updates_Difference()
		}
		return mtproto.MakeTLUpdatesDifference(&mtproto.Updates_Difference{
			NewEncryptedMessages: msgs,
			OtherUpdates:         others,
			State:                state,
		}).To_Updates_Difference()
	case mtproto.Predicate_updates_difference:
		diff.NewEncryptedMessages = append(diff.NewEncryptedMessages, msgs...)
		diff.OtherUpdates = append(diff.OtherUpdates, others...)
		if more {
			state := diff.GetState()
			if state == nil {
				state = mtproto.MakeTLUpdatesState(&mtproto.Updates_State{}).To_Updates_State()
			}
			state.Qts = qts
			return mtproto.MakeTLUpdatesDifferenceSlice(&mtproto.Updates_Difference{
				NewMessages:          diff.NewMessages,
				NewEncryptedMessages: diff.NewEncryptedMessages,
				OtherUpdates:         diff.OtherUpdates,
				Chats:                diff.Chats,
				Users:                diff.Users,
				IntermediateState:    state,
			}).To_Updates_Difference()
		}
		stampState(diff, qts)
		return diff
	case mtproto.Predicate_updates_differenceSlice:
		diff.NewEncryptedMessages = append(diff.NewEncryptedMessages, msgs...)
		diff.OtherUpdates = append(diff.OtherUpdates, others...)
		if diff.IntermediateState != nil {
			diff.IntermediateState.Qts = qts
		}
		return diff
	default:
		stampState(diff, qts)
		return diff
	}
}

func stampState(diff *mtproto.Updates_Difference, qts int32) {
	if diff == nil || qts == 0 {
		return
	}
	if diff.State != nil && diff.State.Qts < qts {
		diff.State.Qts = qts
	}
	if diff.IntermediateState != nil && diff.IntermediateState.Qts < qts {
		diff.IntermediateState.Qts = qts
	}
}
