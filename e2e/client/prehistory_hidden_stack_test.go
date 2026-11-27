//go:build reliability

package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestHiddenPrehistoryProtocolStack starts one isolated MySQL and the
// teamgram processes. bff's MySQL DSN goes through a fault proxy before
// any process starts, so session does not keep a stale bff address.
// The test name does not match TestReliability, so that job does not run it.
func TestHiddenPrehistoryProtocolStack(t *testing.T) {
	var proxy *participantFaultProxy
	beforeReliabilityProcesses = func(st *reliabilityStack) error {
		var err error
		proxy, err = startParticipantFaultProxy(fmt.Sprintf("127.0.0.1:%d", st.mysqlPort))
		if err != nil {
			return err
		}
		proxyDSN := fmt.Sprintf("teamgram:teamgram@tcp(%s)/teamgram?parseTime=true&charset=utf8mb4&timeout=5s", proxy.Addr())
		if err = proxy.SelfCheck(proxyDSN); err != nil {
			return err
		}
		path := st.dir + "/etc2/bff.yaml"
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		from := fmt.Sprintf("@tcp(127.0.0.1:%d)", st.mysqlPort)
		to := fmt.Sprintf("@tcp(%s)", proxy.Addr())
		next := strings.ReplaceAll(string(raw), from, to)
		if next == string(raw) {
			return fmt.Errorf("bff.yaml has no %s", from)
		}
		return os.WriteFile(path, []byte(next), 0o644)
	}
	t.Cleanup(func() {
		beforeReliabilityProcesses = nil
		if proxy != nil {
			proxy.Close()
		}
		prehistoryFault = nil
	})

	st := useReliabilityStack(t)
	if proxy == nil {
		t.Fatal("fault proxy was not installed")
	}
	prehistoryFault = proxy
	t.Setenv("TELLOMI_E2E", "1")
	t.Setenv("TELLOMI_E2E_ADDR", st.cfg.Addr)
	t.Setenv("TELLOMI_E2E_MYSQL_DSN", fmt.Sprintf("teamgram:teamgram@tcp(127.0.0.1:%d)/teamgram?parseTime=true&charset=utf8mb4&timeout=5s", st.mysqlPort))
	TestHiddenPrehistoryProtocol(t)
}
