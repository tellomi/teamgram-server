package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	extract := flag.Bool("extract", false, "rescan client checkouts and rewrite tools/apicoverage/data/clients.json")
	check := flag.Bool("check", false, "exit 1 when docs/api-coverage.md or .csv differ")
	rootFlag := flag.String("root", "", "teamgram-server checkout (default: module root)")
	ios := flag.String("ios", os.Getenv("APICOVERAGE_IOS"), "TelegramCore Sources directory")
	android := flag.String("android", os.Getenv("APICOVERAGE_ANDROID"), "Telegram Android checkout")
	desktop := flag.String("desktop", os.Getenv("APICOVERAGE_DESKTOP"), "tdesktop Telegram/SourceFiles directory")
	macos := flag.String("macos", os.Getenv("APICOVERAGE_MACOS"), "telegram-macos checkout")
	flag.Parse()

	root, err := findRoot(*rootFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	schemaPath := filepath.Join(root, "proto/schema/telegram_api_layer232.tl")
	layer, schema, err := parseSchema(schemaPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	schemaSet := map[string]bool{}
	for _, m := range schema {
		schemaSet[m.Method] = true
	}

	snapPath := filepath.Join(root, "tools/apicoverage/data/clients.json")
	var snap Snapshot
	if *extract {
		snap, err = extractClients(layer, schemaSet, namespaces(schema), *ios, *android, *desktop, *macos)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		raw, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		raw = append(raw, '\n')
		if err := os.MkdirAll(filepath.Dir(snapPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(snapPath, raw, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d methods)\n", snapPath, len(snap.Methods))
	} else {
		b, err := os.ReadFile(snapPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\nRun with -extract when client checkouts are available.\n", snapPath, err)
			os.Exit(1)
		}
		if err := json.Unmarshal(b, &snap); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	srv, err := loadServer(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rows := buildRows(snap, schema, srv)
	md := writeMarkdown(snap, len(schema), layer, rows)
	cs := writeCSV(rows)
	mdPath := filepath.Join(root, "docs/api-coverage.md")
	csvPath := filepath.Join(root, "docs/api-coverage.csv")
	if *check {
		oldMD, _ := os.ReadFile(mdPath)
		oldCSV, _ := os.ReadFile(csvPath)
		bad := false
		if !bytes.Equal(oldMD, md) {
			fmt.Fprintf(os.Stderr, "%s is stale\n", mdPath)
			bad = true
		}
		if !bytes.Equal(oldCSV, cs) {
			fmt.Fprintf(os.Stderr, "%s is stale\n", csvPath)
			bad = true
		}
		if bad {
			os.Exit(1)
		}
		fmt.Println("api coverage docs match")
		return
	}
	if err := os.WriteFile(mdPath, md, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(csvPath, cs, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	byMod, _, clients := tally(rows)
	_ = byMod
	fmt.Printf("methods %d\n", len(rows))
	for _, name := range []string{"ios", "android", "desktop", "macos"} {
		c := clients[name]
		fmt.Printf("%s called=%d implemented=%d stub=%d unregistered=%d missing=%d p0=%d p0_errors=%d\n",
			name, c.n, c.impl, c.stub, c.unr, c.miss, c.p0, c.p0err)
	}
}

func findRoot(flagRoot string) (string, error) {
	if flagRoot != "" {
		return flagRoot, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", dir)
		}
		dir = parent
	}
}

func extractClients(layer int, schema map[string]bool, nss []string, ios, android, desktop, macos string) (Snapshot, error) {
	if ios == "" || android == "" || desktop == "" || macos == "" {
		return Snapshot{}, fmt.Errorf("extract needs -ios, -android, -desktop, and -macos (TelegramCore Sources, Android repo, tdesktop SourceFiles, telegram-macos repo)")
	}
	for _, p := range []string{ios, android, desktop, macos} {
		if _, err := os.Stat(p); err != nil {
			return Snapshot{}, fmt.Errorf("client path %s: %w", p, err)
		}
	}
	cs := newCallSet(schema, nss)
	if err := cs.scanIOS(ios); err != nil {
		return Snapshot{}, err
	}
	if err := cs.scanAndroid(android); err != nil {
		return Snapshot{}, err
	}
	if err := cs.scanDesktop(desktop); err != nil {
		return Snapshot{}, err
	}
	if err := cs.scanMacOS(ios, macos); err != nil {
		return Snapshot{}, err
	}
	iosCommit, iosDate := gitMeta(ios)
	andCommit, andDate := gitMeta(android)
	deskCommit, deskDate := gitMeta(desktop)
	macCommit, macDate := gitMeta(macos)
	sources := []Source{
		{Client: "ios", Repo: "https://github.com/TelegramMessenger/Telegram-iOS", Commit: iosCommit, Date: iosDate, Note: "TelegramCore Api.functions call sites, excluding Sources/MacOS"},
		{Client: "android", Repo: "https://github.com/DrKLO/Telegram", Commit: andCommit, Date: andDate, Note: "TL_* request constructions in Java and jni/tgnet"},
		{Client: "desktop", Repo: "https://github.com/telegramdesktop/tdesktop", Commit: deskCommit, Date: deskDate, Note: "MTPnamespace_Method requests in Telegram/SourceFiles"},
		{Client: "macos", Repo: "https://github.com/TelegramMessenger/telegram-macos", Commit: macCommit, Date: macDate, Note: "TelegramCore methods reached by telegram-macos (overtake/TelegramSwift is not published)"},
	}
	return cs.snapshot(layer, sources), nil
}
