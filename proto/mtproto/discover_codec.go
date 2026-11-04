package mtproto

import "fmt"

// Discover constructors are not in telegram_api_layer232.tl.
//
//	discover.engineCredential#0578ccaa token:string expires:int region:string = discover.EngineCredential;
//	discover.getEngineCredential#a80a7f76 = discover.EngineCredential;
//	discover.audienceToken#da177181 token:string expires:int = discover.AudienceToken;
//	discover.getAudienceToken#dc5213c2 kind:string user_ids:Vector<long> = discover.AudienceToken;
const (
	Predicate_discover_engineCredential           = "discover_engineCredential"
	Predicate_discover_getEngineCredential        = "discover_getEngineCredential"
	Predicate_discover_audienceToken              = "discover_audienceToken"
	Predicate_discover_getAudienceToken           = "discover_getAudienceToken"
	crcDiscoverEngineCredential            uint32 = 0x0578ccaa
	crcDiscoverGetEngineCredential         uint32 = 0xa80a7f76
	crcDiscoverAudienceToken               uint32 = 0xda177181
	crcDiscoverGetAudienceToken            uint32 = 0xdc5213c2
)

func init() {
	registerLayer232Extra(Predicate_discover_engineCredential, crcDiscoverEngineCredential, func(c int32) TLObject {
		return &Discover_EngineCredential{PredicateName: Predicate_discover_engineCredential, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_discover_getEngineCredential, crcDiscoverGetEngineCredential, func(c int32) TLObject {
		return &TLDiscoverGetEngineCredential{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_discover_audienceToken, crcDiscoverAudienceToken, func(c int32) TLObject {
		return &Discover_AudienceToken{PredicateName: Predicate_discover_audienceToken, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_discover_getAudienceToken, crcDiscoverGetAudienceToken, func(c int32) TLObject {
		return &TLDiscoverGetAudienceToken{Constructor: TLConstructor(c)}
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

// MakeDiscoverAudienceToken builds discover.audienceToken.
func MakeDiscoverAudienceToken(token string, expires int32) *Discover_AudienceToken {
	return &Discover_AudienceToken{
		PredicateName: Predicate_discover_audienceToken,
		Constructor:   ctorID(crcDiscoverAudienceToken),
		Token:         token,
		Expires:       expires,
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

func (m *Discover_AudienceToken) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		return fmt.Errorf("discover.audienceToken: nil")
	}
	x.UInt(crcDiscoverAudienceToken)
	x.String(m.GetToken())
	x.Int(m.GetExpires())
	return nil
}

func (m *Discover_AudienceToken) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcDiscoverAudienceToken {
		return fmt.Errorf("discover.audienceToken: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_discover_audienceToken
	m.Token = dBuf.String()
	m.Expires = dBuf.Int()
	return dBuf.GetError()
}

func (m *TLDiscoverGetAudienceToken) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		return fmt.Errorf("discover.getAudienceToken: nil")
	}
	x.UInt(crcDiscoverGetAudienceToken)
	x.String(m.GetKind())
	x.VectorLong(m.GetUserIds())
	return nil
}

func (m *TLDiscoverGetAudienceToken) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcDiscoverGetAudienceToken {
		return fmt.Errorf("discover.getAudienceToken: constructor 0x%x", uint32(m.Constructor))
	}
	m.Kind = dBuf.String()
	m.UserIds = dBuf.VectorLong()
	return dBuf.GetError()
}
