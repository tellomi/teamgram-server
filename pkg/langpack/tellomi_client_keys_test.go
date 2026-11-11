package langpack

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// tellomiClientManifest is data/langpack/tellomi-client-keys.json.
// base is master / release/ios. pull_requests lists keys named by an open
// client PR, including iOS PR 162's 105 Moments strings.
type tellomiClientManifest struct {
	Android tellomiPlatformKeys `json:"android"`
	IOS     tellomiPlatformKeys `json:"ios"`
}

type tellomiPlatformKeys struct {
	Base         []string            `json:"base"`
	PullRequests map[string][]string `json:"pull_requests"`
}

func (p tellomiPlatformKeys) all() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(keys []string) {
		for _, key := range keys {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	add(p.Base)
	for _, keys := range p.PullRequests {
		add(keys)
	}
	return out
}

func loadTellomiManifest(t *testing.T) tellomiClientManifest {
	t.Helper()
	body, err := os.ReadFile("../../data/langpack/tellomi-client-keys.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc tellomiClientManifest
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestTellomiClientKeysInEveryPack fails when a Tellomi client key is missing
// from any Android or iOS language, or when a changed pack did not bump the
// string version to the pack version (so getDifference would skip it).
func TestTellomiClientKeysInEveryPack(t *testing.T) {
	doc := loadTellomiManifest(t)
	ios162 := doc.IOS.PullRequests["pr:162"]
	if len(ios162) != 105 {
		t.Fatalf("ios pr:162 keys %d, want 105", len(ios162))
	}
	ios163 := doc.IOS.PullRequests["pr:163"]
	if len(ios163) != 6 {
		t.Fatalf("ios pr:163 keys %d, want 6", len(ios163))
	}
	for _, key := range []string{
		"Moments.Draft.Message",
		"Moments.Outbox.Title",
		"Moments.Outbox.Text",
		"Moments.Outbox.Retry",
		"Moments.Outbox.Later",
		"Moments.Outbox.Incomplete",
	} {
		if !containsKey(ios163, key) {
			t.Fatalf("ios pr:163 manifest missing %s", key)
		}
	}
	for _, key := range []string{"Moments_Comment_SendFailed", "Moments_Audience_Kept"} {
		if !containsKey(ios162, key) {
			t.Fatalf("ios pr:162 manifest missing %s", key)
		}
	}
	androidKeys := doc.Android.all()
	iosKeys := doc.IOS.all()
	if len(androidKeys) < 100 || len(iosKeys) < 100 {
		t.Fatalf("android keys %d ios keys %d", len(androidKeys), len(iosKeys))
	}
	// Tab captions the menu screen already uses. MainTabs* and Settings are
	// official strings. Moments_Title is Tellomi-only.
	for _, key := range []string{"Moments_Title", "MainTabsChats", "MainTabsProfile", "MainTabsContacts", "MainTabsCalls"} {
		if !containsKey(androidKeys, key) && key == "Moments_Title" {
			t.Fatalf("android manifest missing %s", key)
		}
	}

	for _, tc := range []struct {
		platform string
		keys     []string
	}{
		{"android", androidKeys},
		{"ios", iosKeys},
	} {
		dir := filepath.Join("..", "..", "data", "langpack", tc.platform)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var packs int
		for _, ent := range entries {
			name := ent.Name()
			if ent.IsDir() || len(name) < len(".json.gz") || name[len(name)-len(".json.gz"):] != ".json.gz" {
				continue
			}
			packs++
			code := name[:len(name)-len(".json.gz")]
			pack := readPackVersions(t, filepath.Join(dir, name))
			if pack.lang != code || pack.plural != pluralCodeByLang[code] {
				t.Fatalf("%s %s lang %q plural %q", tc.platform, code, pack.lang, pack.plural)
			}
			if tc.platform == "android" {
				if pack.version != 61730403 || pack.fix != 61730400 {
					t.Fatalf("android %s version %d fix %d", code, pack.version, pack.fix)
				}
			}
			if tc.platform == "ios" {
				want, ok := iosPackVersion[code]
				if !ok || pack.version != want {
					t.Fatalf("ios %s version %d want %d", code, pack.version, want)
				}
			}
			for _, key := range tc.keys {
				ver, ok := pack.strings[key]
				if !ok {
					t.Fatalf("%s %s missing %s", tc.platform, code, key)
				}
				// Changed or new strings are at the pack version. Strings this
				// fill left alone stay at the previous pack, or up to three
				// further back if they were already unchanged in #298, #299,
				// and #302.
				if ver < pack.version-4 || ver > pack.version {
					t.Fatalf("%s %s %s string version %d pack %d", tc.platform, code, key, ver, pack.version)
				}
			}
			for _, key := range []string{"MainTabsChats", "MainTabsProfile", "MainTabsContacts", "MainTabsCalls"} {
				if tc.platform != "android" {
					continue
				}
				if _, ok := pack.strings[key]; !ok {
					t.Fatalf("android %s missing %s", code, key)
				}
			}
		}
		if packs < 30 {
			t.Fatalf("%s packs %d", tc.platform, packs)
		}
	}

	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	checks := []struct {
		platform string
		code     string
		key      string
		value    string
	}{
		{"android", "en", "TellomiMenuSettings", "Menu settings"},
		{"android", "en", "MainTabsMenuSettings", "Menu settings"},
		{"android", "en", "SettingsTools", "Tools"},
		{"android", "en", "MainTabsHideLabels", "Hide button labels"},
		{"android", "en", "MainTabsButtons", "Navigation bar buttons"},
		{"android", "zh-hans", "TellomiMenuSettings", "选单设定"},
		{"android", "zh-hans", "MainTabsMenuSettings", "选单设定"},
		{"android", "zh-hans", "SettingsTools", "工具"},
		{"android", "zh-hans", "MainTabsHideLabels", "隐藏按钮文字"},
		{"android", "zh-hans", "MainTabsButtons", "导航栏按钮"},
		{"android", "zh-hans", "Moments_Title", "动态"},
		{"android", "zh-hans", "Moments_FollowingEntry", "关注"},
		{"android", "zh-hant", "TellomiMenuSettings", "選單設定"},
		{"android", "zh-hant", "MainTabsMenuSettings", "選單設定"},
		{"android", "zh-hant", "MainTabsHideLabels", "隱藏按鈕文字"},
		{"android", "de", "TellomiMenuSettings", "Menüeinstellungen"},
		{"ios", "en", "Tellomi.MenuSettings.Title", "Menu settings"},
		{"ios", "en", "Tellomi.MenuSettings.HideButtonTitles", "Hide button titles"},
		{"ios", "en", "Moments.Title", "Moments"},
		{"ios", "zh-hans", "Tellomi.MenuSettings.Title", "选单设定"},
		{"ios", "zh-hans", "Moments.Title", "动态"},
		{"ios", "zh-hans", "Moments.Followers", "粉丝"},
		{"ios", "zh-hant", "Tellomi.MenuSettings.Title", "選單設定"},
		{"ios", "zh-hant", "Moments.Title", "動態"},
		{"ios", "zh-hant", "Moments.Followers", "粉絲"},
		{"ios", "en", "Moments.Draft.Message", "Save this post as a draft, or discard it?"},
		{"ios", "en", "Moments.Outbox.Title", "Post not sent"},
		{"ios", "en", "Moments.Outbox.Text", "A post is still waiting to be sent."},
		{"ios", "en", "Moments.Outbox.Retry", "Try again"},
		{"ios", "en", "Moments.Outbox.Later", "Not now"},
		{"ios", "en", "Moments.Outbox.Incomplete", "An attachment is missing, so this post was not sent."},
		{"ios", "en", "Moments_Comment_SendFailed", "Couldn't send this comment. Your text is still here."},
		{"ios", "en", "Moments_Audience_Kept", "Couldn't change the audience. Your selection was kept."},
		{"ios", "zh-hans", "Moments.Draft.Message", "把这条动态存成草稿，还是放弃？"},
		{"ios", "zh-hans", "Moments.Outbox.Title", "动态没有发出"},
		{"ios", "zh-hans", "Moments.Outbox.Text", "还有一条动态在等着发送。"},
		{"ios", "zh-hans", "Moments.Outbox.Retry", "再试一次"},
		{"ios", "zh-hans", "Moments.Outbox.Later", "以后再说"},
		{"ios", "zh-hans", "Moments.Outbox.Incomplete", "附件不见了，所以这条动态没有发出。"},
		{"ios", "zh-hans", "Moments_Comment_SendFailed", "评论没有发出去，内容还在。"},
		{"ios", "zh-hans", "Moments_Audience_Kept", "没能修改可见范围，你的选择还在。"},
		{"ios", "zh-hant", "Moments.Draft.Message", "要把這則動態存成草稿，還是放棄？"},
		{"ios", "zh-hant", "Moments.Outbox.Title", "動態沒有送出"},
		{"ios", "zh-hant", "Moments.Outbox.Text", "還有一則動態在等著傳送。"},
		{"ios", "zh-hant", "Moments.Outbox.Retry", "再試一次"},
		{"ios", "zh-hant", "Moments.Outbox.Later", "暫時不要"},
		{"ios", "zh-hant", "Moments.Outbox.Incomplete", "附件不見了，所以這則動態沒有送出。"},
		{"ios", "zh-hant", "Moments_Comment_SendFailed", "留言沒有送出，內容還在。"},
		{"ios", "zh-hant", "Moments_Audience_Kept", "沒能修改可見範圍，你的選擇還在。"},
	}
	for _, tc := range checks {
		got, err := cat.Strings(tc.platform, tc.code, []string{tc.key})
		if err != nil || len(got) != 1 || got[0].GetPredicateName() != mtproto.Predicate_langPackString || got[0].GetValue() != tc.value {
			value := ""
			if err == nil && len(got) == 1 {
				value = got[0].GetValue()
			}
			t.Fatalf("%s %s %s got %q (%v) want %q", tc.platform, tc.code, tc.key, value, err, tc.value)
		}
	}
	// zh-hans and zh-hant must not keep the English text for a Tellomi key
	// that has Chinese. A string that is the same in Chinese (PDF) may match.
	for _, platform := range []string{"android", "ios"} {
		keys := androidKeys
		if platform == "ios" {
			keys = iosKeys
		}
		enText := readPackText(t, "../../data/langpack/"+platform+"/en.json.gz")
		hansText := readPackText(t, "../../data/langpack/"+platform+"/zh-hans.json.gz")
		hantText := readPackText(t, "../../data/langpack/"+platform+"/zh-hant.json.gz")
		for _, key := range keys {
			en, hans, hant := enText[key], hansText[key], hantText[key]
			if hans == "" || hant == "" {
				t.Fatalf("%s %s empty hans %q hant %q", platform, key, hans, hant)
			}
			if hans == en && hant == en {
				continue
			}
			if hans == en || hant == en {
				t.Fatalf("%s %s still English hans %q hant %q en %q", platform, key, hans, hant, en)
			}
		}
	}
	if hans := readPackText(t, "../../data/langpack/ios/zh-hans.json.gz"); hans["Story.Privacy.CategoryMutualFollowers"] != "互相关注" || hans["Story.Privacy.WhoCanViewMoment"] != "谁可以看" {
		t.Fatalf("ios story hans needs review copy: %+v", []string{hans["Story.Privacy.CategoryMutualFollowers"], hans["Story.Privacy.WhoCanViewMoment"]})
	}
	if hant := readPackText(t, "../../data/langpack/ios/zh-hant.json.gz"); hant["Story.Privacy.CategoryMutualFollowers"] != "互相關注" || hant["Story.Privacy.WhoCanViewMoment"] != "誰可以看" {
		t.Fatalf("ios story hant needs review copy: %+v", []string{hant["Story.Privacy.CategoryMutualFollowers"], hant["Story.Privacy.WhoCanViewMoment"]})
	}
	assertTellomiNotEnglish(t, "android", androidKeys)
	assertTellomiNotEnglish(t, "ios", iosKeys)

	ver, ok := cat.Version("android", "zh-hans")
	if !ok || ver != 61730403 {
		t.Fatalf("android zh-hans version %d", ver)
	}
	// 61730400 is the full-replace cutover. A phone still there receives the
	// Tellomi strings as an incremental difference, not a new file.
	diff, err := cat.Difference("android", "zh-hans", 61730400)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetFromVersion() != 61730400 || diff.GetVersion() != ver || len(diff.GetStrings()) == 0 {
		t.Fatalf("android cutover diff from %d version %d n %d", diff.GetFromVersion(), diff.GetVersion(), len(diff.GetStrings()))
	}
	seen := map[string]string{}
	for _, s := range diff.GetStrings() {
		if s.GetPredicateName() == mtproto.Predicate_langPackString {
			seen[s.GetKey()] = s.GetValue()
		}
	}
	if seen["MainTabsMenuSettings"] != "选单设定" || seen["TellomiMenuSettings"] != "选单设定" || seen["Moments_Title"] != "动态" {
		t.Fatalf("android getDifference menu %+v", []string{seen["MainTabsMenuSettings"], seen["TellomiMenuSettings"], seen["Moments_Title"]})
	}
	// Chinese has new wording at 61730403. A phone already at 61730402
	// receives the changed audience labels and the new recommendation keys.
	// Moments_Title did not change, so it stays out of that incremental diff.
	zhInc, err := cat.Difference("android", "zh-hans", 61730402)
	if err != nil {
		t.Fatal(err)
	}
	if zhInc.GetFromVersion() != 61730402 || zhInc.GetVersion() != ver || len(zhInc.GetStrings()) == 0 {
		t.Fatalf("android zh increment from %d version %d n %d", zhInc.GetFromVersion(), zhInc.GetVersion(), len(zhInc.GetStrings()))
	}
	zhSeen := map[string]string{}
	for _, s := range zhInc.GetStrings() {
		if s.GetPredicateName() == mtproto.Predicate_langPackString {
			zhSeen[s.GetKey()] = s.GetValue()
		}
	}
	if zhSeen["Moments_Audience_Mutual"] != "互相关注" || zhSeen["Moments_ChangeAudience"] != "修改可见范围" || zhSeen["Moments_Recommendations"] != "推荐" || zhSeen["Moments_Personalized"] != "个性化推荐" || zhSeen["Moments_ExternalShare"] != "其他应用" || zhSeen["Moments_VideoTooLongN"] != "请把视频裁剪到 %1$d 秒以内" {
		t.Fatalf("android zh increment copy %+v", []string{zhSeen["Moments_Audience_Mutual"], zhSeen["Moments_ChangeAudience"], zhSeen["Moments_Recommendations"], zhSeen["Moments_Personalized"], zhSeen["Moments_ExternalShare"], zhSeen["Moments_VideoTooLongN"]})
	}
	// Moments_WhoCanSee is a page title, not the own-post menu item; it stays on the master wording and version.
	if _, ok := zhSeen["Moments_WhoCanSee"]; ok {
		t.Fatalf("android zh increment resent unchanged Moments_WhoCanSee")
	}
	if _, ok := zhSeen["Moments_Title"]; ok {
		t.Fatalf("android zh increment resent unchanged Moments_Title")
	}
	ja, err := cat.Difference("android", "ja", 61730401)
	if err != nil {
		t.Fatal(err)
	}
	if ja.GetFromVersion() != 61730401 || ja.GetVersion() != ver || len(ja.GetStrings()) == 0 {
		t.Fatalf("android ja diff from %d n %d", ja.GetFromVersion(), len(ja.GetStrings()))
	}
	jaTitle := ""
	for _, s := range ja.GetStrings() {
		if s.GetKey() == "Moments_Title" {
			jaTitle = s.GetValue()
		}
	}
	if jaTitle != "モーメント" {
		t.Fatalf("android ja Moments_Title %q", jaTitle)
	}
	jaOld, err := cat.Difference("android", "ja", 61730400)
	if err != nil || jaOld.GetFromVersion() != 61730400 {
		t.Fatalf("android ja cutover from %d %v", jaOld.GetFromVersion(), err)
	}
	caught, err := cat.Difference("android", "zh-hans", ver)
	if err != nil || len(caught.GetStrings()) != 0 {
		t.Fatalf("android caught up %+v %v", caught, err)
	}

	iosVer, ok := cat.Version("ios", "zh-hans")
	if !ok || iosVer != 22454355 {
		t.Fatalf("ios zh-hans version %d", iosVer)
	}
	// The Chinese Moments title did not change, so it stays at the previous
	// string version. A client one behind that string still receives it.
	iosDiff, err := cat.Difference("ios", "zh-hans", 22454350)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range iosDiff.GetStrings() {
		if s.GetKey() == "Moments.Title" && s.GetValue() == "动态" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ios getDifference missed Moments.Title")
	}
	iosCaught, err := cat.Difference("ios", "zh-hans", iosVer)
	if err != nil || len(iosCaught.GetStrings()) != 0 {
		t.Fatalf("ios zh caught up %+v %v", iosCaught, err)
	}
	jaIOS, err := cat.Difference("ios", "ja", 22418204)
	if err != nil {
		t.Fatal(err)
	}
	jaMoments := ""
	for _, s := range jaIOS.GetStrings() {
		if s.GetKey() == "Moments.Title" {
			jaMoments = s.GetValue()
		}
	}
	if jaIOS.GetFromVersion() != 22418204 || jaMoments != "モーメント" {
		t.Fatalf("ios ja Moments.Title %q from %d", jaMoments, jaIOS.GetFromVersion())
	}
	hantVer, ok := cat.Version("ios", "zh-hant")
	if !ok || hantVer != 22421540 {
		t.Fatalf("ios zh-hant version %d", hantVer)
	}
	iosInc, err := cat.Difference("ios", "zh-hans", 22454352)
	if err != nil {
		t.Fatal(err)
	}
	iosSeen := map[string]string{}
	for _, s := range iosInc.GetStrings() {
		if s.GetPredicateName() == mtproto.Predicate_langPackString {
			iosSeen[s.GetKey()] = s.GetValue()
		}
	}
	if iosInc.GetFromVersion() != 22454352 || iosSeen["Story.Privacy.CategoryMutualFollowers"] != "互相关注" || iosSeen["Moments.Audience.Mutual"] != "互相关注" || iosSeen["Moments.Outbox.Title"] != "动态没有发出" || iosSeen["Moments.Settings.Recommendations"] != "推荐" {
		t.Fatalf("ios zh increment copy from %d %+v", iosInc.GetFromVersion(), []string{iosSeen["Story.Privacy.CategoryMutualFollowers"], iosSeen["Moments.Audience.Mutual"], iosSeen["Moments.Outbox.Title"], iosSeen["Moments.Settings.Recommendations"]})
	}
	// Story.Privacy.WhoCanViewMoment is the composer/nav title, not the own-post menu item.
	if _, ok := iosSeen["Story.Privacy.WhoCanViewMoment"]; ok {
		t.Fatalf("ios zh increment resent unchanged Story.Privacy.WhoCanViewMoment")
	}

	// A phone already at the #299 pack (22454353) still receives the crash-prompt
	// keys, plus the Moments wording updated after #302.
	crashDiff, err := cat.Difference("ios", "zh-hans", 22454353)
	if err != nil {
		t.Fatal(err)
	}
	crashSeen := map[string]string{}
	for _, s := range crashDiff.GetStrings() {
		if s.GetPredicateName() == mtproto.Predicate_langPackString {
			crashSeen[s.GetKey()] = s.GetValue()
		}
	}
	wantText := "应用意外关闭了。\n\n要把崩溃日志发给我们，帮我们修好这个问题吗？"
	if crashDiff.GetFromVersion() != 22454353 || crashDiff.GetVersion() != iosVer || crashSeen["CrashReport.Prompt.Title"] != "有新的崩溃日志" || crashSeen["CrashReport.Prompt.Text"] != wantText || crashSeen["CrashReport.Prompt.NotNow"] != "以后再说" || crashSeen["CrashReport.Prompt.Send"] != "发送日志" {
		t.Fatalf("ios crash-prompt increment from %d version %d %+v", crashDiff.GetFromVersion(), crashDiff.GetVersion(), []string{crashSeen["CrashReport.Prompt.Title"], crashSeen["CrashReport.Prompt.Text"], crashSeen["CrashReport.Prompt.NotNow"], crashSeen["CrashReport.Prompt.Send"]})
	}
	if _, ok := crashSeen["Moments.Title"]; ok {
		t.Fatalf("ios crash-prompt increment resent unchanged Moments.Title")
	}

	// A phone already at the #302 pack (22454354) receives only the updated Moments copy.
	momentsDiff, err := cat.Difference("ios", "zh-hans", 22454354)
	if err != nil {
		t.Fatal(err)
	}
	momentsSeen := map[string]string{}
	for _, s := range momentsDiff.GetStrings() {
		if s.GetPredicateName() == mtproto.Predicate_langPackString {
			momentsSeen[s.GetKey()] = s.GetValue()
		}
	}
	if momentsDiff.GetFromVersion() != 22454354 || momentsDiff.GetVersion() != iosVer || momentsSeen["Moments.Draft.Message"] != "把这条动态存成草稿，还是放弃？" || momentsSeen["Moments.Outbox.Title"] != "动态没有发出" || momentsSeen["Moments.Outbox.Text"] != "还有一条动态在等着发送。" || momentsSeen["Moments.Outbox.Retry"] != "再试一次" || momentsSeen["Moments.Outbox.Later"] != "以后再说" || momentsSeen["Moments.Outbox.Incomplete"] != "附件不见了，所以这条动态没有发出。" || momentsSeen["Moments_Audience_Kept"] != "没能修改可见范围，你的选择还在。" {
		t.Fatalf("ios moments increment from %d version %d %+v", momentsDiff.GetFromVersion(), momentsDiff.GetVersion(), []string{momentsSeen["Moments.Draft.Message"], momentsSeen["Moments.Outbox.Title"], momentsSeen["Moments.Outbox.Text"], momentsSeen["Moments.Outbox.Retry"], momentsSeen["Moments.Outbox.Later"], momentsSeen["Moments.Outbox.Incomplete"], momentsSeen["Moments_Audience_Kept"]})
	}
	if _, ok := momentsSeen["Moments_Comment_SendFailed"]; ok {
		t.Fatalf("ios moments increment resent unchanged Moments_Comment_SendFailed")
	}
	if _, ok := momentsSeen["Moments.Title"]; ok {
		t.Fatalf("ios moments increment resent unchanged Moments.Title")
	}
	if _, ok := momentsSeen["CrashReport.Prompt.Title"]; ok {
		t.Fatalf("ios moments increment resent unchanged CrashReport.Prompt.Title")
	}
}

// englishAllowlist is data/langpack/tellomi-english-allowlist.json.
// brands match the whole value. entries name one key a language keeps in English.
type englishAllowlist struct {
	Brands  []string `json:"brands"`
	Entries []struct {
		Platform string `json:"platform"`
		Code     string `json:"code"`
		Key      string `json:"key"`
		Value    string `json:"value"`
		Reason   string `json:"reason"`
	} `json:"entries"`
}

func assertTellomiNotEnglish(t *testing.T, platform string, keys []string) {
	t.Helper()
	body, err := os.ReadFile("../../data/langpack/tellomi-english-allowlist.json")
	if err != nil {
		t.Fatal(err)
	}
	var allow englishAllowlist
	if err := json.Unmarshal(body, &allow); err != nil {
		t.Fatal(err)
	}
	brands := map[string]struct{}{}
	for _, brand := range allow.Brands {
		if brand != "PDF" && brand != "Tellomi" && brand != "TELLOMI" {
			t.Fatalf("brand %q is not a documented name", brand)
		}
		brands[brand] = struct{}{}
	}
	entries := map[string]string{}
	for _, row := range allow.Entries {
		if row.Reason == "" {
			t.Fatalf("allowlist entry %s %s %s has no reason", row.Platform, row.Code, row.Key)
		}
		entries[row.Platform+"\x00"+row.Code+"\x00"+row.Key] = row.Value
	}
	dir := filepath.Join("..", "..", "data", "langpack", platform)
	en := readPackText(t, filepath.Join(dir, "en.json.gz"))
	entriesDir, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for _, ent := range entriesDir {
		name := ent.Name()
		if ent.IsDir() || len(name) < len(".json.gz") || name[len(name)-len(".json.gz"):] != ".json.gz" {
			continue
		}
		code := name[:len(name)-len(".json.gz")]
		if code == "en" {
			continue
		}
		text := readPackText(t, filepath.Join(dir, name))
		for _, key := range keys {
			value, english := text[key], en[key]
			if value != english {
				continue
			}
			id := platform + "\x00" + code + "\x00" + key
			if _, ok := brands[value]; ok {
				if want, listed := entries[id]; listed {
					if want != value {
						t.Fatalf("allowlist value drifted %s %s %s", platform, code, key)
					}
					seen[id] = struct{}{}
				}
				continue
			}
			if entries[id] != value {
				t.Fatalf("%s %s %s still English %q", platform, code, key, value)
			}
			seen[id] = struct{}{}
		}
	}
	for _, row := range allow.Entries {
		if row.Platform != platform {
			continue
		}
		id := row.Platform + "\x00" + row.Code + "\x00" + row.Key
		if _, ok := seen[id]; !ok {
			t.Fatalf("allowlist entry is not an English copy anymore: %s %s %s", row.Platform, row.Code, row.Key)
		}
	}
}

func containsKey(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

type packVersions struct {
	version int32
	fix     int32
	lang    string
	plural  string
	strings map[string]int32
}

// iosPackVersion is one past the versions shipped in #302.
var iosPackVersion = map[string]int32{
	"ar": 22452761, "be": 22420173, "ca": 22412801, "cs": 22418782,
	"de": 22431605, "el": 22202262, "en": 22347353, "es": 22412203,
	"fa": 22437416, "fi": 22410857, "fr": 22391562, "he": 22452945,
	"hi": 22202262, "id": 22321178, "it": 22388321, "ja": 22418208,
	"ko": 22454173, "ms": 22352694, "nb": 22420486, "nl": 22449825,
	"pl": 22392161, "pt-br": 22431436, "ro": 22444910, "ru": 22421470,
	"sv": 22427917, "th": 22202262, "tr": 22434112, "uk": 22460359,
	"uz": 22449136, "vi": 22347893, "zh-hans": 22454355, "zh-hant": 22421540,
}

// pluralCodeByLang is the plural_code each pack already ships. This fill must
// not change it. pt-br uses pt. Chinese uses zh.
var pluralCodeByLang = map[string]string{
	"en": "en", "ar": "ar", "be": "be", "ca": "ca", "cs": "cs", "de": "de",
	"el": "el", "es": "es", "fa": "fa", "fi": "fi", "fr": "fr", "he": "he",
	"hi": "hi", "id": "id", "it": "it", "ja": "ja", "ko": "ko", "ms": "ms",
	"nb": "nb", "nl": "nl", "pl": "pl", "pt-br": "pt", "ro": "ro", "ru": "ru",
	"sv": "sv", "th": "th", "tr": "tr", "uk": "uk", "uz": "uz", "vi": "vi",
	"zh-hans": "zh", "zh-hant": "zh",
}

func readPackVersions(t *testing.T, path string) packVersions {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var doc struct {
		Lang    string `json:"lang_code"`
		Plural  string `json:"plural_code"`
		Version int32  `json:"version"`
		Fix     int32  `json:"android_xml_fix_version"`
		Strings []struct {
			Key     string `json:"key"`
			Version int32  `json:"version"`
		} `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(path, err)
	}
	out := packVersions{version: doc.Version, fix: doc.Fix, lang: doc.Lang, plural: doc.Plural, strings: make(map[string]int32, len(doc.Strings))}
	for _, s := range doc.Strings {
		out.strings[s.Key] = s.Version
	}
	return out
}

func readPackText(t *testing.T, path string) map[string]string {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var doc struct {
		Strings []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(path, err)
	}
	out := make(map[string]string, len(doc.Strings))
	for _, s := range doc.Strings {
		out[s.Key] = s.Value
	}
	return out
}
