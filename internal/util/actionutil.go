package util

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"piodatasolver/model"
)

var reBet = regexp.MustCompile(`^b(\d+)$`)

// Pot = “OOPCum IPCum DeadPot”
type Pot struct{ oop, ip, dead int }

func parsePot(s string) Pot {
	f := strings.Fields(s)
	a, _ := strconv.Atoi(f[0])
	b, _ := strconv.Atoi(f[1])
	c, _ := strconv.Atoi(f[2])
	return Pot{oop: a, ip: b, dead: c}
}

// ---------------------------------------------
// BuildActionLabel 把 childNode 转成 "bet 33%" / "raise 50%" / "call" / "check" / "fold"
//
// parentPotStr —— 父节点 show_node 的 pot 行（"0 0 60"）
// cn            —— show_children 拿到的子节点对象
// ---------------------------------------------
func BuildActionLabel(parentPotStr string, cn model.ChildNode) (string, error) {

	parent := parsePot(parentPotStr)
	child := parsePot(cn.PotInfo)

	// 1. 拿 NodeID 最后一个片段
	tail := cn.NodeID[strings.LastIndex(cn.NodeID, ":")+1:]

	// ----------------- 2. FOLD -----------------
	if tail == "f" {
		return "fold", nil
	}

	// ----------------- 3. CHECK / CALL ---------
	if tail == "c" {
		// 若对手之前没下注 → check；否则 call
		if parent.oop == parent.ip {
			return "check", nil
		}
		return "call", nil
	}

	// ----------------- 4. BET / RAISE ----------
	m := reBet.FindStringSubmatch(tail)
	if m == nil {
		return "", fmt.Errorf("un-recognized tail: %s", tail)
	}

	// 4.1 确定行动方 & 关键数字

	oldCum, newCum, oppCum := 0, 0, 0

	if child.ip != parent.ip { // IP 投注/加注

		oldCum, newCum = parent.ip, child.ip
		oppCum = parent.oop
	} else { // OOP 投注/加注

		oldCum, newCum = parent.oop, child.oop
		oppCum = parent.ip
	}

	// 4.2 下注前底池（first bet） or 跟注后底池（raise）
	betPct := 0.0
	if oppCum == oldCum { // **首次下注**
		potBefore := parent.dead + parent.oop + parent.ip
		betPct = float64(newCum-oldCum) / float64(potBefore)
		return fmt.Sprintf("bet %.0f%%", betPct*100), nil
	}

	// **加注**
	callAmt := oppCum - oldCum // 需先跟注
	potAfter := parent.dead + parent.oop + parent.ip + callAmt
	raiseInc := newCum - oldCum - callAmt
	betPct = float64(raiseInc) / float64(potAfter)
	return fmt.Sprintf("raise %.0f%%", betPct*100), nil
}
