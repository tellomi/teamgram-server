package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"
)

// androidConstructorAlias maps DrKLO/Telegram class names onto the layer-232
// method that uses the same constructor. The Java types live in TL_account,
// TL_wallet, or TL_stars even when the schema namespace differs.
// Checked against Telegram Android 3de37c8cfb039452 (TL_stars.java request
// classes) and proto/schema/telegram_api_layer232.tl. Session dispatch is by
// constructor id, so these calls already decode to the schema type.
//
//	account.getBirthdays                 0xdaeda864 contacts.getBirthdays
//	account.getRequirementsToContact     0xd89a83a3 users.getRequirementsToContact
//	account.getWebPagePreview            0x570d6f6f messages.getWebPagePreview
//	account.initPasskeyLogin             0x518ad0b7 auth.initPasskeyLogin
//	account.finishPasskeyLogin           0x9857ad07 auth.finishPasskeyLogin
//	wallet.getCurrencyRates              0xd35983e8 payments.getCurrencyRates
//	stars.checkCanSendGift               0xc0c4edc9 payments.checkCanSendGift
//	stars.convertStarGift                0x74bf076b payments.convertStarGift
//	stars.createStarGiftCollection       0x1f4a0e87 payments.createStarGiftCollection
//	stars.deleteStarGift                 0x3cdaf8aa payments.deleteStarGift
//	stars.deleteStarGiftCollection       0xad5648e8 payments.deleteStarGiftCollection
//	stars.getCraftStarGifts              0xfd05dd00 payments.getCraftStarGifts
//	stars.getResaleStarGifts             0x7a5fa236 payments.getResaleStarGifts
//	stars.getSavedStarGift               0xb455a106 payments.getSavedStarGift
//	stars.getSavedStarGifts              0xa319e569 payments.getSavedStarGifts
//	stars.getStarGiftCollections         0x981b91dd payments.getStarGiftCollections
//	stars.getStarGiftUpgradeAttributes   0x6d038b58 payments.getStarGiftUpgradeAttributes
//	stars.getStarGiftUpgradePreview      0x9c9abcb1 payments.getStarGiftUpgradePreview
//	stars.getStarGiftWithdrawalUrl       0xc8ca817b payments.getStarGiftWithdrawalUrl
//	stars.getStarGifts                   0xc4563590 payments.getStarGifts
//	stars.getUniqueStarGift              0xa1974d72 payments.getUniqueStarGift
//	stars.getUniqueStarGiftValueInfo     0x4365af6b payments.getUniqueStarGiftValueInfo
//	stars.reorderStarGiftCollections     0xc32af4cc payments.reorderStarGiftCollections
//	stars.saveStarGift                   0x2a2a697c payments.saveStarGift
//	stars.toggleChatStarGiftNotifications 0x60eaefa1 payments.toggleChatStarGiftNotifications
//	stars.toggleStarGiftsPinnedToTop     0x1513e7b0 payments.toggleStarGiftsPinnedToTop
//	stars.updatePaidMessagesPrice        0x4b12327b channels.updatePaidMessagesPrice
//	stars.updateStarGiftCollection       0x4fddbee7 payments.updateStarGiftCollection
//	stars.updateStarGiftPrice            0xedbe6ccb payments.updateStarGiftPrice
//
// core.telegram.org still publishes payments.getStarGiftWithdrawalUrl#d06e93a8
// (password:InputCheckPasswordSRP). Layer 232 and this Android checkout send
// #c8ca817b (flags:# and tmp_password:bytes). Both ids decode to
// TLPaymentsGetStarGiftWithdrawalUrl. Android names flags.0 on
// updatePaidMessagesPrice suggestions_allowed; the schema and
// core.telegram.org call that bit broadcast_messages_allowed. The bytes match.
var androidConstructorAlias = map[string]string{
	"account.getBirthdays":                  "contacts.getBirthdays",
	"account.getRequirementsToContact":      "users.getRequirementsToContact",
	"account.getWebPagePreview":             "messages.getWebPagePreview",
	"account.initPasskeyLogin":              "auth.initPasskeyLogin",
	"account.finishPasskeyLogin":            "auth.finishPasskeyLogin",
	"wallet.getCurrencyRates":               "payments.getCurrencyRates",
	"stars.checkCanSendGift":                "payments.checkCanSendGift",
	"stars.convertStarGift":                 "payments.convertStarGift",
	"stars.createStarGiftCollection":        "payments.createStarGiftCollection",
	"stars.deleteStarGift":                  "payments.deleteStarGift",
	"stars.deleteStarGiftCollection":        "payments.deleteStarGiftCollection",
	"stars.getCraftStarGifts":               "payments.getCraftStarGifts",
	"stars.getResaleStarGifts":              "payments.getResaleStarGifts",
	"stars.getSavedStarGift":                "payments.getSavedStarGift",
	"stars.getSavedStarGifts":               "payments.getSavedStarGifts",
	"stars.getStarGiftCollections":          "payments.getStarGiftCollections",
	"stars.getStarGiftUpgradeAttributes":    "payments.getStarGiftUpgradeAttributes",
	"stars.getStarGiftUpgradePreview":       "payments.getStarGiftUpgradePreview",
	"stars.getStarGiftWithdrawalUrl":        "payments.getStarGiftWithdrawalUrl",
	"stars.getStarGifts":                    "payments.getStarGifts",
	"stars.getUniqueStarGift":               "payments.getUniqueStarGift",
	"stars.getUniqueStarGiftValueInfo":      "payments.getUniqueStarGiftValueInfo",
	"stars.reorderStarGiftCollections":      "payments.reorderStarGiftCollections",
	"stars.saveStarGift":                    "payments.saveStarGift",
	"stars.toggleChatStarGiftNotifications": "payments.toggleChatStarGiftNotifications",
	"stars.toggleStarGiftsPinnedToTop":      "payments.toggleStarGiftsPinnedToTop",
	"stars.updatePaidMessagesPrice":         "channels.updatePaidMessagesPrice",
	"stars.updateStarGiftCollection":        "payments.updateStarGiftCollection",
	"stars.updateStarGiftPrice":             "payments.updateStarGiftPrice",
}

// skippedClientMethods are call sites that are commented out in the client
// and have no constructor in layer 232. Android SharedMediaLayout.sendTabsOrder
// leaves TL_account_reorderProfileTabs and TL_channels_reorderProfileTabs
// commented. Layer 232 has account.setMainProfileTab and channels.setMainProfileTab.
var skippedClientMethods = map[string]bool{
	"account.reorderProfileTabs":  true,
	"channels.reorderProfileTabs": true,
}

func canonicalClientMethod(name string) (string, bool) {
	if skippedClientMethods[name] {
		return "", false
	}
	if alt, ok := androidConstructorAlias[name]; ok {
		return alt, true
	}
	return name, true
}

func buildRows(snap Snapshot, schema []SchemaMethod, srv *serverModel) []Row {
	known := map[string]SchemaMethod{}
	for _, m := range schema {
		known[m.Method] = m
	}
	sites := map[string][]Site{}
	for _, m := range snap.Methods {
		name, ok := canonicalClientMethod(m.Method)
		if !ok {
			continue
		}
		sites[name] = append(sites[name], m.Sites...)
	}
	names := map[string]bool{}
	for name, ss := range sites {
		if len(ss) > 0 {
			names[name] = true
		}
	}
	var list []string
	for n := range names {
		list = append(list, n)
	}
	sort.Strings(list)

	var rows []Row
	for _, name := range list {
		ss := sites[name]
		row := Row{
			Method:  name,
			Module:  moduleOf(name),
			Clients: map[string][]Site{},
		}
		var startup, background bool
		for _, s := range ss {
			row.Clients[s.Client] = append(row.Clients[s.Client], s)
			st, bg := whenOf(s.File, s.Func, name)
			startup = startup || st
			background = background || bg
			switch s.Client {
			case "ios":
				row.IOS = true
			case "android":
				row.Android = true
			case "desktop":
				row.Desktop = true
			case "macos":
				row.MacOS = true
			}
		}
		row.Startup = startup
		row.Background = background
		row.When = whenLabel(startup, background)
		row.Phase = phaseOf(row.Module, startup, background)
		st, effect, service, handler, notes, routed := srv.resolve(name)
		row.Status = st
		row.Effect = effect
		row.Service = service
		row.Handler = handler
		row.Route = routed
		row.Notes = notes
		if _, ok := known[name]; !ok {
			row.Notes = strings.TrimSpace(row.Notes + " not in layer schema")
		}
		rows = append(rows, row)
	}
	return rows
}

func yn(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func writeCSV(rows []Row) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{
		"method", "module", "phase", "when", "ios", "android", "desktop", "macos",
		"status", "effect", "routed", "handler", "rpc_service", "notes",
	})
	for _, r := range rows {
		_ = w.Write([]string{
			r.Method, r.Module, r.Phase, r.When,
			yn(r.IOS), yn(r.Android), yn(r.Desktop), yn(r.MacOS),
			r.Status, r.Effect, yn(r.Route), r.Handler, r.Service, r.Notes,
		})
	}
	w.Flush()
	return buf.Bytes()
}

type counts struct {
	n, impl, stub, unr, miss, p0, p0err int
}

func tally(rows []Row) (byMod map[string]*counts, order []string, clients map[string]*counts) {
	byMod = map[string]*counts{}
	clients = map[string]*counts{
		"ios": {}, "android": {}, "desktop": {}, "macos": {},
	}
	for _, r := range rows {
		c := byMod[r.Module]
		if c == nil {
			c = &counts{}
			byMod[r.Module] = c
		}
		addCount(c, r)
		for _, name := range []string{"ios", "android", "desktop", "macos"} {
			on := false
			switch name {
			case "ios":
				on = r.IOS
			case "android":
				on = r.Android
			case "desktop":
				on = r.Desktop
			case "macos":
				on = r.MacOS
			}
			if on {
				addCount(clients[name], r)
			}
		}
	}
	seen := map[string]bool{}
	for _, m := range preferredModuleOrder {
		if byMod[m] != nil {
			order = append(order, m)
			seen[m] = true
		}
	}
	var rest []string
	for m := range byMod {
		if !seen[m] {
			rest = append(rest, m)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)
	return byMod, order, clients
}

func addCount(c *counts, r Row) {
	c.n++
	switch r.Status {
	case statusImplemented:
		c.impl++
	case statusStub:
		c.stub++
	case statusRoutedUnregistered:
		c.unr++
	default:
		c.miss++
	}
	if r.Phase == "P0" {
		c.p0++
		if r.Effect == effectError {
			c.p0err++
		}
	}
}

func writeMarkdown(snap Snapshot, schemaN, layer int, rows []Row) []byte {
	byMod, order, clients := tally(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "# API coverage matrix\n\n")
	fmt.Fprintf(&b, "Methods **called by at least one official client**, classified against Tellomi `main`.\n\n")
	fmt.Fprintf(&b, "Layer **%d** schema (`proto/schema/telegram_api_layer232.tl`, %d functions). ", layer, schemaN)
	fmt.Fprintf(&b, "%d of those functions are called by a client. Methods below include client calls that are not in the schema.\n\n", countInSchema(rows, schemaN))
	fmt.Fprintf(&b, "Regenerate with `go run ./tools/apicoverage`. ")
	fmt.Fprintf(&b, "Refresh client call sites with `go run ./tools/apicoverage -extract` (see the source notes). ")
	fmt.Fprintf(&b, "CI runs `go run ./tools/apicoverage -check` and fails when this file or `docs/api-coverage.csv` drifts from the server tree.\n\n")
	fmt.Fprintf(&b, "This file is generated. Edit the generator, not the tables.\n\n")

	b.WriteString("## How a cell is decided\n\n")
	b.WriteString("**Server status** is what a connected client gets from the `bff` process session actually routes to (`teamgramd/etc/session.yaml` `BFFProxyClients.IDMap`, same map in `teamgramd/etc2/session.yaml`).\n\n")
	b.WriteString("| Status | Meaning |\n| --- | --- |\n")
	b.WriteString("| `implemented` | IDMap routes the RPC service, `app/bff/bff/internal/server/server.go` registers it, and the handler reads or writes real state (or returns a filled static payload). |\n")
	b.WriteString("| `stub` | The client gets a successful empty object, `Bool`, or `*NotModified`, either from a placeholder handler or from `TryReturnFakeRpcResult` when the service is not routed. |\n")
	b.WriteString("| `routed-but-unregistered` | IDMap routes the service, but the bff process does not register a handler for that method, so the call fails. |\n")
	b.WriteString("| `missing` | No IDMap route and no empty proxy response. The session returns `ENTERPRISE_IS_BLOCKED`. A handler file may still exist; the notes say when it does. |\n\n")
	b.WriteString("`effect=error` is a response that fails the RPC. `empty`, `bool`, and `not_modified` succeed with no data. `ok` is a real result.\n\n")
	b.WriteString("**When** is `startup`, `background`, `startup+background`, or `interactive`. Startup and background sites are the ones that run from session bootstrap (config, updates difference, presence, langpack, login, push registration) rather than from a screen the user opened. Any such site makes the method **P0**.\n\n")
	b.WriteString("**Phases.** P0: startup or background, and the server must not fail the RPC. P1: core chat (auth, account, users, contacts, messages, updates, photos, upload, channels) that is not already P0. P2: everything else.\n\n")
	b.WriteString("Android keeps some requests in `TL_account` even when the constructor belongs to another namespace. Those call sites are counted on the layer-232 method (`contacts.getBirthdays`, `users.getRequirementsToContact`, `messages.getWebPagePreview`, `auth.initPasskeyLogin`, `auth.finishPasskeyLogin`). `TL_wallet.getCurrencyRates` is constructor `0xd35983e8`, counted as `payments.getCurrencyRates`. `account.reorderProfileTabs` and `channels.reorderProfileTabs` are commented out in Android and are not in layer 232; `account.setMainProfileTab` and `channels.setMainProfileTab` are the schema methods.\n\n")
	b.WriteString("Android `TL_stars` (DrKLO `3de37c8`) uses the same layer-232 constructor ids as `payments.*StarGift*`, `payments.checkCanSendGift`, and `channels.updatePaidMessagesPrice` (`stars.updatePaidMessagesPrice` is `0x4b12327b`). Those call sites are counted on the layer-232 method. The server does not register a `stars` RPC namespace: the session decodes the constructor id into the payments or channels type that is already routed. `payments.getStarGiftWithdrawalUrl` is `#c8ca817b` in layer 232 (`tmp_password:bytes`); `core.telegram.org` still shows the layer-231 id `#d06e93a8`, and both ids decode to the same handler.\n\n")

	b.WriteString("## Client sources\n\n")
	b.WriteString("| Client | Repository | Commit | What was counted |\n| --- | --- | --- | --- |\n")
	for _, s := range snap.Sources {
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s |\n", s.Client, s.Repo, s.Commit, s.Note)
	}
	b.WriteString("\n")
	b.WriteString("iOS is `Api.functions` call sites in TelegramCore, excluding `Sources/MacOS`.\n\n")
	b.WriteString("Android is `TL_*` request constructions in Java and in `jni/tgnet` (including native `TL_help_getConfig`), excluding generated `TLRPC.java`, `tl/TL_*.java`, and `ApiScheme`.\n\n")
	b.WriteString("Desktop is `MTPnamespace_Method` requests under `Telegram/SourceFiles`: direct calls, `::` flag builders, and `PrepareMessage<MTPmessages_SendMessage>`. Generated `mtproto/scheme` and tests are excluded.\n\n")
	b.WriteString("`overtake/TelegramSwift` is not cloneable (the remote returns \"repository not found\"). The current macOS source is `TelegramMessenger/telegram-macos`, which pins the same Telegram-iOS commit and does not call `Api.functions` itself. The macOS column is TelegramCore methods reached from that app: every RPC in the account/session files it constructs (`State`, `Account`, `Network`, `SyncCore`, `PendingMessages`, auth, langpack, `Sources/MacOS`) plus TelegramCore functions whose names appear in `Telegram-Mac` and `packages`.\n\n")

	b.WriteString("## Counts by module\n\n")
	b.WriteString("| Module | Called | Implemented | Stub | Routed-unregistered | Missing | P0 | P0 errors |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	tot := counts{}
	for _, m := range order {
		c := byMod[m]
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d | %d |\n", m, c.n, c.impl, c.stub, c.unr, c.miss, c.p0, c.p0err)
		tot.n += c.n
		tot.impl += c.impl
		tot.stub += c.stub
		tot.unr += c.unr
		tot.miss += c.miss
		tot.p0 += c.p0
		tot.p0err += c.p0err
	}
	fmt.Fprintf(&b, "| **total** | **%d** | **%d** | **%d** | **%d** | **%d** | **%d** | **%d** |\n\n", tot.n, tot.impl, tot.stub, tot.unr, tot.miss, tot.p0, tot.p0err)

	b.WriteString("## Counts by client\n\n")
	b.WriteString("A method counts for a client when that client has at least one call site. Implemented / stub / missing describe the **server** for those methods, not the client.\n\n")
	b.WriteString("| Client | Called | Implemented | Stub | Routed-unregistered | Missing | P0 | P0 errors |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, name := range []string{"ios", "android", "desktop", "macos"} {
		c := clients[name]
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d | %d |\n", name, c.n, c.impl, c.stub, c.unr, c.miss, c.p0, c.p0err)
	}
	all4 := 0
	for _, r := range rows {
		if r.IOS && r.Android && r.Desktop && r.MacOS {
			all4++
		}
	}
	fmt.Fprintf(&b, "\n%d methods are called by all four clients.\n\n", all4)

	b.WriteString("## Phase proposal\n\n")
	writePhase(&b, rows, "P0", "Startup and background calls. These run before the chat list is usable. `effect=error` drops the session or the login; empty stubs return success with no data and still break clients that require a config, difference, or langpack.")
	writePhase(&b, rows, "P1", "Core chat that is not already covered by P0: sign-in extras, profiles, contacts, history, send, media, groups, and channels.")
	writePhase(&b, rows, "P2", "Everything else the official clients call: stories, payments, bots, folders, business, stats, and similar.")

	b.WriteString("## Methods\n\n")
	b.WriteString("Columns: which clients call the method, when, server status, and the effect a client observes.\n\n")
	for _, m := range order {
		var part []Row
		for _, r := range rows {
			if r.Module == m {
				part = append(part, r)
			}
		}
		sort.Slice(part, func(i, j int) bool {
			if part[i].Phase != part[j].Phase {
				return part[i].Phase < part[j].Phase
			}
			if part[i].Status != part[j].Status {
				return statusRank(part[i].Status) < statusRank(part[j].Status)
			}
			return part[i].Method < part[j].Method
		})
		c := byMod[m]
		fmt.Fprintf(&b, "### %s\n\n", m)
		fmt.Fprintf(&b, "%d called, %d implemented, %d stub, %d routed-unregistered, %d missing.\n\n", c.n, c.impl, c.stub, c.unr, c.miss)
		b.WriteString("| Method | iOS | Android | Desktop | macOS | When | Status | Effect |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, r := range part {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
				r.Method, yn(r.IOS), yn(r.Android), yn(r.Desktop), yn(r.MacOS), r.When, r.Status, r.Effect)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Notes for non-implemented methods\n\n")
	b.WriteString("| Method | Status | Notes |\n| --- | --- | --- |\n")
	for _, r := range rows {
		if r.Status == statusImplemented {
			continue
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", r.Method, r.Status, escapeCell(r.Notes))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

func countInSchema(rows []Row, schemaN int) int {
	_ = schemaN
	n := 0
	for _, r := range rows {
		if !strings.Contains(r.Notes, "not in layer schema") {
			n++
		}
	}
	return n
}

func statusRank(s string) int {
	switch s {
	case statusMissing:
		return 0
	case statusRoutedUnregistered:
		return 1
	case statusStub:
		return 2
	default:
		return 3
	}
}

func writePhase(b *strings.Builder, rows []Row, phase, intro string) {
	var gaps []Row
	for _, r := range rows {
		if r.Phase == phase && r.Status != statusImplemented {
			gaps = append(gaps, r)
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		ie, je := gaps[i].Effect == effectError, gaps[j].Effect == effectError
		if ie != je {
			return ie
		}
		if gaps[i].Module != gaps[j].Module {
			return gaps[i].Module < gaps[j].Module
		}
		return gaps[i].Method < gaps[j].Method
	})
	fmt.Fprintf(b, "### %s\n\n%s\n\n", phase, intro)
	if len(gaps) == 0 {
		b.WriteString("No gaps.\n\n")
		return
	}
	errN, emptyN := 0, 0
	for _, r := range gaps {
		if r.Effect == effectError {
			errN++
		} else {
			emptyN++
		}
	}
	fmt.Fprintf(b, "%d gaps (%d error, %d empty or bool or not-modified).\n\n", len(gaps), errN, emptyN)
	if phase == "P0" {
		b.WriteString("| Method | Module | Effect | Status | Clients |\n| --- | --- | --- | --- | --- |\n")
		for _, r := range gaps {
			fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s |\n", r.Method, r.Module, r.Effect, r.Status, clientList(r))
		}
		b.WriteString("\n")
		return
	}
	by := map[string]int{}
	var mods []string
	for _, r := range gaps {
		if by[r.Module] == 0 {
			mods = append(mods, r.Module)
		}
		by[r.Module]++
	}
	sort.Strings(mods)
	b.WriteString("| Module | Gaps |\n| --- | ---: |\n")
	for _, m := range mods {
		fmt.Fprintf(b, "| %s | %d |\n", m, by[m])
	}
	b.WriteString("\n")
}

func clientList(r Row) string {
	var xs []string
	if r.IOS {
		xs = append(xs, "ios")
	}
	if r.Android {
		xs = append(xs, "android")
	}
	if r.Desktop {
		xs = append(xs, "desktop")
	}
	if r.MacOS {
		xs = append(xs, "macos")
	}
	return strings.Join(xs, ", ")
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "/")
	if len(s) > 240 {
		s = s[:237] + "..."
	}
	return s
}
