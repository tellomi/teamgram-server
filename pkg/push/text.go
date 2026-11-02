package push

import "strings"

// NormalizeLang maps an initConnection / registerDevice lang_code onto the
// languages this server can render as a plain notification body.
// Anything that is not Chinese stays English. zh, zh-CN and zh-Hans are
// simplified. zh-Hant, zh-TW and zh-HK are traditional.
func NormalizeLang(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	c = strings.ReplaceAll(c, "_", "-")
	switch {
	case c == "zh-tw" || c == "zh-hk" || c == "zh-mo" || strings.HasPrefix(c, "zh-hant"):
		return "zh-hant"
	case c == "zh" || c == "zh-cn" || c == "zh-sg" || strings.HasPrefix(c, "zh-hans") || strings.HasPrefix(c, "zh"):
		return "zh-hans"
	default:
		return "en"
	}
}

// baseKey strips the CHAT_ / CHANNEL_ peer prefix. PINNED_* and REACT_* stay
// as they are. CHAT_REACT_TEXT becomes REACT_TEXT.
func baseKey(loc string) string {
	switch {
	case strings.HasPrefix(loc, "CHANNEL_"):
		return strings.TrimPrefix(loc, "CHANNEL_")
	case strings.HasPrefix(loc, "CHAT_"):
		return strings.TrimPrefix(loc, "CHAT_")
	default:
		return loc
	}
}

// phraseFor is the plain alert body for locKey in lang. The sender name and
// chat title are the notification title / subtitle, so the body is only the
// action ("发来一条语音消息"), matching the English "sent you a voice message".
func phraseFor(lang, locKey string) string {
	key := baseKey(locKey)
	switch NormalizeLang(lang) {
	case "zh-hans":
		if s, ok := zhHansPhrases[key]; ok {
			return s
		}
		return "发来一条消息"
	case "zh-hant":
		if s, ok := zhHantPhrases[key]; ok {
			return s
		}
		return "發來一條訊息"
	default:
		if s, ok := enPhrases[key]; ok {
			return s
		}
		return "sent you a message"
	}
}

// enPhrases are the bodies previously hard-coded in previewBody.
var enPhrases = map[string]string{
	"MESSAGE_NOTEXT":       "sent you a message",
	"MESSAGE_PHOTO":        "sent you a photo",
	"MESSAGE_PHOTO_SECRET": "sent you a self-destructing photo",
	"MESSAGE_VIDEO":        "sent you a video",
	"MESSAGE_VIDEO_SECRET": "sent you a self-destructing video",
	"MESSAGE_GIF":          "sent you a GIF",
	"MESSAGE_ROUND":        "sent you a video message",
	"MESSAGE_AUDIO":        "sent you a voice message",
	"MESSAGE_STICKER":      "sent you a sticker",
	"MESSAGE_DOC":          "sent you a file",
	"MESSAGE_GEO":          "sent you a map",
	"MESSAGE_GEOLIVE":      "started sharing their live location",
	"MESSAGE_CONTACT":      "shared a contact",
	"MESSAGE_POLL":         "sent you a poll",
	"MESSAGE_QUIZ":         "sent you a quiz",
	"PINNED_NOTEXT":        "pinned a message",
	"PINNED_TEXT":          "pinned a message",
	"PINNED_PHOTO":         "pinned a photo",
	"PINNED_VIDEO":         "pinned a video",
	"PINNED_GIF":           "pinned a GIF",
	"PINNED_ROUND":         "pinned a video message",
	"PINNED_AUDIO":         "pinned a voice message",
	"PINNED_STICKER":       "pinned a sticker",
	"PINNED_DOC":           "pinned a file",
	"PINNED_GEO":           "pinned a map",
	"PINNED_GEOLIVE":       "pinned a live location",
	"PINNED_CONTACT":       "pinned a contact",
	"PINNED_POLL":          "pinned a poll",
	// Reaction pushes are not emitted (updateMessageReactions has no single
	// actor). The phrases are here so a REACT_* loc-key still renders.
	"REACT_NOTEXT":  "reacted to your message",
	"REACT_TEXT":    "reacted to your message",
	"REACT_PHOTO":   "reacted to your photo",
	"REACT_VIDEO":   "reacted to your video",
	"REACT_GIF":     "reacted to your GIF",
	"REACT_ROUND":   "reacted to your video message",
	"REACT_AUDIO":   "reacted to your voice message",
	"REACT_STICKER": "reacted to your sticker",
	"REACT_DOC":     "reacted to your file",
	"REACT_GEO":     "reacted to your map",
	"REACT_GEOLIVE": "reacted to your live location",
	"REACT_CONTACT": "reacted to your contact",
	"REACT_POLL":    "reacted to your poll",
}

var zhHansPhrases = map[string]string{
	"MESSAGE_NOTEXT":       "发来一条消息",
	"MESSAGE_PHOTO":        "发来一张图片",
	"MESSAGE_PHOTO_SECRET": "发来一张阅后即焚图片",
	"MESSAGE_VIDEO":        "发来一段视频",
	"MESSAGE_VIDEO_SECRET": "发来一段阅后即焚视频",
	"MESSAGE_GIF":          "发来一张 GIF",
	"MESSAGE_ROUND":        "发来一条视频消息",
	"MESSAGE_AUDIO":        "发来一条语音消息",
	"MESSAGE_STICKER":      "发来一张贴纸",
	"MESSAGE_DOC":          "发来一个文件",
	"MESSAGE_GEO":          "发来一个位置",
	"MESSAGE_GEOLIVE":      "开始共享实时位置",
	"MESSAGE_CONTACT":      "分享了一个联系人",
	"MESSAGE_POLL":         "发来一个投票",
	"MESSAGE_QUIZ":         "发来一个问答",
	"PINNED_NOTEXT":        "置顶了一条消息",
	"PINNED_TEXT":          "置顶了一条消息",
	"PINNED_PHOTO":         "置顶了一张图片",
	"PINNED_VIDEO":         "置顶了一段视频",
	"PINNED_GIF":           "置顶了一张 GIF",
	"PINNED_ROUND":         "置顶了一条视频消息",
	"PINNED_AUDIO":         "置顶了一条语音消息",
	"PINNED_STICKER":       "置顶了一张贴纸",
	"PINNED_DOC":           "置顶了一个文件",
	"PINNED_GEO":           "置顶了一个位置",
	"PINNED_GEOLIVE":       "置顶了实时位置",
	"PINNED_CONTACT":       "置顶了一个联系人",
	"PINNED_POLL":          "置顶了一个投票",
	"REACT_NOTEXT":         "回应了你的消息",
	"REACT_TEXT":           "回应了你的消息",
	"REACT_PHOTO":          "回应了你的图片",
	"REACT_VIDEO":          "回应了你的视频",
	"REACT_GIF":            "回应了你的 GIF",
	"REACT_ROUND":          "回应了你的视频消息",
	"REACT_AUDIO":          "回应了你的语音消息",
	"REACT_STICKER":        "回应了你的贴纸",
	"REACT_DOC":            "回应了你的文件",
	"REACT_GEO":            "回应了你的位置",
	"REACT_GEOLIVE":        "回应了你的实时位置",
	"REACT_CONTACT":        "回应了你的联系人",
	"REACT_POLL":           "回应了你的投票",
}

var zhHantPhrases = map[string]string{
	"MESSAGE_NOTEXT":       "發來一條訊息",
	"MESSAGE_PHOTO":        "發來一張圖片",
	"MESSAGE_PHOTO_SECRET": "發來一張閱後即焚圖片",
	"MESSAGE_VIDEO":        "發來一段影片",
	"MESSAGE_VIDEO_SECRET": "發來一段閱後即焚影片",
	"MESSAGE_GIF":          "發來一張 GIF",
	"MESSAGE_ROUND":        "發來一條影片訊息",
	"MESSAGE_AUDIO":        "發來一條語音訊息",
	"MESSAGE_STICKER":      "發來一張貼圖",
	"MESSAGE_DOC":          "發來一個檔案",
	"MESSAGE_GEO":          "發來一個位置",
	"MESSAGE_GEOLIVE":      "開始分享即時位置",
	"MESSAGE_CONTACT":      "分享了一個聯絡人",
	"MESSAGE_POLL":         "發來一個投票",
	"MESSAGE_QUIZ":         "發來一個問答",
	"PINNED_NOTEXT":        "置頂了一條訊息",
	"PINNED_TEXT":          "置頂了一條訊息",
	"PINNED_PHOTO":         "置頂了一張圖片",
	"PINNED_VIDEO":         "置頂了一段影片",
	"PINNED_GIF":           "置頂了一張 GIF",
	"PINNED_ROUND":         "置頂了一條影片訊息",
	"PINNED_AUDIO":         "置頂了一條語音訊息",
	"PINNED_STICKER":       "置頂了一張貼圖",
	"PINNED_DOC":           "置頂了一個檔案",
	"PINNED_GEO":           "置頂了一個位置",
	"PINNED_GEOLIVE":       "置頂了即時位置",
	"PINNED_CONTACT":       "置頂了一個聯絡人",
	"PINNED_POLL":          "置頂了一個投票",
	"REACT_NOTEXT":         "回應了你的訊息",
	"REACT_TEXT":           "回應了你的訊息",
	"REACT_PHOTO":          "回應了你的圖片",
	"REACT_VIDEO":          "回應了你的影片",
	"REACT_GIF":            "回應了你的 GIF",
	"REACT_ROUND":          "回應了你的影片訊息",
	"REACT_AUDIO":          "回應了你的語音訊息",
	"REACT_STICKER":        "回應了你的貼圖",
	"REACT_DOC":            "回應了你的檔案",
	"REACT_GEO":            "回應了你的位置",
	"REACT_GEOLIVE":        "回應了你的即時位置",
	"REACT_CONTACT":        "回應了你的聯絡人",
	"REACT_POLL":           "回應了你的投票",
}
