package push

// NotifySettings is one user_notify_settings row.
// Found is false when the user has no row. A stored value of -1 means unset
// and falls through to the global (users/chats/channels) row, then to the default.
type NotifySettings struct {
	Found        bool
	ShowPreviews int32
	Silent       int32
	MuteUntil    int32
	Sound        string
}

// Decision is whether a new message should produce a visible push.
type Decision struct {
	Send    bool
	Preview bool
	Silent  bool
	Sound   string
}

// Decide applies per-chat settings over the global settings.
// A mention is delivered even when the chat is muted.
// mute_until <= 0 means not muted. A future unix time means muted.
func Decide(now int64, peer, global NotifySettings, mentioned, messageSilent bool) Decision {
	d := Decision{
		Send:    true,
		Preview: true,
		Sound:   "default",
	}
	if muted(resolveInt(peer, global, peer.MuteUntil, global.MuteUntil, -1), now) && !mentioned {
		d.Send = false
		return d
	}
	if resolveInt(peer, global, peer.ShowPreviews, global.ShowPreviews, 1) == 0 {
		d.Preview = false
	}
	d.Silent = messageSilent || resolveInt(peer, global, peer.Silent, global.Silent, 0) == 1
	if sound := resolveString(peer, global); sound != "" && sound != "-1" {
		d.Sound = sound
	}
	if d.Silent {
		d.Sound = ""
	}
	return d
}

func resolveInt(peer, global NotifySettings, peerVal, globalVal, fallback int32) int32 {
	if peer.Found && peerVal != -1 {
		return peerVal
	}
	if global.Found && globalVal != -1 {
		return globalVal
	}
	return fallback
}

func resolveString(peer, global NotifySettings) string {
	if peer.Found && peer.Sound != "" && peer.Sound != "-1" {
		return peer.Sound
	}
	if global.Found && global.Sound != "" && global.Sound != "-1" {
		return global.Sound
	}
	return ""
}

func muted(until int32, now int64) bool {
	if until <= 0 {
		return false
	}
	return int64(until) > now
}
