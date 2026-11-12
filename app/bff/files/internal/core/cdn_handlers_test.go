package core

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/files/cdn"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestHelpGetCdnConfigPublishesNoAddress(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	c := &FilesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
	}
	got, err := c.HelpGetCdnConfig(&mtproto.TLHelpGetCdnConfig{Constructor: mtproto.CRC32_help_getCdnConfig})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetPredicateName() != mtproto.Predicate_cdnConfig {
		t.Fatalf("%v", got)
	}
	if len(got.GetPublicKeys()) != 0 {
		t.Fatalf("public_keys %d", len(got.GetPublicKeys()))
	}
	text := got.String()
	for _, ip := range []string{"8.156.40.105", "8.137.198.240", "10.20.0.218"} {
		if strings.Contains(text, ip) {
			t.Fatalf("cdn config contains %s", ip)
		}
	}
}

func TestCdnHandlersWithoutKeys(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	c := &FilesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 4},
	}
	if _, err := c.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{Limit: 1000}); err != mtproto.ErrLimitInvalid {
		t.Fatal(err)
	}
	if _, err := c.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{Limit: 4096}); err != mtproto.ErrFileTokenInvalid {
		t.Fatal(err)
	}
	if _, err := c.UploadGetCdnFileHashes(&mtproto.TLUploadGetCdnFileHashes{}); err != mtproto.ErrFileTokenInvalid {
		t.Fatal(err)
	}
	if _, err := c.UploadReuploadCdnFile(&mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}}); err != mtproto.ErrFileTokenInvalid {
		t.Fatal(err)
	}
	bot := &FilesCore{ctx: c.ctx, Logger: c.Logger, MD: &metadata.RpcMetadata{UserId: 9, IsBot: true}}
	if _, err := bot.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{Limit: 4096}); err != mtproto.ErrBotMethodInvalid {
		t.Fatal(err)
	}
}

func TestCdnHandlersWithKeys(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "1|-----BEGIN PUBLIC KEY-----MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA")
	book := cdn.New()
	cdn.Install(book)
	t.Cleanup(func() { cdn.Install(cdn.New()) })
	token, request, err := book.Stage([]byte("abcdefghijklmnopqrstuvwxyz012345"))
	if err != nil {
		t.Fatal(err)
	}
	c := &FilesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 4},
	}
	redir, err := c.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{
		FileToken: token,
		Limit:     4096,
	})
	if err != nil || redir.GetPredicateName() != mtproto.Predicate_upload_cdnFileReuploadNeeded {
		t.Fatal(err, redir)
	}
	hashes, err := c.UploadReuploadCdnFile(&mtproto.TLUploadReuploadCdnFile{
		FileToken:    token,
		RequestToken: request,
	})
	if err != nil || len(hashes.GetDatas()) != 1 || len(hashes.Datas[0].GetHash()) != 32 {
		t.Fatal(err, hashes)
	}
	file, err := c.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{
		FileToken: token,
		Limit:     4096,
	})
	if err != nil || file.GetPredicateName() != mtproto.Predicate_upload_cdnFile || len(file.GetBytes()) == 0 {
		t.Fatal(err, file)
	}
	from, err := c.UploadGetCdnFileHashes(&mtproto.TLUploadGetCdnFileHashes{FileToken: token})
	if err != nil || len(from.GetDatas()) != 1 {
		t.Fatal(err, from)
	}
}
