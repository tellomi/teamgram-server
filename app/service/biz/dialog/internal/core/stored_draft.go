package core

import (
	"fmt"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// appendStoredDraft adds the draft stored on one dialogs row.
// messages.saveDraft writes that JSON onto dialogs.draft_message_data.
// A blank column is not a draft. JSON that is not a draft is reported
// and left out, so one bad row does not hide the others.
func appendStoredDraft(dst []*dialog.PeerWithDraftMessage, row *dataobject.DialogsDO) ([]*dialog.PeerWithDraftMessage, error) {
	item, err := peerWithStoredDraft(row)
	if err != nil || item == nil {
		return dst, err
	}
	return append(dst, item), nil
}

func peerWithStoredDraft(row *dataobject.DialogsDO) (*dialog.PeerWithDraftMessage, error) {
	if row == nil {
		return nil, nil
	}
	draft, err := decodeStoredDraft(row.DraftMessageData)
	if err != nil || draft == nil {
		return nil, err
	}
	return dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
		Peer:  mtproto.MakePeer(row.PeerType, row.PeerId),
		Draft: draft,
	}).To_PeerWithDraftMessage(), nil
}

// decodeStoredDraft reads the JSON dialog.saveDraftMessage stores.
// "null" is the column default for a dialog that has no draft.
func decodeStoredDraft(raw string) (*mtproto.DraftMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}

	draft := &mtproto.DraftMessage{}
	if err := jsonx.UnmarshalFromString(raw, &draft); err != nil {
		return nil, fmt.Errorf("decode draft: %w", err)
	}
	if draft == nil {
		return nil, nil
	}

	switch draft.GetPredicateName() {
	case mtproto.Predicate_draftMessage, mtproto.Predicate_draftMessageEmpty:
	default:
		if !storedDraftHasBody(draft) {
			return nil, nil
		}
		draft.PredicateName = mtproto.Predicate_draftMessage
	}
	draft.SanitizeEntities()
	return draft, nil
}

// storedDraftHasBody reports a draftMessage that was stored without
// predicate_name. An empty object is not a draft.
func storedDraftHasBody(draft *mtproto.DraftMessage) bool {
	if draft == nil {
		return false
	}
	if draft.GetMessage() != "" || len(draft.GetEntities()) > 0 {
		return true
	}
	if draft.GetNoWebpage() || draft.GetInvertMedia() {
		return true
	}
	if draft.GetReplyTo() != nil || draft.GetReplyToMsgId() != nil {
		return true
	}
	if draft.GetMedia() != nil || draft.GetEffect() != nil || draft.GetSuggestedPost() != nil || draft.GetRichMessage() != nil {
		return true
	}
	return draft.GetDate_INT32() != 0
}
