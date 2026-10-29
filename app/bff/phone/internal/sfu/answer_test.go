package sfu

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnswerMatchesTgcallsShape(t *testing.T) {
	raw := `{"ssrc":12345,"ufrag":"clientu","pwd":"clientpasswordvalue","fingerprints":[{"hash":"sha-256","fingerprint":"AA:BB","setup":"passive"}],"ssrc-groups":[{"semantics":"SIM","sources":[10,11]},{"semantics":"FID","sources":[10,12]}]}`
	offer, err := ParseOffer(raw)
	if err != nil {
		t.Fatal(err)
	}
	if offer.AudioSSRC != 12345 || offer.Ufrag != "clientu" || len(offer.VideoGroups) != 2 {
		t.Fatalf("%+v", offer)
	}
	answer, err := AnswerJSON(offer, Local{
		Ufrag: "sfuufrag", Pwd: "sfupasswordvaluehere", Fingerprint: "CC:DD",
		IP: "203.0.113.5", Port: 3480, Endpoint: "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(answer), &got); err != nil {
		t.Fatal(err)
	}
	tr := got["transport"].(map[string]any)
	if tr["ufrag"] != "sfuufrag" || tr["pwd"] != "sfupasswordvaluehere" {
		t.Fatalf("transport %+v", tr)
	}
	fps := tr["fingerprints"].([]any)
	fp := fps[0].(map[string]any)
	if fp["setup"] != "active" || fp["hash"] != "sha-256" || fp["fingerprint"] != "CC:DD" {
		t.Fatalf("fingerprint %+v", fp)
	}
	cands := tr["candidates"].([]any)
	c := cands[0].(map[string]any)
	if c["ip"] != "203.0.113.5" || c["port"] != "3480" || c["protocol"] != "udp" || c["type"] != "host" {
		t.Fatalf("candidate %+v", c)
	}
	video := got["video"].(map[string]any)
	if video["endpoint"] != "7" || !strings.Contains(answer, "H264") || !strings.Contains(answer, "payload-types") {
		t.Fatalf("video %+v", video)
	}
}

func TestAudioOnlyOmitsVideo(t *testing.T) {
	offer, err := ParseOffer(`{"ssrc":9,"ufrag":"u","pwd":"p","fingerprints":[{"hash":"sha-256","fingerprint":"AA"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := AnswerJSON(offer, Local{Ufrag: "a", Pwd: "b", Fingerprint: "c", IP: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answer, `"video"`) {
		t.Fatal(answer)
	}
}
