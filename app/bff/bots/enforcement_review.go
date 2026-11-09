package bots

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/service/biz/report"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"

	_ "github.com/go-sql-driver/mysql"
)

// StartEnforcementReview connects a stored user report to Tellomi AI and the
// admin allowlist. TELLOMI_ENFORCEMENT_REVIEW unset returns before any client
// is built. The mysql store, the AI key, and at least one admin username are
// all required. A missing piece is logged and does not stop bff.
func StartEnforcementReview(conf zrpc.RpcClientConf, dsn string) {
	if strings.TrimSpace(os.Getenv(enforcement.EnvReview)) != "1" {
		return
	}
	svc := enforcement.Current()
	if svc == nil {
		logx.Error("enforcement: review is on but the mysql store is not installed")
		return
	}
	if !tellomiai.Advertised() {
		logx.Error("enforcement: review is on but TELLOMI_AI is off or the API key is empty")
		return
	}
	admins := enforcement.ParseAdmins(os.Getenv(enforcement.EnvAdmins))
	if len(admins) == 0 {
		logx.Error("enforcement: TELLOMI_ENFORCEMENT_ADMINS is empty")
		return
	}
	dir, err := openChatDir(dsn)
	if err != nil {
		logx.Error("enforcement: chat directory is unavailable")
		return
	}
	timeout := 12 * time.Second
	if raw := strings.TrimSpace(os.Getenv(tellomiai.EnvTimeoutSeconds)); raw != "" {
		if n, nerr := strconv.Atoi(raw); nerr == nil && n > 0 && n <= 60 {
			timeout = time.Duration(n) * time.Second
		}
	}
	ai := tellomiai.NewClient(
		os.Getenv(tellomiai.EnvBaseURL),
		os.Getenv(tellomiai.EnvAPIKey),
		os.Getenv(tellomiai.EnvModelPrimary),
		os.Getenv(tellomiai.EnvModelSmall),
		os.Getenv(tellomiai.EnvModelFallback),
		timeout,
	)
	c := &enforcement.Confirmer{
		Svc: svc, AI: ai, Model: os.Getenv(tellomiai.EnvModelPrimary),
		Admins: admins, Threshold: enforcement.ConfidenceFromEnv(), Dir: dir,
		Send: caseSender{c: msg_client.NewMsgClient(rpcx.GetCachedRpcClient(conf))}.Send,
	}
	enforcement.UseConfirmer(c)
	report.SetAccountHook(func(ctx context.Context, row *report.Row, ev report.Evidence) {
		c.OnAccount(ctx, dir.accountInput(ctx, row, ev))
	})
	logx.Infof("enforcement: review on admins=%s", strings.Join(admins, ","))
}

type caseSender struct {
	c msg_client.MsgClient
}

func (s caseSender) Send(ctx context.Context, to int64, text string) (int32, error) {
	if s.c == nil {
		return 0, errNoMsg
	}
	text = tellomiai.WithLabel(text)
	up, err := s.c.MsgSendMessageV2(ctx, buildAIMessage(to, text, true, time.Now().UnixNano()))
	if err != nil {
		return 0, err
	}
	return messageID(up), nil
}

func messageID(up *mtproto.Updates) int32 {
	if up == nil {
		return 0
	}
	for _, u := range up.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m != nil && m.GetId() != 0 {
			return m.GetId()
		}
	}
	return 0
}

type chatDir struct {
	db *sql.DB
}

func openChatDir(dsn string) (*chatDir, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("empty dsn")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &chatDir{db: db}, nil
}

func (d *chatDir) Username(ctx context.Context, userID int64) (string, error) {
	if d == nil || d.db == nil || userID == 0 {
		return "", nil
	}
	var name string
	err := d.db.QueryRowContext(ctx, `SELECT username FROM username WHERE peer_type=? AND peer_id=? AND deleted=0 ORDER BY order2 DESC LIMIT 1`, mtproto.PEER_USER, userID).Scan(&name)
	if err == sql.ErrNoRows {
		err = d.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id=?`, userID).Scan(&name)
	}
	if err == sql.ErrNoRows {
		return "", nil
	}
	return name, err
}

func (d *chatDir) UserID(ctx context.Context, username string) (int64, error) {
	if d == nil || d.db == nil {
		return 0, nil
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return 0, nil
	}
	var id int64
	err := d.db.QueryRowContext(ctx, `SELECT peer_id FROM username WHERE username=? AND peer_type=? AND deleted=0 LIMIT 1`, username, mtproto.PEER_USER).Scan(&id)
	if err == sql.ErrNoRows {
		err = d.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username=? LIMIT 1`, username).Scan(&id)
	}
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (d *chatDir) accountInput(ctx context.Context, row *report.Row, ev report.Evidence) enforcement.AccountInput {
	if row == nil {
		return enforcement.AccountInput{}
	}
	in := enforcement.AccountInput{ReportID: row.ID, SubjectUserID: row.SubjectUserID, Reason: row.Reason, Note: row.Message}
	var user, first, about string
	_ = d.db.QueryRowContext(ctx, `SELECT IFNULL(username,''), IFNULL(first_name,''), IFNULL(about,'') FROM users WHERE id=?`, row.SubjectUserID).Scan(&user, &first, &about)
	in.Label = strconv.FormatInt(row.SubjectUserID, 10)
	if user != "" {
		in.Label += " @" + user
	}
	if first != "" {
		in.Label += " " + first
	}
	in.Profile = in.Label
	if about != "" {
		in.Profile += "\n" + about
	}
	for _, item := range ev.Items {
		if strings.TrimSpace(item.Text) != "" {
			in.Messages = append(in.Messages, item.Text)
		}
	}
	if len(in.Messages) == 0 {
		in.Messages = append(in.Messages, d.recent(ctx, row.Reporter, row.SubjectUserID)...)
		in.Messages = append(in.Messages, d.recent(ctx, row.SubjectUserID, row.Reporter)...)
	}
	rows, err := d.db.QueryContext(ctx, `SELECT reason, message FROM reports WHERE subject_user_id=? AND id<>? ORDER BY id DESC LIMIT 8`, row.SubjectUserID, row.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var reason, message string
			if rows.Scan(&reason, &message) != nil {
				continue
			}
			line := strings.TrimSpace(reason)
			if message != "" {
				line += " " + message
			}
			if line != "" {
				in.Prior = append(in.Prior, line)
			}
		}
	}
	return in
}

func (d *chatDir) recent(ctx context.Context, owner, peer int64) []string {
	if owner == 0 || peer == 0 {
		return nil
	}
	rows, err := d.db.QueryContext(ctx, `SELECT message FROM messages WHERE user_id=? AND peer_type=? AND peer_id=? AND deleted=0 ORDER BY user_message_box_id DESC LIMIT 5`, owner, mtproto.PEER_USER, peer)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var text string
		if rows.Scan(&text) == nil && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}
