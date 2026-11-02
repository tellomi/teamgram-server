package e2e

import (
	"encoding/binary"
	"fmt"

	"github.com/gotd/td/bin"
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// crcVector is vector#1cb5c415. Several startup RPCs return a bare vector
// (langpack.getLanguages, contacts.getStatuses, messages.getCustomEmojiDocuments,
// stories.getPeerMaxIDs). It is not a registered TL object.
const crcVector = 0x1cb5c415

// tlVector is a decoded bare vector. Startup checks only need it to decode.
type tlVector struct {
	Count int32
}

func (v *tlVector) Encode(*mtproto.EncodeBuf, int32) error { return nil }
func (v *tlVector) Decode(*mtproto.DecodeBuf) error        { return nil }
func (v *tlVector) String() string                         { return fmt.Sprintf("vector<%d>", v.Count) }

const layer = 232

func encodeTL(obj mtproto.TLObject) ([]byte, error) {
	buf := mtproto.NewEncodeBuf(512)
	if err := obj.Encode(buf, layer); err != nil {
		return nil, err
	}
	raw := append([]byte(nil), buf.GetBuf()...)
	if len(raw) < 4 {
		return nil, fmt.Errorf("encoded %T is empty", obj)
	}
	return raw, nil
}

// wrapInit encodes invokeWithLayer(initConnection(query)) the way official
// clients wrap every RPC. The inner query is already encoded.
func wrapInit(cfg Config, profile Profile, query []byte) ([]byte, error) {
	init := &mtproto.TLInitConnection{
		Constructor:    mtproto.CRC32_initConnection_c1cd5ea9,
		ApiId:          cfg.APIID,
		DeviceModel:    profile.DeviceModel,
		SystemVersion:  profile.SystemVersion,
		AppVersion:     profile.AppVersion,
		SystemLangCode: profile.SystemLangCode,
		LangPack:       profile.LangPack,
		LangCode:       profile.LangCode,
		Query:          query,
	}
	initRaw, err := encodeTL(init)
	if err != nil {
		return nil, err
	}
	wrapped := &mtproto.TLInvokeWithLayer{
		Constructor: mtproto.CRC32_invokeWithLayer,
		Layer:       layer,
		Query:       initRaw,
	}
	return encodeTL(wrapped)
}

type rawTL []byte

func (r rawTL) Encode(b *bin.Buffer) error {
	b.Put(r)
	return nil
}

type captured struct {
	obj mtproto.TLObject
}

func (c *captured) Decode(b *bin.Buffer) error {
	obj, err := decodeTL(append([]byte(nil), b.Buf...))
	if err != nil {
		return err
	}
	c.obj = obj
	return nil
}

func decodeTL(raw []byte) (mtproto.TLObject, error) {
	if len(raw) < 4 {
		return nil, fmt.Errorf("short tl payload (%d bytes)", len(raw))
	}
	if binary.LittleEndian.Uint32(raw[:4]) == crcVector {
		if len(raw) < 8 {
			return nil, fmt.Errorf("short vector")
		}
		n := int32(binary.LittleEndian.Uint32(raw[4:8]))
		if n < 0 {
			return nil, fmt.Errorf("vector size %d", n)
		}
		return &tlVector{Count: n}, nil
	}
	d := mtproto.NewDecodeBuf(raw)
	obj := d.Object()
	if err := d.GetError(); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("empty tl object")
	}
	if gz, ok := obj.(*mtproto.TLGzipPacked); ok {
		if gz.Obj == nil {
			return nil, fmt.Errorf("empty gzip payload")
		}
		return gz.Obj, nil
	}
	return obj, nil
}

func inputPeerEmpty() *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer()
}

func inputPeerSelf() *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
}

func inputPeerUser(id, accessHash int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId:     id,
		AccessHash: accessHash,
	}).To_InputPeer()
}

func inputPeerChat(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: id}).To_InputPeer()
}

func inputPeerChannel(id, accessHash int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  id,
		AccessHash: accessHash,
	}).To_InputPeer()
}

func inputUser(id, accessHash int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{
		UserId:     id,
		AccessHash: accessHash,
	}).To_InputUser()
}

func inputUserSelf() *mtproto.InputUser {
	return mtproto.MakeTLInputUserSelf(nil).To_InputUser()
}

func inputChannel(id, accessHash int64) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  id,
		AccessHash: accessHash,
	}).To_InputChannel()
}

func strFlag(v string) *wrapperspb.StringValue {
	return &wrapperspb.StringValue{Value: v}
}

func entityBold(offset, length int32) *mtproto.MessageEntity {
	return mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{
		Offset: offset,
		Length: length,
	}).To_MessageEntity()
}

func reactionEmoji(emoticon string) *mtproto.Reaction {
	return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emoticon}).To_Reaction()
}
