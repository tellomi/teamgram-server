/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"context"
	"log"

	"github.com/bwmarrin/snowflake"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/config"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/seqfloor"
	"github.com/zeromicro/go-zero/core/stores/kv"
)

type Dao struct {
	*snowflake.Node
	KV     kv.Store
	Floors *seqfloor.Cache
}

func New(c config.Config) *Dao {
	var (
		err error
		d   = new(Dao)
	)

	d.Node, err = snowflake.NewNode(c.NodeId)
	if err != nil {
		log.Fatal("new snowflake node error: ", err)
	}
	d.KV = kv.NewStore(c.SeqIDGen)

	return d
}

// AllocSeq returns the next n sequence values for key.
// When Floors was loaded at startup, a missing or low Redis counter is
// raised to MAX(id) from MySQL before the increment. Without that, INCR on
// a flushed key starts at 1 and new message ids sort above older ones.
func (d *Dao) AllocSeq(ctx context.Context, key string, n int64) (int64, error) {
	if d.Floors == nil {
		return d.KV.IncrbyCtx(ctx, key, n)
	}
	return seqfloor.Alloc(ctx, d.KV, d.Floors, key, n)
}
