package webpage

import (
	"context"
	"fmt"
	"strings"
)

var (
	inviteNamedGroupFmt = map[string]string{
		"en":      "%s invites you to join this group on Tellomi.",
		"ar":      "يدعوك %s للانضمام إلى هذه المجموعة على Tellomi.",
		"be":      "%s запрашае вас далучыцца да гэтай групы ў Tellomi.",
		"ca":      "%s t'convida a unir-te a aquest grup a Tellomi.",
		"cs":      "%s vás zve do této skupiny na Tellomi.",
		"de":      "%s lädt dich ein, dieser Gruppe auf Tellomi beizutreten.",
		"el":      "Ο χρήστης %s σας προσκαλεί να συμμετάσχετε σε αυτήν την ομάδα στο Tellomi.",
		"es":      "%s te invita a unirte a este grupo en Tellomi.",
		"fa":      "%s شما را به پیوستن به این گروه در Tellomi دعوت می‌کند.",
		"fi":      "%s kutsuu sinut tähän ryhmään Tellomissa.",
		"fr":      "%s vous invite à rejoindre ce groupe sur Tellomi.",
		"he":      "%s מזמין אותך להצטרף לקבוצה הזאת ב-Tellomi.",
		"hi":      "%s आपको Tellomi पर इस समूह में शामिल होने के लिए आमंत्रित करते हैं।",
		"id":      "%s mengundang Anda untuk bergabung dengan grup ini di Tellomi.",
		"it":      "%s ti invita a unirti a questo gruppo su Tellomi.",
		"ja":      "%s さんから Tellomi のこのグループへの招待が届いています。",
		"ko":      "%s님이 Tellomi의 이 그룹에 초대했습니다.",
		"ms":      "%s menjemput anda untuk menyertai kumpulan ini di Tellomi.",
		"nb":      "%s inviterer deg til å bli med i denne gruppen på Tellomi.",
		"nl":      "%s nodigt je uit om lid te worden van deze groep op Tellomi.",
		"pl":      "%s zaprasza cię do tej grupy w Tellomi.",
		"pt-br":   "%s convida você para entrar neste grupo no Tellomi.",
		"ro":      "%s te invită să te alături acestui grup pe Tellomi.",
		"ru":      "%s приглашает вас в эту группу в Tellomi.",
		"sv":      "%s bjuder in dig att gå med i den här gruppen på Tellomi.",
		"th":      "%s เชิญคุณเข้าร่วมกลุ่มนี้บน Tellomi",
		"tr":      "%s sizi Tellomi'deki bu gruba katılmaya davet ediyor.",
		"uk":      "%s запрошує вас долучитися до цієї групи в Tellomi.",
		"uz":      "%s sizni Tellomi’dagi ushbu guruhga qo‘shilishga taklif qiladi.",
		"vi":      "%s mời bạn tham gia nhóm này trên Tellomi.",
		"zh-hans": "%s 邀请你加入 Tellomi 上的这个群组。",
		"zh-hant": "%s 邀請你加入 Tellomi 上的這個群組。",
	}
	inviteNamedChannelFmt = map[string]string{
		"en":      "%s invites you to join this channel on Tellomi.",
		"ar":      "يدعوك %s للانضمام إلى هذه القناة على Tellomi.",
		"be":      "%s запрашае вас далучыцца да гэтага канала ў Tellomi.",
		"ca":      "%s t'convida a unir-te a aquest canal a Tellomi.",
		"cs":      "%s vás zve na tento kanál na Tellomi.",
		"de":      "%s lädt dich ein, diesem Kanal auf Tellomi beizutreten.",
		"el":      "Ο χρήστης %s σας προσκαλεί να συμμετάσχετε σε αυτό το κανάλι στο Tellomi.",
		"es":      "%s te invita a unirte a este canal en Tellomi.",
		"fa":      "%s شما را به پیوستن به این کانال در Tellomi دعوت می‌کند.",
		"fi":      "%s kutsuu sinut tälle kanavalle Tellomissa.",
		"fr":      "%s vous invite à rejoindre cette chaîne sur Tellomi.",
		"he":      "%s מזמין אותך להצטרף לערוץ הזה ב-Tellomi.",
		"hi":      "%s आपको Tellomi पर इस चैनल में शामिल होने के लिए आमंत्रित करते हैं।",
		"id":      "%s mengundang Anda untuk bergabung dengan saluran ini di Tellomi.",
		"it":      "%s ti invita a unirti a questo canale su Tellomi.",
		"ja":      "%s さんから Tellomi のこのチャンネルへの招待が届いています。",
		"ko":      "%s님이 Tellomi의 이 채널에 초대했습니다.",
		"ms":      "%s menjemput anda untuk menyertai saluran ini di Tellomi.",
		"nb":      "%s inviterer deg til å bli med i denne kanalen på Tellomi.",
		"nl":      "%s nodigt je uit om lid te worden van dit kanaal op Tellomi.",
		"pl":      "%s zaprasza cię do tego kanału w Tellomi.",
		"pt-br":   "%s convida você para entrar neste canal no Tellomi.",
		"ro":      "%s te invită să te alături acestui canal pe Tellomi.",
		"ru":      "%s приглашает вас на этот канал в Tellomi.",
		"sv":      "%s bjuder in dig att gå med i den här kanalen på Tellomi.",
		"th":      "%s เชิญคุณเข้าร่วมช่องนี้บน Tellomi",
		"tr":      "%s sizi Tellomi'deki bu kanala katılmaya davet ediyor.",
		"uk":      "%s запрошує вас долучитися до цього каналу в Tellomi.",
		"uz":      "%s sizni Tellomi’dagi ushbu kanalga qo‘shilishga taklif qiladi.",
		"vi":      "%s mời bạn tham gia kênh này trên Tellomi.",
		"zh-hans": "%s 邀请你加入 Tellomi 上的这个频道。",
		"zh-hant": "%s 邀請你加入 Tellomi 上的這個頻道。",
	}
	inviteAnonGroupFmt = map[string]string{
		"en":      "You're invited to join this group on Tellomi.",
		"ar":      "تمت دعوتك للانضمام إلى هذه المجموعة على Tellomi.",
		"be":      "Вас запрашаюць далучыцца да гэтай групы ў Tellomi.",
		"ca":      "Estàs convidat a unir-te a aquest grup a Tellomi.",
		"cs":      "Jste zváni do této skupiny na Tellomi.",
		"de":      "Du bist eingeladen, dieser Gruppe auf Tellomi beizutreten.",
		"el":      "Έχετε προσκληθεί να συμμετάσχετε σε αυτήν την ομάδα στο Tellomi.",
		"es":      "Estás invitado a unirte a este grupo en Tellomi.",
		"fa":      "شما به پیوستن به این گروه در Tellomi دعوت شده‌اید.",
		"fi":      "Sinut on kutsuttu tähän ryhmään Tellomissa.",
		"fr":      "Vous êtes invité à rejoindre ce groupe sur Tellomi.",
		"he":      "הוזמנת להצטרף לקבוצה הזאת ב-Tellomi.",
		"hi":      "आपको Tellomi पर इस समूह में शामिल होने के लिए आमंत्रित किया गया है।",
		"id":      "Anda diundang untuk bergabung dengan grup ini di Tellomi.",
		"it":      "Sei invitato a unirti a questo gruppo su Tellomi.",
		"ja":      "Tellomi のこのグループに招待されています。",
		"ko":      "Tellomi의 이 그룹에 초대되었습니다.",
		"ms":      "Anda dijemput untuk menyertai kumpulan ini di Tellomi.",
		"nb":      "Du er invitert til å bli med i denne gruppen på Tellomi.",
		"nl":      "Je bent uitgenodigd om lid te worden van deze groep op Tellomi.",
		"pl":      "Zapraszamy cię do tej grupy w Tellomi.",
		"pt-br":   "Você foi convidado para entrar neste grupo no Tellomi.",
		"ro":      "Ești invitat să te alături acestui grup pe Tellomi.",
		"ru":      "Вас приглашают в эту группу в Tellomi.",
		"sv":      "Du är inbjuden att gå med i den här gruppen på Tellomi.",
		"th":      "คุณได้รับเชิญให้เข้าร่วมกลุ่มนี้บน Tellomi",
		"tr":      "Tellomi'deki bu gruba katılmaya davet edildiniz.",
		"uk":      "Вас запрошують долучитися до цієї групи в Tellomi.",
		"uz":      "Siz Tellomi’dagi ushbu guruhga qo‘shilishga taklif qilinasiz.",
		"vi":      "Bạn được mời tham gia nhóm này trên Tellomi.",
		"zh-hans": "你被邀请加入 Tellomi 上的这个群组。",
		"zh-hant": "你被邀請加入 Tellomi 上的這個群組。",
	}
	inviteAnonChannelFmt = map[string]string{
		"en":      "You're invited to join this channel on Tellomi.",
		"ar":      "تمت دعوتك للانضمام إلى هذه القناة على Tellomi.",
		"be":      "Вас запрашаюць далучыцца да гэтага канала ў Tellomi.",
		"ca":      "Estàs convidat a unir-te a aquest canal a Tellomi.",
		"cs":      "Jste zváni na tento kanál na Tellomi.",
		"de":      "Du bist eingeladen, diesem Kanal auf Tellomi beizutreten.",
		"el":      "Έχετε προσκληθεί να συμμετάσχετε σε αυτό το κανάλι στο Tellomi.",
		"es":      "Estás invitado a unirte a este canal en Tellomi.",
		"fa":      "شما به پیوستن به این کانال در Tellomi دعوت شده‌اید.",
		"fi":      "Sinut on kutsuttu tälle kanavalle Tellomissa.",
		"fr":      "Vous êtes invité à rejoindre cette chaîne sur Tellomi.",
		"he":      "הוזמנת להצטרף לערוץ הזה ב-Tellomi.",
		"hi":      "आपको Tellomi पर इस चैनल में शामिल होने के लिए आमंत्रित किया गया है।",
		"id":      "Anda diundang untuk bergabung dengan saluran ini di Tellomi.",
		"it":      "Sei invitato a unirti a questo canale su Tellomi.",
		"ja":      "Tellomi のこのチャンネルに招待されています。",
		"ko":      "Tellomi의 이 채널에 초대되었습니다.",
		"ms":      "Anda dijemput untuk menyertai saluran ini di Tellomi.",
		"nb":      "Du er invitert til å bli med i denne kanalen på Tellomi.",
		"nl":      "Je bent uitgenodigd om lid te worden van dit kanaal op Tellomi.",
		"pl":      "Zapraszamy cię do tego kanału w Tellomi.",
		"pt-br":   "Você foi convidado para entrar neste canal no Tellomi.",
		"ro":      "Ești invitat să te alături acestui canal pe Tellomi.",
		"ru":      "Вас приглашают на этот канал в Tellomi.",
		"sv":      "Du är inbjuden att gå med i den här kanalen på Tellomi.",
		"th":      "คุณได้รับเชิญให้เข้าร่วมช่องนี้บน Tellomi",
		"tr":      "Tellomi'deki bu kanala katılmaya davet edildiniz.",
		"uk":      "Вас запрошують долучитися до цього каналу в Tellomi.",
		"uz":      "Siz Tellomi’dagi ushbu kanalga qo‘shilishga taklif qilinasiz.",
		"vi":      "Bạn được mời tham gia kênh này trên Tellomi.",
		"zh-hans": "你被邀请加入 Tellomi 上的这个频道。",
		"zh-hant": "你被邀請加入 Tellomi 上的這個頻道。",
	}
)

func inviteLine(ctx context.Context, inviter, kind string) string {
	lang := cardLang(ctx)
	inviter = strings.TrimSpace(inviter)
	channel := kind == "channel"
	if inviter != "" {
		if channel {
			return fmt.Sprintf(pickFmt(inviteNamedChannelFmt, lang), inviter)
		}
		return fmt.Sprintf(pickFmt(inviteNamedGroupFmt, lang), inviter)
	}
	if channel {
		return pickFmt(inviteAnonChannelFmt, lang)
	}
	return pickFmt(inviteAnonGroupFmt, lang)
}

func matchInvite(desc string) (inviter, kind string, ok bool) {
	for _, fmtStr := range inviteNamedGroupFmt {
		if name, yes := extractNamed(fmtStr, desc); yes {
			return name, "group", true
		}
	}
	for _, fmtStr := range inviteNamedChannelFmt {
		if name, yes := extractNamed(fmtStr, desc); yes {
			return name, "channel", true
		}
	}
	for _, s := range inviteAnonGroupFmt {
		if s == desc {
			return "", "group", true
		}
	}
	for _, s := range inviteAnonChannelFmt {
		if s == desc {
			return "", "channel", true
		}
	}
	return "", "", false
}

func extractNamed(fmtStr, desc string) (string, bool) {
	i := strings.Index(fmtStr, "%s")
	if i < 0 {
		return "", false
	}
	prefix, suffix := fmtStr[:i], fmtStr[i+2:]
	if !strings.HasPrefix(desc, prefix) || !strings.HasSuffix(desc, suffix) {
		return "", false
	}
	name := desc[len(prefix) : len(desc)-len(suffix)]
	if strings.TrimSpace(name) == "" {
		return "", false
	}
	return name, true
}
