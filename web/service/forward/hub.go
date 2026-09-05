package forward

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// CommandMessage is a panel→agent command (identical to the agent protocol).
type CommandMessage struct {
	Type      string `json:"type"`
	Data      any    `json:"data"`
	RequestId string `json:"requestId,omitempty"`
}

// CommandResponse is an agent→panel reply for a command.
type CommandResponse struct {
	Type      string          `json:"type"`
	Success   bool            `json:"success"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	RequestId string          `json:"requestId,omitempty"`
}

// agentConn is one live agent (node) WebSocket session.
type agentConn struct {
	nodeId  int
	secret  string
	conn    *websocket.Conn
	writeMu sync.Mutex

	pendMu  sync.Mutex
	pending map[string]chan *CommandResponse

	done     chan struct{}
	doneOnce sync.Once
}

// Hub tracks every connected agent by node id.
type Hub struct {
	mu    sync.RWMutex
	conns map[int]*agentConn
}

// GlobalHub is the process-wide agent registry.
var GlobalHub = &Hub{conns: make(map[int]*agentConn)}

// IsOnline reports whether the node currently has a live agent session.
func (h *Hub) IsOnline(nodeId int) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.conns[nodeId]
	return ok && c != nil
}

func (h *Hub) add(c *agentConn) {
	h.mu.Lock()
	old := h.conns[c.nodeId]
	h.conns[c.nodeId] = c
	h.mu.Unlock()
	if old != nil {
		old.close()
	}
}

func (h *Hub) remove(nodeId int, c *agentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.conns[nodeId]; ok && cur == c {
		delete(h.conns, nodeId)
	}
}

// close tears down the connection. Pending command channels are never closed
// (a concurrent dispatch would panic on send-to-closed); waiters observe the
// done channel instead and fail fast.
// CloseNode drops the live agent session of a node, if any. Used when the
// node is deleted so stale agents cannot keep reporting.
func (h *Hub) CloseNode(nodeId int) {
	h.mu.Lock()
	c := h.conns[nodeId]
	delete(h.conns, nodeId)
	h.mu.Unlock()
	if c != nil {
		c.close()
	}
}

func (c *agentConn) close() {
	c.doneOnce.Do(func() { close(c.done) })
	c.pendMu.Lock()
	c.pending = make(map[string]chan *CommandResponse)
	c.pendMu.Unlock()
	c.conn.Close()
}

// dispatch routes a command response to its waiter, if still pending.
func (c *agentConn) dispatch(resp *CommandResponse) {
	c.pendMu.Lock()
	ch, ok := c.pending[resp.RequestId]
	c.pendMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- resp:
	default:
	}
}

// wrapMessage AES-encrypts a payload when a secret is present.
func (c *agentConn) wrapMessage(payload []byte) ([]byte, error) {
	crypto, err := NewAESCrypto(c.secret)
	if err != nil {
		return payload, nil
	}
	encrypted, err := crypto.Encrypt(payload)
	if err != nil {
		return payload, nil
	}
	return mustJSON(map[string]any{
		"encrypted": true,
		"data":      encrypted,
		"timestamp": time.Now().Unix(),
	}), nil
}

// unwrapMessage decrypts an agent payload when needed.
func unwrapMessage(secret string, message []byte) ([]byte, error) {
	var wrapper struct {
		Encrypted bool   `json:"encrypted"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(message, &wrapper); err != nil {
		return message, nil
	}
	if !wrapper.Encrypted || wrapper.Data == "" {
		return message, nil
	}
	crypto, err := NewAESCrypto(secret)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return crypto.Decrypt(wrapper.Data)
}

// SendCommand delivers a command to the agent and awaits its response.
func (h *Hub) SendCommand(nodeId int, cmdType string, data any, timeout time.Duration) (*CommandResponse, error) {
	h.mu.RLock()
	c := h.conns[nodeId]
	h.mu.RUnlock()
	if c == nil {
		return nil, fmt.Errorf("节点不在线")
	}

	requestId := newRequestId()
	ch := make(chan *CommandResponse, 1)
	c.pendMu.Lock()
	c.pending[requestId] = ch
	c.pendMu.Unlock()
	defer func() {
		c.pendMu.Lock()
		delete(c.pending, requestId)
		c.pendMu.Unlock()
	}()

	payload := mustJSON(CommandMessage{Type: cmdType, Data: data, RequestId: requestId})
	wire, err := c.wrapMessage(payload)
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	err = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err == nil {
		err = c.conn.WriteMessage(websocket.TextMessage, wire)
	}
	c.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("发送命令失败: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("连接已断开")
		}
		return resp, nil
	case <-c.done:
		return nil, fmt.Errorf("连接已断开")
	case <-time.After(timeout):
		return nil, fmt.Errorf("等待节点响应超时")
	}
}

// readLoop pumps agent messages until the connection dies. Agent system-info
// payloads are acknowledged with {"type":"call"} exactly like flux-panel does,
// and command responses are routed to their waiters.
func (h *Hub) readLoop(c *agentConn, onSystemInfo func(nodeId int, payload []byte)) {
	defer func() {
		h.remove(c.nodeId, c)
		c.close()
	}()
	for {
		c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			continue
		}
		payload, err := unwrapMessage(c.secret, message)
		if err != nil {
			continue
		}

		// Command response?
		if strings.Contains(string(payload), "requestId") {
			var resp CommandResponse
			if json.Unmarshal(payload, &resp) == nil && resp.RequestId != "" {
				c.dispatch(&resp)
				continue
			}
		}

		// System info heartbeat?
		if strings.Contains(string(payload), "memory_usage") {
			c.writeMu.Lock()
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			c.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"call"}`))
			c.writeMu.Unlock()
			if onSystemInfo != nil {
				onSystemInfo(c.nodeId, payload)
			}
		}
	}
}
