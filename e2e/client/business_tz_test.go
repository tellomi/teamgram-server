package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBusinessHoursAcceptShanghai is help.getTimezonesList and
// account.updateBusinessWorkHours. Asia/Shanghai is an IANA id from the
// same database the picker lists, including when the image has no tzdata files.
func TestBusinessHoursAcceptShanghai(t *testing.T) {
	cfg := requireE2E(t)
	c, self := loginNamed(t, cfg, ProfileIOS, "biz_tz", "BizTz")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	listObj, err := c.Invoke(ctx, &mtproto.TLHelpGetTimezonesList{
		Constructor: mtproto.CRC32_help_getTimezonesList,
	})
	if err != nil {
		t.Fatal(err)
	}
	zones := timezonesOf(t, listObj)
	if len(zones) < 20 {
		t.Fatalf("timezones %d", len(zones))
	}
	var sawUTC, sawShanghai bool
	for _, zone := range zones {
		if zone.GetId() == "UTC" || zone.GetId() == "Etc/UTC" {
			sawUTC = true
		}
		if zone.GetId() == "Asia/Shanghai" && zone.GetName() != "" {
			sawShanghai = true
		}
	}
	if !sawUTC || !sawShanghai {
		t.Fatalf("utc=%v shanghai=%v count=%d", sawUTC, sawShanghai, len(zones))
	}

	hours := mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
		TimezoneId: "Asia/Shanghai",
		WeeklyOpen: []*mtproto.BusinessWeeklyOpen{
			mtproto.MakeTLBusinessWeeklyOpen(&mtproto.BusinessWeeklyOpen{
				StartMinute: 9 * 60,
				EndMinute:   18 * 60,
			}).To_BusinessWeeklyOpen(),
		},
	}).To_BusinessWorkHours()
	if _, err = c.Invoke(ctx, &mtproto.TLAccountUpdateBusinessWorkHours{
		Constructor:       mtproto.CRC32_account_updateBusinessWorkHours,
		BusinessWorkHours: hours,
	}); err != nil {
		t.Fatal(err)
	}
	fullObj, err := c.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUserSelf(),
	})
	if err != nil {
		t.Fatal(err)
	}
	full := fullUserOf(t, fullObj)
	if full.GetId() != self.ID {
		t.Fatalf("full user %d", full.GetId())
	}
	if got := full.GetBusinessWorkHours().GetTimezoneId(); got != "Asia/Shanghai" {
		t.Fatalf("work hours %q", got)
	}
	if _, err = c.Invoke(ctx, &mtproto.TLAccountUpdateBusinessWorkHours{
		Constructor: mtproto.CRC32_account_updateBusinessWorkHours,
		BusinessWorkHours: mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
			TimezoneId: "Not/AZone",
			WeeklyOpen: hours.GetWeeklyOpen(),
		}).To_BusinessWorkHours(),
	}); err == nil {
		t.Fatal("invalid zone accepted")
	} else if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "TIMEZONE_INVALID" {
		t.Fatalf("invalid zone: %v", err)
	}
}

func timezonesOf(t *testing.T, obj mtproto.TLObject) []*mtproto.Timezone {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLHelpTimezonesList:
		return v.GetTimezones()
	case *mtproto.Help_TimezonesList:
		return v.GetTimezones()
	default:
		t.Fatalf("timezones %T", obj)
		return nil
	}
}
