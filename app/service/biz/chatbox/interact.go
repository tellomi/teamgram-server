package chatbox

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/report"
	"github.com/teamgram/teamgram-server/pkg/reportcopy"
)

// Message reports, delivery acks, read metrics, music listens, and paid
// media reveal. Each method returns the constructor the layer schema names
// for that RPC. Reports are stored for the account that filed them.

const reportCap = 200

type reportItem struct {
	User     int64   `json:"user"`
	PeerType int32   `json:"peer_type"`
	PeerID   int64   `json:"peer_id"`
	MsgIDs   []int32 `json:"msg_ids,omitempty"`
	Option   string  `json:"option,omitempty"`
	Message  string  `json:"message,omitempty"`
	Kind     string  `json:"kind"`
	Extra    int64   `json:"extra,omitempty"`
}

func signalKey() string { return "signals" }

func paidKey(user int64, peer *mtproto.PeerUtil, msgID int32) string {
	return "paid:" + transKey(user, peer) + ":" + itoa(int(msgID))
}

type paidDoc struct {
	Media []*mtproto.MessageExtendedMedia `json:"media,omitempty"`
}

// ReportMessages walks the messages.report option tree.
// An empty option asks the client to choose. "other" asks for a comment.
// Any other known option, or "other" plus a comment, stores the report and
// returns reportResultReported.
// https://core.telegram.org/method/messages.report
func (b *Book) ReportMessages(ctx context.Context, user int64, peer *mtproto.PeerUtil, ids []int32, option []byte, message, lang string) (*mtproto.ReportResult, error) {
	ctx = report.WithLang(ctx, lang)
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
	}
	if len(message) > 1024 {
		return nil, mtproto.ErrMessageTooLong
	}
	if len(option) == 0 {
		return reportcopy.Choose(lang, reportcopy.Message), nil
	}
	switch string(option) {
	case "spam", "violence", "pornography", "child", "drugs", "personal":
		if err := b.storeReport(ctx, user, peer, ids, string(option), message, "messages.report", 0); err != nil {
			return nil, err
		}
		return reported(), nil
	case "other":
		if message == "" {
			return mtproto.MakeTLReportResultAddComment(&mtproto.ReportResult{
				Optional: false,
				Option:   []byte("other"),
			}).To_ReportResult(), nil
		}
		if err := b.storeReport(ctx, user, peer, ids, "other", message, "messages.report", 0); err != nil {
			return nil, err
		}
		return reported(), nil
	default:
		return nil, mtproto.ErrOptionInvalid
	}
}

// ReportLegacy records the older messages.report constructor that takes a
// ReportReason and returns Bool.
func (b *Book) ReportLegacy(ctx context.Context, user int64, peer *mtproto.PeerUtil, ids []int32, reason, message string) (*mtproto.Bool, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	if err := b.storeReport(ctx, user, peer, ids, reason, message, "messages.report.legacy", 0); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// ReportReaction records messages.reportReaction.
func (b *Book) ReportReaction(ctx context.Context, user int64, peer *mtproto.PeerUtil, msgID int32, reactionPeer *mtproto.PeerUtil) error {
	if peer == nil || !peer.IsUserOrChatOrChannel() || reactionPeer == nil || !reactionPeer.IsUserOrChatOrChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	if msgID <= 0 {
		return mtproto.ErrMessageIdInvalid
	}
	sub := report.Submission{
		Source:         report.SourceReaction,
		Reporter:       user,
		PeerType:       peer.PeerType,
		PeerID:         peer.PeerId,
		ObjectIDs:      []int32{msgID},
		ReactionPeerID: reactionPeer.PeerId,
	}
	if reactionPeer.PeerType == mtproto.PEER_USER {
		sub.SubjectUserID = reactionPeer.PeerId
	} else if peer.PeerType == mtproto.PEER_USER {
		sub.SubjectUserID = peer.PeerId
	}
	_, err := report.Shared().Submit(ctx, sub)
	return err
}

// ReportReadMetrics records messages.reportReadMetrics.
func (b *Book) ReportReadMetrics(ctx context.Context, user int64, peer *mtproto.PeerUtil, metrics []*mtproto.InputMessageReadMetric) error {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	ids := make([]int32, 0, len(metrics))
	for _, m := range metrics {
		if m == nil || m.GetMsgId() <= 0 {
			return mtproto.ErrMessageIdInvalid
		}
		if m.GetTimeInViewMs() < 0 || m.GetActiveTimeInViewMs() < 0 {
			return mtproto.ErrMessageIdInvalid
		}
		ids = append(ids, m.GetMsgId())
	}
	return b.storeReport(ctx, user, peer, ids, "", "", "messages.reportReadMetrics", int64(len(ids)))
}

// ReportMusicListen records messages.reportMusicListen.
func (b *Book) ReportMusicListen(ctx context.Context, user int64, doc *mtproto.InputDocument, duration int32) error {
	if doc == nil || doc.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty || doc.GetId() == 0 {
		return mtproto.ErrDocumentInvalid
	}
	if duration < 0 {
		return mtproto.ErrMessageIdInvalid
	}
	return b.storeReport(ctx, user, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: user}, nil, "", "", "messages.reportMusicListen", doc.GetId())
}

// ReportDelivery records messages.reportMessagesDelivery.
func (b *Book) ReportDelivery(ctx context.Context, user int64, peer *mtproto.PeerUtil, ids []int32, push bool) error {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return mtproto.ErrMessageIdsEmpty
	}
	for _, id := range ids {
		if id <= 0 {
			return mtproto.ErrMessageIdInvalid
		}
	}
	extra := int64(0)
	if push {
		extra = 1
	}
	return b.storeReport(ctx, user, peer, ids, "", "", "messages.reportMessagesDelivery", extra)
}

// Reports returns delivery, listen, and read signals. User reports are not here.
func (b *Book) Reports() []reportItem {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]reportItem, len(b.signals))
	copy(out, b.signals)
	return out
}

func (b *Book) storeReport(ctx context.Context, user int64, peer *mtproto.PeerUtil, ids []int32, option, message, kind string, extra int64) error {
	switch kind {
	case "messages.report", "messages.report.legacy":
		sub := report.Submission{
			Source:    report.SourceMessages,
			Reporter:  user,
			Reason:    option,
			Message:   message,
			ObjectIDs: ids,
		}
		if peer != nil {
			sub.PeerType = peer.PeerType
			sub.PeerID = peer.PeerId
			if peer.PeerType == mtproto.PEER_USER {
				sub.SubjectUserID = peer.PeerId
			}
		}
		_, err := report.Shared().Submit(ctx, sub)
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_ = b.load(ctx, signalKey(), &b.signals)
	item := reportItem{
		User: user, MsgIDs: append([]int32(nil), ids...),
		Option: option, Message: message, Kind: kind, Extra: extra,
	}
	if peer != nil {
		item.PeerType = peer.PeerType
		item.PeerID = peer.PeerId
	}
	b.signals = append(b.signals, item)
	if len(b.signals) > reportCap {
		b.signals = b.signals[len(b.signals)-reportCap:]
	}
	return b.save(ctx, signalKey(), b.signals)
}

func reported() *mtproto.ReportResult {
	return mtproto.MakeTLReportResultReported(&mtproto.ReportResult{}).To_ReportResult()
}

// NotePaidLocked stores extended media from a paid-media message.
// Caller holds b.mu.
func (b *Book) notePaidLocked(ctx context.Context, user int64, peer *mtproto.PeerUtil, msg *mtproto.Message) {
	if msg == nil || msg.GetMedia() == nil {
		return
	}
	if msg.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaPaidMedia {
		return
	}
	media := msg.GetMedia().GetExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA()
	if len(media) == 0 {
		return
	}
	key := paidKey(user, peer, msg.GetId())
	cp := append([]*mtproto.MessageExtendedMedia(nil), media...)
	b.paid[key] = cp
	_ = b.save(ctx, key, &paidDoc{Media: cp})
}

// ExtendedMedia is messages.getExtendedMedia.
// Unknown ids return MESSAGE_ID_INVALID. A message that was stored without
// paid media returns the same error. The result is one
// updateMessageExtendedMedia per id.
func (b *Book) ExtendedMedia(ctx context.Context, user int64, peer *mtproto.PeerUtil, ids []int32) (*mtproto.Updates, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var ups []*mtproto.Update
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		key := paidKey(user, peer, id)
		media := b.paid[key]
		if media == nil {
			doc := &paidDoc{}
			_ = b.load(ctx, key, doc)
			media = doc.Media
			if len(media) > 0 {
				b.paid[key] = media
			}
		}
		if len(media) == 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		ups = append(ups, mtproto.MakeTLUpdateMessageExtendedMedia(&mtproto.Update{
			Peer_PEER:                                peer.ToPeer(),
			MsgId_INT32:                              id,
			ExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA: media,
		}).To_Update())
	}
	return mtproto.MakeUpdatesByUpdates(ups...), nil
}
