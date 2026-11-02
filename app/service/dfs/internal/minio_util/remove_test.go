package minio_util

import (
	"context"
	"testing"
)

func TestRemoveClassObject(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	srv, cfg := fake.server(t)
	defer srv.Close()
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := client.PutPhotoFile(ctx, "x/9.dat", []byte("full")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutPhotoFile(ctx, "s/9.dat", []byte("thumb")); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveClassObject(ctx, ClassPhotos, "x/9.dat"); err != nil {
		t.Fatal(err)
	}
	if fake.has("tellomi-attachments", "photos/x/9.dat", "full") {
		t.Fatal("photos/x/9.dat still stored")
	}
	if !fake.has("tellomi-attachments", "photos/s/9.dat", "thumb") {
		t.Fatal("thumbnail removed")
	}
	// Already gone is fine, so a retried sweep succeeds.
	if err := client.RemoveClassObject(ctx, ClassPhotos, "x/9.dat"); err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{ClassAvatars, ClassStickers, ClassEmoji, ClassWallpapers, ClassBackups} {
		if err := client.RemoveClassObject(ctx, class, "1.dat"); err == nil {
			t.Fatalf("%s: removal of public or backup media allowed", class)
		}
	}
}
