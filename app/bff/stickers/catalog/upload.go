package catalog

import (
	"context"
	"fmt"
	"strconv"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/zeromicro/go-zero/core/logx"
)

// uploadAssets writes seeded documents with the dfs S3 client.
// Bucket and key come from TELLOMI_S3_* via MinioConfig.ApplyEnv and
// resolveLocation, the same path upload.getFile uses for stickers and emoji:
// <TELLOMI_S3_BUCKET_PUBLIC_MEDIA>/stickers/<id>.dat and .../emoji/<id>.dat.
// Missing S3 settings skip the upload so the catalog can still be served.
func uploadAssets(ctx context.Context, packs []*Pack) error {
	client, err := objectstore.Open()
	if err != nil {
		return fmt.Errorf("sticker storage: %w", err)
	}
	if client == nil {
		return nil
	}
	seen := map[string]bool{}
	n := 0
	for _, p := range packs {
		for _, item := range p.Items {
			dedupe := item.StorageClass + "/" + item.ObjectPath
			if item.ObjectPath == "" || seen[dedupe] {
				continue
			}
			seen[dedupe] = true
			body, err := assetFS.ReadFile(item.AssetPath)
			if err != nil {
				return fmt.Errorf("read %s: %w", item.AssetPath, err)
			}
			if err = client.Put(ctx, item.StorageClass, item.ObjectPath, item.MIME, body); err != nil {
				return err
			}
			n++
		}
	}
	if err = uploadReactionAssets(ctx, client); err != nil {
		return err
	}
	logx.Infof("stickers: uploaded %d objects with the dfs S3 client", n)
	return nil
}

// uploadReactionAssets writes each availableReaction document into the public
// emoji prefix on Cloudflare R2 (or MinIO). upload.getFile also serves the
// same bytes from the binary, so a missing object cannot stall the session.
func uploadReactionAssets(ctx context.Context, client *objectstore.Client) error {
	if client == nil {
		return nil
	}
	seen := map[int64]bool{}
	for _, spec := range reactionSpecs() {
		for _, slot := range reactionSlots {
			id := reactionSlotID(spec.code, slot.name)
			if seen[id] {
				continue
			}
			seen[id] = true
			body, mime := reactionSlotBytes(spec.code, slot.name, slot.tgs)
			if len(body) == 0 {
				continue
			}
			path := strconv.FormatInt(id, 10) + ".dat"
			if err := client.Put(ctx, "emoji", path, mime, body); err != nil {
				return err
			}
		}
	}
	return nil
}
