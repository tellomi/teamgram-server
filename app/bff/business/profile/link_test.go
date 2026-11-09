package profile

import (
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/brand"
)

func TestBusinessChatLinkUsesBrandHost(t *testing.T) {
	t.Setenv("TELLOMI_LINK_BASE_URL", "")
	t.Setenv("TEAMGRAM_TME_HOST", "")
	got := linkTL(&linkRow{slug: "abc", message: "hi"})
	if got.GetLink() != "https://tell.cc/m/abc" {
		t.Fatal(got.GetLink())
	}
	if strings.Contains(got.GetLink(), "t.me") {
		t.Fatal(got.GetLink())
	}
	t.Setenv("TELLOMI_LINK_BASE_URL", "https://links.example")
	got = linkTL(&linkRow{slug: "abc", message: "hi"})
	if got.GetLink() != brand.LinkBase()+"/m/abc" || got.GetLink() != "https://links.example/m/abc" {
		t.Fatal(got.GetLink())
	}
}
