package terms

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
)

func TestDefaultUpdateIsEmpty(t *testing.T) {
	s := New(Config{}, nil)
	now := int32(1_700_000_000)
	got := s.TermsUpdate(context.Background(), 42, now)
	if got.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	if got.GetExpires() != now+EmptyTTL {
		t.Fatalf("expires %d", got.GetExpires())
	}
	if got.GetTermsOfService() != nil {
		t.Fatal("empty update carried a document")
	}
	if err := roundTrip(got); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateShownUntilAccepted(t *testing.T) {
	s := New(Config{UpdateId: "tellomi-tos-2", PopupEnabled: true}, nil)
	ctx := context.Background()
	now := int32(1_700_000_000)
	const userID int64 = 7

	got := s.TermsUpdate(ctx, userID, now)
	if got.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdate {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	doc := got.GetTermsOfService()
	if doc.GetPredicateName() != mtproto.Predicate_help_termsOfService {
		t.Fatalf("terms predicate %s", doc.GetPredicateName())
	}
	if !doc.GetPopup() {
		t.Fatal("popup")
	}
	if doc.GetId().GetData() != `{"id":"tellomi-tos-2"}` {
		t.Fatalf("id %s", doc.GetId().GetData())
	}
	if strings.Contains(doc.GetText(), "https://") {
		t.Fatalf("summary should not be raw urls: %s", doc.GetText())
	}
	assertLinks(t, doc, brand.TermsURL, brand.PrivacyURL)
	if err := roundTrip(got); err != nil {
		t.Fatal(err)
	}

	if err := s.Accept(ctx, userID, `{"id":"tellomi-tos-1"}`); err != nil {
		t.Fatal(err)
	}
	if again := s.TermsUpdate(ctx, userID, now); again.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdate {
		t.Fatalf("other id cleared the update: %s", again.GetPredicateName())
	}

	if err := s.Accept(ctx, userID, doc.GetId().GetData()); err != nil {
		t.Fatal(err)
	}
	if err := s.Accept(ctx, userID, "tellomi-tos-2"); err != nil {
		t.Fatal(err)
	}
	cleared := s.TermsUpdate(ctx, userID, now)
	if cleared.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty || cleared.GetExpires() != now+EmptyTTL {
		t.Fatalf("after accept: %s expires %d", cleared.GetPredicateName(), cleared.GetExpires())
	}
}

func TestSignupDocumentMatchesSummary(t *testing.T) {
	s := New(Config{}, nil)
	doc := s.SignupTerms()
	if doc.GetId().GetData() != `{"id":"tellomi-tos-1"}` {
		t.Fatalf("id %s", doc.GetId().GetData())
	}
	assertLinks(t, doc, brand.TermsURL, brand.PrivacyURL)
	if doc.GetMinAgeConfirm() != nil {
		t.Fatal("default min age")
	}

	s.AcceptSignup(context.Background(), 9)
	forced := New(Config{UpdateId: DefaultID}, s.store)
	got := forced.TermsUpdate(context.Background(), 9, 100)
	if got.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatalf("signup acceptance should cover the same id: %s", got.GetPredicateName())
	}
}

func TestCustomTextAndAge(t *testing.T) {
	s := New(Config{
		Text:          "请先阅读《用户服务协议》，再看《隐私政策》。",
		TermsURL:      "https://example.com/terms",
		PrivacyURL:    "https://example.com/privacy",
		MinAgeConfirm: 16,
		UpdateId:      "v2",
		PopupEnabled:  true,
	}, nil)
	doc := s.TermsUpdate(context.Background(), 1, 50).GetTermsOfService()
	if doc.GetMinAgeConfirm().GetValue() != 16 {
		t.Fatalf("age %v", doc.GetMinAgeConfirm())
	}
	assertLinks(t, doc, "https://example.com/terms", "https://example.com/privacy")
	if doc.GetText() != "请先阅读《用户服务协议》，再看《隐私政策》。" {
		t.Fatalf("text rewritten: %s", doc.GetText())
	}
}

func TestTextWithoutLabelsAppendsLinks(t *testing.T) {
	s := New(Config{Text: "继续使用即表示同意。", UpdateId: "v"}, nil)
	doc := s.SignupTerms()
	if !strings.Contains(doc.GetText(), "用户服务协议") || !strings.Contains(doc.GetText(), "隐私政策") {
		t.Fatalf("labels not appended: %s", doc.GetText())
	}
	assertLinks(t, doc, brand.TermsURL, brand.PrivacyURL)
}

func TestRawURLsInText(t *testing.T) {
	text := "条款见 " + brand.TermsURL + " ，隐私见 " + brand.PrivacyURL
	s := New(Config{Text: text, TermsLabel: "不存在的条款", PrivacyLabel: "不存在的隐私"}, nil)
	doc := s.SignupTerms()
	if doc.GetText() != text {
		t.Fatalf("text %s", doc.GetText())
	}
	ents := doc.GetEntities()
	if len(ents) != 2 {
		t.Fatalf("entities %d", len(ents))
	}
	for _, e := range ents {
		if e.GetPredicateName() != mtproto.Predicate_messageEntityUrl {
			t.Fatalf("predicate %s", e.GetPredicateName())
		}
	}
	if span(doc.GetText(), ents[0]) != brand.TermsURL || span(doc.GetText(), ents[1]) != brand.PrivacyURL {
		t.Fatalf("spans %q %q", span(doc.GetText(), ents[0]), span(doc.GetText(), ents[1]))
	}
}

func TestAcceptWithoutUser(t *testing.T) {
	s := New(Config{UpdateId: "tellomi-tos-2", PopupEnabled: true}, nil)
	if err := s.Accept(context.Background(), 0, `{"id":"tellomi-tos-2"}`); err != nil {
		t.Fatal(err)
	}
	got := s.TermsUpdate(context.Background(), 3, 10)
	if got.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdate {
		t.Fatal("user 0 accept was attributed to someone else")
	}
	if s.TermsUpdate(context.Background(), 0, 10).GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatal("missing user id should not open the popup")
	}
}

func TestStoreErrorDoesNotTrapClient(t *testing.T) {
	s := New(Config{UpdateId: "tellomi-tos-2", PopupEnabled: true}, errStore{})
	now := int32(1_700_000_000)
	got := s.TermsUpdate(context.Background(), 4, now)
	if got.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty || got.GetExpires() != now+retryTTL {
		t.Fatalf("got %s expires %d", got.GetPredicateName(), got.GetExpires())
	}
	if err := s.Accept(context.Background(), 4, "tellomi-tos-2"); err == nil {
		t.Fatal("accept stored despite the database error")
	}
}

func TestParseID(t *testing.T) {
	cases := map[string]string{
		`{"id":"tellomi-tos-2"}`:      "tellomi-tos-2",
		`{"id":"abc","country":"CN"}`: "abc",
		`"plain"`:                     "plain",
		"  tellomi-tos-1  ":           "tellomi-tos-1",
		"":                            "",
		`{"id":"` + strings.Repeat("x", 200) + `"}`: "",
	}
	for in, want := range cases {
		if got := parseID(in); got != want {
			t.Fatalf("parseID(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMigrationMatchesStartupSchema(t *testing.T) {
	b, err := os.ReadFile("../../../../teamgramd/deploy/sql/migrate-20261029-tos.sql")
	if err != nil {
		t.Fatal(err)
	}
	mig := string(b)
	for _, needle := range []string{
		"CREATE TABLE IF NOT EXISTS user_tos_acceptances",
		"user_id BIGINT NOT NULL",
		"tos_id VARCHAR(128) NOT NULL",
		"accepted_at INT NOT NULL",
		"PRIMARY KEY (user_id, tos_id)",
	} {
		if !strings.Contains(mig, needle) || !strings.Contains(schemaSQL, needle) {
			t.Fatalf("schema drift on %q", needle)
		}
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("TELLOMI_TOS_UPDATE_ID", "tellomi-tos-9")
	t.Setenv("TELLOMI_TOS_TERMS_URL", "https://example.com/t")
	cfg := Normalize(applyEnv(Config{}))
	if cfg.UpdateId != "tellomi-tos-9" || cfg.TermsURL != "https://example.com/t" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Id != DefaultID || cfg.PrivacyURL != brand.PrivacyURL {
		t.Fatalf("defaults lost: %+v", cfg)
	}
}

type errStore struct{}

func (errStore) Accepted(context.Context, int64, string) (bool, error) {
	return false, errors.New("db down")
}
func (errStore) Accept(context.Context, int64, string) error {
	return errors.New("db down")
}

func assertLinks(t *testing.T, doc *mtproto.Help_TermsOfService, termsURL, privacyURL string) {
	t.Helper()
	text := doc.GetText()
	if !strings.Contains(text, "用户服务协议") || !strings.Contains(text, "隐私政策") {
		t.Fatalf("summary %s", text)
	}
	ents := doc.GetEntities()
	if len(ents) != 2 {
		t.Fatalf("entities %d", len(ents))
	}
	if ents[0].GetPredicateName() != mtproto.Predicate_messageEntityTextUrl || ents[0].GetUrl() != termsURL {
		t.Fatalf("terms entity %+v", ents[0])
	}
	if ents[1].GetPredicateName() != mtproto.Predicate_messageEntityTextUrl || ents[1].GetUrl() != privacyURL {
		t.Fatalf("privacy entity %+v", ents[1])
	}
	if span(text, ents[0]) != "《用户服务协议》" || span(text, ents[1]) != "《隐私政策》" {
		t.Fatalf("spans %q %q", span(text, ents[0]), span(text, ents[1]))
	}
	if n := mtproto.UTF16Len(text); int(ents[1].GetOffset()+ents[1].GetLength()) > n {
		t.Fatalf("entity past end %d > %d", ents[1].GetOffset()+ents[1].GetLength(), n)
	}
}

func span(text string, e *mtproto.MessageEntity) string {
	u := utf16.Encode([]rune(text))
	off, n := int(e.GetOffset()), int(e.GetLength())
	if off < 0 || n < 0 || off+n > len(u) {
		return ""
	}
	return string(utf16.Decode(u[off : off+n]))
}

func roundTrip(in *mtproto.Help_TermsOfServiceUpdate) error {
	buf := mtproto.NewEncodeBuf(256)
	if err := in.Encode(buf, 232); err != nil {
		return err
	}
	out := &mtproto.Help_TermsOfServiceUpdate{}
	if err := out.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		return err
	}
	if out.GetPredicateName() != in.GetPredicateName() || out.GetExpires() != in.GetExpires() {
		return errors.New("round trip mismatch")
	}
	return nil
}

// Regression: with the default config the server never asks for a popup,
// even when UpdateId is set and unaccepted. Signup terms carry the links.
func TestNoPopupByDefault(t *testing.T) {
	s := New(Config{UpdateId: "tellomi-tos-2"}, nil)
	u := s.TermsUpdate(context.Background(), 42, 1000)
	if u.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatalf("want termsOfServiceUpdateEmpty, got %s", u.GetPredicateName())
	}
	doc := s.SignupTerms()
	if doc.GetPopup() {
		t.Fatal("signup terms must have popup=false by default")
	}
	urls := map[string]bool{}
	for _, e := range doc.GetEntities() {
		urls[e.GetUrl()] = true
	}
	if !urls["https://tellomi.cn/legal/terms/"] || !urls["https://tellomi.cn/legal/privacy/"] {
		t.Fatalf("missing terms/privacy links: %v", urls)
	}
	p := New(Config{UpdateId: "tellomi-tos-2", PopupEnabled: true}, nil)
	if p.TermsUpdate(context.Background(), 42, 1000).GetPredicateName() == mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatal("PopupEnabled=true should still allow the update")
	}
}
