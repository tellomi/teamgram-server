package p1rest

import "github.com/teamgram/proto/mtproto"

func factChecks(self int64, peer *mtproto.InputPeer, ids []int32) (*mtproto.Vector_FactCheck, error) {
	if _, _, err := peerOf(self, peer); err != nil {
		return nil, err
	}
	// No fact-checker writes text here. One empty factCheck per id clears
	// need_check so the client stops waiting. An empty id list is an empty vector.
	rows := make([]*mtproto.FactCheck, 0, len(ids))
	for range ids {
		rows = append(rows, mtproto.MakeTLFactCheck(&mtproto.FactCheck{Hash: 1}).To_FactCheck())
	}
	return &mtproto.Vector_FactCheck{Datas: rows}, nil
}

func editFact(self int64, peer *mtproto.InputPeer, msgID int32, text *mtproto.TextWithEntities) (*mtproto.Updates, error) {
	_ = msgID
	_ = text
	if _, _, err := peerOf(self, peer); err != nil {
		return nil, err
	}
	// appConfig.can_edit_factcheck is not set. Only independent fact-checkers
	// may edit, and that refusal is CHAT_ACTION_FORBIDDEN.
	return nil, mtproto.ErrChatActionForbidden
}

func deleteFact(self int64, peer *mtproto.InputPeer, msgID int32) (*mtproto.Updates, error) {
	return editFact(self, peer, msgID, nil)
}
