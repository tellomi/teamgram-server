package tellomiai

// Fixed replies. None of these are produced by the model. An env var, when
// set to a non-empty value, replaces the default sentence.
const (
	// EnvMsgNotAllowed overrides the allowlist refusal.
	EnvMsgNotAllowed = "TELLOMI_AI_MSG_NOT_ALLOWED"
	// EnvMsgDaily overrides the per-user daily quota sentence.
	EnvMsgDaily = "TELLOMI_AI_MSG_DAILY"
	// EnvMsgMinute overrides the per-user per-minute sentence.
	EnvMsgMinute = "TELLOMI_AI_MSG_MINUTE"
	// EnvMsgChat overrides the per-chat per-minute sentence.
	EnvMsgChat = "TELLOMI_AI_MSG_CHAT"
	// EnvMsgBusy overrides the model, upstream, and timeout sentence.
	EnvMsgBusy = "TELLOMI_AI_MSG_BUSY"
	// EnvMsgBlocked overrides the upstream moderation sentence.
	EnvMsgBlocked = "TELLOMI_AI_MSG_BLOCKED"
	// EnvMsgTextOnly overrides the empty or non-text input sentence.
	EnvMsgTextOnly = "TELLOMI_AI_MSG_TEXT_ONLY"
	// EnvMsgDisabled overrides the sentence used when TELLOMI_AI is off.
	EnvMsgDisabled = "TELLOMI_AI_MSG_DISABLED"
	// EnvMsgBudget overrides the monthly budget sentence.
	EnvMsgBudget = "TELLOMI_AI_MSG_BUDGET"

	// KindAllow is a user outside the allowlist (or not real-name verified).
	KindAllow = "allow"
	// KindDaily is the per-user daily quota.
	KindDaily = "daily"
	// KindMinute is the per-user per-minute cap.
	KindMinute = "minute"
	// KindChat is the per-chat per-minute cap.
	KindChat = "chat"
	// KindBudget is the monthly spend cap.
	KindBudget = "budget"
	// KindBusy is a model, upstream, timeout, or lookup failure.
	KindBusy = "error"
	// KindBlocked is upstream content moderation.
	KindBlocked = "blocked"
	// KindInput is empty text or media without a caption.
	KindInput = "input"
	// KindDisabled is TELLOMI_AI off.
	KindDisabled = "disabled"

	// NotAllowedMessage is the default allowlist refusal.
	NotAllowedMessage = "AI 助手目前仅对部分用户开放。如需使用 AI，请先完成实名认证。"
	// DailyMessage is shown once the user has used today's 100 questions.
	DailyMessage = "今天的 AI 使用次数已用完，明天再来找我吧～"
	// MinuteMessage is shown on the 6th question in a minute.
	MinuteMessage = "消息有点太快了，请稍等一分钟再试。"
	// ChatMessage is shown when one group exceeds 20 questions in a minute.
	ChatMessage = "本群 AI 使用太频繁啦，请稍后再试。"
	// BusyMessage is shown when the model or a lookup fails or times out.
	BusyMessage = "AI 暂时有点忙，请稍后再试。"
	// Refusal is the fixed sentence when upstream moderation blocks a turn.
	Refusal = "这个问题我暂时无法回答，换个话题试试吧。"
	// TextOnlyMessage is shown for stickers, media without text, and empty input.
	TextOnlyMessage = "目前我只能处理文字消息哦。"
	// DisabledMessage is shown when the assistant is switched off.
	DisabledMessage = "AI 助手暂未开放，敬请期待。"
	// BudgetMessage is shown when the monthly budget is spent.
	BudgetMessage = "本月的提问额度已用完"
)

// Text is the sentence for a non-success kind. It never calls the model.
// An empty or unknown kind uses the daily sentence so a new ledger kind
// still says something a person can read.
func Text(kind string) string {
	env, def := messageDefault(kind)
	if env != "" {
		if v := getenv(env); v != "" {
			return v
		}
	}
	if def == "" {
		return DailyMessage
	}
	return def
}

func messageDefault(kind string) (env, def string) {
	switch kind {
	case KindAllow:
		return EnvMsgNotAllowed, NotAllowedMessage
	case KindDaily:
		return EnvMsgDaily, DailyMessage
	case KindMinute:
		return EnvMsgMinute, MinuteMessage
	case KindChat:
		return EnvMsgChat, ChatMessage
	case KindBudget:
		return EnvMsgBudget, BudgetMessage
	case KindBusy:
		return EnvMsgBusy, BusyMessage
	case KindBlocked:
		return EnvMsgBlocked, Refusal
	case KindInput:
		return EnvMsgTextOnly, TextOnlyMessage
	case KindDisabled:
		return EnvMsgDisabled, DisabledMessage
	default:
		return "", ""
	}
}

func limitText(kind string) string {
	return Text(kind)
}
