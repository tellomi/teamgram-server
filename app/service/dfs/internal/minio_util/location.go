package minio_util

import (
	"fmt"
	"strings"
)

// Logical classes. The first four match Teamgram's historical MinIO buckets
// and become key prefixes. The public classes are served from the public
// media bucket when one is configured.
const (
	ClassDocuments      = "documents"
	ClassPhotos         = "photos"
	ClassVideos         = "videos"
	ClassEncryptedFiles = "encryptedfiles"
	ClassAvatars        = "avatars"
	ClassStickers       = "stickers"
	ClassEmoji          = "emoji"
	ClassWallpapers     = "wallpapers"
	ClassBackups        = "backups"
)

func resolveLocation(c *MinioConfig, class, path string) (bucket, key string, err error) {
	if c == nil {
		return "", "", fmt.Errorf("object storage config is nil")
	}
	path, err = cleanObjectPath(path)
	if err != nil {
		return "", "", err
	}
	switch class {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles:
		if c.Bucket.Attachments != "" {
			return c.Bucket.Attachments, class + "/" + path, nil
		}
		return legacyBucket(c, class), path, nil
	case ClassAvatars, ClassStickers, ClassEmoji, ClassWallpapers:
		if c.Bucket.PublicMedia != "" {
			return c.Bucket.PublicMedia, class + "/" + path, nil
		}
		if c.Bucket.Attachments != "" {
			return c.Bucket.Attachments, class + "/" + path, nil
		}
		// Legacy installs have no public bucket. Keep the object in the
		// photos bucket under a class prefix so profile photos still upload.
		return legacyBucket(c, ClassPhotos), class + "/" + path, nil
	case ClassBackups:
		if strings.TrimSpace(c.Bucket.Backups) == "" {
			return "", "", fmt.Errorf("backup bucket is not configured (set %s)", EnvBucketBackups)
		}
		return c.Bucket.Backups, path, nil
	default:
		return "", "", fmt.Errorf("unknown object storage class %q", class)
	}
}

func legacyBucket(c *MinioConfig, class string) string {
	switch class {
	case ClassDocuments:
		if c.Bucket.Documents != "" {
			return c.Bucket.Documents
		}
	case ClassPhotos:
		if c.Bucket.Photos != "" {
			return c.Bucket.Photos
		}
	case ClassVideos:
		if c.Bucket.Videos != "" {
			return c.Bucket.Videos
		}
	case ClassEncryptedFiles:
		if c.Bucket.EncryptedFiles != "" {
			return c.Bucket.EncryptedFiles
		}
	}
	return class
}

func cleanObjectPath(path string) (string, error) {
	path = strings.TrimPrefix(strings.TrimSpace(path), "/")
	if path == "" {
		return "", fmt.Errorf("object key is empty")
	}
	if strings.Contains(path, "\\") {
		return "", fmt.Errorf("object key %q is invalid", path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("object key %q is invalid", path)
		}
	}
	return path, nil
}

func joinPublicURL(base, key string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(key, "/")
}

func bucketsToCheck(c *MinioConfig) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if c.Bucket.Attachments != "" {
		if c.Bucket.PublicMedia == "" {
			return nil, fmt.Errorf("%s is required when %s is set", EnvBucketPublicMedia, EnvBucketAttachments)
		}
		add(c.Bucket.Attachments)
		add(c.Bucket.PublicMedia)
		add(c.Bucket.Backups)
		return out, nil
	}
	add(legacyBucket(c, ClassDocuments))
	add(legacyBucket(c, ClassPhotos))
	add(legacyBucket(c, ClassVideos))
	add(legacyBucket(c, ClassEncryptedFiles))
	add(c.Bucket.PublicMedia)
	add(c.Bucket.Backups)
	return out, nil
}

func envVarForBucket(c *MinioConfig, bucket string) string {
	switch bucket {
	case c.Bucket.Attachments:
		if c.Bucket.Attachments != "" {
			return EnvBucketAttachments
		}
	case c.Bucket.PublicMedia:
		if c.Bucket.PublicMedia != "" {
			return EnvBucketPublicMedia
		}
	case c.Bucket.Backups:
		if c.Bucket.Backups != "" {
			return EnvBucketBackups
		}
	}
	return "bucket"
}
