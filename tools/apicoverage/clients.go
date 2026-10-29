package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	reIOS    = regexp.MustCompile(`Api\.functions\.([A-Za-z0-9_]+)\.([A-Za-z0-9_]+)\s*\(`)
	reAndNS  = regexp.MustCompile(`\bTL_([A-Za-z0-9]+)\.([A-Za-z][A-Za-z0-9_]*)\s*\(`)
	reAndOld = regexp.MustCompile(`\bTL_([A-Za-z0-9_]+)\s*\(`)
	// Paren is a direct construction. "::" and ">" cover flag builders and
	// PrepareMessage<MTPmessages_SendMessage>, which is how Desktop sends.
	reDesk  = regexp.MustCompile(`\bMTP([a-z][A-Za-z0-9]*)_([A-Z][A-Za-z0-9]*)(?:\s*\(|\s*::|\s*>)`)
	reSwift = regexp.MustCompile(`\bfunc\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*[(<]`)
	reJava  = regexp.MustCompile(`(?:public|private|protected)\s+(?:static\s+)?(?:final\s+)?(?:synchronized\s+)?[\w.<>,\[\]]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	reCPP   = regexp.MustCompile(`\b(?:([A-Za-z_][A-Za-z0-9_]*)::)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	reCall  = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

var verbs = []string{
	"unregister", "register", "reorder", "resolve", "request", "recover", "report",
	"restrict", "translate", "toggle", "search", "import", "export", "accept",
	"finish", "cancel", "delete", "create", "update", "upload", "discard",
	"confirm", "verify", "install", "unblock", "forward", "compose", "connect",
	"dismiss", "refresh", "migrate", "convert", "assign", "resend", "reset",
	"check", "leave", "invite", "block", "click", "start", "apply", "clear",
	"save", "send", "hide", "mark", "read", "edit", "join", "init", "view",
	"vote", "fave", "drop", "bind", "stop", "sign", "load", "get", "set", "add",
	"pin",
}

var genericCall = map[string]bool{
	"init": true, "body": true, "then": true, "map": true, "filter": true,
	"reduce": true, "flatMap": true, "request": true, "start": true, "dispose": true,
	"catch": true, "take": true, "single": true, "fail": true, "complete": true,
	"subscribe": true, "after": true, "timeout": true, "delay": true, "run": true,
	"call": true, "get": true, "set": true, "update": true, "load": true, "apply": true,
	"process": true, "handle": true, "reset": true, "merge": true, "append": true,
	"remove": true, "contains": true, "first": true, "last": true, "count": true,
	"make": true, "new": true, "copy": true, "read": true, "write": true, "open": true,
	"close": true, "send": true, "post": true, "add": true, "signal": true,
	"String": true, "Error": true, "mapToSignal": true,
	"deliverOn": true, "deliverOnMainQueue": true, "switchToLatest": true,
	"startStrict": true, "startStandalone": true, "ignoreValues": true,
	"retryRequest": true, "combineLatest": true, "distinctUntilChanged": true,
}

// lifecyclePrefix marks TelegramCore files the macOS app runs because it
// constructs Account / TelegramEngine, not because a UI file named the helper.
var lifecyclePrefix = []string{
	"State/",
	"Account/",
	"Network/",
	"SyncCore/",
	"PendingMessages/",
	"Authorization.swift",
	"TelegramEngine/Localization/",
	"TelegramEngine/Auth/",
	"TelegramEngine/AccountData/RegisterNotificationToken.swift",
	"TelegramEngine/AccountData/TermsOfService.swift",
	"TelegramEngine/Contacts/ContactManagement.swift",
	"MacOS/",
}

type callSet struct {
	schema map[string]bool
	nss    []string
	// method -> client -> file -> func set
	hits map[string]map[string]map[string]map[string]struct{}
}

func newCallSet(schema map[string]bool, nss []string) *callSet {
	return &callSet{schema: schema, nss: nss, hits: map[string]map[string]map[string]map[string]struct{}{}}
}

func (c *callSet) add(client, method, file, fn string) {
	if !acceptMethod(method, c.schema) {
		return
	}
	if c.hits[method] == nil {
		c.hits[method] = map[string]map[string]map[string]struct{}{}
	}
	if c.hits[method][client] == nil {
		c.hits[method][client] = map[string]map[string]struct{}{}
	}
	if c.hits[method][client][file] == nil {
		c.hits[method][client][file] = map[string]struct{}{}
	}
	if fn == "" {
		fn = "?"
	}
	c.hits[method][client][file][fn] = struct{}{}
}

func acceptMethod(method string, schema map[string]bool) bool {
	if schema[method] {
		return true
	}
	_, name, ok := strings.Cut(method, ".")
	if !ok || name == "" {
		return false
	}
	return verbMethod(name)
}

func verbMethod(name string) bool {
	for _, v := range verbs {
		if name == v {
			return true
		}
		if strings.HasPrefix(name, v) && len(name) > len(v) {
			r := rune(name[len(v)])
			if unicode.IsUpper(r) {
				return true
			}
		}
	}
	return false
}

func (c *callSet) snapshot(layer int, sources []Source) Snapshot {
	var methods []MethodCalls
	for method, byClient := range c.hits {
		var sites []Site
		for client, files := range byClient {
			for file, fns := range files {
				names := make([]string, 0, len(fns))
				for fn := range fns {
					names = append(names, fn)
				}
				sort.Strings(names)
				if len(names) > 8 {
					names = names[:8]
				}
				for _, fn := range names {
					sites = append(sites, Site{Client: client, File: file, Func: fn})
				}
			}
		}
		sort.Slice(sites, func(i, j int) bool {
			if sites[i].Client != sites[j].Client {
				return sites[i].Client < sites[j].Client
			}
			if sites[i].File != sites[j].File {
				return sites[i].File < sites[j].File
			}
			return sites[i].Func < sites[j].Func
		})
		methods = append(methods, MethodCalls{Method: method, Sites: sites})
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].Method < methods[j].Method })
	sort.Slice(sources, func(i, j int) bool { return sources[i].Client < sources[j].Client })
	return Snapshot{Layer: layer, Sources: sources, Methods: methods}
}

func skipScan(path string) bool {
	p := filepath.ToSlash(path)
	base := filepath.Base(p)
	switch base {
	case "TLRPC.java", "ApiScheme.cpp", "ApiScheme.h", "MTProtoScheme.cpp", "MTProtoScheme.h":
		return true
	}
	if strings.HasSuffix(base, "Test.java") || strings.HasSuffix(base, "Tests.java") {
		return true
	}
	for _, s := range []string{"/test/", "/tests/", "/tgnet/tl/", "/mtproto/scheme/", "/.git/"} {
		if strings.Contains(p, s) {
			return true
		}
	}
	return false
}

func scanRoot(root string, ext string, visit func(rel string, lines []string)) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "third-party" || base == "ffmpeg" {
				return filepath.SkipDir
			}
			return nil
		}
		if ext != "" && !strings.HasSuffix(path, ext) {
			return nil
		}
		if skipScan(path) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		var lines []string
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		visit(filepath.ToSlash(rel), lines)
		return sc.Err()
	})
}

func enclosing(lines []string, idx int, kind string) string {
	limit := idx - 80
	if limit < 0 {
		limit = 0
	}
	for i := idx; i >= limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "/*") {
			continue
		}
		switch kind {
		case "swift":
			if m := reSwift.FindStringSubmatch(line); m != nil {
				return m[1]
			}
		case "java":
			if strings.Contains(line, " class ") || strings.HasPrefix(line, "class ") || strings.Contains(line, " new ") {
				continue
			}
			if m := reJava.FindStringSubmatch(line); m != nil {
				return m[1]
			}
		case "cpp":
			if m := reCPP.FindStringSubmatch(line); m != nil {
				name := m[2]
				if name != "if" && name != "for" && name != "while" && name != "switch" && name != "return" {
					return name
				}
			}
		}
	}
	return ""
}

func splitLegacy(ident string, nss []string) (string, bool) {
	for _, ns := range nss {
		prefix := ns + "_"
		if strings.HasPrefix(ident, prefix) {
			rest := stripConstructor(strings.TrimPrefix(ident, prefix))
			if rest == "" {
				return "", false
			}
			return ns + "." + rest, true
		}
	}
	return "", false
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// scanIOS records TelegramCore Api.functions call sites.
// Sources/MacOS is attributed to macos only; every other file is ios.
func (c *callSet) scanIOS(root string) error {
	return scanRoot(root, ".swift", func(rel string, lines []string) {
		client := "ios"
		if strings.HasPrefix(rel, "MacOS/") {
			client = "macos"
		}
		for i, line := range lines {
			for _, m := range reIOS.FindAllStringSubmatch(line, -1) {
				c.add(client, m[1]+"."+m[2], rel, enclosing(lines, i, "swift"))
			}
		}
	})
}

func (c *callSet) scanAndroid(root string) error {
	walk := func(rel string, lines []string, kind string) {
		for i, line := range lines {
			if strings.Contains(line, " class TL_") || strings.Contains(line, "class TL_") {
				continue
			}
			fn := enclosing(lines, i, kind)
			for _, m := range reAndNS.FindAllStringSubmatch(line, -1) {
				c.add("android", m[1]+"."+m[2], rel, fn)
			}
			for _, m := range reAndOld.FindAllStringSubmatch(line, -1) {
				method, ok := splitLegacy(m[1], c.nss)
				if !ok {
					continue
				}
				c.add("android", method, rel, fn)
			}
		}
	}
	if err := scanRoot(root, ".java", func(rel string, lines []string) { walk(rel, lines, "java") }); err != nil {
		return err
	}
	return scanRoot(root, ".cpp", func(rel string, lines []string) { walk(rel, lines, "cpp") })
}

func (c *callSet) scanDesktop(root string) error {
	return scanRoot(root, "", func(rel string, lines []string) {
		if !strings.HasSuffix(rel, ".cpp") && !strings.HasSuffix(rel, ".h") {
			return
		}
		for i, line := range lines {
			fn := enclosing(lines, i, "cpp")
			for _, m := range reDesk.FindAllStringSubmatch(line, -1) {
				c.add("desktop", m[1]+"."+lowerFirst(m[2]), rel, fn)
			}
		}
	})
}

type swiftFunc struct {
	name    string
	file    string
	methods []string
	calls   []string
	life    bool
}

func isLifecycle(rel string) bool {
	for _, p := range lifecyclePrefix {
		if rel == strings.TrimSuffix(p, "/") || strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

// scanMacOS marks TelegramCore methods the macOS app can reach.
// telegram-macos does not call Api.functions itself; it links this TelegramCore.
func (c *callSet) scanMacOS(coreRoot, appRoot string) error {
	var funcs []swiftFunc
	err := scanRoot(coreRoot, ".swift", func(rel string, lines []string) {
		funcs = append(funcs, parseSwiftFuncs(rel, lines)...)
	})
	if err != nil {
		return err
	}
	byName := map[string][]int{}
	for i, fn := range funcs {
		byName[fn.name] = append(byName[fn.name], i)
	}
	seeds := map[string]bool{}
	for _, fn := range funcs {
		if !fn.life {
			continue
		}
		// Direct RPCs in session-lifecycle files count even when the Swift
		// function name is too generic to use as a graph edge.
		for _, method := range fn.methods {
			c.add("macos", method, fn.file, fn.name)
		}
		if genericCall[fn.name] {
			for _, callee := range fn.calls {
				if !genericCall[callee] {
					seeds[callee] = true
				}
			}
			continue
		}
		seeds[fn.name] = true
	}
	if appRoot != "" {
		_ = scanRoot(appRoot, ".swift", func(rel string, lines []string) {
			if strings.Contains(rel, "/Tests/") || strings.Contains(rel, "Tests/") {
				return
			}
			for _, line := range lines {
				for _, m := range reCall.FindAllStringSubmatch(line, -1) {
					name := m[1]
					if genericCall[name] || len(name) < 6 {
						continue
					}
					if _, ok := byName[name]; ok {
						seeds[name] = true
					}
				}
			}
		})
	}
	seen := map[int]bool{}
	queue := []int{}
	for name := range seeds {
		for _, idx := range byName[name] {
			if !seen[idx] {
				seen[idx] = true
				queue = append(queue, idx)
			}
		}
	}
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		fn := funcs[idx]
		for _, method := range fn.methods {
			c.add("macos", method, fn.file, fn.name)
		}
		for _, callee := range fn.calls {
			if genericCall[callee] {
				continue
			}
			for _, j := range byName[callee] {
				if !seen[j] {
					seen[j] = true
					queue = append(queue, j)
				}
			}
		}
	}
	return nil
}

func parseSwiftFuncs(rel string, lines []string) []swiftFunc {
	text := strings.Join(lines, "\n")
	type frame struct {
		name  string
		depth int
		fn    swiftFunc
	}
	var (
		stack   []frame
		depth   int
		pending string
		out     []swiftFunc
		life    = isLifecycle(rel)
	)
	flush := func(fn swiftFunc) {
		if len(fn.methods) == 0 && len(fn.calls) == 0 {
			return
		}
		fn.life = life
		out = append(out, fn)
	}
	i := 0
	for i < len(text) {
		switch text[i] {
		case '/':
			if i+1 < len(text) && text[i+1] == '/' {
				for i < len(text) && text[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(text) && text[i+1] == '*' {
				i += 2
				for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
					i++
				}
				i += 2
				continue
			}
		case '"':
			i++
			for i < len(text) && text[i] != '"' {
				if text[i] == '\\' && i+1 < len(text) {
					i += 2
					continue
				}
				i++
			}
			if i < len(text) {
				i++
			}
			continue
		case '{':
			depth++
			if pending != "" {
				stack = append(stack, frame{name: pending, depth: depth, fn: swiftFunc{name: pending, file: rel}})
				pending = ""
			}
			i++
			continue
		case '}':
			depth--
			if len(stack) > 0 && depth < stack[len(stack)-1].depth {
				flush(stack[len(stack)-1].fn)
				stack = stack[:len(stack)-1]
			}
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "func ") || (i == 0 && strings.HasPrefix(text, "func")) {
			if m := reSwift.FindStringSubmatch(text[i:]); m != nil && (i == 0 || !isIdentByte(text[i-1])) {
				pending = m[1]
			}
		}
		if strings.HasPrefix(text[i:], "Api.functions.") {
			if m := reIOS.FindStringSubmatch(text[i:]); m != nil && len(stack) > 0 {
				stack[len(stack)-1].fn.methods = append(stack[len(stack)-1].fn.methods, m[1]+"."+m[2])
				i += len(m[0])
				continue
			}
		}
		if isIdentStart(text[i]) && len(stack) > 0 {
			j := i + 1
			for j < len(text) && isIdentByte(text[j]) {
				j++
			}
			name := text[i:j]
			k := j
			for k < len(text) && (text[k] == ' ' || text[k] == '\n' || text[k] == '\t') {
				k++
			}
			if k < len(text) && text[k] == '(' && !genericCall[name] && len(name) >= 4 {
				stack[len(stack)-1].fn.calls = append(stack[len(stack)-1].fn.calls, name)
			}
			i = j
			continue
		}
		i++
	}
	for _, fr := range stack {
		flush(fr.fn)
	}
	// A lifecycle file with only top-level calls still counts: rescan lines.
	if life {
		var methods []string
		for _, line := range lines {
			for _, m := range reIOS.FindAllStringSubmatch(line, -1) {
				methods = append(methods, m[1]+"."+m[2])
			}
		}
		if len(methods) > 0 {
			out = append(out, swiftFunc{name: "lifecycle", file: rel, methods: methods, life: true})
		}
	}
	return out
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isIdentByte(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func gitMeta(dir string) (commit, date string) {
	if dir == "" {
		return "", ""
	}
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%H%n%cI").Output()
	if err != nil {
		return "", ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		commit = lines[0]
	}
	if len(lines) > 1 {
		date = lines[1]
	}
	if len(commit) > 12 {
		commit = commit[:12]
	}
	return commit, date
}
