package cache

import (
	"sort"
	"strings"
	"sync"
)

type BoardOrder struct {
	order []string
	idx   map[string]int
	once  sync.Once
	err   error
}

// 生成所有可能的三张公牌组合
func generateBoardCombinations() []string {
	// 52张牌的数组
	cards := []string{
		"2c", "3c", "4c", "5c", "6c", "7c", "8c", "9c", "Tc", "Jc", "Qc", "Kc", "Ac",
		"2d", "3d", "4d", "5d", "6d", "7d", "8d", "9d", "Td", "Jd", "Qd", "Kd", "Ad",
		"2h", "3h", "4h", "5h", "6h", "7h", "8h", "9h", "Th", "Jh", "Qh", "Kh", "Ah",
		"2s", "3s", "4s", "5s", "6s", "7s", "8s", "9s", "Ts", "Js", "Qs", "Ks", "As",
	}

	var boards []string
	// 生成所有三张牌的组合
	for i := 0; i < len(cards)-2; i++ {
		for j := i + 1; j < len(cards)-1; j++ {
			for k := j + 1; k < len(cards); k++ {
				// 创建一个临时切片来存储这三张牌
				board := []string{cards[i], cards[j], cards[k]}
				// 对牌进行排序以确保一致性
				sort.Strings(board)
				// 将三张牌组合成一个字符串
				boardStr := strings.Join(board, "")
				boards = append(boards, boardStr)
			}
		}
	}

	// 对所有公板组合进行排序以确保顺序一致性
	sort.Strings(boards)
	return boards
}

// Init 初始化 BoardOrder，生成所有可能的三张公牌组合
func (b *BoardOrder) Init() error {
	b.once.Do(func() {
		// 生成所有可能的三张公牌组合
		cards := []string{
			"2c", "3c", "4c", "5c", "6c", "7c", "8c", "9c", "Tc", "Jc", "Qc", "Kc", "Ac",
			"2d", "3d", "4d", "5d", "6d", "7d", "8d", "9d", "Td", "Jd", "Qd", "Kd", "Ad",
			"2h", "3h", "4h", "5h", "6h", "7h", "8h", "9h", "Th", "Jh", "Qh", "Kh", "Ah",
			"2s", "3s", "4s", "5s", "6s", "7s", "8s", "9s", "Ts", "Js", "Qs", "Ks", "As",
		}

		// 生成所有三张牌的组合
		var boards []string
		for i := 0; i < len(cards)-2; i++ {
			for j := i + 1; j < len(cards)-1; j++ {
				for k := j + 1; k < len(cards); k++ {
					// 按照标准顺序排列三张牌（大的牌在前）
					board := []string{cards[i], cards[j], cards[k]}
					sort.Slice(board, func(m, n int) bool {
						// 获取牌值和花色
						rank1, suit1 := board[m][0], board[m][1]
						rank2, suit2 := board[n][0], board[n][1]
						
						// 转换 T、J、Q、K、A 为对应的数值
						rankValue := func(r byte) int {
							switch r {
							case 'T':
								return 10
							case 'J':
								return 11
							case 'Q':
								return 12
							case 'K':
								return 13
							case 'A':
								return 14
							default:
								if r >= '2' && r <= '9' {
									return int(r - '0')
								}
								return 0
							}
						}
						
						// 首先按牌值比较
						rank1Val := rankValue(rank1)
						rank2Val := rankValue(rank2)
						if rank1Val != rank2Val {
							return rank1Val > rank2Val // 大的牌在前面
						}
						
						// 牌值相同时按花色排序 (s > h > d > c)
						suitValue := func(s byte) int {
							switch s {
							case 's':
								return 4
							case 'h':
								return 3
							case 'd':
								return 2
							case 'c':
								return 1
							default:
								return 0
							}
						}
						return suitValue(suit1) > suitValue(suit2)
					})
					
					boards = append(boards, strings.Join(board, " "))
				}
			}
		}

		// 初始化映射
		b.order = boards
		b.idx = make(map[string]int, len(boards))
		for i, board := range boards {
			b.idx[board] = i
		}
	})
	return nil
}

// Order 返回所有公牌组合的切片（只读）
func (b *BoardOrder) Order() []string {
	return b.order
}

// Index 根据公牌字符串返回其索引
func (b *BoardOrder) Index(board string) (int64, bool) {
	// 移除多余的空格并按空格分割成单张牌
	cards := strings.Fields(strings.TrimSpace(board))
	if len(cards) != 3 {
		return 0, false
	}
	
	// 对牌进行排序（按照值和花色）
	sort.Slice(cards, func(i, j int) bool {
		// 获取牌值和花色
		rank1, suit1 := cards[i][0], cards[i][1]
		rank2, suit2 := cards[j][0], cards[j][1]
		
		// 转换 T、J、Q、K、A 为对应的数值
		rankValue := func(r byte) int {
			switch r {
			case 'T':
				return 10
			case 'J':
				return 11
			case 'Q':
				return 12
			case 'K':
				return 13
			case 'A':
				return 14
			default:
				if r >= '2' && r <= '9' {
					return int(r - '0')
				}
				return 0
			}
		}
		
		// 首先按牌值比较
		rank1Val := rankValue(rank1)
		rank2Val := rankValue(rank2)
		if rank1Val != rank2Val {
			return rank1Val > rank2Val // 大的牌在前面
		}
		
		// 牌值相同时按花色排序 (s > h > d > c)
		suitValue := func(s byte) int {
			switch s {
			case 's':
				return 4
			case 'h':
				return 3
			case 'd':
				return 2
			case 'c':
				return 1
			default:
				return 0
			}
		}
		return suitValue(suit1) > suitValue(suit2)
	})
	
	// 组合成标准格式的字符串
	standardBoard := strings.Join(cards, " ")
	
	// 在映射中查找索引
	i, ok := b.idx[standardBoard]
	return int64(i), ok
}

// FormatBoard 格式化公牌字符串，确保一致的格式
func (b *BoardOrder) FormatBoard(board string) string {
	// 移除多余的空格并按空格分割成单张牌
	cards := strings.Fields(strings.TrimSpace(board))
	if len(cards) != 3 {
		return board
	}
	
	// 对牌进行排序（按照值和花色）
	sort.Slice(cards, func(i, j int) bool {
		// 获取牌值和花色
		rank1, suit1 := cards[i][0], cards[i][1]
		rank2, suit2 := cards[j][0], cards[j][1]
		
		// 转换 T、J、Q、K、A 为对应的数值
		rankValue := func(r byte) int {
			switch r {
			case 'T':
				return 10
			case 'J':
				return 11
			case 'Q':
				return 12
			case 'K':
				return 13
			case 'A':
				return 14
			default:
				if r >= '2' && r <= '9' {
					return int(r - '0')
				}
				return 0
			}
		}
		
		// 首先按牌值比较
		rank1Val := rankValue(rank1)
		rank2Val := rankValue(rank2)
		if rank1Val != rank2Val {
			return rank1Val > rank2Val // 大的牌在前面
		}
		
		// 牌值相同时按花色排序 (s > h > d > c)
		suitValue := func(s byte) int {
			switch s {
			case 's':
				return 4
			case 'h':
				return 3
			case 'd':
				return 2
			case 'c':
				return 1
			default:
				return 0
			}
		}
		return suitValue(suit1) > suitValue(suit2)
	})
	
	return strings.Join(cards, " ")
}

// GetBoardById 根据ID获取公牌组合
func (b *BoardOrder) GetBoardById(id int64) (string, bool) {
	if id < 0 || int(id) >= len(b.order) {
		return "", false
	}
	return b.order[int(id)], true
}

// Count 返回所有可能的公牌组合数量
func (b *BoardOrder) Count() int {
	return len(b.order)
} 