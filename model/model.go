package model

type Action struct {
	Label string  `json:"label"` //"bet 75%" / "check"
	Freq  float64 `json:"freq"`  //0.00-1.00
	Ev    float64 `json:"ev"`    //expected value
	Eq    float64 `json:"eq"`    //equity
}

type Record struct {
	Node    string   `json:"node"`    //节点id
	Board   string   `json:"board"`   //公共牌
	Hand    string   `json:"hand"`    //玩家手牌
	Actions []Action `json:"actions"` //玩家动作
}
