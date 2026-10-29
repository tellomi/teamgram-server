package catalog

import (
	"embed"
	"strconv"
)

//go:embed assets
var assetFS embed.FS

type emojiDef struct {
	Code     string
	Keywords []string
	Reaction string
	Gift     bool
	Status   bool
	Topic    bool
	Channel  bool
}

// emojiCatalog is the openly licensed subset shipped with the server.
// Twemoji PNG and Fluent PNG were converted to lossless WebP.
// Noto animated Lottie was gzipped to .tgs.
var emojiCatalog = []emojiDef{
	{"1f600", []string{"grinning", "smile", "happy"}, "Grinning face", false, true, false, true},
	{"1f602", []string{"joy", "laugh", "tears"}, "Face with tears of joy", false, true, false, false},
	{"1f60d", []string{"heart eyes", "love"}, "Smiling face with heart-eyes", false, true, false, false},
	{"1f618", []string{"kiss"}, "Face blowing a kiss", false, false, false, false},
	{"1f44d", []string{"thumbs up", "like", "yes"}, "Thumbs up", false, true, true, true},
	{"1f44e", []string{"thumbs down", "dislike", "no"}, "Thumbs down", false, false, false, false},
	{"2764", []string{"heart", "love"}, "Red heart", false, true, false, true},
	{"1f525", []string{"fire", "lit", "hot"}, "Fire", false, true, false, false},
	{"1f389", []string{"party", "tada", "celebration"}, "Party popper", false, true, false, false},
	{"1f60e", []string{"cool", "sunglasses"}, "Smiling face with sunglasses", false, false, false, false},
	{"1f622", []string{"cry", "sad"}, "Crying face", false, true, false, false},
	{"1f621", []string{"angry", "mad"}, "Angry face", false, true, false, false},
	{"1f64f", []string{"please", "pray", "thanks"}, "Folded hands", false, false, false, false},
	{"1f680", []string{"rocket", "ship"}, "Rocket", false, false, true, false},
	{"2705", []string{"check", "done", "yes"}, "Check mark", false, true, false, true},
	{"274c", []string{"cross", "no"}, "Cross mark", false, false, false, false},
	{"1f31f", []string{"star", "glow"}, "Glowing star", false, true, true, false},
	{"1f308", []string{"rainbow"}, "Rainbow", false, false, false, false},
	{"1f436", []string{"dog"}, "", false, false, false, false},
	{"1f431", []string{"cat"}, "", false, false, true, false},
	{"1f355", []string{"pizza", "food"}, "", false, false, false, false},
	{"2615", []string{"coffee", "tea"}, "", false, false, false, false},
	{"1f4a1", []string{"idea", "bulb"}, "", false, false, true, false},
	{"1f381", []string{"gift", "present"}, "Wrapped gift", true, false, false, false},
}

func builtinPacks() []*Pack {
	var twemoji, fluent, noto []*Item
	var gifts, statuses, topics, channels []*Item
	for _, def := range emojiCatalog {
		alt := emojiFromCode(def.Code)
		if webp, ok := readAsset("assets/twemoji/" + def.Code + ".webp"); ok {
			item := newItem("twemoji", def.Code, alt, "image/webp", def.Code+".webp", 72, 72, false, false, "stickers", webp, def.Keywords)
			twemoji = append(twemoji, item)
		}
		if tgs, ok := readAsset("assets/noto/" + def.Code + ".tgs"); ok {
			item := newItem("noto", def.Code, alt, "application/x-tgsticker", def.Code+".tgs", 512, 512, true, true, "emoji", tgs, def.Keywords)
			noto = append(noto, item)
			if def.Gift {
				gifts = append(gifts, item)
			}
			if def.Status {
				statuses = append(statuses, item)
			}
			if def.Topic {
				topics = append(topics, item)
			}
			if def.Channel {
				channels = append(channels, item)
			}
		}
		if webp, ok := readAsset("assets/fluent/" + def.Code + ".webp"); ok {
			item := newItem("fluent", def.Code, alt, "image/webp", def.Code+".webp", 256, 256, false, false, "stickers", webp, def.Keywords)
			fluent = append(fluent, item)
		}
	}
	packs := []*Pack{
		newPack(ShortTwemoji, "Twemoji", KindSticker, false, false, true, true,
			"CC-BY-4.0", "Twemoji by Twitter, Inc. and contributors",
			"https://github.com/jdecked/twemoji", twemoji),
		newPack(ShortFluent, "Fluent Emoji", KindSticker, false, false, true, false,
			"MIT", "Fluent Emoji by Microsoft",
			"https://github.com/microsoft/fluentui-emoji", fluent),
		newPack(ShortNoto, "Noto Emoji", KindCustomEmoji, true, true, true, true,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", noto),
		newPack(ShortAnimatedEmoji, "Animated Emoji", KindAnimatedEmoji, true, true, false, false,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", noto),
		newPack(ShortAnimatedAnimations, "Animated Emoji Animations", KindAnimatedAnimations, true, true, false, false,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", noto),
		newPack(ShortGenericAnimations, "Emoji Generic Animations", KindGenericAnimations, true, true, false, false,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", noto),
		newPack(ShortPremiumGifts, "Premium Gifts", KindPremiumGifts, true, true, false, false,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", gifts),
		newPack(ShortDefaultStatuses, "Emoji Default Statuses", KindDefaultStatuses, true, true, false, false,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", statuses),
		newPack(ShortTopicIcons, "Emoji Default Topic Icons", KindTopicIcons, true, true, false, false,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", topics),
		newPack(ShortChannelStatuses, "Emoji Channel Default Statuses", KindChannelStatuses, true, true, false, false,
			"OFL-1.1 AND Apache-2.0", "Noto Emoji Animation by Google",
			"https://github.com/googlefonts/noto-emoji", channels),
	}
	var out []*Pack
	for _, p := range packs {
		if len(p.Items) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func readAsset(path string) ([]byte, bool) {
	b, err := assetFS.ReadFile(path)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

func newItem(family, code, alt, mime, file string, w, h int32, animated, custom bool, class string, body []byte, keywords []string) *Item {
	id := stableID("doc", family, code)
	return &Item{
		DocumentID:   id,
		AccessHash:   stableID("access", family, code),
		Emoticon:     alt,
		MIME:         mime,
		FileName:     file,
		Width:        w,
		Height:       h,
		Animated:     animated,
		CustomEmoji:  custom,
		StorageClass: class,
		ObjectPath:   strconv.FormatInt(id, 10) + ".dat",
		AssetPath:    "assets/" + family + "/" + file,
		Keywords:     keywords,
		Size:         int32(len(body)),
	}
}

func newPack(short, title, kind string, emojis, animated, featured, installed bool, license, attribution, source string, items []*Item) *Pack {
	return &Pack{
		ID:               stableID("pack", short),
		AccessHash:       stableID("pack-access", short),
		ShortName:        short,
		Title:            title,
		Kind:             kind,
		Official:         true,
		Emojis:           emojis,
		Animated:         animated,
		Featured:         featured,
		InstalledDefault: installed,
		License:          license,
		Attribution:      attribution,
		SourceURL:        source,
		Items:            items,
	}
}
