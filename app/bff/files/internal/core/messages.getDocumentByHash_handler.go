// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"
	"encoding/hex"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/teamgram/teamgram-server/pkg/mediaotel"

	"github.com/zeromicro/go-zero/core/logx"
)

// MessagesGetDocumentByHash
// messages.getDocumentByHash#b1f2061f sha256:bytes size:long mime_type:string = Document;
// messages.getDocumentByHash#338e2464 sha256:bytes size:int mime_type:string = Document;
//
// https://core.telegram.org/method/messages.getDocumentByHash
//
// Returns an existing document when sha256, size and mime match, or
// documentEmpty. The returned record keeps its own access_hash. Uploading
// the same bytes still creates a new document row; this method is the only
// path that hands back an existing one, and only to the caller who already
// has the content hash.

// hashFinder is the content-address lookup. A nil *mediablob.Store is a
// valid value: FindDocument returns no row.
type hashFinder interface {
	FindDocument(ctx context.Context, shaHex string, size int64, mime string) (int64, error)
}

// documentGetter loads the document row for a hash hit.
type documentGetter interface {
	MediaGetDocument(ctx context.Context, in *media.TLMediaGetDocument) (*mtproto.Document, error)
}

func (c *FilesCore) MessagesGetDocumentByHash(in *mtproto.TLMessagesGetDocumentByHash) (*mtproto.Document, error) {
	var blobs hashFinder
	var docs documentGetter
	if c != nil && c.svcCtx != nil {
		blobs = c.svcCtx.Blobs
		if c.svcCtx.Dao != nil {
			docs = c.svcCtx.Dao.MediaClient
		}
	}
	var log logx.Logger
	if c != nil {
		log = c.Logger
	}
	ctx := context.Background()
	if c != nil && c.ctx != nil {
		ctx = c.ctx
	}
	return lookupDocumentByHash(ctx, log, blobs, docs, in)
}

// lookupDocumentByHash runs the hash lookup and records hit versus
// documentEmpty on one span. The span does not carry the sha256, the
// mime string, or the document access_hash.
func lookupDocumentByHash(ctx context.Context, log logx.Logger, blobs hashFinder, docs documentGetter, in *mtproto.TLMessagesGetDocumentByHash) (*mtproto.Document, error) {
	size := documentByHashSize(in)
	ctx, finish := mediaotel.BeginDocumentLookup(ctx)
	doc, err := findDocumentByHash(ctx, log, blobs, docs, in)
	finish(documentLookupResult(doc, err), size)
	return doc, err
}

func findDocumentByHash(ctx context.Context, log logx.Logger, blobs hashFinder, docs documentGetter, in *mtproto.TLMessagesGetDocumentByHash) (*mtproto.Document, error) {
	empty := mtproto.MakeTLDocumentEmpty(nil).To_Document()
	if in == nil || blobs == nil {
		return empty, nil
	}
	sum := in.GetSha256()
	if len(sum) != 32 || in.GetMimeType() == "" {
		return empty, nil
	}
	id, err := blobs.FindDocument(ctx, hex.EncodeToString(sum), documentByHashSize(in), in.GetMimeType())
	if err != nil {
		if log != nil {
			log.Errorf("messages.getDocumentByHash: %v", err)
		}
		return nil, err
	}
	if id == 0 || docs == nil {
		return empty, nil
	}
	doc, err := docs.MediaGetDocument(ctx, &media.TLMediaGetDocument{Id: id})
	if err != nil {
		if log != nil {
			log.Errorf("messages.getDocumentByHash document %d: %v", id, err)
		}
		return nil, err
	}
	if doc == nil || doc.GetPredicateName() == mtproto.Predicate_documentEmpty || doc.GetId() == 0 {
		return empty, nil
	}
	return doc, nil
}

func documentLookupResult(doc *mtproto.Document, err error) string {
	if err != nil {
		return mediaotel.LookupError
	}
	if doc == nil || doc.GetId() == 0 || doc.GetPredicateName() == mtproto.Predicate_documentEmpty {
		return mediaotel.LookupEmpty
	}
	return mediaotel.LookupHit
}

// documentByHashSize reads the size field of the constructor the client sent.
// Layer 122 used a 32-bit size; the current method uses a 64-bit size.
func documentByHashSize(in *mtproto.TLMessagesGetDocumentByHash) int64 {
	if in == nil {
		return 0
	}
	if in.GetConstructor() == mtproto.CRC32_messages_getDocumentByHash_338e2464 {
		return int64(in.GetSize2_INT32())
	}
	if in.GetSize2_INT64() != 0 {
		return in.GetSize2_INT64()
	}
	return int64(in.GetSize2_INT32())
}
