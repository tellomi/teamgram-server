package report

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestSubsNilDatabase(t *testing.T) {
	ctx := context.Background()
	if _, err := (*mysqlStore)(nil).Subs(ctx, []int64{34}); !errors.Is(err, errSubjectsUnavailable) {
		t.Fatalf("nil store %v", err)
	}
	if _, err := (&mysqlStore{}).Subs(ctx, []int64{34}); !errors.Is(err, errSubjectsUnavailable) {
		t.Fatalf("nil db %v", err)
	}
	hits, err := (&mysqlStore{}).Subs(ctx, nil)
	if err != nil || len(hits) != 0 {
		t.Fatalf("empty input %+v %v", hits, err)
	}
	if _, err := (&mysqlStore{db: unusedDB(t)}).Subs(ctx, []int64{0}); !errors.Is(err, errSubjectsUnavailable) {
		t.Fatalf("non-positive %v", err)
	}

	h := Handler{Service: New(newMySQLStore(nil)), Key: bytesKey(t)}
	token := base64.StdEncoding.EncodeToString(h.Key)
	rec := postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)
}

func TestSubsFaultsAreNotEmpty(t *testing.T) {
	noUsers := &mysql.MySQLError{Number: 1146, Message: "Table 'teamgram.users' doesn't exist"}
	noSubject := &mysql.MySQLError{Number: 1146, Message: "Table 'teamgram.discover_subject' doesn't exist"}
	noDatabase := &mysql.MySQLError{Number: 1049, Message: "Unknown database 'teamgram'"}
	reset := errors.New("connection reset")

	t.Run("users table missing", func(t *testing.T) {
		script := newSubjectScript()
		script.usersErr = noUsers
		assertSubjectFailure(t, script, []int64{35}, noUsers)
		if script.subsCalls != 0 {
			t.Fatalf("classified after users failed, subject queries %d", script.subsCalls)
		}
	})
	t.Run("database missing", func(t *testing.T) {
		script := newSubjectScript()
		script.usersErr = noDatabase
		assertSubjectFailure(t, script, []int64{35}, noDatabase)
	})
	t.Run("subject table missing", func(t *testing.T) {
		script := newSubjectScript()
		script.users = []userFixture{{id: 35}}
		script.subsErr = noSubject
		assertSubjectFailure(t, script, []int64{35}, noSubject)
	})
	t.Run("cursor fails before a row", func(t *testing.T) {
		script := newSubjectScript()
		script.users = []userFixture{{id: 35}}
		script.usersFailAt = 0
		script.usersRowsErr = reset
		assertSubjectFailure(t, script, []int64{35}, reset)
	})
	t.Run("cursor fails after a row", func(t *testing.T) {
		script := newSubjectScript()
		script.users = []userFixture{{id: 34}, {id: 35}}
		script.usersFailAt = 1
		script.usersRowsErr = reset
		assertSubjectFailure(t, script, []int64{34, 35}, reset)
	})
	t.Run("short sub", func(t *testing.T) {
		script := liveSubjectScript(34, []byte{1, 2, 3})
		assertSubjectFailure(t, script, []int64{34}, errSubjectCorrupt)
	})
	t.Run("null sub", func(t *testing.T) {
		script := liveSubjectScript(34, nil)
		assertSubjectFailure(t, script, []int64{34}, errSubjectCorrupt)
	})
	t.Run("conflicting subs", func(t *testing.T) {
		script := newSubjectScript()
		script.users = []userFixture{{id: 34}}
		script.subs = []subFixture{
			{id: 34, sub: bytes.Repeat([]byte{1}, 16)},
			{id: 34, sub: bytes.Repeat([]byte{2}, 16)},
		}
		assertSubjectFailure(t, script, []int64{34}, errSubjectCorrupt)
	})
	t.Run("corrupt sub on a closed account", func(t *testing.T) {
		script := newSubjectScript()
		script.users = []userFixture{{id: 37, deleted: 1}}
		script.subs = []subFixture{{id: 37, sub: []byte{9}}}
		assertSubjectFailure(t, script, []int64{37}, errSubjectCorrupt)
	})
	t.Run("corrupt orphan sub", func(t *testing.T) {
		script := newSubjectScript()
		script.subs = []subFixture{{id: 36, sub: []byte{9}}}
		assertSubjectFailure(t, script, []int64{36}, errSubjectCorrupt)
	})
	t.Run("conflicting account flags", func(t *testing.T) {
		script := newSubjectScript()
		script.users = []userFixture{{id: 37, deleted: 1}, {id: 37}}
		assertSubjectFailure(t, script, []int64{37}, errSubjectCorrupt)
	})
}

func TestSubsClassifiesIdentity(t *testing.T) {
	mapped := bytes.Repeat([]byte{7}, 16)
	orphan := bytes.Repeat([]byte{8}, 16)
	closedSub := bytes.Repeat([]byte{9}, 16)
	script := newSubjectScript()
	script.users = []userFixture{
		{id: 34},
		{id: 35},
		{id: 37, deleted: 1},
		{id: 38, restricted: 1},
	}
	script.subs = []subFixture{
		{id: 34, sub: mapped},
		{id: 34, sub: append([]byte(nil), mapped...)},
		{id: 36, sub: orphan},
		{id: 37, sub: closedSub},
		{id: 38, sub: closedSub},
	}
	db := openSubjectDB(t, script)
	hits, err := newMySQLStore(db).Subs(context.Background(), []int64{36, 34, 35, 37, 38})
	if err != nil {
		t.Fatal(err)
	}
	want := []subjectHit{
		{UserID: 36, Status: subjectNotFound},
		{UserID: 34, Status: subjectMapped, Sub: mapped},
		{UserID: 35, Status: subjectEmpty},
		{UserID: 37, Status: subjectClosed},
		{UserID: 38, Status: subjectClosed},
	}
	if len(hits) != len(want) {
		t.Fatalf("%+v", hits)
	}
	for i := range want {
		if hits[i].UserID != want[i].UserID || hits[i].Status != want[i].Status || !bytes.Equal(hits[i].Sub, want[i].Sub) {
			t.Fatalf("hit %d %+v sub %x", i, hits[i], hits[i].Sub)
		}
	}

	h := Handler{Service: New(newMySQLStore(db)), Key: bytesKey(t)}
	token := base64.StdEncoding.EncodeToString(h.Key)
	rec := postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[36,34,35,37,38]}`)
	if rec.Code != 200 {
		t.Fatalf("http %d %s", rec.Code, rec.Body.String())
	}
	got := decodeByUsers(t, rec.Body.Bytes())
	encoded := base64.RawURLEncoding.EncodeToString(mapped)
	httpWant := []subjectByUser{
		{UserID: 36, Status: subjectNotFound},
		{UserID: 34, Status: subjectMapped, Sub: encoded},
		{UserID: 35, Status: subjectEmpty},
		{UserID: 37, Status: subjectClosed},
		{UserID: 38, Status: subjectClosed},
	}
	if len(got) != len(httpWant) {
		t.Fatalf("%+v", got)
	}
	for i := range httpWant {
		if got[i] != httpWant[i] {
			t.Fatalf("json %d %+v", i, got[i])
		}
	}
}

func assertSubjectFailure(t *testing.T, script *subjectScript, ids []int64, cause error) {
	t.Helper()
	db := openSubjectDB(t, script)
	hits, err := newMySQLStore(db).Subs(context.Background(), ids)
	if hits != nil || err == nil || !errors.Is(err, cause) {
		t.Fatalf("hits %+v err %v", hits, err)
	}
	if !errors.Is(err, errSubjectsUnavailable) && !errors.Is(err, errSubjectCorrupt) {
		t.Fatalf("unclassified failure %v", err)
	}
	h := Handler{Service: New(newMySQLStore(db)), Key: bytesKey(t)}
	token := base64.StdEncoding.EncodeToString(h.Key)
	body := `{"user_ids":[`
	for i, id := range ids {
		if i > 0 {
			body += ","
		}
		body += strconv.FormatInt(id, 10)
	}
	body += `]}`
	rec := postJSON(t, h, token, pathSubjectsByUsers, body)
	assertUnavailable(t, rec)
}

func liveSubjectScript(id int64, sub []byte) *subjectScript {
	script := newSubjectScript()
	script.users = []userFixture{{id: id}}
	script.subs = []subFixture{{id: id, sub: sub}}
	return script
}

type userFixture struct {
	id         int64
	deleted    int64
	restricted int64
}

type subFixture struct {
	id  int64
	sub []byte
}

type subjectScript struct {
	users        []userFixture
	subs         []subFixture
	usersErr     error
	subsErr      error
	usersFailAt  int
	subsFailAt   int
	usersRowsErr error
	subsRowsErr  error
	usersCalls   int
	subsCalls    int
}

func newSubjectScript() *subjectScript {
	return &subjectScript{usersFailAt: -1, subsFailAt: -1}
}

func (s *subjectScript) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "FROM users"):
		s.usersCalls++
		if s.usersErr != nil {
			return nil, s.usersErr
		}
		data := make([][]driver.Value, len(s.users))
		for i, row := range s.users {
			data[i] = []driver.Value{row.id, row.deleted, row.restricted}
		}
		return &scriptRows{cols: []string{"id", "deleted", "restricted"}, data: data, failAt: s.usersFailAt, fail: s.usersRowsErr}, nil
	case strings.Contains(query, "FROM discover_subject"):
		s.subsCalls++
		if s.subsErr != nil {
			return nil, s.subsErr
		}
		data := make([][]driver.Value, len(s.subs))
		for i, row := range s.subs {
			data[i] = []driver.Value{row.sub, row.id}
		}
		return &scriptRows{cols: []string{"sub", "user_id"}, data: data, failAt: s.subsFailAt, fail: s.subsRowsErr}, nil
	default:
		return nil, errors.New("unexpected query")
	}
}

type scriptRows struct {
	cols   []string
	data   [][]driver.Value
	i      int
	failAt int
	fail   error
}

func (r *scriptRows) Columns() []string { return r.cols }
func (r *scriptRows) Close() error      { return nil }
func (r *scriptRows) Next(dest []driver.Value) error {
	if r.failAt >= 0 && r.i == r.failAt {
		if r.fail == nil {
			r.fail = errors.New("rows failed")
		}
		return r.fail
	}
	if r.i >= len(r.data) {
		return io.EOF
	}
	row := r.data[r.i]
	r.i++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		}
	}
	return nil
}

type scriptConnector struct{ q driver.QueryerContext }

func (c scriptConnector) Connect(context.Context) (driver.Conn, error) {
	return &scriptConn{q: c.q}, nil
}
func (c scriptConnector) Driver() driver.Driver { return nopDriver{} }

type nopDriver struct{}

func (nopDriver) Open(string) (driver.Conn, error) { return nil, errors.New("unused") }

type scriptConn struct{ q driver.QueryerContext }

func (c *scriptConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("prepare") }
func (c *scriptConn) Close() error                        { return nil }
func (c *scriptConn) Begin() (driver.Tx, error)           { return nil, errors.New("begin") }
func (c *scriptConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.q.QueryContext(ctx, query, args)
}

func openSubjectDB(t *testing.T, script *subjectScript) *sql.DB {
	t.Helper()
	db := sql.OpenDB(scriptConnector{q: script})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func unusedDB(t *testing.T) *sql.DB {
	t.Helper()
	return openSubjectDB(t, newSubjectScript())
}
