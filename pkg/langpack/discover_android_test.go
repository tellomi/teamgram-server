package langpack

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// androidDiscoverReport is Android-only. Traditional Chinese is OpenCC s2tw
// of the simplified text. iOS keeps Telegram's own report sheet strings.
var androidDiscoverReport = []iosDiscoverRow{
	{"Tellomi.Discover.Report", "Report", "举报", "舉報"},
	{"Tellomi.Discover.ReportSent", "Report sent", "举报已发送", "舉報已發送"},
	{"Tellomi.Discover.ReportFailed", "Couldn't send the report", "举报没有发出去", "舉報沒有發出去"},
	{"Tellomi.Discover.Report.Spam", "Spam", "垃圾信息", "垃圾信息"},
	{"Tellomi.Discover.Report.Violence", "Violence", "暴力", "暴力"},
	{"Tellomi.Discover.Report.Pornography", "Pornography", "色情", "色情"},
	{"Tellomi.Discover.Report.Child", "Child abuse", "虐待儿童", "虐待兒童"},
	{"Tellomi.Discover.Report.Drugs", "Illegal drugs", "毒品", "毒品"},
	{"Tellomi.Discover.Report.Personal", "Personal details", "侵犯隐私", "侵犯隱私"},
	{"Tellomi.Discover.Report.Other", "Other", "其他", "其他"},
}

func androidDiscoverRows() []iosDiscoverRow {
	rows := make([]iosDiscoverRow, 0, len(iosDiscoverSignUp)+len(androidDiscoverReport))
	rows = append(rows, iosDiscoverSignUp...)
	rows = append(rows, androidDiscoverReport...)
	return rows
}

func TestAndroidDiscoverSignUpKeysResolve(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	rows := androidDiscoverRows()
	if len(iosDiscoverSignUp) != 103 || len(androidDiscoverReport) != 10 || len(rows) != 113 {
		t.Fatalf("shared %d report %d total %d", len(iosDiscoverSignUp), len(androidDiscoverReport), len(rows))
	}
	for _, tc := range []struct {
		code    string
		prev    int32
		version int32
	}{
		{"en", 61510239, 61510240},
		{"zh-hans", 61570951, 61570952},
		{"zh-hant", 61519201, 61519202},
	} {
		ver, ok := cat.Version("android", tc.code)
		if !ok || ver != tc.version {
			t.Fatalf("%s version %d %v", tc.code, ver, ok)
		}
		want := map[string]string{}
		keys := make([]string, 0, len(rows))
		for _, row := range rows {
			want[row.key] = row.text(tc.code)
			keys = append(keys, row.key)
		}
		got, err := cat.Strings("android", tc.code, keys)
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
		diff, err := cat.Difference("android", tc.code, tc.prev)
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
		caught, err := cat.Difference("android", tc.code, tc.version)
		if err != nil || len(caught.GetStrings()) != 0 || caught.GetVersion() != tc.version {
			t.Fatalf("%s caught up %+v %v", tc.code, caught, err)
		}
		override := readOverride(t, "../../data/langpack/overrides/android/"+tc.code+".json.gz")
		for key, value := range want {
			entry, ok := override[key]
			if !ok || entry.value != value || entry.version != tc.version {
				t.Fatalf("override %s %s %+v", tc.code, key, entry)
			}
		}
	}

	for _, row := range androidDiscoverReport {
		iosRows, err := cat.Strings("ios", "en", []string{row.key})
		if err != nil || len(iosRows) != 1 || iosRows[0].GetPredicateName() != mtproto.Predicate_langPackStringDeleted {
			t.Fatalf("ios should omit android-only %s %+v %v", row.key, iosRows, err)
		}
	}
	for _, row := range rows {
		got, ok := cat.Plain("android", "ja", row.key)
		if !ok || got != row.en {
			t.Fatalf("ja fallback %s %q %v", row.key, got, ok)
		}
		packed, err := cat.Strings("android", "ja", []string{row.key})
		if err != nil || len(packed) != 1 || packed[0].GetPredicateName() != mtproto.Predicate_langPackStringDeleted {
			t.Fatalf("ja pack should omit %s %+v %v", row.key, packed, err)
		}
	}
}
