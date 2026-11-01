// Package cdn is the master-DC side of Telegram's encrypted CDN download.
//
// https://core.telegram.org/cdn
// https://core.telegram.org/method/upload.getCdnFile
// https://core.telegram.org/method/upload.getCdnFileHashes
// https://core.telegram.org/method/upload.reuploadCdnFile
//
// help.getCdnConfig publishes TELLOMI_CDN_PUBLIC_KEYS. While that list is
// empty this server is not a CDN, and the three methods reject every token
// with FILE_TOKEN_INVALID so the client continues with upload.getFile.
// When keys are published, a staged file is encrypted with AES-256-CTR.
// upload.getCdnFile answers cdnFileReuploadNeeded until upload.reuploadCdnFile
// accepts the CDN's request_token. Hashes are SHA-256 of the ciphertext in
// 128 KiB parts, which is what the client checks before it decrypts.
//
// upload.getFile emits upload.fileCdnRedirect for a file that fits in this
// cache when the client sets cdn_supported and TELLOMI_CDN_PUBLIC_KEYS is set.
// The file_token from that redirect is the one getCdnFile and reuploadCdnFile
// already accept.
package cdn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const (
	align     = 4096
	megabyte  = 1024 * 1024
	hashChunk = 128 * 1024
	tokenLen  = 32
	maxToken  = 256
	// MaxPlain is the largest plaintext this process will encrypt for a CDN
	// redirect. Bigger files stay on the master DC.
	MaxPlain = 8 << 20
	maxFiles = 32
	ttl      = time.Hour
)

// CheckRange applies the CDN download limits from https://core.telegram.org/cdn.
// offset and limit must be multiples of 4096, limit must divide 1 MiB, and the
// chunk must stay inside one 1 MiB fragment.
func CheckRange(offset int64, limit int32) error {
	if offset < 0 || offset%align != 0 {
		return mtproto.ErrOffsetInvalid
	}
	if limit <= 0 || int(limit)%align != 0 || megabyte%int(limit) != 0 {
		return mtproto.ErrLimitInvalid
	}
	if offset/megabyte != (offset+int64(limit)-1)/megabyte {
		return mtproto.ErrLimitInvalid
	}
	return nil
}

// Book stores encrypted CDN objects for one process.
type Book struct {
	mu    sync.Mutex
	now   func() time.Time
	files map[string]*object
	byLoc map[string]*object
}

type object struct {
	loc      string
	token    []byte
	request  []byte
	key      []byte
	iv       []byte
	cipher   []byte
	hashes   []*mtproto.FileHash
	uploaded bool
	exp      time.Time
}

// Issued is the upload.fileCdnRedirect material for one staged file.
type Issued struct {
	FileToken []byte
	Key       []byte
	IV        []byte
	Hashes    []*mtproto.FileHash
}

// New returns an empty cache.
func New() *Book {
	return &Book{
		now:   time.Now,
		files: map[string]*object{},
		byLoc: map[string]*object{},
	}
}

var (
	sharedMu sync.Mutex
	shared   = New()
)

// Shared is the cache the files handlers use.
func Shared() *Book {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return shared
}

// Install replaces the process cache. Tests use it.
func Install(b *Book) {
	if b == nil {
		b = New()
	}
	sharedMu.Lock()
	shared = b
	sharedMu.Unlock()
}

// Stage encrypts plain and remembers it until reupload. The returned request
// token is what upload.getCdnFile reports in cdnFileReuploadNeeded.
func (b *Book) Stage(plain []byte) (fileToken, requestToken []byte, err error) {
	obj, err := b.put("", plain)
	if err != nil {
		return nil, nil, err
	}
	return append([]byte(nil), obj.token...), append([]byte(nil), obj.request...), nil
}

// Issue encrypts plain for upload.getFile's fileCdnRedirect. loc ties repeat
// downloads of the same file to one token, key, and hash list until it expires.
func (b *Book) Issue(loc string, plain []byte) (*Issued, error) {
	obj, err := b.put(loc, plain)
	if err != nil {
		return nil, err
	}
	return issuedCopy(obj), nil
}

// Lookup returns the redirect already issued for loc.
func (b *Book) Lookup(loc string) (*Issued, bool) {
	if loc == "" {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dropExpired()
	old := b.byLoc[loc]
	if old == nil || !b.now().Before(old.exp) {
		return nil, false
	}
	return issuedCopy(old), true
}

func issuedCopy(obj *object) *Issued {
	hashes := make([]*mtproto.FileHash, len(obj.hashes))
	for i, hash := range obj.hashes {
		hashes[i] = cloneHash(hash)
	}
	return &Issued{
		FileToken: append([]byte(nil), obj.token...),
		Key:       append([]byte(nil), obj.key...),
		IV:        append([]byte(nil), obj.iv...),
		Hashes:    hashes,
	}
}

// LocationKey identifies one inputFileLocation for the CDN cache.
func LocationKey(loc *mtproto.InputFileLocation) string {
	if loc == nil || loc.GetPredicateName() == "" {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d|%s|%d|%t|%d",
		loc.GetPredicateName(),
		loc.GetId(),
		loc.GetAccessHash(),
		loc.GetThumbSize(),
		loc.GetPhotoId(),
		loc.GetBig(),
		loc.GetThumbVersion(),
	)
}

func (b *Book) put(loc string, plain []byte) (*object, error) {
	if len(plain) == 0 || len(plain) > MaxPlain {
		return nil, mtproto.ErrFilePartInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dropExpired()
	if loc != "" {
		if old := b.byLoc[loc]; old != nil && b.now().Before(old.exp) {
			return old, nil
		}
	}
	key, iv, err := randomPair()
	if err != nil {
		return nil, err
	}
	fileToken, err := random(tokenLen)
	if err != nil {
		return nil, err
	}
	requestToken, err := random(tokenLen)
	if err != nil {
		return nil, err
	}
	if len(b.files) >= maxFiles {
		b.dropOldest()
	}
	obj := &object{
		loc:     loc,
		token:   fileToken,
		request: requestToken,
		key:     key,
		iv:      iv,
		cipher:  encrypt(plain, key, iv),
		exp:     b.now().Add(ttl),
	}
	obj.hashes = fileHashes(obj.cipher)
	b.files[hex.EncodeToString(fileToken)] = obj
	if loc != "" {
		b.byLoc[loc] = obj
	}
	return obj, nil
}

// Get is upload.getCdnFile. A staged file that the master has not reuploaded
// returns cdnFileReuploadNeeded. After reupload it returns the ciphertext
// slice, or an empty cdnFile once offset is at the end.
func (b *Book) Get(token []byte, offset int64, limit int32) (*mtproto.Upload_CdnFile, error) {
	if err := CheckRange(offset, limit); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, err := b.lookup(token)
	if err != nil {
		return nil, err
	}
	if !obj.uploaded {
		return mtproto.MakeTLUploadCdnFileReuploadNeeded(&mtproto.Upload_CdnFile{
			RequestToken: append([]byte(nil), obj.request...),
		}).To_Upload_CdnFile(), nil
	}
	chunk := slice(obj.cipher, offset, int64(limit))
	return mtproto.MakeTLUploadCdnFile(&mtproto.Upload_CdnFile{
		Bytes: chunk,
	}).To_Upload_CdnFile(), nil
}

// Hashes is upload.getCdnFileHashes. It returns SHA-256 parts at or after offset.
func (b *Book) Hashes(token []byte, offset int64) ([]*mtproto.FileHash, error) {
	if offset < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, err := b.lookup(token)
	if err != nil {
		return nil, err
	}
	var out []*mtproto.FileHash
	for _, hash := range obj.hashes {
		if hash.GetOffset_INT64() >= offset {
			out = append(out, cloneHash(hash))
		}
	}
	return out, nil
}

// Reupload is upload.reuploadCdnFile. The request_token must be the one
// getCdnFile returned. The file then becomes readable and the hashes of the
// ciphertext are returned.
func (b *Book) Reupload(fileToken, requestToken []byte) ([]*mtproto.FileHash, error) {
	if len(requestToken) == 0 || len(requestToken) > maxToken {
		return nil, mtproto.ErrRequestTokenInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, err := b.lookup(fileToken)
	if err != nil {
		return nil, err
	}
	if !bytesEqual(obj.request, requestToken) {
		return nil, mtproto.ErrRequestTokenInvalid
	}
	obj.uploaded = true
	out := make([]*mtproto.FileHash, len(obj.hashes))
	for i, hash := range obj.hashes {
		out[i] = cloneHash(hash)
	}
	return out, nil
}

func (b *Book) lookup(token []byte) (*object, error) {
	b.dropExpired()
	if len(token) == 0 || len(token) > maxToken {
		return nil, mtproto.ErrFileTokenInvalid
	}
	obj := b.files[hex.EncodeToString(token)]
	if obj == nil || !b.now().Before(obj.exp) {
		if obj != nil {
			b.forget(obj)
		}
		return nil, mtproto.ErrFileTokenInvalid
	}
	return obj, nil
}

func (b *Book) dropExpired() {
	now := b.now()
	for _, obj := range b.files {
		if !now.Before(obj.exp) {
			b.forget(obj)
		}
	}
}

func (b *Book) dropOldest() {
	var oldest *object
	for _, obj := range b.files {
		if oldest == nil || obj.exp.Before(oldest.exp) {
			oldest = obj
		}
	}
	if oldest != nil {
		b.forget(oldest)
	}
}

func (b *Book) forget(obj *object) {
	if obj == nil {
		return
	}
	delete(b.files, hex.EncodeToString(obj.token))
	if obj.loc != "" {
		delete(b.byLoc, obj.loc)
	}
}

func (b *Book) at(t time.Time) { b.now = func() time.Time { return t } }

func fileHashes(body []byte) []*mtproto.FileHash {
	if len(body) == 0 {
		return nil
	}
	var out []*mtproto.FileHash
	for off := 0; off < len(body); off += hashChunk {
		n := hashChunk
		if off+n > len(body) {
			n = len(body) - off
		}
		sum := sha256.Sum256(body[off : off+n])
		out = append(out, mtproto.MakeTLFileHash(&mtproto.FileHash{
			Offset_INT64: int64(off),
			Limit:        int32(n),
			Hash:         append([]byte(nil), sum[:]...),
		}).To_FileHash())
	}
	return out
}

func slice(body []byte, offset, limit int64) []byte {
	if offset >= int64(len(body)) {
		return []byte{}
	}
	end := offset + limit
	if end > int64(len(body)) {
		end = int64(len(body))
	}
	return append([]byte(nil), body[offset:end]...)
}

// encrypt applies AES-256-CTR the way a CDN client decrypts a chunk.
// For a read at `offset`, the client replaces the last four bytes of the IV
// with offset/16 in big-endian. Encrypting from offset 0 with that counter
// makes every later aligned read match.
func encrypt(plain, key, iv []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	ctrIV := append([]byte(nil), iv...)
	binary.BigEndian.PutUint32(ctrIV[len(ctrIV)-4:], 0)
	out := make([]byte, len(plain))
	cipher.NewCTR(block, ctrIV).XORKeyStream(out, plain)
	return out
}

func randomPair() (key, iv []byte, err error) {
	key, err = random(32)
	if err != nil {
		return nil, nil, err
	}
	iv, err = random(aes.BlockSize)
	if err != nil {
		return nil, nil, err
	}
	return key, iv, nil
}

func random(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func cloneHash(h *mtproto.FileHash) *mtproto.FileHash {
	if h == nil {
		return nil
	}
	return mtproto.MakeTLFileHash(&mtproto.FileHash{
		Offset_INT64: h.GetOffset_INT64(),
		Limit:        h.GetLimit(),
		Hash:         append([]byte(nil), h.GetHash()...),
	}).To_FileHash()
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
