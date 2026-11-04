// Command seedofficialavatars gives the seeded official accounts (777000 to
// 777005) their profile photos through dfs, media and the user service, the
// same path photos.uploadProfilePhoto takes. Run it inside the teamgram
// container (host network, etcd on 127.0.0.1:2379) after the services are up:
//
//	/app/bin/seedofficialavatars            # accounts without a photo
//	/app/bin/seedofficialavatars -force -users 777001
//	TEAMGRAM_MYSQL_DSN=... /app/bin/seedofficialavatars -users none -channels all
//
// -channels seeds the official channels (1073800001 to 1073800004) as their
// owner 777002 through the MySQL channel service (channels.editPhoto's
// ApplyPhoto), so the change gets a "photo updated" service message and a
// new channel version like an edit from a client.
//
// It is safe to run again: accounts that already have a photo are skipped
// unless -force is set.
package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/officialavatars"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	dfsclient "github.com/teamgram/teamgram-server/app/service/dfs/client"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/zrpc"
)

func main() {
	etcd := flag.String("etcd", envOr("TEAMGRAM_ETCD", "127.0.0.1:2379"), "comma separated etcd hosts")
	users := flag.String("users", "", "comma separated user ids (default: all official accounts)")
	channels := flag.String("channels", "", "comma separated channel ids, or all (default: none)")
	dsn := flag.String("mysql", os.Getenv("TEAMGRAM_MYSQL_DSN"), "MySQL DSN for -channels (default $TEAMGRAM_MYSQL_DSN)")
	force := flag.Bool("force", false, "upload a new photo even when the account already has one")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall timeout")
	flag.Parse()

	skipUsers := *users == "none"
	if skipUsers {
		*users = ""
	}
	ids, err := parseIDs(*users)
	if err != nil {
		log.Fatal(err)
	}
	var chIDs []int64
	if *channels != "" && *channels != "all" {
		chIDs, err = parseIDs(*channels)
	}
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
	if !skipUsers {
		res, err := s.Run(ctx, ids)
		for _, r := range res {
			log.Printf("result user=%d photo_id=%d skipped=%v video=%v", r.UserID, r.PhotoID, r.Skipped, r.Video)
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	if *channels == "" {
		return
	}
	if *dsn == "" {
		log.Fatal("-channels needs -mysql or TEAMGRAM_MYSQL_DSN")
	}
	store, err := openChannels(*dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer store.db.Close()
	cres, err := s.RunChannels(ctx, store, chIDs)
	for _, r := range cres {
		log.Printf("result channel=%d photo_id=%d skipped=%v", r.ChannelID, r.PhotoID, r.Skipped)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// mysqlChannels sets channel photos with the MySQL channel service.
type mysqlChannels struct {
	db  *sql.DB
	svc *channel.Service
}

func openChannels(dsn string) (*mysqlChannels, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	svc, err := channel.OpenMySQL(dsn)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &mysqlChannels{db: db, svc: svc}, nil
}

func (m *mysqlChannels) row(ctx context.Context, id int64) (hash, photo int64, err error) {
	err = m.db.QueryRowContext(ctx, `SELECT access_hash, photo_id FROM channels WHERE id=? AND deleted=0`, id).Scan(&hash, &photo)
	return
}

func (m *mysqlChannels) ChannelPhotoID(ctx context.Context, id int64) (int64, error) {
	_, photo, err := m.row(ctx, id)
	return photo, err
}

func (m *mysqlChannels) ApplyChannelPhoto(ctx context.Context, actor, id int64, p *mtproto.Photo) error {
	hash, _, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: id, AccessHash: hash}).To_InputChannel()
	_, err = m.svc.ApplyPhoto(ctx, actor, 0, in, p.GetId(), p)
	return err
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
