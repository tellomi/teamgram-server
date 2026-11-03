package mtproto

import "fmt"

// Discover constructors are not in telegram_api_layer232.tl.
//
//	discover.engineCredential#0578ccaa token:string expires:int region:string = discover.EngineCredential;
//	discover.getEngineCredential#a80a7f76 = discover.EngineCredential;
const (
	Predicate_discover_engineCredential           = "discover_engineCredential"
	Predicate_discover_getEngineCredential        = "discover_getEngineCredential"
	crcDiscoverEngineCredential            uint32 = 0x0578ccaa
	crcDiscoverGetEngineCredential         uint32 = 0xa80a7f76
)

func init() {
	registerLayer232Extra(Predicate_discover_engineCredential, crcDiscoverEngineCredential, func(c int32) TLObject {
		return &Discover_EngineCredential{PredicateName: Predicate_discover_engineCredential, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_discover_getEngineCredential, crcDiscoverGetEngineCredential, func(c int32) TLObject {
		return &TLDiscoverGetEngineCredential{Constructor: TLConstructor(c)}
	})
}

// MakeDiscoverEngineCredential builds discover.engineCredential.
func MakeDiscoverEngineCredential(token, region string, expires int32) *Discover_EngineCredential {
	return &Discover_EngineCredential{
		PredicateName: Predicate_discover_engineCredential,
		Constructor:   ctorID(crcDiscoverEngineCredential),
		Token:         token,
		Expires:       expires,
		Region:        region,
	}
}

func (m *Discover_EngineCredential) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		return fmt.Errorf("discover.engineCredential: nil")
	}
	x.UInt(crcDiscoverEngineCredential)
	x.String(m.GetToken())
	x.Int(m.GetExpires())
	x.String(m.GetRegion())
	return nil
}

func (m *Discover_EngineCredential) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcDiscoverEngineCredential {
		return fmt.Errorf("discover.engineCredential: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_discover_engineCredential
	m.Token = dBuf.String()
	m.Expires = dBuf.Int()
	m.Region = dBuf.String()
	return dBuf.GetError()
}

func (m *TLDiscoverGetEngineCredential) Encode(x *EncodeBuf, layer int32) error {
	x.UInt(crcDiscoverGetEngineCredential)
	return nil
}

func (m *TLDiscoverGetEngineCredential) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcDiscoverGetEngineCredential {
		return fmt.Errorf("discover.getEngineCredential: constructor 0x%x", uint32(m.Constructor))
	}
	return dBuf.GetError()
}
