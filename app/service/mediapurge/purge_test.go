package mediapurge

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestFromMessagePhotoAndDocument(t *testing.T) {
	msg := &mtproto.Message{
		Message: "hello secret",
		Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 42}).To_Photo(),
		}).To_MessageMedia(),
	}
	got := FromMessage(msg)
	if len(got) != 1 || got[0].Kind != KindPhoto || got[0].ID != 42 {
		t.Fatalf("photo media %+v", got)
	}
	msg.Media = mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
		Document: mtproto.MakeTLDocument(&mtproto.Document{Id: 7}).To_Document(),
	}).To_MessageMedia()
	got = FromMessage(msg)
	if len(got) != 1 || got[0].Kind != KindDocument || got[0].ID != 7 {
		t.Fatalf("document media %+v", got)
	}
}

func TestFromJSONRoundTrip(t *testing.T) {
	raw := `{"message":"do not keep","media":{"predicate_name":"messageMediaPhoto","photo_FLAGPHOTO":{"predicate_name":"photo","id":99}}}`
	got := FromJSON(raw)
	if len(got) != 1 || got[0].ID != 99 || got[0].Kind != KindPhoto {
		t.Fatalf("json media %+v", got)
	}
	if len(FromJSON(Tombstone)) != 0 {
		t.Fatal("tombstone still has media")
	}
	if strings.Contains(Tombstone, "do not keep") {
		t.Fatal("tombstone contains text")
	}
}

func TestBoxMediasDedup(t *testing.T) {
	b := Box{Media: []Media{{Kind: KindPhoto, ID: 1}, {Kind: KindPhoto, ID: 1}, {Kind: KindDocument, ID: 0}}}
	got := b.Medias()
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestReportString(t *testing.T) {
	s := Report{Candidates: 2, WouldFreeBlobs: 1, WouldFreeObjects: 4, WouldFreeBytes: 9, Photos: 1, Documents: 1}.String()
	for _, part := range []string{"DRY_RUN", "candidates=2", "would_free_blobs=1", "would_free_objects=4", "would_free_bytes=9", "kept_message=0"} {
		if !strings.Contains(s, part) {
			t.Fatalf("report %q missing %s", s, part)
		}
	}
}

func TestGraceDefault(t *testing.T) {
	t.Setenv(EnvGrace, "")
	if Grace() != 24*time.Hour {
		t.Fatalf("grace %s", Grace())
	}
	t.Setenv(EnvGrace, "90s")
	if Grace() != 90*time.Second {
		t.Fatalf("grace %s", Grace())
	}
	t.Setenv(EnvGrace, "0")
	if Grace() != 0 {
		t.Fatalf("grace %s", Grace())
	}
}

func TestComposeLeavesPurgeOff(t *testing.T) {
	raw, err := os.ReadFile("../../../docker-compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, line := range []string{
		"TELLOMI_MEDIA_PURGE_ENABLED: ${TELLOMI_MEDIA_PURGE_ENABLED:-false}",
		"TELLOMI_MEDIA_PURGE_DRY_RUN: ${TELLOMI_MEDIA_PURGE_DRY_RUN:-true}",
		"TELLOMI_MEDIA_PURGE_GRACE: ${TELLOMI_MEDIA_PURGE_GRACE:-24h}",
	} {
		if !strings.Contains(text, line) {
			t.Fatalf("docker-compose.yaml missing %s", line)
		}
	}
}

func TestFromEnvDryRunDefaultsOn(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	t.Setenv(EnvDryRun, "")
	c := FromEnv()
	if c.Enabled {
		t.Fatal("worker enabled by default")
	}
	if !c.DryRun {
		t.Fatal("dry-run default")
	}
	t.Setenv(EnvDryRun, "false")
	t.Setenv(EnvEnabled, "1")
	c = FromEnv()
	if !c.Enabled || c.DryRun {
		t.Fatalf("%+v", c)
	}
}

func TestCrashReportsAreNotDeletable(t *testing.T) {
	if !protectedObject("crash-reports", "2026/11/10/9/3_log.zip") {
		t.Fatal("class")
	}
	if !protectedObject("documents", "crash-reports/2026/11/10/9/3_log.zip") {
		t.Fatal("path")
	}
	if protectedObject("documents", "12.dat") || protectedObject("photos", "0/9.dat") {
		t.Fatal("chat media was protected")
	}
	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(src), "protectedObject(o.Class, o.Path)") < 2 {
		t.Fatal("purge worker does not skip crash-report objects on the dry-run and live paths")
	}
}

func TestResultString(t *testing.T) {
	if !strings.Contains((Result{DryRun: true, Due: 3}).String(), "DRY_RUN") {
		t.Fatal(Result{DryRun: true}.String())
	}
	if !strings.Contains((Result{Due: 1, Objects: 2}).String(), "LIVE") {
		t.Fatal(Result{}.String())
	}
}
