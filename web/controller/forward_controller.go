package controller

import (
	"net/http"
	"strconv"

	"github.com/YCJE/XPanel/database/model"
	"github.com/YCJE/XPanel/web/service/forward"

	"github.com/gin-gonic/gin"
)

// ForwardController exposes the relay-forwarding management API.
type ForwardController struct {
	BaseController
	service forward.Service
}

// NewForwardController creates the controller under /panel/api/forward.
func NewForwardController(g *gin.RouterGroup) *ForwardController {
	a := &ForwardController{}
	a.initRouter(g.Group("/forward"))
	return a
}

func (a *ForwardController) initRouter(g *gin.RouterGroup) {
	g.GET("/nodes", a.getNodes)
	g.POST("/nodes/add", a.addNode)
	g.POST("/nodes/update", a.updateNode)
	g.POST("/nodes/del/:id", a.delNode)
	g.POST("/nodes/sync/:id", a.syncNode)

	g.GET("/rules", a.getRules)
	g.POST("/rules/add", a.addRule)
	g.POST("/rules/update", a.updateRule)
	g.POST("/rules/del/:id", a.delRule)
	g.POST("/rules/toggle/:id", a.toggleRule)
	g.POST("/rules/traffic/:id", a.resetRuleTraffic)

	g.GET("/tunnels", a.getTunnels)
	g.POST("/tunnels/add", a.addTunnel)
	g.POST("/tunnels/update", a.updateTunnel)
	g.POST("/tunnels/del/:id", a.delTunnel)
	g.POST("/tunnels/toggle/:id", a.toggleTunnel)
	g.POST("/tunnels/traffic/:id", a.resetTunnelTraffic)
}

func idParam(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "msg": "无效的 ID"})
		return 0, false
	}
	return id, true
}

// ---------- nodes ----------

func (a *ForwardController) getNodes(c *gin.Context) {
	nodes, err := a.service.GetNodes()
	jsonObj(c, nodes, err)
}

type nodeForm struct {
	Name      string `json:"name" form:"name"`
	ServerIP  string `json:"serverIp" form:"serverIp"`
	InAddr    string `json:"inAddr" form:"inAddr"`
	PortStart int    `json:"portStart" form:"portStart"`
	PortEnd   int    `json:"portEnd" form:"portEnd"`
	Id        int    `json:"id" form:"id"`
}

func (a *ForwardController) addNode(c *gin.Context) {
	var form nodeForm
	if err := c.ShouldBind(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误")
		return
	}
	node, err := a.service.AddNode(form.Name, form.ServerIP, form.InAddr, form.PortStart, form.PortEnd)
	if err != nil {
		pureJsonMsg(c, http.StatusOK, false, err.Error())
		return
	}
	jsonObj(c, node, nil)
}

func (a *ForwardController) updateNode(c *gin.Context) {
	var form nodeForm
	if err := c.ShouldBind(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误")
		return
	}
	err := a.service.UpdateNode(&model.ForwardNode{Id: form.Id, Name: form.Name, ServerIP: form.ServerIP, InAddr: form.InAddr, PortStart: form.PortStart, PortEnd: form.PortEnd})
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) delNode(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	err := a.service.DelNode(id)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) syncNode(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	go a.service.SyncNode(id)
	pureJsonMsg(c, http.StatusOK, true, "")
}

// ---------- port forwarding rules ----------

func (a *ForwardController) getRules(c *gin.Context) {
	rules, err := a.service.GetForwardRules()
	jsonObj(c, rules, err)
}

type forwardRuleForm struct {
	Id       int     `json:"id" form:"id"`
	NodeId   int     `json:"nodeId" form:"nodeId"`
	Name     string  `json:"name" form:"name"`
	InPort   int     `json:"inPort" form:"inPort"`
	Target   string  `json:"target" form:"target"`
	Strategy string  `json:"strategy" form:"strategy"`
	Ratio    float64 `json:"ratio" form:"ratio"`
	Speed    int     `json:"speed" form:"speed"`
	Enable   *bool   `json:"enable" form:"enable"`
	Remark   string  `json:"remark" form:"remark"`
}

func (f *forwardRuleForm) toModel() *model.ForwardRule {
	return &model.ForwardRule{
		Id: f.Id, NodeId: f.NodeId, Name: f.Name, InPort: f.InPort,
		Target: f.Target, Strategy: f.Strategy, Ratio: f.Ratio,
		Speed: f.Speed, Remark: f.Remark,
	}
}

func (a *ForwardController) addRule(c *gin.Context) {
	var form forwardRuleForm
	if err := c.ShouldBind(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误")
		return
	}
	err := a.service.AddForwardRule(form.toModel(), form.Enable)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) updateRule(c *gin.Context) {
	var form forwardRuleForm
	if err := c.ShouldBind(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误")
		return
	}
	err := a.service.UpdateForwardRule(form.toModel(), form.Enable)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) delRule(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	err := a.service.DelForwardRule(id)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) toggleRule(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var form struct {
		Enable *bool `json:"enable" form:"enable"`
	}
	if err := c.ShouldBind(&form); err != nil || form.Enable == nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误: 缺少 enable 字段")
		return
	}
	err := a.service.ToggleForwardRule(id, *form.Enable)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) resetRuleTraffic(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	err := a.service.ResetRuleTraffic("forward", id)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

// ---------- tunnel rules ----------

func (a *ForwardController) getTunnels(c *gin.Context) {
	rules, err := a.service.GetTunnelRules()
	jsonObj(c, rules, err)
}

type tunnelRuleForm struct {
	Id        int     `json:"id" form:"id"`
	Name      string  `json:"name" form:"name"`
	InNodeId  int     `json:"inNodeId" form:"inNodeId"`
	OutNodeId int     `json:"outNodeId" form:"outNodeId"`
	InPort    int     `json:"inPort" form:"inPort"`
	OutPort   int     `json:"outPort" form:"outPort"`
	Transport string  `json:"transport" form:"transport"`
	Target    string  `json:"target" form:"target"`
	Strategy  string  `json:"strategy" form:"strategy"`
	Ratio     float64 `json:"ratio" form:"ratio"`
	Speed     int     `json:"speed" form:"speed"`
	Enable    *bool   `json:"enable" form:"enable"`
	Remark    string  `json:"remark" form:"remark"`
}

func (f *tunnelRuleForm) toModel() *model.TunnelRule {
	return &model.TunnelRule{
		Id: f.Id, Name: f.Name, InNodeId: f.InNodeId, OutNodeId: f.OutNodeId,
		InPort: f.InPort, OutPort: f.OutPort, Transport: f.Transport,
		Target: f.Target, Strategy: f.Strategy, Ratio: f.Ratio,
		Speed: f.Speed, Remark: f.Remark,
	}
}

func (a *ForwardController) addTunnel(c *gin.Context) {
	var form tunnelRuleForm
	if err := c.ShouldBind(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误")
		return
	}
	err := a.service.AddTunnelRule(form.toModel(), form.Enable)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) updateTunnel(c *gin.Context) {
	var form tunnelRuleForm
	if err := c.ShouldBind(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误")
		return
	}
	err := a.service.UpdateTunnelRule(form.toModel(), form.Enable)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) delTunnel(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	err := a.service.DelTunnelRule(id)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) toggleTunnel(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var form struct {
		Enable *bool `json:"enable" form:"enable"`
	}
	if err := c.ShouldBind(&form); err != nil || form.Enable == nil {
		pureJsonMsg(c, http.StatusOK, false, "参数错误: 缺少 enable 字段")
		return
	}
	err := a.service.ToggleTunnelRule(id, *form.Enable)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func (a *ForwardController) resetTunnelTraffic(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	err := a.service.ResetRuleTraffic("tunnel", id)
	pureJsonMsg(c, http.StatusOK, err == nil, errString(err))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
