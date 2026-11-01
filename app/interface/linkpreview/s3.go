package linkpreview

import (
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// objectLoc is one GetObject attempt. Production objects live in one R2
// bucket with a class prefix (photos/<file_path>). Older MinIO installs use
// a bucket per class and the file path as the key.
type objectLoc struct {
	bucket string
	key    string
}

// objectCandidates lists bucket/key pairs for a photo_sizes or documents
// file_path. When the three buckets are the same physical bucket, the class
// prefix is tried first. That is the Cloudflare R2 layout (tellomi-attachments).
func objectCandidates(cfg Config, path string) []objectLoc {
	type spec struct {
		bucket string
		class  string
	}
	specs := []spec{
		{cfg.MinioBucketPhotos, "photos"},
		{cfg.MinioBucketDocuments, "documents"},
		{cfg.MinioBucketVideos, "videos"},
	}
	shared := cfg.MinioBucketPhotos != "" &&
		cfg.MinioBucketPhotos == cfg.MinioBucketDocuments &&
		cfg.MinioBucketDocuments == cfg.MinioBucketVideos
	var out []objectLoc
	seen := map[string]struct{}{}
	add := func(bucket, key string) {
		if bucket == "" || key == "" {
			return
		}
		id := bucket + "\x00" + key
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, objectLoc{bucket: bucket, key: key})
	}
	for _, s := range specs {
		prefixed := s.class + "/" + strings.TrimPrefix(path, s.class+"/")
		if shared {
			add(s.bucket, prefixed)
			add(s.bucket, path)
			continue
		}
		add(s.bucket, path)
		add(s.bucket, prefixed)
	}
	return out
}

// minioClientOptions builds the minio-go client settings for an S3-compatible
// endpoint. minio-go v7 prepends the scheme from Options.Secure, so the
// endpoint must be a host (and optional port) with no scheme and no path.
//
// Region must be set. An empty region makes the client call GetBucketLocation.
// Cloudflare answers that with NotImplemented, and minio-go then signs as
// us-east-1, which R2 rejects. R2 requires region "auto". BucketLookupPath
// matches the dfs client so the bucket is not turned into a subdomain.
func (c Config) minioClientOptions() (endpoint string, opts *minio.Options, err error) {
	host, scheme, err := splitMinioEndpoint(c.MinioEndpoint)
	if err != nil || host == "" {
		return "", nil, err
	}
	secure := c.MinioSSL
	if !c.minioSSLSet {
		switch scheme {
		case "https":
			secure = true
		case "http":
			secure = false
		}
	}
	region := strings.TrimSpace(c.MinioRegion)
	if region == "" {
		region = "auto"
	}
	return host, &minio.Options{
		Creds:        credentials.NewStaticV4(c.MinioAccessKey, c.MinioSecretKey, ""),
		Secure:       secure,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	}, nil
}

func newMinioClient(cfg Config) (*minio.Client, error) {
	endpoint, opts, err := cfg.minioClientOptions()
	if err != nil || opts == nil {
		return nil, err
	}
	return minio.New(endpoint, opts)
}

func splitMinioEndpoint(raw string) (host, scheme string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	if !strings.Contains(raw, "://") {
		return raw, "", nil
	}
	u, perr := url.Parse(raw)
	if perr != nil {
		return "", "", perr
	}
	if u.Host == "" {
		return "", "", errInvalidArgument("object storage endpoint is missing a host")
	}
	if u.Path != "" && u.Path != "/" {
		return "", "", errInvalidArgument("object storage endpoint must not include a path")
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http":
	default:
		return "", "", errInvalidArgument("object storage endpoint scheme must be http or https")
	}
	return u.Host, strings.ToLower(u.Scheme), nil
}

type argError struct{ msg string }

func (e argError) Error() string { return e.msg }

func errInvalidArgument(msg string) error { return argError{msg: msg} }
