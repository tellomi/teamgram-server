// Package bootstrap retries connections that panic or fail while Kafka,
// MySQL, Redis, or etcd are still starting.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/IBM/sarama"
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	defaultAttempts = 30
	maxBackoff      = 30 * time.Second
)

var (
	attempts = defaultAttempts
	sleepFn  = time.Sleep
	dialFn   = net.DialTimeout
)

// SetRetryForTest replaces the attempt count and sleep used by startup
// retries. The returned function restores the previous hooks.
func SetRetryForTest(n int, sleep func(time.Duration), dial func(string, string, time.Duration) (net.Conn, error)) func() {
	oldN, oldSleep, oldDial := attempts, sleepFn, dialFn
	if n > 0 {
		attempts = n
	}
	if sleep != nil {
		sleepFn = sleep
	}
	if dial != nil {
		dialFn = dial
	}
	return func() {
		attempts, sleepFn, dialFn = oldN, oldSleep, oldDial
	}
}

// MustKafkaProducer dials Kafka until the broker accepts the client.
// It does not give up while Kafka is down.
func MustKafkaProducer(c *kafka.KafkaProducerConf) *kafka.Producer {
	if c == nil {
		logx.Must(fmt.Errorf("kafka producer config is nil"))
	}
	return mustForever("kafka producer "+strings.Join(c.Brokers, ","), func() (p *kafka.Producer, err error) {
		defer recoverErr(&err)
		p = kafka.MustKafkaProducer(c)
		return p, nil
	})
}

// GetCachedMQClient is MustKafkaProducer with one producer per topic.
func GetCachedMQClient(c *kafka.KafkaProducerConf) *kafka.Producer {
	return MustKafkaProducer(c)
}

// MustKafkaConsumer joins a consumer group, backing off while Kafka is down.
// It does not give up while Kafka is down.
func MustKafkaConsumer(c *kafka.KafkaConsumerConf) *kafka.ConsumerGroup {
	if c == nil {
		logx.Must(fmt.Errorf("kafka consumer config is nil"))
	}
	return mustForever("kafka consumer "+strings.Join(c.Brokers, ","), func() (g *kafka.ConsumerGroup, err error) {
		defer recoverErr(&err)
		g = kafka.MustKafkaConsumer(c)
		return g, nil
	})
}

// KeepConsuming runs the consumer until the group is closed. A broker outage
// backs off and tries again instead of exiting the process.
func KeepConsuming(name string, g *kafka.ConsumerGroup) {
	if g == nil {
		return
	}
	delay := time.Second
	ctx := context.Background()
	for {
		err := g.Consume(ctx, g.Topics(), g)
		if err == nil {
			delay = time.Second
			continue
		}
		if errors.Is(err, sarama.ErrClosedConsumerGroup) || errors.Is(err, context.Canceled) {
			logx.Infof("%s: kafka consumer stopped: %v", name, err)
			return
		}
		logx.Errorf("%s: kafka consume failed: %v; retry in %s", name, err, delay)
		sleepFn(delay)
		delay = nextDelay(delay)
	}
}

// Retry calls fn until it succeeds or ctx is cancelled. The wait doubles from
// one second and stays at 30 seconds. Use this for publishes while Kafka is down.
func Retry(ctx context.Context, name string, fn func() error) error {
	delay := time.Second
	var last error
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			if last == nil {
				return err
			}
			return fmt.Errorf("%s: %w", name, last)
		}
		err := fn()
		if err == nil {
			if attempt > 1 {
				logx.Infof("%s recovered", name)
			}
			return nil
		}
		last = err
		logx.Errorf("%s not ready (attempt %d): %v; retry in %s", name, attempt, err, delay)
		sleepFn(delay)
		delay = nextDelay(delay)
	}
}

// MustMySQL opens MySQL, backing off while the server refuses connections.
func MustMySQL(c *sqlx.Config) *sqlx.DB {
	return mustValue("mysql", func() (db *sqlx.DB, err error) {
		defer recoverErr(&err)
		db = sqlx.NewMySQL(c)
		return db, nil
	})
}

// MustRedis pings Redis, backing off while it is down.
func MustRedis(c redis.RedisConf) *redis.Redis {
	return mustValue("redis "+c.Host, func() (r *redis.Redis, err error) {
		defer recoverErr(&err)
		r = redis.MustNewRedis(c)
		return r, nil
	})
}

// Deps are the addresses msg, sync, and bff need before they serve.
type Deps struct {
	Kafka []string
	MySQL []string
	Redis []string
	Etcd  []string
}

// Wait dials every dependency. Kafka is retried until it accepts a connection.
// MySQL, Redis, and etcd stop after the attempt budget.
func Wait(name string, d Deps) {
	var addrs []string
	addrs = append(addrs, d.Kafka...)
	for _, dsn := range d.MySQL {
		if addr := MySQLAddr(dsn); addr != "" {
			addrs = append(addrs, addr)
		}
	}
	addrs = append(addrs, d.Redis...)
	addrs = append(addrs, d.Etcd...)
	seen := map[string]struct{}{}
	var kafkaAddrs []string
	for _, addr := range d.Kafka {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		kafkaAddrs = append(kafkaAddrs, addr)
	}
	for _, addr := range kafkaAddrs {
		waitKafka(name, addr)
	}
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		if err := waitAddr(name, addr); err != nil {
			logx.Must(err)
		}
	}
}

// ProducerBrokers collects broker addresses from producer configs.
func ProducerBrokers(confs ...*kafka.KafkaProducerConf) []string {
	var out []string
	for _, c := range confs {
		if c != nil {
			out = append(out, c.Brokers...)
		}
	}
	return out
}

// RedisHosts collects cache/kv node addresses.
func RedisHosts(nodes ...cache.ClusterConf) []string {
	var out []string
	for _, conf := range nodes {
		for _, node := range conf {
			if node.Host != "" {
				out = append(out, node.Host)
			}
		}
	}
	return out
}

// MySQLAddr returns host:port from a go-sql-driver DSN.
func MySQLAddr(dsn string) string {
	start := strings.Index(dsn, "tcp(")
	if start < 0 {
		return ""
	}
	rest := dsn[start+4:]
	end := strings.IndexByte(rest, ')')
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

func nextDelay(d time.Duration) time.Duration {
	if d < time.Second {
		return time.Second
	}
	d *= 2
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func waitKafka(name, addr string) {
	delay := time.Second
	for i := 1; ; i++ {
		conn, err := dialFn("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			if i > 1 {
				logx.Infof("%s: %s is accepting connections", name, addr)
			}
			return
		}
		logx.Errorf("%s: %s not ready (attempt %d): %v; retry in %s", name, addr, i, err, delay)
		sleepFn(delay)
		delay = nextDelay(delay)
	}
}

func waitAddr(name, addr string) error {
	delay := time.Second
	var last error
	for i := 1; i <= attempts; i++ {
		conn, err := dialFn("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			if i > 1 {
				logx.Infof("%s: %s is accepting connections", name, addr)
			}
			return nil
		}
		last = err
		logx.Errorf("%s: %s not ready (attempt %d/%d): %v", name, addr, i, attempts, err)
		if i < attempts {
			sleepFn(delay)
			if delay < 5*time.Second {
				delay += time.Second
			}
		}
	}
	return fmt.Errorf("%s: %s unavailable after %d attempts: %w", name, addr, attempts, last)
}

func mustForever[T any](name string, fn func() (T, error)) T {
	delay := time.Second
	for i := 1; ; i++ {
		v, err := fn()
		if err == nil {
			if i > 1 {
				logx.Infof("%s is ready", name)
			}
			return v
		}
		logx.Errorf("%s not ready (attempt %d): %v; retry in %s", name, i, err, delay)
		sleepFn(delay)
		delay = nextDelay(delay)
	}
}

func mustValue[T any](name string, fn func() (T, error)) T {
	v, err := retryValue(name, fn)
	if err != nil {
		logx.Must(err)
	}
	return v
}

func retryValue[T any](name string, fn func() (T, error)) (T, error) {
	var zero T
	delay := time.Second
	var last error
	for i := 1; i <= attempts; i++ {
		v, err := fn()
		if err == nil {
			if i > 1 {
				logx.Infof("%s is ready", name)
			}
			return v, nil
		}
		last = err
		logx.Errorf("%s not ready (attempt %d/%d): %v", name, i, attempts, err)
		if i < attempts {
			sleepFn(delay)
			if delay < 5*time.Second {
				delay += time.Second
			}
		}
	}
	return zero, fmt.Errorf("%s unavailable after %d attempts: %w", name, attempts, last)
}

func recoverErr(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("%v", r)
	}
}
