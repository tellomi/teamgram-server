//go:build reliability

package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "github.com/go-sql-driver/mysql"
)

// Pinned images. Redis is the SSDB stand-in: teamgramd/etc2/dfs.yaml points
// the SSDB block at the same Redis the cache uses (Redis protocol, not the
// ssdb text server). The object store is the MinIO-compatible image already
// pinned for local R2 stand-in in docker-compose-env.yaml. Production
// compose files are not started and not modified.
const (
	imageMySQL = "mysql:8.0.40"
	imageRedis = "redis:7.4.2-alpine"
	imageEtcd  = "quay.io/coreos/etcd:v3.5.11"
	imageKafka = "apache/kafka:3.9.0"
	imageS3    = "docker.io/pgsty/silo:RELEASE.2026-10-26T00-00-00Z"

	s3User   = "minio"
	s3Secret = "miniostorage"
)

type svcSpec struct {
	name string
	pkg  string
	yaml string
	port int
}

// Order matches teamgramd/bin/runall-docker.sh, without linkpreview.
var reliabilityServices = []svcSpec{
	{"idgen", "app/service/idgen/cmd/idgen", "idgen.yaml", 20660},
	{"status", "app/service/status/cmd/status", "status.yaml", 20670},
	{"authsession", "app/service/authsession/cmd/authsession", "authsession.yaml", 20450},
	{"dfs", "app/service/dfs/cmd/dfs", "dfs.yaml", 20640},
	{"media", "app/service/media/cmd/media", "media.yaml", 20650},
	{"biz", "app/service/biz/biz/cmd/biz", "biz.yaml", 20020},
	{"msg", "app/messenger/msg/cmd/msg", "msg.yaml", 20030},
	// sync consumes Kafka and does not open its configured ListenOn port.
	{"sync", "app/messenger/sync/cmd/sync", "sync.yaml", 0},
	{"bff", "app/bff/bff/cmd/bff", "bff.yaml", 20010},
	{"session", "app/interface/session/cmd/session", "session.yaml", 20120},
	{"gnetway", "app/interface/gnetway/cmd/gnetway", "gnetway.yaml", 10443},
}

type reliabilityStack struct {
	root string
	cfg  Config
	dir  string

	mysqlPort int
	redisPort int
	etcdPort  int
	kafkaPort int
	s3Port    int

	db     *sql.DB
	s3     *minio.Client
	proxy  *s3Proxy
	conts  []testcontainers.Container
	procs  map[string]*exec.Cmd
	logs   map[string]*os.File
	procMu sync.Mutex
	closed bool
}

var (
	binOnce sync.Once
	binErr  error
)

func newReliabilityStack(t *testing.T) *reliabilityStack {
	t.Helper()
	root := reliabilityRoot(t)
	// A photo put that hits object-store retries can take several seconds
	// before the rpc_result. The regression suite keeps the shorter default.
	_ = os.Setenv("TELLOMI_E2E_ACK_RETRIES", "8")
	if err := buildReliabilityBins(root); err != nil {
		t.Fatalf("build services: %v", err)
	}
	if err := checkServicePorts(); err != nil {
		t.Fatal(err)
	}
	s := &reliabilityStack{
		root:  root,
		procs: map[string]*exec.Cmd{},
		logs:  map[string]*os.File{},
		cfg: Config{
			Addr:       "127.0.0.1:10443",
			APIID:      10840,
			APIHash:    "0123456789abcdef0123456789abcdef",
			DC:         1,
			RSAKeyFile: filepath.Join(root, "teamgramd/bin/server_pkcs1.key"),
		},
	}
	t.Cleanup(func() { s.Close(t) })
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if err := s.startDatastores(ctx); err != nil {
		t.Fatalf("datastores: %v", err)
	}
	if err := s.startProcesses(); err != nil {
		t.Fatalf("services: %v", err)
	}
	return s
}

func reliabilityRoot(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("TELLOMI_E2E_ROOT"); v != "" {
		return v
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	if _, err := os.Stat(filepath.Join(root, "teamgramd/bin/server_pkcs1.key")); err != nil {
		t.Fatalf("repo root %s: %v", root, err)
	}
	return root
}

func reliabilityPhone(suffix string) Phone {
	return Phone{Number: "999661" + suffix, Code: "11111"}
}

func buildReliabilityBins(root string) error {
	binOnce.Do(func() {
		binErr = buildReliabilityBinsOnce(root)
	})
	return binErr
}

func buildReliabilityBinsOnce(root string) error {
	if _, err := exec.LookPath("pkg-config"); err != nil {
		return fmt.Errorf("pkg-config is required to link libvips into dfs")
	}
	if err := exec.Command("pkg-config", "--exists", "vips").Run(); err != nil {
		return fmt.Errorf("libvips is required to build dfs (pkg-config vips): %w", err)
	}
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	errc := make(chan error, len(reliabilityServices))
	env := append(os.Environ(), "CGO_ENABLED=1")
	for _, svc := range reliabilityServices {
		wg.Add(1)
		go func(svc svcSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out := filepath.Join(root, "teamgramd/bin", svc.name)
			cmd := exec.Command("go", "build", "-o", out, "./"+svc.pkg)
			cmd.Dir = root
			cmd.Env = env
			buf, err := cmd.CombinedOutput()
			if err != nil {
				errc <- fmt.Errorf("%s: %w\n%s", svc.name, err, tailBytes(buf, 4000))
			}
		}(svc)
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			return err
		}
	}
	return nil
}

func checkServicePorts() error {
	var busy []string
	for _, svc := range reliabilityServices {
		if svc.port == 0 {
			continue
		}
		if err := portFree(svc.port); err != nil {
			busy = append(busy, fmt.Sprintf("%d (%s)", svc.port, svc.name))
		}
	}
	for _, port := range []int{5222, 11443} {
		if err := portFree(port); err != nil {
			busy = append(busy, strconv.Itoa(port))
		}
	}
	if len(busy) > 0 {
		return fmt.Errorf("reliability tests need these ports free: %s", strings.Join(busy, ", "))
	}
	return nil
}

func portFree(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	return ln.Close()
}

func reservePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

func (s *reliabilityStack) startDatastores(ctx context.Context) error {
	var err error
	s.dir, err = os.MkdirTemp("", "tellomi-reliability-")
	if err != nil {
		return err
	}
	ports := []*int{&s.mysqlPort, &s.redisPort, &s.etcdPort, &s.kafkaPort, &s.s3Port}
	for _, p := range ports {
		*p, err = reservePort()
		if err != nil {
			return err
		}
	}
	var wg sync.WaitGroup
	errc := make(chan error, 5)
	start := func(fn func() (testcontainers.Container, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := fn()
			if err != nil {
				errc <- err
				return
			}
			s.procMu.Lock()
			s.conts = append(s.conts, c)
			s.procMu.Unlock()
		}()
	}
	sqlDir := filepath.Join(s.root, "teamgramd/deploy/sql")
	start(func() (testcontainers.Container, error) {
		return s.run(ctx, "mysql", testcontainers.ContainerRequest{
			Image:        imageMySQL,
			ExposedPorts: []string{"3306/tcp"},
			Env: map[string]string{
				"MYSQL_ROOT_PASSWORD": "root",
				"MYSQL_DATABASE":      "teamgram",
				"MYSQL_USER":          "teamgram",
				"MYSQL_PASSWORD":      "teamgram",
			},
			Cmd: []string{
				"--character-set-server=utf8mb4",
				"--collation-server=utf8mb4_unicode_ci",
			},
			HostConfigModifier: bindPort("3306/tcp", s.mysqlPort, sqlDir+":/docker-entrypoint-initdb.d:ro"),
			WaitingFor:         wait.ForLog("ready for connections").WithOccurrence(2).WithStartupTimeout(3 * time.Minute),
		})
	})
	start(func() (testcontainers.Container, error) {
		return s.run(ctx, "redis", testcontainers.ContainerRequest{
			Image:              imageRedis,
			ExposedPorts:       []string{"6379/tcp"},
			Cmd:                []string{"redis-server", "--appendonly", "yes"},
			HostConfigModifier: bindPort("6379/tcp", s.redisPort),
			WaitingFor:         wait.ForLog("Ready to accept connections").WithStartupTimeout(time.Minute),
		})
	})
	start(func() (testcontainers.Container, error) {
		return s.run(ctx, "etcd", testcontainers.ContainerRequest{
			Image:        imageEtcd,
			ExposedPorts: []string{"2379/tcp"},
			Env: map[string]string{
				"ETCD_NAME":                        "node1",
				"ETCD_DATA_DIR":                    "/tmp/etcd-data",
				"ETCD_LISTEN_CLIENT_URLS":          "http://0.0.0.0:2379",
				"ETCD_ADVERTISE_CLIENT_URLS":       fmt.Sprintf("http://127.0.0.1:%d", s.etcdPort),
				"ETCD_LISTEN_PEER_URLS":            "http://127.0.0.1:2380",
				"ETCD_INITIAL_ADVERTISE_PEER_URLS": "http://127.0.0.1:2380",
				"ETCD_INITIAL_CLUSTER":             "node1=http://127.0.0.1:2380",
				"ETCD_INITIAL_CLUSTER_TOKEN":       "tellomi-reliability",
				"ETCD_INITIAL_CLUSTER_STATE":       "new",
			},
			HostConfigModifier: bindPort("2379/tcp", s.etcdPort),
			WaitingFor:         wait.ForListeningPort("2379/tcp").WithStartupTimeout(time.Minute),
		})
	})
	start(func() (testcontainers.Container, error) {
		return s.run(ctx, "kafka", testcontainers.ContainerRequest{
			Image:        imageKafka,
			ExposedPorts: []string{"9092/tcp"},
			Env: map[string]string{
				"KAFKA_NODE_ID":                                  "1",
				"KAFKA_PROCESS_ROLES":                            "broker,controller",
				"KAFKA_LISTENERS":                                "PLAINTEXT://0.0.0.0:9092,CONTROLLER://127.0.0.1:9093",
				"KAFKA_ADVERTISED_LISTENERS":                     fmt.Sprintf("PLAINTEXT://127.0.0.1:%d", s.kafkaPort),
				"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":           "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",
				"KAFKA_CONTROLLER_LISTENER_NAMES":                "CONTROLLER",
				"KAFKA_CONTROLLER_QUORUM_VOTERS":                 "1@127.0.0.1:9093",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
				"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
				"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
				"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":         "0",
				"KAFKA_NUM_PARTITIONS":                           "1",
				"KAFKA_AUTO_CREATE_TOPICS_ENABLE":                "true",
			},
			HostConfigModifier: bindPort("9092/tcp", s.kafkaPort),
			WaitingFor:         wait.ForLog("Kafka Server started").WithStartupTimeout(3 * time.Minute),
		})
	})
	start(func() (testcontainers.Container, error) {
		return s.run(ctx, "s3", testcontainers.ContainerRequest{
			Image:        imageS3,
			ExposedPorts: []string{"9000/tcp"},
			Env: map[string]string{
				"MINIO_ROOT_USER":     s3User,
				"MINIO_ROOT_PASSWORD": s3Secret,
			},
			Cmd:                []string{"server", "/data", "--address", "0.0.0.0:9000", "--console-address", "0.0.0.0:9001"},
			HostConfigModifier: bindPort("9000/tcp", s.s3Port),
			WaitingFor:         wait.ForListeningPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
		})
	})
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			return err
		}
	}
	dsn := fmt.Sprintf("teamgram:teamgram@tcp(127.0.0.1:%d)/teamgram?parseTime=true&charset=utf8mb4&timeout=5s", s.mysqlPort)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	s.db = db
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := waitDB(pingCtx, db); err != nil {
		return err
	}
	// stories.sendStory refuses to commit until the gated advertisement
	// tables exist. apply-gated-sql.sh is an operator step and is not part
	// of process start. This database was just created, so the test applies
	// that same file itself.
	if err := applyGatedStorySQL(pingCtx, s.mysqlPort, filepath.Join(s.root, "teamgramd/deploy/gated/20261120-story-advert.sql")); err != nil {
		return err
	}
	s.s3, err = minio.New(fmt.Sprintf("127.0.0.1:%d", s.s3Port), &minio.Options{
		Creds:        credentials.NewStaticV4(s3User, s3Secret, ""),
		Secure:       false,
		Region:       "us-east-1",
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return err
	}
	for _, bucket := range []string{"tellomi-attachments", "tellomi-public-media", "tellomi-backups"} {
		if err := makeBucket(ctx, s.s3, bucket); err != nil {
			return err
		}
	}
	s.proxy, err = newS3Proxy(fmt.Sprintf("127.0.0.1:%d", s.s3Port))
	if err != nil {
		return err
	}
	return s.writeConfigs()
}

func bindPort(port string, hostPort int, binds ...string) func(*container.HostConfig) {
	return func(hc *container.HostConfig) {
		if hc.PortBindings == nil {
			hc.PortBindings = nat.PortMap{}
		}
		hc.PortBindings[nat.Port(port)] = []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: strconv.Itoa(hostPort)}}
		hc.Binds = append(hc.Binds, binds...)
	}
}

func (s *reliabilityStack) run(ctx context.Context, name string, req testcontainers.ContainerRequest) (testcontainers.Container, error) {
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("%s (%s): %w", name, req.Image, err)
	}
	return c, nil
}

func waitDB(ctx context.Context, db *sql.DB) error {
	var last error
	for {
		last = db.PingContext(ctx)
		if last == nil {
			var n int
			last = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'teamgram' AND table_name = 'users'").Scan(&n)
			if last == nil && n == 1 {
				return nil
			}
			if last == nil {
				last = fmt.Errorf("users table is missing")
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("mysql schema: %w", last)
		case <-time.After(time.Second):
		}
	}
}

func applyGatedStorySQL(ctx context.Context, port int, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") || trim == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	dsn := fmt.Sprintf("teamgram:teamgram@tcp(127.0.0.1:%d)/teamgram?parseTime=true&charset=utf8mb4&multiStatements=true&timeout=10s", port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, b.String()); err != nil {
		return fmt.Errorf("gated story sql: %w", err)
	}
	return nil
}

func makeBucket(ctx context.Context, cl *minio.Client, bucket string) error {
	var last error
	for i := 0; i < 30; i++ {
		err := cl.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"})
		if err == nil {
			return nil
		}
		exists, okErr := cl.BucketExists(ctx, bucket)
		if okErr == nil && exists {
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return last
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("bucket %s: %w", bucket, last)
}

func (s *reliabilityStack) writeConfigs() error {
	replacer := strings.NewReplacer(
		"etcd:2379", fmt.Sprintf("127.0.0.1:%d", s.etcdPort),
		"redis:6379", fmt.Sprintf("127.0.0.1:%d", s.redisPort),
		"mysql:3306", fmt.Sprintf("127.0.0.1:%d", s.mysqlPort),
		"kafka:9092", fmt.Sprintf("127.0.0.1:%d", s.kafkaPort),
		"minio:9000", fmt.Sprintf("127.0.0.1:%d", s.proxy.Port()),
		"/app/logs", filepath.Join(s.dir, "logs"),
		"Timeout: 3000", "Timeout: 20000",
	)
	src := filepath.Join(s.root, "teamgramd/etc2")
	dst := filepath.Join(s.dir, "etc2")
	if err := os.MkdirAll(filepath.Join(s.dir, "logs"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(src, ent.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, ent.Name()), []byte(replacer.Replace(string(raw))), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *reliabilityStack) startProcesses() error {
	for _, svc := range reliabilityServices {
		if err := s.startProcess(svc); err != nil {
			return err
		}
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (s *reliabilityStack) startProcess(svc svcSpec) error {
	s.stopProcess(svc.name)
	if svc.port != 0 {
		if err := waitPortFree(svc.port, 15*time.Second); err != nil {
			return fmt.Errorf("%s port %d: %w", svc.name, svc.port, err)
		}
	}
	logPath := filepath.Join(s.dir, svc.name+".log")
	f, err := os.Create(logPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(s.root, "teamgramd/bin", svc.name), "-f="+filepath.Join(s.dir, "etc2", svc.yaml))
	cmd.Dir = filepath.Join(s.root, "teamgramd/bin")
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.Env = filteredEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return err
	}
	s.procMu.Lock()
	s.procs[svc.name] = cmd
	s.logs[svc.name] = f
	s.procMu.Unlock()
	if svc.port == 0 {
		time.Sleep(300 * time.Millisecond)
	} else if err := waitPort(svc.port, 45*time.Second); err != nil {
		return fmt.Errorf("%s did not listen on %d: %w\n%s", svc.name, svc.port, err, s.tailLog(svc.name))
	}
	if !processAlive(cmd) {
		return fmt.Errorf("%s exited\n%s", svc.name, s.tailLog(svc.name))
	}
	return nil
}

func filteredEnv() []string {
	out := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		switch {
		case strings.HasPrefix(e, "TELLOMI_S3_"),
			strings.HasPrefix(e, "TELLOMI_MINIO_"),
			strings.HasPrefix(e, "TELLOMI_OTEL_"),
			strings.HasPrefix(e, "TEAMGRAM_MYSQL_"):
			continue
		}
		out = append(out, e)
	}
	return append(out, "TELLOMI_OTEL_ENDPOINT=")
}

func (s *reliabilityStack) restartProcesses(t *testing.T) {
	t.Helper()
	s.stopProcesses()
	if err := s.startProcesses(); err != nil {
		t.Fatal(err)
	}
}

func (s *reliabilityStack) restartService(t *testing.T, name string) {
	t.Helper()
	var svc svcSpec
	for _, item := range reliabilityServices {
		if item.name == name {
			svc = item
			break
		}
	}
	if svc.name == "" {
		t.Fatalf("unknown service %s", name)
	}
	if err := s.startProcess(svc); err != nil {
		t.Fatal(err)
	}
}

func (s *reliabilityStack) stopProcesses() {
	for i := len(reliabilityServices) - 1; i >= 0; i-- {
		s.stopProcess(reliabilityServices[i].name)
	}
}

func (s *reliabilityStack) stopProcess(name string) {
	s.procMu.Lock()
	cmd := s.procs[name]
	f := s.logs[name]
	delete(s.procs, name)
	delete(s.logs, name)
	s.procMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	}
	if f != nil {
		_ = f.Close()
	}
}

func processAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	err := cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

func waitPort(port int, d time.Duration) error {
	deadline := time.Now().Add(d)
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			return conn.Close()
		}
		last = err
		time.Sleep(200 * time.Millisecond)
	}
	return last
}

func waitPortFree(port int, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if err := portFree(port); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("still in use")
}

func (s *reliabilityStack) Close(t *testing.T) {
	s.procMu.Lock()
	if s.closed {
		s.procMu.Unlock()
		return
	}
	s.closed = true
	failed := t != nil && t.Failed()
	s.procMu.Unlock()
	if failed {
		s.dumpLogs(t)
	}
	s.stopProcesses()
	if s.db != nil {
		_ = s.db.Close()
	}
	if s.proxy != nil {
		s.proxy.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.procMu.Lock()
	conts := append([]testcontainers.Container(nil), s.conts...)
	s.procMu.Unlock()
	for _, c := range conts {
		_ = c.Terminate(ctx)
	}
	if s.dir != "" && !failed {
		_ = os.RemoveAll(s.dir)
	}
}

func (s *reliabilityStack) dumpLogs(t *testing.T) {
	t.Helper()
	for _, svc := range reliabilityServices {
		text := s.tailLog(svc.name)
		if strings.TrimSpace(text) == "" {
			continue
		}
		t.Logf("---- %s log ----\n%s", svc.name, text)
	}
}

func (s *reliabilityStack) tailLog(name string) string {
	b, err := os.ReadFile(filepath.Join(s.dir, name+".log"))
	if err != nil {
		return ""
	}
	return tailBytes(b, 4000)
}

func tailBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}

func (s *reliabilityStack) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var v any
	if err := s.db.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("sql: %v", err)
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	default:
		return fmt.Sprint(x)
	}
}

func (s *reliabilityStack) execSQL(t *testing.T, query string, args ...any) {
	t.Helper()
	res, err := s.db.Exec(query, args...)
	if err != nil {
		t.Fatalf("sql: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("sql changed 0 rows")
	}
}

func (s *reliabilityStack) objectKeys(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var keys []string
	for _, bucket := range []string{"tellomi-attachments", "tellomi-public-media", "tellomi-backups"} {
		for obj := range s.s3.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if obj.Err != nil {
				t.Fatalf("list %s: %v", bucket, obj.Err)
			}
			keys = append(keys, bucket+"/"+obj.Key)
		}
	}
	return keys
}

func (s *reliabilityStack) login(t *testing.T, profile Profile, phone Phone, first string) (*Client, Self) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	var last error
	for i := 0; i < 8; i++ {
		c, err := Dial(ctx, s.cfg, profile)
		if err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		self, err := c.Login(ctx, phone, first)
		if err != nil {
			c.Close()
			last = err
			time.Sleep(time.Second)
			continue
		}
		if _, _, _, err = c.State(ctx); err != nil {
			c.Close()
			last = err
			time.Sleep(time.Second)
			continue
		}
		t.Cleanup(c.Close)
		return c, self
	}
	t.Fatalf("login %s: %v", phone.Number, last)
	return nil, Self{}
}

func callState(t *testing.T, c *Client) (pts, qts, date int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pts, qts, date, err := c.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return pts, qts, date
}
