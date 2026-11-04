package report

import (
	"strconv"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/langpack"
)

// Copy is the private-message text. Chinese says 经平台审核 (traditional:
// 經平台審核). No language names a model.
func copyLang(code string) string {
	c, ok := langpack.CanonicalLang(code)
	if !ok {
		return "en"
	}
	switch c {
	case "zh-hans", "zh-hant":
		return c
	default:
		return "en"
	}
}

func etaText(hours int, lang string) string {
	if hours <= 0 {
		hours = 24
	}
	n := strconv.Itoa(hours)
	switch copyLang(lang) {
	case "zh-hans":
		return n + "小时"
	case "zh-hant":
		return n + "小時"
	default:
		if hours == 1 {
			return n + " hour"
		}
		return n + " hours"
	}
}

// JoinTickets renders T1、T2 or T1, T2 in id order.
func JoinTickets(ids []int64, lang string) string {
	cp := append([]int64(nil), ids...)
	for i := 1; i < len(cp); i++ {
		j := i
		for j > 0 && cp[j] < cp[j-1] {
			cp[j], cp[j-1] = cp[j-1], cp[j]
			j--
		}
	}
	parts := make([]string, len(cp))
	for i, id := range cp {
		parts[i] = Ticket(id)
	}
	sep := ", "
	switch copyLang(lang) {
	case "zh-hans", "zh-hant":
		sep = "、"
	}
	return strings.Join(parts, sep)
}

// Receipt is the immediate @admin note to the reporter.
func Receipt(lang, ticket, eta string) string {
	switch copyLang(lang) {
	case "zh-hans":
		return "已收到你的举报。工单号：" + ticket + "。预计处理时间：" + eta + "。经平台审核后会把结果发给你。"
	case "zh-hant":
		return "已收到你的檢舉。工單號：" + ticket + "。預計處理時間：" + eta + "。經平台審核後會把結果發給你。"
	default:
		return "We received your report. Ticket " + ticket + ". Expected time: " + eta + ". You will get the result after a platform review."
	}
}

// DecisionForReporter tells the person who filed the report how it ended.
// tickets is already rendered. A clear result is only for the reporter.
func DecisionForReporter(lang, verdict, tickets string) string {
	switch copyLang(lang) {
	case "zh-hans":
		if verdict == StatusViolation {
			return "工单 " + tickets + " 已处理。经平台审核，确认违规。感谢你的反馈。"
		}
		return "工单 " + tickets + " 已处理。经平台审核，未发现违规。"
	case "zh-hant":
		if verdict == StatusViolation {
			return "工單 " + tickets + " 已處理。經平台審核，確認違規。感謝你的回報。"
		}
		return "工單 " + tickets + " 已處理。經平台審核，未發現違規。"
	default:
		if verdict == StatusViolation {
			return "Ticket " + tickets + " is closed. A platform review found a rule breach. Thank you for the report."
		}
		return "Ticket " + tickets + " is closed. A platform review found no rule breach."
	}
}

// DecisionForSubject tells the reported person about a violation.
// The text has no reporter id, name, or ticket. A clear result returns empty
// because that person is not notified.
func DecisionForSubject(lang, verdict string) string {
	if verdict != StatusViolation {
		return ""
	}
	switch copyLang(lang) {
	case "zh-hans":
		return "经平台审核，你的内容违反平台规则。相关记录已保留。请遵守平台规则。"
	case "zh-hant":
		return "經平台審核，你的內容違反平台規則。相關記錄已保留。請遵守平台規則。"
	default:
		return "A platform review found that your content broke a rule. The record stays. Please follow the rules."
	}
}
