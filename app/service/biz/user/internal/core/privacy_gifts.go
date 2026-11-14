package core

import (
	"encoding/json"

	"github.com/teamgram/proto/mtproto"
)

type giftDeny struct {
	Unlimited bool `json:"unlimited"`
	Limited   bool `json:"limited"`
	Unique    bool `json:"unique"`
	Premium   bool `json:"premium"`
	Channels  bool `json:"channels"`
}

func paidStars(in *mtproto.GlobalPrivacySettings) int64 {
	if in == nil || in.GetNoncontactPeersPaidStars() == nil {
		return 0
	}
	return in.GetNoncontactPeersPaidStars().GetValue()
}

func encodeGifts(in *mtproto.DisallowedGiftsSettings) string {
	if in == nil {
		return ""
	}
	raw, err := json.Marshal(giftDeny{
		Unlimited: in.GetDisallowUnlimitedStargifts(),
		Limited:   in.GetDisallowLimitedStargifts(),
		Unique:    in.GetDisallowUniqueStargifts(),
		Premium:   in.GetDisallowPremiumGifts(),
		Channels:  in.GetDisallowStargiftsFromChannels(),
	})
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodeGifts(raw string) *mtproto.DisallowedGiftsSettings {
	if raw == "" {
		return nil
	}
	var row giftDeny
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		return nil
	}
	return mtproto.MakeTLDisallowedGiftsSettings(&mtproto.DisallowedGiftsSettings{
		DisallowUnlimitedStargifts:    row.Unlimited,
		DisallowLimitedStargifts:      row.Limited,
		DisallowUniqueStargifts:       row.Unique,
		DisallowPremiumGifts:          row.Premium,
		DisallowStargiftsFromChannels: row.Channels,
	}).To_DisallowedGiftsSettings()
}
