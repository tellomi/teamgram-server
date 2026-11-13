package searchorder

import (
	"context"
	"strconv"

	"google.golang.org/grpc/metadata"
)

// The message service RPC has no cursor fields. The BFF sends the tuple on
// the context so a channel message id is not stuffed into user_message_box_id.
const (
	metaOn       = "x-search-cursor-on"
	metaDate     = "x-search-cursor-date"
	metaPeerType = "x-search-cursor-peer-type"
	metaPeerID   = "x-search-cursor-peer-id"
	metaMsgID    = "x-search-cursor-msg-id"

	metaScope         = "x-search-scope"
	metaScopeUsers    = "x-search-scope-users"
	metaScopeGroups   = "x-search-scope-groups"
	metaScopeChannels = "x-search-scope-channels"
)

type ctxKey struct{}

type stored struct {
	on  bool
	key Key
}

// WithOutgoing attaches an active cursor for an in-process call and for the
// outgoing gRPC metadata the message service reads.
func WithOutgoing(ctx context.Context, key Key) context.Context {
	ctx = context.WithValue(ctx, ctxKey{}, stored{on: true, key: key})
	return metadata.AppendToOutgoingContext(ctx,
		metaOn, "1",
		metaDate, strconv.FormatInt(int64(key.Date), 10),
		metaPeerType, strconv.FormatInt(int64(key.PeerType), 10),
		metaPeerID, strconv.FormatInt(key.PeerID, 10),
		metaMsgID, strconv.FormatInt(int64(key.MsgID), 10),
	)
}

// FromContext reads a cursor set by WithOutgoing or by incoming gRPC metadata.
func FromContext(ctx context.Context) (Key, bool) {
	if v, ok := ctx.Value(ctxKey{}).(stored); ok && v.on {
		return v.key, true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md.Get(metaOn)) == 0 || md.Get(metaOn)[0] != "1" {
		return Key{}, false
	}
	date, err1 := parseI32(md, metaDate)
	peerType, err2 := parseI32(md, metaPeerType)
	peerID, err3 := parseI64(md, metaPeerID)
	msgID, err4 := parseI32(md, metaMsgID)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return Key{}, false
	}
	return Key{Date: date, PeerType: peerType, PeerID: peerID, MsgID: msgID}, true
}

func parseI32(md metadata.MD, k string) (int32, error) {
	vs := md.Get(k)
	if len(vs) == 0 {
		return 0, strconv.ErrSyntax
	}
	n, err := strconv.ParseInt(vs[0], 10, 32)
	return int32(n), err
}

type scopeStored struct {
	on         bool
	users      bool
	groups     bool
	broadcasts bool
}

type scopeKey struct{}

// WithPeerScope tells the message service which peer kinds the count includes.
// broadcasts-only means the user-message table contributes nothing.
func WithPeerScope(ctx context.Context, users, groups, broadcasts bool) context.Context {
	ctx = context.WithValue(ctx, scopeKey{}, scopeStored{on: true, users: users, groups: groups, broadcasts: broadcasts})
	return metadata.AppendToOutgoingContext(ctx,
		metaScope, "1",
		metaScopeUsers, boolBit(users),
		metaScopeGroups, boolBit(groups),
		metaScopeChannels, boolBit(broadcasts),
	)
}

// PeerScope reports a kind restriction. ok is false when the caller did not set one.
func PeerScope(ctx context.Context) (users, groups, broadcasts bool, ok bool) {
	if v, hit := ctx.Value(scopeKey{}).(scopeStored); hit && v.on {
		return v.users, v.groups, v.broadcasts, true
	}
	md, hit := metadata.FromIncomingContext(ctx)
	if !hit || len(md.Get(metaScope)) == 0 || md.Get(metaScope)[0] != "1" {
		return false, false, false, false
	}
	return bit(md, metaScopeUsers), bit(md, metaScopeGroups), bit(md, metaScopeChannels), true
}

func boolBit(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func bit(md metadata.MD, k string) bool {
	vs := md.Get(k)
	return len(vs) > 0 && vs[0] == "1"
}

func parseI64(md metadata.MD, k string) (int64, error) {
	vs := md.Get(k)
	if len(vs) == 0 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(vs[0], 10, 64)
}
