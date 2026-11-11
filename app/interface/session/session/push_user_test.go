package session

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestPushUpdatesUserRoundTrip(t *testing.T) {
	msg := &TLSessionPushUpdatesData{PermAuthKeyId: 469193, Notification: true}
	SetPushUpdatesUser(msg, 11)
	if got := PushUpdatesUser(msg); got != 11 {
		t.Fatalf("in memory user = %d", got)
	}

	raw, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TLSessionPushUpdatesData
	if err = proto.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := PushUpdatesUser(&decoded); got != 11 {
		t.Fatalf("unary user = %d", got)
	}
	if decoded.GetPermAuthKeyId() != 469193 || !decoded.GetNotification() {
		t.Fatalf("payload changed: %+v", &decoded)
	}

	stream := &SessionStreamRequest{
		RequestId: "1",
		Payload:   &SessionStreamRequest_PushUpdates{PushUpdates: msg},
	}
	raw, err = proto.Marshal(stream)
	if err != nil {
		t.Fatal(err)
	}
	var streamOut SessionStreamRequest
	if err = proto.Unmarshal(raw, &streamOut); err != nil {
		t.Fatal(err)
	}
	if got := PushUpdatesUser(streamOut.GetPushUpdates()); got != 11 {
		t.Fatalf("stream user = %d", got)
	}
}

func TestPushSessionUpdatesUserRoundTrip(t *testing.T) {
	msg := &TLSessionPushSessionUpdatesData{PermAuthKeyId: 9, AuthKeyId: 9, SessionId: 3}
	SetPushSessionUpdatesUser(msg, 22)
	raw, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TLSessionPushSessionUpdatesData
	if err = proto.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := PushSessionUpdatesUser(&decoded); got != 22 {
		t.Fatalf("user = %d", got)
	}
	if decoded.GetSessionId() != 3 {
		t.Fatalf("session id = %d", decoded.GetSessionId())
	}
}

func TestPushUpdatesUserZeroClearsStamp(t *testing.T) {
	msg := &TLSessionPushUpdatesData{}
	SetPushUpdatesUser(msg, 11)
	SetPushUpdatesUser(msg, 0)
	if got := PushUpdatesUser(msg); got != 0 {
		t.Fatalf("user = %d", got)
	}
}
