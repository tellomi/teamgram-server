package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// TestDialogFiltersIncludeDefault locks messages.getDialogFilters to the
// official shape: exactly one dialogFilterDefault when the account has no
// folders, tags_enabled left as stored, and a new folder appended after it.
func TestDialogFiltersIncludeDefault(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "folders", "Folders")

	clearCustomFilters(t, c)
	t.Cleanup(func() { clearCustomFilters(t, c) })

	first := getDialogFilters(t, c)
	assertOnlyDefault(t, first)
	if first.GetTagsEnabled() {
		t.Fatal("tags_enabled is set on an account that never toggled folder tags")
	}

	ok := invokeOK(t, c, &mtproto.TLMessagesUpdateDialogFilter{
		Constructor: mtproto.CRC32_messages_updateDialogFilter,
		Id:          2,
		Filter: mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
			Id:       2,
			Contacts: true,
			Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
				Text:     "People",
				Entities: []*mtproto.MessageEntity{},
			}).To_TextWithEntities(),
		}).To_DialogFilter(),
	})
	if _, isTrue := ok.(*mtproto.TLBoolTrue); !isTrue {
		t.Fatalf("updateDialogFilter returned %T", ok)
	}

	second := getDialogFilters(t, c)
	if second.GetTagsEnabled() {
		t.Fatal("tags_enabled changed after updateDialogFilter")
	}
	filters := second.GetFilters()
	if len(filters) != 2 {
		t.Fatalf("filters %d", len(filters))
	}
	if filters[0].GetPredicateName() != mtproto.Predicate_dialogFilterDefault {
		t.Fatalf("first %s", filters[0].GetPredicateName())
	}
	if filters[1].GetPredicateName() != mtproto.Predicate_dialogFilter || filters[1].GetId() != 2 {
		t.Fatalf("second %s id %d", filters[1].GetPredicateName(), filters[1].GetId())
	}

	clearCustomFilters(t, c)
	third := getDialogFilters(t, c)
	assertOnlyDefault(t, third)
	if third.GetTagsEnabled() {
		t.Fatal("tags_enabled changed after deleting the folder")
	}
}

func getDialogFilters(t *testing.T, c *Client) *mtproto.TLMessagesDialogFilters {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLMessagesGetDialogFiltersEFD48C89{
		Constructor: mtproto.CRC32_messages_getDialogFiltersEFD48C89,
	})
	got, ok := obj.(*mtproto.TLMessagesDialogFilters)
	if !ok {
		t.Fatalf("messages.getDialogFilters returned %T (%s)", obj, predicateName(obj))
	}
	if got.GetPredicateName() != mtproto.Predicate_messages_dialogFilters {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	return got
}

func assertOnlyDefault(t *testing.T, got *mtproto.TLMessagesDialogFilters) {
	t.Helper()
	filters := got.GetFilters()
	if len(filters) != 1 || filters[0].GetPredicateName() != mtproto.Predicate_dialogFilterDefault {
		names := make([]string, 0, len(filters))
		for _, filter := range filters {
			names = append(names, filter.GetPredicateName())
		}
		t.Fatalf("filters %v", names)
	}
}

func clearCustomFilters(t *testing.T, c *Client) {
	t.Helper()
	current := getDialogFilters(t, c)
	for _, filter := range current.GetFilters() {
		if filter.GetPredicateName() == mtproto.Predicate_dialogFilterDefault || filter.GetId() == 0 {
			continue
		}
		obj := invokeOK(t, c, &mtproto.TLMessagesUpdateDialogFilter{
			Constructor: mtproto.CRC32_messages_updateDialogFilter,
			Id:          filter.GetId(),
		})
		if _, ok := obj.(*mtproto.TLBoolTrue); !ok {
			t.Fatalf("delete filter %d returned %T", filter.GetId(), obj)
		}
	}
}
