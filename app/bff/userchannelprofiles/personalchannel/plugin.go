// Package personalchannel fills userFull.personal_channel_id and
// personal_channel_message from the channel the user set with
// account.updatePersonalChannel.
package personalchannel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// Plugin is users.getFullUser's PersonalChannelPlugin.
type Plugin struct{}

// GetPersonalChannel returns the channel card and its latest message id
// when viewer may see the channel. A private channel stays off a profile
// the viewer cannot open.
func (Plugin) GetPersonalChannel(ctx context.Context, channelID, viewer int64) (*mtproto.Chat, int32) {
	chat, top, err := channel.Shared().PersonalChannelCard(ctx, viewer, channelID)
	if err != nil || chat == nil {
		return nil, 0
	}
	return chat, top
}
