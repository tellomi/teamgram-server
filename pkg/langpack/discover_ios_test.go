package langpack

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// iosDiscoverRow is one Discover or sign-up string in English, simplified
// Chinese, and traditional Chinese.
type iosDiscoverRow struct {
	key  string
	en   string
	hans string
	hant string
}

func (r iosDiscoverRow) text(code string) string {
	switch code {
	case "zh-hans":
		return r.hans
	case "zh-hant":
		return r.hant
	default:
		return r.en
	}
}

// iosDiscoverSignUp is the Discover and sign-up avatar copy added for iOS.
// English and simplified Chinese match the key table. Traditional Chinese
// matches Telegram-iOS zh-Hant.lproj on cursor/discover-i18n-report-signup-27fd.
var iosDiscoverSignUp = []iosDiscoverRow{
	{"Tellomi.Discover.Recommend", "For You", "推荐", "推薦"},
	{"Tellomi.Discover.Following", "Following", "关注", "追蹤"},
	{"Tellomi.Discover.Nearby", "Nearby", "同城", "同城"},
	{"Tellomi.Discover.Compose", "Post", "发布", "發布"},
	{"Tellomi.Discover.EmptyTitle", "No posts yet", "还没有帖子", "還沒有貼文"},
	{"Tellomi.Discover.EmptyText", "Posts from people you follow, and recommendations, show up here.", "关注的人和推荐会显示在这里。", "追蹤的人和推薦會顯示在這裡。"},
	{"Tellomi.Discover.Unavailable", "This post isn't available", "这条动态已不可见", "這則動態已不可見"},
	{"Tellomi.Discover.Failed", "Couldn't load this. Pull to try again.", "暂时打不开，下拉再试一次。", "暫時打不開，下拉再試一次。"},
	{"Tellomi.Discover.CityPrompt", "Choose a city", "选择城市", "選擇城市"},
	{"Tellomi.Discover.CityPlaceholder", "City", "输入城市", "輸入城市"},
	{"Tellomi.Discover.UseLocation", "Use current location", "使用当前位置", "使用目前位置"},
	{"Tellomi.Discover.ConfirmCity", "Done", "确定", "確定"},
	{"Tellomi.Discover.CityDenied", "You can pick a city without location. Only the city is kept.", "可以不定位，直接从列表里选。只保存城市，不保存坐标。", "可以不定位，直接從列表裡選。只保存城市，不保存座標。"},
	{"Tellomi.Discover.Visibility", "Who can see this", "谁可以看", "誰可以看"},
	{"Tellomi.Discover.Everyone", "Everyone", "所有人", "所有人"},
	{"Tellomi.Discover.Contacts", "My Contacts", "我的联系人", "我的聯絡人"},
	{"Tellomi.Discover.CloseFriends", "Close Friends", "密友", "密友"},
	{"Tellomi.Discover.Selected", "Selected People", "选定的用户", "指定的使用者"},
	{"Tellomi.Discover.Exclude", "Exclude", "不给谁看", "不給誰看"},
	{"Tellomi.Discover.AudienceInvalid", "That list can't be used. Choose again.", "名单无效，请重新选择。", "名單無效，請重新選擇。"},
	{"Tellomi.Discover.UserInvalid", "Someone on the list can't be added.", "名单里有人无法使用。", "名單裡有人無法使用。"},
	{"Tellomi.Discover.UserBlocked", "Someone on the list is blocked.", "名单里有人已互相拉黑。", "名單裡有人已互相封鎖。"},
	{"Tellomi.Discover.Disabled", "Discover isn't available right now.", "发现暂时不可用。", "發現暫時無法使用。"},
	{"Tellomi.Discover.NearbyEmpty", "No posts in this city yet.", "这个城市还没有帖子。", "這個城市還沒有貼文。"},
	{"Tellomi.Discover.InvalidCity", "That city can't be used. Choose another.", "这个城市不能用，请重新选择。", "這個城市不能用，請重新選擇。"},
	{"Tellomi.Discover.AiGenerated", "AI generated", "AI 生成", "AI 生成"},
	{"Tellomi.Discover.QuoteDeleted", "The original post was deleted by its author", "原动态已被作者删除", "原動態已被作者刪除"},
	{"Tellomi.Discover.Publish", "Post", "发布", "發布"},
	{"Tellomi.Discover.TellomiId", "Tellomi ID: %@", "Tellomi 号：%@", "Tellomi 號：%@"},
	{"Tellomi.Discover.Likes", "%d likes", "%d 赞", "%d 讚"},
	{"Tellomi.Discover.Comments", "%d comments", "%d 评论", "%d 則留言"},
	{"Tellomi.Discover.Photos", "%d photos", "%d 张图片", "%d 張圖片"},
	{"Tellomi.Discover.Feed", "Discover", "发现", "發現"},
	{"Tellomi.Discover.HideFeed", "Hide Discover", "隐藏发现", "隱藏發現"},
	{"Tellomi.Discover.HideContacts", "Hide Contacts", "隐藏联系人", "隱藏聯絡人"},
	{"Tellomi.Discover.NavigationTitle", "Navigation Bar", "导航栏", "導覽列"},
	{"Tellomi.Discover.NavigationFooter", "Chats and Settings always stay in the bar.", "聊天和设置始终显示，不能关闭。", "聊天和設定始終顯示，不能關閉。"},
	{"Tellomi.Discover.FeedEmptyTitle", "Nothing here yet", "还没有内容", "還沒有內容"},
	{"Tellomi.Discover.FeedEmptyText", "Posts will show up here.", "帖子会显示在这里。", "貼文會顯示在這裡。"},
	{"Tellomi.Discover.FeedPostUnavailable", "This post can't be opened yet.", "这条帖子暂时无法打开。", "這則貼文暫時無法打開。"},
	{"Tellomi.SignUp.AvatarFailed", "Couldn't upload the profile photo (%@)", "头像上传失败（%@）", "頭像上傳失敗（%@）"},
	{"Tellomi.SignUp.AvatarRetry", "Retry", "重试", "重試"},
	{"Tellomi.Discover.City.cn_beijing", "Beijing", "北京", "北京"},
	{"Tellomi.Discover.City.cn_shanghai", "Shanghai", "上海", "上海"},
	{"Tellomi.Discover.City.cn_tianjin", "Tianjin", "天津", "天津"},
	{"Tellomi.Discover.City.cn_chongqing", "Chongqing", "重庆", "重慶"},
	{"Tellomi.Discover.City.cn_chengdu", "Chengdu", "成都", "成都"},
	{"Tellomi.Discover.City.cn_hangzhou", "Hangzhou", "杭州", "杭州"},
	{"Tellomi.Discover.City.cn_ningbo", "Ningbo", "宁波", "寧波"},
	{"Tellomi.Discover.City.cn_nanjing", "Nanjing", "南京", "南京"},
	{"Tellomi.Discover.City.cn_suzhou", "Suzhou", "苏州", "蘇州"},
	{"Tellomi.Discover.City.cn_wuxi", "Wuxi", "无锡", "無錫"},
	{"Tellomi.Discover.City.cn_wuhan", "Wuhan", "武汉", "武漢"},
	{"Tellomi.Discover.City.cn_xian", "Xi'an", "西安", "西安"},
	{"Tellomi.Discover.City.cn_guangzhou", "Guangzhou", "广州", "廣州"},
	{"Tellomi.Discover.City.cn_shenzhen", "Shenzhen", "深圳", "深圳"},
	{"Tellomi.Discover.City.cn_zhuhai", "Zhuhai", "珠海", "珠海"},
	{"Tellomi.Discover.City.cn_dongguan", "Dongguan", "东莞", "東莞"},
	{"Tellomi.Discover.City.cn_foshan", "Foshan", "佛山", "佛山"},
	{"Tellomi.Discover.City.cn_changsha", "Changsha", "长沙", "長沙"},
	{"Tellomi.Discover.City.cn_zhengzhou", "Zhengzhou", "郑州", "鄭州"},
	{"Tellomi.Discover.City.cn_qingdao", "Qingdao", "青岛", "青島"},
	{"Tellomi.Discover.City.cn_jinan", "Jinan", "济南", "濟南"},
	{"Tellomi.Discover.City.cn_hefei", "Hefei", "合肥", "合肥"},
	{"Tellomi.Discover.City.cn_fuzhou", "Fuzhou", "福州", "福州"},
	{"Tellomi.Discover.City.cn_xiamen", "Xiamen", "厦门", "廈門"},
	{"Tellomi.Discover.City.cn_kunming", "Kunming", "昆明", "昆明"},
	{"Tellomi.Discover.City.cn_guiyang", "Guiyang", "贵阳", "貴陽"},
	{"Tellomi.Discover.City.cn_nanning", "Nanning", "南宁", "南寧"},
	{"Tellomi.Discover.City.cn_haikou", "Haikou", "海口", "海口"},
	{"Tellomi.Discover.City.cn_sanya", "Sanya", "三亚", "三亞"},
	{"Tellomi.Discover.City.cn_shenyang", "Shenyang", "沈阳", "瀋陽"},
	{"Tellomi.Discover.City.cn_dalian", "Dalian", "大连", "大連"},
	{"Tellomi.Discover.City.cn_harbin", "Harbin", "哈尔滨", "哈爾濱"},
	{"Tellomi.Discover.City.cn_changchun", "Changchun", "长春", "長春"},
	{"Tellomi.Discover.City.cn_shijiazhuang", "Shijiazhuang", "石家庄", "石家莊"},
	{"Tellomi.Discover.City.cn_taiyuan", "Taiyuan", "太原", "太原"},
	{"Tellomi.Discover.City.cn_nanchang", "Nanchang", "南昌", "南昌"},
	{"Tellomi.Discover.City.cn_lanzhou", "Lanzhou", "兰州", "蘭州"},
	{"Tellomi.Discover.City.cn_yinchuan", "Yinchuan", "银川", "銀川"},
	{"Tellomi.Discover.City.cn_xining", "Xining", "西宁", "西寧"},
	{"Tellomi.Discover.City.cn_urumqi", "Urumqi", "乌鲁木齐", "烏魯木齊"},
	{"Tellomi.Discover.City.cn_hohhot", "Hohhot", "呼和浩特", "呼和浩特"},
	{"Tellomi.Discover.City.cn_lhasa", "Lhasa", "拉萨", "拉薩"},
	{"Tellomi.Discover.City.cn_hong-kong", "Hong Kong", "香港", "香港"},
	{"Tellomi.Discover.City.cn_macau", "Macau", "澳门", "澳門"},
	{"Tellomi.Discover.City.cn_taipei", "Taipei", "台北", "台北"},
	{"Tellomi.Discover.City.cn_kaohsiung", "Kaohsiung", "高雄", "高雄"},
	{"Tellomi.Discover.City.cn_taichung", "Taichung", "台中", "台中"},
	{"Tellomi.Discover.City.cn_tainan", "Tainan", "台南", "台南"},
	{"Tellomi.Discover.City.us_new-york", "New York", "纽约", "紐約"},
	{"Tellomi.Discover.City.us_los-angeles", "Los Angeles", "洛杉矶", "洛杉磯"},
	{"Tellomi.Discover.City.us_san-francisco", "San Francisco", "旧金山", "舊金山"},
	{"Tellomi.Discover.City.gb_london", "London", "伦敦", "倫敦"},
	{"Tellomi.Discover.City.jp_tokyo", "Tokyo", "东京", "東京"},
	{"Tellomi.Discover.City.jp_osaka", "Osaka", "大阪", "大阪"},
	{"Tellomi.Discover.City.kr_seoul", "Seoul", "首尔", "首爾"},
	{"Tellomi.Discover.City.sg_singapore", "Singapore", "新加坡", "新加坡"},
	{"Tellomi.Discover.City.fr_paris", "Paris", "巴黎", "巴黎"},
	{"Tellomi.Discover.City.de_berlin", "Berlin", "柏林", "柏林"},
	{"Tellomi.Discover.City.au_sydney", "Sydney", "悉尼", "悉尼"},
	{"Tellomi.Discover.City.ca_toronto", "Toronto", "多伦多", "多倫多"},
	{"Tellomi.Discover.City.th_bangkok", "Bangkok", "曼谷", "曼谷"},
}

func TestIOSDiscoverSignUpKeysResolve(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	// The key table has 103 rows. Tellomi. occurs 104 times in the note because
	// the prose also says Tellomi.Discover.*. AvatarFailed and AvatarRetry are
	// both here. The iOS strings files and the city catalog have no extra key.
	if len(iosDiscoverSignUp) != 103 {
		t.Fatalf("keys %d", len(iosDiscoverSignUp))
	}
	for _, tc := range []struct {
		code    string
		prev    int32
		version int32
	}{
		{"en", 22347344, 22347345},
		{"zh-hans", 22454344, 22454345},
		{"zh-hant", 22421529, 22421530},
	} {
		ver, ok := cat.Version("ios", tc.code)
		if !ok || ver != tc.version {
			t.Fatalf("%s version %d %v", tc.code, ver, ok)
		}
		want := map[string]string{}
		keys := make([]string, 0, len(iosDiscoverSignUp))
		for _, row := range iosDiscoverSignUp {
			want[row.key] = row.text(tc.code)
			keys = append(keys, row.key)
		}
		got, err := cat.Strings("ios", tc.code, keys)
		if err != nil {
			t.Fatal(tc.code, err)
		}
		if len(got) != len(keys) {
			t.Fatalf("%s strings %d", tc.code, len(got))
		}
		for i, s := range got {
			if s.GetPredicateName() != mtproto.Predicate_langPackString || s.GetKey() != keys[i] || s.GetValue() != want[keys[i]] {
				t.Fatalf("%s %s got %q (%s) want %q", tc.code, keys[i], s.GetValue(), s.GetPredicateName(), want[keys[i]])
			}
		}
		diff, err := cat.Difference("ios", tc.code, tc.prev)
		if err != nil {
			t.Fatal(tc.code, err)
		}
		if diff.GetVersion() != tc.version || len(diff.GetStrings()) != len(want) {
			t.Fatalf("%s difference v=%d n=%d", tc.code, diff.GetVersion(), len(diff.GetStrings()))
		}
		seen := map[string]string{}
		for _, s := range diff.GetStrings() {
			if s.GetPredicateName() != mtproto.Predicate_langPackString {
				t.Fatalf("%s %s predicate %s", tc.code, s.GetKey(), s.GetPredicateName())
			}
			seen[s.GetKey()] = s.GetValue()
		}
		for key, value := range want {
			if seen[key] != value {
				t.Fatalf("%s refresh %s got %q want %q", tc.code, key, seen[key], value)
			}
		}
		caught, err := cat.Difference("ios", tc.code, tc.version)
		if err != nil || len(caught.GetStrings()) != 0 || caught.GetVersion() != tc.version {
			t.Fatalf("%s caught up %+v %v", tc.code, caught, err)
		}
		override := readOverride(t, "../../data/langpack/overrides/ios/"+tc.code+".json.gz")
		for key, value := range want {
			entry, ok := override[key]
			if !ok || entry.value != value || entry.version != tc.version {
				t.Fatalf("override %s %s %+v", tc.code, key, entry)
			}
		}
	}

	// Other languages omit the keys. The client loads base_lang_code (en).
	for _, row := range iosDiscoverSignUp {
		got, ok := cat.Plain("ios", "ja", row.key)
		if !ok || got != row.en {
			t.Fatalf("ja fallback %s %q %v", row.key, got, ok)
		}
		rows, err := cat.Strings("ios", "ja", []string{row.key})
		if err != nil || len(rows) != 1 || rows[0].GetPredicateName() != mtproto.Predicate_langPackStringDeleted {
			t.Fatalf("ja pack should omit %s %+v %v", row.key, rows, err)
		}
	}
}

type overrideEntry struct {
	value   string
	version int32
}

func readOverride(t *testing.T, path string) map[string]overrideEntry {
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
			Key     string `json:"key"`
			Value   string `json:"value"`
			Version int32  `json:"version"`
		} `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	out := make(map[string]overrideEntry, len(doc.Strings))
	for _, s := range doc.Strings {
		out[s.Key] = overrideEntry{value: s.Value, version: s.Version}
	}
	return out
}
