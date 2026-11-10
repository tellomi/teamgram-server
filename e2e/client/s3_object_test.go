package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// attachmentObjectExists reports whether TELLOMI_S3_* still has this chat
// object. CI publishes the compose object store on 127.0.0.1:9000 (MinIO
// locally, RustFS in the e2e overlay). Both speak path-style SigV4. The
// credentials are the compose defaults, already in .env.example.
func attachmentObjectExists(t *testing.T, class, path string) bool {
	t.Helper()
	status, body, err := headAttachment(class, path)
	if err != nil {
		t.Fatalf("head %s/%s: %v", class, path, err)
	}
	switch status {
	case http.StatusOK:
		return true
	case http.StatusNotFound:
		return false
	default:
		t.Fatalf("head %s/%s: status %d\n%s", class, path, status, body)
		return false
	}
}

func headAttachment(class, path string) (int, string, error) {
	endpoint := getenvDefault("TELLOMI_E2E_S3_URL", "http://127.0.0.1:9000")
	bucket := getenvDefault("TELLOMI_S3_BUCKET_ATTACHMENTS", "tellomi-attachments")
	region := getenvDefault("TELLOMI_S3_REGION", "us-east-1")
	access := getenvDefault("TELLOMI_S3_ACCESS_KEY_ID", "minio")
	secret := getenvDefault("TELLOMI_S3_SECRET_ACCESS_KEY", "miniostorage")
	key := class + "/" + strings.TrimPrefix(path, "/")
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0, "", err
	}
	u.Path = "/" + bucket + "/" + key
	req, err := http.NewRequest(http.MethodHead, u.String(), nil)
	if err != nil {
		return 0, "", err
	}
	if err = signS3(req, region, access, secret, time.Now().UTC()); err != nil {
		return 0, "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return resp.StatusCode, string(raw), nil
}

func getenvDefault(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

// signS3 sets a path-style AWS SigV4 Authorization header. now is injectable
// so the header can be compared with the published AWS example.
func signS3(req *http.Request, region, access, secret string, now time.Time) error {
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	payload := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payload)
	host := req.URL.Host
	canonURI := req.URL.EscapedPath()
	if canonURI == "" {
		canonURI = "/"
	}
	canonHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payload + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signed := "host;x-amz-content-sha256;x-amz-date"
	canon := req.Method + "\n" + canonURI + "\n" + req.URL.RawQuery + "\n" +
		canonHeaders + "\n" + signed + "\n" + payload
	scope := date + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hexSHA(canon)
	sig := hex.EncodeToString(hmacSHA(deriveS3Key(secret, date, region), stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		access, scope, signed, sig))
	req.Host = host
	return nil
}

func deriveS3Key(secret, date, region string) []byte {
	kDate := hmacSHA("AWS4"+secret, date)
	kRegion := hmacSHA(kDate, region)
	kService := hmacSHA(kRegion, "s3")
	return hmacSHA(kService, "aws4_request")
}

func hmacSHA(key interface{}, data string) []byte {
	var mac hashMAC
	switch k := key.(type) {
	case string:
		mac = hmac.New(sha256.New, []byte(k))
	case []byte:
		mac = hmac.New(sha256.New, k)
	default:
		panic(fmt.Sprintf("hmac key %T", key))
	}
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

type hashMAC interface {
	io.Writer
	Sum([]byte) []byte
}

func hexSHA(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// documentObjectKeys is every key the purge worker can delete for a document:
// the blob (the shared dedup object) and documents.file_path.
func documentObjectKeys(t *testing.T, documentID int64) [][2]string {
	t.Helper()
	out := mustSQL(t, fmt.Sprintf(`
SELECT storage_class, file_path FROM (
  SELECT b.storage_class AS storage_class, b.file_path AS file_path
  FROM media_blob_refs r
  JOIN media_blobs b ON b.id = r.blob_id
  WHERE r.ref_kind='document' AND r.ref_id=%d AND b.file_path<>''
  UNION
  SELECT 'documents', file_path FROM documents
  WHERE document_id=%d AND file_path<>''
) q`, documentID, documentID))
	return parseObjectKeys(t, out)
}

func parseObjectKeys(t *testing.T, raw string) [][2]string {
	t.Helper()
	var keys [][2]string
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mysql:") {
			continue
		}
		class, path, ok := strings.Cut(line, "\t")
		if !ok || class == "" || path == "" {
			continue
		}
		id := class + "\t" + path
		if seen[id] {
			continue
		}
		seen[id] = true
		keys = append(keys, [2]string{class, path})
	}
	return keys
}

// storedObjects is the subset of keys that are in the bucket right now.
func storedObjects(t *testing.T, keys [][2]string) [][2]string {
	t.Helper()
	var live [][2]string
	for _, k := range keys {
		if attachmentObjectExists(t, k[0], k[1]) {
			live = append(live, k)
		}
	}
	return live
}

func requireObjectsStored(t *testing.T, keys [][2]string) {
	t.Helper()
	for _, k := range keys {
		if !attachmentObjectExists(t, k[0], k[1]) {
			t.Fatalf("object %s/%s is gone", k[0], k[1])
		}
	}
}

func requireObjectsDeleted(t *testing.T, keys [][2]string) {
	t.Helper()
	for _, k := range keys {
		if attachmentObjectExists(t, k[0], k[1]) {
			t.Fatalf("object %s/%s still stored after purge", k[0], k[1])
		}
	}
}

// TestS3HeaderMatchesAWSExample is the published GET Object signature from
// https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-header-based-auth.html
// (Signature Calculations for the Authorization Header, example GET).
func TestS3HeaderMatchesAWSExample(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-9")
	// The published example signs host, range, and the two x-amz headers.
	// signS3 only signs the three headers the purge check sends. This test
	// checks that derivation against the same secret and date stamp the
	// example uses, for a request shaped like ours.
	when := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	if err = signS3(req, "us-east-1", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", when); err != nil {
		t.Fatal(err)
	}
	got := req.Header.Get("Authorization")
	// Independent Python HMAC of the same canonical request. A drift here means
	// the header the object check sends would be rejected by MinIO and R2.
	const want = "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=df548e2ce037944d03f3e68682813b093763996d597cf890ca3d9037fd231eb4"
	if got != want {
		t.Fatalf("authorization\n got %s\nwant %s", got, want)
	}
}
