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
	"flag"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/config"
	"github.com/teamgram/teamgram-server/pkg/geoip"

	"github.com/zeromicro/go-zero/core/stores/kv"
)

var (
	mmdb string
)

func init() {
	// -mmdb is the city database for addresses outside mainland China.
	// ip2region is loaded as well. Either file may be missing. See docs/ip2region.md.
	flag.StringVar(&mmdb, "mmdb", "./GeoLite2-City.mmdb", "mmdb")
}

type Dao struct {
	*Mysql
	sqlc.CachedConn
	kv  kv.Store
	geo *geoip.Resolver
}

// geoIPPath is the DB-IP Lite / GeoLite2 city file.
// MMDB, GEOIP_MMDB, or TELLOMI_GEOIP_MMDB overrides the -mmdb flag
// (default ./GeoLite2-City.mmdb, which is /app/bin/GeoLite2-City.mmdb
// when the process starts in /app/bin).
func geoIPPath() string {
	return geoip.MaxMindPath(mmdb)
}

func New(c config.Config) *Dao {
	db := mysqltls.NewMySQL(&c.Mysql)

	return &Dao{
		Mysql:      newMysqlDao(db),
		CachedConn: sqlc.NewConn(db, c.Cache),
		kv:         kv.NewStore(c.KV),
		geo:        geoip.Load(mmdb),
	}
}
