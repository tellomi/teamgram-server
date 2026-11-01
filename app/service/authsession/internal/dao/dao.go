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
	"os"
	"strings"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/config"

	"github.com/oschwald/geoip2-golang"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/kv"
)

var (
	mmdb string
)

func init() {
	flag.StringVar(&mmdb, "mmdb", "./GeoLite2-City.mmdb", "mmdb")
}

type Dao struct {
	*Mysql
	sqlc.CachedConn
	kv   kv.Store
	MMDB *geoip2.Reader
}

// geoIPPath is the MaxMind GeoLite2-City database used to fill
// authorization.country and authorization.region.
// MMDB or GEOIP_MMDB overrides the -mmdb flag (default ./GeoLite2-City.mmdb,
// resolved from the authsession process working directory).
func geoIPPath() string {
	if p := strings.TrimSpace(os.Getenv("MMDB")); p != "" {
		return p
	}
	if p := strings.TrimSpace(os.Getenv("GEOIP_MMDB")); p != "" {
		return p
	}
	return mmdb
}

func openGeoIP() *geoip2.Reader {
	path := geoIPPath()
	reader, err := geoip2.Open(path)
	if err != nil {
		logx.Errorf("authsession GeoIP database not loaded from %q: %v. account.getAuthorizations cannot resolve a country until a MaxMind GeoLite2-City.mmdb is available. Set -mmdb, MMDB, or GEOIP_MMDB to that file (cwd default is ./GeoLite2-City.mmdb).", path, err)
		return nil
	}
	logx.Infof("authsession GeoIP database loaded from %q", path)
	return reader
}

func New(c config.Config) *Dao {
	db := sqlx.NewMySQL(&c.Mysql)

	return &Dao{
		Mysql:      newMysqlDao(db),
		CachedConn: sqlc.NewConn(db, c.Cache),
		kv:         kv.NewStore(c.KV),
		MMDB:       openGeoIP(),
	}
}
