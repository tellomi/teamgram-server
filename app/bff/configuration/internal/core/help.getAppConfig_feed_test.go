package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/appconfig"
)

func TestGetAppConfigConstructorsMatchAndCache(t *testing.T) {
	t.Setenv(appconfig.EnvDiscoverFeed, "")
	t.Setenv(appconfig.EnvDiscoverFeedURL, "")
	c := New(context.Background(), nil)

	hashed, err := c.HelpGetAppConfig61E3F854(&mtproto.TLHelpGetAppConfig61E3F854{})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := c.HelpGetAppConfig98914110(&mtproto.TLHelpGetAppConfig98914110{})
	if err != nil {
		t.Fatal(err)
	}
	if hashed.GetPredicateName() != mtproto.Predicate_help_appConfig {
		t.Fatalf("predicate %s", hashed.GetPredicateName())
	}
	if !sameJSON(hashed.GetConfig(), plain) {
		t.Fatal("the two getAppConfig constructors differ")
	}
	cached, err := c.HelpGetAppConfig61E3F854(&mtproto.TLHelpGetAppConfig61E3F854{Hash: hashed.GetHash()})
	if err != nil {
		t.Fatal(err)
	}
	if cached.GetPredicateName() != mtproto.Predicate_help_appConfigNotModified {
		t.Fatalf("cached predicate %s", cached.GetPredicateName())
	}

	t.Setenv(appconfig.EnvDiscoverFeed, "1")
	t.Setenv(appconfig.EnvDiscoverFeedURL, "https://10.0.2.2:8443")
	again, err := c.HelpGetAppConfig61E3F854(&mtproto.TLHelpGetAppConfig61E3F854{Hash: hashed.GetHash()})
	if err != nil {
		t.Fatal(err)
	}
	if again.GetPredicateName() != mtproto.Predicate_help_appConfig || again.GetHash() == hashed.GetHash() {
		t.Fatal("a changed feed URL was treated as not modified")
	}
	plain = nil
	plain, err = c.HelpGetAppConfig98914110(&mtproto.TLHelpGetAppConfig98914110{})
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(again.GetConfig(), plain) {
		t.Fatal("constructors differ after the feed is published")
	}
	obj, _ := mtproto.FromJSONValue(plain).(map[string]any)
	if obj["tellomi_discover_feed_enabled"] != true || obj["tellomi_discover_feed_base_url"] != "https://10.0.2.2:8443" {
		t.Fatalf("published %#v %#v", obj["tellomi_discover_feed_enabled"], obj["tellomi_discover_feed_base_url"])
	}
}

func sameJSON(a, b *mtproto.JSONValue) bool {
	left, okL := mtproto.FromJSONValue(a).(map[string]any)
	right, okR := mtproto.FromJSONValue(b).(map[string]any)
	if !okL || !okR || len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if !jsonEqual(value, right[key]) {
			return false
		}
	}
	return true
}

func jsonEqual(a, b any) bool {
	switch av := a.(type) {
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
