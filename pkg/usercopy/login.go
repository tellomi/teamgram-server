package usercopy

import (
	"fmt"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// shanghai is the clock shown in the new-login notice. China has no DST.
var shanghai = time.FixedZone("CST", 8*3600)

// codeLine is the 777000 login-code message.
// label is bold, the digits are a spoiler, and warn is bold.
// rest contains the app name twice, as %s.
type codeLine struct {
	label string
	gap   string
	warn  string
	rest  string
}

// alertLine is the 777000 new-login notice.
// body contains the Beijing stamp and the device, as %s %s.
type alertLine struct {
	title   string
	unknown string
	body    string
}

// LoginCode is the in-app message from user 777000 that carries a sign-in code.
// The digits are a spoiler. An unknown language uses English.
func LoginCode(lang, code, appName string) (string, []*mtproto.MessageEntity) {
	line := codeLines[resolve(lang)]
	if line.label == "" {
		line = codeLines["en"]
	}
	rest := fmt.Sprintf(line.rest, appName, appName)
	return mtproto.MakeTextAndMessageEntities([]mtproto.MessageBuildEntry{
		{Param: line.label, EntityType: mtproto.Predicate_messageEntityBold},
		{Param: code, EntityType: mtproto.Predicate_messageEntitySpoiler},
		{Text: line.gap, Param: line.warn, EntityType: mtproto.Predicate_messageEntityBold},
		{Text: rest},
	})
}

// LoginAlert is the in-app message from user 777000 after a finished login.
// It names the device and the Beijing minute. It does not carry a code.
func LoginAlert(lang, device string, when time.Time) (string, []*mtproto.MessageEntity) {
	line := alertLines[resolve(lang)]
	if line.title == "" {
		line = alertLines["en"]
	}
	device = strings.TrimSpace(device)
	if device == "" {
		device = line.unknown
	}
	stamp := when.In(shanghai).Format("2006-01-02 15:04")
	body := fmt.Sprintf(line.body, stamp, device)
	return mtproto.MakeTextAndMessageEntities([]mtproto.MessageBuildEntry{
		{Param: line.title, EntityType: mtproto.Predicate_messageEntityBold},
		{Text: body},
	})
}

var codeLines = map[string]codeLine{
	"en": {
		label: "Login code: ",
		gap:   ". Do ",
		warn:  "not",
		rest:  " give this code to anyone, even if they say they are from %s!\n\nThis code can sign in to your %s account. We will never ask for it for any other reason.\n\nIf you did not try to sign in on another device, ignore this message.",
	},
	"ar": {
		label: "رمز تسجيل الدخول: ",
		gap:   ". لا ",
		warn:  "تشارك",
		rest:  " هذا الرمز مع أي شخص، حتى إذا قال إنه من %s!\n\nيمكن لهذا الرمز تسجيل الدخول إلى حسابك في %s. لن نطلبه لأي سبب آخر.\n\nإذا لم تحاول تسجيل الدخول على جهاز آخر، فتجاهل هذه الرسالة.",
	},
	"be": {
		label: "Код уваходу: ",
		gap:   ". Нікому яго ",
		warn:  "не",
		rest:  " паведамляйце, нават калі кажуць, што яны з %s!\n\nГэты код дае ўваход у ваш акаўнт %s. Мы ніколі не папросім яго з іншай прычыны.\n\nКалі вы не спрабавалі ўвайсці на іншай прыладзе, праігнаруйце гэтае паведамленне.",
	},
	"ca": {
		label: "Codi d'inici de sessió: ",
		gap:   ". ",
		warn:  "No",
		rest:  " el comparteixis amb ningú, ni tan sols si diuen que són de %s!\n\nAquest codi permet iniciar la sessió al teu compte de %s. No el demanarem per cap altre motiu.\n\nSi no has provat d'iniciar la sessió en un altre dispositiu, ignora aquest missatge.",
	},
	"cs": {
		label: "Přihlašovací kód: ",
		gap:   ". ",
		warn:  "Nikomu",
		rest:  " ho nesdělujte, ani když tvrdí, že jsou z %s!\n\nTímto kódem se lze přihlásit k vašemu účtu %s. Z jiného důvodu o něj nikdy nepožádáme.\n\nPokud jste se nepokoušeli přihlásit na jiném zařízení, tuto zprávu ignorujte.",
	},
	"de": {
		label: "Anmeldecode: ",
		gap:   ". Gib ihn ",
		warn:  "nicht",
		rest:  " weiter, auch wenn jemand behauptet, von %s zu sein!\n\nMit diesem Code kann man sich in deinem %s-Konto anmelden. Wir fragen aus keinem anderen Grund danach.\n\nWenn du dich nicht auf einem anderen Gerät anmelden wolltest, ignoriere diese Nachricht.",
	},
	"el": {
		label: "Κωδικός σύνδεσης: ",
		gap:   ". Μην τον ",
		warn:  "μοιραστείς",
		rest:  " με κανέναν, ακόμα κι αν λέει ότι είναι από το %s!\n\nΑυτός ο κωδικός συνδέεται στον λογαριασμό σου στο %s. Δεν θα τον ζητήσουμε για κανέναν άλλο λόγο.\n\nΑν δεν προσπάθησες να συνδεθείς σε άλλη συσκευή, αγνόησε αυτό το μήνυμα.",
	},
	"es": {
		label: "Código de inicio de sesión: ",
		gap:   ". ",
		warn:  "No",
		rest:  " se lo des a nadie, aunque diga que es de %s.\n\nEste código permite entrar en tu cuenta de %s. No lo pediremos por ningún otro motivo.\n\nSi no intentaste iniciar sesión en otro dispositivo, ignora este mensaje.",
	},
	"fa": {
		label: "کد ورود: ",
		gap:   ". آن را با ",
		warn:  "هیچ‌کس",
		rest:  " به اشتراک نگذارید، حتی اگر بگوید از %s است!\n\nبا این کد می‌توان وارد حساب %s شما شد. ما هرگز به دلیل دیگری آن را نمی‌خواهیم.\n\nاگر تلاشی برای ورود در دستگاه دیگری نداشته‌اید، این پیام را نادیده بگیرید.",
	},
	"fi": {
		label: "Kirjautumiskoodi: ",
		gap:   ". Älä ",
		warn:  "jaa",
		rest:  " sitä kenellekään, vaikka hän väittäisi olevansa %sista!\n\nTällä koodilla voi kirjautua %s-tilillesi. Emme pyydä sitä mistään muusta syystä.\n\nJos et yrittänyt kirjautua toisella laitteella, jätä tämä viesti huomiotta.",
	},
	"fr": {
		label: "Code de connexion : ",
		gap:   ". Ne le ",
		warn:  "donnez",
		rest:  " à personne, même si l'on prétend venir de %s !\n\nCe code permet de se connecter à votre compte %s. Nous ne le demanderons pour aucune autre raison.\n\nSi vous n'avez pas essayé de vous connecter sur un autre appareil, ignorez ce message.",
	},
	"he": {
		label: "קוד כניסה: ",
		gap:   ". אל ",
		warn:  "תשתפו",
		rest:  " אותו עם אף אחד, גם אם אומרים שהם מ-%s!\n\nהקוד הזה מאפשר כניסה לחשבון %s שלכם. לא נבקש אותו משום סיבה אחרת.\n\nאם לא ניסיתם להיכנס במכשיר אחר, התעלמו מההודעה הזאת.",
	},
	"hi": {
		label: "लॉगिन कोड: ",
		gap:   ". इसे किसी को ",
		warn:  "न",
		rest:  " बताएं, भले ही वे कहें कि वे %s से हैं!\n\nइस कोड से आपके %s खाते में साइन इन हो सकता है। हम इसे किसी और वजह से कभी नहीं माँगेंगे।\n\nअगर आपने किसी दूसरे डिवाइस पर साइन इन करने की कोशिश नहीं की, तो इस संदेश को नज़रअंदाज़ करें।",
	},
	"id": {
		label: "Kode masuk: ",
		gap:   ". ",
		warn:  "Jangan",
		rest:  " berikan kepada siapa pun, meskipun mereka mengaku dari %s!\n\nKode ini dapat masuk ke akun %s Anda. Kami tidak akan memintanya untuk alasan lain.\n\nJika Anda tidak mencoba masuk di perangkat lain, abaikan pesan ini.",
	},
	"it": {
		label: "Codice di accesso: ",
		gap:   ". ",
		warn:  "Non",
		rest:  " darlo a nessuno, anche se dice di essere di %s!\n\nQuesto codice può accedere al tuo account %s. Non lo chiederemo per nessun altro motivo.\n\nSe non hai provato ad accedere da un altro dispositivo, ignora questo messaggio.",
	},
	"ja": {
		label: "ログインコード: ",
		gap:   "。誰にも",
		warn:  "教えないで",
		rest:  "ください。%s の者だと言っても同じです。\n\nこのコードで %s アカウントにログインできます。ほかの理由でこのコードを求めることはありません。\n\n別の端末からログインしようとしていない場合は、このメッセージを無視してください。",
	},
	"ko": {
		label: "로그인 코드: ",
		gap:   ". 누구에게도 ",
		warn:  "알려 주지 마세요",
		rest:  ". %s에서 왔다고 해도 마찬가지입니다!\n\n이 코드로 %s 계정에 로그인할 수 있습니다. 다른 이유로 이 코드를 요구하지 않습니다.\n\n다른 기기에서 로그인하려 한 적이 없다면 이 메시지를 무시하세요.",
	},
	"ms": {
		label: "Kod log masuk: ",
		gap:   ". ",
		warn:  "Jangan",
		rest:  " beritahu sesiapa, walaupun mereka mengatakan mereka dari %s!\n\nKod ini boleh log masuk ke akaun %s anda. Kami tidak akan memintanya atas sebab lain.\n\nJika anda tidak cuba log masuk pada peranti lain, abaikan mesej ini.",
	},
	"nb": {
		label: "Innloggingskode: ",
		gap:   ". Ikke ",
		warn:  "del",
		rest:  " den med noen, selv om de sier at de er fra %s!\n\nDenne koden kan logge inn på %s-kontoen din. Vi ber aldri om den av en annen grunn.\n\nHvis du ikke prøvde å logge inn på en annen enhet, kan du se bort fra denne meldingen.",
	},
	"nl": {
		label: "Inlogcode: ",
		gap:   ". Geef hem aan ",
		warn:  "niemand",
		rest:  ", ook niet als iemand zegt van %s te zijn!\n\nMet deze code kan iemand inloggen op je %s-account. We vragen er nooit om voor een andere reden.\n\nAls je niet probeerde in te loggen op een ander apparaat, negeer dit bericht.",
	},
	"pl": {
		label: "Kod logowania: ",
		gap:   ". ",
		warn:  "Nie",
		rest:  " podawaj go nikomu, nawet jeśli twierdzi, że jest z %s!\n\nTen kod pozwala zalogować się na Twoje konto %s. Nie poprosimy o niego z żadnego innego powodu.\n\nJeśli nie próbowałeś logować się na innym urządzeniu, zignoruj tę wiadomość.",
	},
	"pt-br": {
		label: "Código de login: ",
		gap:   ". ",
		warn:  "Não",
		rest:  " o conte a ninguém, mesmo que digam ser do %s!\n\nEste código entra na sua conta %s. Nunca vamos pedi-lo por outro motivo.\n\nSe você não tentou entrar em outro aparelho, ignore esta mensagem.",
	},
	"ro": {
		label: "Cod de conectare: ",
		gap:   ". Nu îl ",
		warn:  "spune",
		rest:  " nimănui, chiar dacă zice că este de la %s!\n\nAcest cod poate intra în contul tău %s. Nu îl vom cere pentru niciun alt motiv.\n\nDacă nu ai încercat să te conectezi pe alt dispozitiv, ignoră acest mesaj.",
	},
	"ru": {
		label: "Код входа: ",
		gap:   ". Никому его ",
		warn:  "не",
		rest:  " сообщайте, даже если говорят, что они из %s!\n\nЭтим кодом можно войти в ваш аккаунт %s. Мы никогда не попросим его по другой причине.\n\nЕсли вы не пытались войти на другом устройстве, проигнорируйте это сообщение.",
	},
	"sv": {
		label: "Inloggningskod: ",
		gap:   ". ",
		warn:  "Dela",
		rest:  " den inte med någon, även om de säger att de kommer från %s!\n\nDen här koden kan logga in på ditt %s-konto. Vi ber aldrig om den av något annat skäl.\n\nOm du inte försökte logga in på en annan enhet kan du ignorera det här meddelandet.",
	},
	"th": {
		label: "รหัสเข้าสู่ระบบ: ",
		gap:   " อย่า",
		warn:  "บอก",
		rest:  "ใคร แม้เขาจะอ้างว่ามาจาก %s\n\nรหัสนี้ใช้เข้าบัญชี %s ของคุณได้ เราจะไม่ขอรหัสนี้ด้วยเหตุผลอื่น\n\nถ้าคุณไม่ได้พยายามเข้าสู่ระบบบนอุปกรณ์อื่น ให้ข้ามข้อความนี้",
	},
	"tr": {
		label: "Giriş kodu: ",
		gap:   ". Kimseyle ",
		warn:  "paylaşmayın",
		rest:  ", %s'den olduğunu söyleseler bile!\n\nBu kodla %s hesabınıza girilebilir. Başka bir nedenle bu kodu istemeyiz.\n\nBaşka bir cihazda oturum açmaya çalışmadıysanız bu mesajı yok sayın.",
	},
	"uk": {
		label: "Код входу: ",
		gap:   ". Нікому його ",
		warn:  "не",
		rest:  " повідомляйте, навіть якщо кажуть, що вони з %s!\n\nЦим кодом можна увійти у ваш акаунт %s. Ми ніколи не попросимо його з іншої причини.\n\nЯкщо ви не намагалися увійти на іншому пристрої, проігноруйте це повідомлення.",
	},
	"uz": {
		label: "Kirish kodi: ",
		gap:   ". Uni ",
		warn:  "hech kimga",
		rest:  " aytmang, hatto o‘zini %s xodimi deb aytsa ham!\n\nBu kod bilan %s hisobingizga kirish mumkin. Boshqa sabab bilan bu kodni so‘ramaymiz.\n\nAgar boshqa qurilmada kirishga urinib ko‘rmagan bo‘lsangiz, bu xabarni e’tiborsiz qoldiring.",
	},
	"vi": {
		label: "Mã đăng nhập: ",
		gap:   ". ",
		warn:  "Đừng",
		rest:  " đưa cho bất kỳ ai, kể cả khi họ nói mình từ %s!\n\nMã này có thể đăng nhập vào tài khoản %s của bạn. Chúng tôi không bao giờ hỏi mã vì lý do khác.\n\nNếu bạn không thử đăng nhập trên thiết bị khác, hãy bỏ qua tin nhắn này.",
	},
	"zh-hans": {
		label: "登录验证码：",
		gap:   "。请",
		warn:  "不要",
		rest:  "把此验证码告诉任何人，即使对方声称来自 %s！\n\n此验证码可用于登录你的 %s 账号。我们绝不会以其他理由向你索要它。\n\n如果你并未尝试在其他设备上登录，请忽略此消息。",
	},
	"zh-hant": {
		label: "登入驗證碼：",
		gap:   "。請",
		warn:  "不要",
		rest:  "把此驗證碼告訴任何人，即使對方聲稱來自 %s！\n\n此驗證碼可用於登入你的 %s 帳號。我們絕不會以其他理由向你索取它。\n\n如果你並未嘗試在其他裝置上登入，請忽略此訊息。",
	},
}

var alertLines = map[string]alertLine{
	"en": {
		title:   "New login alert",
		unknown: "Unknown device",
		body:    "\nYour account signed in at %s (Beijing time) on %s.\nIf this was you, you can ignore this message. If it was not, open Settings > Devices and end that session.",
	},
	"ar": {
		title:   "تنبيه بتسجيل دخول جديد",
		unknown: "جهاز غير معروف",
		body:    "\nتم تسجيل الدخول إلى حسابك في %s (بتوقيت بكين) على %s.\nإذا كنت أنت، يمكنك تجاهل هذه الرسالة. وإلا فافتح الإعدادات > الأجهزة وأنهِ تلك الجلسة.",
	},
	"be": {
		title:   "Новы ўваход",
		unknown: "Невядомая прылада",
		body:    "\nВаш акаўнт увайшоў у %s (па пекінскім часе) на %s.\nКалі гэта былі вы, можна праігнараваць паведамленне. Калі не, адкрыйце Налады > Прылады і завяршыце сеанс.",
	},
	"ca": {
		title:   "Avís d'inici de sessió nou",
		unknown: "Dispositiu desconegut",
		body:    "\nEl teu compte ha iniciat la sessió a les %s (hora de Pequín) a %s.\nSi has estat tu, pots ignorar aquest missatge. Si no, obre Configuració > Dispositius i tanca aquella sessió.",
	},
	"cs": {
		title:   "Upozornění na nové přihlášení",
		unknown: "Neznámé zařízení",
		body:    "\nVáš účet se přihlásil v %s (pekingský čas) na zařízení %s.\nPokud jste to byli vy, zprávu ignorujte. Pokud ne, otevřete Nastavení > Zařízení a ukončete tuto relaci.",
	},
	"de": {
		title:   "Hinweis zu einer neuen Anmeldung",
		unknown: "Unbekanntes Gerät",
		body:    "\nDein Konto wurde um %s (Pekinger Zeit) auf %s angemeldet.\nWenn du das warst, kannst du diese Nachricht ignorieren. Sonst öffne Einstellungen > Geräte und beende diese Sitzung.",
	},
	"el": {
		title:   "Ειδοποίηση νέας σύνδεσης",
		unknown: "Άγνωστη συσκευή",
		body:    "\nΟ λογαριασμός σου συνδέθηκε στις %s (ώρα Πεκίνου) στη συσκευή %s.\nΑν ήσουν εσύ, αγνόησε αυτό το μήνυμα. Αλλιώς άνοιξε Ρυθμίσεις > Συσκευές και τερμάτισε εκείνη τη συνεδρία.",
	},
	"es": {
		title:   "Aviso de nuevo inicio de sesión",
		unknown: "Dispositivo desconocido",
		body:    "\nTu cuenta inició sesión a las %s (hora de Pekín) en %s.\nSi fuiste tú, puedes ignorar este mensaje. Si no, abre Ajustes > Dispositivos y termina esa sesión.",
	},
	"fa": {
		title:   "هشدار ورود جدید",
		unknown: "دستگاه ناشناس",
		body:    "\nحساب شما در %s (به وقت پکن) روی %s وارد شد.\nاگر خودتان بودید، این پیام را نادیده بگیرید. اگر نه، تنظیمات > دستگاه‌ها را باز کنید و آن نشست را پایان دهید.",
	},
	"fi": {
		title:   "Ilmoitus uudesta kirjautumisesta",
		unknown: "Tuntematon laite",
		body:    "\nTilillesi kirjauduttiin %s (Pekingin aikaa) laitteella %s.\nJos se olit sinä, voit ohittaa tämän viestin. Jos et, avaa Asetukset > Laitteet ja lopeta istunto.",
	},
	"fr": {
		title:   "Alerte de nouvelle connexion",
		unknown: "Appareil inconnu",
		body:    "\nVotre compte s'est connecté à %s (heure de Pékin) sur %s.\nSi c'était vous, ignorez ce message. Sinon, ouvrez Paramètres > Appareils et terminez cette session.",
	},
	"he": {
		title:   "התראה על כניסה חדשה",
		unknown: "מכשיר לא ידוע",
		body:    "\nהחשבון שלך נכנס בשעה %s (שעון בייג'ינג) במכשיר %s.\nאם זה הייתם אתם, אפשר להתעלם. אם לא, פתחו הגדרות > מכשירים וסיימו את אותה הפעלה.",
	},
	"hi": {
		title:   "नए लॉगिन की सूचना",
		unknown: "अज्ञात डिवाइस",
		body:    "\nआपके खाते में %s (बीजिंग समय) पर %s से साइन इन हुआ।\nअगर यह आप थे, तो इस संदेश को छोड़ दें। अगर नहीं, तो सेटिंग > डिवाइस खोलें और वह सत्र समाप्त करें।",
	},
	"id": {
		title:   "Peringatan masuk baru",
		unknown: "Perangkat tidak dikenal",
		body:    "\nAkun Anda masuk pada %s (waktu Beijing) di %s.\nJika itu Anda, abaikan pesan ini. Jika bukan, buka Setelan > Perangkat dan akhiri sesi itu.",
	},
	"it": {
		title:   "Avviso di nuovo accesso",
		unknown: "Dispositivo sconosciuto",
		body:    "\nIl tuo account ha effettuato l'accesso alle %s (ora di Pechino) su %s.\nSe sei stato tu, puoi ignorare questo messaggio. Altrimenti apri Impostazioni > Dispositivi e termina quella sessione.",
	},
	"ja": {
		title:   "新しいログインの通知",
		unknown: "不明な端末",
		body:    "\nあなたのアカウントが %s（北京時間）に %s でログインしました。\nご本人の操作なら、このメッセージは無視してかまいません。心当たりがなければ、設定 > デバイス からそのセッションを終了してください。",
	},
	"ko": {
		title:   "새 로그인 알림",
		unknown: "알 수 없는 기기",
		body:    "\n계정이 %s(베이징 시간)에 %s에서 로그인했습니다.\n본인이라면 이 메시지를 무시해도 됩니다. 아니라면 설정 > 기기에서 해당 세션을 종료하세요.",
	},
	"ms": {
		title:   "Amaran log masuk baharu",
		unknown: "Peranti tidak diketahui",
		body:    "\nAkaun anda log masuk pada %s (waktu Beijing) di %s.\nJika itu anda, abaikan mesej ini. Jika bukan, buka Tetapan > Peranti dan tamatkan sesi itu.",
	},
	"nb": {
		title:   "Varsel om ny innlogging",
		unknown: "Ukjent enhet",
		body:    "\nKontoen din ble logget inn %s (Beijing-tid) på %s.\nHvis det var deg, kan du se bort fra meldingen. Hvis ikke, åpne Innstillinger > Enheter og avslutt økten.",
	},
	"nl": {
		title:   "Melding over een nieuwe login",
		unknown: "Onbekend apparaat",
		body:    "\nJe account is ingelogd om %s (Peking-tijd) op %s.\nWas jij dat, dan kun je dit bericht negeren. Zo niet, open Instellingen > Apparaten en beëindig die sessie.",
	},
	"pl": {
		title:   "Alert o nowym logowaniu",
		unknown: "Nieznane urządzenie",
		body:    "\nTwoje konto zalogowało się o %s (czasu pekińskiego) na %s.\nJeśli to byłeś ty, zignoruj tę wiadomość. Jeśli nie, otwórz Ustawienia > Urządzenia i zakończ tę sesję.",
	},
	"pt-br": {
		title:   "Alerta de novo login",
		unknown: "Dispositivo desconhecido",
		body:    "\nSua conta entrou às %s (horário de Pequim) em %s.\nSe foi você, ignore esta mensagem. Se não foi, abra Configurações > Dispositivos e encerre essa sessão.",
	},
	"ro": {
		title:   "Alertă de conectare nouă",
		unknown: "Dispozitiv necunoscut",
		body:    "\nContul tău s-a conectat la %s (ora Beijing) pe %s.\nDacă ai fost tu, poți ignora mesajul. Dacă nu, deschide Setări > Dispozitive și încheie acea sesiune.",
	},
	"ru": {
		title:   "Новый вход в аккаунт",
		unknown: "Неизвестное устройство",
		body:    "\nВаш аккаунт вошёл в %s (по пекинскому времени) на %s.\nЕсли это были вы, сообщение можно проигнорировать. Если нет, откройте Настройки > Устройства и завершите этот сеанс.",
	},
	"sv": {
		title:   "Varning om ny inloggning",
		unknown: "Okänd enhet",
		body:    "\nDitt konto loggade in %s (Peking-tid) på %s.\nOm det var du kan du ignorera meddelandet. Annars öppnar du Inställningar > Enheter och avslutar sessionen.",
	},
	"th": {
		title:   "แจ้งเตือนการเข้าสู่ระบบใหม่",
		unknown: "อุปกรณ์ที่ไม่รู้จัก",
		body:    "\nบัญชีของคุณเข้าสู่ระบบเมื่อ %s (เวลาปักกิ่ง) บน %s\nถ้าเป็นคุณเอง ไม่ต้องสนใจข้อความนี้ ถ้าไม่ใช่ ให้เปิด การตั้งค่า > อุปกรณ์ แล้วจบเซสชันนั้น",
	},
	"tr": {
		title:   "Yeni oturum uyarısı",
		unknown: "Bilinmeyen cihaz",
		body:    "\nHesabınız %s (Pekin saati) itibarıyla %s üzerinde oturum açtı.\nSiz yaptıysanız bu mesajı yok sayabilirsiniz. Değilse Ayarlar > Cihazlar bölümünden o oturumu sonlandırın.",
	},
	"uk": {
		title:   "Новий вхід в акаунт",
		unknown: "Невідомий пристрій",
		body:    "\nВаш акаунт увійшов о %s (за пекінським часом) на %s.\nЯкщо це були ви, повідомлення можна проігнорувати. Якщо ні, відкрийте Налаштування > Пристрої і завершіть цей сеанс.",
	},
	"uz": {
		title:   "Yangi kirish haqida ogohlantirish",
		unknown: "Noma’lum qurilma",
		body:    "\nHisobingiz %s (Pekin vaqti) da %s qurilmasida ochildi.\nAgar bu o‘zingiz bo‘lsangiz, xabarni e’tiborsiz qoldiring. Aks holda Sozlamalar > Qurilmalar bo‘limida o‘sha seansni tugating.",
	},
	"vi": {
		title:   "Cảnh báo đăng nhập mới",
		unknown: "Thiết bị không xác định",
		body:    "\nTài khoản của bạn đã đăng nhập lúc %s (giờ Bắc Kinh) trên %s.\nNếu là bạn, hãy bỏ qua tin nhắn này. Nếu không, mở Cài đặt > Thiết bị và kết thúc phiên đó.",
	},
	"zh-hans": {
		title:   "新设备登录提醒",
		unknown: "未知设备",
		body:    "\n你的账号于 %s（北京时间）在 %s 上登录。\n如果这是你本人的操作，可以忽略这条消息。如果不是，请到设置 > 设备中结束该会话。",
	},
	"zh-hant": {
		title:   "新裝置登入提醒",
		unknown: "未知裝置",
		body:    "\n你的帳號於 %s（北京時間）在 %s 上登入。\n如果這是你本人的操作，可以忽略這則訊息。如果不是，請到設定 > 裝置中結束該工作階段。",
	},
}
