package usernamepolicy

type termSpec struct {
	prefix  string
	source  string
	outcome string
	matches []string
	fields  []string
	words   []string
}

func exact() []string { return []string{MatchNormalizedExact} }

func affix() []string {
	return []string{MatchNormalizedExact, MatchPrefix, MatchSuffix}
}

// Overlay terms from requirements v2 appendix C / D-56 that are not rules in
// policy-2026092301.json. The official file is not edited.
//
// Three corpus rows would go red if the overlay were broader, so they are
// not covered here:
//   - must-allow.txt:8525 username "weibo" is an exact allow, so weibo is omitted
//   - must-allow.txt:61 username "telegram_fan" shares a prefix boundary with
//     telegram, so telegram is exact-only
//   - must-allow.txt:18910 username "x____…" would match a username prefix of
//     "x", so x stays slug-only (same as qq and jd, which are under 3 characters)
//
// facebook, twitter, and messenger are already NORMALIZED_EXACT in the
// official file. The overlay adds only their underscore prefix and suffix.

func overlaySpecs() []termSpec {
	user := []string{FieldUsername, FieldSlug, FieldBotUsername}
	return []termSpec{
		{prefix: "overlay/d56", source: "requirements-v2/appendix-c", outcome: OutcomeReserved, matches: exact(), fields: user, words: []string{"telegram"}},
		{prefix: "overlay/d56", source: "requirements-v2/appendix-c", outcome: OutcomeReserved, matches: affix(), fields: user, words: []string{
			"botfather", "wechat", "weixin", "whatsapp", "kakaotalk", "signal", "discord", "viber",
			"tencent", "alipay", "alibaba", "aliyun", "taobao", "tmall", "douyin", "kuaishou", "bytedance", "toutiao",
			"xiaohongshu", "baidu", "bilibili", "zhihu", "meituan", "jingdong", "pinduoduo", "huawei", "xiaomi",
			"google", "gmail", "youtube", "apple", "icloud", "appstore", "microsoft", "meta", "instagram", "threads",
			"tiktok", "amazon", "netflix", "paypal", "snapchat", "reddit",
			"openai", "chatgpt", "grok", "xai", "claude", "anthropic", "gemini", "deepseek", "qwen", "tongyi", "kimi",
			"doubao", "wenxin", "ernie", "line",
		}},
		{prefix: "overlay/d56", source: "requirements-v2/appendix-c", outcome: OutcomeReserved, matches: exact(), fields: []string{FieldSlug}, words: []string{"qq", "jd", "x"}},
		{prefix: "overlay/d56-affix", source: "requirements-v2/appendix-c", outcome: OutcomeReserved, matches: []string{MatchPrefix, MatchSuffix}, fields: user, words: []string{"facebook", "twitter", "messenger"}},
	}
}
