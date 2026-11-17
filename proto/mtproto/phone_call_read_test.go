package mtproto

import "testing"

func TestAnsweredPhoneCallVoiceAndVideo(t *testing.T) {
	hangup := MakeTLPhoneCallDiscardReasonHangup(nil).To_PhoneCallDiscardReason()
	disconnect := MakeTLPhoneCallDiscardReasonDisconnect(nil).To_PhoneCallDiscardReason()
	missed := MakeTLPhoneCallDiscardReasonMissed(nil).To_PhoneCallDiscardReason()
	busy := MakeTLPhoneCallDiscardReasonBusy(nil).To_PhoneCallDiscardReason()

	cases := []struct {
		name     string
		video    bool
		reason   *PhoneCallDiscardReason
		duration int32
		want     bool
	}{
		{name: "voice hangup", video: false, reason: hangup, duration: 12, want: true},
		{name: "video hangup", video: true, reason: hangup, duration: 12, want: true},
		{name: "voice disconnect", video: false, reason: disconnect, duration: 3, want: true},
		{name: "video disconnect", video: true, reason: disconnect, duration: 3, want: true},
		{name: "voice missed", video: false, reason: missed, duration: 0, want: false},
		{name: "video missed", video: true, reason: missed, duration: 0, want: false},
		{name: "voice busy", video: false, reason: busy, duration: 0, want: false},
		{name: "video busy", video: true, reason: busy, duration: 0, want: false},
		{name: "voice decline", video: false, reason: hangup, duration: 0, want: false},
		{name: "video decline", video: true, reason: hangup, duration: 0, want: false},
		{name: "missed with stray duration", video: false, reason: missed, duration: 9, want: false},
		{name: "busy with stray duration", video: true, reason: busy, duration: 9, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action := MakeMessageActionPhoneCall(tc.video, 42, tc.reason, tc.duration)
			if got := AnsweredPhoneCall(action); got != tc.want {
				t.Fatalf("video=%v reason=%s duration=%d: got %v, want %v", tc.video, tc.reason.GetPredicateName(), tc.duration, got, tc.want)
			}
		})
	}
	if AnsweredPhoneCall(nil) {
		t.Fatal("nil action is not an answered call")
	}
	if AnsweredPhoneCall(MakeTLMessageActionHistoryClear(nil).To_MessageAction()) {
		t.Fatal("another service action is not an answered call")
	}
}
