package cache

import (
	"strings"
	"sync"
	"time"

	"piodatasolver/internal/upi"
)

type HandOrder struct {
	order []string
	idx   map[string]int
	once  sync.Once
	err   error
}

// Init 会自动调用 UPI 客户端拉取 hand order，只执行一次
func (h *HandOrder) Init(client *upi.Client) error {
	h.once.Do(func() {
		lines, err := client.ExecuteCommand("show_hand_order", 10*time.Second)
		if err != nil {
			h.err = err
			return
		}
		h.order = strings.Fields(lines[0]) // 一行 1326 手牌
		h.idx = make(map[string]int, len(h.order))
		for i, hand := range h.order {
			h.idx[hand] = i
		}
	})
	return h.err
}

func (h *HandOrder) Order() []string { return h.order } //只读切片

func (h *HandOrder) Index(hand string) (int, bool) { //O(1)查手牌序号
	i, ok := h.idx[hand]
	return i, ok
}
