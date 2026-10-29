package minio_util

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactedMasksS3Credentials(t *testing.T) {
	const access = "r2-access-key-ABCDEFGH1234"
	const secret = "r2-secret-value-WXYZ9876"
	cfg := MinioConfig{
		Endpoint:        "account.r2.cloudflarestorage.com",
		AccessKeyID:     access,
		SecretAccessKey: secret,
		UseSSL:          true,
		Region:          "auto",
		Bucket: BucketConfig{
			Attachments: "tellomi-attachments",
			PublicMedia: "tellomi-public-media",
		},
		PublicMediaBaseURL: "https://media.tellomi.app",
	}

	logged := cfg.Redacted()
	if cfg.AccessKeyID != access || cfg.SecretAccessKey != secret {
		t.Fatal("Redacted mutated the live config")
	}
	if logged.AccessKeyID != "***1234" {
		t.Fatalf("access key mask %q", logged.AccessKeyID)
	}
	if logged.SecretAccessKey != "***9876" {
		t.Fatalf("secret mask %q", logged.SecretAccessKey)
	}
	if strings.Contains(logged.AccessKeyID, access) || strings.Contains(logged.SecretAccessKey, secret) {
		t.Fatalf("redacted config still contains a credential: %+v", logged)
	}
	if logged.Endpoint != cfg.Endpoint || logged.Region != cfg.Region || logged.Bucket.Attachments != cfg.Bucket.Attachments {
		t.Fatalf("non-secret fields changed: %+v", logged)
	}

	// logx.Infov writes the value as JSON. The startup log must not contain either key.
	raw, err := json.Marshal(struct{ Minio MinioConfig }{Minio: logged})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, access) || strings.Contains(text, secret) {
		t.Fatalf("json log leaked credentials: %s", text)
	}
	if !strings.Contains(text, "***1234") || !strings.Contains(text, "***9876") {
		t.Fatalf("json log missing masks: %s", text)
	}
}

func TestRedactedShortAndEmptyCredentials(t *testing.T) {
	logged := MinioConfig{AccessKeyID: "min", SecretAccessKey: ""}.Redacted()
	if logged.AccessKeyID != "***" || logged.SecretAccessKey != "" {
		t.Fatalf("short/empty: access %q secret %q", logged.AccessKeyID, logged.SecretAccessKey)
	}
	if got := (MinioConfig{AccessKeyID: "abcd", SecretAccessKey: "abcde"}).Redacted(); got.AccessKeyID != "***" || got.SecretAccessKey != "***bcde" {
		t.Fatalf("boundary: access %q secret %q", got.AccessKeyID, got.SecretAccessKey)
	}
}
