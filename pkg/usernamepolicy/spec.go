package usernamepolicy

// Lexicon version embedded in this repo. It is not policy-2026092301:
// that file lives in private tellomi/tellomi and was not readable here.
const lexiconVersion = 2026110301

type termSpec struct {
	prefix  string
	source  string
	outcome string
	matches []string
	fields  []string
	words   []string
}

func usernameFields() []string {
	return []string{FieldUsername, FieldSlug, FieldBotUsername}
}

func displayFields() []string {
	return []string{FieldDisplayName, FieldProfileName, FieldGroupName, FieldGroupDescription, FieldChannelName, FieldBio}
}

func allFields() []string {
	return append(append([]string{}, usernameFields()...), displayFields()...)
}

func exact() []string { return []string{MatchNormalizedExact} }

func affix() []string {
	return []string{MatchNormalizedExact, MatchPrefix, MatchSuffix}
}

func exactPrefix() []string { return []string{MatchNormalizedExact, MatchPrefix} }

func brandConfusable() []string {
	return []string{MatchNormalizedExact, MatchPrefix, MatchSuffix, MatchConfusable}
}

func exactConfusable() []string { return []string{MatchNormalizedExact, MatchConfusable} }

func lexiconSpecs() []termSpec {
	user := usernameFields()
	both := allFields()
	return []termSpec{
		{prefix: "brand", source: "tellomi/brand-protected.toml", outcome: OutcomeBrandProtected, matches: brandConfusable(), fields: both, words: []string{"tellomi"}},
		{prefix: "brand", source: "tellomi/brand-protected.toml", outcome: OutcomeBrandProtected, matches: exactConfusable(), fields: both, words: []string{"tellomiapp"}},
		// tellcc was exact and confusable; appendix C also adds prefix and suffix.
		{prefix: "brand", source: "tellomi/brand-protected.toml", outcome: OutcomeBrandProtected, matches: brandConfusable(), fields: both, words: []string{"tellcc"}},
		{prefix: "brand", source: "tellomi/brand-protected.toml", outcome: OutcomeBrandProtected, matches: exactConfusable(), fields: both, words: []string{
			"tellomi_support", "tellomi_official", "tellomi_team", "tellomi_admin", "tellomi_security", "tellomi_verify",
		}},
		{prefix: "brand", source: "tellomi/brand-protected.toml", outcome: OutcomeBrandProtected, matches: affix(), fields: user, words: []string{"tell_cc"}},
		{prefix: "brand", source: "tellomi/brand-protected.toml", outcome: OutcomeBrandProtected, matches: affix(), fields: user, words: []string{
			"telomi", "tellomy", "tellommi", "tellemi", "tello_mi", "tel_lomi",
		}},
		{prefix: "reserved", source: "tellomi/impersonation.toml", outcome: OutcomeReserved, matches: affix(), fields: both, words: []string{
			"admin", "administrator", "root", "superuser", "sysadmin", "system", "support", "helpdesk", "service",
			"customer_service", "official", "staff", "team", "moderator", "mod", "security", "abuse", "report",
			"trust", "safety", "operator", "webmaster", "postmaster", "hostmaster",
		}},
		{prefix: "impersonation", source: "tellomi/impersonation.toml", outcome: OutcomeImpersonation, matches: affix(), fields: both, words: []string{
			"kefu", "guanliyuan", "guanfang", "xitong", "anquan", "kefuzhongxin", "rengongkefu", "guanfangkefu",
			"xitongguanliyuan", "xitongxiaoxi", "xitongtongzhi", "anquanzhongxin", "zhanghaoanquan", "guanfangtuandui",
			"guanfangzhushou", "jubaozhongxin", "gongzuorenyuan",
		}},
		{prefix: "reserved", source: "tellomi/impersonation.toml", outcome: OutcomeReserved, matches: exact(), fields: user, words: []string{
			"help", "helpcenter", "help_center", "bangzhu", "customer_support", "customersupport", "fankui", "jubao",
		}},
		{prefix: "reserved", source: "tellomi/impersonation.toml", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"news", "xinwen", "gonggao", "announcement", "announcements", "notice", "tongzhi", "guanwang", "official_account",
		}},
		{prefix: "reserved", source: "tellomi/impersonation.toml", outcome: OutcomeReserved, matches: exact(), fields: user, words: []string{
			"shenhe", "yunying", "chanpin",
		}},
		{prefix: "reserved", source: "tellomi/impersonation.toml", outcome: OutcomeReserved, matches: exactPrefix(), fields: user, words: []string{
			"renzheng", "verified", "verify",
		}},
		{prefix: "security", source: "tellomi/security-sensitive.toml", outcome: OutcomeSecuritySensitive, matches: exactPrefix(), fields: both, words: []string{
			"payment", "wallet", "billing", "refund", "tuikuan", "yanzhengma", "password", "otp", "2fa",
			"giveaway", "airdrop", "lottery", "prize",
		}},
		{prefix: "security", source: "tellomi/security-sensitive.toml", outcome: OutcomeSecuritySensitive, matches: affix(), fields: user, words: []string{
			"pay", "payments", "hongbao", "redpacket", "red_packet", "choujiang", "zhongjiang", "cashback", "fanli",
		}},
		{prefix: "security", source: "tellomi/security-sensitive.toml", outcome: OutcomeSecuritySensitive, matches: exact(), fields: user, words: []string{"zhifu"}},
		{prefix: "reserved", source: "tellomi/reserved-system.toml", outcome: OutcomeReserved, matches: exact(), fields: []string{FieldUsername, FieldSlug}, words: []string{
			"feed", "bot", "bots", "premium", "group", "groups", "games", "api", "dev", "developer", "test", "beta",
			"staging", "null", "login", "signup", "register", "settings", "config", "chat", "call", "profile", "app",
			"apps", "download", "store", "shop", "info", "contact", "feedback", "privacy", "terms", "legal", "media",
			"press", "notifications", "noreply", "subscribe", "account", "user", "username",
		}},
		{prefix: "reserved", source: "tellomi/reserved-system.toml", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"faxian", "discover", "explore", "dongtai", "moments", "tongcheng", "nearby", "tuijian", "guanzhu", "huati",
			"stories", "story", "calls", "chats", "contacts", "lianxiren", "tonghua", "liaotian", "shezhi", "channel",
			"channels", "pindao", "zhibo", "live", "sticker", "stickers", "emoji", "miniapp", "webapp", "vip", "svip",
			"huiyuan", "dingyue", "subscription", "stars",
		}},
		{prefix: "reserved", source: "tellomi/reserved-system.toml", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"ai_bot", "aibot", "assistant", "zhushou", "jiqiren", "robot", "tellomi_ai", "tellomiai", "tellomi_bot", "tellomibot",
		}},
		{prefix: "reserved", source: "tellomi/reserved-system.toml", outcome: OutcomeReserved, matches: exact(), fields: user, words: []string{
			"all", "everyone", "here", "me", "self", "undefined",
		}},
		{prefix: "reserved", source: "tellomi/reserved-system.toml", outcome: OutcomeReserved, matches: exact(), fields: []string{FieldSlug}, words: []string{
			"u", "g", "s", "i", "m", "qq", "x", "jd", "well_known",
		}},
		{prefix: "reserved", source: "tellomi/reserved-system.toml", outcome: OutcomeReserved, matches: exact(), fields: []string{FieldUsername, FieldSlug}, words: []string{
			"www", "web", "mail", "email", "cdn", "static", "img", "image", "images", "file", "files", "upload",
			"updates", "assets", "uploads", "status",
		}},
		{prefix: "third", source: "tellomi/third-party.toml", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"telegram", "botfather", "wechat", "weixin", "whatsapp", "messenger", "line", "kakaotalk", "signal", "discord", "viber",
		}},
		{prefix: "third", source: "tellomi/third-party.toml", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"tencent", "alipay", "alibaba", "aliyun", "taobao", "tmall", "douyin", "kuaishou", "bytedance", "toutiao",
			"weibo", "xiaohongshu", "baidu", "bilibili", "zhihu", "meituan", "jingdong", "pinduoduo", "huawei", "xiaomi",
		}},
		{prefix: "third", source: "tellomi/third-party.toml", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"google", "gmail", "youtube", "apple", "icloud", "appstore", "microsoft", "facebook", "meta", "instagram",
			"threads", "tiktok", "twitter", "amazon", "netflix", "paypal", "snapchat", "reddit",
		}},
		{prefix: "third", source: "tellomi/third-party.toml", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"openai", "chatgpt", "grok", "xai", "claude", "anthropic", "gemini", "deepseek", "qwen", "tongyi", "kimi",
			"doubao", "wenxin", "ernie",
		}},
	}
}

func buildRules() ([]rule, error) {
	var rules []rule
	seen := map[string]struct{}{}
	for _, spec := range lexiconSpecs() {
		for _, word := range spec.words {
			id := spec.prefix + ":" + word
			if _, ok := seen[id]; ok {
				return nil, errDuplicate(id)
			}
			seen[id] = struct{}{}
			normalized := Normalize(word)
			skeleton := ""
			if containsStr(spec.matches, MatchConfusable) {
				skeleton = Skeleton(normalized)
				if skeleton == "" {
					return nil, errDuplicate(id + " empty skeleton")
				}
			}
			rules = append(rules, rule{
				ID:         id,
				Term:       word,
				Normalized: normalized,
				Skeleton:   skeleton,
				Matches:    append([]string{}, spec.matches...),
				Fields:     append([]string{}, spec.fields...),
				Regions:    []string{"global"},
				Outcome:    spec.outcome,
				Source:     spec.source,
			})
		}
	}
	return rules, nil
}

type dupError string

func (e dupError) Error() string { return string(e) }

func errDuplicate(id string) error { return dupError("usernamepolicy: spec " + id) }
