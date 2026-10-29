package minio_util

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Env var names are part of the ops contract. Do not rename them.
const (
	EnvEndpoint           = "TELLOMI_S3_ENDPOINT"
	EnvRegion             = "TELLOMI_S3_REGION"
	EnvAccessKeyID        = "TELLOMI_S3_ACCESS_KEY_ID"
	EnvSecretAccessKey    = "TELLOMI_S3_SECRET_ACCESS_KEY"
	EnvForcePathStyle     = "TELLOMI_S3_FORCE_PATH_STYLE"
	EnvUseSSL             = "TELLOMI_S3_USE_SSL"
	EnvBucketAttachments  = "TELLOMI_S3_BUCKET_ATTACHMENTS"
	EnvBucketPublicMedia  = "TELLOMI_S3_BUCKET_PUBLIC_MEDIA"
	EnvBucketBackups      = "TELLOMI_S3_BUCKET_BACKUPS"
	EnvPublicMediaBaseURL = "TELLOMI_S3_PUBLIC_MEDIA_BASE_URL"
	EnvAttachmentsBaseURL = "TELLOMI_S3_ATTACHMENTS_BASE_URL"
)

// BucketConfig names the physical buckets.
//
// When Attachments is set, the historical MinIO buckets (documents, photos,
// videos, encryptedfiles) are key prefixes inside that bucket. Public assets
// use PublicMedia. Backups is optional.
//
// When Attachments is empty, Documents/Photos/Videos/EncryptedFiles are used
// as separate buckets with no prefix. That keeps an unchanged legacy yaml
// working. Docker and R2 set Attachments.
type BucketConfig struct {
	Documents      string `json:",default=documents"`
	Photos         string `json:",default=photos"`
	Videos         string `json:",default=videos"`
	EncryptedFiles string `json:",default=encryptedfiles"`

	Attachments string `json:",optional"`
	PublicMedia string `json:",optional"`
	Backups     string `json:",optional"`
}

// MinioConfig is the S3 client configuration. The historical yaml key stays
// "Minio" so existing dfs.yaml files still load. Values are an S3-compatible
// endpoint (MinIO locally, Cloudflare R2 in production).
type MinioConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	// Region defaults to "auto" (required by Cloudflare R2). Local MinIO
	// accepts that value with path-style requests; set us-east-1 if a
	// particular MinIO build rejects it.
	Region string `json:",optional"`
	// ForcePathStyle defaults to true. Set TELLOMI_S3_FORCE_PATH_STYLE=false
	// to use virtual-hosted-style requests.
	ForcePathStyle bool `json:",default=true"`
	// pathStyleDisabled is set only when TELLOMI_S3_FORCE_PATH_STYLE is
	// explicitly false. Unexported so go-zero does not load it: several
	// json:"-" fields become the same key and conf.Load fails with
	// "conflict key minio.-".
	pathStyleDisabled bool
	// useSSLExplicit is set when TELLOMI_S3_USE_SSL is present.
	useSSLExplicit bool

	Bucket BucketConfig

	PublicMediaBaseURL string `json:",optional"`
	AttachmentsBaseURL string `json:",optional"`

	// BucketProbeAttempts is how many times startup checks each bucket.
	// Zero means the default. Tests set 1. Missing buckets are not created.
	BucketProbeAttempts int `json:",optional"`

	// transport overrides the HTTP client. Tests use it to trust a local TLS server.
	// Unexported for the same reason as pathStyleDisabled: go-zero 1.10 rejects
	// duplicate json:"-" keys, and a RoundTripper cannot be loaded from yaml.
	transport http.RoundTripper
}

// ApplyEnv overlays the TELLOMI_S3_* variables onto c. Unset variables keep
// the values already loaded from yaml. Call this once before building the client.
func (c *MinioConfig) ApplyEnv() error {
	if c == nil {
		return fmt.Errorf("object storage config is nil")
	}
	if v := strings.TrimSpace(os.Getenv(EnvEndpoint)); v != "" {
		c.Endpoint = v
	}
	if v, ok := os.LookupEnv(EnvRegion); ok && strings.TrimSpace(v) != "" {
		c.Region = strings.TrimSpace(v)
	}
	if v := os.Getenv(EnvAccessKeyID); v != "" {
		c.AccessKeyID = v
	}
	if v := os.Getenv(EnvSecretAccessKey); v != "" {
		c.SecretAccessKey = v
	}
	if v, ok := os.LookupEnv(EnvForcePathStyle); ok && strings.TrimSpace(v) != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("%s: %w", EnvForcePathStyle, err)
		}
		c.ForcePathStyle = b
		c.pathStyleDisabled = !b
	}
	if v, ok := os.LookupEnv(EnvUseSSL); ok && strings.TrimSpace(v) != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("%s: %w", EnvUseSSL, err)
		}
		c.UseSSL = b
		c.useSSLExplicit = true
	}
	if v := strings.TrimSpace(os.Getenv(EnvBucketAttachments)); v != "" {
		c.Bucket.Attachments = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvBucketPublicMedia)); v != "" {
		c.Bucket.PublicMedia = v
	}
	if v, ok := os.LookupEnv(EnvBucketBackups); ok {
		c.Bucket.Backups = strings.TrimSpace(v)
	}
	if v, ok := os.LookupEnv(EnvPublicMediaBaseURL); ok {
		c.PublicMediaBaseURL = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	if v, ok := os.LookupEnv(EnvAttachmentsBaseURL); ok {
		c.AttachmentsBaseURL = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	return c.normalize()
}

func (c *MinioConfig) normalize() error {
	if strings.TrimSpace(c.Region) == "" {
		c.Region = "auto"
	}
	if !c.pathStyleDisabled {
		c.ForcePathStyle = true
	}
	host, scheme, err := splitEndpoint(c.Endpoint)
	if err != nil {
		return err
	}
	if !c.useSSLExplicit {
		switch scheme {
		case "https":
			c.UseSSL = true
		case "http":
			c.UseSSL = false
		}
	}
	c.Endpoint = host
	c.PublicMediaBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicMediaBaseURL), "/")
	c.AttachmentsBaseURL = strings.TrimRight(strings.TrimSpace(c.AttachmentsBaseURL), "/")
	if c.Bucket.Attachments == "" {
		if c.Bucket.Documents == "" {
			c.Bucket.Documents = "documents"
		}
		if c.Bucket.Photos == "" {
			c.Bucket.Photos = "photos"
		}
		if c.Bucket.Videos == "" {
			c.Bucket.Videos = "videos"
		}
		if c.Bucket.EncryptedFiles == "" {
			c.Bucket.EncryptedFiles = "encryptedfiles"
		}
	} else if c.Bucket.PublicMedia == "" {
		return fmt.Errorf("%s is required when %s is set", EnvBucketPublicMedia, EnvBucketAttachments)
	}
	return nil
}

func splitEndpoint(raw string) (host, scheme string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("object storage endpoint is empty (set %s)", EnvEndpoint)
	}
	if !strings.Contains(raw, "://") {
		return raw, "", nil
	}
	u, perr := url.Parse(raw)
	if perr != nil {
		return "", "", fmt.Errorf("invalid %s %q: %w", EnvEndpoint, raw, perr)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("invalid %s %q: missing host", EnvEndpoint, raw)
	}
	if u.Path != "" && u.Path != "/" {
		return "", "", fmt.Errorf("invalid %s %q: a path is not allowed", EnvEndpoint, raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http":
	default:
		return "", "", fmt.Errorf("invalid %s %q: scheme must be http or https", EnvEndpoint, raw)
	}
	return u.Host, strings.ToLower(u.Scheme), nil
}

func parseBool(v string) (bool, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return false, fmt.Errorf("empty boolean")
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid boolean %q", v)
	}
	return b, nil
}
