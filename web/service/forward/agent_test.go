package forward

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/YCJE/XPanel/database"
	"github.com/YCJE/XPanel/database/model"
	"github.com/YCJE/XPanel/logger"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"github.com/op/go-logging"
)

func setupTestEnv(t *testing.T) (*Service, *model.ForwardNode) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	logger.InitLogger(logging.ERROR)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		GlobalHub.mu.Lock()
		for id, c := range GlobalHub.conns {
			c.close()
			delete(GlobalHub.conns, id)
		}
		GlobalHub.mu.Unlock()
		_ = database.CloseDB()
	})
	svc := &Service{}
	node, err := svc.AddNode("测试中转", "10.0.0.2", "", 0, 0)
	if err != nil {
		t.Fatalf("add node: %v", err)
	}
	return svc, node
}

func newTestRouter(svc *Service) *gin.Engine {
	r := gin.New()
	r.GET("/system-info", svc.AgentWS)
	r.POST("/flow/upload", svc.FlowUpload)
	r.POST("/flow/config", svc.FlowConfig)
	return r
}

func dialAgent(t *testing.T, url string, secret string) *ws.Conn {
	t.Helper()
	u := "ws" + url[4:] + "/system-info?type=1&secret=" + secret + "&version=test&http=0&tls=0&socks=0"
	conn, _, err := ws.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	return conn
}

// TestAgentLifecycle walks the full agent protocol: handshake → heartbeat ack
// → command round-trip → traffic accounting.
func TestAgentLifecycle(t *testing.T) {
	svc, node := setupTestEnv(t)
	r := newTestRouter(svc)
	server := httptest.NewServer(r)
	defer server.Close()

	// Bad secret must be rejected before upgrade.
	resp, err := http.Get(server.URL + "/system-info?type=1&secret=wrong")
	if err != nil {
		t.Fatalf("bad secret probe: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for bad secret, got %d", resp.StatusCode)
	}

	conn := dialAgent(t, server.URL, node.Secret)
	defer conn.Close()

	// Wait for the panel to register the connection.
	deadline := time.Now().Add(5 * time.Second)
	for !GlobalHub.IsOnline(node.Id) {
		if time.Now().After(deadline) {
			t.Fatal("agent never came online")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Heartbeat: agent sends system info, panel must ack {"type":"call"}.
	info, _ := json.Marshal(map[string]any{
		"uptime": 100, "cpu_usage": 12.5, "memory_usage": 40.0,
		"bytes_received": 1, "bytes_transmitted": 2,
	})
	wrapper, _ := mustEncryptForTest(node.Secret, info)
	if err := conn.WriteMessage(ws.TextMessage, wrapper); err != nil {
		t.Fatalf("send info: %v", err)
	}
	_, ack, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if string(ack) != `{"type":"call"}` {
		t.Fatalf("unexpected ack: %s", ack)
	}

	// Fake agent loop: read every panel command and reply success.
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var payload struct {
				Encrypted bool   `json:"encrypted"`
				Data      string `json:"data"`
			}
			if json.Unmarshal(msg, &payload) != nil || !payload.Encrypted {
				return
			}
			plain, err := decryptForTest(node.Secret, payload.Data)
			if err != nil {
				return
			}
			var cmd CommandMessage
			if json.Unmarshal(plain, &cmd) != nil {
				return
			}
			respData, _ := json.Marshal(CommandResponse{
				Type: "AddServiceResponse", Success: true, Message: "OK", RequestId: cmd.RequestId,
			})
			out, _ := mustEncryptForTest(node.Secret, respData)
			conn.WriteMessage(ws.TextMessage, out)
		}
	}()

	services, err := ForwardServices(node, &model.ForwardRule{
		Id: 1, NodeId: node.Id, Name: "测试", InPort: 12345, Target: "10.0.0.3:443",
	})
	if err != nil {
		t.Fatalf("build services: %v", err)
	}
	cmdResp, err := GlobalHub.SendCommand(node.Id, "AddService", services, 10*time.Second)
	if err != nil {
		t.Fatalf("SendCommand: %v", err)
	}
	if !cmdResp.Success {
		t.Fatalf("command failed: %s", cmdResp.Message)
	}

	// Traffic upload: encrypted {"n":"pf1_tcp","u":100,"d":200}.
	rule := &model.ForwardRule{NodeId: node.Id, Name: "测试", InPort: 12345, Target: "10.0.0.3:443", Ratio: 2, Enable: true}
	if err := svc.AddForwardRule(rule); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	flow, _ := json.Marshal(flowItem{N: "pf" + itoa(rule.Id) + "_tcp", U: 100, D: 200})
	body, _ := mustEncryptForTest(node.Secret, flow)
	upload, err := http.Post(server.URL+"/flow/upload?secret="+node.Secret, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	upload.Body.Close()

	rules, err := svc.GetForwardRules()
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	// Ratio 2 applied: up 200, down 400, allTime 600.
	if rules[0].Up != 200 || rules[0].Down != 400 || rules[0].AllTime != 600 {
		t.Fatalf("traffic accounting wrong: up=%d down=%d allTime=%d", rules[0].Up, rules[0].Down, rules[0].AllTime)
	}
}

// mustEncryptForTest / decryptForTest wrap the AES wire format for the fake agent.
func mustEncryptForTest(secret string, data []byte) ([]byte, error) {
	crypto, err := NewAESCrypto(secret)
	if err != nil {
		return nil, err
	}
	enc, err := crypto.Encrypt(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"encrypted": true, "data": enc, "timestamp": time.Now().Unix()})
}

func decryptForTest(secret, data string) ([]byte, error) {
	crypto, err := NewAESCrypto(secret)
	if err != nil {
		return nil, err
	}
	return crypto.Decrypt(data)
}

func itoa(v int) string {
	return json.Number(fmtInt(v)).String()
}

func fmtInt(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
