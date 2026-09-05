package model

// ForwardNode is a relay server running the gost agent that registers back to the panel.
type ForwardNode struct {
	Id        int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name      string `json:"name" form:"name"`
	ServerIP  string `json:"serverIp" form:"serverIp"`   // 节点间互连/对用户展示的真实地址
	InAddr    string `json:"inAddr" form:"inAddr"`       // 入口IP（多线 BGP 场景可与 ServerIP 不同，空则用 ServerIP）
	Secret    string `json:"secret" form:"secret" gorm:"unique;size:64"`
	PortStart int    `json:"portStart" form:"portStart"` // 允许使用的端口范围（0 表示不限）
	PortEnd   int    `json:"portEnd" form:"portEnd"`
	Version   string `json:"version" gorm:"default:''"`
	EnableHttp bool  `json:"enableHttp" gorm:"default:false"`
	EnableTls  bool  `json:"enableTls" gorm:"default:false"`
	EnableSocks bool `json:"enableSocks" gorm:"default:false"`
	// 运行时状态（由 agent 上报）
	Online   bool    `json:"online" gorm:"default:false"`
	CpuUsage float64 `json:"cpuUsage" gorm:"default:0"`
	MemUsage float64 `json:"memUsage" gorm:"default:0"`
	NetUp    uint64  `json:"netUp" gorm:"default:0"`
	NetDown  uint64  `json:"netDown" gorm:"default:0"`
	Uptime   uint64  `json:"uptime" gorm:"default:0"`
	LastSeen int64   `json:"lastSeen" gorm:"default:0"`
}

// ForwardRule is a single-node port forwarding rule (TCP+UDP).
type ForwardRule struct {
	Id       int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	NodeId   int    `json:"nodeId" form:"nodeId"`
	Name     string `json:"name" form:"name"`
	InPort   int    `json:"inPort" form:"inPort"`
	Target   string `json:"target" form:"target"`         // 逗号分隔的 host:port 列表，支持多目标
	Strategy string `json:"strategy" form:"strategy" gorm:"default:fifo"` // fifo / round / hash
	Ratio    float64 `json:"ratio" form:"ratio" gorm:"default:1"`   // 流量倍率
	Speed    int    `json:"speed" form:"speed"`           // 限速 Mbps，0 表示不限
	Enable   bool   `json:"enable" form:"enable" gorm:"default:true"`
	Up       int64  `json:"up" gorm:"default:0"`
	Down     int64  `json:"down" gorm:"default:0"`
	AllTime  int64  `json:"allTime" gorm:"default:0"`
	Remark   string `json:"remark" form:"remark"`
}

// TunnelRule is an entry-node → exit-node tunnel forwarding rule.
// 客户端连接入口节点的 InPort，流量经 ws/tls/quic 隧道送达出口节点的
// OutPort（relay 服务），再由出口节点转发到最终目标 Target。
type TunnelRule struct {
	Id        int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name      string `json:"name" form:"name"`
	InNodeId  int    `json:"inNodeId" form:"inNodeId"`
	OutNodeId int    `json:"outNodeId" form:"outNodeId"`
	InPort    int    `json:"inPort" form:"inPort"`
	OutPort   int    `json:"outPort" form:"outPort"`
	Transport string `json:"transport" form:"transport" gorm:"default:ws"` // ws / tls / quic
	Target    string `json:"target" form:"target"`   // 出口侧最终目标 host:port（逗号分隔可多目标）
	Strategy  string `json:"strategy" form:"strategy" gorm:"default:fifo"`
	Ratio     float64 `json:"ratio" form:"ratio" gorm:"default:1"`
	Speed     int    `json:"speed" form:"speed"`
	Enable    bool   `json:"enable" form:"enable" gorm:"default:true"`
	Up        int64  `json:"up" gorm:"default:0"`
	Down      int64  `json:"down" gorm:"default:0"`
	AllTime   int64  `json:"allTime" gorm:"default:0"`
	Remark    string `json:"remark" form:"remark"`
}

// ForwardStats keeps daily traffic aggregates per rule for trend display.
// RuleType: "forward" / "tunnel"
type ForwardStats struct {
	Id       int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	RuleType string `json:"ruleType" gorm:"index:idx_rule_day,priority:1"`
	RuleId   int    `json:"ruleId" gorm:"index:idx_rule_day,priority:2"`
	Day      string `json:"day" gorm:"index:idx_rule_day,priority:3;size:10"` // 2006-01-02
	Up       int64  `json:"up" gorm:"default:0"`
	Down     int64  `json:"down" gorm:"default:0"`
}
