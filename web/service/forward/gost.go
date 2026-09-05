package forward

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/YCJE/XPanel/database/model"
)

// Service naming (reported back by agents in traffic uploads):
//   port-forward rule id=5  -> pf5_tcp / pf5_udp
//   tunnel rule id=7 entry  -> ti7_tcp / ti7_udp (client-facing, counted)
//   tunnel rule id=7 exit   -> te7_tls           (mirror traffic, ignored)
//   tunnel chain            -> ti7_chains
//   limiter                 -> <family> (pf5 / ti7 / te7)

// LimiterConfig builds a traffic limiter with symmetric up/down speed caps.
func LimiterConfig(family string, speedMbps int) map[string]any {
	return map[string]any{
		"name": family,
		"limits": []string{
			fmt.Sprintf("$ %dMB %dMB", speedMbps, speedMbps),
		},
	}
}

// deleteServiceRequest is the payload shape expected by the agent's
// DeleteService command.
func deleteServiceRequest(names ...string) map[string]any {
	return map[string]any{"services": names}
}

// deleteChainRequest is the payload shape for the DeleteChains command.
func deleteChainRequest(name string) map[string]any {
	return map[string]any{"chain": name}
}

// entryListenAddr returns the address the entry node binds for a rule.
// Empty node.InAddr falls back to node.ServerIP.
func entryListenAddr(node *model.ForwardNode) string {
	addr := node.InAddr
	if addr == "" {
		addr = node.ServerIP
	}
	if addr == "" {
		addr = "0.0.0.0"
	}
	return addr
}

// buildForwarder builds the multi-target forwarder block shared by services.
func buildForwarder(targets string, strategy string) (map[string]any, error) {
	targets = strings.TrimSpace(targets)
	if targets == "" {
		return nil, fmt.Errorf("目标地址不能为空")
	}
	nodes := make([]map[string]any, 0)
	for i, addr := range strings.Split(targets, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		nodes = append(nodes, map[string]any{
			"name": "node_" + strconv.Itoa(i+1),
			"addr": addr,
		})
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("目标地址不能为空")
	}
	if strategy == "" {
		strategy = "fifo"
	}
	return map[string]any{
		"nodes": nodes,
		"selector": map[string]any{
			"strategy":    strategy,
			"maxFails":    1,
			"failTimeout": "600s",
		},
	}, nil
}

// ForwardServices builds the tcp/udp service pair for a port-forward rule.
func ForwardServices(node *model.ForwardNode, rule *model.ForwardRule) ([]map[string]any, error) {
	forwarder, err := buildForwarder(rule.Target, rule.Strategy)
	if err != nil {
		return nil, err
	}
	listen := entryListenAddr(node)
	services := make([]map[string]any, 0)
	for _, proto := range []string{"tcp", "udp"} {
		svc := map[string]any{
			"name":      fmt.Sprintf("pf%d_%s", rule.Id, proto),
			"addr":      listen + ":" + strconv.Itoa(rule.InPort),
			"handler":   map[string]any{"type": proto},
			"listener":  map[string]any{"type": proto},
			"forwarder": forwarder,
		}
		if proto == "udp" {
			svc["listener"] = map[string]any{"type": "udp", "metadata": map[string]any{"keepAlive": true}}
		}
		if rule.Speed > 0 {
			svc["limiter"] = fmt.Sprintf("pf%d", rule.Id)
		}
		services = append(services, svc)
	}
	return services, nil
}

// TunnelEntryServices builds the tcp/udp service pair bound on the tunnel's
// entry node; traffic is relayed through <family>_chains to the exit node.
func TunnelEntryServices(node *model.ForwardNode, rule *model.TunnelRule) []map[string]any {
	family := fmt.Sprintf("ti%d", rule.Id)
	listen := entryListenAddr(node)
	services := make([]map[string]any, 0)
	for _, proto := range []string{"tcp", "udp"} {
		svc := map[string]any{
			"name":     family + "_" + proto,
			"addr":     listen + ":" + strconv.Itoa(rule.InPort),
			"handler":  map[string]any{"type": proto, "chain": family + "_chains"},
			"listener": map[string]any{"type": proto},
		}
		if proto == "udp" {
			svc["listener"] = map[string]any{"type": "udp", "metadata": map[string]any{"keepAlive": true}}
		}
		if rule.Speed > 0 {
			svc["limiter"] = family
		}
		services = append(services, svc)
	}
	return services
}

// TunnelChain builds the chain that dials the exit node over the tunnel
// transport (ws / tls / quic).
func TunnelChain(outNode *model.ForwardNode, rule *model.TunnelRule) map[string]any {
	family := fmt.Sprintf("ti%d", rule.Id)
	dialer := map[string]any{"type": rule.Transport}
	if rule.Transport == "quic" {
		dialer["metadata"] = map[string]any{"keepAlive": true, "ttl": "10s"}
	}
	node := map[string]any{
		"name":      "node-" + family,
		"addr":      outNode.ServerIP + ":" + strconv.Itoa(rule.OutPort),
		"connector": map[string]any{"type": "relay"},
		"dialer":    dialer,
	}
	return map[string]any{
		"name": family + "_chains",
		"hops": []any{
			map[string]any{
				"name":  "hop-" + family,
				"nodes": []any{node},
			},
		},
	}
}

// TunnelExitService builds the relay listener on the tunnel's exit node.
func TunnelExitService(rule *model.TunnelRule) ([]map[string]any, error) {
	forwarder, err := buildForwarder(rule.Target, rule.Strategy)
	if err != nil {
		return nil, err
	}
	svc := map[string]any{
		"name":      fmt.Sprintf("te%d_tls", rule.Id),
		"addr":      ":" + strconv.Itoa(rule.OutPort),
		"handler":   map[string]any{"type": "relay"},
		"listener":  map[string]any{"type": rule.Transport},
		"forwarder": forwarder,
	}
	if rule.Speed > 0 {
		svc["limiter"] = fmt.Sprintf("te%d", rule.Id)
	}
	return []map[string]any{svc}, nil
}

// mustJSON marshals v or panics; used only with map literals we build here.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
