package usernamepolicy

import (
	"context"
	"os"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/contentsafety"
	"github.com/zeromicro/go-zero/core/logx"
)

// EnvEnabled is the feature flag. Only the value 1 turns the policy on.
const EnvEnabled = "TELLOMI_USERNAME_POLICY"

// Enabled reports whether username checks consult the lexicon.
func Enabled() bool { return os.Getenv(EnvEnabled) == "1" }

// RejectUser checks a personal username. The owning appendix D user is allowed.
func RejectUser(ctx context.Context, userID int64, username string) error {
	return reject(ctx, username, mtproto.PEER_USER, userID, FieldUsername)
}

// RejectChannel checks a channel or supergroup public username against
// both the username and slug fields. Slug-only reserves (qq, x, jd) apply
// here. The owning appendix D channel is allowed.
func RejectChannel(ctx context.Context, channelID int64, username string) error {
	return reject(ctx, username, mtproto.PEER_CHANNEL, channelID, FieldUsername, FieldSlug)
}

// RejectBot checks a bot username against the username and bot_username fields.
// userID is the human managing the bot. An appendix D user may pass the
// policy for their own exact name; a different peer still loses to the
// username table.
func RejectBot(ctx context.Context, userID int64, username string) error {
	return reject(ctx, username, mtproto.PEER_USER, userID, FieldUsername, FieldBotUsername)
}

// RejectDisplay checks a nickname, bio, group or channel title, group or
// channel description, or bot name and description against the display_name
// field, then Aliyun or Bailian text moderation when that flag is on. A hit
// returns FIRSTNAME_INVALID. account.updateProfile maps a last-name hit to
// LASTNAME_INVALID. The rule id stays in the log. An empty string is
// allowed. The appendix D owner of an exact reserved username may use that
// string as their display name. Moderation still runs for them. A moderation
// outage allows the name. Chat messages are not checked.
func RejectDisplay(ctx context.Context, peerType int32, peerID int64, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if Enabled() && !officialExact(Normalize(text), peerType, peerID) {
		v := embeddedEngine().Check(text, FieldDisplayName, []string{"global", "cn"})
		if !v.IsAllowed() {
			if v.RuleID != "" {
				logx.WithContext(ctx).Errorf("usernamepolicy: denied field=%s rule_id=%s", FieldDisplayName, v.RuleID)
			} else {
				logx.WithContext(ctx).Errorf("usernamepolicy: denied field=%s outcome=%s", FieldDisplayName, v.Outcome)
			}
			return mtproto.ErrFirstnameInvalid
		}
	}
	if contentsafety.Decide(ctx, text) == contentsafety.Blocked {
		return mtproto.ErrFirstnameInvalid
	}
	return nil
}

func reject(ctx context.Context, username string, peerType int32, peerID int64, fields ...string) error {
	if !Enabled() {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	normalized := Normalize(username)
	if officialExact(normalized, peerType, peerID) {
		return nil
	}
	eng := embeddedEngine()
	regions := []string{"global", "cn"}
	for _, field := range fields {
		v := eng.Check(username, field, regions)
		if v.IsAllowed() {
			continue
		}
		if v.RuleID != "" {
			logx.WithContext(ctx).Errorf("usernamepolicy: denied field=%s rule_id=%s", field, v.RuleID)
		} else {
			logx.WithContext(ctx).Errorf("usernamepolicy: denied field=%s outcome=%s", field, v.Outcome)
		}
		return mtproto.ErrUsernameOccupied
	}
	return nil
}
