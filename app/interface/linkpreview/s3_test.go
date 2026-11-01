package linkpreview

import (
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestObjectCandidatesR2SharedBucket(t *testing.T) {
	cfg := Config{
		MinioBucketPhotos:    "tellomi-attachments",
		MinioBucketDocuments: "tellomi-attachments",
		MinioBucketVideos:    "tellomi-attachments",
	}
	got := objectCandidates(cfg, "m/123.dat")
	want := []objectLoc{
		{"tellomi-attachments", "photos/m/123.dat"},
		{"tellomi-attachments", "m/123.dat"},
		{"tellomi-attachments", "documents/m/123.dat"},
		{"tellomi-attachments", "videos/m/123.dat"},
	}
	if len(got) != len(want) {
		t.Fatalf("len %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestObjectCandidatesLegacyBuckets(t *testing.T) {
	cfg := Config{
		MinioBucketPhotos:    "photos",
		MinioBucketDocuments: "documents",
		MinioBucketVideos:    "videos",
	}
	got := objectCandidates(cfg, "m/123.dat")
	if len(got) == 0 || got[0].bucket != "photos" || got[0].key != "m/123.dat" {
		t.Fatalf("first candidate %+v, want photos/m/123.dat", got)
	}
}

func TestMinioGoAcceptsR2Endpoint(t *testing.T) {
	cfg := Config{
		MinioEndpoint:  "https://accountid.r2.cloudflarestorage.com",
		MinioAccessKey: "access-key",
		MinioSecretKey: "secret-key",
		MinioRegion:    "auto",
	}
	endpoint, opts, err := cfg.minioClientOptions()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "accountid.r2.cloudflarestorage.com" {
		t.Fatalf("endpoint %q", endpoint)
	}
	if opts.Region != "auto" {
		t.Fatalf("region %q", opts.Region)
	}
	if !opts.Secure {
		t.Fatal("https endpoint must enable TLS when TELLOMI_MINIO_USE_SSL is unset")
	}
	if opts.BucketLookup != minio.BucketLookupPath {
		t.Fatalf("lookup %v", opts.BucketLookup)
	}
	// minio-go v7 stores Region and does not call GetBucketLocation when it is set.
	// New does not dial. A failure here means this client cannot target R2.
	cl, err := minio.New(endpoint, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cl == nil {
		t.Fatal("nil client")
	}
}

func TestMinioOptionsHostAndExplicitSSL(t *testing.T) {
	cfg := Config{
		MinioEndpoint:  "accountid.r2.cloudflarestorage.com",
		MinioAccessKey: "access-key",
		MinioSecretKey: "secret-key",
		MinioSSL:       true,
		minioSSLSet:    true,
	}
	endpoint, opts, err := cfg.minioClientOptions()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "accountid.r2.cloudflarestorage.com" || !opts.Secure || opts.Region != "auto" {
		t.Fatalf("endpoint %q secure %v region %q", endpoint, opts.Secure, opts.Region)
	}
}

func TestLoadLinkpreviewYaml(t *testing.T) {
	cfg := Load("../../../teamgramd/etc2/linkpreview.yaml")
	if cfg.MinioRegion != "auto" || cfg.MinioBucketPhotos != "tellomi-attachments" {
		t.Fatalf("region %q bucket %q", cfg.MinioRegion, cfg.MinioBucketPhotos)
	}
	if cfg.MinioEndpoint != "minio:9000" || cfg.MinioSSL {
		t.Fatalf("endpoint %q ssl %v", cfg.MinioEndpoint, cfg.MinioSSL)
	}
}

func TestApplyEnvMinioRegion(t *testing.T) {
	t.Setenv("TELLOMI_MINIO_ENDPOINT", "accountid.r2.cloudflarestorage.com")
	t.Setenv("TELLOMI_MINIO_REGION", "auto")
	t.Setenv("TELLOMI_MINIO_ACCESS_KEY", "ak")
	t.Setenv("TELLOMI_MINIO_SECRET_KEY", "sk")
	t.Setenv("TELLOMI_MINIO_USE_SSL", "true")
	t.Setenv("TELLOMI_MINIO_BUCKET_PHOTOS", "tellomi-attachments")
	t.Setenv("TELLOMI_MINIO_BUCKET_DOCUMENTS", "tellomi-attachments")
	t.Setenv("TELLOMI_MINIO_BUCKET_VIDEOS", "tellomi-attachments")
	cfg := Load("")
	if cfg.MinioRegion != "auto" || !cfg.MinioSSL || !cfg.minioSSLSet {
		t.Fatalf("region %q ssl %v explicit %v", cfg.MinioRegion, cfg.MinioSSL, cfg.minioSSLSet)
	}
	if cfg.MinioBucketPhotos != "tellomi-attachments" {
		t.Fatalf("bucket %q", cfg.MinioBucketPhotos)
	}
	endpoint, opts, err := cfg.minioClientOptions()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "accountid.r2.cloudflarestorage.com" || !opts.Secure || opts.Region != "auto" {
		t.Fatalf("endpoint %q secure %v region %q", endpoint, opts.Secure, opts.Region)
	}
}
