package mtproto

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func banRights(until int32, set func(*ChatBannedRights)) *ChatBannedRights {
	r := &ChatBannedRights{UntilDate: until}
	set(r)
	return MakeTLChatBannedRights(r).To_ChatBannedRights()
}

func TestChatSendForbiddenCodesAndUntil(t *testing.T) {
	now := int32(1_700_000_000)
	forever := int32(0)
	temp := now + 3600
	photo := MakeTLMessage(&Message{Media: MakeTLMessageMediaPhoto(&MessageMedia{}).To_MessageMedia()}).To_Message()
	video := MakeTLMessage(&Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{MimeType: "video/mp4", Attributes: []*DocumentAttribute{
			MakeTLDocumentAttributeVideo(&DocumentAttribute{W: 100, H: 100}).To_DocumentAttribute(),
		}}).To_Document(),
	}).To_MessageMedia()}).To_Message()
	round := MakeTLMessage(&Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{MimeType: "video/mp4", Attributes: []*DocumentAttribute{
			MakeTLDocumentAttributeVideo(&DocumentAttribute{RoundMessage: true, W: 100, H: 100}).To_DocumentAttribute(),
		}}).To_Document(),
	}).To_MessageMedia()}).To_Message()
	voice := MakeTLMessage(&Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{Attributes: []*DocumentAttribute{
			MakeTLDocumentAttributeAudio(&DocumentAttribute{Voice: true}).To_DocumentAttribute(),
		}}).To_Document(),
	}).To_MessageMedia()}).To_Message()
	audio := MakeTLMessage(&Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{Attributes: []*DocumentAttribute{
			MakeTLDocumentAttributeAudio(&DocumentAttribute{}).To_DocumentAttribute(),
		}}).To_Document(),
	}).To_MessageMedia()}).To_Message()
	doc := MakeTLMessage(&Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{}).To_Document(),
	}).To_MessageMedia()}).To_Message()
	sticker := MakeTLMessage(&Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{Attributes: []*DocumentAttribute{
			MakeTLDocumentAttributeSticker(&DocumentAttribute{}).To_DocumentAttribute(),
		}}).To_Document(),
	}).To_MessageMedia()}).To_Message()
	gif := MakeTLMessage(&Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{MimeType: "image/gif", Thumbs: []*PhotoSize{
			MakeTLPhotoSize(&PhotoSize{}).To_PhotoSize(),
		}}).To_Document(),
	}).To_MessageMedia()}).To_Message()
	game := MakeTLMessage(&Message{Media: MakeTLMessageMediaGame(&MessageMedia{}).To_MessageMedia()}).To_Message()
	poll := MakeTLMessage(&Message{Media: MakeTLMessageMediaPoll(&MessageMedia{}).To_MessageMedia()}).To_Message()
	inline := MakeTLMessage(&Message{ViaBotId: &wrapperspb.Int64Value{Value: 7}, Message: "hi"}).To_Message()
	text := MakeTLMessage(&Message{Message: "hello"}).To_Message()
	link := MakeTLMessage(&Message{Message: "see https://example.com"}).To_Message()

	cases := []struct {
		name string
		msg  *Message
		set  func(*ChatBannedRights)
		want string
	}{
		{"write", text, func(r *ChatBannedRights) { r.SendMessages = true }, "CHAT_WRITE_FORBIDDEN"},
		{"photo", photo, func(r *ChatBannedRights) { r.SendPhotos = true }, "CHAT_SEND_PHOTOS_FORBIDDEN"},
		{"video", video, func(r *ChatBannedRights) { r.SendVideos = true }, "CHAT_SEND_VIDEOS_FORBIDDEN"},
		{"round", round, func(r *ChatBannedRights) { r.SendRoundvideos = true }, "CHAT_SEND_ROUNDVIDEOS_FORBIDDEN"},
		{"voice", voice, func(r *ChatBannedRights) { r.SendVoices = true }, "CHAT_SEND_VOICES_FORBIDDEN"},
		{"audio", audio, func(r *ChatBannedRights) { r.SendAudios = true }, "CHAT_SEND_AUDIOS_FORBIDDEN"},
		{"doc", doc, func(r *ChatBannedRights) { r.SendDocs = true }, "CHAT_SEND_DOCS_FORBIDDEN"},
		{"sticker", sticker, func(r *ChatBannedRights) { r.SendStickers = true }, "CHAT_SEND_STICKERS_FORBIDDEN"},
		{"gif", gif, func(r *ChatBannedRights) { r.SendGifs = true }, "CHAT_SEND_GIFS_FORBIDDEN"},
		{"game", game, func(r *ChatBannedRights) { r.SendGames = true }, "CHAT_SEND_GAME_FORBIDDEN"},
		{"poll", poll, func(r *ChatBannedRights) { r.SendPolls = true }, "CHAT_SEND_POLL_FORBIDDEN"},
		{"inline", inline, func(r *ChatBannedRights) { r.SendInline = true }, "CHAT_SEND_INLINE_FORBIDDEN"},
		{"media", photo, func(r *ChatBannedRights) { r.SendMedia = true }, "CHAT_SEND_MEDIA_FORBIDDEN"},
		{"plain", text, func(r *ChatBannedRights) { r.SendPlain = true }, "CHAT_SEND_PLAIN_FORBIDDEN"},
		{"webpage", link, func(r *ChatBannedRights) { r.EmbedLinks = true }, "CHAT_SEND_WEBPAGE_FORBIDDEN"},
	}
	for _, tc := range cases {
		err := ChatSendForbidden(now, false, []*ChatBannedRights{banRights(forever, tc.set)}, tc.msg)
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), tc.want+"_") {
			t.Fatalf("%s forever: %v", tc.name, err)
		}
		err = ChatSendForbidden(now, false, []*ChatBannedRights{banRights(temp, tc.set)}, tc.msg)
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), tc.want+"_") {
			t.Fatalf("%s temporary ban must stay the plain code: %v", tc.name, err)
		}
		if err := ChatSendForbidden(now, true, []*ChatBannedRights{banRights(forever, tc.set)}, tc.msg); err != nil {
			t.Fatalf("%s admin: %v", tc.name, err)
		}
	}
	past := banRights(now-10, func(r *ChatBannedRights) { r.SendPhotos = true })
	if err := ChatSendForbidden(now, false, []*ChatBannedRights{past}, photo); err != nil {
		t.Fatalf("expired ban still blocks: %v", err)
	}
	userBan := banRights(forever, func(r *ChatBannedRights) { r.SendPhotos = true })
	if err := ChatSendForbidden(now, false, []*ChatBannedRights{{}, userBan}, photo); err == nil || !strings.Contains(err.Error(), "CHAT_SEND_PHOTOS_FORBIDDEN") {
		t.Fatalf("per-user ban: %v", err)
	}
	service := MakeTLMessageService(&Message{Action: MakeTLMessageActionPinMessage(&MessageAction{}).To_MessageAction()}).To_Message()
	if err := ChatSendForbidden(now, false, []*ChatBannedRights{banRights(forever, func(r *ChatBannedRights) { r.SendPlain = true })}, service); err != nil {
		t.Fatalf("service message: %v", err)
	}
}

func TestBasicGroupAndPinTopics(t *testing.T) {
	now := int32(1_700_000_000)
	rights := banRights(0, func(r *ChatBannedRights) {
		r.SendPhotos = true
		r.PinMessages = true
		r.ManageTopics = true
	})
	chat := &MutableChat{
		Chat: &ImmutableChat{DefaultBannedRights: rights},
		ChatParticipants: []*ImmutableChatParticipant{
			{UserId: 2, ParticipantType: ChatMemberNormal, State: ChatMemberStateNormal},
			{UserId: 1, ParticipantType: ChatMemberCreator, State: ChatMemberStateNormal},
		},
	}
	photo := MakeTLMessage(&Message{Media: MakeTLMessageMediaPhoto(&MessageMedia{}).To_MessageMedia()}).To_Message()
	if err := BasicGroupSendForbidden(chat, 2, now, []*Message{photo}); err == nil || !strings.Contains(err.Error(), "CHAT_SEND_PHOTOS_FORBIDDEN") {
		t.Fatalf("member photo: %v", err)
	}
	if err := BasicGroupSendForbidden(chat, 1, now, []*Message{photo}); err != nil {
		t.Fatalf("creator photo: %v", err)
	}
	if err := MemberPinForbidden(now, false, []*ChatBannedRights{rights}); err != ErrChatAdminRequired {
		t.Fatalf("pin: %v", err)
	}
	if err := MemberPinForbidden(now, true, []*ChatBannedRights{rights}); err != nil {
		t.Fatalf("admin pin: %v", err)
	}
	open := banRights(0, func(*ChatBannedRights) {})
	if err := MemberPinForbidden(now, false, []*ChatBannedRights{open}); err != nil {
		t.Fatalf("members may pin: %v", err)
	}
	if err := MemberTopicsForbidden(now, false, []*ChatBannedRights{rights}); err != ErrChatAdminRequired {
		t.Fatalf("topics: %v", err)
	}
	if err := MemberTopicsForbidden(now, false, []*ChatBannedRights{open}); err != nil {
		t.Fatalf("members may create topics: %v", err)
	}
}
