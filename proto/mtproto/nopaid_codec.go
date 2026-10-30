package mtproto

import "fmt"

// Layer 232 constructors from proto/schema/telegram_api_layer232.tl.
//
//	exportedNoPaidMessagesToken#695dd042 flags:# token:string url:string expires:flags.0?int = ExportedNoPaidMessagesToken;
//	contacts.exportNoPaidMessagesToken#83c586ae flags:# revoke:flags.0?true temporary:flags.1?true = ExportedNoPaidMessagesToken;
//	contacts.importNoPaidMessagesToken#35a7c6ca user_id:InputUser token:string = User;
const (
	Predicate_exportedNoPaidMessagesToken              = "exportedNoPaidMessagesToken"
	Predicate_contacts_exportNoPaidMessagesToken       = "contacts_exportNoPaidMessagesToken"
	Predicate_contacts_importNoPaidMessagesToken       = "contacts_importNoPaidMessagesToken"
	crcExportedNoPaidMessagesToken               int32 = 0x695dd042
	crcContactsExportNoPaidMessagesToken         int32 = -2084206930 // 0x83c586ae
	crcContactsImportNoPaidMessagesToken         int32 = 0x35a7c6ca
)

func ctorU32(v int32) uint32 { return uint32(v) }

func init() {
	clazzNameRegisters2[Predicate_exportedNoPaidMessagesToken] = map[int]int32{
		0:   crcExportedNoPaidMessagesToken,
		232: crcExportedNoPaidMessagesToken,
	}
	clazzNameRegisters2[Predicate_contacts_exportNoPaidMessagesToken] = map[int]int32{
		0:   crcContactsExportNoPaidMessagesToken,
		232: crcContactsExportNoPaidMessagesToken,
	}
	clazzNameRegisters2[Predicate_contacts_importNoPaidMessagesToken] = map[int]int32{
		0:   crcContactsImportNoPaidMessagesToken,
		232: crcContactsImportNoPaidMessagesToken,
	}
	clazzIdNameRegisters2[crcExportedNoPaidMessagesToken] = Predicate_exportedNoPaidMessagesToken
	clazzIdNameRegisters2[crcContactsExportNoPaidMessagesToken] = Predicate_contacts_exportNoPaidMessagesToken
	clazzIdNameRegisters2[crcContactsImportNoPaidMessagesToken] = Predicate_contacts_importNoPaidMessagesToken

	clazzIdRegisters2[crcExportedNoPaidMessagesToken] = func() TLObject {
		return &ExportedNoPaidMessagesToken{
			PredicateName: Predicate_exportedNoPaidMessagesToken,
			Constructor:   TLConstructor(crcExportedNoPaidMessagesToken),
		}
	}
	clazzIdRegisters2[crcContactsExportNoPaidMessagesToken] = func() TLObject {
		return &TLContactsExportNoPaidMessagesToken{
			Constructor: TLConstructor(crcContactsExportNoPaidMessagesToken),
		}
	}
	clazzIdRegisters2[crcContactsImportNoPaidMessagesToken] = func() TLObject {
		return &TLContactsImportNoPaidMessagesToken{
			Constructor: TLConstructor(crcContactsImportNoPaidMessagesToken),
		}
	}
}

// MakeExportedNoPaidMessagesToken builds the RPC result.
// expires == 0 leaves the expires flag unset (a permanent token).
func MakeExportedNoPaidMessagesToken(token, url string, expires int32) *ExportedNoPaidMessagesToken {
	return &ExportedNoPaidMessagesToken{
		PredicateName: Predicate_exportedNoPaidMessagesToken,
		Constructor:   TLConstructor(crcExportedNoPaidMessagesToken),
		Token:         token,
		Url:           url,
		Expires:       expires,
	}
}

// Encode writes exportedNoPaidMessagesToken#695dd042.
func (m *ExportedNoPaidMessagesToken) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = MakeExportedNoPaidMessagesToken("", "", 0)
	}
	_ = layer
	x.UInt(ctorU32(crcExportedNoPaidMessagesToken))
	var flags uint32
	if m.GetExpires() != 0 {
		flags |= 1 << 0
	}
	x.UInt(flags)
	x.String(m.GetToken())
	x.String(m.GetUrl())
	if flags&(1<<0) != 0 {
		x.Int(m.GetExpires())
	}
	return nil
}

// Decode reads the body. Object() has already consumed the constructor.
func (m *ExportedNoPaidMessagesToken) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(crcExportedNoPaidMessagesToken)
	}
	if ctorU32(int32(m.Constructor)) != ctorU32(crcExportedNoPaidMessagesToken) {
		return fmt.Errorf("exportedNoPaidMessagesToken: constructor 0x%x", ctorU32(int32(m.Constructor)))
	}
	flags := dBuf.UInt()
	m.Token = dBuf.String()
	m.Url = dBuf.String()
	if flags&(1<<0) != 0 {
		m.Expires = dBuf.Int()
	}
	m.PredicateName = Predicate_exportedNoPaidMessagesToken
	return dBuf.GetError()
}

// Encode writes contacts.exportNoPaidMessagesToken#83c586ae.
func (m *TLContactsExportNoPaidMessagesToken) Encode(x *EncodeBuf, layer int32) error {
	_ = layer
	x.UInt(ctorU32(crcContactsExportNoPaidMessagesToken))
	var flags uint32
	if m.GetRevoke() {
		flags |= 1 << 0
	}
	if m.GetTemporary() {
		flags |= 1 << 1
	}
	x.UInt(flags)
	return nil
}

// Decode reads flags. Object() has already consumed the constructor.
func (m *TLContactsExportNoPaidMessagesToken) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	m.Revoke = flags&(1<<0) != 0
	m.Temporary = flags&(1<<1) != 0
	return dBuf.GetError()
}

// Encode writes contacts.importNoPaidMessagesToken#35a7c6ca.
func (m *TLContactsImportNoPaidMessagesToken) Encode(x *EncodeBuf, layer int32) error {
	x.UInt(ctorU32(crcContactsImportNoPaidMessagesToken))
	if m.GetUserId() == nil {
		empty := MakeTLInputUserEmpty(nil)
		if err := empty.Encode(x, layer); err != nil {
			return err
		}
	} else if err := m.GetUserId().Encode(x, layer); err != nil {
		return err
	}
	x.String(m.GetToken())
	return nil
}

// Decode reads user_id and token. Object() has already consumed the constructor.
func (m *TLContactsImportNoPaidMessagesToken) Decode(dBuf *DecodeBuf) error {
	user := &InputUser{}
	if err := user.Decode(dBuf); err != nil {
		return err
	}
	m.UserId = user
	m.Token = dBuf.String()
	return dBuf.GetError()
}
