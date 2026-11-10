package catalog

import "testing"

func TestPublicObjectRoutesOfficialPacks(t *testing.T) {
	s := newService([]*Pack{{
		ID: 1,
		Items: []*Item{{
			DocumentID: 42, StorageClass: "stickers", ObjectPath: "42.dat",
		}},
	}, {
		ID: 2, Owner: 9,
		Items: []*Item{{
			DocumentID: 43, StorageClass: "stickers", ObjectPath: "43.dat",
		}},
	}, {
		ID: 3,
		Items: []*Item{{
			DocumentID: 44, StorageClass: "emoji", ObjectPath: "",
		}},
	}})
	setCurrent(s)
	t.Cleanup(func() { setCurrent(nil) })

	class, path, ok := PublicObject(42)
	if !ok || class != "stickers" || path != "42.dat" {
		t.Fatalf("sticker %s %s %v", class, path, ok)
	}
	if _, _, ok = PublicObject(43); ok {
		t.Fatal("user pack was routed to the public bucket")
	}
	class, path, ok = PublicObject(44)
	if !ok || class != "emoji" || path != "44.dat" {
		t.Fatalf("emoji %s %s %v", class, path, ok)
	}
	if _, _, ok = PublicObject(0); ok {
		t.Fatal("id 0")
	}
}

func TestIndexPublicRowsSkipsOwners(t *testing.T) {
	idx := indexPublicRows([]publicRow{
		{DocumentID: 1, Class: "stickers", Path: "1.dat", Owner: 0},
		{DocumentID: 1, Class: "emoji", Path: "other.dat", Owner: 0},
		{DocumentID: 2, Class: "stickers", Path: "2.dat", Owner: 8},
		{DocumentID: 3, Class: "documents", Path: "3.dat"},
		{DocumentID: 4, Class: "emoji", Path: ""},
	})
	if idx[1].Class != "stickers" || idx[1].Path != "1.dat" {
		t.Fatalf("first location %#v", idx[1])
	}
	if _, ok := idx[2]; ok {
		t.Fatal("owned row indexed")
	}
	if _, ok := idx[3]; ok {
		t.Fatal("documents class indexed")
	}
	if idx[4].Path != "4.dat" {
		t.Fatalf("path %q", idx[4].Path)
	}
}
