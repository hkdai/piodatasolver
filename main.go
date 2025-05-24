package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"piodatasolver/internal/cache"
	"piodatasolver/internal/upi"
	"piodatasolver/internal/util"
	"piodatasolver/model"
)

var handOrder *cache.HandOrder
var boardOrder *cache.BoardOrder

// CFR文件路径 - 用于生成输出文件名
var cfrFilePath string

// 全局变量，用于统计过滤的动作数量
var (
	filteredActionCount int = 0
)

// 新增：从set_board命令提取公牌信息
func extractBoardFromTemplate(templateContent string) string {
	// 正则表达式：匹配set_board命令
	setBoardRegex := regexp.MustCompile(`(?m)^set_board\s+([A-Za-z0-9]+)`)
	match := setBoardRegex.FindStringSubmatch(templateContent)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}



// 修改main函数，添加命令行参数支持
func main() {
	// 检查命令行参数
	if len(os.Args) < 2 {
		fmt.Println("用法: go run main.go [parse|calc]")
		fmt.Println("  parse - 解析PioSolver数据并生成JSON/SQL文件")
		fmt.Println("  calc  - 执行PioSolver计算功能")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "parse":
		log.Println("执行解析功能...")
		runParseCommand()
	case "calc":
		log.Println("执行计算功能...")
		runCalcCommand()
	default:
		fmt.Printf("未知命令: %s\n", command)
		fmt.Println("支持的命令: parse, calc")
		os.Exit(1)
	}
}

// runParseCommand 执行原有的解析功能
func runParseCommand() {
	// 原有的单个CFR文件处理逻辑
	client := upi.NewClient("./PioSOLVER3-edge.exe", `D:\gto\piosolver3`)

	// 设置目标节点
	targetNode := "r:0"

	// 启动PioSolver
	if err := client.Start(); err != nil {
		log.Fatalf("启动PioSolver失败: %v", err)
	}
	defer client.Close()

	// 检查PioSolver是否准备好
	ready, err := client.IsReady()
	if err != nil || !ready {
		log.Fatalf("PioSolver未准备好: %v", err)
	}

	//创建HandOrder实例
	handOrder = &cache.HandOrder{}
	boardOrder = &cache.BoardOrder{}

	// 初始化HandOrder
	if err := handOrder.Init(client); err != nil {
		log.Fatalf("初始化HandOrder失败: %v", err)
	}

	// 初始化BoardOrder
	if err := boardOrder.Init(); err != nil {
		log.Fatalf("初始化BoardOrder失败: %v", err)
	}

	// 加载树并保存CFR文件路径
	cfrFilePath = `D:\gto\piosolver3\saves\asth4d-allin-flops.cfr`
	_, err = client.LoadTree(cfrFilePath)
	if err != nil {
		log.Fatalf("加载树失败: %v", err)
	}

	// 创建输出目录
	err = os.MkdirAll("data", 0755)
	if err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	// 解析节点并生成JSON
	log.Println("开始解析节点并生成JSON...")
	parseNode(client, targetNode)
	log.Println("节点解析完成，JSON生成完毕")

	// 读取生成的JSON文件并统计有效record总数
	_, cfrFileName := filepath.Split(cfrFilePath)
	cfrFileName = strings.TrimSuffix(cfrFileName, filepath.Ext(cfrFileName))
	outputPath := filepath.Join("data", cfrFileName+".json")

	// 读取JSON文件
	fileData, err := os.ReadFile(outputPath)
	if err != nil {
		log.Printf("读取JSON文件失败: %v", err)
	} else {
		// 解析JSON数据
		var records []*model.Record
		err = json.Unmarshal(fileData, &records)
		if err != nil {
			log.Printf("解析JSON数据失败: %v", err)
		} else {
			// 统计总记录数和有效动作数
			totalActions := 0
			for _, record := range records {
				totalActions += len(record.Actions)
			}

			// 计算过滤比例
			totalOriginalActions := totalActions + filteredActionCount
			filterRatio := float64(filteredActionCount) / float64(totalOriginalActions) * 100

			fmt.Printf("\n\n==================================\n")
			fmt.Printf("【统计信息】共生成有效record %d 条，包含有效动作 %d 个\n", len(records), totalActions)
			fmt.Printf("【过滤信息】共过滤掉无效动作 %d 个 (占总数的 %.2f%%)\n", filteredActionCount, filterRatio)
			fmt.Printf("==================================\n\n")
		}
	}

	// 给程序时间响应
	time.Sleep(5 * time.Second)
}

// runCalcCommand 执行计算功能（新功能框架）
func runCalcCommand() {
	log.Println("==================================")
	log.Println("【计算功能】正在初始化...")
	log.Println("==================================")

	// TODO: 这里将实现PioSolver的计算功能
	// 可能包括：
	// 1. 连接PioSolver
	// 2. 设置计算参数
	// 3. 执行求解
	// 4. 监控计算进度
	// 5. 保存计算结果

	log.Println("初始化PioSolver客户端...")
	client := upi.NewClient("./PioSOLVER3-edge.exe", `D:\gto\piosolver3`)

	// 启动PioSolver
	if err := client.Start(); err != nil {
		log.Fatalf("启动PioSolver失败: %v", err)
	}
	defer client.Close()

	// 检查PioSolver是否准备好
	ready, err := client.IsReady()
	if err != nil || !ready {
		log.Fatalf("PioSolver未准备好: %v", err)
	}

	log.Println("PioSolver已就绪，准备执行计算任务...")
	
	// 获取公牌子集数据
	flopSubsets := cache.GetFlopSubsets()
	log.Printf("已加载 %d 个公牌组合", len(flopSubsets))
	
	// 模拟计算过程
	log.Println("开始执行计算...")
	log.Println("设置游戏参数...")
	log.Println("配置求解参数...")
	log.Println("启动求解进程...")
	
	// 演示使用公牌数据
	if len(flopSubsets) > 0 {
		log.Printf("第一个公牌组合示例: %s", flopSubsets[0])
		log.Printf("最后一个公牌组合示例: %s", flopSubsets[len(flopSubsets)-1])
	}
	
	// 这里可以添加实际的计算逻辑
	time.Sleep(2 * time.Second)
	
	log.Println("计算完成！")
	log.Println("==================================")
	log.Println("【计算功能】执行完毕")
	log.Println("==================================")
}

func parseNode(client *upi.Client, node string) {
	//show_node 获取当前节点信息，公牌，行动方（IP/OOP）
	cmd := fmt.Sprintf("show_node %s", node)
	responses, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Printf("执行指令失败: %v，跳过此节点", err)
		return
	}

	// 检查响应是否足够
	if len(responses) < 4 {
		log.Printf("响应数据不足，跳过此节点: %v", responses)
		return
	}

	actor := responses[1]
	board := responses[2]
	pot := responses[3]

	// 检查是否为终端节点（无子节点）
	childrenCount := "0"
	for _, resp := range responses {
		if strings.Contains(resp, "children") {
			parts := strings.Fields(resp)
			if len(parts) > 0 {
				childrenCount = parts[0]
			}
			break
		}
	}

	// 如果是终端节点，则不需要进一步处理
	if childrenCount == "0" {
		log.Printf("节点 %s 没有子节点，跳过进一步处理", node)
		return
	}

	//show_children 获取当前节点下的子节点，每一个子节点代表一个行动，与后续的show_strategy、每一行的结果对应
	cmd = fmt.Sprintf("show_children %s", node)
	responses, err = client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Printf("执行指令show_children失败: %v，跳过此节点", err)
		return
	}

	// 如果返回为空，表示没有子节点
	if len(responses) == 0 {
		log.Printf("节点 %s 返回空的子节点列表，跳过进一步处理", node)
		return
	}

	// 解析子节点信息,生成对应的action
	var children []model.ChildNode
	var actions []model.Action

	for i := 0; i < len(responses); i += 7 {
		// 确保有足够的行数
		if i+6 >= len(responses) {
			break
		}
		// 解析索引行，格式如 "child 0:"
		var index int
		_, err := fmt.Sscanf(responses[i], "child %d:", &index)
		if err != nil {
			log.Printf("解析子节点索引失败: %s, %v", responses[i], err)
			continue
		}

		// 创建ChildNode对象并填充数据
		child := model.ChildNode{
			Index:    index,
			NodeID:   responses[i+1], // 节点ID
			NodeType: responses[i+2], // 节点类型 IP_DEC/OOP_DEC/SPLIT_NODE
			Board:    responses[i+3], // 公牌
			PotInfo:  responses[i+4], // 底池信息
			ChildNum: responses[i+5], // 子节点数量
			Flag:     responses[i+6], // 标志
		}

		// 打印提取的子节点信息
		log.Printf("解析到子节点 %d: NodeID=%s, NodeType=%s, Board=%s, PotInfo=%s, Flag=%s",
			child.Index, child.NodeID, child.NodeType, child.Board, child.PotInfo, child.Flag)
		label, _ := util.BuildActionLabel(pot, child)
		action := model.Action{
			Label:       label,
			ChildNodeID: child.NodeID,
		}

		children = append(children, child)
		actions = append(actions, action)
	}

	// 如果没有解析到任何子节点，则返回
	if len(children) == 0 {
		log.Printf("节点 %s 没有解析到有效子节点，跳过进一步处理", node)
		return
	}

	//show_strategy 获取当前节点1326手牌各行动对应的策略频率，行动类别参考show_children的结果
	cmd = fmt.Sprintf("show_strategy %s", node)
	strategy_lines, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Printf("执行指令show_strategy失败: %v，尝试继续处理", err)
		// 不返回，继续尝试其他命令
	} else if len(strategy_lines) == 0 || strings.Contains(strategy_lines[0], "ERROR") {
		log.Printf("show_strategy返回错误或为空: %v", strategy_lines)
		// 不返回，继续尝试其他命令
	}

	// 获取所有手牌
	handCards := handOrder.Order()
	if len(handCards) != 1326 {
		log.Printf("手牌数量错误: %d，使用现有手牌继续", len(handCards))
	}

	// 创建一个映射，存储每个手牌的Record
	handRecords := make(map[string]*model.Record)

	// 先为每个手牌创建一个Record
	for _, hand := range handCards {
		handRecords[hand] = &model.Record{
			Node:    node,
			Actor:   actor,
			Board:   board,
			Hand:    hand,
			Actions: []model.Action{}, // 初始化空的Actions数组
		}
	}

	// 只有当strategy_lines有效时才处理策略频率
	if len(strategy_lines) > 0 && !strings.Contains(strategy_lines[0], "ERROR") {
		// 收集每个手牌在所有动作下的频率
		for i := 0; i < len(actions); i++ {
			if i >= len(strategy_lines) {
				log.Printf("警告: 动作数量 %d 超出策略行数 %d", len(actions), len(strategy_lines))
				break
			}

			sline := strategy_lines[i]
			sline_split := strings.Fields(sline) // 使用Fields代替Split，可以处理多个空格

			for j, hand := range handCards {
				if j >= len(sline_split) {
					continue // 防止数组越界
				}

				freq, err := strconv.ParseFloat(sline_split[j], 64)
				if err != nil {
					log.Printf("转换策略频率失败: %v，使用0.0替代", err)
					freq = 0.0
				}

				// 始终添加所有动作，无论频率是否为0
				// 复制action，并设置频率
				action := actions[i]
				action.Freq = freq
				action.ChildNodeID = children[i].NodeID

				// 添加到对应手牌的Record中
				record := handRecords[hand]
				record.Actions = append(record.Actions, action)
			}
		}

	} else {
		log.Printf("节点 %s 的策略数据无效，跳过策略处理", node)
	}

	//calc_ev 计算当前节点下1326手牌各行动的期望值,返回结果两行，只取第一行的ev值
	// 先根据actor生成actorCmd
	// actor如果是IP_DEC，则actorCmd为IP
	// actor如果是OOP_DEC，则actorCmd为OOP
	// actor如果是SPLIT_NODE，则RETURN
	var actorCmd string
	if actor == "IP_DEC" {
		actorCmd = "IP"
	} else if actor == "OOP_DEC" {
		actorCmd = "OOP"
	} else {
		log.Printf("节点 %s 的actor不是IP_DEC或OOP_DEC: %s，跳过EV和EQ计算", node, actor)
		// 这里不返回，因为我们可能已经有部分有用数据
	}

	// 只有当actorCmd有效时才计算EV
	if actorCmd != "" {
		// 遍历所有动作获取EV
		for i := 0; i < len(actions); i++ {
			action := actions[i]
			childNodeID := action.ChildNodeID

			// 计算当前动作的EV值
			cmd = fmt.Sprintf("calc_ev %s %s", actorCmd, childNodeID)
			ev_lines, err := client.ExecuteCommand(cmd, 10*time.Second)
			if err != nil {
				log.Printf("执行指令失败: %v，跳过当前动作", err)
				continue
			}

			// 检查响应是否合法
			if len(ev_lines) == 0 || strings.Contains(ev_lines[0], "ERROR") {
				log.Printf("calc_ev命令返回错误或无效响应: %v，跳过当前动作", ev_lines)
				continue
			}

			// 通常ev_lines的第一行包含所有手牌的EV值
			ev_line := ev_lines[0] // 使用第0行
			ev_split := strings.Fields(ev_line)

			//ev_lines的第二行包含所有手牌的match-up值
			matchup_line := ev_lines[1]
			matchup_split := strings.Fields(matchup_line)

			// 遍历所有手牌，添加EV值到对应的Action中
			for j, hand := range handCards {
				if j >= len(ev_split) {
					continue // 防止数组越界
				}

				// 解析EV值
				ev, err := strconv.ParseFloat(ev_split[j], 64)
				if err != nil || strings.Contains(strings.ToLower(ev_split[j]), "nan") {
					// 跳过解析失败或NaN的值
					continue
				}

				// 解析match-up值
				matchup, err := strconv.ParseFloat(matchup_split[j], 64)
				if err != nil || strings.Contains(strings.ToLower(matchup_split[j]), "nan") {
					// 跳过解析失败或NaN的值
					continue
				}

				// 在手牌的记录中查找对应的action并更新EV
				record := handRecords[hand]
				if record == nil {
					continue
				}

				// 查找action并更新EV
				for k := range record.Actions {
					// 确认是否为同一个action（通过比较ChildNodeID或其他唯一标识）
					if record.Actions[k].ChildNodeID == childNodeID {
						record.Actions[k].Ev = ev
						record.Actions[k].Matchup = matchup
						break
					}
				}
			}
		}

		//calc_eq_node 计算当前节点下1326手牌的胜率，只取第一行的eq值
		cmd = fmt.Sprintf("calc_eq_node %s %s", actorCmd, node)

		eq_lines, err := client.ExecuteCommand(cmd, 10*time.Second)
		if err != nil {
			log.Printf("执行指令calc_eq_node失败: %v，跳过EQ处理", err)
		} else if len(eq_lines) == 0 || strings.Contains(eq_lines[0], "ERROR") {
			log.Printf("calc_eq_node返回错误或为空: %v，跳过EQ处理", eq_lines)
		} else {
			//只读取第一行的数据
			eq_line := eq_lines[0]
			eq_split := strings.Fields(eq_line)

			// 按照handCards顺序为每个手牌设置EQ值
			for j, hand := range handCards {
				if j >= len(eq_split) {
					continue // 防止数组越界
				}

				// 跳过NaN值
				if strings.Contains(strings.ToLower(eq_split[j]), "nan") {
					continue
				}

				eq, err := strconv.ParseFloat(eq_split[j], 64)
				if err != nil {
					log.Printf("转换eq失败: %s, %v", eq_split[j], err)
					continue
				}

				record := handRecords[hand]
				if record == nil {
					continue
				}

				// 为所有action设置相同的EQ值
				for k := range record.Actions {
					record.Actions[k].Eq = eq
				}
			}
		}
	}

	// 过滤NaN值和空记录并按手牌顺序重建records
	var finalRecords []*model.Record
	for _, hand := range handCards {
		record := handRecords[hand]
		if record == nil {
			continue
		}

		// 过滤掉EV或EQ为0、NaN或Inf的Action，以及freq为0的Action
		var validActions []model.Action
		for _, action := range record.Actions {
			// 检查是否所有三个值(freq、ev、eq)都是无效值(0、NaN或Inf)
			freqIsInvalid := action.Freq == 0
			evIsInvalid := action.Ev == 0 || math.IsInf(action.Ev, 0) || math.IsNaN(action.Ev)
			eqIsInvalid := action.Eq == 0 || math.IsInf(action.Eq, 0) || math.IsNaN(action.Eq)

			// 检查ev*matchup是否等于0且不是fold动作
			evMultMatchupIsZero := action.Ev*action.Matchup == 0 && action.Label != "fold"

			// 只有当所有三个值都无效时或者ev*matchup=0（非fold）时才过滤
			if (freqIsInvalid && evIsInvalid && eqIsInvalid) || evMultMatchupIsZero {
				filteredActionCount++ // 增加过滤计数
				continue
			}

			validActions = append(validActions, action)
		}

		// 更新record的Actions
		record.Actions = validActions

		// 只有当有有效Action时才添加到finalRecords
		// 新增条件：如果只有一个action且为fold，也过滤掉
		if len(record.Actions) > 0 {
			// 过滤掉只有一个fold动作的record
			if len(record.Actions) == 1 && record.Actions[0].Label == "fold" {
				continue
			}
			finalRecords = append(finalRecords, record)
		}
	}

	// 打印JSON格式并写入到文件
	if len(finalRecords) > 0 {
		// 从CFR文件路径提取文件名
		_, cfrFileName := filepath.Split(cfrFilePath)
		cfrFileName = strings.TrimSuffix(cfrFileName, filepath.Ext(cfrFileName))

		// 构建输出文件路径
		outputJsonPath := filepath.Join("data", cfrFileName+".json")
		outputSqlPath := filepath.Join("data", cfrFileName+".sql")
		
		log.Printf("准备写入数据到文件: %s, 记录数: %d", outputJsonPath, len(finalRecords))

		// 检查输出目录是否存在，不存在则创建
		err = os.MkdirAll("data", 0755)
		if err != nil {
			log.Printf("创建输出目录失败: %v", err)
			return
		}

		// 判断是否为根节点(深度为1)
		isRootNode := strings.Count(node, ":") <= 1

		// 如果是根节点，则创建或覆盖文件
		if isRootNode {
			// 将所有记录序列化为JSON
			jsonData, err := json.MarshalIndent(finalRecords, "", "  ")
			if err != nil {
				log.Printf("JSON序列化失败: %v", err)
				return
			}

			// 创建或覆盖JSON文件
			err = os.WriteFile(outputJsonPath, jsonData, 0644)
			if err != nil {
				log.Printf("写入JSON文件失败: %v", err)
				return
			}

			// 创建SQL文件并写入表头
			sqlFile, err := os.Create(outputSqlPath)
			if err != nil {
				log.Printf("创建SQL文件失败: %v", err)
				return
			}
			defer sqlFile.Close()

			// 写入SQL文件头部
			sqlFile.WriteString("-- Generated SQL insert statements\n")
			sqlFile.WriteString(fmt.Sprintf("-- Total records: %d\n\n", len(finalRecords)))

			// 为每条记录生成SQL插入语句
			log.Printf("开始生成SQL语句，总记录数: %d", len(finalRecords))
			
			for _, record := range finalRecords {
				// 转换节点路径为标准格式
				nodePrefix := convertNodePath(record.Node)
				betLevel := calculateBetLevel(nodePrefix)
				
				// 标准化公牌顺序并获取board_id
				standardizedBoard := standardizeBoard(record.Board)
				boardId, ok := boardOrder.Index(standardizedBoard)
				if !ok {
					log.Printf("警告：无法找到公牌 %s (标准化后: %s) 的索引", record.Board, standardizedBoard)
					continue
				}
				
				// 计算bet_pct和spr
				betPct, spr := calculateBetMetrics(record.PotInfo)
				
				// 生成SQL插入语句
				sqlInsert := generateSQLInsert(record, nodePrefix, betLevel, boardId, record.Hand, betPct, spr)
				if sqlInsert != "" {
					if _, err := sqlFile.WriteString(sqlInsert); err != nil {
						log.Printf("写入SQL语句失败: %v", err)
					}
				}
			}

			log.Printf("SQL生成完成，正在关闭文件...")

			// 打印总结信息
			log.Printf("处理完成根节点 %s，数据已保存到文件中", node)
		} else {
			// 如果不是根节点，尝试读取现有文件
			var existingRecords []*model.Record

			fileData, err := os.ReadFile(outputJsonPath)
			if err == nil && len(fileData) > 0 {
				// 文件存在且不为空，尝试解析现有记录
				err = json.Unmarshal(fileData, &existingRecords)
				if err != nil {
					log.Printf("解析现有文件失败: %v，将创建新文件", err)
					existingRecords = []*model.Record{}
				}
			} else {
				// 文件不存在或为空，创建空记录数组
				existingRecords = []*model.Record{}
			}

			// 将新记录合并到现有记录中
			existingRecords = append(existingRecords, finalRecords...)

			// 序列化所有记录
			jsonData, err := json.MarshalIndent(existingRecords, "", "  ")
			if err != nil {
				log.Printf("JSON序列化失败: %v", err)
				return
			}

			// 写入合并后的记录
			err = os.WriteFile(outputJsonPath, jsonData, 0644)
			if err != nil {
				log.Printf("写入文件失败: %v", err)
				return
			}

			log.Printf("已更新文件数据: %s，大小: %d 字节", outputJsonPath, len(jsonData))
		}
	}

	//遍历子节点，递归调用解析，但是当子节点的类型为SPLIT_NODE时，不再递归调用
	for _, child := range children {
		if child.NodeType != "SPLIT_NODE" {
			// 递归处理子节点
			parseNode(client, child.NodeID)
		}
	}

	// 如果是根节点(深度为1)，关闭JSON数组
	if strings.Count(node, ":") <= 1 {
		// 打印总结信息
		log.Printf("处理完成根节点 %s，数据已保存到文件中", node)
	}
}

// 新增：转换节点路径为标准格式
func convertNodePath(path string) string {
	// 使用正则表达式匹配数字
	re := regexp.MustCompile(`[rb](\d+)`)
	// 将带数字的动作替换为单个字母
	return re.ReplaceAllString(path, "${1}")
}

// 新增：计算下注次数
func calculateBetLevel(nodePath string) int {
	// 统计路径中的b（bet）的次数
	return strings.Count(nodePath, "b")
}

// 修改：计算bet_pct和spr
func calculateBetMetrics(potInfo string) (float64, float64) {
	log.Printf("解析底池信息: %s", potInfo)

	// 默认值
	pot := 60.0  // 默认底池为60bb
	bet := 0.0   // 默认没有下注
	stack := 60.0 // 默认筹码为60bb

	// 尝试从potInfo中解析信息
	if potInfo != "" {
		parts := strings.Split(potInfo, "|")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			fields := strings.Fields(part)
			if len(fields) >= 2 {
				value, err := strconv.ParseFloat(fields[1], 64)
				if err == nil {
					switch fields[0] {
					case "pot":
						pot = value
					case "bet":
						bet = value
					case "stack":
						stack = value
					}
				}
			}
		}
	}

	// 计算bet_pct（下注占底池比例）
	betPct := 0.0
	if pot > 0 {
		betPct = bet / pot
	}

	// 计算spr（栈底比）
	spr := 0.0
	if pot > 0 {
		spr = stack / pot
	}

	log.Printf("计算结果: bet_pct=%.3f, spr=%.3f (pot=%.2f, bet=%.2f, stack=%.2f)",
		betPct, spr, pot, bet, stack)

	return betPct, spr
}

// 新增：生成SQL插入语句
func generateSQLInsert(record *model.Record, nodePrefix string, betLevel int, boardId int64, hand string, betPct float64, spr float64) string {
	// 确保至少有一个动作
	if len(record.Actions) == 0 {
		return ""
	}

	// 获取手牌的combo_id
	comboId, ok := handOrder.Index(record.Hand)
	if !ok {
		log.Printf("警告：无法找到手牌 %s 的索引", record.Hand)
		return ""
	}

	// 准备第一个动作的数据
	action1 := record.Actions[0]
	action1Label := action1.Label
	action1Freq := action1.Freq
	action1Ev := action1.Ev
	action1Eq := action1.Eq

	// 准备第二个动作的数据（如果存在）
	var action2Label string
	var action2Freq, action2Ev, action2Eq float64
	if len(record.Actions) > 1 {
		action2 := record.Actions[1]
		action2Label = action2.Label
		action2Freq = action2.Freq
		action2Ev = action2.Ev
		action2Eq = action2.Eq
	}

	// 生成INSERT语句
	sql := fmt.Sprintf("INSERT INTO flop_60bb_co_bb (node_prefix, bet_level, board_id, combo_id, bet_pct, spr, "+
		"action1, freq1, ev1, eq1, action2, freq2, ev2, eq2) VALUES "+
		"('%s', %d, %d, %d, %.3f, %.3f, '%s', %.3f, %.3f, %.3f, '%s', %.3f, %.3f, %.3f);\n",
		nodePrefix, betLevel, boardId, comboId, betPct, spr,
		action1Label, action1Freq, action1Ev, action1Eq,
		action2Label, action2Freq, action2Ev, action2Eq)

	return sql
}

// 新增：标准化公牌顺序
func standardizeBoard(board string) string {
	// 移除多余的空格
	board = strings.TrimSpace(board)
	
	// 分割成单张牌
	cards := strings.Fields(board)
	if len(cards) != 3 {
		return board // 如果不是3张牌，返回原始字符串
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
	
	// 重新组合成字符串
	return strings.Join(cards, " ")
}
