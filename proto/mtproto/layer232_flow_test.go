package mtproto

import (
	"encoding/binary"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func ctorOf(b []byte) uint32 {
	return binary.LittleEndian.Uint32(b[:4])
}

func encodeAt(t *testing.T, obj TLObject, layer int32) []byte {
	t.Helper()
	buf := NewEncodeBuf(512)
	if err := obj.Encode(buf, layer); err != nil {
		t.Fatalf("encode layer %d: %v", layer, err)
	}
	if len(buf.GetBuf()) < 4 {
		t.Fatalf("short encode (%d bytes)", len(buf.GetBuf()))
	}
	return buf.GetBuf()
}

func TestLayer232EveryPredicateResolves(t *testing.T) {
	for name := range clazzNameRegisters2 {
		if GetClazzID(name, Layer) == 0 {
			t.Errorf("GetClazzID(%q, %d) = 0", name, Layer)
		}
	}
}

func TestLayer232Constructors(t *testing.T) {
	// Official ids from telegram_api_layer232.tl. Unchanged types keep the
	// layer 231 id; the types below changed CRC between 231 and 232.
	want := map[string]uint32{
		Predicate_invokeWithLayer:           0xda9b0d0d,
		Predicate_auth_sendCode:             0xa677244f,
		Predicate_auth_signIn:               0x8d52a951,
		Predicate_auth_signUp:               0xaac7b717,
		Predicate_auth_authorization:        0x2ea2c0d4,
		Predicate_auth_sentCode:             0x5e002502,
		Predicate_user:                      0xb1b8cc83,
		Predicate_userFull:                  0xb90c2afd,
		Predicate_message:                   0x7600b9d3,
		Predicate_messageMediaPhoto:         0xe216eb63,
		Predicate_photo:                     0xfb197a65,
		Predicate_inputReplyToMessage:       0x75c1ad70,
		Predicate_messageReplyHeader:        0xddb5ab75,
		Predicate_channel:                   0x7ad15240,
		Predicate_channelFull:               0xc4c4dd40,
		Predicate_contacts_importedContacts: 0x77d01c3b,
		Predicate_contacts_importContacts:   0x2c800be5,
		Predicate_messages_sendMessage:      0xfef48f62,
		Predicate_messages_sendMedia:        0x0330e77f,
		Predicate_upload_saveFilePart:       0xb304a621,
	}
	for name, id := range want {
		got := uint32(GetClazzID(name, Layer))
		if got != id {
			t.Errorf("GetClazzID(%s, 232) = 0x%x, want 0x%x", name, got, id)
		}
	}

	// Older clients keep the layer 231 constructor where it differs.
	if uint32(GetClazzID(Predicate_userFull, 231)) != 0x6cbe645 {
		t.Fatalf("layer 231 userFull changed")
	}
	if uint32(GetClazzID(Predicate_channel, 231)) != 0xd49f34c6 {
		t.Fatalf("layer 231 channel changed")
	}
	if uint32(GetClazzID(Predicate_inputReplyToMessage, 231)) != 0x3bd4b7c2 {
		t.Fatalf("layer 231 inputReplyToMessage changed")
	}
	if uint32(GetClazzID(Predicate_messageReplyHeader, 231)) != 0x1b97dd66 {
		t.Fatalf("layer 231 messageReplyHeader changed")
	}
}

func sampleUser() *User {
	u := MakeTLUser(nil)
	u.SetSelf(true)
	u.SetId(1001)
	u.SetFirstName(&wrapperspb.StringValue{Value: "Ada"})
	u.SetPhone(&wrapperspb.StringValue{Value: "15551212"})
	return u.To_User()
}

func TestLayer232RegisterLoginContactsTextAndPhoto(t *testing.T) {
	user := sampleUser()

	// Login result: auth.authorization wrapping the user.
	authz := MakeTLAuthAuthorization(nil)
	authz.SetUser(user)
	raw := encodeAt(t, authz.To_Auth_Authorization(), Layer)
	if ctorOf(raw) != 0x2ea2c0d4 {
		t.Fatalf("auth.authorization ctor 0x%x", ctorOf(raw))
	}
	gotAuth := &Auth_Authorization{}
	if err := gotAuth.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if gotAuth.GetUser().GetId() != 1001 || gotAuth.GetUser().GetFirstName().GetValue() != "Ada" {
		t.Fatalf("user roundtrip: %+v", gotAuth.GetUser())
	}
	if uint32(gotAuth.GetUser().GetConstructor()) != 0xb1b8cc83 {
		t.Fatalf("user ctor 0x%x", uint32(gotAuth.GetUser().GetConstructor()))
	}

	// users.getFullUser result. Settings and notify settings are required.
	full := MakeTLUserFull(nil)
	full.SetId(1001)
	full.SetSettings(MakeTLPeerSettings(nil).To_PeerSettings())
	full.SetNotifySettings(MakeTLPeerNotifySettings(nil).To_PeerNotifySettings())
	full.SetAbout(&wrapperspb.StringValue{Value: "phase1"})
	raw = encodeAt(t, full.To_UserFull(), Layer)
	if ctorOf(raw) != 0xb90c2afd {
		t.Fatalf("userFull ctor 0x%x", ctorOf(raw))
	}
	// Same body as layer 231 except the constructor.
	raw231 := encodeAt(t, full.To_UserFull(), 231)
	if ctorOf(raw231) != 0x6cbe645 {
		t.Fatalf("userFull 231 ctor 0x%x", ctorOf(raw231))
	}
	if string(raw[4:]) != string(raw231[4:]) {
		t.Fatal("userFull layer 232 body diverged from 231 while new flags are unset")
	}
	gotFull := &UserFull{}
	if err := gotFull.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if gotFull.GetId() != 1001 || gotFull.GetAbout().GetValue() != "phase1" {
		t.Fatalf("userFull roundtrip id=%d about=%v", gotFull.GetId(), gotFull.GetAbout())
	}

	// contacts.importContacts result.
	imported := MakeTLContactsImportedContacts(nil)
	imported.SetUsers([]*User{user})
	raw = encodeAt(t, imported.To_Contacts_ImportedContacts(), Layer)
	if ctorOf(raw) != 0x77d01c3b {
		t.Fatalf("importedContacts ctor 0x%x", ctorOf(raw))
	}
	gotImp := &Contacts_ImportedContacts{}
	if err := gotImp.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if len(gotImp.GetUsers()) != 1 || gotImp.GetUsers()[0].GetId() != 1001 {
		t.Fatalf("imported users: %+v", gotImp.GetUsers())
	}

	peer := MakeTLPeerUser(nil)
	peer.SetUserId(2002)

	// Private text message.
	text := MakeTLMessage(nil)
	text.SetOut(true)
	text.SetId(7)
	text.SetPeerId(peer.To_Peer())
	text.SetDate(1_700_000_000)
	text.SetMessage("hello layer 232")
	raw = encodeAt(t, text.To_Message(), Layer)
	if ctorOf(raw) != 0x7600b9d3 {
		t.Fatalf("message ctor 0x%x", ctorOf(raw))
	}
	gotMsg := &Message{}
	if err := gotMsg.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if gotMsg.GetMessage() != "hello layer 232" || gotMsg.GetId() != 7 || !gotMsg.GetOut() {
		t.Fatalf("text roundtrip: id=%d out=%v msg=%q", gotMsg.GetId(), gotMsg.GetOut(), gotMsg.GetMessage())
	}

	// Private photo message.
	photo := MakeTLPhoto(nil)
	photo.SetId(42)
	photo.SetAccessHash(99)
	photo.SetFileReference([]byte{1, 2, 3, 4})
	photo.SetDate(1_700_000_000)
	photo.SetDcId(2)
	media := MakeTLMessageMediaPhoto(nil)
	media.SetPhoto_FLAGPHOTO(photo.To_Photo())
	pic := MakeTLMessage(nil)
	pic.SetOut(true)
	pic.SetId(8)
	pic.SetPeerId(peer.To_Peer())
	pic.SetDate(1_700_000_001)
	pic.SetMessage("")
	pic.SetMedia(media.To_MessageMedia())
	raw = encodeAt(t, pic.To_Message(), Layer)
	gotPic := &Message{}
	if err := gotPic.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if gotPic.GetMedia().GetPhoto_FLAGPHOTO().GetId() != 42 {
		t.Fatalf("photo id %d", gotPic.GetMedia().GetPhoto_FLAGPHOTO().GetId())
	}
	if uint32(gotPic.GetMedia().GetConstructor()) != 0xe216eb63 {
		t.Fatalf("media ctor 0x%x", uint32(gotPic.GetMedia().GetConstructor()))
	}

	// Reply header the client sends on messages.sendMessage.
	reply := MakeTLInputReplyToMessage(nil)
	reply.SetReplyToMsgId(7)
	raw = encodeAt(t, reply.To_InputReplyTo(), Layer)
	if ctorOf(raw) != 0x75c1ad70 {
		t.Fatalf("inputReplyToMessage ctor 0x%x", ctorOf(raw))
	}
	gotReply := &InputReplyTo{}
	if err := gotReply.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if gotReply.GetReplyToMsgId() != 7 {
		t.Fatalf("reply id %d", gotReply.GetReplyToMsgId())
	}

	hdr := MakeTLMessageReplyHeader(nil)
	hdr.SetReplyToMsgId_FLAGINT32(&wrapperspb.Int32Value{Value: 7})
	raw = encodeAt(t, hdr.To_MessageReplyHeader(), Layer)
	if ctorOf(raw) != 0xddb5ab75 {
		t.Fatalf("messageReplyHeader ctor 0x%x", ctorOf(raw))
	}
	gotHdr := &MessageReplyHeader{}
	if err := gotHdr.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if gotHdr.GetReplyToMsgId_FLAGINT32().GetValue() != 7 {
		t.Fatalf("header reply %v", gotHdr.GetReplyToMsgId_FLAGINT32())
	}

	// channel constructor changed even though phase 1 is private chat.
	ch := MakeTLChannel(nil)
	ch.SetId(50)
	ch.SetTitle("news")
	ch.SetDate(10)
	ch.SetPhoto(MakeTLChatPhotoEmpty(nil).To_ChatPhoto())
	raw = encodeAt(t, ch.To_Chat(), Layer)
	if ctorOf(raw) != 0x7ad15240 {
		t.Fatalf("channel ctor 0x%x", ctorOf(raw))
	}
	gotCh := &Chat{}
	if err := gotCh.Decode(NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if gotCh.GetId() != 50 || gotCh.GetTitle() != "news" {
		t.Fatalf("channel roundtrip id=%d title=%q", gotCh.GetId(), gotCh.GetTitle())
	}
}

func TestLayer232QuoteRichAndPinnedMsgAreConsumed(t *testing.T) {
	// inputReplyToMessage#75c1ad70 with quote_rich:flags.8?Vector<RichText> empty.
	b := NewEncodeBuf(64)
	b.UInt(0x75c1ad70)
	b.UInt(1 << 8)
	b.Int(9)
	b.Int(int32(CRC32_vector))
	b.Int(0)
	got := &InputReplyTo{}
	if err := got.Decode(NewDecodeBuf(b.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if got.GetReplyToMsgId() != 9 {
		t.Fatalf("reply %d", got.GetReplyToMsgId())
	}

	// channel#7ad15240 with pinned_msg_id:flags2.22?int, other flags clear.
	// flags, flags2, id, access_hash absent, title, photoEmpty, date, pinned.
	b = NewEncodeBuf(64)
	b.UInt(0x7ad15240)
	b.UInt(0)        // flags
	b.UInt(1 << 22)  // flags2 pinned_msg_id
	b.Long(50)       // id
	b.String("news") // title
	photo := encodeAt(t, MakeTLChatPhotoEmpty(nil).To_ChatPhoto(), Layer)
	b.buf = append(b.buf, photo...)
	b.Int(10) // date
	b.Int(77) // pinned_msg_id
	gotCh := &Chat{}
	if err := gotCh.Decode(NewDecodeBuf(b.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if gotCh.GetId() != 50 || gotCh.GetTitle() != "news" {
		t.Fatalf("channel id=%d title=%q", gotCh.GetId(), gotCh.GetTitle())
	}
}

func TestLayer232StarGiftWithdrawalUrl(t *testing.T) {
	gift := MakeTLInputSavedStarGiftUser(nil)
	gift.SetMsgId(3)
	req := &TLPaymentsGetStarGiftWithdrawalUrl{
		Constructor: TLConstructor(int32(GetClazzID(Predicate_payments_getStarGiftWithdrawalUrl, Layer))),
		Stargift:    gift.To_InputSavedStarGift(),
		TmpPassword: []byte("secret"),
	}
	if _, err := proto.Marshal(req); err != nil {
		t.Fatal(err)
	}
	raw := encodeAt(t, req, Layer)
	if ctorOf(raw) != 0xc8ca817b {
		t.Fatalf("ctor 0x%x", ctorOf(raw))
	}
	decoded := NewTLObjectByClassID(int32(ctorOf(raw)))
	if decoded == nil {
		t.Fatal("withdrawal url constructor not registered")
	}
	if err := decoded.Decode(NewDecodeBuf(raw[4:])); err != nil {
		t.Fatal(err)
	}
	got := decoded.(*TLPaymentsGetStarGiftWithdrawalUrl)
	if string(got.GetTmpPassword()) != "secret" || got.GetStargift().GetMsgId() != 3 {
		t.Fatalf("roundtrip %+v", got)
	}
}
