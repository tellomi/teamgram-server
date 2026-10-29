package rpc

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogfilter"
)

// Service stores chat folders for the signed-in user.
type Service struct {
	mtproto.UnimplementedRPCFoldersServer
	mtproto.UnimplementedRPCFolderTagsServer
}

func New() *Service { return &Service{} }

func userID(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.GetUserId()
}

func (Service) MessagesGetDialogFiltersEFD48C89(ctx context.Context, _ *mtproto.TLMessagesGetDialogFiltersEFD48C89) (*mtproto.Messages_DialogFilters, error) {
	folders, tags, err := load(ctx)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesDialogFilters(&mtproto.Messages_DialogFilters{
		TagsEnabled: tags,
		Filters:     folders,
	}).To_Messages_DialogFilters(), nil
}

func (Service) MessagesGetDialogFiltersF19ED96D(ctx context.Context, _ *mtproto.TLMessagesGetDialogFiltersF19ED96D) (*mtproto.Vector_DialogFilter, error) {
	folders, _, err := load(ctx)
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_DialogFilter{Datas: folders}, nil
}

func load(ctx context.Context) ([]*mtproto.DialogFilter, bool, error) {
	uid := userID(ctx)
	if uid <= 0 {
		return []*mtproto.DialogFilter{}, false, nil
	}
	rows, tags, err := dialogfilter.Current().List(ctx, uid)
	if err != nil {
		return nil, false, err
	}
	out := make([]*mtproto.DialogFilter, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Filter)
	}
	return out, tags, nil
}

func (Service) MessagesUpdateDialogFilter(ctx context.Context, in *mtproto.TLMessagesUpdateDialogFilter) (*mtproto.Bool, error) {
	uid := userID(ctx)
	if uid <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetId() < 0 {
		return nil, mtproto.ErrFilterIdInvalid
	}
	st := dialogfilter.Current()
	if in.GetFilter() == nil {
		if err := st.Delete(ctx, uid, in.GetId()); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	f := in.GetFilter()
	pred := f.GetPredicateName()
	if (pred == mtproto.Predicate_dialogFilter || pred == mtproto.Predicate_dialogFilterChatlist) && f.GetId() != in.GetId() {
		return nil, mtproto.ErrFilterIdInvalid
	}
	if err := st.Put(ctx, uid, in.GetId(), f); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (Service) MessagesUpdateDialogFiltersOrder(ctx context.Context, in *mtproto.TLMessagesUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	uid := userID(ctx)
	if uid <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	for _, id := range in.GetOrder() {
		if id < 0 {
			return nil, mtproto.ErrFilterIdInvalid
		}
	}
	if err := dialogfilter.Current().Order(ctx, uid, in.GetOrder()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (Service) MessagesToggleDialogFilterTags(ctx context.Context, in *mtproto.TLMessagesToggleDialogFilterTags) (*mtproto.Bool, error) {
	uid := userID(ctx)
	if uid <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if err := dialogfilter.Current().SetTags(ctx, uid, mtproto.FromBool(in.GetEnabled())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
