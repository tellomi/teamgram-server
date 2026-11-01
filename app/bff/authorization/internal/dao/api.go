// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"strings"

	"github.com/teamgram/teamgram-server/pkg/geoip"
)

func (d *Dao) CheckApiIdAndHash(apiId int32, apiHash string) error {
	// TODO(@benqi): check api_id and api_hash
	// 400	API_ID_INVALID	API ID无效
	// 400	API_ID_PUBLISHED_FLOOD	这个API ID已发布在某个地方，您现在不能使用

	_ = apiId
	_ = apiHash

	return nil
}

// GetCountryAndRegionByIp is the location line on the sign-in service
// notice. The first string is the place ("中国 四川 成都", or the country
// alone when the address is abroad). The second string is authorization.region.
// A missing database or an unknown address returns the client IP and an
// empty region. The call does not fail sign-in.
func (d *Dao) GetCountryAndRegionByIp(ip string) (string, string) {
	if d != nil && d.geo != nil {
		country, region := d.geo.Lookup(ip)
		if country != "" && !strings.EqualFold(country, "UNKNOWN") {
			return geoip.LocationLine(country, region), region
		}
	}
	fallback := ""
	if parsed := geoip.ParseClientIP(ip); parsed != nil {
		fallback = parsed.String()
	}
	if strings.EqualFold(fallback, "UNKNOWN") {
		fallback = ""
	}
	return fallback, ""
}
