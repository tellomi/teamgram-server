package accesshash

import (
	"reflect"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Collect lists the input constructors on req that carry an access_hash or
// a from-message proof. inputUserSelf, inputPeerSelf, inputPeerChat, empty
// constructors, and username peers are omitted.
func Collect(req proto.Message) []Subject {
	if req == nil {
		return nil
	}
	var out []Subject
	seen := map[uintptr]struct{}{}
	walk(req, seen, 0, &out)
	return out
}

func walk(m proto.Message, seen map[uintptr]struct{}, depth int, out *[]Subject) {
	if m == nil || depth > 24 {
		return
	}
	rv := reflect.ValueOf(m)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	ptr := rv.Pointer()
	if _, ok := seen[ptr]; ok {
		return
	}
	seen[ptr] = struct{}{}

	switch v := m.(type) {
	case *mtproto.InputUser:
		if s, ok := userSubject(v); ok {
			*out = append(*out, s)
		}
	case *mtproto.InputPeer:
		if s, ok := peerSubject(v); ok {
			*out = append(*out, s)
		}
	case *mtproto.InputChannel:
		if s, ok := channelSubject(v); ok {
			*out = append(*out, s)
		}
	}

	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Kind() != protoreflect.MessageKind || fd.IsMap() {
			return true
		}
		if fd.IsList() {
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				if msg := list.Get(i).Message(); msg.IsValid() {
					walk(msg.Interface(), seen, depth+1, out)
				}
			}
			return true
		}
		if msg := v.Message(); msg.IsValid() {
			walk(msg.Interface(), seen, depth+1, out)
		}
		return true
	})
}

func userSubject(in *mtproto.InputUser) (Subject, bool) {
	if in == nil {
		return Subject{}, false
	}
	name, ctor := in.GetPredicateName(), in.GetConstructor()
	switch {
	case named(name, ctor, mtproto.Predicate_inputUserSelf, mtproto.CRC32_inputUserSelf):
		return Subject{}, false
	case named(name, ctor, mtproto.Predicate_inputUserEmpty, mtproto.CRC32_inputUserEmpty):
		return Subject{}, false
	case named(name, ctor, mtproto.Predicate_inputUserFromMessage, mtproto.CRC32_inputUserFromMessage):
		return Subject{Kind: KindUserFromMessage, ID: in.GetUserId(), MsgID: in.GetMsgId(), Peer: in.GetPeer()}, true
	case named(name, ctor, mtproto.Predicate_inputUser, mtproto.CRC32_inputUser):
		return Subject{Kind: KindUser, ID: in.GetUserId(), Provided: in.GetAccessHash()}, true
	case name == "" && ctor == 0 && in.GetUserId() > 0:
		// Handlers treat a bare user_id as inputUser. Leaving it unchecked
		// would keep the enumeration path open.
		return Subject{Kind: KindUser, ID: in.GetUserId(), Provided: in.GetAccessHash()}, true
	default:
		return Subject{}, false
	}
}

func peerSubject(in *mtproto.InputPeer) (Subject, bool) {
	if in == nil {
		return Subject{}, false
	}
	name, ctor := in.GetPredicateName(), in.GetConstructor()
	switch {
	case named(name, ctor, mtproto.Predicate_inputPeerSelf, mtproto.CRC32_inputPeerSelf),
		named(name, ctor, mtproto.Predicate_inputPeerEmpty, mtproto.CRC32_inputPeerEmpty),
		named(name, ctor, mtproto.Predicate_inputPeerChat, mtproto.CRC32_inputPeerChat),
		named(name, ctor, mtproto.Predicate_inputPeerUsername, mtproto.CRC32_inputPeerUsername):
		return Subject{}, false
	case named(name, ctor, mtproto.Predicate_inputPeerUserFromMessage, mtproto.CRC32_inputPeerUserFromMessage):
		return Subject{Kind: KindPeerUserFromMessage, ID: in.GetUserId(), MsgID: in.GetMsgId(), Peer: in.GetPeer()}, true
	case named(name, ctor, mtproto.Predicate_inputPeerChannelFromMessage, mtproto.CRC32_inputPeerChannelFromMessage):
		return Subject{Kind: KindPeerChannelFromMessage, ID: in.GetChannelId(), MsgID: in.GetMsgId(), Peer: in.GetPeer()}, true
	case named(name, ctor, mtproto.Predicate_inputPeerUser, mtproto.CRC32_inputPeerUser):
		return Subject{Kind: KindPeerUser, ID: in.GetUserId(), Provided: in.GetAccessHash()}, true
	case named(name, ctor, mtproto.Predicate_inputPeerChannel, mtproto.CRC32_inputPeerChannel):
		return Subject{Kind: KindPeerChannel, ID: in.GetChannelId(), Provided: in.GetAccessHash()}, true
	case name == "" && ctor == 0 && in.GetUserId() > 0 && in.GetChannelId() == 0:
		return Subject{Kind: KindPeerUser, ID: in.GetUserId(), Provided: in.GetAccessHash()}, true
	case name == "" && ctor == 0 && in.GetChannelId() > 0 && in.GetUserId() == 0:
		return Subject{Kind: KindPeerChannel, ID: in.GetChannelId(), Provided: in.GetAccessHash()}, true
	default:
		return Subject{}, false
	}
}

func channelSubject(in *mtproto.InputChannel) (Subject, bool) {
	if in == nil {
		return Subject{}, false
	}
	name, ctor := in.GetPredicateName(), in.GetConstructor()
	switch {
	case named(name, ctor, mtproto.Predicate_inputChannelEmpty, mtproto.CRC32_inputChannelEmpty):
		return Subject{}, false
	case named(name, ctor, mtproto.Predicate_inputChannelFromMessage, mtproto.CRC32_inputChannelFromMessage):
		return Subject{Kind: KindChannelFromMessage, ID: in.GetChannelId(), MsgID: in.GetMsgId(), Peer: in.GetPeer()}, true
	case named(name, ctor, mtproto.Predicate_inputChannel, mtproto.CRC32_inputChannel):
		return Subject{Kind: KindChannel, ID: in.GetChannelId(), Provided: in.GetAccessHash()}, true
	case name == "" && ctor == 0 && in.GetChannelId() > 0:
		return Subject{Kind: KindChannel, ID: in.GetChannelId(), Provided: in.GetAccessHash()}, true
	default:
		return Subject{}, false
	}
}

// InputPeerClass classifies the container peer of a from-message constructor.
func InputPeerClass(in *mtproto.InputPeer) Kind {
	if in == nil {
		return KindNone
	}
	name, ctor := in.GetPredicateName(), in.GetConstructor()
	switch {
	case named(name, ctor, mtproto.Predicate_inputPeerSelf, mtproto.CRC32_inputPeerSelf):
		return KindSelf
	case named(name, ctor, mtproto.Predicate_inputPeerChat, mtproto.CRC32_inputPeerChat):
		return KindChat
	case named(name, ctor, mtproto.Predicate_inputPeerUser, mtproto.CRC32_inputPeerUser):
		return KindPeerUser
	case named(name, ctor, mtproto.Predicate_inputPeerChannel, mtproto.CRC32_inputPeerChannel):
		return KindPeerChannel
	case name == "" && ctor == 0 && in.GetUserId() > 0 && in.GetChannelId() == 0:
		return KindPeerUser
	case name == "" && ctor == 0 && in.GetChannelId() > 0:
		return KindPeerChannel
	default:
		return KindNone
	}
}

func named(name string, ctor mtproto.TLConstructor, pred string, id mtproto.TLConstructor) bool {
	return name == pred || ctor == id
}
