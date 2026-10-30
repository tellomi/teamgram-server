package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/p1rest"
)

// ThemesService serves messages.setChatTheme and the account theme RPCs.
// https://core.telegram.org/api/themes
type ThemesService struct {
	mtproto.UnimplementedRPCThemesServer
}

func (ThemesService) MessagesSetChatTheme(ctx context.Context, in *mtproto.TLMessagesSetChatTheme) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return p1rest.SetChatTheme(self, in)
}

// WallpapersService serves messages.setChatWallPaper and the account wallpaper RPCs.
// https://core.telegram.org/api/wallpapers
type WallpapersService struct {
	mtproto.UnimplementedRPCWallpapersServer
}

func (WallpapersService) MessagesSetChatWallPaper(ctx context.Context, in *mtproto.TLMessagesSetChatWallPaper) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return p1rest.SetChatWallPaper(self, in)
}

// ShareThemesService serves messages.setShareTheme and messages.getShareThemeBackdrops.
type ShareThemesService struct {
	mtproto.UnimplementedRPCShareThemesServer
}

func (ShareThemesService) MessagesSetShareTheme(ctx context.Context, in *mtproto.TLMessagesSetShareTheme) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return p1rest.SetShareTheme(self, in.GetPeer(), in.GetTheme())
}

func (ShareThemesService) MessagesGetShareThemeBackdrops(_ context.Context, in *mtproto.TLMessagesGetShareThemeBackdrops) (*mtproto.ShareThemeBackdrops, error) {
	return p1rest.ShareThemeBackdrops(in.GetHash()), nil
}

// ImportedChatsService serves the history-import RPCs.
// https://core.telegram.org/api/import
type ImportedChatsService struct {
	mtproto.UnimplementedRPCImportedChatsServer
}

func (ImportedChatsService) MessagesCheckHistoryImport(_ context.Context, in *mtproto.TLMessagesCheckHistoryImport) (*mtproto.Messages_HistoryImportParsed, error) {
	return p1rest.CheckHistoryImport(in.GetImportHead())
}

func (ImportedChatsService) MessagesCheckHistoryImportPeer(ctx context.Context, in *mtproto.TLMessagesCheckHistoryImportPeer) (*mtproto.Messages_CheckedHistoryImportPeer, error) {
	self, _ := selfID(ctx)
	return p1rest.CheckHistoryImportPeer(self, in.GetPeer())
}

func (ImportedChatsService) MessagesInitHistoryImport(ctx context.Context, in *mtproto.TLMessagesInitHistoryImport) (*mtproto.Messages_HistoryImport, error) {
	self, _ := selfID(ctx)
	return p1rest.InitHistoryImport(self, in)
}

func (ImportedChatsService) MessagesUploadImportedMedia(ctx context.Context, in *mtproto.TLMessagesUploadImportedMedia) (*mtproto.MessageMedia, error) {
	self, _ := selfID(ctx)
	return p1rest.UploadImportedMedia(self, in)
}

func (ImportedChatsService) MessagesStartHistoryImport(ctx context.Context, in *mtproto.TLMessagesStartHistoryImport) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return p1rest.StartHistoryImport(self, in)
}

// TodoListsService serves checklist append and completion toggles.
// https://core.telegram.org/api/todo
type TodoListsService struct {
	mtproto.UnimplementedRPCTodoListsServer
}

func (TodoListsService) MessagesAppendTodoList(ctx context.Context, in *mtproto.TLMessagesAppendTodoList) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return p1rest.AppendTodoList(self, in)
}

func (TodoListsService) MessagesToggleTodoCompleted(ctx context.Context, in *mtproto.TLMessagesToggleTodoCompleted) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return p1rest.ToggleTodoCompleted(self, in)
}

// FactChecksService serves fact-check reads and the fact-checker edits.
// https://core.telegram.org/api/factcheck
type FactChecksService struct {
	mtproto.UnimplementedRPCFactChecksServer
}

func (FactChecksService) MessagesGetFactCheck(ctx context.Context, in *mtproto.TLMessagesGetFactCheck) (*mtproto.Vector_FactCheck, error) {
	self, _ := selfID(ctx)
	return p1rest.GetFactCheck(self, in.GetPeer(), in.GetMsgId())
}

func (FactChecksService) MessagesEditFactCheck(ctx context.Context, in *mtproto.TLMessagesEditFactCheck) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return p1rest.EditFactCheck(self, in.GetPeer(), in.GetMsgId(), in.GetText())
}

func (FactChecksService) MessagesDeleteFactCheck(ctx context.Context, in *mtproto.TLMessagesDeleteFactCheck) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return p1rest.DeleteFactCheck(self, in.GetPeer(), in.GetMsgId())
}

// AIComposeService serves messages.composeMessageWithAI.
// https://core.telegram.org/method/messages.composeMessageWithAI
type AIComposeService struct {
	mtproto.UnimplementedRPCAiComposeToneServer
}

func (AIComposeService) MessagesComposeMessageWithAI(_ context.Context, in *mtproto.TLMessagesComposeMessageWithAI) (*mtproto.Messages_ComposedMessageWithAI, error) {
	return p1rest.ComposeMessageWithAI(in)
}

// GamesService serves messages.getEmojiGameInfo.
// https://core.telegram.org/method/messages.getEmojiGameInfo
type GamesService struct {
	mtproto.UnimplementedRPCGamesServer
}

func (GamesService) MessagesGetEmojiGameInfo(context.Context, *mtproto.TLMessagesGetEmojiGameInfo) (*mtproto.Messages_EmojiGameInfo, error) {
	return p1rest.EmojiGameInfo(), nil
}

// StatsURLService serves the Android messages.getStatsURL constructor.
// The method is not in the layer-232 schema. Android still sends 0x812c2ae6.
type StatsURLService struct {
	mtproto.UnimplementedRPCStatsURLServer
}

func (StatsURLService) MessagesGetStatsURL(ctx context.Context, in *mtproto.TLMessagesGetStatsURL) (*mtproto.StatsURL, error) {
	self, _ := selfID(ctx)
	return p1rest.StatsURL(self, in)
}

// SuggestedPostsService serves messages.toggleSuggestedPostApproval.
// https://core.telegram.org/method/messages.toggleSuggestedPostApproval
type SuggestedPostsService struct {
	mtproto.UnimplementedRPCSuggestedPostsServer
}

func (SuggestedPostsService) MessagesToggleSuggestedPostApproval(ctx context.Context, in *mtproto.TLMessagesToggleSuggestedPostApproval) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return p1rest.ToggleSuggestedPostApproval(self, in)
}
