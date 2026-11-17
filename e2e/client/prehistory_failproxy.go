package e2e

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// participantSelect is the text mysqlStore.GetParticipant sends. The proxy
// fails a later execute of this statement so the RPC still completes and the
// prehistory filter sees the member-row error.
const participantSelect = "SELECT channel_id, user_id, role, state, admin_rights, banned_rights, rank_name, inviter_user_id, is_bot, date2 FROM channel_participants WHERE channel_id=? AND user_id=?"

const (
	cmdQuit         byte = 0x01
	cmdQuery        byte = 0x03
	cmdStmtPrepare  byte = 0x16
	cmdStmtExecute  byte = 0x17
	cmdStmtSendLong byte = 0x18
	cmdStmtClose    byte = 0x19
	mysqlAuthMore   byte = 0x01
	mysqlOK         byte = 0x00
	mysqlEOF        byte = 0xfe
	mysqlErr        byte = 0xff
	cachingFastAuth byte = 3
	cachingFullAuth byte = 4
)

// participantFaultProxy sits in front of one isolated MySQL and can fail
// GetParticipant for one user after a set number of successful executes.
// Every other statement is forwarded. It is not part of the server.
type participantFaultProxy struct {
	upstream string
	ln       net.Listener
	done     chan struct{}

	mu            sync.Mutex
	armedUser     int64
	allow         int
	failFeatRead  bool
	failFeatWrite bool
	fails         int
	lines         []string
}

func startParticipantFaultProxy(upstream string) (*participantFaultProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &participantFaultProxy{
		upstream: upstream,
		ln:       ln,
		done:     make(chan struct{}),
	}
	go p.accept()
	return p, nil
}

func (p *participantFaultProxy) Addr() string {
	return p.ln.Addr().String()
}

func (p *participantFaultProxy) Close() {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	_ = p.ln.Close()
}

// Arm fails GetParticipant for user once allow successful executes of that
// statement have been forwarded. allow 1 lets requireView through and fails
// the read that decides whether the body is returned.
func (p *participantFaultProxy) Arm(user int64, allow int) {
	p.mu.Lock()
	p.armedUser = user
	p.allow = allow
	p.mu.Unlock()
}

func (p *participantFaultProxy) Disarm() {
	p.mu.Lock()
	p.armedUser = 0
	p.allow = 0
	p.failFeatRead = false
	p.failFeatWrite = false
	p.mu.Unlock()
}

// ArmFeatureRead fails the next SELECT of channel_feature.
func (p *participantFaultProxy) ArmFeatureRead() {
	p.mu.Lock()
	p.failFeatRead = true
	p.mu.Unlock()
}

// ArmFeatureWrite fails the next INSERT/UPDATE of channel_feature.
func (p *participantFaultProxy) ArmFeatureWrite() {
	p.mu.Lock()
	p.failFeatWrite = true
	p.mu.Unlock()
}

func (p *participantFaultProxy) Fails() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fails
}

func (p *participantFaultProxy) LogText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.lines, "\n")
}

func (p *participantFaultProxy) note(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	p.mu.Lock()
	p.lines = append(p.lines, line)
	if len(p.lines) > 300 {
		p.lines = p.lines[len(p.lines)-300:]
	}
	p.mu.Unlock()
}

func (p *participantFaultProxy) accept() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		go p.serve(c)
	}
}

func (p *participantFaultProxy) serve(client net.Conn) {
	defer client.Close()
	server, err := net.DialTimeout("tcp", p.upstream, 5*time.Second)
	if err != nil {
		p.note("dial upstream: %v", err)
		return
	}
	defer server.Close()
	deadline := func() {
		when := time.Now().Add(20 * time.Second)
		_ = client.SetDeadline(when)
		_ = server.SetDeadline(when)
	}
	deadline()
	if err = bridgeAuth(client, server); err != nil {
		p.note("auth: %v", err)
		return
	}
	st := map[uint32]stmtMeta{}
	for {
		deadline()
		seq, payload, err := readMySQLPacket(client)
		if err != nil {
			return
		}
		if len(payload) == 0 {
			return
		}
		switch payload[0] {
		case cmdQuit:
			_ = writeMySQLPacket(server, seq, payload)
			return
		case cmdStmtClose, cmdStmtSendLong:
			if payload[0] == cmdStmtClose && len(payload) >= 5 {
				delete(st, binary.LittleEndian.Uint32(payload[1:5]))
			}
			if err = writeMySQLPacket(server, seq, payload); err != nil {
				return
			}
		case cmdQuery:
			sqlText := string(payload[1:])
			if p.failQuery(sqlText) {
				if err = writeMySQLPacket(client, seq+1, mysqlErrorPacket(1105, "channel_feature read failed")); err != nil {
					return
				}
				continue
			}
			if err = writeMySQLPacket(server, seq, payload); err != nil {
				return
			}
			if err = forwardResult(server, client); err != nil {
				p.note("query result: %v", err)
				return
			}
		case cmdStmtPrepare:
			sqlText := string(payload[1:])
			if err = writeMySQLPacket(server, seq, payload); err != nil {
				return
			}
			id, params, cols, perr := forwardPrepare(server, client)
			if perr != nil {
				p.note("prepare: %v", perr)
				return
			}
			meta := stmtMeta{
				params:       params,
				participant:  isParticipantSQL(sqlText),
				featureRead:  isFeatureReadSQL(sqlText),
				featureWrite: isFeatureWriteSQL(sqlText),
			}
			st[id] = meta
			if meta.featureRead || meta.featureWrite {
				p.note("prepare feature stmt=%d read=%v write=%v", id, meta.featureRead, meta.featureWrite)
			}
			_ = cols
		case cmdStmtExecute:
			if len(payload) < 5 {
				return
			}
			id := binary.LittleEndian.Uint32(payload[1:5])
			meta := st[id]
			if p.failStatement(payload, meta) {
				if err = writeMySQLPacket(client, seq+1, mysqlErrorPacket(1105, "participant read failed")); err != nil {
					return
				}
				continue
			}
			if err = writeMySQLPacket(server, seq, payload); err != nil {
				return
			}
			if err = forwardResult(server, client); err != nil {
				p.note("execute result: %v", err)
				return
			}
		default:
			if err = writeMySQLPacket(server, seq, payload); err != nil {
				return
			}
			if err = forwardResult(server, client); err != nil {
				p.note("result cmd=%d: %v", payload[0], err)
				return
			}
		}
	}
}

type stmtMeta struct {
	params       int
	participant  bool
	featureRead  bool
	featureWrite bool
}

func truncateSQL(q string) string {
	q = strings.Join(strings.Fields(q), " ")
	if len(q) > 160 {
		return q[:160]
	}
	return q
}

func isFeatureReadSQL(q string) bool {
	s := strings.ToLower(q)
	return strings.Contains(s, "from channel_feature") && strings.Contains(s, "select")
}

func isFeatureWriteSQL(q string) bool {
	s := strings.ToLower(q)
	return strings.Contains(s, "channel_feature") && (strings.Contains(s, "insert") || strings.Contains(s, "update"))
}

func (p *participantFaultProxy) failQuery(sqlText string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failFeatRead && isFeatureReadSQL(sqlText) {
		p.fails++
		p.lines = append(p.lines, "fail channel_feature query")
		return true
	}
	if p.failFeatWrite && isFeatureWriteSQL(sqlText) {
		p.failFeatWrite = false
		p.fails++
		p.lines = append(p.lines, "fail channel_feature query write")
		return true
	}
	return false
}

func (p *participantFaultProxy) failStatement(payload []byte, meta stmtMeta) bool {
	if meta.featureRead || meta.featureWrite {
		p.mu.Lock()
		defer p.mu.Unlock()
		if meta.featureRead && p.failFeatRead {
			p.fails++
			p.lines = append(p.lines, "fail channel_feature read")
			return true
		}
		if meta.featureWrite && p.failFeatWrite {
			p.failFeatWrite = false
			p.fails++
			p.lines = append(p.lines, "fail channel_feature write")
			return true
		}
		return false
	}
	if !meta.participant {
		return false
	}
	return p.failThis(payload, meta)
}

func (p *participantFaultProxy) failThis(payload []byte, meta stmtMeta) bool {
	user, ok := executeUserID(payload, meta.params)
	if !ok {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.armedUser == 0 || user != p.armedUser {
		return false
	}
	if p.allow > 0 {
		p.allow--
		p.lines = append(p.lines, fmt.Sprintf("allow participant user=%d remaining=%d", user, p.allow))
		return false
	}
	p.fails++
	p.lines = append(p.lines, fmt.Sprintf("fail participant user=%d", user))
	return true
}

func isParticipantSQL(q string) bool {
	s := strings.ToLower(strings.TrimSpace(q))
	return strings.Contains(s, "from channel_participants") && strings.Contains(s, "user_id=?") && !strings.Contains(s, "order by")
}

// executeUserID reads the second BIGINT of a COM_STMT_EXECUTE packet.
// GetParticipant binds channel_id then user_id.
func executeUserID(payload []byte, params int) (int64, bool) {
	if params != 2 || len(payload) < 11 || payload[0] != cmdStmtExecute {
		return 0, false
	}
	pos := 10 // cmd, stmt id, flags, iteration count
	maskLen := (params + 7) / 8
	if len(payload) < pos+maskLen+1 {
		return 0, false
	}
	pos += maskLen
	newBound := payload[pos] == 1
	pos++
	if newBound {
		if len(payload) < pos+params*2 {
			return 0, false
		}
		pos += params * 2
	}
	if len(payload) < pos+16 {
		return 0, false
	}
	return int64(binary.LittleEndian.Uint64(payload[pos+8 : pos+16])), true
}

func bridgeAuth(client, server net.Conn) error {
	if err := forwardOne(server, client); err != nil {
		return err
	}
	if err := forwardOne(client, server); err != nil {
		return err
	}
	for {
		seq, pkt, err := readMySQLPacket(server)
		if err != nil {
			return err
		}
		if err = writeMySQLPacket(client, seq, pkt); err != nil {
			return err
		}
		if len(pkt) == 0 {
			return fmt.Errorf("empty auth packet")
		}
		switch pkt[0] {
		case mysqlOK:
			return nil
		case mysqlErr:
			return fmt.Errorf("mysql auth refused")
		case mysqlEOF:
			if err = forwardOne(client, server); err != nil {
				return err
			}
		case mysqlAuthMore:
			if len(pkt) == 2 && pkt[1] == cachingFullAuth {
				if err = forwardOne(client, server); err != nil {
					return err
				}
				if err = forwardOne(server, client); err != nil {
					return err
				}
				if err = forwardOne(client, server); err != nil {
					return err
				}
				continue
			}
			if len(pkt) == 2 && pkt[1] == cachingFastAuth {
				continue
			}
			return fmt.Errorf("auth more len=%d", len(pkt))
		default:
			return fmt.Errorf("auth status %d", pkt[0])
		}
	}
}

func forwardPrepare(server, client net.Conn) (uint32, int, int, error) {
	seq, pkt, err := readMySQLPacket(server)
	if err != nil {
		return 0, 0, 0, err
	}
	if err = writeMySQLPacket(client, seq, pkt); err != nil {
		return 0, 0, 0, err
	}
	if len(pkt) < 9 || pkt[0] != mysqlOK {
		return 0, 0, 0, fmt.Errorf("prepare status %v", pkt[:min(1, len(pkt))])
	}
	id := binary.LittleEndian.Uint32(pkt[1:5])
	cols := int(binary.LittleEndian.Uint16(pkt[5:7]))
	params := int(binary.LittleEndian.Uint16(pkt[7:9]))
	if params > 0 {
		if err = forwardUntilEOF(server, client); err != nil {
			return 0, 0, 0, err
		}
	}
	if cols > 0 {
		if err = forwardUntilEOF(server, client); err != nil {
			return 0, 0, 0, err
		}
	}
	return id, params, cols, nil
}

func forwardResult(server, client net.Conn) error {
	seq, pkt, err := readMySQLPacket(server)
	if err != nil {
		return err
	}
	if err = writeMySQLPacket(client, seq, pkt); err != nil {
		return err
	}
	if len(pkt) == 0 {
		return nil
	}
	switch pkt[0] {
	case mysqlOK, mysqlErr:
		return nil
	case mysqlEOF:
		if len(pkt) < 9 {
			return nil
		}
	case 0xfb:
		return fmt.Errorf("local infile is not proxied")
	}
	if err = forwardUntilEOF(server, client); err != nil {
		return err
	}
	return forwardUntilEOF(server, client)
}

func forwardUntilEOF(server, client net.Conn) error {
	for i := 0; i < 100000; i++ {
		seq, pkt, err := readMySQLPacket(server)
		if err != nil {
			return err
		}
		if err = writeMySQLPacket(client, seq, pkt); err != nil {
			return err
		}
		if len(pkt) > 0 && pkt[0] == mysqlEOF && len(pkt) < 9 {
			return nil
		}
		if len(pkt) > 0 && pkt[0] == mysqlErr {
			return nil
		}
	}
	return fmt.Errorf("result did not end")
}

func forwardOne(from, to net.Conn) error {
	seq, pkt, err := readMySQLPacket(from)
	if err != nil {
		return err
	}
	return writeMySQLPacket(to, seq, pkt)
}

func readMySQLPacket(r io.Reader) (byte, []byte, error) {
	var full []byte
	var seq byte
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return 0, nil, err
		}
		n := int(hdr[0]) | int(hdr[1])<<8 | int(hdr[2])<<16
		if len(full) == 0 {
			seq = hdr[3]
		}
		chunk := make([]byte, n)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return 0, nil, err
		}
		full = append(full, chunk...)
		if n < 0xffffff {
			return seq, full, nil
		}
	}
}

func writeMySQLPacket(w io.Writer, seq byte, payload []byte) error {
	if len(payload) >= 0xffffff {
		return fmt.Errorf("packet too large")
	}
	hdr := []byte{byte(len(payload)), byte(len(payload) >> 8), byte(len(payload) >> 16), seq}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func mysqlErrorPacket(code uint16, msg string) []byte {
	out := []byte{mysqlErr, byte(code), byte(code >> 8), '#', 'H', 'Y', '0', '0', '0'}
	return append(out, msg...)
}

// SelfCheck proves the proxy forwards a normal read and then fails the
// second GetParticipant execute for the armed user. The row is removed.
func (p *participantFaultProxy) SelfCheck(dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err = db.Ping(); err != nil {
		return fmt.Errorf("ping through proxy: %w", err)
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS channel_participants (
  channel_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role INT NOT NULL DEFAULT 0,
  state INT NOT NULL DEFAULT 0,
  admin_rights INT NOT NULL DEFAULT 0,
  banned_rights BIGINT NOT NULL DEFAULT 0,
  rank_name VARCHAR(16) NOT NULL DEFAULT '',
  inviter_user_id BIGINT NOT NULL DEFAULT 0,
  is_bot TINYINT NOT NULL DEFAULT 0,
  date2 INT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	const ch, user int64 = 910199, 910199001
	if _, err = db.Exec(`DELETE FROM channel_participants WHERE channel_id=? AND user_id=?`, ch, user); err != nil {
		return err
	}
	if _, err = db.Exec(`INSERT INTO channel_participants (channel_id, user_id, role, state, date2) VALUES (?,?,0,0,1)`, ch, user); err != nil {
		return err
	}
	defer db.Exec(`DELETE FROM channel_participants WHERE channel_id=? AND user_id=?`, ch, user)
	p.Arm(user, 1)
	defer p.Disarm()
	var got int64
	if err = db.QueryRow(participantSelect, ch, user).Scan(&got, new(int64), new(int), new(int), new(int), new(int64), new(string), new(int64), new(int), new(int)); err != nil {
		return fmt.Errorf("first participant read: %w\n%s", err, p.LogText())
	}
	err = db.QueryRow(participantSelect, ch, user).Scan(&got, new(int64), new(int), new(int), new(int), new(int64), new(string), new(int64), new(int), new(int))
	if err == nil || !strings.Contains(err.Error(), "participant read failed") {
		return fmt.Errorf("second participant read = %v, want the injected error\n%s", err, p.LogText())
	}
	return nil
}
