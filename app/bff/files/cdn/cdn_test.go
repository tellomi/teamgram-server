package cdn

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestIssueReusesLocation(t *testing.T) {
	b := New()
	loc := mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{Id: 5, AccessHash: 6}).To_InputFileLocation()
	key := LocationKey(loc)
	first, err := b.Issue(key, []byte("same-file"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Issue(key, []byte("same-file"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.FileToken, second.FileToken) || !bytes.Equal(first.Key, second.Key) {
		t.Fatal("token changed")
	}
	if LocationKey(nil) != "" {
		t.Fatal("nil location")
	}
}

func TestRangeRules(t *testing.T) {
	if err := CheckRange(0, 4096); err != nil {
		t.Fatal(err)
	}
	if err := CheckRange(100, 4096); err != mtproto.ErrOffsetInvalid {
		t.Fatal(err)
	}
	if err := CheckRange(0, 1000); err != mtproto.ErrLimitInvalid {
		t.Fatal(err)
	}
	if err := CheckRange(0, 12288); err != mtproto.ErrLimitInvalid {
		t.Fatal(err)
	}
	if err := CheckRange(megabyte-4096, 8192); err != mtproto.ErrLimitInvalid {
		t.Fatal(err)
	}
}

func TestReuploadThenReadAndHash(t *testing.T) {
	b := New()
	b.at(time.Unix(1_700_000_000, 0))
	plain := bytes.Repeat([]byte("tellomi-cdn"), 12000)
	token, request, err := b.Stage(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Get(token, 0, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_upload_cdnFileReuploadNeeded || !bytes.Equal(got.GetRequestToken(), request) {
		t.Fatalf("%s %d", got.GetPredicateName(), len(got.GetRequestToken()))
	}
	if _, err = b.Reupload(token, []byte("nope")); err != mtproto.ErrRequestTokenInvalid {
		t.Fatal(err)
	}
	hashes, err := b.Reupload(token, request)
	if err != nil || len(hashes) != 2 {
		t.Fatalf("%v %d", err, len(hashes))
	}
	file, err := b.Get(token, 8192, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if file.GetPredicateName() != mtproto.Predicate_upload_cdnFile || len(file.GetBytes()) != 4096 {
		t.Fatalf("%s %d", file.GetPredicateName(), len(file.GetBytes()))
	}
	obj := b.files[hex.EncodeToString(token)]
	sum := sha256.Sum256(obj.cipher[:hashChunk])
	if !bytes.Equal(hashes[0].GetHash(), sum[:]) || hashes[0].GetLimit() != hashChunk || hashes[0].GetOffset_INT64() != 0 {
		t.Fatal("hash")
	}
	if hashes[1].GetOffset_INT64() != hashChunk || hashes[1].GetLimit() != int32(len(obj.cipher)-hashChunk) {
		t.Fatal("second hash")
	}
	if !bytes.Equal(decryptAt(file.GetBytes(), obj.key, obj.iv, 8192), plain[8192:8192+4096]) {
		t.Fatal("decrypt")
	}
	aligned := int64(len(plain) / 4096 * 4096)
	tail, err := b.Get(token, aligned, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.GetBytes()) != len(plain)%4096 {
		t.Fatalf("tail %d", len(tail.GetBytes()))
	}
	end, err := b.Get(token, aligned+4096, 4096)
	if err != nil || len(end.GetBytes()) != 0 {
		t.Fatalf("%v %d", err, len(end.GetBytes()))
	}
	part, err := b.Hashes(token, 1)
	if err != nil || len(part) != 1 {
		t.Fatal(err, len(part))
	}
	none, err := b.Hashes(token, int64(len(plain)))
	if err != nil || len(none) != 0 {
		t.Fatal(err, len(none))
	}
	if _, err = b.Get([]byte("missing"), 0, 4096); err != mtproto.ErrFileTokenInvalid {
		t.Fatal(err)
	}
	if _, err = b.Reupload(token, nil); err != mtproto.ErrRequestTokenInvalid {
		t.Fatal(err)
	}
	b.at(time.Unix(1_700_000_000, 0).Add(ttl + time.Second))
	if _, err = b.Hashes(token, 0); err != mtproto.ErrFileTokenInvalid {
		t.Fatal(err)
	}
}

func decryptAt(chunk, key, iv []byte, offset int64) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	ctrIV := append([]byte(nil), iv...)
	binary.BigEndian.PutUint32(ctrIV[len(ctrIV)-4:], uint32(offset/16))
	out := make([]byte, len(chunk))
	cipher.NewCTR(block, ctrIV).XORKeyStream(out, chunk)
	return out
}
