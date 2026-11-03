package account_helper

import (
	"context"
	"os"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/account/deletion"
	"github.com/teamgram/teamgram-server/app/bff/account/deletion/wipe"
	accountcore "github.com/teamgram/teamgram-server/app/bff/account/internal/core"
	accountsvc "github.com/teamgram/teamgram-server/app/bff/account/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/accountdeletion"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/code/conf"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

// StartWebDeletion serves the account-deletion page and the /v1 endpoints
// from the bff process, and expires seven-day deletions with the same purge
// as account.deleteAccount. TELLOMI_ACCOUNT_DELETE_ADDR=off skips the HTTP
// listener; the reaper still runs.
func StartWebDeletion(mysqlDSN string, smsCfg *conf.SmsVerifyCodeConfig, acct *accountsvc.Service, pts func(context.Context, int64, int32) (int32, error)) {
	if acct == nil {
		return
	}
	addr := os.Getenv("TELLOMI_ACCOUNT_DELETE_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8802"
	}
	deletion.ConfigureMySQL(mysqlDSN)
	deletion.SetPts(pts)
	svc := acct.GetServiceContext()
	var cache wipe.Cache
	if c := svc.Dao.Cache(); c != nil {
		cache = c
	}
	if err := deletion.ConfigureWipe(mysqlDSN, svc.Dao.SyncClient, cache); err != nil {
		logx.Errorf("account deletion wipe: %v", err)
	} else {
		deletion.StartGC(context.Background(), time.Minute)
	}
	deletion.StartReaper(context.Background(), time.Minute, func(ctx context.Context, userID int64, reason string) error {
		return accountcore.New(ctx, svc).PurgeAccount(userID, reason)
	})
	if addr == "off" {
		return
	}
	cfg := verify.Load()
	fixed := ""
	if cfg.SMS.Provider != verify.ProviderAliyun {
		fixed = "12345"
		if smsCfg != nil && smsCfg.FixedCode != "" {
			fixed = smsCfg.FixedCode
		}
	}
	sms := code.NewVerifyCode(smsCfg)
	// Same sender auth.sendCode uses for an email in phone_number.
	emailSender := verify.NewEmailSender(cfg.Email)
	var allow func(context.Context, string, string, string) error
	if cache := svc.Dao.Cache(); cache != nil {
		smsLim := verify.NewLimiter(cache, cfg.Limits)
		emailLim := verify.NewLimiter(cache, cfg.EmailLimits)
		allow = func(ctx context.Context, kind, target, ip string) error {
			lim := smsLim
			if kind == "email" {
				lim = emailLim
			} else {
				kind = "sms"
			}
			return lim.AllowSend(ctx, kind, target, ip)
		}
	}
	accountdeletion.Listen(addr, accountdeletion.Deps{
		// phoneOrEmail is a normalized phone, or an email. The user service
		// reads users.email when the value contains "@".
		Lookup: func(ctx context.Context, phoneOrEmail string) (accountdeletion.Account, bool, error) {
			u, err := svc.UserClient.UserGetImmutableUserByPhone(ctx, &userpb.TLUserGetImmutableUserByPhone{Phone: phoneOrEmail})
			if err != nil {
				if status.Convert(err).Message() == "PHONE_NUMBER_UNOCCUPIED" {
					return accountdeletion.Account{}, false, nil
				}
				return accountdeletion.Account{}, false, err
			}
			if u == nil || u.Deleted() {
				return accountdeletion.Account{}, false, nil
			}
			return accountdeletion.Account{UserID: u.Id(), Username: u.Username(), Phone: u.Phone()}, true, nil
		},
		Password: func(ctx context.Context, userID int64) (bool, int64, string, error) {
			return password.Info(ctx, userID)
		},
		CheckPassword: password.CheckPlaintext,
		Purge: func(ctx context.Context, userID int64, reason string) error {
			return accountcore.New(ctx, svc).PurgeAccount(userID, reason)
		},
		SendSMS: func(ctx context.Context, phone, codeDigits string) error {
			_, err := sms.SendSmsVerifyCode(ctx, phone, codeDigits, "")
			return err
		},
		SendEmail: func(ctx context.Context, email, codeDigits string) error {
			return emailSender.SendCode(ctx, email, codeDigits)
		},
		AllowSend:    allow,
		FixedCode:    fixed,
		CodeLen:      cfg.SMS.CodeLength,
		EmailCodeLen: cfg.Email.CodeLength,
	})
}
