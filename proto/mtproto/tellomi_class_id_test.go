package mtproto

import "testing"

// Tellomi constructors are not in the generated scheme. Each one has to be
// in clazzIdRegisters2 under the int32 form of its wire id, which is what
// DecodeBuf.Object looks up.
func TestTellomiConstructorsResolve(t *testing.T) {
	ids := []uint32{
		uint32(DraftTranscribeConstructor),
		crcDiscoverEngineCredential,
		crcDiscoverGetEngineCredential,
		crcDiscoverAudienceToken,
		crcDiscoverGetAudienceToken,
		crcFeedPosition,
		crcFeedItemMessage,
		crcFeedItemMessageGroup,
		crcFeedItemInterestCategories,
		crcFeedMessagesNotModified,
		crcFeedMessages,
		crcFeedForYou,
		crcFeedInterestCategory,
		crcFeedInterestCategoriesNotModified,
		crcFeedInterestCategories,
		crcFeedGetFeed,
		crcFeedGetForYou,
		crcFeedGetInterestCategories,
		crcFeedReadFeed,
		crcFeedSetInterestCategories,
		uint32(crcExportedNoPaidMessagesToken),
		uint32(crcContactsImportNoPaidMessagesToken),
		crcStarGiftDepositPayload,
		crcPaymentsDeleteStarGift,
		crcPaymentsWithdrawStarGift,
		crcPaymentsDepositStarGift,
		crcPaymentsDisplayStarGift,
		crcInputWalletCardSticker,
		crcWalletCardSticker,
		crcWalletCard,
		crcWalletSetCardStickers,
		crcShareThemeDefault,
		crcShareThemeEmojiBackdrop,
		crcShareThemeGift,
		crcInputShareThemeGift,
		crcShareThemeBackdropOption,
		crcShareThemeBackdropsNotModified,
		crcShareThemeBackdrops,
		crcMessagesSetShareTheme,
		crcMessagesGetShareThemeBackdrops,
		crcMessagesGetStatsURL,
	}
	// Negative int32 constants are not representable as uint32 constants.
	// The lookup still uses the int32 bit pattern.
	signed := []int32{crcContactsExportNoPaidMessagesToken}
	for _, id := range signed {
		if NewTLObjectByClassID(id) == nil {
			t.Errorf("constructor 0x%08x is not registered", uint32(id))
		}
	}
	for _, id := range ids {
		// id is a variable, so this is the same bit cast Object() uses.
		if NewTLObjectByClassID(int32(id)) == nil {
			t.Errorf("constructor 0x%08x is not registered", id)
		}
	}
}
