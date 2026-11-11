package session

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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
	msg.UserId = 0
	if got := PushUpdatesUser(msg); got != 0 {
		t.Fatalf("user = %d", got)
	}
}

func TestPushUserFieldsAreInTheDescriptor(t *testing.T) {
	assertUserField(t, (&TLSessionPushUpdatesData{}).ProtoReflect().Descriptor(), 6)
	assertUserField(t, (&TLSessionPushSessionUpdatesData{}).ProtoReflect().Descriptor(), 7)
}

func assertUserField(t *testing.T, md protoreflect.MessageDescriptor, number protoreflect.FieldNumber) {
	t.Helper()
	fd := md.Fields().ByName("user_id")
	if fd == nil {
		t.Fatalf("%s has no user_id field", md.Name())
	}
	if fd.Number() != number {
		t.Fatalf("%s user_id number = %d, want %d", md.Name(), fd.Number(), number)
	}
	if fd.Kind() != protoreflect.Int64Kind {
		t.Fatalf("%s user_id kind = %s", md.Name(), fd.Kind())
	}
}
