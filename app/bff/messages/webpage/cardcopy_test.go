package webpage

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestContactAndPackCopyEveryLanguage(t *testing.T) {
	if got, want := len(contactFmt), len(langpack.OfficialLanguages); got != want {
		t.Fatalf("contactFmt %d languages, official %d", got, want)
	}
	for _, lang := range langpack.OfficialLanguages {
		if contactFmt[lang.Code] == "" || stickerOneFmt[lang.Code] == "" || stickerManyFmt[lang.Code] == "" || emojiOneFmt[lang.Code] == "" || emojiManyFmt[lang.Code] == "" {
			t.Fatalf("missing card copy for %s", lang.Code)
		}
		ctx := withLang(t, lang.Code, "ios")
		got := contactLine(ctx, "erel")
		if !strings.Contains(got, "@erel") {
			t.Fatalf("%s contact missing @erel: %q", lang.Code, got)
		}
		if lang.Code != "en" && got == contactFmt["en"] {
			t.Fatalf("%s fell back to English contact line", lang.Code)
		}
		if packLine(ctx, 1, false) == "" || packLine(ctx, 5, true) == "" {
			t.Fatalf("%s empty pack line", lang.Code)
		}
		if lang.Code != "en" && packLine(ctx, 5, false) == packLine(context.Background(), 5, false) {
			t.Fatalf("%s sticker pack fell back to English", lang.Code)
		}
	}
	zh := withLang(t, "zh-hans", "ios")
	if got := contactLine(zh, "erel"); got != "你可以立即联系 @erel。" {
		t.Fatalf("zh contact %q", got)
	}
	if got := packLine(zh, 5, false); got != "包含 5 个贴纸的免费贴纸包。" {
		t.Fatalf("zh stickers %q", got)
	}
	if got := packLine(zh, 1, true); got != "包含 1 个表情的免费表情包。" {
		t.Fatalf("zh emoji %q", got)
	}
	if got := contactLine(context.Background(), "quiet"); got != "You can contact @quiet right away." {
		t.Fatalf("default en %q", got)
	}
}

func TestLocalizeInternalPagePerViewer(t *testing.T) {
	page := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Type:        wrapperspb.String("telegram_user"),
		SiteName:    wrapperspb.String(siteTellomi),
		Description: wrapperspb.String("You can contact @erel right away."),
	}).To_WebPage()
	LocalizeInternalPage(withLang(t, "zh-hans", "ios"), page)
	if page.GetDescription().GetValue() != "你可以立即联系 @erel。" {
		t.Fatalf("localized %q", page.GetDescription().GetValue())
	}
	if page.GetSiteName().GetValue() != siteTellomi {
		t.Fatalf("site_name changed %q", page.GetSiteName().GetValue())
	}
	bio := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Type:        wrapperspb.String("telegram_user"),
		Description: wrapperspb.String("hello from erel"),
	}).To_WebPage()
	LocalizeInternalPage(withLang(t, "zh-hans", "ios"), bio)
	if bio.GetDescription().GetValue() != "hello from erel" {
		t.Fatal("user bio was rewritten")
	}
	pack := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Type:        wrapperspb.String("telegram_stickerset"),
		Description: wrapperspb.String("Free stickerpack with 5 stickers."),
	}).To_WebPage()
	LocalizeInternalPage(withLang(t, "zh-hans", "ios"), pack)
	if pack.GetDescription().GetValue() != "包含 5 个贴纸的免费贴纸包。" {
		t.Fatalf("pack %q", pack.GetDescription().GetValue())
	}
}

func TestUserCardFollowsLangCode(t *testing.T) {
	BindUserResolver(func(_ context.Context, name string) (UserCard, bool) {
		if name != "quiet" {
			return UserCard{}, false
		}
		return UserCard{ID: 7, Title: "Quiet", Username: "quiet"}, true
	})
	t.Cleanup(func() { BindUserResolver(nil) })
	zh, _, _, err := Get(withLang(t, "zh-hans", "ios"), "https://tell.cc/quiet", 0)
	if err != nil || zh.GetDescription().GetValue() != "你可以立即联系 @quiet。" {
		t.Fatalf("zh user card %q %v", zh.GetDescription().GetValue(), err)
	}
	en, _, _, err := Get(context.Background(), "https://tell.cc/quiet", 0)
	if err != nil || en.GetDescription().GetValue() != "You can contact @quiet right away." {
		t.Fatalf("en user card %q %v", en.GetDescription().GetValue(), err)
	}
}

func TestInviteCopyEveryLanguage(t *testing.T) {
	tables := []map[string]string{inviteNamedGroupFmt, inviteNamedChannelFmt, inviteAnonGroupFmt, inviteAnonChannelFmt}
	for _, table := range tables {
		if got, want := len(table), len(langpack.OfficialLanguages); got != want {
			t.Fatalf("invite table %d languages, official %d", got, want)
		}
		for _, lang := range langpack.OfficialLanguages {
			text := table[lang.Code]
			if text == "" {
				t.Fatalf("missing invite copy for %s", lang.Code)
			}
			if !strings.Contains(text, "Tellomi") {
				t.Fatalf("%s missing Tellomi: %q", lang.Code, text)
			}
			if strings.Contains(strings.ToLower(text), "telegram") {
				t.Fatalf("%s used Telegram: %q", lang.Code, text)
			}
		}
		if strings.ToLower(table["en"]) == strings.ToLower(table["en"]) && strings.Contains(strings.ToLower(table["en"]), "telegram") {
			t.Fatal("english invite used Telegram")
		}
	}
	zh := withLang(t, "zh-hans", "ios")
	if got := inviteLine(zh, "Erel", "group"); got != "Erel 邀请你加入 Tellomi 上的这个群组。" {
		t.Fatalf("zh named group %q", got)
	}
	if got := inviteLine(zh, "", "channel"); got != "你被邀请加入 Tellomi 上的这个频道。" {
		t.Fatalf("zh anon channel %q", got)
	}
	if got := inviteLine(context.Background(), "Erel", "group"); got != "Erel invites you to join this group on Tellomi." {
		t.Fatalf("en named group %q", got)
	}
	if got := inviteLine(context.Background(), "", "group"); got != "You're invited to join this group on Tellomi." {
		t.Fatalf("en anon group %q", got)
	}
}

func TestInviteCardFollowsLangCode(t *testing.T) {
	hash := "1" + strings.Repeat("c", 19)
	BindGroupResolver(func(context.Context, string) (GroupCard, bool) {
		return GroupCard{Title: "Room", Members: 5, Inviter: "Erel"}, true
	})
	t.Cleanup(func() { BindGroupResolver(nil) })
	zh, _, _, err := Get(withLang(t, "zh-hans", "ios"), "https://tell.cc/+"+hash, 0)
	if err != nil || zh.GetDescription().GetValue() != "Erel 邀请你加入 Tellomi 上的这个群组。" {
		t.Fatalf("zh invite %q %v", zh.GetDescription().GetValue(), err)
	}
	if zh.GetSiteName().GetValue() != siteTellomi {
		t.Fatalf("site %q", zh.GetSiteName().GetValue())
	}
	en, _, _, err := Get(context.Background(), "https://tell.cc/+"+hash, 0)
	if err != nil || en.GetDescription().GetValue() != "Erel invites you to join this group on Tellomi." {
		t.Fatalf("en invite %q %v", en.GetDescription().GetValue(), err)
	}
	LocalizeInternalPage(withLang(t, "zh-hans", "ios"), en)
	if en.GetDescription().GetValue() != "Erel 邀请你加入 Tellomi 上的这个群组。" {
		t.Fatalf("localized stored invite %q", en.GetDescription().GetValue())
	}
}

func TestGeneratedCopyNeverSaysTelegram(t *testing.T) {
	for _, lang := range langpack.OfficialLanguages {
		ctx := withLang(t, lang.Code, "ios")
		samples := []string{
			contactLine(ctx, "erel"),
			packLine(ctx, 5, false),
			packLine(ctx, 1, true),
			inviteLine(ctx, "Erel", "group"),
			inviteLine(ctx, "Erel", "channel"),
			inviteLine(ctx, "", "group"),
			inviteLine(ctx, "", "channel"),
			siteTellomi,
		}
		for _, s := range samples {
			if strings.Contains(strings.ToLower(s), "telegram") {
				t.Fatalf("%s leaked Telegram: %q", lang.Code, s)
			}
		}
	}
}
