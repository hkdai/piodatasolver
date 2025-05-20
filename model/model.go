package model

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
	Node     string   `json:"node"`      //节点id
	Actor    string   `json:"actor"`     //行动方
	Board    string   `json:"board"`     //公共牌
	Hand     string   `json:"hand"`      //玩家手牌
	Actions  []Action `json:"actions"`   //玩家动作
	PotInfo  string   `json:"pot_info"`  //底池信息
}
