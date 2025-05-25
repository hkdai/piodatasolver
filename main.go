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

// 导出文件保存路径 - 方便修改
const exportSavePath = `D:\gto\piosolver3\saves\`

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
		fmt.Println("用法: go run main.go [parse|calc] [参数]")
		fmt.Println("  parse - 解析PioSolver数据并生成JSON/SQL文件")
		fmt.Println("  calc <路径> - 执行PioSolver批量计算功能")
		fmt.Println("    例如: go run main.go calc \"D:\\gto\\piosolver3\\TreeBuilding\\mtt\\40bb\"")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "parse":
		log.Println("执行解析功能...")
		runParseCommand()
	case "calc":
		if len(os.Args) < 3 {
			fmt.Println("错误: calc命令需要指定脚本路径")
			fmt.Println("用法: go run main.go calc <脚本路径>")
			fmt.Println("例如: go run main.go calc \"D:\\gto\\piosolver3\\TreeBuilding\\mtt\\40bb\"")
			os.Exit(1)
		}
		scriptPath := os.Args[2]
		log.Printf("执行计算功能，脚本路径: %s", scriptPath)
		runCalcCommand(scriptPath)
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
	cfrFilePath = exportSavePath + `asth4d-allin-flops.cfr`
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

// runCalcCommand 执行批量计算功能
func runCalcCommand(scriptPath string) {
	log.Println("==================================")
	log.Println("【批量计算功能】正在初始化...")
	log.Printf("脚本路径: %s", scriptPath)
	log.Println("==================================")

	// 检查脚本路径是否存在
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		log.Fatalf("脚本路径不存在: %s", scriptPath)
	}

	// 获取路径最后一个文件夹名称作为前缀
	pathPrefix := filepath.Base(scriptPath)
	log.Printf("文件名前缀: %s", pathPrefix)

	// 读取脚本文件
	scriptFiles, err := readScriptFiles(scriptPath)
	if err != nil {
		log.Fatalf("读取脚本文件失败: %v", err)
	}

	log.Printf("找到 %d 个脚本文件", len(scriptFiles))
	for i, file := range scriptFiles {
		log.Printf("  %d. %s", i+1, file)
	}

	// 获取公牌子集数据
	allFlopSubsets := cache.GetFlopSubsets()
	// 使用所有公牌组合
	flopSubsets := allFlopSubsets
	log.Printf("已加载 %d 个公牌组合 (生产模式，处理全部公牌)", len(flopSubsets))

	// 开始批量处理
	totalTasks := len(scriptFiles) * len(flopSubsets)
	currentTask := 0

	// 时间统计变量
	var totalTime time.Duration = 0
	var completedTasks int = 0

	log.Printf("总任务数: %d (脚本文件: %d × 公牌组合: %d)", totalTasks, len(scriptFiles), len(flopSubsets))

	// 遍历脚本文件
	for _, scriptFile := range scriptFiles {
		scriptName := getScriptName(scriptFile)
		log.Printf("\n处理脚本文件: %s", scriptName)

		// 读取脚本内容
		scriptContent, err := readScriptContent(scriptFile)
		if err != nil {
			log.Printf("读取脚本内容失败: %v，跳过此文件", err)
			continue
		}

		// 遍历公牌组合
		for flopIndex, flop := range flopSubsets {
			currentTask++
			flopProgress := flopIndex + 1 // 从1开始计数

			// 记录任务开始时间
			taskStartTime := time.Now()

			// 计算平均时间显示
			var avgTimeStr string
			if completedTasks > 0 {
				avgTime := totalTime / time.Duration(completedTasks)
				avgTimeStr = fmt.Sprintf(", 平均用时: %v", avgTime.Round(time.Second))
			} else {
				avgTimeStr = ""
			}

			log.Printf("\n[%d/%d] 处理脚本: %s, 公牌: %s (%d/%d)%s", currentTask, totalTasks, scriptName, flop, flopProgress, len(flopSubsets), avgTimeStr)

			// 为每个任务创建新的PioSolver实例
			log.Printf("  → 启动新的PioSolver实例... (%d/%d)", flopProgress, len(flopSubsets))
			client := upi.NewClient("./PioSOLVER3-edge.exe", `D:\gto\piosolver3`)

			// 启动PioSolver
			if err := client.Start(); err != nil {
				log.Printf("  ❌ 启动PioSolver失败: %v，跳过此任务 (%d/%d)", err, flopProgress, len(flopSubsets))
				continue
			}

			// 检查PioSolver是否准备好
			ready, err := client.IsReady()
			if err != nil || !ready {
				log.Printf("  ❌ PioSolver未准备好: %v，跳过此任务 (%d/%d)", err, flopProgress, len(flopSubsets))
				client.Close()
				continue
			}

			log.Printf("  ✓ PioSolver实例就绪 (%d/%d)", flopProgress, len(flopSubsets))

			// 处理单个任务（计算+导出）
			err = processSingleTask(client, scriptContent, scriptName, flop, pathPrefix, flopProgress, len(flopSubsets))

			// 关闭PioSolver实例
			log.Printf("  → 关闭PioSolver实例... (%d/%d)", flopProgress, len(flopSubsets))
			client.Close()

			// 计算任务用时并更新统计
			taskDuration := time.Since(taskStartTime)

			if err != nil {
				log.Printf("  ❌ 处理任务失败: %v (%d/%d)", err, flopProgress, len(flopSubsets))
			} else {
				// 更新时间统计
				totalTime += taskDuration
				completedTasks++

				// 计算新的平均时间
				avgTime := totalTime / time.Duration(completedTasks)

				log.Printf("  ✓ [%d/%d] 任务完成: %s_%s (%d/%d) [用时: %v, 平均: %v]",
					currentTask, totalTasks, scriptName, flop, flopProgress, len(flopSubsets),
					taskDuration.Round(time.Second), avgTime.Round(time.Second))
			}
		}
	}

	log.Println("\n==================================")
	log.Println("【批量计算功能】全部完成！")
	if completedTasks > 0 {
		avgTime := totalTime / time.Duration(completedTasks)
		log.Printf("成功处理 %d 个任务，总用时: %v，平均用时: %v",
			completedTasks, totalTime.Round(time.Second), avgTime.Round(time.Second))
	} else {
		log.Printf("处理了 %d 个任务", totalTasks)
	}
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
	pot := 60.0   // 默认底池为60bb
	bet := 0.0    // 默认没有下注
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

// readScriptFiles 读取指定路径下的所有脚本文件
func readScriptFiles(scriptPath string) ([]string, error) {
	var scriptFiles []string

	// 读取目录下的所有文件
	files, err := os.ReadDir(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	// 过滤出脚本文件（.txt文件）
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".txt") {
			fullPath := filepath.Join(scriptPath, fileName)
			scriptFiles = append(scriptFiles, fullPath)
		}
	}

	if len(scriptFiles) == 0 {
		return nil, fmt.Errorf("在路径 %s 下未找到任何 .txt 脚本文件", scriptPath)
	}

	return scriptFiles, nil
}

// getScriptName 从完整路径中提取脚本文件名（不含扩展名）
func getScriptName(scriptPath string) string {
	fileName := filepath.Base(scriptPath)
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

// readScriptContent 读取脚本文件内容
func readScriptContent(scriptPath string) (string, error) {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	return string(content), nil
}

// replaceSetBoard 替换脚本中的set_board命令
func replaceSetBoard(scriptContent, flop string) string {
	// 使用正则表达式匹配set_board命令并替换
	setBoardRegex := regexp.MustCompile(`(?m)^set_board\s+.*$`)
	newSetBoard := fmt.Sprintf("set_board %s", flop)
	return setBoardRegex.ReplaceAllString(scriptContent, newSetBoard)
}

// processSingleTask 处理单个计算任务
func processSingleTask(client *upi.Client, scriptContent, scriptName, flop, pathPrefix string, flopProgress, totalFlops int) error {
	log.Printf("  → 开始执行任务... (%d/%d)", flopProgress, totalFlops)

	log.Printf("  → 替换set_board命令为: set_board %s (%d/%d)", flop, flopProgress, totalFlops)

	// 替换脚本中的set_board命令
	modifiedScript := replaceSetBoard(scriptContent, flop)

	// 将修改后的脚本按行分割
	scriptLines := strings.Split(modifiedScript, "\n")

	log.Printf("  → 执行脚本命令 (%d 行)", len(scriptLines))

	// 逐行执行脚本命令
	executedCount := 0
	for _, line := range scriptLines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // 跳过空行和注释
		}

		// 执行命令
		_, err := client.ExecuteCommand(line, 30*time.Second)
		if err != nil {
			return fmt.Errorf("执行命令失败 '%s': %v", line, err)
		}

		executedCount++
	}

	log.Printf("  ✓ 脚本执行完成，共执行 %d 条命令 (%d/%d)", executedCount, flopProgress, totalFlops)

	log.Printf("  → 确保设置正确的精度...")

	// 在执行go命令之前，确保设置正确的精度
	accuracyResponses, err := client.ExecuteCommand("set_accuracy 0.12", 5*time.Second)
	if err != nil {
		log.Printf("  警告：设置精度失败: %v", err)
	} else {
		for _, response := range accuracyResponses {
			log.Printf("  精度设置响应: %s", response)
		}
	}

	log.Printf("  → 执行go命令启动计算... (%d/%d)", flopProgress, totalFlops)

	// 使用专门的方法执行go命令，获取实时输出流
	outputChan, errChan, err := client.ExecuteGoCommandWithStream()
	if err != nil {
		return fmt.Errorf("执行go命令失败: %v", err)
	}

	log.Printf("  → 计算已启动，开始监听PioSolver输出... (%d/%d)", flopProgress, totalFlops)

	// 等待计算完成，使用实时输出流
	err = waitForCalculationCompleteWithStream(outputChan, errChan)
	if err != nil {
		return fmt.Errorf("等待计算完成失败: %v", err)
	}

	// 简短等待让stream完全停止
	log.Printf("  → 等待输出流停止... (%d/%d)", flopProgress, totalFlops)
	time.Sleep(1 * time.Second)

	log.Printf("  ✓ 计算完成，开始导出... (%d/%d)", flopProgress, totalFlops)

	// 生成导出文件名
	outputFileName := fmt.Sprintf("%s_%s_%s.cfr", pathPrefix, scriptName, flop)
	outputPath := fmt.Sprintf(`%s%s`, exportSavePath, outputFileName)

	log.Printf("  → 导出文件: %s (%d/%d)", outputFileName, flopProgress, totalFlops)

	// 直接发送导出命令，不等待响应
	dumpCmd := fmt.Sprintf(`dump_tree "%s" no_rivers `, outputPath)
	log.Printf("  → 执行导出命令: %s (%d/%d)", dumpCmd, flopProgress, totalFlops)

	// 直接发送命令，不使用ExecuteCommand以避免等待响应
	_, err = fmt.Fprintln(client.GetStdin(), dumpCmd)
	if err != nil {
		log.Printf("  ❌ 发送导出命令失败: %v (%d/%d)", err, flopProgress, totalFlops)
		return fmt.Errorf("发送导出命令失败: %v", err)
	}

	// 等待一点时间让导出命令执行，但不等待响应
	time.Sleep(2 * time.Second)

	log.Printf("  ✓ 导出命令已发送: %s (%d/%d)", outputFileName, flopProgress, totalFlops)

	return nil
}

// waitForCalculationCompleteWithStream 通过实时输出流等待计算完成
func waitForCalculationCompleteWithStream(outputChan <-chan string, errChan <-chan error) error {
	log.Printf("    监控PioSolver实时输出...")

	maxWaitTime := 30 * time.Minute     // 最长等待30分钟
	noOutputTimeout := 30 * time.Second // 如果30秒没有输出，认为计算完成

	startTime := time.Now()
	lastOutputTime := time.Now()
	goOkFound := false

	for {
		select {
		case line, ok := <-outputChan:
			if !ok {
				// 输出通道关闭，PioSolver进程结束
				log.Printf("    ✓ PioSolver进程结束，计算完成")
				return nil
			}

			// 更新最后输出时间
			lastOutputTime = time.Now()
			elapsed := time.Since(startTime)

			// 检查go命令启动确认
			if strings.Contains(line, "go ok!") {
				goOkFound = true
				log.Printf("    PioSolver: %s - 计算已启动", line)
				continue
			}

			// 如果还没有看到go ok!，继续等待
			if !goOkFound {
				log.Printf("    PioSolver: %s", line)
				continue
			}

			// 过滤并显示重要的计算信息
			if strings.Contains(line, "running time:") ||
				strings.Contains(line, "EV OOP:") ||
				strings.Contains(line, "EV IP:") ||
				strings.Contains(line, "Exploitable for:") ||
				strings.Contains(line, "SOLVER:") {
				log.Printf("    PioSolver: %s (用时: %v)", line, elapsed.Round(time.Second))

				// 检查计算完成的信号
				if strings.Contains(line, "SOLVER: stopped (required accuracy reached)") {
					log.Printf("    ✓ 检测到计算完成信号！")
					return nil
				}
				if strings.Contains(line, "SOLVER: stopped") && !strings.Contains(line, "started") {
					log.Printf("    ✓ 检测到求解器停止！")
					return nil
				}

				// 检测可剥削值 - 保持严格的精度要求
				if strings.Contains(line, "Exploitable for:") {
					parts := strings.Fields(line)
					if len(parts) >= 3 {
						exploitableStr := parts[2]
						if exploitable, err := strconv.ParseFloat(exploitableStr, 64); err == nil {
							log.Printf("    → 当前可剥削值: %.6f (目标: ≤0.12)", exploitable)
							// 保持严格的精度要求：可剥削值小于等于0.12
							if exploitable <= 0.12 {
								log.Printf("    ✓ 可剥削值 %.6f 达到精度要求，计算完成！", exploitable)
								return nil
							}
						}
					}
				}
			}

		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("读取PioSolver输出时出错: %v", err)
			}

		case <-time.After(1 * time.Second):
			// 定期检查超时条件
			elapsed := time.Since(startTime)

			// 检查总超时时间
			if elapsed > maxWaitTime {
				return fmt.Errorf("计算超时，超过最大等待时间 %v", maxWaitTime)
			}

			// 检查是否长时间没有输出
			if time.Since(lastOutputTime) > noOutputTimeout {
				log.Printf("    ✓ 长时间无输出，认为计算已完成（无输出时间: %v）", time.Since(lastOutputTime).Round(time.Second))
				return nil
			}

			// 每30秒显示一次进度
			if int(elapsed.Seconds())%30 == 0 && goOkFound {
				log.Printf("    计算进行中... (已用时: %v)", elapsed.Round(time.Second))
			}
		}
	}
}

// waitForCalculationComplete 等待计算完成（保留原方法作为备用）
func waitForCalculationComplete(client *upi.Client) error {
	log.Printf("    监控PioSolver计算日志...")

	maxWaitTime := 30 * time.Minute       // 最长等待30分钟
	checkInterval := 2 * time.Second      // 每2秒检查一次
	noResponseTimeout := 30 * time.Second // 如果30秒没有响应，认为计算完成

	startTime := time.Now()
	lastResponseTime := time.Now()
	consecutiveCalculatingCount := 0 // 连续"计算中..."的计数器

	for {
		// 检查是否超时
		if time.Since(startTime) > maxWaitTime {
			return fmt.Errorf("计算超时，超过最大等待时间 %v", maxWaitTime)
		}

		// 检查是否长时间没有响应（可能计算已完成）
		if time.Since(lastResponseTime) > noResponseTimeout {
			log.Printf("    ✓ 长时间无响应，认为计算已完成（无响应时间: %v）", time.Since(lastResponseTime).Round(time.Second))
			return nil
		}

		// 使用show_memory命令获取计算状态
		responses, err := client.ExecuteCommand("show_memory", 3*time.Second)
		if err != nil {
			elapsed := time.Since(startTime)
			consecutiveCalculatingCount++
			log.Printf("    计算中... (已用时: %v, 连续%d次)", elapsed.Round(time.Second), consecutiveCalculatingCount)

			// 当连续出现"计算中..."五次以上时，主动查询当前精度
			if consecutiveCalculatingCount >= 5 {
				log.Printf("    → 连续%d次显示计算中，主动查询当前计算精度...", consecutiveCalculatingCount)

				// 尝试使用不同的命令查询状态
				statusResponses, statusErr := client.ExecuteCommand("show_memory", 5*time.Second)
				if statusErr == nil {
					for _, response := range statusResponses {
						response = strings.TrimSpace(response)
						if response == "" {
							continue
						}

						log.Printf("    状态查询: %s", response)

						// 检测可剥削值
						if strings.Contains(response, "Exploitable for:") {
							parts := strings.Fields(response)
							if len(parts) >= 3 {
								exploitableStr := parts[2]
								if exploitable, err := strconv.ParseFloat(exploitableStr, 64); err == nil {
									log.Printf("    → 当前可剥削值: %.6f (目标: ≤0.12)", exploitable)
									// 保持原来的精度要求：小于等于0.12
									if exploitable <= 0.12 {
										log.Printf("    ✓ 可剥削值 %.6f 达到精度要求，计算完成！", exploitable)
										return nil
									}
									// 重置计数器，因为我们获得了有效的状态信息
									consecutiveCalculatingCount = 0
									lastResponseTime = time.Now()
								}
							}
						}

						// 检查其他完成信号
						if strings.Contains(response, "SOLVER: stopped (required accuracy reached)") {
							log.Printf("    ✓ 检测到计算完成信号！")
							return nil
						}
						if strings.Contains(response, "SOLVER: stopped") && !strings.Contains(response, "started") {
							log.Printf("    ✓ 检测到求解器停止！")
							return nil
						}
					}
				} else {
					log.Printf("    状态查询失败: %v", statusErr)
				}
			}
		} else {
			// 显示经过时间
			elapsed := time.Since(startTime)

			// 过滤并显示有用的PioSolver计算信息
			hasValidResponse := false
			for _, response := range responses {
				response = strings.TrimSpace(response)
				if response == "" {
					continue
				}

				// 只显示计算相关的重要信息
				if strings.Contains(response, "running time:") ||
					strings.Contains(response, "EV OOP:") ||
					strings.Contains(response, "EV IP:") ||
					strings.Contains(response, "Exploitable for:") ||
					strings.Contains(response, "SOLVER:") {
					log.Printf("    PioSolver: %s", response)
					hasValidResponse = true
					lastResponseTime = time.Now()   // 更新最后响应时间
					consecutiveCalculatingCount = 0 // 重置计数器

					// 检查计算完成的信号
					if strings.Contains(response, "SOLVER: stopped (required accuracy reached)") {
						log.Printf("    ✓ 检测到计算完成信号！")
						return nil
					}
					if strings.Contains(response, "SOLVER: stopped") && !strings.Contains(response, "started") {
						log.Printf("    ✓ 检测到求解器停止！")
						return nil
					}
					// 检测可剥削值 - 保持原来的精度要求
					if strings.Contains(response, "Exploitable for:") {
						parts := strings.Fields(response)
						if len(parts) >= 3 {
							exploitableStr := parts[2]
							if exploitable, err := strconv.ParseFloat(exploitableStr, 64); err == nil {
								// 保持严格的精度要求：可剥削值小于等于0.12
								if exploitable <= 0.12 {
									log.Printf("    ✓ 可剥削值 %.6f 达到精度要求，计算完成！", exploitable)
									return nil
								}
							}
						}
					}
				}
			}

			// 如果没有有效的计算信息，只显示时间
			if !hasValidResponse {
				consecutiveCalculatingCount++
				log.Printf("    计算中... (已用时: %v, 连续%d次)", elapsed.Round(time.Second), consecutiveCalculatingCount)
			}
		}

		// 等待下次检查
		time.Sleep(checkInterval)
	}
}
