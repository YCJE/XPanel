package forward

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/YCJE/XPanel/database"
	"github.com/YCJE/XPanel/database/model"
	"github.com/YCJE/XPanel/logger"
	"gorm.io/gorm"
)

// Service implements the relay-forwarding business logic.
type Service struct{}

func newRequestId() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------- Nodes ----------

// GetNodes lists all relay nodes with live online status.
func (s *Service) GetNodes() ([]*model.ForwardNode, error) {
	nodes := make([]*model.ForwardNode, 0)
	err := database.GetDB().Order("id").Find(&nodes).Error
	for _, n := range nodes {
		n.Online = GlobalHub.IsOnline(n.Id)
	}
	return nodes, err
}

// AddNode creates a node and generates its agent secret.
func (s *Service) AddNode(name, serverIp, inAddr string, portStart, portEnd int) (*model.ForwardNode, error) {
	if err := validateNodeFields(name, serverIp, inAddr, portStart, portEnd); err != nil {
		return nil, err
	}
	node := &model.ForwardNode{
		Name:      name,
		ServerIP:  serverIp,
		InAddr:    inAddr,
		PortStart: portStart,
		PortEnd:   portEnd,
		Secret:    newSecret(),
	}
	if err := database.GetDB().Create(node).Error; err != nil {
		return nil, err
	}
	return node, nil
}

func validateNodeFields(name, serverIp, inAddr string, portStart, portEnd int) error {
	if name == "" || serverIp == "" {
		return fmt.Errorf("节点名称与服务器IP不能为空")
	}
	if inAddr != "" && net.ParseIP(inAddr) == nil {
		return fmt.Errorf("入口IP必须是合法 IP 地址（留空则绑定 0.0.0.0）")
	}
	if portStart < 0 || portEnd < 0 {
		return fmt.Errorf("端口范围不能为负数")
	}
	if portStart > 0 && portEnd >= portStart && portEnd > 65535 {
		return fmt.Errorf("端口范围超出 65535")
	}
	return nil
}

// UpdateNode saves editable node fields, keeping the existing secret.
func (s *Service) UpdateNode(node *model.ForwardNode) error {
	if node.Id <= 0 {
		return fmt.Errorf("无效的节点")
	}
	if err := validateNodeFields(node.Name, node.ServerIP, node.InAddr, node.PortStart, node.PortEnd); err != nil {
		return err
	}
	return database.GetDB().Model(&model.ForwardNode{}).
		Where("id = ?", node.Id).
		Updates(map[string]any{
			"name":       node.Name,
			"server_ip":  node.ServerIP,
			"in_addr":    node.InAddr,
			"port_start": node.PortStart,
			"port_end":   node.PortEnd,
		}).Error
}

// DelNode removes a node together with its rules.
func (s *Service) DelNode(id int) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Delete(&model.ForwardNode{}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", id).Delete(&model.ForwardRule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("in_node_id = ? OR out_node_id = ?", id, id).
			Delete(&model.TunnelRule{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// ---------- Port forwarding rules ----------

// GetForwardRules lists port-forward rules.
func (s *Service) GetForwardRules() ([]*model.ForwardRule, error) {
	rules := make([]*model.ForwardRule, 0)
	err := database.GetDB().Order("id").Find(&rules).Error
	return rules, err
}

func (s *Service) validateForwardRule(rule *model.ForwardRule) error {
	if rule.Name == "" || rule.InPort <= 0 || rule.InPort > 65535 {
		return fmt.Errorf("规则名称与监听端口必填且端口需在 1-65535")
	}
	node := &model.ForwardNode{}
	if err := database.GetDB().First(node, rule.NodeId).Error; err != nil {
		return fmt.Errorf("节点不存在")
	}
	if node.PortStart > 0 && node.PortEnd >= node.PortStart {
		if rule.InPort < node.PortStart || rule.InPort > node.PortEnd {
			return fmt.Errorf("端口超出节点允许范围 %d-%d", node.PortStart, node.PortEnd)
		}
	}
	if s.portTaken(rule.NodeId, rule.InPort, rule.Id, 0) {
		return fmt.Errorf("该端口在节点上已被其他转发/隧道规则占用")
	}
	if rule.Ratio <= 0 {
		rule.Ratio = 1
	}
	return nil
}

// portTaken reports whether any other forward/tunnel rule already binds the
// given port on the node (disabled rules still hold their port reservation).
func (s *Service) portTaken(nodeId, port, excludeForwardId, excludeTunnelId int) bool {
	var forwardCount, tunnelCount int64
	database.GetDB().Model(&model.ForwardRule{}).
		Where("node_id = ? AND in_port = ? AND id <> ?", nodeId, port, excludeForwardId).
		Count(&forwardCount)
	database.GetDB().Model(&model.TunnelRule{}).
		Where("(in_node_id = ? AND in_port = ? AND id <> ?) OR (out_node_id = ? AND out_port = ? AND id <> ?)",
			nodeId, port, excludeTunnelId, nodeId, port, excludeTunnelId).
		Count(&tunnelCount)
	return forwardCount+tunnelCount > 0
}

// AddForwardRule persists a rule and pushes it to the agent.
func (s *Service) AddForwardRule(rule *model.ForwardRule, enable *bool) error {
	if err := s.validateForwardRule(rule); err != nil {
		return err
	}
	rule.Enable = enable == nil || *enable
	if err := database.GetDB().Create(rule).Error; err != nil {
		return err
	}
	s.applyForwardRule(rule)
	return nil
}

// UpdateForwardRule replaces the rule config on the agent while preserving
// traffic counters (and enable state unless explicitly toggled).
func (s *Service) UpdateForwardRule(rule *model.ForwardRule, enable *bool) error {
	existing := &model.ForwardRule{}
	if err := database.GetDB().First(existing, rule.Id).Error; err != nil {
		return fmt.Errorf("规则不存在")
	}
	if err := s.validateForwardRule(rule); err != nil {
		return err
	}
	rule.Up, rule.Down, rule.AllTime = existing.Up, existing.Down, existing.AllTime
	rule.Enable = existing.Enable
	if enable != nil {
		rule.Enable = *enable
	}
	if err := database.GetDB().Save(rule).Error; err != nil {
		return err
	}
	s.applyForwardRule(rule)
	return nil
}

// ToggleForwardRule enables/disables a rule.
func (s *Service) ToggleForwardRule(id int, enable bool) error {
	rule := &model.ForwardRule{}
	if err := database.GetDB().First(rule, id).Error; err != nil {
		return err
	}
	rule.Enable = enable
	if err := database.GetDB().Save(rule).Error; err != nil {
		return err
	}
	s.applyForwardRule(rule)
	return nil
}

// DelForwardRule removes the rule and its services on the agent.
func (s *Service) DelForwardRule(id int) error {
	rule := &model.ForwardRule{}
	if err := database.GetDB().First(rule, id).Error; err != nil {
		return err
	}
	s.removeForwardFromAgent(rule)
	return database.GetDB().Delete(rule).Error
}

// applyForwardRule pushes (or removes when disabled) the rule services.
func (s *Service) applyForwardRule(rule *model.ForwardRule) {
	node := &model.ForwardNode{}
	if err := database.GetDB().First(node, rule.NodeId).Error; err != nil {
		return
	}
	family := fmt.Sprintf("pf%d", rule.Id)
	if !rule.Enable {
		s.removeServices(node.Id, family, true)
		return
	}
	services, err := ForwardServices(node, rule)
	if err != nil {
		logger.Errorf("forward rule %d: %v", rule.Id, err)
		return
	}
	s.pushServicesWithSpeed(node.Id, family, services, rule.Speed, true)
}

// removeServices deletes a service family (+udp twin) and its limiter.
func (s *Service) removeServices(nodeId int, family string, withUdp bool) {
	if !GlobalHub.IsOnline(nodeId) {
		return
	}
	names := []string{family + "_tcp"}
	if withUdp {
		names = append(names, family+"_udp")
	}
	_, _ = GlobalHub.SendCommand(nodeId, "DeleteService", deleteServiceRequest(names...), 10*time.Second)
	_, _ = GlobalHub.SendCommand(nodeId, "DeleteLimiters", map[string]any{"limiter": family}, 10*time.Second)
}

// pushServicesWithSpeed upserts a service family on a node: delete (ignore
// error) then add, plus a limiter when speed > 0.
func (s *Service) pushServicesWithSpeed(nodeId int, family string, services []map[string]any, speed int, withUdp bool) {
	if !GlobalHub.IsOnline(nodeId) {
		logger.Infof("node %d offline; config will sync on connect", nodeId)
		return
	}
	names := []string{family + "_tcp"}
	if withUdp {
		names = append(names, family+"_udp")
	}
	_, _ = GlobalHub.SendCommand(nodeId, "DeleteService", deleteServiceRequest(names...), 10*time.Second)
	if _, err := GlobalHub.SendCommand(nodeId, "AddService", services, 15*time.Second); err != nil {
		logger.Errorf("node %d AddService %s failed: %v", nodeId, family, err)
		return
	}
	if speed > 0 {
		_, _ = GlobalHub.SendCommand(nodeId, "AddLimiters", LimiterConfig(family, speed), 10*time.Second)
	} else {
		// Rule previously had a speed cap — drop the now-unused limiter.
		_, _ = GlobalHub.SendCommand(nodeId, "DeleteLimiters", map[string]any{"limiter": family}, 10*time.Second)
	}
}

// ---------- Tunnel rules ----------

// GetTunnelRules lists tunnel rules.
func (s *Service) GetTunnelRules() ([]*model.TunnelRule, error) {
	rules := make([]*model.TunnelRule, 0)
	err := database.GetDB().Order("id").Find(&rules).Error
	return rules, err
}

func (s *Service) validateTunnelRule(rule *model.TunnelRule) error {
	if rule.Name == "" || rule.InPort <= 0 || rule.InPort > 65535 ||
		rule.OutPort <= 0 || rule.OutPort > 65535 {
		return fmt.Errorf("规则名称与端口必填且端口需在 1-65535")
	}
	if rule.Transport == "" {
		rule.Transport = "ws"
	}
	inNode := &model.ForwardNode{}
	if err := database.GetDB().First(inNode, rule.InNodeId).Error; err != nil {
		return fmt.Errorf("入口节点不存在")
	}
	outNode := &model.ForwardNode{}
	if err := database.GetDB().First(outNode, rule.OutNodeId).Error; err != nil {
		return fmt.Errorf("出口节点不存在")
	}
	if rule.InNodeId == rule.OutNodeId {
		return fmt.Errorf("入口与出口不能是同一节点")
	}
	if s.portTaken(rule.InNodeId, rule.InPort, 0, rule.Id) {
		return fmt.Errorf("入口端口在节点上已被其他转发/隧道规则占用")
	}
	if s.portTaken(rule.OutNodeId, rule.OutPort, 0, rule.Id) {
		return fmt.Errorf("出口端口在节点上已被其他转发/隧道规则占用")
	}
	if rule.Ratio <= 0 {
		rule.Ratio = 1
	}
	return nil
}

// AddTunnelRule persists a tunnel rule and pushes config to both ends.
func (s *Service) AddTunnelRule(rule *model.TunnelRule, enable *bool) error {
	if err := s.validateTunnelRule(rule); err != nil {
		return err
	}
	rule.Enable = enable == nil || *enable
	if err := database.GetDB().Create(rule).Error; err != nil {
		return err
	}
	s.applyTunnelRule(rule)
	return nil
}

// UpdateTunnelRule replaces the tunnel config on both nodes while preserving
// traffic counters (and enable state unless explicitly toggled).
func (s *Service) UpdateTunnelRule(rule *model.TunnelRule, enable *bool) error {
	existing := &model.TunnelRule{}
	if err := database.GetDB().First(existing, rule.Id).Error; err != nil {
		return fmt.Errorf("规则不存在")
	}
	if err := s.validateTunnelRule(rule); err != nil {
		return err
	}
	rule.Up, rule.Down, rule.AllTime = existing.Up, existing.Down, existing.AllTime
	rule.Enable = existing.Enable
	if enable != nil {
		rule.Enable = *enable
	}
	if err := database.GetDB().Save(rule).Error; err != nil {
		return err
	}
	s.applyTunnelRule(rule)
	return nil
}

// ToggleTunnelRule enables/disables a tunnel rule.
func (s *Service) ToggleTunnelRule(id int, enable bool) error {
	rule := &model.TunnelRule{}
	if err := database.GetDB().First(rule, id).Error; err != nil {
		return err
	}
	rule.Enable = enable
	if err := database.GetDB().Save(rule).Error; err != nil {
		return err
	}
	s.applyTunnelRule(rule)
	return nil
}

// DelTunnelRule removes the tunnel config from both nodes and the DB.
func (s *Service) DelTunnelRule(id int) error {
	rule := &model.TunnelRule{}
	if err := database.GetDB().First(rule, id).Error; err != nil {
		return err
	}
	s.removeTunnelFromAgent(rule)
	return database.GetDB().Delete(rule).Error
}

// applyTunnelRule pushes (or removes when disabled) the tunnel config on the
// entry node (services + chain) and the exit node (relay listener).
func (s *Service) applyTunnelRule(rule *model.TunnelRule) {
	inNode := &model.ForwardNode{}
	outNode := &model.ForwardNode{}
	if err := database.GetDB().First(inNode, rule.InNodeId).Error; err != nil {
		return
	}
	if err := database.GetDB().First(outNode, rule.OutNodeId).Error; err != nil {
		return
	}
	family := fmt.Sprintf("ti%d", rule.Id)
	if !rule.Enable {
		s.removeServices(inNode.Id, family, true)
		if GlobalHub.IsOnline(inNode.Id) {
			_, _ = GlobalHub.SendCommand(inNode.Id, "DeleteChains", deleteChainRequest(family), 10*time.Second)
		}
		if GlobalHub.IsOnline(outNode.Id) {
			_, _ = GlobalHub.SendCommand(outNode.Id, "DeleteService",
				deleteServiceRequest(fmt.Sprintf("te%d_tls", rule.Id)), 10*time.Second)
			_, _ = GlobalHub.SendCommand(outNode.Id, "DeleteLimiters",
				map[string]any{"limiter": fmt.Sprintf("te%d", rule.Id)}, 10*time.Second)
		}
		return
	}

	// Entry node: chain + services.
	if GlobalHub.IsOnline(inNode.Id) {
		chain := TunnelChain(outNode, rule)
		_, _ = GlobalHub.SendCommand(inNode.Id, "DeleteChains", deleteChainRequest(family), 10*time.Second)
		if _, err := GlobalHub.SendCommand(inNode.Id, "AddChains", chain, 10*time.Second); err != nil {
			logger.Errorf("tunnel %d: add chain on node %d: %v", rule.Id, inNode.Id, err)
		} else if rule.Speed > 0 {
			_, _ = GlobalHub.SendCommand(inNode.Id, "AddLimiters",
				LimiterConfig(family, rule.Speed), 10*time.Second)
		}
		s.pushServicesWithSpeed(inNode.Id, family, TunnelEntryServices(inNode, rule), rule.Speed, true)
	} else {
		logger.Infof("tunnel %d entry node %d offline; sync on connect", rule.Id, inNode.Id)
	}

	// Exit node: relay listener + limiter.
	if GlobalHub.IsOnline(outNode.Id) {
		exitFamily := fmt.Sprintf("te%d", rule.Id)
		services, err := TunnelExitService(rule)
		if err != nil {
			logger.Errorf("tunnel %d: %v", rule.Id, err)
			return
		}
		s.pushServicesWithSpeed(outNode.Id, exitFamily, services, rule.Speed, false)
	} else {
		logger.Infof("tunnel %d exit node %d offline; sync on connect", rule.Id, outNode.Id)
	}
}

func (s *Service) removeForwardFromAgent(rule *model.ForwardRule) {
	node := &model.ForwardNode{}
	if err := database.GetDB().First(node, rule.NodeId).Error; err != nil {
		return
	}
	s.removeServices(node.Id, fmt.Sprintf("pf%d", rule.Id), true)
}

func (s *Service) removeTunnelFromAgent(rule *model.TunnelRule) {
	family := fmt.Sprintf("ti%d", rule.Id)
	s.removeServices(rule.InNodeId, family, true)
	if GlobalHub.IsOnline(rule.InNodeId) {
		_, _ = GlobalHub.SendCommand(rule.InNodeId, "DeleteChains", deleteChainRequest(family), 10*time.Second)
	}
	if GlobalHub.IsOnline(rule.OutNodeId) {
		_, _ = GlobalHub.SendCommand(rule.OutNodeId, "DeleteService",
			deleteServiceRequest(fmt.Sprintf("te%d_tls", rule.Id)), 10*time.Second)
		_, _ = GlobalHub.SendCommand(rule.OutNodeId, "DeleteLimiters",
			map[string]any{"limiter": fmt.Sprintf("te%d", rule.Id)}, 10*time.Second)
	}
}

// SyncNode re-applies every rule of a node — invoked when an agent connects
// or when the admin requests a manual resync.
func (s *Service) SyncNode(nodeId int) {
	node := &model.ForwardNode{}
	if err := database.GetDB().First(node, nodeId).Error; err != nil {
		return
	}

	forwards := make([]*model.ForwardRule, 0)
	if err := database.GetDB().Where("node_id = ?", nodeId).Find(&forwards).Error; err == nil {
		for _, rule := range forwards {
			if !rule.Enable {
				continue
			}
			services, err := ForwardServices(node, rule)
			if err != nil {
				logger.Errorf("sync forward rule %d: %v", rule.Id, err)
				continue
			}
			s.pushServicesWithSpeed(nodeId, fmt.Sprintf("pf%d", rule.Id), services, rule.Speed, true)
		}
	}

	// Tunnels where this node is the entry.
	tunnels := make([]*model.TunnelRule, 0)
	if err := database.GetDB().Where("in_node_id = ?", nodeId).Find(&tunnels).Error; err == nil {
		outNode := &model.ForwardNode{}
		for _, rule := range tunnels {
			if !rule.Enable {
				continue
			}
			if err := database.GetDB().First(outNode, rule.OutNodeId).Error; err != nil {
				continue
			}
			family := fmt.Sprintf("ti%d", rule.Id)
			chain := TunnelChain(outNode, rule)
			_, _ = GlobalHub.SendCommand(nodeId, "DeleteChains", deleteChainRequest(family), 10*time.Second)
			if _, err := GlobalHub.SendCommand(nodeId, "AddChains", chain, 10*time.Second); err == nil && rule.Speed > 0 {
				_, _ = GlobalHub.SendCommand(nodeId, "AddLimiters", LimiterConfig(family, rule.Speed), 10*time.Second)
			}
			s.pushServicesWithSpeed(nodeId, family, TunnelEntryServices(node, rule), rule.Speed, true)
		}
	}

	// Tunnels where this node is the exit.
	exits := make([]*model.TunnelRule, 0)
	if err := database.GetDB().Where("out_node_id = ?", nodeId).Find(&exits).Error; err == nil {
		for _, rule := range exits {
			if !rule.Enable {
				continue
			}
			services, err := TunnelExitService(rule)
			if err != nil {
				logger.Errorf("sync tunnel exit %d: %v", rule.Id, err)
				continue
			}
			s.pushServicesWithSpeed(nodeId, fmt.Sprintf("te%d", rule.Id), services, rule.Speed, false)
		}
	}
}

// ---------- Traffic accounting ----------

type flowItem struct {
	N string `json:"n"`
	U int64  `json:"u"`
	D int64  `json:"d"`
}

// ProcessFlow applies an agent traffic report to the matching rule.
func (s *Service) ProcessFlow(item *flowItem) {
	if item == nil || item.N == "" || item.N == "web_api" {
		return
	}
	if item.U < 0 {
		item.U = 0
	}
	if item.D < 0 {
		item.D = 0
	}
	if item.U == 0 && item.D == 0 {
		return
	}
	parts := strings.SplitN(item.N, "_", 2)
	if len(parts) < 2 {
		return
	}
	prefix := parts[0]
	if prefix == "" {
		return
	}
	day := time.Now().Format("2006-01-02")

	switch {
	case strings.HasPrefix(prefix, "pf"):
		var id int
		if _, err := fmt.Sscanf(prefix, "pf%d", &id); err != nil {
			return
		}
		rule := &model.ForwardRule{}
		if err := database.GetDB().Select("id", "ratio").First(rule, id).Error; err != nil {
			return
		}
		up := int64(float64(item.U) * rule.Ratio)
		down := int64(float64(item.D) * rule.Ratio)
		// Atomic increments: concurrent reports for the same rule must not
		// overwrite each other's counters.
		database.GetDB().Model(&model.ForwardRule{}).Where("id = ?", rule.Id).Updates(map[string]any{
			"up":       gorm.Expr("up + ?", up),
			"down":     gorm.Expr("down + ?", down),
			"all_time": gorm.Expr("all_time + ?", up+down),
		})
		s.addDailyStats("forward", rule.Id, day, up, down)

	case strings.HasPrefix(prefix, "ti"):
		var id int
		if _, err := fmt.Sscanf(prefix, "ti%d", &id); err != nil {
			return
		}
		rule := &model.TunnelRule{}
		if err := database.GetDB().Select("id", "ratio").First(rule, id).Error; err != nil {
			return
		}
		up := int64(float64(item.U) * rule.Ratio)
		down := int64(float64(item.D) * rule.Ratio)
		database.GetDB().Model(&model.TunnelRule{}).Where("id = ?", rule.Id).Updates(map[string]any{
			"up":       gorm.Expr("up + ?", up),
			"down":     gorm.Expr("down + ?", down),
			"all_time": gorm.Expr("all_time + ?", up+down),
		})
		s.addDailyStats("tunnel", rule.Id, day, up, down)

	case strings.HasPrefix(prefix, "te"):
		// Exit-side traffic mirrors the entry side; already counted there.
		return
	}
}

// addDailyStats upserts the per-day aggregate row.
func (s *Service) addDailyStats(ruleType string, ruleId int, day string, up, down int64) {
	stats := &model.ForwardStats{}
	err := database.GetDB().
		Where("rule_type = ? AND rule_id = ? AND day = ?", ruleType, ruleId, day).
		First(stats).Error
	if database.IsNotFound(err) {
		database.GetDB().Create(&model.ForwardStats{
			RuleType: ruleType, RuleId: ruleId, Day: day, Up: up, Down: down,
		})
		return
	}
	if err == nil {
		database.GetDB().Model(stats).Updates(map[string]any{
			"up": stats.Up + up, "down": stats.Down + down,
		})
	}
}

// ResetRuleTraffic zeroes the counters of one rule.
func (s *Service) ResetRuleTraffic(ruleType string, id int) error {
	updates := map[string]any{"up": 0, "down": 0, "all_time": 0}
	if ruleType == "forward" {
		return database.GetDB().Model(&model.ForwardRule{}).Where("id = ?", id).Updates(updates).Error
	}
	return database.GetDB().Model(&model.TunnelRule{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateNodeStatus persists a system-info heartbeat from an agent.
func (s *Service) UpdateNodeStatus(nodeId int, payload []byte) {
	var info struct {
		Uptime           uint64  `json:"uptime"`
		BytesReceived    uint64  `json:"bytes_received"`
		BytesTransmitted uint64  `json:"bytes_transmitted"`
		CPUUsage         float64 `json:"cpu_usage"`
		MemoryUsage      float64 `json:"memory_usage"`
	}
	if err := json.Unmarshal(payload, &info); err != nil {
		return
	}
	database.GetDB().Model(&model.ForwardNode{}).Where("id = ?", nodeId).Updates(map[string]any{
		"online":    true,
		"cpu_usage": info.CPUUsage,
		"mem_usage": info.MemoryUsage,
		"net_up":    info.BytesTransmitted,
		"net_down":  info.BytesReceived,
		"uptime":    info.Uptime,
		"last_seen": time.Now().Unix(),
	})
}
