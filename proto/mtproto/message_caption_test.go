package mtproto

import "testing"

func TestCaptionEditable(t *testing.T) {
	text := MakeTLMessage(&Message{Message: "hello"}).To_Message()
	if CaptionEditable(text) || CaptionEditable(nil) {
		t.Fatal("plain text has no caption")
	}
	photo := MakeTLMessage(&Message{
		Media: MakeTLMessageMediaPhoto(&MessageMedia{}).To_MessageMedia(),
	}).To_Message()
	if !CaptionEditable(photo) {
		t.Fatal("photo caption should be editable")
	}
	file := MakeTLMessage(&Message{
		Media: MakeTLMessageMediaDocument(&MessageMedia{
			Document: MakeTLDocument(&Document{}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
	if !CaptionEditable(file) {
		t.Fatal("file caption should be editable")
	}
	webpage := MakeTLMessage(&Message{
		Media: MakeTLMessageMediaWebPage(&MessageMedia{}).To_MessageMedia(),
	}).To_Message()
	if CaptionEditable(webpage) {
		t.Fatal("webpage is not a caption")
	}
}
