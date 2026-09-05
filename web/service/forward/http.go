package forward

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/YCJE/XPanel/database"
	"github.com/YCJE/XPanel/database/model"
	"github.com/YCJE/XPanel/logger"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024 * 32,
	WriteBufferSize: 1024 * 32,
	// Agents authenticate via the secret query parameter.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// nodeBySecret resolves an agent token to its node record.
func nodeBySecret(secret string) *model.ForwardNode {
	if secret == "" {
		return nil
	}
	node := &model.ForwardNode{}
	if err := database.GetDB().Where("secret = ?", secret).First(node).Error; err != nil {
		return nil
	}
	return node
}

// AgentWS handles GET /system-info — the agent WebSocket channel.
func (s *Service) AgentWS(c *gin.Context) {
	secret := c.Query("secret")
	if c.Query("type") != "1" {
		c.Status(http.StatusForbidden)
		return
	}
	node := nodeBySecret(secret)
	if node == nil {
		logger.Info("agent handshake rejected: unknown secret")
		c.Status(http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	// Persist the agent capabilities reported on the query string.
	updates := map[string]any{
		"online":    true,
		"last_seen": time.Now().Unix(),
		"version":   c.Query("version"),
	}
	if v, err := strconv.Atoi(c.Query("http")); err == nil {
		updates["enable_http"] = v == 1
	}
	if v, err := strconv.Atoi(c.Query("tls")); err == nil {
		updates["enable_tls"] = v == 1
	}
	if v, err := strconv.Atoi(c.Query("socks")); err == nil {
		updates["enable_socks"] = v == 1
	}
	database.GetDB().Model(node).Updates(updates)

	agent := &agentConn{
		nodeId:  node.Id,
		secret:  secret,
		conn:    conn,
		pending: make(map[string]chan *CommandResponse),
		done:    make(chan struct{}),
	}
	GlobalHub.add(agent)
	logger.Infof("node %d (%s) agent connected", node.Id, node.Name)

	// Resync the full rule set shortly after the channel is live so a
	// restarted node/panel converges without manual action.
	go func() {
		time.Sleep(2 * time.Second)
		if GlobalHub.IsOnline(node.Id) {
			s.SyncNode(node.Id)
		}
	}()

	s.readLoop(agent)
	logger.Infof("node %d (%s) agent disconnected", node.Id, node.Name)
	database.GetDB().Model(node).Updates(map[string]any{
		"online":    false,
		"last_seen": time.Now().Unix(),
	})
}

// readLoop overrides the generic hub read loop with service-bound callbacks.
func (s *Service) readLoop(agent *agentConn) {
	GlobalHub.readLoop(agent, func(nodeId int, payload []byte) {
		s.UpdateNodeStatus(nodeId, payload)
	})
}

// maxFlowBody caps agent request bodies (1 MiB for traffic reports, 32 MiB
// for full-config dumps) so a misbehaving agent cannot exhaust memory.
const (
	maxFlowBodyBytes   = 1 << 20
	maxConfigBodyBytes = 32 << 20
)

// FlowUpload handles POST /flow/upload — per-service traffic reports.
func (s *Service) FlowUpload(c *gin.Context) {
	node := nodeBySecret(c.Query("secret"))
	if node == nil {
		c.String(http.StatusOK, "ok")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxFlowBodyBytes))
	if err != nil {
		c.String(http.StatusOK, "ok")
		return
	}
	payload, err := unwrapMessage(c.Query("secret"), raw)
	if err != nil {
		c.String(http.StatusOK, "ok")
		return
	}
	item := &flowItem{}
	if err := json.Unmarshal(payload, item); err != nil {
		c.String(http.StatusOK, "ok")
		return
	}
	s.ProcessFlow(item)
	c.String(http.StatusOK, "ok")
}

// FlowConfig handles POST /flow/config — periodic full-config reports from
// agents; acknowledged but not stored (the panel owns the source of truth).
func (s *Service) FlowConfig(c *gin.Context) {
	if nodeBySecret(c.Query("secret")) == nil {
		c.String(http.StatusOK, "ok")
		return
	}
	_, _ = io.Copy(io.Discard, http.MaxBytesReader(c.Writer, c.Request.Body, maxConfigBodyBytes))
	c.String(http.StatusOK, "ok")
}
