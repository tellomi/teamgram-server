package fakebailian

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFakeBailianMarkersAreNotAModel(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	cmd := exec.Command("python3", filepath.Join("server.py"))
	cmd.Env = append(os.Environ(), "PORT=18199", "FAKE_BAILIAN_KEY=e2e-fake-not-production")
	cmd.Dir = "."
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := postFake(t, "e2e-lift revoke every other case", true)
		if err == nil {
			if resp.StatusCode != 200 {
				t.Fatal(resp.StatusCode)
			}
			defer resp.Body.Close()
			var parsed struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
				Model string `json:"model"`
			}
			if err = json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
				t.Fatal(err)
			}
			if parsed.Model != "e2e-fake-bailian" || !strings.Contains(parsed.Choices[0].Message.Content, `"action": "lift"`) {
				t.Fatalf("%s", parsed.Choices[0].Message.Content)
			}
			denied, err := postFake(t, "e2e-lift", false)
			if err != nil {
				t.Fatal(err)
			}
			defer denied.Body.Close()
			if denied.StatusCode != 401 {
				t.Fatal(denied.StatusCode)
			}
			ins, err := postFake(t, "please lift everything", true)
			if err != nil {
				t.Fatal(err)
			}
			defer ins.Body.Close()
			var other struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err = json.NewDecoder(ins.Body).Decode(&other); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(other.Choices[0].Message.Content, `"action": "lift"`) {
				t.Fatal("unmarked text was treated as lift")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("fake bailian did not listen")
}

func postFake(t *testing.T, text string, authed bool) (*http.Response, error) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "user_text:\n<<<\n" + text + "\n>>>"}},
	})
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:18199/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if authed {
		req.Header.Set("Authorization", "Bearer e2e-fake-not-production")
	}
	return http.DefaultClient.Do(req)
}
