package catalog

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// seedPutConcurrency bounds how many public-media objects a startup
	// uploads at once. R2 from the app servers is about 0.8s per request;
	// a sequential rewrite of every emoji object exceeded the old 2-minute
	// deadline.
	seedPutConcurrency = 8
	// seedPutTimeout is the deadline for one object. A slow key does not
	// cancel the rest of the seed.
	seedPutTimeout = 30 * time.Second
)

type seedObject struct {
	class string
	path  string
	mime  string
	body  []byte
}

// seedStore is the subset of object storage the sticker seed uses.
type seedStore interface {
	Put(ctx context.Context, class, path, contentType string, body []byte) error
	List(ctx context.Context, class string) (map[string]objectstore.Head, error)
}

// uploadAssets writes seeded documents with the dfs S3 client.
// Bucket and key come from TELLOMI_S3_* via MinioConfig.ApplyEnv and
// resolveLocation, the same path upload.getFile uses for stickers and emoji:
// <TELLOMI_S3_BUCKET_PUBLIC_MEDIA>/stickers/<id>.dat and .../emoji/<id>.dat.
// Missing S3 settings skip the upload so the catalog can still be served.
// Unchanged objects (matching MD5 ETag and size) are not written again.
func uploadAssets(ctx context.Context, packs []*Pack) error {
	client, err := objectstore.Open()
	if err != nil {
		return fmt.Errorf("sticker storage: %w", err)
	}
	if client == nil {
		return nil
	}
	objs, err := seedObjects(packs)
	if err != nil {
		return err
	}
	return uploadChanged(ctx, client, objs)
}

func seedObjects(packs []*Pack) ([]seedObject, error) {
	seen := map[string]bool{}
	var out []seedObject
	add := func(class, path, mime string, body []byte) {
		if path == "" || len(body) == 0 {
			return
		}
		key := class + "/" + path
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, seedObject{class: class, path: path, mime: mime, body: body})
	}
	for _, p := range packs {
		for _, item := range p.Items {
			// Packs imported with -dir have no embedded asset; their bytes
			// were uploaded by uploadPrepared, so the seed pass skips them.
			if item.ObjectPath == "" || item.AssetPath == "" {
				continue
			}
			body, err := assetFS.ReadFile(item.AssetPath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", item.AssetPath, err)
			}
			add(item.StorageClass, item.ObjectPath, item.MIME, body)
		}
	}
	for _, spec := range reactionSpecs() {
		for _, slot := range reactionSlots {
			body, mime := reactionSlotBytes(spec.code, slot.name, slot.tgs)
			add("emoji", strconv.FormatInt(reactionSlotID(spec.code, slot.name), 10)+".dat", mime, body)
		}
		// Rewrite the previous center_icon key as well when it changed. A
		// store that still has the full-bleed file would otherwise keep
		// serving it. A matching ETag is left in place.
		body, mime := reactionSlotBytes(spec.code, "center", true)
		add("emoji", strconv.FormatInt(legacyCenterID(spec.code), 10)+".dat", mime, body)
	}
	return out, nil
}

func uploadChanged(ctx context.Context, store seedStore, objs []seedObject) error {
	if len(objs) == 0 {
		return nil
	}
	heads := listHeads(ctx, store, objs)
	var todo []seedObject
	for _, obj := range objs {
		if seedUnchanged(heads[obj.class+"/"+obj.path], obj.body) {
			continue
		}
		todo = append(todo, obj)
	}
	logx.Infof("stickers: %d objects, %d changed, %d already match", len(objs), len(todo), len(objs)-len(todo))
	if len(todo) == 0 {
		return nil
	}
	sem := make(chan struct{}, seedPutConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	uploaded := 0
	for _, obj := range todo {
		if ctx.Err() != nil {
			break
		}
		obj := obj
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			putCtx, cancel := context.WithTimeout(ctx, seedPutTimeout)
			err := store.Put(putCtx, obj.class, obj.path, obj.mime, obj.body)
			cancel()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				logx.Errorf("stickers: put %s/%s: %v", obj.class, obj.path, err)
				if first == nil {
					first = err
				}
				return
			}
			uploaded++
		}()
	}
	wg.Wait()
	logx.Infof("stickers: uploaded %d objects", uploaded)
	if ctx.Err() != nil && first == nil {
		return ctx.Err()
	}
	return first
}

func listHeads(ctx context.Context, store seedStore, objs []seedObject) map[string]objectstore.Head {
	classes := map[string]bool{}
	for _, obj := range objs {
		classes[obj.class] = true
	}
	out := map[string]objectstore.Head{}
	for class := range classes {
		heads, err := store.List(ctx, class)
		if err != nil {
			logx.Errorf("stickers: list %s: %v (uploading that class)", class, err)
			continue
		}
		for path, head := range heads {
			out[class+"/"+path] = head
		}
	}
	return out
}

// seedUnchanged reports whether the stored object is the same bytes.
// Only a 32-hex MD5 ETag counts. Multipart ETags and the test double-quote
// placeholders are treated as different so a changed file is still uploaded.
func seedUnchanged(head objectstore.Head, body []byte) bool {
	if head.Size == 0 || head.Size != int64(len(body)) || head.ETag == "" {
		return false
	}
	etag := strings.TrimPrefix(strings.TrimSpace(head.ETag), "W/")
	etag = strings.Trim(etag, `"`)
	if len(etag) != 32 || strings.Contains(etag, "-") {
		return false
	}
	sum := md5.Sum(body)
	return strings.EqualFold(etag, hex.EncodeToString(sum[:]))
}
