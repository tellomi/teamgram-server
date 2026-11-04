// Command seedofficialavatars gives the seeded official accounts (777000 to
// 777005) their profile photos through dfs, media and the user service, the
// same path photos.uploadProfilePhoto takes. Run it inside the teamgram
// container (host network, etcd on 127.0.0.1:2379) after the services are up:
//
//	/app/bin/seedofficialavatars            # accounts without a photo
//	/app/bin/seedofficialavatars -force -users 777001
//
// It is safe to run again: accounts that already have a photo are skipped
// unless -force is set.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/officialavatars"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	dfsclient "github.com/teamgram/teamgram-server/app/service/dfs/client"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/zrpc"
)

func main() {
	etcd := flag.String("etcd", envOr("TEAMGRAM_ETCD", "127.0.0.1:2379"), "comma separated etcd hosts")
	users := flag.String("users", "", "comma separated user ids (default: all official accounts)")
	force := flag.Bool("force", false, "upload a new photo even when the account already has one")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall timeout")
	flag.Parse()

	ids, err := parseIDs(*users)
	if err != nil {
		log.Fatal(err)
	}
	hosts := strings.Split(*etcd, ",")
	conf := func(key string) zrpc.RpcClientConf {
		return zrpc.RpcClientConf{Etcd: discov.EtcdConf{Hosts: hosts, Key: key}, NonBlock: false, Timeout: 60000}
	}
	s := &officialavatars.Seeder{
		Dfs:   dfsclient.NewDfsClient(rpcx.GetCachedRpcClient(conf("service.dfs"))),
		Media: mediaclient.NewMediaClient(rpcx.GetCachedRpcClient(conf("service.media"))),
		User:  userclient.NewUserClient(rpcx.GetCachedRpcClient(conf("service.biz_service"))),
		Force: *force,
		Logf:  log.Printf,
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	res, err := s.Run(ctx, ids)
	for _, r := range res {
		log.Printf("result user=%d photo_id=%d skipped=%v video=%v", r.UserID, r.PhotoID, r.Skipped, r.Video)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseIDs(s string) ([]int64, error) {
	var out []int64
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		id, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
