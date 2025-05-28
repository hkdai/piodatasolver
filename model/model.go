package model

import (
	"encoding/json"
	"fmt"
)

type Action struct {
	ChildNodeID string  `json:"ChildNodeID"` // 这个action对应的子节点ID
	Label       string  `json:"label"`       //"bet 75%" / "check"
	Freq        float64 `json:"freq"`        //0.00-1.00
	Ev          float64 `json:"ev"`          //expected value
	Eq          float64 `json:"eq"`          //equity
	Matchup     float64 `json:"matchup"`     //match-up
}

// ChildNode 表示一个子节点的信息
type ChildNode struct {
	Index    int    `json:"index"`    // 子节点索引
	NodeID   string `json:"nodeId"`   // 子节点ID
	NodeType string `json:"nodeType"` // 节点类型 IP_DEC/OOP_DEC/SPLIT_NODE
	Board    string `json:"board"`    // 公牌
	PotInfo  string `json:"potInfo"`  // 底池信息
	ChildNum string `json:"childNum"` // 子节点数量
	Flag     string `json:"flag"`     // 标志
}

type Record struct {
	Node       string   `json:"node"`        //节点id
	Actor      string   `json:"actor"`       //行动方
	Board      string   `json:"board"`       //公共牌
	BoardId    int64    `json:"board_id"`    //公牌ID索引
	Hand       string   `json:"hand"`        //玩家手牌
	ComboId    int      `json:"combo_id"`    //手牌ID索引
	Actions    []Action `json:"actions"`     //玩家动作
	PotInfo    string   `json:"pot_info"`    //底池信息
	StackDepth float64  `json:"stack_depth"` //筹码深度（后手筹码）
	Spr        float64  `json:"-"`           //栈底比 - 使用自定义序列化
	BetPct     float64  `json:"-"`           //下注占底池比例 - 使用自定义序列化
	IpOrOop    string   `json:"ip_or_oop"`   //策略执行者（IP或OOP）
	BetLevel   int      `json:"bet_level"`   //主动下注次数
}

// MarshalJSON 自定义JSON序列化，控制Spr和BetPct的小数位数
func (r Record) MarshalJSON() ([]byte, error) {
	// 创建一个临时的结构体，包含格式化后的字段
	type Alias Record
	return json.Marshal(&struct {
		Spr    string `json:"spr"`
		BetPct string `json:"bet_pct"`
		*Alias
	}{
		Spr:    fmt.Sprintf("%.4f", r.Spr),
		BetPct: fmt.Sprintf("%.4f", r.BetPct),
		Alias:  (*Alias)(&r),
	})
}
