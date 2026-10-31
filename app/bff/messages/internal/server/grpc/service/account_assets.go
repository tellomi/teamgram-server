package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountassets"
)

func (ThemesService) AccountUploadTheme(ctx context.Context, in *mtproto.TLAccountUploadTheme) (*mtproto.Document, error) {
	self, _ := selfID(ctx)
	return accountassets.UploadTheme(self, in)
}

func (ThemesService) AccountCreateTheme(ctx context.Context, in *mtproto.TLAccountCreateTheme) (*mtproto.Theme, error) {
	self, _ := selfID(ctx)
	return accountassets.CreateTheme(self, in)
}

func (ThemesService) AccountUpdateTheme(ctx context.Context, in *mtproto.TLAccountUpdateTheme) (*mtproto.Theme, error) {
	self, _ := selfID(ctx)
	return accountassets.UpdateTheme(self, in)
}

func (ThemesService) AccountSaveTheme(ctx context.Context, in *mtproto.TLAccountSaveTheme) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return accountassets.SaveTheme(self, in.GetTheme(), mtproto.FromBool(in.GetUnsave()))
}

func (ThemesService) AccountInstallTheme(ctx context.Context, in *mtproto.TLAccountInstallTheme) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return accountassets.InstallTheme(self, in)
}

func (ThemesService) AccountGetTheme(ctx context.Context, in *mtproto.TLAccountGetTheme) (*mtproto.Theme, error) {
	self, _ := selfID(ctx)
	return accountassets.GetTheme(self, in.GetFormat(), in.GetTheme())
}

func (ThemesService) AccountGetThemes(ctx context.Context, in *mtproto.TLAccountGetThemes) (*mtproto.Account_Themes, error) {
	self, _ := selfID(ctx)
	return accountassets.GetThemes(self, in.GetFormat(), in.GetHash())
}

func (ThemesService) AccountGetChatThemes(_ context.Context, in *mtproto.TLAccountGetChatThemes) (*mtproto.Account_Themes, error) {
	return accountassets.GetChatThemes(in.GetHash()), nil
}

func (ThemesService) AccountGetUniqueGiftChatThemes(_ context.Context, in *mtproto.TLAccountGetUniqueGiftChatThemes) (*mtproto.Account_ChatThemes, error) {
	return accountassets.UniqueGiftChatThemes(in.GetOffset_STRING(), in.GetLimit(), in.GetHash()), nil
}

func (WallpapersService) AccountGetWallPapers(ctx context.Context, in *mtproto.TLAccountGetWallPapers) (*mtproto.Account_WallPapers, error) {
	self, _ := selfID(ctx)
	return accountassets.GetWallPapers(self, in.GetHash()), nil
}

func (WallpapersService) AccountGetWallPaper(ctx context.Context, in *mtproto.TLAccountGetWallPaper) (*mtproto.WallPaper, error) {
	self, _ := selfID(ctx)
	return accountassets.GetWallPaper(self, in.GetWallpaper())
}

func (WallpapersService) AccountGetMultiWallPapers(ctx context.Context, in *mtproto.TLAccountGetMultiWallPapers) (*mtproto.Vector_WallPaper, error) {
	self, _ := selfID(ctx)
	return accountassets.GetMultiWallPapers(self, in.GetWallpapers())
}

func (WallpapersService) AccountUploadWallPaper(ctx context.Context, in *mtproto.TLAccountUploadWallPaper) (*mtproto.WallPaper, error) {
	self, _ := selfID(ctx)
	return accountassets.UploadWallPaper(self, in)
}

func (WallpapersService) AccountSaveWallPaper(ctx context.Context, in *mtproto.TLAccountSaveWallPaper) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return accountassets.SaveWallPaper(self, in)
}

func (WallpapersService) AccountInstallWallPaper(ctx context.Context, in *mtproto.TLAccountInstallWallPaper) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return accountassets.InstallWallPaper(self, in)
}

func (WallpapersService) AccountResetWallPapers(ctx context.Context, _ *mtproto.TLAccountResetWallPapers) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return accountassets.ResetWallPapers(self), nil
}

// RingtoneService serves the saved-notification-sound RPCs.
// https://core.telegram.org/method/account.getSavedRingtones
type RingtoneService struct {
	mtproto.UnimplementedRPCRingtoneServer
}

func (RingtoneService) AccountGetSavedRingtones(ctx context.Context, in *mtproto.TLAccountGetSavedRingtones) (*mtproto.Account_SavedRingtones, error) {
	self, _ := selfID(ctx)
	return accountassets.GetSavedRingtones(self, in.GetHash()), nil
}

func (RingtoneService) AccountSaveRingtone(ctx context.Context, in *mtproto.TLAccountSaveRingtone) (*mtproto.Account_SavedRingtone, error) {
	self, _ := selfID(ctx)
	return accountassets.SaveRingtone(self, in)
}

func (RingtoneService) AccountUploadRingtone(ctx context.Context, in *mtproto.TLAccountUploadRingtone) (*mtproto.Document, error) {
	self, _ := selfID(ctx)
	return accountassets.UploadRingtone(self, in)
}

// ConnectedBotsService serves the business-bot connection RPCs.
// https://core.telegram.org/api/business
type ConnectedBotsService struct {
	mtproto.UnimplementedRPCBusinessConnectedBotsServer
}

func (ConnectedBotsService) AccountGetConnectedBots(ctx context.Context, _ *mtproto.TLAccountGetConnectedBots) (*mtproto.Account_ConnectedBots, error) {
	self, _ := selfID(ctx)
	return accountassets.GetConnectedBots(self), nil
}

func (ConnectedBotsService) AccountUpdateConnectedBot(ctx context.Context, in *mtproto.TLAccountUpdateConnectedBot) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return accountassets.UpdateConnectedBot(self, in)
}

func (ConnectedBotsService) AccountToggleConnectedBotPaused(ctx context.Context, in *mtproto.TLAccountToggleConnectedBotPaused) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return accountassets.ToggleConnectedBotPaused(self, in.GetPeer(), mtproto.FromBool(in.GetPaused()))
}

func (ConnectedBotsService) AccountDisablePeerConnectedBot(ctx context.Context, in *mtproto.TLAccountDisablePeerConnectedBot) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return accountassets.DisablePeerConnectedBot(self, in.GetPeer())
}
