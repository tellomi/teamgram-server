// Copyright 2026 Tellomi
//
// Layer 232 extensions on top of the teamgram/proto mtproto codec.
// The schema is proto/schema/telegram_api_layer232.tl (Telegram Desktop
// api.tl, LAYER 232).

package mtproto

import "fmt"

// Layer is the Telegram API layer this fork encodes and decodes.
// Stock Telegram iOS (TelegramMessenger/Telegram-iOS fb0e62ba) and Android
// (DrKLO/Telegram 3de37c8) both speak this layer.
const Layer = 232

// consumeRichTextVector reads Vector<RichText> so a layer-232 quote_rich
// field does not desynchronize the rest of the object. The vector is not
// stored; simple private messages leave the flag unset.
func consumeRichTextVector(dBuf *DecodeBuf) error {
	c := dBuf.Int()
	if dBuf.GetError() != nil {
		return dBuf.GetError()
	}
	if c != int32(CRC32_vector) {
		return fmt.Errorf("quote_rich: expected vector, got 0x%x", uint32(c))
	}
	n := dBuf.Int()
	if n < 0 || n > maxVectorLen {
		return fmt.Errorf("quote_rich: invalid vector length %d", n)
	}
	for i := int32(0); i < n; i++ {
		rt := &RichText{}
		if err := rt.Decode(dBuf); err != nil {
			return err
		}
	}
	return dBuf.GetError()
}
