// Package reportcopy is the option tree for stories.report and messages.report.
//
// Clients show the title and option text exactly as the server sends them.
// Option ids stay the official Telegram ids. zh-hans and zh-hant have their
// own text. Every other language, including an empty code, uses English.
package reportcopy

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/langpack"
)

// Kind selects the option list. Stories and messages do not share a title
// or the same set of reasons.
type Kind int

const (
	Message Kind = iota
	Story
)

type phrase struct {
	en   string
	hans string
	hant string
}

func (p phrase) text(lang string) string {
	switch packLang(lang) {
	case "zh-hans":
		return p.hans
	case "zh-hant":
		return p.hant
	default:
		return p.en
	}
}

// packLang keeps zh-hans, zh-hant, and en. CanonicalLang maps zh, zh-CN,
// zh-TW, and en-US onto those packs. Anything else falls back to en.
func packLang(code string) string {
	canon, _ := langpack.CanonicalLang(code)
	switch canon {
	case "zh-hans", "zh-hant", "en":
		return canon
	default:
		return "en"
	}
}

var phrases = map[string]phrase{
	"title.message": {"What is wrong with this message?", "这条消息有什么问题？", "這則訊息有什麼問題？"},
	"title.story":   {"Report story", "举报动态", "舉報動態"},
	"spam":          {"Spam", "垃圾信息", "垃圾訊息"},
	"violence":      {"Violence", "暴力", "暴力"},
	"pornography":   {"Pornography", "色情", "色情"},
	"child":         {"Child abuse", "虐待儿童", "虐待兒童"},
	"drugs":         {"Illegal drugs", "违禁药物", "違禁藥物"},
	"personal":      {"Personal details", "个人信息", "個人資訊"},
	"illegal":       {"Illegal content", "违法内容", "違法內容"},
	"other":         {"Other", "其他", "其他"},
}

var messageIDs = []string{"spam", "violence", "pornography", "child", "drugs", "personal", "other"}
var storyIDs = []string{"spam", "violence", "illegal", "other"}

// Choose is reportResultChooseOption for an empty option.
func Choose(lang string, kind Kind) *mtproto.ReportResult {
	titleKey := "title.message"
	ids := messageIDs
	if kind == Story {
		titleKey = "title.story"
		ids = storyIDs
	}
	opts := make([]*mtproto.MessageReportOption, 0, len(ids))
	for _, id := range ids {
		opts = append(opts, mtproto.MakeTLMessageReportOption(&mtproto.MessageReportOption{
			Text:   phrases[id].text(lang),
			Option: []byte(id),
		}).To_MessageReportOption())
	}
	return mtproto.MakeTLReportResultChooseOption(&mtproto.ReportResult{
		Title:   phrases[titleKey].text(lang),
		Options: opts,
	}).To_ReportResult()
}
