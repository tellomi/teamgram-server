package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestVenueRequestsEncode(t *testing.T) {
	reqs := []mtproto.TLObject{
		&mtproto.TLMessagesGetInlineBotResults{
			Constructor: mtproto.CRC32_messages_getInlineBotResults,
			Bot:         mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: mtproto.BotVenueId, AccessHash: 1}).To_InputUser(),
			Peer:        inputPeerSelf(),
			GeoPoint:    mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint(),
			Query:       "咖啡",
		},
		&mtproto.TLUploadGetWebFile{
			Constructor: mtproto.CRC32_upload_getWebFile,
			Location: mtproto.MakeTLInputWebFileGeoPointLocation(&mtproto.InputWebFileLocation{
				GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint(),
				W:        200, H: 120, Zoom: 15, Scale: 1,
			}).To_InputWebFileLocation(),
			Limit: 1024,
		},
	}
	for _, req := range reqs {
		raw, err := encodeTL(req)
		if err != nil {
			t.Fatal(err)
		}
		obj, err := decodeTL(raw)
		if err != nil {
			t.Fatalf("%T decode: %v", req, err)
		}
		if obj == nil {
			t.Fatalf("%T decoded nil", req)
		}
	}
}

func TestVenueConfigAndWebFile(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	obj := invokeOK(t, a, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	cfg, ok := obj.(*mtproto.TLConfig)
	if !ok {
		t.Fatalf("help.getConfig returned %T", obj)
	}
	if cfg.GetVenueSearchUsername().GetValue() != "venue" {
		t.Fatalf("venue_search_username %q", cfg.GetVenueSearchUsername().GetValue())
	}
	if cfg.GetWebfileDcId() != 1 {
		t.Fatalf("webfile_dc_id %d", cfg.GetWebfileDcId())
	}

	c, cancel := ctx()
	defer cancel()
	resolved, err := a.Invoke(c, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    "venue",
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, ok := resolved.(*mtproto.TLContactsResolvedPeer)
	if !ok {
		t.Fatalf("resolve %T", resolved)
	}
	botID := mtproto.BotVenueId
	access := int64(0)
	if peer.GetPeer().GetUserId() == botID {
		for _, u := range peer.GetUsers() {
			if u.GetId() == botID {
				access = u.GetAccessHash().GetValue()
				if !u.GetBot() || u.GetUsername().GetValue() != "venue" {
					t.Fatalf("bot username %q bot %v", u.GetUsername().GetValue(), u.GetBot())
				}
			}
		}
	}

	c, cancel = ctx()
	defer cancel()
	results, err := a.Invoke(c, &mtproto.TLMessagesGetInlineBotResults{
		Constructor: mtproto.CRC32_messages_getInlineBotResults,
		Bot: mtproto.MakeTLInputUser(&mtproto.InputUser{
			UserId:     botID,
			AccessHash: access,
		}).To_InputUser(),
		Peer: inputPeerSelf(),
		GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{
			Lat:  39.9087,
			Long: 116.3975,
		}).To_InputGeoPoint(),
		Query:  "咖啡",
		Offset: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	botResults, ok := results.(*mtproto.TLMessagesBotResults)
	if !ok {
		t.Fatalf("inline %T", results)
	}
	for _, row := range botResults.GetResults() {
		if row.GetType() != "venue" || row.GetSendMessage().GetPredicateName() != mtproto.Predicate_botInlineMessageMediaVenue {
			t.Fatalf("row type %s send %s", row.GetType(), row.GetSendMessage().GetPredicateName())
		}
		provider := row.GetSendMessage().GetProvider()
		if provider != "amap" && provider != "tencent" {
			t.Fatalf("provider %s", provider)
		}
	}

	c, cancel = ctx()
	defer cancel()
	web, err := a.Invoke(c, &mtproto.TLUploadGetWebFile{
		Constructor: mtproto.CRC32_upload_getWebFile,
		Location: mtproto.MakeTLInputWebFileGeoPointLocation(&mtproto.InputWebFileLocation{
			GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{
				Lat:  39.9087,
				Long: 116.3975,
			}).To_InputGeoPoint(),
			W:     200,
			H:     120,
			Zoom:  15,
			Scale: 1,
		}).To_InputWebFileLocation(),
		Offset: 0,
		Limit:  128 * 1024,
	})
	if err == nil {
		if _, ok := web.(*mtproto.TLUploadWebFile); !ok {
			t.Fatalf("webfile %T", web)
		}
		return
	}
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "WEBFILE_NOT_AVAILABLE" {
		t.Fatal(err)
	}
}
