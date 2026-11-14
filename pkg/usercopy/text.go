// Package usercopy is the language for text this server writes itself
// and sends to a client: app-update dialogs, invite lines, email login
// codes, the 777000 notices, and the account-deletion notice.
// Chat service actions stay TL constructors. The client renders those
// from langpack.
package usercopy

import (
	"fmt"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/langpack"
)

func resolve(lang string) string {
	return langpack.ClientLang(lang, "", "")
}

func pick(table map[string]string, lang string) string {
	if s := strings.TrimSpace(table[resolve(lang)]); s != "" {
		return s
	}
	return table["en"]
}

// AppUpdate is the built-in help.appUpdate dialog sentence.
// A manifest note or a changelog string replaces it.
func AppUpdate(lang string) string {
	return pick(appUpdate, lang)
}

// Invite is the help.getInviteText sentence, including the link.
func Invite(lang, link string) string {
	return pick(invite, lang) + link
}

// Support is the help.getSupportName label.
// A loaded profile name wins.
func Support(lang, profileName string) string {
	if name := strings.TrimSpace(profileName); name != "" {
		return name
	}
	return pick(supportName, lang)
}

// AccountDeletionPending is the service notification sent when a deletion
// is scheduled.
func AccountDeletionPending(lang string) string {
	return pick(deletionPending, lang)
}

// EmailCode is the subject and plain body of a login-code email.
// ok is false when lang is empty, so a sender that does not know the
// client keeps its previous text.
func EmailCode(lang, code string) (subject, body string, ok bool) {
	if strings.TrimSpace(lang) == "" {
		return "", "", false
	}
	code = strings.TrimSpace(code)
	return pick(emailSubject, lang), fmt.Sprintf(pick(emailBody, lang), code), true
}

// appUpdate is one sentence. zh-hans is the copy Simplified Chinese
// clients see when latest.json has no changelog and no notes.
var appUpdate = map[string]string{
	"en":      "A new version of Tellomi is available.",
	"ar":      "يتوفر إصدار جديد من Tellomi.",
	"be":      "Даступная новая версія Tellomi.",
	"ca":      "Hi ha una nova versió de Tellomi.",
	"cs":      "Je k dispozici nová verze Tellomi.",
	"de":      "Eine neue Version von Tellomi ist verfügbar.",
	"el":      "Υπάρχει μια νέα έκδοση του Tellomi.",
	"es":      "Hay una nueva versión de Tellomi.",
	"fa":      "نسخهٔ جدیدی از Tellomi در دسترس است.",
	"fi":      "Uusi Tellomi-versio on saatavilla.",
	"fr":      "Une nouvelle version de Tellomi est disponible.",
	"he":      "גרסה חדשה של Tellomi זמינה.",
	"hi":      "Tellomi का नया संस्करण उपलब्ध है।",
	"id":      "Versi baru Tellomi sudah tersedia.",
	"it":      "È disponibile una nuova versione di Tellomi.",
	"ja":      "Tellomi の新しいバージョンがあります。",
	"ko":      "Tellomi의 새 버전이 있습니다.",
	"ms":      "Versi baharu Tellomi tersedia.",
	"nb":      "En ny versjon av Tellomi er tilgjengelig.",
	"nl":      "Er is een nieuwe versie van Tellomi beschikbaar.",
	"pl":      "Dostępna jest nowa wersja Tellomi.",
	"pt-br":   "Há uma nova versão do Tellomi.",
	"ro":      "Este disponibilă o versiune nouă de Tellomi.",
	"ru":      "Доступна новая версия Tellomi.",
	"sv":      "En ny version av Tellomi finns tillgänglig.",
	"th":      "Tellomi มีเวอร์ชันใหม่แล้ว",
	"tr":      "Tellomi'nin yeni bir sürümü var.",
	"uk":      "Доступна нова версія Tellomi.",
	"uz":      "Tellomi’ning yangi versiyasi chiqdi.",
	"vi":      "Đã có phiên bản Tellomi mới.",
	"zh-hans": "发现新版本 Tellomi，建议更新。",
	"zh-hant": "發現新版本 Tellomi，建議更新。",
}

var invite = map[string]string{
	"en":      "I'm on Tellomi. Join me: ",
	"ar":      "أنا على Tellomi. انضم إلي: ",
	"be":      "Я ў Tellomi. Далучайся: ",
	"ca":      "Soc a Tellomi. Uneix-t'hi: ",
	"cs":      "Jsem na Tellomi. Přidej se: ",
	"de":      "Ich bin bei Tellomi. Mach mit: ",
	"el":      "Είμαι στο Tellomi. Έλα κι εσύ: ",
	"es":      "Estoy en Tellomi. Únete: ",
	"fa":      "من در Tellomi هستم. تو هم بیا: ",
	"fi":      "Olen Tellomissa. Liity mukaan: ",
	"fr":      "Je suis sur Tellomi. Rejoins-moi : ",
	"he":      "אני ב-Tellomi. הצטרפו אליי: ",
	"hi":      "मैं Tellomi पर हूँ। मेरे साथ जुड़ें: ",
	"id":      "Saya di Tellomi. Bergabunglah: ",
	"it":      "Sono su Tellomi. Unisciti a me: ",
	"ja":      "Tellomi を使っています。参加してください: ",
	"ko":      "Tellomi를 쓰고 있어요. 함께해요: ",
	"ms":      "Saya di Tellomi. Sertai saya: ",
	"nb":      "Jeg er på Tellomi. Bli med: ",
	"nl":      "Ik gebruik Tellomi. Doe mee: ",
	"pl":      "Jestem na Tellomi. Dołącz: ",
	"pt-br":   "Estou no Tellomi. Venha comigo: ",
	"ro":      "Sunt pe Tellomi. Alătură-te: ",
	"ru":      "Я в Tellomi. Присоединяйся: ",
	"sv":      "Jag finns på Tellomi. Gå med: ",
	"th":      "ฉันใช้ Tellomi มาเข้าร่วม: ",
	"tr":      "Tellomi'deyim. Sen de katıl: ",
	"uk":      "Я в Tellomi. Приєднуйся: ",
	"uz":      "Men Tellomi’daman. Qo‘shiling: ",
	"vi":      "Mình đang dùng Tellomi. Tham gia nhé: ",
	"zh-hans": "我在用 Tellomi 聊天，来加入我：",
	"zh-hant": "我在用 Tellomi 聊天，來加入我：",
}

var supportName = map[string]string{
	"en":      "Tellomi Support",
	"ar":      "دعم Tellomi",
	"be":      "Падтрымка Tellomi",
	"ca":      "Suport de Tellomi",
	"cs":      "Podpora Tellomi",
	"de":      "Tellomi-Support",
	"el":      "Υποστήριξη Tellomi",
	"es":      "Soporte de Tellomi",
	"fa":      "پشتیبانی Tellomi",
	"fi":      "Tellomi-tuki",
	"fr":      "Assistance Tellomi",
	"he":      "תמיכת Tellomi",
	"hi":      "Tellomi सहायता",
	"id":      "Dukungan Tellomi",
	"it":      "Assistenza Tellomi",
	"ja":      "Tellomi サポート",
	"ko":      "Tellomi 지원",
	"ms":      "Sokongan Tellomi",
	"nb":      "Tellomi-støtte",
	"nl":      "Tellomi-ondersteuning",
	"pl":      "Pomoc Tellomi",
	"pt-br":   "Suporte do Tellomi",
	"ro":      "Asistență Tellomi",
	"ru":      "Поддержка Tellomi",
	"sv":      "Tellomi-support",
	"th":      "การสนับสนุน Tellomi",
	"tr":      "Tellomi Destek",
	"uk":      "Підтримка Tellomi",
	"uz":      "Tellomi yordami",
	"vi":      "Hỗ trợ Tellomi",
	"zh-hans": "Tellomi 支持",
	"zh-hant": "Tellomi 支援",
}

var deletionPending = map[string]string{
	"en":      "Your Tellomi account will be deleted in 7 days. Sign in again before then to cancel.",
	"ar":      "سيُحذف حسابك في Tellomi بعد 7 أيام. سجّل الدخول مرة أخرى قبل ذلك للإلغاء.",
	"be":      "Ваш акаўнт Tellomi будзе выдалены праз 7 дзён. Увайдзіце зноў да гэтага тэрміну, каб скасаваць.",
	"ca":      "El teu compte de Tellomi s'eliminarà d'aquí a 7 dies. Torna a iniciar la sessió abans per cancel·lar-ho.",
	"cs":      "Váš účet Tellomi bude za 7 dní smazán. Přihlaste se znovu předtím a zrušíte to.",
	"de":      "Dein Tellomi-Konto wird in 7 Tagen gelöscht. Melde dich vorher erneut an, um das abzubrechen.",
	"el":      "Ο λογαριασμός σου στο Tellomi θα διαγραφεί σε 7 ημέρες. Συνδέσου ξανά πριν από τότε για ακύρωση.",
	"es":      "Tu cuenta de Tellomi se eliminará en 7 días. Vuelve a iniciar sesión antes para cancelarlo.",
	"fa":      "حساب Tellomi شما تا ۷ روز دیگر حذف می‌شود. برای لغو، پیش از آن دوباره وارد شوید.",
	"fi":      "Tellomi-tilisi poistetaan 7 päivän kuluttua. Kirjaudu uudelleen ennen sitä, jos haluat perua.",
	"fr":      "Votre compte Tellomi sera supprimé dans 7 jours. Reconnectez-vous avant pour annuler.",
	"he":      "חשבון Tellomi שלך יימחק בעוד 7 ימים. התחברו שוב לפני כן כדי לבטל.",
	"hi":      "आपका Tellomi खाता 7 दिनों में हटा दिया जाएगा। रद्द करने के लिए उससे पहले फिर से साइन इन करें।",
	"id":      "Akun Tellomi Anda akan dihapus dalam 7 hari. Masuk lagi sebelum itu untuk membatalkan.",
	"it":      "Il tuo account Tellomi sarà eliminato tra 7 giorni. Accedi di nuovo prima di allora per annullare.",
	"ja":      "Tellomi アカウントは 7 日後に削除されます。それまでに再度ログインすると取り消せます。",
	"ko":      "Tellomi 계정은 7일 후 삭제됩니다. 그전에 다시 로그인하면 취소됩니다.",
	"ms":      "Akaun Tellomi anda akan dipadam dalam 7 hari. Log masuk semula sebelum itu untuk membatalkan.",
	"nb":      "Tellomi-kontoen din slettes om 7 dager. Logg inn igjen før det for å avbryte.",
	"nl":      "Je Tellomi-account wordt over 7 dagen verwijderd. Meld je eerder opnieuw aan om dit te annuleren.",
	"pl":      "Twoje konto Tellomi zostanie usunięte za 7 dni. Zaloguj się ponownie wcześniej, aby to anulować.",
	"pt-br":   "Sua conta Tellomi será excluída em 7 dias. Entre de novo antes disso para cancelar.",
	"ro":      "Contul tău Tellomi va fi șters în 7 zile. Conectează-te din nou înainte ca să anulezi.",
	"ru":      "Ваш аккаунт Tellomi будет удалён через 7 дней. Войдите снова до этого срока, чтобы отменить.",
	"sv":      "Ditt Tellomi-konto raderas om 7 dagar. Logga in igen innan dess för att avbryta.",
	"th":      "บัญชี Tellomi ของคุณจะถูกลบใน 7 วัน เข้าสู่ระบบอีกครั้งก่อนหน้านั้นเพื่อยกเลิก",
	"tr":      "Tellomi hesabınız 7 gün sonra silinecek. İptal etmek için o zamandan önce yeniden giriş yapın.",
	"uk":      "Ваш акаунт Tellomi буде видалено за 7 днів. Увійдіть знову до того, щоб скасувати.",
	"uz":      "Tellomi hisobingiz 7 kundan keyin o‘chiriladi. Bekor qilish uchun shu muddatgacha qayta kiring.",
	"vi":      "Tài khoản Tellomi của bạn sẽ bị xóa sau 7 ngày. Đăng nhập lại trước đó để hủy.",
	"zh-hans": "你的 Tellomi 账号将在 7 天后删除。在此之前重新登录即可取消。",
	"zh-hant": "你的 Tellomi 帳號將在 7 天後刪除。在此之前重新登入即可取消。",
}

var emailSubject = map[string]string{
	"en":      "Tellomi verification code",
	"ar":      "رمز تحقق Tellomi",
	"be":      "Код пацверджання Tellomi",
	"ca":      "Codi de verificació de Tellomi",
	"cs":      "Ověřovací kód Tellomi",
	"de":      "Tellomi-Bestätigungscode",
	"el":      "Κωδικός επαλήθευσης Tellomi",
	"es":      "Código de verificación de Tellomi",
	"fa":      "کد تأیید Tellomi",
	"fi":      "Tellomin vahvistuskoodi",
	"fr":      "Code de vérification Tellomi",
	"he":      "קוד האימות של Tellomi",
	"hi":      "Tellomi सत्यापन कोड",
	"id":      "Kode verifikasi Tellomi",
	"it":      "Codice di verifica Tellomi",
	"ja":      "Tellomi 認証コード",
	"ko":      "Tellomi 인증 코드",
	"ms":      "Kod pengesahan Tellomi",
	"nb":      "Tellomi-bekreftelseskode",
	"nl":      "Tellomi-verificatiecode",
	"pl":      "Kod weryfikacyjny Tellomi",
	"pt-br":   "Código de verificação do Tellomi",
	"ro":      "Cod de verificare Tellomi",
	"ru":      "Код подтверждения Tellomi",
	"sv":      "Tellomi-verifieringskod",
	"th":      "รหัสยืนยัน Tellomi",
	"tr":      "Tellomi doğrulama kodu",
	"uk":      "Код підтвердження Tellomi",
	"uz":      "Tellomi tasdiqlash kodi",
	"vi":      "Mã xác minh Tellomi",
	"zh-hans": "Tellomi 验证码",
	"zh-hant": "Tellomi 驗證碼",
}

// emailBody has exactly one %s, the numeric code.
var emailBody = map[string]string{
	"en":      "Your verification code is %s. Do not share it.",
	"ar":      "رمز التحقق الخاص بك هو %s. لا تشاركه مع أحد.",
	"be":      "Ваш код пацверджання: %s. Нікому яго не паведамляйце.",
	"ca":      "El teu codi de verificació és %s. No el comparteixis.",
	"cs":      "Váš ověřovací kód je %s. Nikomu ho nesdělujte.",
	"de":      "Dein Bestätigungscode lautet %s. Gib ihn nicht weiter.",
	"el":      "Ο κωδικός επαλήθευσης είναι %s. Μην τον κοινοποιήσεις.",
	"es":      "Tu código de verificación es %s. No lo compartas.",
	"fa":      "کد تأیید شما %s است. آن را در اختیار دیگران نگذارید.",
	"fi":      "Vahvistuskoodisi on %s. Älä jaa sitä.",
	"fr":      "Votre code de vérification est %s. Ne le partagez pas.",
	"he":      "קוד האימות שלך הוא %s. אל תשתפו אותו.",
	"hi":      "आपका सत्यापन कोड %s है। इसे किसी के साथ साझा न करें।",
	"id":      "Kode verifikasi Anda adalah %s. Jangan bagikan kepada siapa pun.",
	"it":      "Il tuo codice di verifica è %s. Non condividerlo.",
	"ja":      "認証コードは %s です。他人に教えないでください。",
	"ko":      "인증 코드는 %s입니다. 다른 사람에게 알려 주지 마세요.",
	"ms":      "Kod pengesahan anda ialah %s. Jangan kongsikannya.",
	"nb":      "Bekreftelseskoden din er %s. Ikke del den.",
	"nl":      "Je verificatiecode is %s. Deel deze met niemand.",
	"pl":      "Twój kod weryfikacyjny to %s. Nie udostępniaj go nikomu.",
	"pt-br":   "Seu código de verificação é %s. Não o compartilhe.",
	"ro":      "Codul tău de verificare este %s. Nu îl spune nimănui.",
	"ru":      "Ваш код подтверждения: %s. Никому его не сообщайте.",
	"sv":      "Din verifieringskod är %s. Dela den inte.",
	"th":      "รหัสยืนยันของคุณคือ %s อย่าบอกรหัสนี้กับผู้อื่น",
	"tr":      "Doğrulama kodunuz: %s. Kimseyle paylaşmayın.",
	"uk":      "Ваш код підтвердження: %s. Нікому його не повідомляйте.",
	"uz":      "Tasdiqlash kodingiz: %s. Uni hech kimga aytmang.",
	"vi":      "Mã xác minh của bạn là %s. Đừng chia sẻ mã này.",
	"zh-hans": "您的验证码为：%s，请勿泄露于他人！",
	"zh-hant": "您的驗證碼為：%s，請勿洩露於他人！",
}
