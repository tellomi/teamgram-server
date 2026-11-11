package webpage

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	gmd "google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// contactFmt is the no-bio user/bot card line for every built-in language.
// %s is the username without a leading @; the format itself adds @.
// site_name stays the brand "Tellomi" in every language.
var contactFmt = map[string]string{
	"en":      "You can contact @%s right away.",
	"ar":      "يمكنك التواصل مع @%s فورًا.",
	"be":      "Вы можаце адразу звязацца з @%s.",
	"ca":      "Pots contactar amb @%s de seguida.",
	"cs":      "S @%s se můžete spojit hned teď.",
	"de":      "Du kannst @%s sofort kontaktieren.",
	"el":      "Μπορείτε να επικοινωνήσετε αμέσως με τον χρήστη @%s.",
	"es":      "Puedes contactar a @%s de inmediato.",
	"fa":      "می‌توانید همین حالا با @%s تماس بگیرید.",
	"fi":      "Voit ottaa heti yhteyttä käyttäjään @%s.",
	"fr":      "Vous pouvez contacter @%s immédiatement.",
	"he":      "אפשר ליצור קשר עם @%s מיד.",
	"hi":      "आप @%s से तुरंत संपर्क कर सकते हैं।",
	"id":      "Anda dapat menghubungi @%s sekarang juga.",
	"it":      "Puoi contattare @%s subito.",
	"ja":      "@%s にすぐに連絡できます。",
	"ko":      "@%s님에게 바로 연락할 수 있습니다.",
	"ms":      "Anda boleh menghubungi @%s sekarang juga.",
	"nb":      "Du kan kontakte @%s med en gang.",
	"nl":      "Je kunt @%s meteen bereiken.",
	"pl":      "Możesz od razu skontaktować się z @%s.",
	"pt-br":   "Você pode entrar em contato com @%s agora mesmo.",
	"ro":      "Îl poți contacta pe @%s imediat.",
	"ru":      "Вы можете сразу связаться с @%s.",
	"sv":      "Du kan kontakta @%s med en gång.",
	"th":      "คุณสามารถติดต่อ @%s ได้ทันที",
	"tr":      "@%s ile hemen iletişime geçebilirsiniz.",
	"uk":      "Ви можете одразу зв’язатися з @%s.",
	"uz":      "@%s bilan darhol bog‘lanishingiz mumkin.",
	"vi":      "Bạn có thể liên hệ với @%s ngay.",
	"zh-hans": "你可以立即联系 @%s。",
	"zh-hant": "你可以立即聯絡 @%s。",
}

// packOneFmt / packManyFmt are sticker (emoji=false) and emoji pack lines.
// %d is the count. one is used when n==1; many is used otherwise.
var (
	stickerOneFmt = map[string]string{
		"en":      "Free stickerpack with 1 sticker.",
		"ar":      "حزمة ملصقات مجانية تحتوي على ملصق واحد.",
		"be":      "Бясплатны набор з 1 стыкерам.",
		"ca":      "Pack d'adhesius gratuït amb 1 adhesiu.",
		"cs":      "Bezplatný balíček s 1 samolepkou.",
		"de":      "Kostenloses Stickerpack mit 1 Sticker.",
		"el":      "Δωρεάν πακέτο αυτοκόλλητων με 1 αυτοκόλλητο.",
		"es":      "Paquete de stickers gratis con 1 sticker.",
		"fa":      "بسته استیکر رایگان با ۱ استیکر.",
		"fi":      "Ilmainen tarrapaketti, jossa on 1 tarra.",
		"fr":      "Pack d’autocollants gratuit avec 1 autocollant.",
		"he":      "חבילת מדבקות חינם עם מדבקה אחת.",
		"hi":      "1 स्टिकर वाला मुफ़्त स्टिकर पैक।",
		"id":      "Paket stiker gratis dengan 1 stiker.",
		"it":      "Pacchetto di sticker gratuito con 1 sticker.",
		"ja":      "スタンプ 1 個の無料スタンプパック。",
		"ko":      "스티커 1개가 포함된 무료 스티커 팩.",
		"ms":      "Pek pelekat percuma dengan 1 pelekat.",
		"nb":      "Gratis klistremerkepakke med 1 klistremerke.",
		"nl":      "Gratis stickerpakket met 1 sticker.",
		"pl":      "Darmowa paczka z 1 naklejką.",
		"pt-br":   "Pacote de figurinhas grátis com 1 figurinha.",
		"ro":      "Pachet gratuit de sticker-e cu 1 sticker.",
		"ru":      "Бесплатный набор с 1 стикером.",
		"sv":      "Gratis paket med 1 dekal.",
		"th":      "ชุดสติกเกอร์ฟรีที่มี 1 สติกเกอร์",
		"tr":      "1 çıkartmalı ücretsiz çıkartma paketi.",
		"uk":      "Безкоштовний набір із 1 стікером.",
		"uz":      "1 ta stikerli bepul stiker to‘plami.",
		"vi":      "Gói nhãn dán miễn phí với 1 nhãn dán.",
		"zh-hans": "包含 1 个贴纸的免费贴纸包。",
		"zh-hant": "包含 1 個貼圖的免費貼圖包。",
	}
	stickerManyFmt = map[string]string{
		"en":      "Free stickerpack with %d stickers.",
		"ar":      "حزمة ملصقات مجانية تحتوي على %d ملصقًا.",
		"be":      "Бясплатны набор з %d стыкерамі.",
		"ca":      "Pack d'adhesius gratuït amb %d adhesius.",
		"cs":      "Bezplatný balíček s %d samolepkami.",
		"de":      "Kostenloses Stickerpack mit %d Stickern.",
		"el":      "Δωρεάν πακέτο αυτοκόλλητων με %d αυτοκόλλητα.",
		"es":      "Paquete de stickers gratis con %d stickers.",
		"fa":      "بسته استیکر رایگان با %d استیکر.",
		"fi":      "Ilmainen tarrapaketti, jossa on %d tarraa.",
		"fr":      "Pack d’autocollants gratuit avec %d autocollants.",
		"he":      "חבילת מדבקות חינם עם %d מדבקות.",
		"hi":      "%d स्टिकर वाला मुफ़्त स्टिकर पैक।",
		"id":      "Paket stiker gratis dengan %d stiker.",
		"it":      "Pacchetto di sticker gratuito con %d sticker.",
		"ja":      "スタンプ %d 個の無料スタンプパック。",
		"ko":      "스티커 %d개가 포함된 무료 스티커 팩.",
		"ms":      "Pek pelekat percuma dengan %d pelekat.",
		"nb":      "Gratis klistremerkepakke med %d klistremerker.",
		"nl":      "Gratis stickerpakket met %d stickers.",
		"pl":      "Darmowa paczka z %d naklejkami.",
		"pt-br":   "Pacote de figurinhas grátis com %d figurinhas.",
		"ro":      "Pachet gratuit de sticker-e cu %d sticker-e.",
		"ru":      "Бесплатный набор с %d стикерами.",
		"sv":      "Gratis paket med %d dekaler.",
		"th":      "ชุดสติกเกอร์ฟรีที่มี %d สติกเกอร์",
		"tr":      "%d çıkartmalı ücretsiz çıkartma paketi.",
		"uk":      "Безкоштовний набір із %d стікерами.",
		"uz":      "%d ta stikerli bepul stiker to‘plami.",
		"vi":      "Gói nhãn dán miễn phí với %d nhãn dán.",
		"zh-hans": "包含 %d 个贴纸的免费贴纸包。",
		"zh-hant": "包含 %d 個貼圖的免費貼圖包。",
	}
	emojiOneFmt = map[string]string{
		"en":      "Free emoji pack with 1 emoji.",
		"ar":      "حزمة رموز تعبيرية مجانية تحتوي على رمز واحد.",
		"be":      "Бясплатны набор з 1 эмодзі.",
		"ca":      "Pack d'emojis gratuït amb 1 emoji.",
		"cs":      "Bezplatný balíček s 1 emoji.",
		"de":      "Kostenloses Emoji-Pack mit 1 Emoji.",
		"el":      "Δωρεάν πακέτο emoji με 1 emoji.",
		"es":      "Paquete de emojis gratis con 1 emoji.",
		"fa":      "بسته اموجی رایگان با ۱ اموجی.",
		"fi":      "Ilmainen emoji-paketti, jossa on 1 emoji.",
		"fr":      "Pack d’emojis gratuit avec 1 emoji.",
		"he":      "חבילת אימוג'י חינם עם אימוג'י אחד.",
		"hi":      "1 इमोजी वाला मुफ़्त इमोजी पैक।",
		"id":      "Paket emoji gratis dengan 1 emoji.",
		"it":      "Pacchetto di emoji gratuito con 1 emoji.",
		"ja":      "絵文字 1 個の無料絵文字パック。",
		"ko":      "이모지 1개가 포함된 무료 이모지 팩.",
		"ms":      "Pek emoji percuma dengan 1 emoji.",
		"nb":      "Gratis emoji-pakke med 1 emoji.",
		"nl":      "Gratis emoji-pakket met 1 emoji.",
		"pl":      "Darmowa paczka z 1 emoji.",
		"pt-br":   "Pacote de emojis grátis com 1 emoji.",
		"ro":      "Pachet gratuit de emoji cu 1 emoji.",
		"ru":      "Бесплатный набор с 1 эмодзи.",
		"sv":      "Gratis paket med 1 emoji.",
		"th":      "ชุดอิโมจิฟรีที่มี 1 อิโมจิ",
		"tr":      "1 emojili ücretsiz emoji paketi.",
		"uk":      "Безкоштовний набір із 1 емодзі.",
		"uz":      "1 ta emojili bepul emoji to‘plami.",
		"vi":      "Gói emoji miễn phí với 1 emoji.",
		"zh-hans": "包含 1 个表情的免费表情包。",
		"zh-hant": "包含 1 個表情的免費表情包。",
	}
	emojiManyFmt = map[string]string{
		"en":      "Free emoji pack with %d emoji.",
		"ar":      "حزمة رموز تعبيرية مجانية تحتوي على %d رمزًا.",
		"be":      "Бясплатны набор з %d эмодзі.",
		"ca":      "Pack d'emojis gratuït amb %d emojis.",
		"cs":      "Bezplatný balíček s %d emoji.",
		"de":      "Kostenloses Emoji-Pack mit %d Emojis.",
		"el":      "Δωρεάν πακέτο emoji με %d emoji.",
		"es":      "Paquete de emojis gratis con %d emojis.",
		"fa":      "بسته اموجی رایگان با %d اموجی.",
		"fi":      "Ilmainen emoji-paketti, jossa on %d emojia.",
		"fr":      "Pack d’emojis gratuit avec %d emojis.",
		"he":      "חבילת אימוג'י חינם עם %d אימוג'ים.",
		"hi":      "%d इमोजी वाला मुफ़्त इमोजी पैक।",
		"id":      "Paket emoji gratis dengan %d emoji.",
		"it":      "Pacchetto di emoji gratuito con %d emoji.",
		"ja":      "絵文字 %d 個の無料絵文字パック。",
		"ko":      "이모지 %d개가 포함된 무료 이모지 팩.",
		"ms":      "Pek emoji percuma dengan %d emoji.",
		"nb":      "Gratis emoji-pakke med %d emojier.",
		"nl":      "Gratis emoji-pakket met %d emoji's.",
		"pl":      "Darmowa paczka z %d emoji.",
		"pt-br":   "Pacote de emojis grátis com %d emojis.",
		"ro":      "Pachet gratuit de emoji cu %d emoji.",
		"ru":      "Бесплатный набор с %d эмодзи.",
		"sv":      "Gratis paket med %d emojis.",
		"th":      "ชุดอิโมจิฟรีที่มี %d อิโมจิ",
		"tr":      "%d emojili ücretsiz emoji paketi.",
		"uk":      "Безкоштовний набір із %d емодзі.",
		"uz":      "%d ta emojili bepul emoji to‘plami.",
		"vi":      "Gói emoji miễn phí với %d emoji.",
		"zh-hans": "包含 %d 个表情的免费表情包。",
		"zh-hant": "包含 %d 個表情的免費表情包。",
	}
)

var atUserRE = regexp.MustCompile(`@([A-Za-z][A-Za-z0-9_]{3,31})`)
var countRE = regexp.MustCompile(`\d+`)

func cardLang(ctx context.Context) string {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil || strings.TrimSpace(md.GetLangCode()) == "" {
		return "en"
	}
	code, _ := langpack.CanonicalLang(strings.TrimSpace(md.GetLangCode()))
	if code == "" {
		return "en"
	}
	return code
}

func pickFmt(table map[string]string, lang string) string {
	if s := table[lang]; s != "" {
		return s
	}
	return table["en"]
}

func contactLine(ctx context.Context, username string) string {
	return fmt.Sprintf(pickFmt(contactFmt, cardLang(ctx)), username)
}

func packLine(ctx context.Context, n int32, emoji bool) string {
	if n < 0 {
		n = 0
	}
	lang := cardLang(ctx)
	if emoji {
		if n == 1 {
			return pickFmt(emojiOneFmt, lang)
		}
		return fmt.Sprintf(pickFmt(emojiManyFmt, lang), n)
	}
	if n == 1 {
		return pickFmt(stickerOneFmt, lang)
	}
	return fmt.Sprintf(pickFmt(stickerManyFmt, lang), n)
}

// LocalizeInternalPage rewrites generated site copy on a stored telegram_*
// card into the viewer's language. User bios, channel abouts, and message
// text are left alone. site_name stays "Tellomi".
//
// A message's card is stored once and delivered to both sides in the
// sender's language. Call this when a later request (getWebPage,
// getWebPagePreview, or a future history hook) has the viewer's lang_code.
func LocalizeInternalPage(ctx context.Context, page *mtproto.WebPage) {
	if page == nil || page.GetPredicateName() != mtproto.Predicate_webPage {
		return
	}
	typ := page.GetType().GetValue()
	desc := page.GetDescription().GetValue()
	if desc == "" {
		return
	}
	switch typ {
	case "telegram_user", "telegram_bot":
		if user, ok := matchContact(desc); ok {
			page.Description = wrapperspb.String(contactLine(ctx, user))
		}
	case "telegram_stickerset":
		if n, emoji, ok := matchPack(desc); ok {
			page.Description = wrapperspb.String(packLine(ctx, n, emoji))
		}
	case "telegram_chat", "telegram_channel", "telegram_megagroup",
		"telegram_channel_request", "telegram_megagroup_request":
		if inviter, kind, ok := matchInvite(desc); ok {
			page.Description = wrapperspb.String(inviteLine(ctx, inviter, kind))
		} else if n, subs, ok := matchAudience(ctx, desc); ok {
			page.Description = wrapperspb.String(audienceLine(ctx, n, subs))
		}
	}
}

func matchContact(desc string) (string, bool) {
	m := atUserRE.FindStringSubmatch(desc)
	if m == nil {
		return "", false
	}
	user := m[1]
	for lang := range contactFmt {
		if fmt.Sprintf(contactFmt[lang], user) == desc {
			return user, true
		}
	}
	return "", false
}

func matchPack(desc string) (int32, bool, bool) {
	for _, emoji := range []bool{false, true} {
		for lang := range stickerOneFmt {
			if packLine(langCtx(lang), 1, emoji) == desc {
				return 1, emoji, true
			}
		}
		n := int32(0)
		if loc := countRE.FindString(desc); loc != "" {
			fmt.Sscanf(loc, "%d", &n)
		}
		if n > 1 {
			for lang := range stickerManyFmt {
				if packLine(langCtx(lang), n, emoji) == desc {
					return n, emoji, true
				}
			}
		}
	}
	return 0, false, false
}

func matchAudience(ctx context.Context, desc string) (int32, bool, bool) {
	n := int32(0)
	if loc := countRE.FindString(desc); loc != "" {
		fmt.Sscanf(loc, "%d", &n)
	}
	if n <= 0 {
		return 0, false, false
	}
	// Compare against every official language, members and subscribers.
	for _, lang := range officialCodes() {
		c := langCtx(lang)
		if audienceLine(c, n, false) == desc {
			return n, false, true
		}
		if audienceLine(c, n, true) == desc {
			return n, true, true
		}
	}
	_ = ctx
	return 0, false, false
}

func officialCodes() []string {
	out := make([]string, 0, len(langpack.OfficialLanguages))
	for _, lang := range langpack.OfficialLanguages {
		out = append(out, lang.Code)
	}
	return out
}

func langCtx(code string) context.Context {
	// withLang in tests uses rpc metadata; keep a tiny helper here too.
	return langContext(code, "ios")
}

func langContext(code, pack string) context.Context {
	buf, err := proto.Marshal(&metadata.RpcMetadata{LangCode: code, Langpack: pack})
	if err != nil {
		return context.Background()
	}
	return gmd.NewIncomingContext(context.Background(), gmd.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}
