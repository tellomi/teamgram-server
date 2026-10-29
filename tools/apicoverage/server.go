package main

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// variant is one generated RPC registration for a TL method.
type variant struct {
	GoName  string // AuthSendCode
	Service string // RPCAuthorization
	Method  string // auth.sendCode
}

// handlerInfo is the best implementation found for a Go RPC method.
type handlerInfo struct {
	Status string // implemented or stub
	Effect string
	File   string
	Note   string
}

// serverModel is the reachable BFF surface on main.
type serverModel struct {
	Routed     map[string]bool        // RPC service -> session.yaml IDMap is active
	Registered map[string]bool        // RPC service -> RegisterRPC*Server in the bff process
	Handlers   map[string]handlerInfo // Go method name -> classification
	Variants   map[string][]variant   // canonical method -> registrations
	Fakes      map[string]bool        // canonical method -> proxy returns a value
	Commented  map[string]bool        // RPC service commented out of session.yaml
	Startup    map[string]string      // canonical method -> effect of StartupRPCResult
}

var (
	registerLine = regexp.MustCompile(`RegisterRPC([A-Za-z0-9]+)Server`)
	idMapLine    = regexp.MustCompile(`^\s*(#)?\s*"(/mtproto\.RPC[A-Za-z0-9]+)"\s*:`)
	rpcRegLine   = regexp.MustCompile(`"(TL[A-Za-z0-9]+)"\s*:\s*RPCContextTuple\{"(/mtproto\.RPC[A-Za-z0-9]+)/([^"]+)"`)
	fakeCase     = regexp.MustCompile(`case "(TL[A-Za-z0-9]+)":`)
)

func loadServer(root string) (*serverModel, error) {
	m := &serverModel{
		Routed:     map[string]bool{},
		Registered: map[string]bool{},
		Handlers:   map[string]handlerInfo{},
		Variants:   map[string][]variant{},
		Fakes:      map[string]bool{},
		Commented:  map[string]bool{},
		Startup:    map[string]string{},
	}
	if err := m.parseIDMap(filepath.Join(root, "teamgramd/etc/session.yaml")); err != nil {
		return nil, err
	}
	if err := m.parseRegisters(filepath.Join(root, "proto/mtproto/rpc_client_registers.go")); err != nil {
		return nil, err
	}
	if err := m.parseFakes(filepath.Join(root, "app/bff/bff/client/fake_rpc_result.go")); err != nil {
		return nil, err
	}
	srv, err := os.ReadFile(filepath.Join(root, "app/bff/bff/internal/server/server.go"))
	if err != nil {
		return nil, err
	}
	for _, g := range registerLine.FindAllStringSubmatch(string(srv), -1) {
		m.Registered["RPC"+g[1]] = true
	}
	handlers, err := parseHandlers(filepath.Join(root, "app/bff"))
	if err != nil {
		return nil, err
	}
	m.Handlers = handlers
	if err := m.parseStartup(filepath.Join(root, "proto/mtproto/startup_rpc_fallback.go")); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *serverModel) parseIDMap(path string) error {
	if m.Routed == nil {
		m.Routed = map[string]bool{}
	}
	if m.Commented == nil {
		m.Commented = map[string]bool{}
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	in := false
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "IDMap:") {
			in = true
			continue
		}
		if !in {
			continue
		}
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") && !strings.HasPrefix(strings.TrimSpace(line), "\"") && !strings.HasPrefix(strings.TrimSpace(line), "/") {
			// Next YAML key at the same or lesser indent ends the map.
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				break
			}
		}
		g := idMapLine.FindStringSubmatch(line)
		if g == nil {
			continue
		}
		svc := strings.TrimPrefix(g[2], "/mtproto.")
		if g[1] == "#" {
			m.Commented[svc] = true
			continue
		}
		m.Routed[svc] = true
	}
	return sc.Err()
}

func (m *serverModel) parseRegisters(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, g := range rpcRegLine.FindAllStringSubmatch(string(b), -1) {
		goName := strings.TrimPrefix(g[1], "TL")
		svc := strings.TrimPrefix(g[2], "/mtproto.")
		method := canonicalGRPC(g[3])
		m.Variants[method] = append(m.Variants[method], variant{
			GoName:  goName,
			Service: svc,
			Method:  method,
		})
	}
	return nil
}

func (m *serverModel) parseFakes(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(b)
	// Cases that return a value live above the enterprise-blocked fallback.
	cut := strings.Index(text, "blocked, License key")
	head := text
	if cut > 0 {
		head = text[:cut]
	}
	byGo := map[string]string{}
	for method, vars := range m.Variants {
		for _, v := range vars {
			byGo[v.GoName] = method
		}
	}
	for _, g := range fakeCase.FindAllStringSubmatch(head, -1) {
		goName := strings.TrimPrefix(g[1], "TL")
		if method, ok := byGo[goName]; ok {
			m.Fakes[method] = true
		}
	}
	return nil
}

type fnBody struct {
	Name    string
	File    string
	Recv    string
	Body    *ast.BlockStmt
	FileAST *ast.File
	Imports map[string]string // local name -> path
}

func parseHandlers(root string) (map[string]handlerInfo, error) {
	fset := token.NewFileSet()
	var funcs []fnBody
	var helpers []fnBody
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		af, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			// A file that does not parse contributes no handlers.
			return nil
		}
		imports := map[string]string{}
		for _, im := range af.Imports {
			p := strings.Trim(im.Path.Value, `"`)
			local := filepath.Base(p)
			if im.Name != nil {
				local = im.Name.Name
			}
			imports[local] = p
		}
		for _, d := range af.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			recv := ""
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recv = typeName(fn.Recv.List[0].Type)
			}
			body := fnBody{
				Name:    fn.Name.Name,
				File:    path,
				Recv:    recv,
				Body:    fn.Body,
				FileAST: af,
				Imports: imports,
			}
			helpers = append(helpers, body)
			if fn.Recv != nil && fn.Type.Params != nil && looksLikeRPC(fn) {
				funcs = append(funcs, body)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	byName := map[string][]fnBody{}
	for _, fn := range funcs {
		byName[fn.Name] = append(byName[fn.Name], fn)
	}
	for _, fn := range helpers {
		byName[fn.Name] = append(byName[fn.Name], fn)
	}
	rpcNames := map[string][]fnBody{}
	for _, fn := range funcs {
		rpcNames[fn.Name] = append(rpcNames[fn.Name], fn)
	}
	out := map[string]handlerInfo{}
	for name, group := range rpcNames {
		out[name] = classifyGroup(name, group, byName)
	}
	return out, nil
}

func selectorExternal(fun ast.Expr) bool {
	s, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if s.Sel.Name == "Shared" {
		return true
	}
	return chainExternal(s.X)
}

func chainExternal(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "svcCtx" || x.Name == "Dao" || strings.HasSuffix(x.Name, "Client")
	case *ast.SelectorExpr:
		if x.Sel.Name == "Dao" || x.Sel.Name == "svcCtx" || x.Sel.Name == "Shared" || strings.HasSuffix(x.Sel.Name, "Client") {
			return true
		}
		return chainExternal(x.X)
	default:
		return false
	}
}

func looksLikeRPC(fn *ast.FuncDecl) bool {
	params := fn.Type.Params.List
	if len(params) < 2 || fn.Type.Results == nil {
		return false
	}
	// First param is context.Context. Second is *mtproto.TL...
	tn := typeName(params[0].Type)
	if tn != "Context" && !strings.HasSuffix(tn, ".Context") {
		return false
	}
	second := typeName(params[1].Type)
	return strings.Contains(second, "TL")
}

func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.SelectorExpr:
		return typeName(t.X) + "." + t.Sel.Name
	case *ast.IndexExpr:
		return typeName(t.X)
	default:
		return ""
	}
}

func classifyGroup(name string, group []fnBody, byName map[string][]fnBody) handlerInfo {
	// Prefer a non-delegating body. Service wrappers only forward to core.
	best := handlerInfo{Status: statusStub, Effect: effectEmpty}
	bestRank := -1
	for _, fn := range group {
		st, effect, note := classifyFunc(fn, byName, map[string]bool{}, 0)
		rank := 0
		if st == statusImplemented {
			rank = 2
		} else if effect == effectError {
			rank = 1
		}
		// Prefer internal/core and handlers.go over the thin service wrapper when ranks tie.
		if strings.Contains(fn.File, "/internal/core/") || strings.HasSuffix(fn.File, "handlers.go") || strings.Contains(fn.File, "handlers_") {
			rank += 0 // rank stays; file is recorded from the winner
		}
		if rank > bestRank || (rank == bestRank && preferFile(fn.File, best.File)) {
			bestRank = rank
			best = handlerInfo{Status: st, Effect: effect, File: relBFF(fn.File), Note: note}
		}
	}
	return best
}

func preferFile(path, cur string) bool {
	if cur == "" {
		return true
	}
	score := func(p string) int {
		switch {
		case strings.Contains(p, "/internal/core/"):
			return 3
		case strings.Contains(p, "handlers"):
			return 2
		default:
			return 1
		}
	}
	return score(path) > score(cur)
}

func relBFF(path string) string {
	i := strings.Index(path, "app/bff/")
	if i >= 0 {
		return path[i:]
	}
	return path
}

var allowPkg = map[string]bool{
	"mtproto": true, "logx": true, "log": true, "fmt": true, "time": true,
	"strings": true, "strconv": true, "errors": true, "sort": true, "bytes": true,
	"math": true, "sync": true, "json": true, "jsonx": true, "proto": true,
	"metadata": true, "os": true, "context": true, "reflect": true, "io": true,
	"path": true, "filepath": true, "regexp": true, "hex": true, "base64": true,
	"binary": true, "net": true, "url": true, "http": true, "unicode": true,
	"mr": true, "utf8": true, "bufio": true, "color": true,
}

var logFuncs = map[string]bool{
	"Debugf": true, "Infof": true, "Errorf": true, "Warnf": true, "Error": true,
	"Info": true, "Debug": true, "Warn": true, "WithContext": true, "Slowf": true,
}

var skipFollow = map[string]bool{
	"New": true, "String": true, "Error": true, "Errorf": true, "Debugf": true,
	"Infof": true, "Warnf": true, "WithContext": true,
}

func classifyFunc(fn fnBody, byName map[string][]fnBody, seen map[string]bool, depth int) (status, effect, note string) {
	if fn.Body == nil || depth > 4 {
		return statusStub, effectEmpty, ""
	}
	key := fn.File + "#" + fn.Name + "#" + fn.Recv
	if seen[key] {
		return statusStub, effectEmpty, ""
	}
	seen[key] = true

	var (
		external   bool
		enterprise bool
		notMod     bool
		boolOnly   = true
		hasValue   bool
		longString bool
		followed   []fnBody
	)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			pkg, sel := callPkg(x.Fun)
			if pkg != "" {
				if path, isImport := fn.Imports[pkg]; isImport && !allowPkg[pkg] && !stdlibImport(path) {
					external = true
				}
				if strings.Contains(sel, "NotModified") {
					notMod = true
				}
				if logFuncs[sel] {
					return true
				}
			}
			if selectorExternal(x.Fun) {
				external = true
			}
			name := callName(x.Fun)
			if name != "" && !skipFollow[name] && !logFuncs[name] {
				followed = append(followed, followTargets(name, fn, byName)...)
			}
			if sel == "ErrEnterpriseIsBlocked" || sel == "ErrMethodNotImpl" || sel == "ErrPaymentUnsupported" {
				enterprise = true
			}
		case *ast.SelectorExpr:
			if x.Sel.Name == "ErrEnterpriseIsBlocked" || x.Sel.Name == "ErrMethodNotImpl" || x.Sel.Name == "ErrPaymentUnsupported" {
				enterprise = true
			}
			if x.Sel.Name == "BoolTrue" || x.Sel.Name == "BoolFalse" {
				// recorded via returns
			}
			if strings.Contains(x.Sel.Name, "NotModified") {
				notMod = true
			}
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				s := strings.Trim(x.Value, `"`)
				if len(s) >= 16 && !strings.Contains(s, "error:") {
					longString = true
				}
			}
		}
		return true
	})

	// Value returns that are not only nil.
	hasNonBoolValue := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, r := range ret.Results {
			if ident, ok := r.(*ast.Ident); ok && ident.Name == "nil" {
				continue
			}
			hasValue = true
			if !isBoolExpr(r) {
				hasNonBoolValue = true
				boolOnly = false
			}
		}
		return true
	})
	if !hasValue {
		boolOnly = false
	} else if hasNonBoolValue {
		boolOnly = false
	}

	for _, c := range followed {
		st, ef, _ := classifyFunc(c, byName, seen, depth+1)
		if st == statusImplemented {
			external = true
			longString = true
		}
		if ef == effectError {
			enterprise = true
		}
		if ef == effectNotModified {
			notMod = true
		}
		if ef == effectBool {
			boolOnly = boolOnly || true
		}
	}

	switch {
	case external || longString:
		note := ""
		if enterprise {
			note = "partial enterprise gate"
		}
		return statusImplemented, effectOK, note
	case enterprise && !hasNonBoolValue:
		return statusStub, effectError, "returns ENTERPRISE_IS_BLOCKED or METHOD_NOT_IMPL"
	case notMod:
		return statusStub, effectNotModified, "returns NotModified"
	case boolOnly && hasValue:
		return statusStub, effectBool, "returns Bool only"
	default:
		return statusStub, effectEmpty, "returns an empty object"
	}
}

func followTargets(name string, fn fnBody, byName map[string][]fnBody) []fnBody {
	var sameFile, sameDir, rest []fnBody
	dir := filepath.Dir(fn.File)
	for _, c := range byName[name] {
		if c.File == fn.File && c.Name == fn.Name && c.Recv == fn.Recv {
			continue
		}
		switch {
		case c.File == fn.File:
			sameFile = append(sameFile, c)
		case filepath.Dir(c.File) == dir:
			sameDir = append(sameDir, c)
		default:
			rest = append(rest, c)
		}
	}
	if len(sameFile) > 0 {
		return sameFile
	}
	if len(sameDir) > 0 {
		return sameDir
	}
	// Service wrappers call the core method of the same name in another directory.
	if name == fn.Name {
		return rest
	}
	return nil
}

func callPkg(fun ast.Expr) (pkg, sel string) {
	s, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	id, ok := s.X.(*ast.Ident)
	if !ok {
		return "", s.Sel.Name
	}
	if len(id.Name) <= 1 {
		return "", s.Sel.Name
	}
	return id.Name, s.Sel.Name
}

func callName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	default:
		return ""
	}
}

func stdlibImport(path string) bool {
	if path == "" {
		return false
	}
	// Standard library import paths have no dot in the first element.
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

func isBoolExpr(e ast.Expr) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return s.Sel.Name == "BoolTrue" || s.Sel.Name == "BoolFalse"
}

// resolve picks the client-visible status for a canonical method.
func (m *serverModel) resolve(method string) (status, effect, service, handler, notes string, routed bool) {
	vars := m.Variants[method]
	if len(vars) == 0 {
		if m.Fakes[method] {
			return statusStub, effectEmpty, "", "absent", "no generated RPC; session proxy returns an empty object", false
		}
		return statusMissing, effectError, "", "absent", "no route in session.yaml and no bff handler", false
	}
	type cand struct {
		status, effect, service, handler, notes string
		routed                                  bool
		rank                                    int
	}
	var cs []cand
	for _, v := range vars {
		h, ok := m.Handlers[v.GoName]
		routed := m.Routed[v.Service]
		registered := m.Registered[v.Service]
		c := cand{service: v.Service, routed: routed}
		switch {
		case routed && registered && ok && h.Status == statusImplemented:
			c.status, c.effect, c.handler = statusImplemented, effectOK, "implemented"
			c.notes = strings.TrimSpace(h.Note + " " + h.File)
			c.rank = 50
		case routed && registered && ok && h.Status == statusStub:
			c.status, c.effect, c.handler = statusStub, h.Effect, "stub"
			c.notes = strings.TrimSpace(h.Note + " " + h.File)
			c.rank = 40
		case routed && (!registered || !ok):
			c.status, c.effect, c.handler = statusRoutedUnregistered, effectError, "absent"
			c.notes = "session.yaml routes " + v.Service + " but the bff process has no " + v.GoName + " handler"
			if ok {
				c.handler = h.Status
				c.notes = "session.yaml routes " + v.Service + " but it is not RegisterRPC'd in app/bff/bff/internal/server/server.go (" + h.File + ")"
			}
			c.rank = 30
		case !routed && m.Fakes[method]:
			c.status, c.effect = statusStub, effectEmpty
			c.handler = "absent"
			if ok {
				c.handler = h.Status
			}
			c.notes = "session.yaml does not route " + v.Service + "; proxy returns an empty object"
			if m.Commented[v.Service] {
				c.notes += " (IDMap entry is commented out)"
			}
			c.rank = 20
		default:
			c.status, c.effect = statusMissing, effectError
			c.handler = "absent"
			c.notes = "no route in session.yaml"
			if m.Commented[v.Service] {
				c.notes += " (" + v.Service + " is commented out)"
			}
			if ok {
				c.handler = h.Status
				c.notes += "; handler source is " + h.Status + " at " + h.File + " but the service is not routed"
				if h.Effect == effectError || h.Status == statusStub {
					c.notes += " (" + h.Note + ")"
				}
			} else {
				c.notes += " and no bff handler"
			}
			c.rank = 10
		}
		cs = append(cs, c)
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].rank > cs[j].rank })
	best := cs[0]
	if len(cs) > 1 {
		disagree := false
		for _, c := range cs[1:] {
			if c.status != best.status {
				disagree = true
				break
			}
		}
		if disagree {
			best.notes = strings.TrimSpace(best.notes + " (layer variants disagree; best reachable status kept)")
		}
	}
	// A routed handler is what the client runs. Below that, an unregistered
	// or unrouted method is answered by StartupRPCResult instead of an RPC error.
	if best.rank < 40 && best.effect == effectError {
		if effect, ok := m.Startup[method]; ok {
			note := "typed startup fallback"
			if best.notes != "" {
				note += "; " + best.notes
			}
			return statusStub, effect, best.service, "stub", strings.TrimSpace(note), best.routed
		}
	}
	return best.status, best.effect, best.service, best.handler, strings.TrimSpace(best.notes), best.routed
}

// parseStartup records TL methods that StartupRPCResult answers with a typed
// object. Effect follows the returned constructor: NotModified, Bool, or empty.
func (m *serverModel) parseStartup(path string) error {
	if m.Startup == nil {
		m.Startup = map[string]string{}
	}
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	byGo := map[string]string{}
	for method, vars := range m.Variants {
		for _, v := range vars {
			byGo[v.GoName] = method
		}
	}
	for _, d := range af.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "StartupRPCResult" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSwitchStmt)
			if !ok {
				return true
			}
			for _, stmt := range ts.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				effect := effectOfStmts(clause.Body)
				for _, name := range caseTypeNames(clause.List) {
					method, ok := byGo[strings.TrimPrefix(name, "TL")]
					if !ok {
						continue
					}
					if prev, exists := m.Startup[method]; exists && prev != effect {
						m.Startup[method] = preferEffect(prev, effect)
						continue
					}
					m.Startup[method] = effect
				}
			}
			return false
		})
	}
	return nil
}

func caseTypeNames(list []ast.Expr) []string {
	var names []string
	for _, e := range list {
		if star, ok := e.(*ast.StarExpr); ok {
			e = star.X
		}
		id, ok := e.(*ast.Ident)
		if !ok {
			continue
		}
		names = append(names, id.Name)
	}
	return names
}

func effectOfStmts(stmts []ast.Stmt) string {
	sawNotMod, sawBool, sawOther := false, false, false
	ast.Inspect(&ast.BlockStmt{List: stmts}, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, r := range ret.Results {
			if id, ok := r.(*ast.Ident); ok && (id.Name == "true" || id.Name == "false" || id.Name == "nil") {
				continue
			}
			if isBoolResult(r) {
				sawBool = true
				continue
			}
			sawOther = true
			if exprNotModified(r) {
				sawNotMod = true
			}
		}
		return true
	})
	switch {
	case sawNotMod:
		return effectNotModified
	case sawBool && !sawOther:
		return effectBool
	default:
		return effectEmpty
	}
}

func isBoolResult(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "BoolTrue" || x.Name == "BoolFalse"
	case *ast.SelectorExpr:
		return x.Sel.Name == "BoolTrue" || x.Sel.Name == "BoolFalse"
	default:
		return false
	}
}

func exprNotModified(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if strings.Contains(x.Name, "NotModified") {
				found = true
			}
		case *ast.SelectorExpr:
			if strings.Contains(x.Sel.Name, "NotModified") {
				found = true
			}
		}
		return true
	})
	return found
}

func preferEffect(a, b string) string {
	rank := func(e string) int {
		switch e {
		case effectNotModified:
			return 3
		case effectBool:
			return 2
		default:
			return 1
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}
