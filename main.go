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

// CFR文件路径 - 用于生成输出文件名（在处理过程中动态设置）
var cfrFilePath string

// PioSolver相关路径配置 - 方便修改
const (
	pioSolverExePath = "./PioSOLVER3-edge.exe"                  // PioSolver可执行文件路径
	pioSolverWorkDir = `E:\zdsbddz\piosolver\piosolver3\`       // PioSolver工作目录
	exportSavePath   = `E:\zdsbddz\piosolver\piosolver3\saves\` // 导出文件保存路径
)

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
		fmt.Println("用法: piodatasolver.exe [parse|calc] [参数]")
		fmt.Println("  parse <CFR文件夹路径> - 解析指定文件夹下的所有CFR文件并生成JSON/SQL文件")
		fmt.Println("    例如: piodatasolver.exe parse \"E:\\zdsbddz\\piosolver\\piosolver3\\saves\"")
		fmt.Println("  calc <脚本路径> - 执行PioSolver批量计算功能")
		fmt.Println("    例如: piodatasolver.exe calc \"D:\\gto\\piosolver3\\TreeBuilding\\mtt\\40bb\"")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "parse":
		if len(os.Args) < 3 {
			fmt.Println("错误: parse命令需要指定CFR文件夹路径")
			fmt.Println("用法: piodatasolver.exe parse <CFR文件夹路径>")
			fmt.Println("例如: piodatasolver.exe parse \"E:\\zdsbddz\\piosolver\\piosolver3\\saves\"")
			os.Exit(1)
		}
		cfrFolderPath := os.Args[2]
		log.Printf("执行解析功能，CFR文件夹路径: %s", cfrFolderPath)
		runParseCommand(cfrFolderPath)
	case "calc":
		if len(os.Args) < 3 {
			fmt.Println("错误: calc命令需要指定脚本路径")
			fmt.Println("用法: piodatasolver.exe calc <脚本路径>")
			fmt.Println("例如: piodatasolver.exe calc \"D:\\gto\\piosolver3\\TreeBuilding\\mtt\\40bb\"")
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

// getEffectiveStack 获取当前树的有效起始筹码
func getEffectiveStack(client *upi.Client) (float64, error) {
	responses, err := client.ExecuteCommand("show_effective_stack", 10*time.Second)
	if err != nil {
		return 0, fmt.Errorf("执行show_effective_stack命令失败: %v", err)
	}

	if len(responses) == 0 {
		return 0, fmt.Errorf("show_effective_stack返回空响应")
	}

	// 解析第一行响应，应该是一个数值
	stackStr := strings.TrimSpace(responses[0])
	stack, err := strconv.ParseFloat(stackStr, 64)
	if err != nil {
		return 0, fmt.Errorf("解析有效筹码失败: %s, %v", stackStr, err)
	}

	return stack, nil
}

// runParseCommand 执行解析功能，处理指定文件夹下的所有CFR文件
func runParseCommand(cfrFolderPath string) {
	log.Println("==================================")
	log.Println("【批量解析功能】正在初始化...")
	log.Printf("CFR文件夹路径: %s", cfrFolderPath)
	log.Println("==================================")

	// 检查CFR文件夹路径是否存在
	if _, err := os.Stat(cfrFolderPath); os.IsNotExist(err) {
		log.Fatalf("CFR文件夹路径不存在: %s", cfrFolderPath)
	}

	// 读取文件夹下的所有CFR文件
	cfrFiles, err := readCfrFiles(cfrFolderPath)
	if err != nil {
		log.Fatalf("读取CFR文件失败: %v", err)
	}

	log.Printf("找到 %d 个CFR文件", len(cfrFiles))
	for i, file := range cfrFiles {
		log.Printf("  %d. %s", i+1, filepath.Base(file))
	}

	// 创建输出目录
	err = os.MkdirAll("data", 0755)
	if err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	// 启动PioSolver
	client := upi.NewClient(pioSolverExePath, pioSolverWorkDir)
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

	// 设置目标节点
	targetNode := "r:0"

	// 检查已存在的解析结果文件
	log.Println("\n==================================")
	log.Println("【检查已存在的解析结果】")
	existingResults, err := checkExistingParseResults()
	if err != nil {
		log.Fatalf("检查已存在解析结果失败: %v", err)
	}

	// 统计需要处理的任务
	totalFiles := len(cfrFiles)
	skippedFiles := 0
	currentFile := 0

	// 预先统计会跳过多少文件
	for _, cfrFile := range cfrFiles {
		_, cfrFileName := filepath.Split(cfrFile)
		cfrFileName = strings.TrimSuffix(cfrFileName, filepath.Ext(cfrFileName))
		jsonFileName := cfrFileName + ".json"
		sqlFileName := cfrFileName + ".sql"

		if existingResults[jsonFileName] && existingResults[sqlFileName] {
			skippedFiles++
		}
	}

	actualFiles := totalFiles - skippedFiles
	log.Printf("总CFR文件数: %d，已解析: %d，需要处理: %d", totalFiles, skippedFiles, actualFiles)
	log.Println("==================================")

	if actualFiles == 0 {
		log.Println("🎉 所有CFR文件都已解析完成，无需重新处理！")
		return
	}

	// 循环处理每个CFR文件
	for i, cfrFile := range cfrFiles {
		currentFile = i + 1

		// 检查文件是否已经解析过
		_, cfrFileName := filepath.Split(cfrFile)
		cfrFileName = strings.TrimSuffix(cfrFileName, filepath.Ext(cfrFileName))
		jsonFileName := cfrFileName + ".json"
		sqlFileName := cfrFileName + ".sql"

		if existingResults[jsonFileName] && existingResults[sqlFileName] {
			log.Printf("\n[%d/%d] ⏭️  跳过已解析: %s (JSON和SQL文件已存在)", currentFile, totalFiles, filepath.Base(cfrFile))
			continue
		}

		log.Printf("\n[%d/%d] 🚀 开始处理CFR文件: %s", currentFile, totalFiles, filepath.Base(cfrFile))

		// 重置过滤计数器
		filteredActionCount = 0

		// 设置全局CFR文件路径
		cfrFilePath = cfrFile

		// 加载树
		_, err = client.LoadTree(cfrFilePath)
		if err != nil {
			log.Printf("  ❌ 加载树失败: %v，跳过此文件", err)
			continue
		}

		log.Printf("  ✓ CFR文件加载成功")

		// 获取有效筹码
		log.Printf("  → 获取有效筹码...")
		effectiveStack, err := getEffectiveStack(client)
		if err != nil {
			log.Printf("  ❌ 获取有效筹码失败: %v，使用默认值60bb", err)
			effectiveStack = 60.0
		} else {
			log.Printf("  ✓ 有效筹码: %.2f bb", effectiveStack)
		}

		// 解析节点并生成JSON
		log.Printf("  → 开始解析节点并生成JSON...")
		parseNode(client, targetNode, effectiveStack)
		log.Printf("  ✓ 节点解析完成")

		// 读取生成的JSON文件并统计有效record总数
		_, cfrFileNameForOutput := filepath.Split(cfrFilePath)
		cfrFileNameForOutput = strings.TrimSuffix(cfrFileNameForOutput, filepath.Ext(cfrFileNameForOutput))
		outputPath := filepath.Join("data", cfrFileNameForOutput+".json")

		// 读取JSON文件
		fileData, err := os.ReadFile(outputPath)
		if err != nil {
			log.Printf("  ❌ 读取JSON文件失败: %v", err)
		} else {
			// 解析JSON数据
			var records []*model.Record
			err = json.Unmarshal(fileData, &records)
			if err != nil {
				log.Printf("  ❌ 解析JSON数据失败: %v", err)
			} else {
				// 统计总记录数和有效动作数
				totalActions := 0
				for _, record := range records {
					totalActions += len(record.Actions)
				}

				// 计算过滤比例
				totalOriginalActions := totalActions + filteredActionCount
				filterRatio := float64(filteredActionCount) / float64(totalOriginalActions) * 100

				log.Printf("  ✓ [%d/%d] 文件处理完成: %s", currentFile, totalFiles, filepath.Base(cfrFile))
				log.Printf("    📊 生成有效record %d 条，包含有效动作 %d 个", len(records), totalActions)
				log.Printf("    🗑️  过滤掉无效动作 %d 个 (占总数的 %.2f%%)", filteredActionCount, filterRatio)
			}
		}
	}

	log.Println("\n==================================")
	log.Println("【批量解析功能】全部完成！")
	log.Printf("📊 总共处理了 %d 个CFR文件", totalFiles)
	log.Println("==================================")

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

	// 检查已存在的文件
	log.Println("\n==================================")
	log.Println("【检查已存在文件】")
	existingFiles, err := checkExistingFiles()
	if err != nil {
		log.Fatalf("检查已存在文件失败: %v", err)
	}

	// 统计需要处理的任务
	totalTasks := len(scriptFiles) * len(flopSubsets)
	skippedTasks := 0
	currentTask := 0

	// 预先统计会跳过多少任务
	for _, scriptFile := range scriptFiles {
		scriptName := getScriptName(scriptFile)
		for _, flop := range flopSubsets {
			taskFileName := generateTaskFileName(pathPrefix, scriptName, flop)
			if existingFiles[taskFileName] {
				skippedTasks++
			}
		}
	}

	actualTasks := totalTasks - skippedTasks
	log.Printf("总任务数: %d，已完成: %d，需要处理: %d", totalTasks, skippedTasks, actualTasks)
	log.Println("==================================")

	if actualTasks == 0 {
		log.Println("🎉 所有任务都已完成，无需重新计算！")
		return
	}

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

			// 检查文件是否已存在
			taskFileName := generateTaskFileName(pathPrefix, scriptName, flop)
			if existingFiles[taskFileName] {
				log.Printf("\n[%d/%d] ⏭️  跳过已存在: %s, 公牌: %s (%d/%d)", currentTask, totalTasks, scriptName, flop, flopProgress, len(flopSubsets))
				continue
			}

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

			log.Printf("\n[%d/%d] 🚀 开始计算: %s, 公牌: %s (%d/%d)%s", currentTask, totalTasks, scriptName, flop, flopProgress, len(flopSubsets), avgTimeStr)

			// 为每个任务创建新的PioSolver实例
			log.Printf("  → 启动新的PioSolver实例... (%d/%d)", flopProgress, len(flopSubsets))
			client := upi.NewClient(pioSolverExePath, pioSolverWorkDir)

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
	log.Printf("📊 任务统计:")
	log.Printf("   总任务数: %d", totalTasks)
	log.Printf("   已跳过: %d (文件已存在)", skippedTasks)
	log.Printf("   新完成: %d", completedTasks)
	if completedTasks > 0 {
		avgTime := totalTime / time.Duration(completedTasks)
		log.Printf("   总用时: %v，平均用时: %v", totalTime.Round(time.Second), avgTime.Round(time.Second))
	}
	log.Println("==================================")
}

func parseNode(client *upi.Client, node string, effectiveStack float64) {
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

	// 计算当前节点的bet_pct、spr和stack_depth
	betPct, spr, stackDepth := calculateBetMetrics(pot, node, effectiveStack)

	// 创建一个映射，存储每个手牌的Record
	handRecords := make(map[string]*model.Record)

	// 先为每个手牌创建一个Record
	for _, hand := range handCards {
		handRecords[hand] = &model.Record{
			Node:       node,
			Actor:      actor,
			Board:      board,
			Hand:       hand,
			Actions:    []model.Action{}, // 初始化空的Actions数组
			PotInfo:    pot,              // 设置底池信息
			StackDepth: stackDepth,       // 设置筹码深度
			Spr:        spr,              // 设置栈底比
			BetPct:     betPct,           // 设置下注比例
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

		// 处理JSON文件：根节点创建新文件，子节点追加到现有文件
		var allRecords []*model.Record
		if isRootNode {
			// 根节点：创建新的JSON文件
			allRecords = finalRecords
		} else {
			// 子节点：读取现有文件并追加新记录
			fileData, err := os.ReadFile(outputJsonPath)
			if err == nil && len(fileData) > 0 {
				// 文件存在且不为空，尝试解析现有记录
				err = json.Unmarshal(fileData, &allRecords)
				if err != nil {
					log.Printf("解析现有JSON文件失败: %v，将创建新文件", err)
					allRecords = []*model.Record{}
				}
			} else {
				// 文件不存在或为空，创建空记录数组
				allRecords = []*model.Record{}
			}
			// 将新记录追加到现有记录中
			allRecords = append(allRecords, finalRecords...)
		}

		// 序列化所有记录并写入JSON文件
		jsonData, err := json.MarshalIndent(allRecords, "", "  ")
		if err != nil {
			log.Printf("JSON序列化失败: %v", err)
			return
		}

		err = os.WriteFile(outputJsonPath, jsonData, 0644)
		if err != nil {
			log.Printf("写入JSON文件失败: %v", err)
			return
		}

		// 处理SQL文件：根节点创建新文件，子节点追加到现有文件
		var sqlFile *os.File
		if isRootNode {
			// 根节点：创建新的SQL文件
			sqlFile, err = os.Create(outputSqlPath)
			if err != nil {
				log.Printf("创建SQL文件失败: %v", err)
				return
			}
			// 写入SQL文件头部
			sqlFile.WriteString("-- Generated SQL insert statements\n")
			sqlFile.WriteString(fmt.Sprintf("-- CFR File: %s\n", filepath.Base(cfrFilePath)))
			sqlFile.WriteString(fmt.Sprintf("-- Total records will be added incrementally\n\n"))
		} else {
			// 子节点：以追加模式打开SQL文件
			sqlFile, err = os.OpenFile(outputSqlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Printf("打开SQL文件失败: %v", err)
				return
			}
		}
		defer sqlFile.Close()

		// 为当前节点的所有记录生成SQL插入语句
		log.Printf("开始生成SQL语句，当前节点记录数: %d", len(finalRecords))

		// 统计变量
		var (
			totalProcessed   = 0
			boardIndexFailed = 0
			handIndexFailed  = 0
			sqlGenerated     = 0
			sqlWriteFailed   = 0
		)

		for _, record := range finalRecords {
			totalProcessed++

			// 转换节点路径为标准格式
			nodePrefix := convertNodePath(record.Node)
			betLevel := calculateBetLevel(nodePrefix)

			// 标准化公牌顺序并获取board_id
			standardizedBoard := standardizeBoard(record.Board)
			boardId, ok := boardOrder.Index(standardizedBoard)
			if !ok {
				boardIndexFailed++
				log.Printf("警告：无法找到公牌 %s (标准化后: %s) 的索引", record.Board, standardizedBoard)
				continue
			}

			// 生成SQL插入语句（使用Record中已计算的值）
			sqlInsert := generateSQLInsert(record, nodePrefix, betLevel, boardId, record.Hand, record.BetPct, record.Spr)
			if sqlInsert != "" {
				sqlGenerated++
				if _, err := sqlFile.WriteString(sqlInsert); err != nil {
					sqlWriteFailed++
					log.Printf("写入SQL语句失败: %v", err)
				}
			} else {
				handIndexFailed++
			}
		}

		// 输出详细统计信息
		nodeType := "根节点"
		if !isRootNode {
			nodeType = "子节点"
		}
		log.Printf("%s SQL生成统计:", nodeType)
		log.Printf("  当前节点处理记录数: %d", totalProcessed)
		log.Printf("  公牌索引失败: %d", boardIndexFailed)
		log.Printf("  手牌索引失败: %d", handIndexFailed)
		log.Printf("  成功生成SQL: %d", sqlGenerated)
		log.Printf("  写入失败: %d", sqlWriteFailed)

		// 打印总结信息
		log.Printf("处理完成节点 %s (%s)，JSON总记录数: %d，当前节点SQL: %d",
			node, nodeType, len(allRecords), sqlGenerated)
	}

	//遍历子节点，递归调用解析，但是当子节点的类型为SPLIT_NODE时，不再递归调用
	for _, child := range children {
		if child.NodeType != "SPLIT_NODE" {
			// 递归处理子节点
			parseNode(client, child.NodeID, effectiveStack)
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

// 修改：计算bet_pct、spr和stack_depth
func calculateBetMetrics(potInfo string, nodeId string, effectiveStack float64) (float64, float64, float64) {
	log.Printf("解析底池信息: %s", potInfo)

	// 默认值
	var oop, ip, dead float64 = 0, 0, 0

	// 解析potInfo：三个整数，以空格分隔，分别对应oop, ip, dead
	if potInfo != "" {
		potInfo = strings.TrimSpace(potInfo)
		fields := strings.Fields(potInfo)

		if len(fields) >= 3 {
			// 解析oop（第一个值）
			if val, err := strconv.ParseFloat(fields[0], 64); err == nil {
				oop = val
			}
			// 解析ip（第二个值）
			if val, err := strconv.ParseFloat(fields[1], 64); err == nil {
				ip = val
			}
			// 解析dead（第三个值）
			if val, err := strconv.ParseFloat(fields[2], 64); err == nil {
				dead = val
			}
		} else {
			log.Printf("警告：底池信息格式不正确，期望3个数值，实际得到: %d 个", len(fields))
		}
	}

	// 计算总底池大小
	totalPot := oop + ip + dead

	// 计算bet_pct（最近一次下注占底池比例）
	// 从nodeId中提取最后一个冒号后的值来判断最近的行动
	betPct := 0.0
	if nodeId != "" {
		// 找到最后一个冒号的位置
		lastColonIndex := strings.LastIndex(nodeId, ":")
		if lastColonIndex != -1 && lastColonIndex < len(nodeId)-1 {
			lastAction := nodeId[lastColonIndex+1:]
			log.Printf("提取最后行动: %s", lastAction)

			if lastAction == "c" {
				// check行动，下注为0
				betPct = 0.0
				log.Printf("检测到check行动，bet_pct = 0.0")
			} else if strings.HasPrefix(lastAction, "b") {
				// 下注行动，提取下注金额
				betAmountStr := strings.TrimPrefix(lastAction, "b")
				if betAmount, err := strconv.ParseFloat(betAmountStr, 64); err == nil {
					if totalPot > 0 {
						betPct = betAmount / totalPot
						log.Printf("检测到下注行动: b%s，下注金额: %.2f，底池: %.2f，bet_pct: %.3f",
							betAmountStr, betAmount, totalPot, betPct)
					}
				} else {
					log.Printf("警告：无法解析下注金额: %s", betAmountStr)
				}
			} else if strings.HasPrefix(lastAction, "r") {
				// raise行动，提取加注金额
				raiseAmountStr := strings.TrimPrefix(lastAction, "r")
				if raiseAmount, err := strconv.ParseFloat(raiseAmountStr, 64); err == nil {
					if totalPot > 0 {
						betPct = raiseAmount / totalPot
						log.Printf("检测到加注行动: r%s，加注金额: %.2f，底池: %.2f，bet_pct: %.3f",
							raiseAmountStr, raiseAmount, totalPot, betPct)
					}
				} else {
					log.Printf("警告：无法解析加注金额: %s", raiseAmountStr)
				}
			} else {
				log.Printf("未识别的行动类型: %s", lastAction)
			}
		} else {
			log.Printf("nodeId中未找到有效的行动信息: %s", nodeId)
		}
	}

	// 计算spr（栈底比）
	// 使用传入的有效筹码，计算剩余筹码与底池的比例
	remainingStack := effectiveStack - math.Max(oop, ip)
	spr := 0.0
	if totalPot > 0 && remainingStack > 0 {
		spr = remainingStack / totalPot
	}

	// 计算筹码深度（后手筹码，两人中筹码量较少的一方）
	stackDepth := math.Min(effectiveStack-oop, effectiveStack-ip)

	log.Printf("计算结果: oop=%.2f, ip=%.2f, dead=%.2f, totalPot=%.2f, bet_pct=%.3f, spr=%.3f, stack_depth=%.2f",
		oop, ip, dead, totalPot, betPct, spr, stackDepth)

	return betPct, spr, stackDepth
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

	// 生成INSERT语句，添加stack_depth字段
	sql := fmt.Sprintf("INSERT INTO flop_60bb_co_bb (node_prefix, bet_level, board_id, combo_id, stack_depth, bet_pct, spr, "+
		"action1, freq1, ev1, eq1, action2, freq2, ev2, eq2) VALUES "+
		"('%s', %d, %d, %d, %.3f, %.3f, %.3f, '%s', %.3f, %.3f, %.3f, '%s', %.3f, %.3f, %.3f);\n",
		nodePrefix, betLevel, boardId, comboId, record.StackDepth, betPct, spr,
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

// readCfrFiles 读取指定路径下的所有CFR文件
func readCfrFiles(cfrFolderPath string) ([]string, error) {
	var cfrFiles []string

	// 读取目录下的所有文件
	files, err := os.ReadDir(cfrFolderPath)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	// 过滤出CFR文件（.cfr文件）
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".cfr") {
			fullPath := filepath.Join(cfrFolderPath, fileName)
			cfrFiles = append(cfrFiles, fullPath)
		}
	}

	if len(cfrFiles) == 0 {
		return nil, fmt.Errorf("在路径 %s 下未找到任何 .cfr 文件", cfrFolderPath)
	}

	// 按文件名排序，确保处理顺序一致
	sort.Strings(cfrFiles)

	return cfrFiles, nil
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

// checkExistingParseResults 检查data目录中已存在的解析结果文件
func checkExistingParseResults() (map[string]bool, error) {
	existingFiles := make(map[string]bool)

	// 检查data目录是否存在
	if _, err := os.Stat("data"); os.IsNotExist(err) {
		log.Printf("data目录不存在，将创建新目录")
		// 创建目录
		if err := os.MkdirAll("data", 0755); err != nil {
			return nil, fmt.Errorf("创建data目录失败: %v", err)
		}
		return existingFiles, nil
	}

	// 读取目录中的所有文件
	files, err := os.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("读取data目录失败: %v", err)
	}

	// 统计已存在的.json和.sql文件
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".json") ||
			strings.HasSuffix(strings.ToLower(fileName), ".sql") {
			existingFiles[fileName] = true
		}
	}

	log.Printf("检查data目录: %s", "data")
	log.Printf("发现已存在的解析结果文件: %d 个", len(existingFiles))

	return existingFiles, nil
}

// checkExistingFiles 检查导出目录中已存在的文件
func checkExistingFiles() (map[string]bool, error) {
	existingFiles := make(map[string]bool)

	// 检查导出目录是否存在
	if _, err := os.Stat(exportSavePath); os.IsNotExist(err) {
		log.Printf("导出目录不存在: %s，将创建新目录", exportSavePath)
		// 创建目录
		if err := os.MkdirAll(exportSavePath, 0755); err != nil {
			return nil, fmt.Errorf("创建导出目录失败: %v", err)
		}
		return existingFiles, nil
	}

	// 读取目录中的所有.cfr文件
	files, err := os.ReadDir(exportSavePath)
	if err != nil {
		return nil, fmt.Errorf("读取导出目录失败: %v", err)
	}

	// 统计已存在的.cfr文件
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".cfr") {
			// 移除.cfr扩展名作为键
			baseName := strings.TrimSuffix(fileName, ".cfr")
			existingFiles[baseName] = true
		}
	}

	log.Printf("检查导出目录: %s", exportSavePath)
	log.Printf("发现已存在的.cfr文件: %d 个", len(existingFiles))

	return existingFiles, nil
}

// generateTaskFileName 生成任务文件名（不含扩展名）
func generateTaskFileName(pathPrefix, scriptName, flop string) string {
	return fmt.Sprintf("%s_%s_%s", pathPrefix, scriptName, flop)
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
