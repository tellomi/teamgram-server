package core

import (
	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesGetSuggestedDialogFilters
// messages.getSuggestedDialogFilters#a29cd42c = Vector<DialogFilterSuggested>;
// https://core.telegram.org/method/messages.getSuggestedDialogFilters
// https://core.telegram.org/api/folders
//
// The folders page says clients show suggested folder combinations built from
// the dialogFilter inclusion flags. These five are that static catalog.
// Accepting one still goes through messages.updateDialogFilter, which assigns
// a real id. The suggested objects themselves use id 0.
func (c *FoldersCore) MessagesGetSuggestedDialogFilters(in *mtproto.TLMessagesGetSuggestedDialogFilters) (*mtproto.Vector_DialogFilterSuggested, error) {
	return suggestedDialogFilters(), nil
}

func suggestedDialogFilters() *mtproto.Vector_DialogFilterSuggested {
	return &mtproto.Vector_DialogFilterSuggested{
		Datas: []*mtproto.DialogFilterSuggested{
			suggest("Unread", "New messages from all chats.", &mtproto.DialogFilter{
				Contacts: true, NonContacts: true, Groups: true, Broadcasts: true, Bots: true, ExcludeRead: true,
			}),
			suggest("Personal", "Only messages from personal chats.", &mtproto.DialogFilter{Contacts: true, NonContacts: true}),
			suggest("Groups", "Messages from group chats.", &mtproto.DialogFilter{Groups: true}),
			suggest("Channels", "Messages from broadcast channels.", &mtproto.DialogFilter{Broadcasts: true}),
			suggest("Bots", "Conversations with bots.", &mtproto.DialogFilter{Bots: true}),
		},
	}
}

func suggest(title, description string, flags *mtproto.DialogFilter) *mtproto.DialogFilterSuggested {
	flags.Title_TEXTWITHENTITIES = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text:     title,
		Entities: []*mtproto.MessageEntity{},
	}).To_TextWithEntities()
	flags.Title = title
	flags.Emoticon = wrapperspb.String(suggestEmoticon(title))
	filter := mtproto.MakeTLDialogFilter(flags).To_DialogFilter()
	return mtproto.MakeTLDialogFilterSuggested(&mtproto.DialogFilterSuggested{
		Filter:      filter,
		Description: description,
	}).To_DialogFilterSuggested()
}

func suggestEmoticon(title string) string {
	switch title {
	case "Unread":
		return "💬"
	case "Personal":
		return "👋"
	case "Groups":
		return "👥"
	case "Channels":
		return "📢"
	case "Bots":
		return "🤖"
	default:
		return ""
	}
}
