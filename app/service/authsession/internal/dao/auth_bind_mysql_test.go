package dao

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/config"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/redis"

	_ "github.com/go-sql-driver/mysql"
)

func TestAuthBindMySQL(t *testing.T) {
	h := openAuthBindHarness(t)
	ctx := context.Background()

	t.Run("same user is idempotent", func(t *testing.T) {
		key := h.insertPerm(t)
		const userID = int64(71)
		hash, err := h.dao.BindAuthKeyUser(ctx, key, userID)
		if err != nil || hash == 0 {
			t.Fatalf("first bind hash=%d err=%v", hash, err)
		}
		h.setCreated(t, key, userID, 100)
		hash2, err := h.dao.BindAuthKeyUser(ctx, key, userID)
		if err != nil || hash2 == 0 {
			t.Fatalf("second bind hash=%d err=%v", hash2, err)
		}
		if hash2 != hash {
			t.Fatalf("rebind hash %d, want the stored hash %d", hash2, hash)
		}
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].userID != userID || rows[0].deleted != 0 || rows[0].created != 100 || rows[0].hash != hash {
			t.Fatalf("same-user rebind changed the row: %+v", rows)
		}
	})

	t.Run("different user is rejected and the row stays", func(t *testing.T) {
		key := h.insertPerm(t)
		const owner, other = int64(81), int64(82)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, owner); err != nil {
			t.Fatal(err)
		}
		before := h.binds(t, key)
		genBefore, err := h.dao.authBindGen(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.dao.GetCacheAuthData(ctx, key); err != nil {
			t.Fatal(err)
		}
		hash, err := h.dao.BindAuthKeyUser(ctx, key, other)
		if !errors.Is(err, ErrAuthKeyUserConflict) || hash != 0 {
			t.Fatalf("conflict hash=%d err=%v", hash, err)
		}
		if got := h.binds(t, key); !sameBinds(before, got) {
			t.Fatalf("conflict wrote the row: before %+v after %+v", before, got)
		}
		genAfter, err := h.dao.authBindGen(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if genAfter != genBefore {
			t.Fatalf("conflict moved the cache generation %d -> %d", genBefore, genAfter)
		}
		got, err := h.dao.GetCacheAuthData(ctx, key)
		if err != nil || got.UserId() != owner {
			t.Fatalf("cache after conflict: user %d err %v", got.UserId(), err)
		}
	})

	t.Run("revoked other user can be replaced", func(t *testing.T) {
		key := h.insertPerm(t)
		const first, second = int64(91), int64(92)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, first); err != nil {
			t.Fatal(err)
		}
		if !h.dao.UnbindAuthUser(ctx, key, first) {
			t.Fatal("unbind")
		}
		if _, err := h.dao.BindAuthKeyUser(ctx, key, second); err != nil {
			t.Fatal(err)
		}
		var active, revoked int
		for _, row := range h.binds(t, key) {
			if row.deleted == 0 && row.userID == second {
				active++
			}
			if row.deleted == 1 && row.userID == first {
				revoked++
			}
		}
		if active != 1 || revoked != 1 {
			t.Fatalf("revoked-owner policy: %+v", h.binds(t, key))
		}
	})

	t.Run("revoked same user is revived in place", func(t *testing.T) {
		key := h.insertPerm(t)
		const userID = int64(101)
		first, err := h.dao.BindAuthKeyUser(ctx, key, userID)
		if err != nil {
			t.Fatal(err)
		}
		h.setCreated(t, key, userID, 100)
		if !h.dao.UnbindAuthUser(ctx, key, userID) {
			t.Fatal("unbind")
		}
		second, err := h.dao.BindAuthKeyUser(ctx, key, userID)
		if err != nil {
			t.Fatal(err)
		}
		if second == 0 || second == first {
			t.Fatalf("revived hash %d, want a new hash other than %d", second, first)
		}
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 0 || rows[0].userID != userID || rows[0].created != 100 || rows[0].hash != second {
			t.Fatalf("revive: %+v", rows)
		}
	})

	t.Run("temp key login becomes one permanent authorization", func(t *testing.T) {
		perm := h.insertPerm(t)
		temp := h.nextKey()
		h.insertKey(t, temp, 1, perm)
		const userID = int64(141)
		if _, err := h.dao.BindAuthKeyUser(ctx, temp, userID); err != nil {
			t.Fatal(err)
		}
		h.insertDevice(t, temp, userID, "temp-token")
		if err := h.dao.AdoptTempAuthorization(ctx, temp, perm); err != nil {
			t.Fatal(err)
		}
		var live int
		for _, row := range append(h.binds(t, temp), h.binds(t, perm)...) {
			if row.deleted == 0 {
				live++
				if row.userID != userID || row.authKey != perm {
					t.Fatalf("live row %+v", row)
				}
			}
		}
		if live != 1 {
			t.Fatalf("live rows %d", live)
		}
		if n := h.deviceCount(t, userID, temp); n != 0 {
			t.Fatalf("temp push tokens left: %d", n)
		}
	})

	t.Run("old bind that is pinging is not expired", func(t *testing.T) {
		const userID = int64(151)
		if _, err := h.db.Exec(`INSERT INTO users (id, authorization_ttl_days) VALUES (?, 30)`, userID); err != nil {
			t.Fatal(err)
		}
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		boundAt := time.Now().Unix() - 40*24*60*60
		h.setActive(t, key, userID, boundAt)
		h.insertDevice(t, key, userID, "live-token")
		cData, err := h.dao.GetCacheAuthData(ctx, key)
		if err != nil || cData == nil || cData.BindUser == nil || cData.BindUser.ActivityTrusted {
			t.Fatalf("cache before ping: trusted %v err %v", cData != nil && cData.BindUser != nil && cData.BindUser.ActivityTrusted, err)
		}
		now := time.Now().Unix()
		h.dao.ApplyAuthorizationLifetime(ctx, key, cData, now)
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 0 {
			t.Fatalf("pinging session was deleted: %+v", rows)
		}
		if rows[0].active < now-5 || h.trusted(t, key, userID) != 1 {
			t.Fatalf("activity was not recorded: active %d trusted %d", rows[0].active, h.trusted(t, key, userID))
		}
		if n := h.deviceCount(t, userID, key); n != 1 {
			t.Fatalf("push token removed from a live session: %d", n)
		}
		h.dao.ExpireStaleAuthorizations(ctx, userID, 0, now)
		rows = h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 0 {
			t.Fatalf("device list expired a session that just pinged: %+v", rows)
		}
	})

	t.Run("legacy bind time is not inactivity", func(t *testing.T) {
		const userID = int64(152)
		if _, err := h.db.Exec(`INSERT INTO users (id, authorization_ttl_days) VALUES (?, 30)`, userID); err != nil {
			t.Fatal(err)
		}
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		h.setActive(t, key, userID, time.Now().Unix()-40*24*60*60)
		h.insertDevice(t, key, userID, "legacy-token")
		h.dao.ExpireStaleAuthorizations(ctx, userID, 0, time.Now().Unix())
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 0 {
			t.Fatalf("untrusted bind was expired: %+v", rows)
		}
		if n := h.deviceCount(t, userID, key); n != 1 {
			t.Fatalf("legacy push token removed: %d", n)
		}
	})

	t.Run("trusted idle session expires on its own ping", func(t *testing.T) {
		const userID = int64(154)
		if _, err := h.db.Exec(`INSERT INTO users (id, authorization_ttl_days) VALUES (?, 30)`, userID); err != nil {
			t.Fatal(err)
		}
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		idle := time.Now().Unix() - 40*24*60*60
		h.setActive(t, key, userID, idle)
		h.setTrusted(t, key, userID)
		h.insertDevice(t, key, userID, "idle-token")
		if err := h.dao.bumpAuthBindGen(ctx, key); err != nil {
			t.Fatal(err)
		}
		cData, err := h.dao.GetCacheAuthData(ctx, key)
		if err != nil || cData == nil || cData.BindUser == nil || !cData.BindUser.ActivityTrusted {
			t.Fatalf("cache before ping: %+v err %v", cData, err)
		}
		h.dao.ApplyAuthorizationLifetime(ctx, key, cData, time.Now().Unix())
		if cData.BindUser != nil || !cData.Revoked {
			t.Fatal("idle trusted ping was renewed")
		}
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 1 {
			t.Fatalf("row after ping: %+v", rows)
		}
		if n := h.deviceCount(t, userID, key); n != 0 {
			t.Fatalf("push token left on an expired session: %d", n)
		}
	})

	t.Run("ttl read error does not expire", func(t *testing.T) {
		const userID = int64(155)
		if _, err := h.db.Exec(`INSERT INTO users (id, authorization_ttl_days) VALUES (?, 30)`, userID); err != nil {
			t.Fatal(err)
		}
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		h.setActive(t, key, userID, time.Now().Unix()-40*24*60*60)
		h.setTrusted(t, key, userID)
		h.insertDevice(t, key, userID, "ttl-err-token")
		cData, err := h.dao.GetCacheAuthData(ctx, key)
		if err != nil || cData == nil || cData.BindUser == nil {
			t.Fatal(err)
		}
		if _, err := h.db.Exec(`RENAME TABLE users TO users_held`); err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = h.db.Exec(`RENAME TABLE users_held TO users`) }()
		h.dao.ApplyAuthorizationLifetime(ctx, key, cData, time.Now().Unix())
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 0 || cData.Revoked {
			t.Fatalf("ttl read error deleted the session: %+v revoked %v", rows, cData.Revoked)
		}
		if n := h.deviceCount(t, userID, key); n != 1 {
			t.Fatalf("token removed when ttl could not be read: %d", n)
		}
	})

	t.Run("refreshed key is not deleted by a stale expiry", func(t *testing.T) {
		const userID = int64(156)
		if _, err := h.db.Exec(`INSERT INTO users (id, authorization_ttl_days) VALUES (?, 30)`, userID); err != nil {
			t.Fatal(err)
		}
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		idle := time.Now().Unix() - 40*24*60*60
		h.setActive(t, key, userID, time.Now().Unix()-60)
		h.setTrusted(t, key, userID)
		h.insertDevice(t, key, userID, "fresh-token")
		if h.dao.expireAuthorization(ctx, userID, key, idle) {
			t.Fatal("stale cutoff deleted a refreshed key")
		}
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 0 {
			t.Fatalf("row: %+v", rows)
		}
		if n := h.deviceCount(t, userID, key); n != 1 {
			t.Fatalf("token removed: %d", n)
		}
	})

	t.Run("tracked idle authorization expires and drops its push token", func(t *testing.T) {
		const userID = int64(153)
		if _, err := h.db.Exec(`INSERT INTO users (id, authorization_ttl_days) VALUES (?, 30)`, userID); err != nil {
			t.Fatal(err)
		}
		staleKey := h.insertPerm(t)
		freshKey := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, staleKey, userID); err != nil {
			t.Fatal(err)
		}
		if _, err := h.dao.BindAuthKeyUser(ctx, freshKey, userID); err != nil {
			t.Fatal(err)
		}
		h.setActive(t, staleKey, userID, time.Now().Unix()-31*24*60*60)
		h.setActive(t, freshKey, userID, time.Now().Unix()-60)
		h.setTrusted(t, staleKey, userID)
		h.setTrusted(t, freshKey, userID)
		h.insertDevice(t, staleKey, userID, "stale-token")
		h.insertDevice(t, freshKey, userID, "fresh-token")
		if _, err := h.dao.GetCacheAuthData(ctx, staleKey); err != nil {
			t.Fatal(err)
		}
		h.dao.ExpireStaleAuthorizations(ctx, userID, freshKey, time.Now().Unix())
		stale := h.binds(t, staleKey)
		if len(stale) != 1 || stale[0].deleted != 1 || stale[0].created != 0 {
			t.Fatalf("stale row %+v", stale)
		}
		fresh := h.binds(t, freshKey)
		if len(fresh) != 1 || fresh[0].deleted != 0 {
			t.Fatalf("fresh row %+v", fresh)
		}
		got, err := h.dao.GetCacheAuthData(ctx, staleKey)
		if err != nil || got == nil || !got.Revoked || got.UserId() != 0 {
			seen := int64(0)
			revoked := false
			if got != nil {
				seen = got.UserId()
				revoked = got.Revoked
			}
			t.Fatalf("cache after expiry user %d revoked %v err %v", seen, revoked, err)
		}
		if n := h.deviceCount(t, userID, staleKey); n != 0 {
			t.Fatalf("stale push tokens left: %d", n)
		}
		if n := h.deviceCount(t, userID, freshKey); n != 1 {
			t.Fatalf("fresh push tokens %d", n)
		}
	})

	t.Run("reset zero created time becomes the new bind time", func(t *testing.T) {
		key := h.insertPerm(t)
		const userID = int64(111)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		h.setCreated(t, key, userID, 100)
		if ids := h.dao.ResetAuthorization(ctx, userID, 0, 0); len(ids) != 1 || ids[0] != key {
			t.Fatalf("reset keys %v", ids)
		}
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 1 || rows[0].created != 0 {
			t.Fatalf("after reset: %+v", rows)
		}
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		rows = h.binds(t, key)
		if len(rows) != 1 || rows[0].deleted != 0 || rows[0].created == 0 || rows[0].created == 100 {
			t.Fatalf("rebind after reset: %+v", rows)
		}
	})

	t.Run("missing key row does not insert", func(t *testing.T) {
		key := h.nextKey()
		_, err := h.dao.BindAuthKeyUser(ctx, key, 121)
		if !errors.Is(err, ErrAuthKeyNotRegistered) {
			t.Fatalf("err=%v", err)
		}
		if rows := h.binds(t, key); len(rows) != 0 {
			t.Fatalf("insert without a lock row: %+v", rows)
		}
	})

	t.Run("sql error does not move the generation", func(t *testing.T) {
		key := h.insertPerm(t)
		const userID = int64(131)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, userID); err != nil {
			t.Fatal(err)
		}
		genBefore, err := h.dao.authBindGen(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.db.Exec("RENAME TABLE auth_users TO auth_users_held"); err != nil {
			t.Fatal(err)
		}
		_, err = h.dao.BindAuthKeyUser(ctx, key, 132)
		if _, backErr := h.db.Exec("RENAME TABLE auth_users_held TO auth_users"); backErr != nil {
			t.Fatal(backErr)
		}
		if err == nil || errors.Is(err, ErrAuthKeyUserConflict) {
			t.Fatalf("expected a sql error, got %v", err)
		}
		genAfter, err := h.dao.authBindGen(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if genAfter != genBefore {
			t.Fatalf("failed bind moved generation %d -> %d", genBefore, genAfter)
		}
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].userID != userID || rows[0].deleted != 0 {
			t.Fatalf("failed bind wrote %+v", rows)
		}
	})

	t.Run("concurrent different users", func(t *testing.T) {
		for i := 0; i < 16; i++ {
			key := h.insertPerm(t)
			h.oneWinner(t, key, h.nextKey(), h.nextKey())
		}
	})

	t.Run("concurrent same user", func(t *testing.T) {
		key := h.insertPerm(t)
		const userID = int64(141)
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, errs[i] = h.dao.BindAuthKeyUser(ctx, key, userID)
			}(i)
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("same user %d: %v", i, err)
			}
		}
		rows := h.binds(t, key)
		if len(rows) != 1 || rows[0].userID != userID || rows[0].deleted != 0 {
			t.Fatalf("same user concurrent: %+v", rows)
		}
	})

	t.Run("logout does not let a stale reader republish the owner", func(t *testing.T) {
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, 7); err != nil {
			t.Fatal(err)
		}
		release := h.blockReader(t, key)
		if !h.dao.UnbindAuthUser(ctx, key, 7) {
			t.Fatal("unbind")
		}
		release()
		got, err := h.dao.GetCacheAuthData(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got.UserId() == 7 || (got.BindUser != nil && !got.Revoked) {
			t.Fatalf("stale owner backfilled: user %d revoked %v", got.UserId(), got.Revoked)
		}
	})

	t.Run("rebind does not let a stale reader republish the old owner", func(t *testing.T) {
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, 7); err != nil {
			t.Fatal(err)
		}
		release := h.blockReader(t, key)
		if !h.dao.UnbindAuthUser(ctx, key, 7) {
			t.Fatal("unbind")
		}
		if _, err := h.dao.BindAuthKeyUser(ctx, key, 8); err != nil {
			t.Fatal(err)
		}
		release()
		got, err := h.dao.GetCacheAuthData(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got.UserId() != 8 {
			t.Fatalf("live cache user %d, want 8", got.UserId())
		}
	})

	t.Run("another cache sees the new generation", func(t *testing.T) {
		other := h.reopen(t)
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, 7); err != nil {
			t.Fatal(err)
		}
		release := h.blockReader(t, key)
		if !other.UnbindAuthUser(ctx, key, 7) {
			t.Fatal("unbind on the other dao")
		}
		release()
		got, err := other.GetCacheAuthData(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got.UserId() == 7 {
			t.Fatal("other instance served the stale owner")
		}
	})

	t.Run("generation increment failure drops the live slot", func(t *testing.T) {
		key := h.insertPerm(t)
		if _, err := h.dao.BindAuthKeyUser(ctx, key, 7); err != nil {
			t.Fatal(err)
		}
		if _, err := h.dao.GetCacheAuthData(ctx, key); err != nil {
			t.Fatal(err)
		}
		prevKV := h.dao.kv
		t.Cleanup(func() { h.dao.kv = prevKV })
		h.dao.kv = failIncrKV{Store: prevKV}
		if h.dao.UnbindAuthUser(ctx, key, 7) {
			t.Fatal("unbind reported success while the generation increment failed")
		}
		got, err := h.dao.GetCacheAuthData(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got.UserId() == 7 {
			t.Fatal("live slot still has the unbound owner")
		}
	})

	t.Run("temp key normalizes to the permanent key", func(t *testing.T) {
		perm := h.insertPerm(t)
		temp := h.nextKey()
		h.insertKey(t, temp, 1, perm)
		info, err := h.dao.QueryAuthKeyV2(ctx, temp)
		if err != nil {
			t.Fatal(err)
		}
		if info.BoundPermAuthKeyId() != perm {
			t.Fatalf("perm id %d", info.BoundPermAuthKeyId())
		}
		const userID, other = int64(151), int64(152)
		if _, err := h.dao.BindAuthKeyUser(ctx, info.BoundPermAuthKeyId(), userID); err != nil {
			t.Fatal(err)
		}
		if rows := h.binds(t, temp); len(rows) != 0 {
			t.Fatalf("temp key got a binding: %+v", rows)
		}
		rows := h.binds(t, perm)
		if len(rows) != 1 || rows[0].userID != userID || rows[0].deleted != 0 {
			t.Fatalf("perm binding: %+v", rows)
		}
		if _, err := h.dao.BindAuthKeyUser(ctx, info.BoundPermAuthKeyId(), other); !errors.Is(err, ErrAuthKeyUserConflict) {
			t.Fatalf("second owner via temp key: %v", err)
		}
		if got := h.binds(t, perm); len(got) != 1 || got[0].userID != userID || got[0].hash != rows[0].hash {
			t.Fatalf("conflict changed the perm row: %+v", got)
		}
	})
}

func (h *authBindHarness) oneWinner(t *testing.T, key, a, b int64) {
	t.Helper()
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	type result struct {
		err error
	}
	out := make([]result, 2)
	users := []int64{a, b}
	for i := range users {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, out[i].err = h.dao.BindAuthKeyUser(ctx, key, users[i])
		}(i)
	}
	close(start)
	wg.Wait()
	winner := int64(0)
	wins := 0
	for i, r := range out {
		if r.err == nil {
			wins++
			winner = users[i]
			continue
		}
		if !errors.Is(r.err, ErrAuthKeyUserConflict) {
			t.Fatalf("key %d user %d: %v", key, users[i], r.err)
		}
	}
	if wins != 1 {
		t.Fatalf("key %d wins %d", key, wins)
	}
	var activeUser int64
	active := 0
	for _, row := range h.binds(t, key) {
		if row.deleted != 0 {
			continue
		}
		active++
		activeUser = row.userID
	}
	if active != 1 || activeUser != winner {
		t.Fatalf("key %d winner %d rows %+v", key, winner, h.binds(t, key))
	}
}

func (h *authBindHarness) blockReader(t *testing.T, authKeyID int64) func() {
	t.Helper()
	entered := make(chan struct{})
	hold := make(chan struct{})
	var enterOnce sync.Once
	var releaseOnce sync.Once
	authLoadGate.Store(func() {
		enterOnce.Do(func() { close(entered) })
		<-hold
	})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = h.dao.GetCacheAuthData(context.Background(), authKeyID)
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("reader did not reach the cache backfill")
	}
	release := func() {
		releaseOnce.Do(func() {
			authLoadGate.Store((func())(nil))
			close(hold)
			select {
			case <-finished:
			case <-time.After(10 * time.Second):
				t.Fatal("stale reader did not finish")
			}
		})
	}
	t.Cleanup(release)
	return release
}

type failIncrKV struct {
	kv.Store
}

func (f failIncrKV) Incr(key string) (int64, error) {
	return f.IncrCtx(context.Background(), key)
}

func (f failIncrKV) IncrCtx(context.Context, string) (int64, error) {
	return 0, errors.New("auth gen incr failed")
}

type storedBind struct {
	userID  int64
	hash    int64
	created int64
	active  int64
	deleted int
	authKey int64
}

func sameBinds(a, b []storedBind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type authBindHarness struct {
	dao *Dao
	db  *sql.DB
	dsn string
	mr  *miniredis.Miniredis
	seq atomic.Int64
}

func (h *authBindHarness) nextKey() int64 {
	return 10_000_000 + h.seq.Add(1)
}

func (h *authBindHarness) insertPerm(t *testing.T) int64 {
	t.Helper()
	id := h.nextKey()
	h.insertKey(t, id, 0, id)
	return id
}

func (h *authBindHarness) insertKey(t *testing.T, id int64, keyType int, permID int64) {
	t.Helper()
	if _, err := h.db.Exec(`INSERT INTO auth_keys (auth_key_id, body) VALUES (?, ?)`, id, "AQID"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`INSERT INTO auth_key_infos (auth_key_id, auth_key_type, perm_auth_key_id) VALUES (?, ?, ?)`, id, keyType, permID); err != nil {
		t.Fatal(err)
	}
}

func (h *authBindHarness) setCreated(t *testing.T, authKeyID, userID, created int64) {
	t.Helper()
	res, err := h.db.Exec(`UPDATE auth_users SET date_created = ? WHERE auth_key_id = ? AND user_id = ?`, created, authKeyID, userID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("set created rows %d", n)
	}
}

func (h *authBindHarness) setTrusted(t *testing.T, authKeyID, userID int64) {
	t.Helper()
	res, err := h.db.Exec(`UPDATE auth_users SET activity_trusted = 1 WHERE auth_key_id = ? AND user_id = ?`, authKeyID, userID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		t.Fatalf("set trusted rows %d err %v", n, err)
	}
}

func (h *authBindHarness) trusted(t *testing.T, authKeyID, userID int64) int {
	t.Helper()
	var n int
	if err := h.db.QueryRow(`SELECT activity_trusted FROM auth_users WHERE auth_key_id = ? AND user_id = ?`, authKeyID, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (h *authBindHarness) setActive(t *testing.T, authKeyID, userID, active int64) {
	t.Helper()
	res, err := h.db.Exec(`UPDATE auth_users SET date_active = ? WHERE auth_key_id = ? AND user_id = ?`, active, authKeyID, userID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		t.Fatalf("set active rows %d err %v", n, err)
	}
}

func (h *authBindHarness) insertDevice(t *testing.T, authKeyID, userID int64, token string) {
	t.Helper()
	if _, err := h.db.Exec(`INSERT INTO devices (auth_key_id, user_id, token_type, token) VALUES (?, ?, 1, ?)`, authKeyID, userID, token); err != nil {
		t.Fatal(err)
	}
}

func (h *authBindHarness) deviceCount(t *testing.T, userID, authKeyID int64) int {
	t.Helper()
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE user_id = ? AND auth_key_id = ?`, userID, authKeyID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (h *authBindHarness) binds(t *testing.T, authKeyID int64) []storedBind {
	t.Helper()
	rows, err := h.db.Query(`SELECT user_id, hash, date_created, date_active, deleted, auth_key_id FROM auth_users WHERE auth_key_id = ? ORDER BY user_id`, authKeyID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []storedBind
	for rows.Next() {
		var row storedBind
		if err := rows.Scan(&row.userID, &row.hash, &row.created, &row.active, &row.deleted, &row.authKey); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (h *authBindHarness) reopen(t *testing.T) *Dao {
	t.Helper()
	return newAuthBindDao(t, h.dsn, h.mr)
}

func openAuthBindHarness(t *testing.T) *authBindHarness {
	t.Helper()
	adminDSN, err := authBindAdminDSN(t)
	if err != nil {
		if os.Getenv("GITHUB_ACTIONS") == "1" {
			t.Fatalf("mysql is required in CI: %v", err)
		}
		t.Skipf("mysql not available: %v", err)
	}
	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("auth_bind_%d", os.Getpid())
	if _, err := admin.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + schema) })
	dsn := dsnWithSchema(adminDSN, schema)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range strings.Split(authBindSchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	mr := miniredis.RunT(t)
	return &authBindHarness{
		dao: newAuthBindDao(t, dsn, mr),
		db:  db,
		dsn: dsn,
		mr:  mr,
	}
}

func newAuthBindDao(t *testing.T, dsn string, mr *miniredis.Miniredis) *Dao {
	t.Helper()
	cfg := config.Config{}
	cfg.Mysql.DSN = dsn
	cfg.Cache = cacheNodes(mr.Addr())
	cfg.KV = cacheNodes(mr.Addr())
	return New(cfg)
}

func cacheNodes(addr string) cache.ClusterConf {
	return cache.ClusterConf{{
		RedisConf: redis.RedisConf{Host: addr, Type: redis.NodeType},
		Weight:    100,
	}}
}

const authBindSchema = `
CREATE TABLE auth_keys (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  body VARCHAR(512) NOT NULL,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id)
) ENGINE=InnoDB;

CREATE TABLE auth_key_infos (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  auth_key_type INT NOT NULL,
  perm_auth_key_id BIGINT NOT NULL DEFAULT 0,
  temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
  media_temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id)
) ENGINE=InnoDB;

CREATE TABLE auth_users (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL DEFAULT 0,
  hash BIGINT NOT NULL DEFAULT 0,
  date_created BIGINT NOT NULL DEFAULT 0,
  date_active BIGINT NOT NULL DEFAULT 0,
  activity_trusted TINYINT(1) NOT NULL DEFAULT 0,
  android_push_session_id BIGINT NOT NULL DEFAULT 0,
  encrypted_requests_disabled TINYINT(1) NOT NULL DEFAULT 0,
  call_requests_disabled TINYINT(1) NOT NULL DEFAULT 0,
  unconfirmed TINYINT(1) NOT NULL DEFAULT 0,
  deleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id, user_id)
) ENGINE=InnoDB;

CREATE TABLE auths (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  layer INT NOT NULL DEFAULT 0,
  api_id INT NOT NULL DEFAULT 0,
  device_model VARCHAR(128) NOT NULL DEFAULT '',
  system_version VARCHAR(64) NOT NULL DEFAULT '',
  app_version VARCHAR(64) NOT NULL DEFAULT '',
  system_lang_code VARCHAR(64) NOT NULL DEFAULT '',
  lang_pack VARCHAR(64) NOT NULL DEFAULT '',
  lang_code VARCHAR(64) NOT NULL DEFAULT '',
  client_ip VARCHAR(64) NOT NULL DEFAULT '',
  proxy VARCHAR(64) NOT NULL DEFAULT '',
  params VARCHAR(512) NOT NULL DEFAULT '',
  date_active BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id)
) ENGINE=InnoDB;

CREATE TABLE users (
  id BIGINT NOT NULL,
  authorization_ttl_days INT NOT NULL DEFAULT 180,
  PRIMARY KEY (id)
) ENGINE=InnoDB;

CREATE TABLE devices (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  token_type INT NOT NULL DEFAULT 0,
  token VARCHAR(255) NOT NULL DEFAULT '',
  state INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY user_key (user_id, auth_key_id)
) ENGINE=InnoDB
`

func authBindAdminDSN(t *testing.T) (string, error) {
	if dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); dsn != "" {
		if pingMySQL(dsn) {
			return dsn, nil
		}
	}
	candidates := []string{
		"teamgram:teamgram@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:root@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
	}
	for _, dsn := range candidates {
		if pingMySQL(dsn) {
			return dsn, nil
		}
	}
	if port, err := startAuthBindMySQL(t); err == nil {
		dsn := fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/mysql?charset=utf8mb4&parseTime=true", port)
		if pingMySQL(dsn) {
			return dsn, nil
		}
	}
	return "", errors.New("no mysql")
}

func pingMySQL(dsn string) bool {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return false
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return db.PingContext(ctx) == nil
}

func dsnWithSchema(dsn, schema string) string {
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		return dsn + "/" + schema
	}
	rest := dsn[slash+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return dsn[:slash+1] + schema + rest[q:]
	}
	return dsn[:slash+1] + schema
}

func startAuthBindMySQL(t *testing.T) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", err
	}
	name := fmt.Sprintf("teamgram-auth-bind-%d", os.Getpid())
	_ = exec.Command("docker", "rm", "-f", name).Run()
	if t != nil {
		t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	}
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		"-e", "MYSQL_ROOT_PASSWORD=root",
		"-e", "MYSQL_ROOT_HOST=%",
		"-p", "127.0.0.1::3306",
		"mysql:8.0.46").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %w %s", err, out)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		probe, probeErr := exec.Command("docker", "exec", name, "mysql", "-uroot", "-proot", "-N", "-e", "SELECT 1").CombinedOutput()
		if probeErr == nil && strings.Contains(string(probe), "1") {
			portOut, portErr := exec.Command("docker", "port", name, "3306").CombinedOutput()
			if portErr != nil {
				return "", portErr
			}
			fields := strings.Fields(string(portOut))
			if len(fields) == 0 {
				return "", fmt.Errorf("docker port %q", portOut)
			}
			hostport := fields[len(fields)-1]
			port := hostport[strings.LastIndex(hostport, ":")+1:]
			return port, nil
		}
		time.Sleep(time.Second)
	}
	return "", errors.New("mysql container did not become ready")
}
