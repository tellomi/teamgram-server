package book

import (
	"strings"
	"unicode"

	"github.com/teamgram/proto/mtproto"
)

func (b *Book) PremiumGiftCodeOptions(self int64, boostPeer *mtproto.InputPeer) (*mtproto.Vector_PremiumGiftCodeOption, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	if boostPeer != nil {
		if _, _, err := parsePeer(self, boostPeer); err != nil {
			return nil, err
		}
	}
	return &mtproto.Vector_PremiumGiftCodeOption{Datas: []*mtproto.PremiumGiftCodeOption{}}, nil
}

func (b *Book) StarsGiveawayOptions(self int64) (*mtproto.Vector_StarsGiveawayOption, error) {
	if self == 0 {
		return nil, fail(CodeAuthKeyUnregistered)
	}
	return &mtproto.Vector_StarsGiveawayOption{Datas: []*mtproto.StarsGiveawayOption{}}, nil
}

func (b *Book) GiveawayInfo(self int64, peer *mtproto.InputPeer, msgID int32) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return err
	}
	if msgID <= 0 {
		return fail(CodeMsgInvalid)
	}
	return fail(CodeGiveawayMissing)
}

func (b *Book) LaunchPrepaidGiveaway(self int64, peer *mtproto.InputPeer, giveawayID int64, purpose *mtproto.InputStorePaymentPurpose) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if _, _, err := parsePeer(self, peer); err != nil {
		return err
	}
	if giveawayID == 0 {
		return fail(CodeGiveawayIDInvalid)
	}
	if purpose == nil || purpose.GetPredicateName() == "" {
		return fail(CodePurposeInvalid)
	}
	return fail(CodePrepaidMissing)
}

func (b *Book) CheckGiftCode(self int64, slug string) error {
	if self == 0 {
		return fail(CodeAuthKeyUnregistered)
	}
	if err := giftSlug(slug); err != nil {
		return err
	}
	return fail(CodeGiftCodeMissing)
}

func (b *Book) ApplyGiftCode(self int64, slug string) error {
	return b.CheckGiftCode(self, slug)
}

func giftSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" || len(slug) > maxIDLen {
		return fail(CodeSlugInvalid)
	}
	for _, r := range slug {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return fail(CodeSlugInvalid)
	}
	return nil
}
