package main

import (
	"database/sql"
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

	_ "github.com/go-sql-driver/mysql"
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
		fmt.Println("用法: piodatasolver.exe [parse|calc|merge|mergecsv|jsonl] [参数]")
		fmt.Println("  parse <CFR文件夹路径> - 解析指定文件夹下的所有CFR文件并生成JSON/SQL文件")
		fmt.Println("    例如: piodatasolver.exe parse \"E:\\zdsbddz\\piosolver\\piosolver3\\saves\"")
		fmt.Println("  calc <脚本路径> - 执行PioSolver批量计算功能")
		fmt.Println("    例如: piodatasolver.exe calc \"D:\\gto\\piosolver3\\TreeBuilding\\mtt\\40bb\"")
		fmt.Println("  merge - 汇总data目录下的所有SQL文件为data.sql")
		fmt.Println("    例如: piodatasolver.exe merge")
		fmt.Println("  mergecsv - 将data目录下的所有SQL文件转换为CSV格式")
		fmt.Println("    例如: piodatasolver.exe mergecsv")
		fmt.Println("  jsonl - 将data目录下的所有SQL文件转换为JSONL格式")
		fmt.Println("    例如: piodatasolver.exe jsonl")
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
	case "merge":
		log.Printf("执行SQL文件汇总功能")
		runMergeCommand()
	case "mergecsv":
		log.Printf("执行SQL转CSV功能")
		runMergeCSVCommand()
	case "jsonl":
		runJSONLCommand()
	default:
		log.Printf("未知命令: %s", command)
		log.Println("支持的命令: parse, calc, merge, mergecsv, jsonl")
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

	// 计算策略执行者（IP或OOP）
	ipOrOop := calculateIpOrOop(node)

	// 计算主动下注次数（在convertNodePath之前计算，因为convertNodePath会移除b和r前缀）
	betLevel := calculateBetLevel(node)

	// 创建一个映射，存储每个手牌的Record
	handRecords := make(map[string]*model.Record)

	// 先为每个手牌创建一个Record
	for _, hand := range handCards {
		// 计算手牌的combo_id
		comboId, ok := handOrder.Index(hand)
		if !ok {
			log.Printf("警告：无法找到手牌 %s 的索引", hand)
			comboId = -1 // 设置为-1表示未找到
		}

		// 标准化公牌并计算board_id
		standardizedBoard := standardizeBoard(board)
		boardId, ok := boardOrder.Index(standardizedBoard)
		if !ok {
			log.Printf("警告：无法找到公牌 %s (标准化后: %s) 的索引", board, standardizedBoard)
			boardId = -1 // 设置为-1表示未找到
		}

		handRecords[hand] = &model.Record{
			Node:       node,
			Actor:      actor,
			Board:      board,
			BoardId:    boardId, // 设置公牌ID
			Hand:       hand,
			ComboId:    comboId,          // 设置手牌ID
			Actions:    []model.Action{}, // 初始化空的Actions数组
			PotInfo:    pot,              // 设置底池信息
			StackDepth: stackDepth,       // 设置筹码深度
			Spr:        spr,              // 设置栈底比
			BetPct:     betPct,           // 设置下注比例
			IpOrOop:    ipOrOop,          // 设置策略执行者
			BetLevel:   betLevel,         // 设置主动下注次数
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

		// 从CFR文件路径提取文件名并生成表名
		_, cfrFileName = filepath.Split(cfrFilePath)
		tableName := generateTableName(cfrFileName)

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
			// 使用Record中已计算的BetLevel，而不是重新计算
			betLevel := record.BetLevel

			// 生成SQL插入语句（使用Record中已计算的值和动态表名）
			sqlInsert := generateSQLInsert(record, nodePrefix, betLevel, tableName)
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

// 新增：计算下注次数（主动下注行为）
func calculateBetLevel(nodePath string) int {
	// 移除固定前缀 "r:0"
	if !strings.HasPrefix(nodePath, "r:0") {
		return 0
	}

	// 去掉 "r:0" 前缀
	remaining := strings.TrimPrefix(nodePath, "r:0")
	if remaining == "" {
		return 0 // 只有 "r:0"，没有任何行动
	}

	// 移除开头的冒号
	if strings.HasPrefix(remaining, ":") {
		remaining = remaining[1:]
	}

	if remaining == "" {
		return 0
	}

	// 按冒号分割行动
	actions := strings.Split(remaining, ":")
	betCount := 0

	// 统计主动下注次数
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}

		// 检查是否为下注行为：
		// - 以 'b' 开头的是bet下注
		// - 以 'r' 开头的是raise加注（也算作主动下注）
		// - 'c' 是check，不算主动下注
		// - 'f' 是fold，不算主动下注
		if strings.HasPrefix(action, "b") || strings.HasPrefix(action, "r") {
			betCount++
			log.Printf("检测到主动下注行为: %s，当前bet_level: %d", action, betCount)
		}
	}

	log.Printf("节点路径 %s 的bet_level: %d", nodePath, betCount)
	return betCount
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
func generateSQLInsert(record *model.Record, nodePrefix string, betLevel int, tableName string) string {
	// 确保至少有一个动作
	if len(record.Actions) == 0 {
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

	// 生成INSERT语句，使用动态表名
	sql := fmt.Sprintf("INSERT IGNORE INTO %s (node_prefix, bet_level, board_id, combo_id, stack_depth, bet_pct, spr, "+
		"board_str, combo_str, ip_or_oop, action1, freq1, ev1, eq1, action2, freq2, ev2, eq2) VALUES "+
		"('%s', %d, %d, %d, %.3f, %.4f, %.4f, '%s', '%s', '%s', '%s', %.3f, %.3f, %.3f, '%s', %.3f, %.3f, %.3f);\n",
		tableName, nodePrefix, betLevel, record.BoardId, record.ComboId, record.StackDepth, record.BetPct, record.Spr,
		strings.TrimSpace(record.Board), record.Hand, record.IpOrOop, action1Label, action1Freq, action1Ev, action1Eq,
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

// 新增：根据node_prefix判断策略执行者（IP或OOP）
func calculateIpOrOop(nodePrefix string) string {
	// 示例：r:0:c:20:70:170:370
	// r:0 是固定前缀，然后 c(oop) -> 20(ip) -> 70(oop) -> 170(ip) -> 370(oop)
	// 接下来应该是IP执行策略

	// 移除固定前缀 "r:0"
	if !strings.HasPrefix(nodePrefix, "r:0") {
		log.Printf("警告：节点格式不符合预期，返回默认值IP: %s", nodePrefix)
		return "IP"
	}

	// 去掉 "r:0" 前缀
	remaining := strings.TrimPrefix(nodePrefix, "r:0")
	if remaining == "" {
		// 如果只有 "r:0"，那么第一个行动者是OOP
		return "OOP"
	}

	// 移除开头的冒号
	if strings.HasPrefix(remaining, ":") {
		remaining = remaining[1:]
	}

	if remaining == "" {
		return "OOP"
	}

	// 按冒号分割剩余部分
	parts := strings.Split(remaining, ":")

	// 计算行动次数：
	// 第1次行动：OOP (c)
	// 第2次行动：IP (20)
	// 第3次行动：OOP (70)
	// 第4次行动：IP (170)
	// 第5次行动：OOP (370)
	// 第6次行动：IP (下一个策略执行者)

	actionCount := len(parts)
	log.Printf("节点 %s 解析：去除r:0后=%s，行动次数=%d", nodePrefix, remaining, actionCount)

	// 下一个策略执行者：
	// 如果已有奇数次行动，下一个是IP
	// 如果已有偶数次行动，下一个是OOP
	if actionCount%2 == 1 {
		return "IP"
	} else {
		return "OOP"
	}
}

// 新增：从CFR文件名生成表名
func generateTableName(cfrFileName string) string {
	// 移除.cfr扩展名
	baseName := strings.TrimSuffix(cfrFileName, ".cfr")

	// 解析文件名格式: 40bb_COvsBB_8d5c4c
	// 转换为: flop_40bb_co_bb_8d5c4c (包含公牌信息，用于CSV文件名)
	parts := strings.Split(baseName, "_")
	if len(parts) >= 3 {
		// 提取筹码深度 (如 40bb)
		stackDepth := parts[0]

		// 提取位置信息 (如 COvsBB)
		position := parts[1]

		// 提取公牌信息 (如 8d5c4c)
		board := parts[2]

		// 转换位置信息为小写并格式化
		// COvsBB -> co_bb
		positionLower := strings.ToLower(position)
		positionFormatted := strings.ReplaceAll(positionLower, "vs", "_")

		// 生成表名格式: flop_筹码_位置_公牌 (包含公牌，用于CSV文件名)
		tableName := fmt.Sprintf("flop_%s_%s_%s", stackDepth, positionFormatted, board)

		log.Printf("生成CSV文件名: %s -> %s", baseName, tableName)
		return tableName
	}

	// 如果解析失败，使用默认表名
	log.Printf("警告：无法解析CFR文件名 %s，使用默认表名", baseName)
	return "flop_60bb_co_bb"
}

// 新增：从CFR文件名生成表名（不包含公牌信息）
func generateTableNameWithoutBoard(cfrFileName string) string {
	// 移除.cfr扩展名
	baseName := strings.TrimSuffix(cfrFileName, ".cfr")

	// 解析文件名格式: 40bb_COvsBB_8d5c4c
	// 转换为: flop_40bb_co_bb (不包含公牌信息)
	parts := strings.Split(baseName, "_")
	if len(parts) >= 2 {
		// 提取筹码深度 (如 40bb)
		stackDepth := parts[0]

		// 提取位置信息 (如 COvsBB)
		position := parts[1]

		// 转换位置信息为小写并格式化
		// COvsBB -> co_bb
		positionLower := strings.ToLower(position)
		positionFormatted := strings.ReplaceAll(positionLower, "vs", "_")

		// 生成表名格式: flop_筹码_位置 (不包含公牌)
		tableName := fmt.Sprintf("flop_%s_%s", stackDepth, positionFormatted)

		log.Printf("生成表名(不含公牌): %s -> %s", baseName, tableName)
		return tableName
	}

	// 如果解析失败，使用默认表名
	log.Printf("警告：无法解析CFR文件名 %s，使用默认表名", baseName)
	return "flop_60bb_co_bb"
}

// runMergeCommand 执行SQL文件汇总功能
func runMergeCommand() {
	log.Println("==================================")
	log.Println("【SQL文件汇总功能】正在初始化...")
	log.Println("==================================")

	// 检查data目录是否存在
	dataDir := "data"
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		log.Fatalf("data目录不存在: %s", dataDir)
	}

	// 读取data目录下的所有文件
	files, err := os.ReadDir(dataDir)
	if err != nil {
		log.Fatalf("读取data目录失败: %v", err)
	}

	// 过滤出SQL文件，排除data.sql
	var sqlFiles []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".sql") && fileName != "data.sql" {
			fullPath := filepath.Join(dataDir, fileName)
			sqlFiles = append(sqlFiles, fullPath)
		}
	}

	if len(sqlFiles) == 0 {
		log.Printf("在data目录中未找到任何SQL文件（除data.sql外）")
		return
	}

	// 按文件名排序，确保汇总顺序一致
	sort.Strings(sqlFiles)

	log.Printf("找到 %d 个SQL文件需要汇总", len(sqlFiles))
	for i, file := range sqlFiles {
		log.Printf("  %d. %s", i+1, filepath.Base(file))
	}

	// 创建输出文件
	outputPath := filepath.Join(dataDir, "data.sql")
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("创建汇总文件失败: %v", err)
	}
	defer outputFile.Close()

	// 写入文件头部
	outputFile.WriteString("-- 汇总的SQL文件\n")
	outputFile.WriteString(fmt.Sprintf("-- 生成时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	outputFile.WriteString(fmt.Sprintf("-- 汇总了 %d 个SQL文件\n", len(sqlFiles)))
	outputFile.WriteString("-- ========================================\n\n")

	// 统计变量
	totalLines := 0
	totalFiles := 0

	// 逐个读取并合并SQL文件
	for i, sqlFile := range sqlFiles {
		log.Printf("\n[%d/%d] 🔄 处理文件: %s", i+1, len(sqlFiles), filepath.Base(sqlFile))

		// 读取文件内容
		content, err := os.ReadFile(sqlFile)
		if err != nil {
			log.Printf("  ❌ 读取文件失败: %v，跳过此文件", err)
			continue
		}

		// 统计行数（排除空行和注释行）
		lines := strings.Split(string(content), "\n")
		validLines := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "--") {
				validLines++
			}
		}

		// 写入分隔符和文件信息
		outputFile.WriteString(fmt.Sprintf("-- ========================================\n"))
		outputFile.WriteString(fmt.Sprintf("-- 来源文件: %s\n", filepath.Base(sqlFile)))
		outputFile.WriteString(fmt.Sprintf("-- 有效SQL语句: %d 条\n", validLines))
		outputFile.WriteString(fmt.Sprintf("-- ========================================\n\n"))

		// 写入文件内容
		_, err = outputFile.Write(content)
		if err != nil {
			log.Printf("  ❌ 写入文件内容失败: %v", err)
			continue
		}

		// 确保文件末尾有换行符
		outputFile.WriteString("\n\n")

		totalLines += validLines
		totalFiles++
		log.Printf("  ✓ 处理完成，有效SQL语句: %d 条", validLines)
	}

	// 写入文件尾部统计信息
	outputFile.WriteString("-- ========================================\n")
	outputFile.WriteString("-- 汇总统计信息\n")
	outputFile.WriteString(fmt.Sprintf("-- 处理文件数: %d\n", totalFiles))
	outputFile.WriteString(fmt.Sprintf("-- 总SQL语句数: %d\n", totalLines))
	outputFile.WriteString(fmt.Sprintf("-- 汇总完成时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	outputFile.WriteString("-- ========================================\n")

	log.Println("\n==================================")
	log.Println("【SQL文件汇总功能】完成！")
	log.Printf("📊 汇总统计:")
	log.Printf("   处理文件数: %d", totalFiles)
	log.Printf("   总SQL语句数: %d", totalLines)
	log.Printf("   输出文件: %s", outputPath)
	log.Println("==================================")
}

// runMergeCSVCommand 执行SQL转CSV功能
func runMergeCSVCommand() {
	log.Println("==================================")
	log.Println("【SQL转CSV功能】正在初始化...")
	log.Println("==================================")

	// 检查data目录是否存在
	dataDir := "data"
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		log.Fatalf("data目录不存在: %s", dataDir)
	}

	// 读取data目录下的所有文件
	files, err := os.ReadDir(dataDir)
	if err != nil {
		log.Fatalf("读取data目录失败: %v", err)
	}

	// 过滤出SQL文件，排除data.sql
	var sqlFiles []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".sql") && fileName != "data.sql" {
			fullPath := filepath.Join(dataDir, fileName)
			sqlFiles = append(sqlFiles, fullPath)
		}
	}

	if len(sqlFiles) == 0 {
		log.Println("没有找到需要转换的SQL文件")
		return
	}

	log.Printf("找到 %d 个SQL文件需要转换", len(sqlFiles))

	// 创建csv目录
	csvDir := "csv"
	if err := os.MkdirAll(csvDir, 0755); err != nil {
		log.Fatalf("创建csv目录失败: %v", err)
	}

	// 统计信息
	var totalFiles int
	var totalRecords int
	var csvToTableMap = make(map[string]string) // CSV文件名 -> 表名的映射

	// 为每个SQL文件生成独立的CSV文件
	for _, sqlFile := range sqlFiles {
		log.Printf("正在处理SQL文件: %s", filepath.Base(sqlFile))

		// 从SQL文件名推导CFR文件名
		sqlFileName := filepath.Base(sqlFile)
		cfrFileName := strings.TrimSuffix(sqlFileName, ".sql") + ".cfr"

		// 生成完整的CSV文件名（包含公牌）
		csvBaseName := generateTableName(cfrFileName) // 包含公牌的完整名称
		csvFileName := csvBaseName + ".csv"
		csvFilePath := filepath.Join(csvDir, csvFileName)

		// 生成表名（不包含公牌）
		tableName := generateTableNameWithoutBoard(cfrFileName)

		// 记录CSV文件到表名的映射
		csvToTableMap[csvFileName] = tableName

		// 转换单个SQL文件为CSV
		recordCount, err := convertSQLToCSV(sqlFile, csvFilePath, tableName)
		if err != nil {
			log.Printf("转换SQL文件 %s 失败: %v", sqlFile, err)
			continue
		}

		totalFiles++
		totalRecords += recordCount
		log.Printf("已生成CSV文件: %s -> 表: %s (记录数: %d)", csvFileName, tableName, recordCount)
	}

	// 生成LOAD DATA脚本
	if err := generateLoadDataScriptWithMapping(csvDir, csvToTableMap); err != nil {
		log.Printf("生成LOAD DATA脚本失败: %v", err)
	}

	log.Println("\n==================================")
	log.Printf("【SQL转CSV完成】")
	log.Printf("总CSV文件数: %d", totalFiles)
	log.Printf("总记录数: %d", totalRecords)
	log.Printf("CSV文件保存在: %s", csvDir)
	log.Printf("LOAD DATA脚本: %s/load_data.sql", csvDir)
	log.Println("==================================")
}

// parseSQLFile 解析SQL文件，提取表名和数据记录
func parseSQLFile(content string) (string, [][]string, error) {
	lines := strings.Split(content, "\n")
	var records [][]string
	var tableName string

	// 正则表达式匹配INSERT语句
	insertRegex := regexp.MustCompile(`INSERT\s+(?:IGNORE\s+)?INTO\s+(\w+)\s+\([^)]+\)\s+VALUES\s+\(([^)]+)\);?`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		// 匹配INSERT语句
		matches := insertRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			// 提取表名（第一次遇到时）
			if tableName == "" {
				tableName = matches[1]
			}

			// 提取VALUES部分
			valuesStr := matches[2]

			// 解析VALUES中的字段值
			values, err := parseValues(valuesStr)
			if err != nil {
				log.Printf("警告：解析VALUES失败: %v，跳过此行", err)
				continue
			}

			records = append(records, values)
		}
	}

	if tableName == "" {
		return "", nil, fmt.Errorf("未找到有效的表名")
	}

	return tableName, records, nil
}

// parseValues 解析SQL VALUES子句中的值
func parseValues(valuesStr string) ([]string, error) {
	var values []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, char := range valuesStr {
		switch char {
		case '\'':
			if escaped {
				current.WriteRune(char)
				escaped = false
			} else {
				inQuotes = !inQuotes
				// 不将引号写入值中
			}
		case '\\':
			if inQuotes && !escaped {
				escaped = true
				// 不写入转义字符本身，等待下一个字符
			} else {
				current.WriteRune(char)
				escaped = false
			}
		case ',':
			if !inQuotes {
				// 字段分隔符
				value := strings.TrimSpace(current.String())
				values = append(values, value)
				current.Reset()
			} else {
				current.WriteRune(char)
			}
			escaped = false
		case ' ', '\t':
			if inQuotes {
				current.WriteRune(char)
			} else if current.Len() > 0 {
				// 只有在值不为空时才添加空格
				current.WriteRune(char)
			}
			escaped = false
		default:
			current.WriteRune(char)
			escaped = false
		}
	}

	// 添加最后一个值
	if current.Len() > 0 {
		value := strings.TrimSpace(current.String())
		values = append(values, value)
	}

	return values, nil
}

// writeCSVFile 写入CSV文件
func writeCSVFile(filePath string, records [][]string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建CSV文件失败: %v", err)
	}
	defer file.Close()

	// 写入CSV头部（字段名）
	header := []string{
		"node_prefix", "bet_level", "board_id", "combo_id", "stack_depth", "bet_pct", "spr",
		"board_str", "combo_str", "ip_or_oop", "action1", "freq1", "ev1", "eq1",
		"action2", "freq2", "ev2", "eq2",
	}

	// 写入头部行
	headerLine := "\"" + strings.Join(header, "\",\"") + "\"\n"
	_, err = file.WriteString(headerLine)
	if err != nil {
		return fmt.Errorf("写入CSV头部失败: %v", err)
	}

	// 写入数据行
	for _, record := range records {
		// 确保记录有足够的字段
		if len(record) < len(header) {
			// 补齐缺失的字段
			for len(record) < len(header) {
				record = append(record, "")
			}
		}

		// 对每个字段进行CSV转义
		var escapedRecord []string
		for _, field := range record {
			// 移除字段两端的引号（如果有的话）
			field = strings.Trim(field, "'\"")
			// 转义CSV中的双引号
			field = strings.ReplaceAll(field, "\"", "\"\"")
			escapedRecord = append(escapedRecord, field)
		}

		// 写入数据行
		dataLine := "\"" + strings.Join(escapedRecord, "\",\"") + "\"\n"
		_, err = file.WriteString(dataLine)
		if err != nil {
			return fmt.Errorf("写入CSV数据失败: %v", err)
		}
	}

	return nil
}

// generateLoadDataScript 生成LOAD DATA脚本
func generateLoadDataScript(csvDir string, tableNames []string) error {
	scriptPath := filepath.Join(csvDir, "load_data.sql")
	file, err := os.Create(scriptPath)
	if err != nil {
		return fmt.Errorf("创建脚本文件失败: %v", err)
	}
	defer file.Close()

	// 写入脚本头部
	file.WriteString("-- ========================================\n")
	file.WriteString("-- PioSolver数据导入脚本\n")
	file.WriteString("-- 自动生成时间: " + time.Now().Format("2006-01-02 15:04:05") + "\n")
	file.WriteString("-- ========================================\n\n")

	// 为每个表生成LOAD DATA语句
	for _, tableName := range tableNames {
		csvFileName := fmt.Sprintf("%s.csv", tableName)

		file.WriteString(fmt.Sprintf("-- 导入表: %s\n", tableName))
		file.WriteString(fmt.Sprintf("LOAD DATA LOCAL INFILE '%s/%s'\n", csvDir, csvFileName))
		file.WriteString(fmt.Sprintf("INTO TABLE %s\n", tableName))
		file.WriteString("FIELDS TERMINATED BY ',' ENCLOSED BY '\"'\n")
		file.WriteString("LINES TERMINATED BY '\\n'\n")
		file.WriteString("(node_prefix, bet_level, board_id, combo_id, stack_depth, bet_pct, spr, board_str, combo_str, ip_or_oop,\n")
		file.WriteString(" action1, freq1, ev1, eq1,\n")
		file.WriteString(" action2, freq2, ev2, eq2);\n\n")
	}

	// 写入脚本尾部
	file.WriteString("-- ========================================\n")
	file.WriteString("-- 导入完成\n")
	file.WriteString(fmt.Sprintf("-- 总表数: %d\n", len(tableNames)))
	file.WriteString("-- ========================================\n")

	return nil
}

// convertSQLToCSV 将单个SQL文件转换为CSV文件
func convertSQLToCSV(sqlFilePath, csvFilePath, tableName string) (int, error) {
	// 读取SQL文件内容
	content, err := os.ReadFile(sqlFilePath)
	if err != nil {
		return 0, fmt.Errorf("读取SQL文件失败: %v", err)
	}

	// 解析SQL文件，提取数据
	_, records, err := parseSQLFile(string(content))
	if err != nil {
		return 0, fmt.Errorf("解析SQL文件失败: %v", err)
	}

	if len(records) == 0 {
		return 0, fmt.Errorf("文件中没有有效的INSERT语句")
	}

	// 写入CSV文件
	err = writeCSVFile(csvFilePath, records)
	if err != nil {
		return 0, fmt.Errorf("写入CSV文件失败: %v", err)
	}

	return len(records), nil
}

// generateLoadDataScriptWithMapping 生成LOAD DATA脚本，支持CSV文件名到表名的映射
func generateLoadDataScriptWithMapping(csvDir string, csvToTableMap map[string]string) error {
	scriptPath := filepath.Join(csvDir, "load_data.sql")
	file, err := os.Create(scriptPath)
	if err != nil {
		return fmt.Errorf("创建脚本文件失败: %v", err)
	}
	defer file.Close()

	// 获取当前工作目录的绝对路径
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %v", err)
	}

	// 构建CSV目录的绝对路径
	csvAbsPath := filepath.Join(currentDir, csvDir)
	// 将Windows路径分隔符转换为正斜杠（MySQL兼容）
	csvAbsPath = strings.ReplaceAll(csvAbsPath, "\\", "/")

	// 写入脚本头部
	file.WriteString("-- ========================================\n")
	file.WriteString("-- PioSolver数据导入脚本\n")
	file.WriteString("-- 自动生成时间: " + time.Now().Format("2006-01-02 15:04:05") + "\n")
	file.WriteString("-- 支持IGNORE功能，避免重复数据冲突\n")
	file.WriteString(fmt.Sprintf("-- CSV文件路径: %s\n", csvAbsPath))
	file.WriteString("-- ========================================\n\n")

	// 按表名分组CSV文件
	tableToCSVs := make(map[string][]string)
	for csvFile, tableName := range csvToTableMap {
		tableToCSVs[tableName] = append(tableToCSVs[tableName], csvFile)
	}

	// 为每个表生成LOAD DATA语句
	for tableName, csvFiles := range tableToCSVs {
		file.WriteString(fmt.Sprintf("-- ========================================\n"))
		file.WriteString(fmt.Sprintf("-- 导入表: %s (共 %d 个CSV文件)\n", tableName, len(csvFiles)))
		file.WriteString(fmt.Sprintf("-- ========================================\n\n"))

		// 为每个CSV文件生成LOAD DATA语句
		for _, csvFileName := range csvFiles {
			// 构建完整的绝对路径
			csvFullPath := fmt.Sprintf("%s/%s", csvAbsPath, csvFileName)

			file.WriteString(fmt.Sprintf("-- 导入文件: %s\n", csvFileName))
			file.WriteString(fmt.Sprintf("LOAD DATA LOCAL INFILE '%s'\n", csvFullPath))
			file.WriteString(fmt.Sprintf("IGNORE INTO TABLE %s\n", tableName)) // 添加IGNORE关键字
			file.WriteString("FIELDS TERMINATED BY ',' ENCLOSED BY '\"'\n")
			file.WriteString("LINES TERMINATED BY '\\n'\n")
			file.WriteString("IGNORE 1 LINES\n") // 忽略CSV头部行
			file.WriteString("(node_prefix, bet_level, board_id, combo_id, stack_depth, bet_pct, spr, board_str, combo_str, ip_or_oop,\n")
			file.WriteString(" action1, freq1, ev1, eq1,\n")
			file.WriteString(" action2, freq2, ev2, eq2);\n\n")
		}
	}

	// 写入脚本尾部
	file.WriteString("-- ========================================\n")
	file.WriteString("-- 导入完成\n")
	file.WriteString(fmt.Sprintf("-- 总表数: %d\n", len(tableToCSVs)))
	file.WriteString(fmt.Sprintf("-- 总CSV文件数: %d\n", len(csvToTableMap)))
	file.WriteString(fmt.Sprintf("-- CSV文件绝对路径: %s\n", csvAbsPath))
	file.WriteString("-- ========================================\n")

	return nil
}

// runJSONLCommand 执行JSONL生成功能
func runJSONLCommand() {
	log.Println("==================================")
	log.Println("【JSONL生成功能】正在初始化...")
	log.Println("==================================")

	// 连接数据库
	db, err := connectDatabase()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 获取所有表名
	tableNames, err := getTableNames(db)
	if err != nil {
		log.Fatalf("获取表名失败: %v", err)
	}

	log.Printf("找到 %d 个表", len(tableNames))

	var allTrainingData []SimpleTrainingData
	totalRecords := 0

	// 处理每个表
	for _, tableName := range tableNames {
		log.Printf("正在处理表: %s", tableName)

		records, err := fetchTableData(db, tableName)
		if err != nil {
			log.Printf("获取表 %s 数据失败: %v", tableName, err)
			continue
		}

		// 解析位置信息
		playerPos, opponentPos := parsePositionsFromTableName(tableName)

		// 为每条记录生成训练数据
		for _, record := range records {
			// 处理action1
			if record.Action1 != "" && record.Freq1 > 0 {
				// 分析手牌特征
				handFeatures := analyzeHandFeatures(record.ComboStr, record.BoardStr)

				// 计算底池赔率（如果有上一个下注动作）
				potOdds := 0.0
				lastActionSize := extractLastActionSize(record.NodePrefix)
				if lastActionSize > 0 {
					potOdds = lastActionSize / (100 + lastActionSize)
				}

				training := SimpleTrainingData{
					Board:               record.BoardStr,
					HoleCards:           record.ComboStr,
					PlayerPosition:      playerPos,
					OpponentPosition:    opponentPos,
					PlayerIsOOP:         record.IPOrOOP == "OOP",
					SPR:                 record.SPR,
					BoardTextureSummary: analyzeBoardTexture(record.BoardStr),
					ActionHistory:       parseActionHistory(record.NodePrefix, record.IPOrOOP),
					GTOAction:           normalizeActionType(record.Action1),
					FrequencyPct:        record.Freq1 * 100,
					EV:                  record.EV1,
					HandFeatures:        handFeatures,
					Equity:              record.EQ1, // 使用原始的EQ字段
					PotOdds:             potOdds,
					StackDepth:          record.StackDepth,
					BetLevel:            record.BetLevel,
					BetPct:              record.BetPct, // 使用数据库中的bet_pct
				}
				allTrainingData = append(allTrainingData, training)
			}

			// 处理action2
			if record.Action2 != "" && record.Freq2 > 0 {
				// 分析手牌特征
				handFeatures := analyzeHandFeatures(record.ComboStr, record.BoardStr)

				// 计算底池赔率
				potOdds := 0.0
				lastActionSize := extractLastActionSize(record.NodePrefix)
				if lastActionSize > 0 {
					potOdds = lastActionSize / (100 + lastActionSize)
				}

				training := SimpleTrainingData{
					Board:               record.BoardStr,
					HoleCards:           record.ComboStr,
					PlayerPosition:      playerPos,
					OpponentPosition:    opponentPos,
					PlayerIsOOP:         record.IPOrOOP == "OOP",
					SPR:                 record.SPR,
					BoardTextureSummary: analyzeBoardTexture(record.BoardStr),
					ActionHistory:       parseActionHistory(record.NodePrefix, record.IPOrOOP),
					GTOAction:           normalizeActionType(record.Action2),
					FrequencyPct:        record.Freq2 * 100,
					EV:                  record.EV2,
					HandFeatures:        handFeatures,
					Equity:              record.EQ2, // 使用原始的EQ字段
					PotOdds:             potOdds,
					StackDepth:          record.StackDepth,
					BetLevel:            record.BetLevel,
					BetPct:              record.BetPct, // 使用数据库中的bet_pct
				}
				allTrainingData = append(allTrainingData, training)
			}
		}

		totalRecords += len(records)
		log.Printf("表 %s 处理了 %d 条原始记录", tableName, len(records))
	}

	// 过滤掉一些无效数据
	var filteredData []SimpleTrainingData
	for _, data := range allTrainingData {
		// 过滤掉频率太低的动作（小于5%）
		if data.FrequencyPct < 5.0 {
			continue
		}
		// 过滤掉EV异常的数据
		if math.IsNaN(data.EV) || math.IsInf(data.EV, 0) {
			continue
		}
		filteredData = append(filteredData, data)
	}

	// 输出JSONL文件
	err = writeSimpleJSONLFile(filteredData, "train.jsonl")
	if err != nil {
		log.Fatalf("写入JSONL文件失败: %v", err)
	}

	// 生成评估数据集（10%的数据）
	evalData := splitSimpleEvalData(filteredData, 0.1)
	err = writeSimpleJSONLFile(evalData, "eval.jsonl")
	if err != nil {
		log.Printf("写入评估数据集失败: %v", err)
	}

	log.Println("==================================")
	log.Printf("【JSONL生成完成】")
	log.Printf("✅ 原始记录数: %d", totalRecords)
	log.Printf("✅ 生成的训练样本: %d", len(allTrainingData))
	log.Printf("✅ 过滤后的训练样本: %d", len(filteredData))
	log.Printf("✅ 评估数据: %d 条", len(evalData))
	log.Printf("✅ 输出文件: train.jsonl, eval.jsonl")
	log.Println("==================================")
}

// connectDatabase 连接MySQL数据库
func connectDatabase() (*sql.DB, error) {
	// 数据库连接配置 - 使用用户的MySQL数据库
	// 格式: username:password@tcp(host:port)/database?charset=utf8mb4&parseTime=True&loc=Local
	dsn := "root:Dhk@0052410@tcp(localhost:3306)/poker?charset=utf8mb4&parseTime=True&loc=Local"

	// 如果有环境变量，优先使用环境变量
	if envDSN := os.Getenv("MYSQL_DSN"); envDSN != "" {
		dsn = envDSN
	}

	log.Printf("正在连接数据库...")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %v", err)
	}

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("数据库连接测试失败: %v", err)
	}

	log.Printf("数据库连接成功")
	return db, nil
}

// getTableNames 获取所有以flop_开头的表名
func getTableNames(db *sql.DB) ([]string, error) {
	query := "SHOW TABLES LIKE 'flop_%'"
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var tableName string
		err := rows.Scan(&tableName)
		if err != nil {
			return nil, err
		}
		tableNames = append(tableNames, tableName)
	}

	return tableNames, nil
}

// fetchTableData 获取表中的所有数据
func fetchTableData(db *sql.DB, tableName string) ([]DBRecord, error) {
	query := fmt.Sprintf(`
		SELECT node_prefix, bet_level, board_id, combo_id, combo_str, board_str, 
		       ip_or_oop, stack_depth, bet_pct, spr,
		       action1, freq1, ev1, eq1, action2, freq2, ev2, eq2
		FROM %s
	`, tableName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []DBRecord
	for rows.Next() {
		var record DBRecord
		err := rows.Scan(
			&record.NodePrefix, &record.BetLevel, &record.BoardID, &record.ComboID,
			&record.ComboStr, &record.BoardStr, &record.IPOrOOP, &record.StackDepth,
			&record.BetPct, &record.SPR, &record.Action1, &record.Freq1, &record.EV1,
			&record.EQ1, &record.Action2, &record.Freq2, &record.EV2, &record.EQ2,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

// aggregateRecords 聚合记录数据
func aggregateRecords(records []DBRecord) map[AggregationKey]map[string]*ActionAggregation {
	aggregated := make(map[AggregationKey]map[string]*ActionAggregation)

	for _, record := range records {
		key := AggregationKey{
			NodePrefix: record.NodePrefix,
			BoardID:    record.BoardID,
			IPOrOOP:    record.IPOrOOP,
			StackDepth: record.StackDepth,
			BetPct:     record.BetPct,
		}

		if aggregated[key] == nil {
			aggregated[key] = make(map[string]*ActionAggregation)
		}

		// 处理action1
		if record.Action1 != "" && record.Freq1 > 0 {
			actionKey := record.Action1
			sizePct := extractSizeFromAction(record.Action1)

			if aggregated[key][actionKey] == nil {
				aggregated[key][actionKey] = &ActionAggregation{
					ActionType: normalizeActionType(record.Action1),
					SizePctPot: sizePct,
				}
			}

			agg := aggregated[key][actionKey]
			agg.TotalFreq += record.Freq1
			agg.TotalEV += record.EV1 * record.Freq1
			agg.ComboCount++

			// 添加combo示例
			if len(agg.ComboExamples) < 3 {
				note := getComboNote(record.Freq1, record.EV1, record.Action1)
				agg.ComboExamples = append(agg.ComboExamples, ComboExample{
					Combo:        record.ComboStr,
					Action:       record.Action1,
					FrequencyPct: record.Freq1 * 100,
					Note:         note,
				})
			}
		}

		// 处理action2
		if record.Action2 != "" && record.Freq2 > 0 {
			actionKey := record.Action2
			sizePct := extractSizeFromAction(record.Action2)

			if aggregated[key][actionKey] == nil {
				aggregated[key][actionKey] = &ActionAggregation{
					ActionType: normalizeActionType(record.Action2),
					SizePctPot: sizePct,
				}
			}

			agg := aggregated[key][actionKey]
			agg.TotalFreq += record.Freq2
			agg.TotalEV += record.EV2 * record.Freq2
			agg.ComboCount++

			// 添加combo示例
			if len(agg.ComboExamples) < 3 {
				note := getComboNote(record.Freq2, record.EV2, record.Action2)
				agg.ComboExamples = append(agg.ComboExamples, ComboExample{
					Combo:        record.ComboStr,
					Action:       record.Action2,
					FrequencyPct: record.Freq2 * 100,
					Note:         note,
				})
			}
		}
	}

	return aggregated
}

// convertToTrainingData 将聚合数据转换为训练数据
func convertToTrainingData(aggregated map[AggregationKey]map[string]*ActionAggregation, tableName string, records []DBRecord) []TrainingData {
	var trainingData []TrainingData

	// 创建一个映射，用于快速查找board_str和spr
	boardInfoMap := make(map[int]struct {
		BoardStr string
		SPR      float64
	})

	for _, record := range records {
		boardInfoMap[record.BoardID] = struct {
			BoardStr string
			SPR      float64
		}{
			BoardStr: record.BoardStr,
			SPR:      record.SPR,
		}
	}

	for key, actions := range aggregated {
		// 从boardInfoMap中获取真实的board信息
		boardInfo, exists := boardInfoMap[key.BoardID]
		if !exists {
			continue // 如果找不到board信息，跳过这个节点
		}

		boardStr := boardInfo.BoardStr
		spr := boardInfo.SPR

		// 解析位置信息
		playerPos, opponentPos := parsePositionsFromTableName(tableName)

		// 生成策略分布
		var strategies []ActionStrategy
		var totalFreq float64
		var totalEV float64

		// 先统计总频率
		for _, actionAgg := range actions {
			if actionAgg.ComboCount > 0 {
				totalFreq += actionAgg.TotalFreq
			}
		}

		// 计算归一化的频率和平均EV
		for _, actionAgg := range actions {
			if actionAgg.ComboCount > 0 && totalFreq > 0 {
				// 计算该动作在所有动作中的频率占比
				actionFreqPct := (actionAgg.TotalFreq / totalFreq) * 100
				// 计算该动作的平均EV
				avgEV := actionAgg.TotalEV / actionAgg.TotalFreq

				strategies = append(strategies, ActionStrategy{
					ActionType:   actionAgg.ActionType,
					SizePctPot:   actionAgg.SizePctPot,
					FrequencyPct: actionFreqPct,
					AverageEVBB:  avgEV,
				})

				totalEV += avgEV * (actionAgg.TotalFreq / totalFreq)
			}
		}

		// 收集代表性combo示例
		var comboExamples []ComboExample
		for _, actionAgg := range actions {
			comboExamples = append(comboExamples, actionAgg.ComboExamples...)
		}

		// 限制combo示例数量
		if len(comboExamples) > 6 {
			comboExamples = comboExamples[:6]
		}

		// 生成训练数据
		training := TrainingData{
			Instruction: "你是一名德州扑克 GTO 策略助手。根据当前的牌局状态（翻牌圈），请为该位置玩家提供最优的 GTO 行动策略建议。",
			Input: InputData{
				GameStage:                      "翻牌圈",
				Board:                          boardStr,
				PlayerPosition:                 playerPos,
				OpponentPosition:               opponentPos,
				PlayerIsOOP:                    key.IPOrOOP == "OOP",
				CurrentNodeActionHistoryOnFlop: parseActionHistory(key.NodePrefix, key.IPOrOOP),
				SPRAtDecisionPoint:             spr,
				BoardTextureSummary:            analyzeBoardTexture(boardStr),
			},
			Output: OutputData{
				GTOStrategyDistribution:     strategies,
				RepresentativeComboExamples: comboExamples,
				OverallNodeEVBB:             totalEV,
			},
		}

		trainingData = append(trainingData, training)
	}

	return trainingData
}

// 辅助函数
func extractSizeFromAction(action string) float64 {
	// 从动作字符串中提取下注的筹码数量
	// 注意：这里提取的是实际的筹码数，不是百分比
	// 例如：
	// - "bet75" -> 75 (表示下注75个筹码)
	// - "raise150" -> 150 (表示加注到150个筹码)
	// - "bet100" -> 100 (表示下注100个筹码)
	// 这个值仅用于展示动作的大小，实际的下注占底池比例(bet_pct)在calculateBetMetrics中计算
	re := regexp.MustCompile(`(\d+)`)
	matches := re.FindStringSubmatch(action)
	if len(matches) > 1 {
		if size, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return size
		}
	}
	return 0
}

func normalizeActionType(action string) string {
	action = strings.ToLower(action)
	if strings.Contains(action, "bet") || strings.Contains(action, "raise") {
		return "raise"
	} else if strings.Contains(action, "call") {
		return "call"
	} else if strings.Contains(action, "check") {
		return "check"
	} else if strings.Contains(action, "fold") {
		return "fold"
	}
	return action
}

func getComboNote(freq float64, ev float64, action string) string {
	// 特殊处理fold动作
	if strings.ToLower(action) == "fold" || strings.Contains(strings.ToLower(action), "fold") {
		if freq >= 0.8 {
			return "高频 弃牌"
		} else if freq >= 0.5 {
			return "中频 弃牌"
		} else {
			return "低频 弃牌"
		}
	}

	// 根据频率和EV综合判断
	if freq >= 0.8 {
		if ev >= 1.0 {
			return "高频 价值"
		} else if ev >= -0.5 {
			return "高频 平衡"
		} else {
			return "高频 诈唬"
		}
	} else if freq >= 0.5 {
		if ev >= 0.5 {
			return "中频 价值"
		} else {
			return "中频 混合"
		}
	} else if freq >= 0.2 {
		if ev >= 0 {
			return "低频 价值"
		} else {
			return "低频 诈唬"
		}
	} else {
		// 频率小于20%
		if ev >= 0 {
			return "偶尔 价值"
		} else {
			return "偶尔 诈唬"
		}
	}
}

func parsePositionsFromTableName(tableName string) (string, string) {
	// 位置优先级表（数字越小，位置越靠前，越容易是OOP）
	positionPriority := map[string]int{
		"sb":  0,
		"bb":  1,
		"utg": 2,
		"mp":  3,
		"co":  4,
		"btn": 5,
		"bu":  5, // btn的别名
	}

	// 从表名解析位置信息，如 flop_40bb_co_bb -> BB, CO
	parts := strings.Split(tableName, "_")
	if len(parts) >= 4 {
		// 提取两个位置
		pos1 := strings.ToLower(parts[2])
		pos2 := strings.ToLower(parts[3])

		// 获取优先级
		priority1, ok1 := positionPriority[pos1]
		priority2, ok2 := positionPriority[pos2]

		// 如果两个位置都有效
		if ok1 && ok2 {
			// 优先级小的是OOP（位置靠前），返回格式为 (玩家位置, 对手位置)
			// 通常数据库中是以后位玩家视角（如CO vs BB中，CO是玩家）
			if priority1 > priority2 {
				// pos1优先级更大（位置更靠后），所以pos1是玩家（IP）
				return strings.ToUpper(pos1), strings.ToUpper(pos2)
			} else {
				// pos2优先级更大（位置更靠后），所以pos2是玩家（通常是OOP）
				return strings.ToUpper(pos2), strings.ToUpper(pos1)
			}
		}

		// 如果无法识别，尝试返回原始值
		return strings.ToUpper(pos2), strings.ToUpper(pos1)
	}

	// 默认值
	return "BB", "CO"
}

func parseActionHistory(nodePrefix, ipOrOop string) string {
	// 解析节点前缀生成动作历史描述
	if nodePrefix == "r:0" {
		return "游戏开始"
	}

	// 移除 "r:0:" 前缀
	if strings.HasPrefix(nodePrefix, "r:0:") {
		nodePrefix = strings.TrimPrefix(nodePrefix, "r:0:")
	}

	// 分割动作序列
	actions := strings.Split(nodePrefix, ":")
	if len(actions) == 0 {
		return "游戏开始"
	}

	// 构建动作历史描述
	history := []string{}

	// 根据当前节点的IPOrOOP判断第一个行动者
	// 如果当前是IP决策，说明之前的行动序列最后是OOP行动，所以第一个行动者是OOP
	// 如果当前是OOP决策，说明之前的行动序列最后是IP行动，所以第一个行动者也是OOP（翻牌圈总是OOP先行动）
	currentPosition := "OOP" // 翻牌圈第一个行动者总是OOP

	for _, action := range actions {
		if action == "" {
			continue
		}

		actionDesc := ""
		if action == "c" {
			actionDesc = fmt.Sprintf("%s 过牌", currentPosition)
		} else if action == "f" {
			actionDesc = fmt.Sprintf("%s 弃牌", currentPosition)
		} else if strings.HasPrefix(action, "b") {
			// 提取下注大小
			betSize := strings.TrimPrefix(action, "b")
			if betSize != "" {
				actionDesc = fmt.Sprintf("%s 下注 %s 个筹码", currentPosition, betSize)
			} else {
				actionDesc = fmt.Sprintf("%s 下注", currentPosition)
			}
		} else if strings.HasPrefix(action, "r") {
			// 提取加注大小
			raiseSize := strings.TrimPrefix(action, "r")
			if raiseSize != "" {
				actionDesc = fmt.Sprintf("%s 加注到 %s 个筹码", currentPosition, raiseSize)
			} else {
				actionDesc = fmt.Sprintf("%s 加注", currentPosition)
			}
		} else {
			// 数字通常表示下注/加注大小（在convertNodePath后的格式）
			actionDesc = fmt.Sprintf("%s 下注 %s 个筹码", currentPosition, action)
		}

		if actionDesc != "" {
			history = append(history, actionDesc)
			// 切换位置
			if currentPosition == "OOP" {
				currentPosition = "IP"
			} else {
				currentPosition = "OOP"
			}
		}
	}

	if len(history) == 0 {
		return "游戏开始"
	}

	return strings.Join(history, "，")
}

// parseCard 解析单张牌，返回牌面值和花色
func parseCard(card string) (rank string, suit string) {
	card = strings.TrimSpace(card)
	if len(card) < 2 {
		return "", ""
	}

	// 处理标准格式（如 As, Kh, Td）
	if len(card) == 2 {
		return card[:1], card[1:]
	}

	// 处理10的特殊情况（如 10s, 10h）
	if len(card) == 3 && card[:2] == "10" {
		return "T", card[2:]
	}

	// 其他情况返回空
	return "", ""
}

func analyzeBoardTexture(boardStr string) BoardTexture {
	// 分析牌面结构
	cards := strings.Fields(strings.TrimSpace(boardStr))

	texture := BoardTexture{
		Type:          "低张",
		Suitedness:    "彩虹",
		Connectedness: "无顺子听牌",
		IsPaired:      false,
	}

	if len(cards) >= 3 {
		// 解析每张牌的点数和花色
		ranks := make(map[string]int)
		suits := make(map[string]int)
		rankValues := []int{}

		for _, card := range cards {
			rank, suit := parseCard(card)
			if rank != "" && suit != "" {
				ranks[rank]++
				suits[suit]++

				// 转换牌面值
				rankValue := getRankValue(rank)
				rankValues = append(rankValues, rankValue)
			}
		}

		// 排序牌面值
		sort.Sort(sort.Reverse(sort.IntSlice(rankValues)))

		// 先检查配对情况
		maxRankCount := 0
		for _, count := range ranks {
			if count > maxRankCount {
				maxRankCount = count
			}
		}

		// 优先判断三条和对子
		if maxRankCount == 3 {
			texture.Type = "三条"
			texture.IsPaired = true
		} else if maxRankCount == 2 {
			texture.IsPaired = true
			// 对子情况下，再判断高中低张
			highestRank := rankValues[0]
			if highestRank >= 12 { // Q或更高
				texture.Type = "高张对子"
			} else if highestRank >= 9 { // 9-J
				texture.Type = "中张对子"
			} else {
				texture.Type = "低张对子"
			}
		} else {
			// 无对子情况，判断高中低张
			highestRank := rankValues[0]
			if highestRank >= 12 { // Q或更高
				texture.Type = "高张"
			} else if highestRank >= 9 { // 9-J
				texture.Type = "中张"
			} else {
				texture.Type = "低张"
			}

			// 特殊情况：如果有A但是轮牌（A-2-3等），仍然算作低张结构
			if len(rankValues) >= 3 && rankValues[0] == 14 && rankValues[1] <= 5 {
				texture.Type = "低张轮牌"
			}
		}

		// 检查花色
		maxSuitCount := 0
		for _, count := range suits {
			if count > maxSuitCount {
				maxSuitCount = count
			}
		}

		if maxSuitCount == 3 {
			texture.Suitedness = "三同花"
		} else if maxSuitCount == 2 {
			texture.Suitedness = "两张同花"
		} else {
			texture.Suitedness = "彩虹"
		}

		// 检查顺子结构
		if len(rankValues) >= 3 {
			// 检查是否有顺子或顺子听牌
			connectedness := checkConnectedness(rankValues)
			texture.Connectedness = connectedness
		}
	}

	return texture
}

// getRankValue 将牌面转换为数值
func getRankValue(rank string) int {
	switch rank {
	case "A":
		return 14
	case "K":
		return 13
	case "Q":
		return 12
	case "J":
		return 11
	case "T":
		return 10
	default:
		if val, err := strconv.Atoi(rank); err == nil {
			return val
		}
		return 0
	}
}

// checkConnectedness 检查牌面的连续性
func checkConnectedness(ranks []int) string {
	if len(ranks) < 3 {
		return "无顺子听牌"
	}

	// 检查三张连续（如J-T-9）
	if ranks[0]-ranks[1] == 1 && ranks[1]-ranks[2] == 1 {
		return "三张连续"
	}

	// 检查强顺子听牌（一个间隔，如J-T-8 或 J-9-8）
	gap1 := ranks[0] - ranks[1]
	gap2 := ranks[1] - ranks[2]
	totalGap := ranks[0] - ranks[2]

	// 强顺子听牌：总间隔为3且有一个间隔为1或2
	if totalGap == 3 && (gap1 <= 2 || gap2 <= 2) {
		return "强顺子听牌"
	}

	// 检查两张连续（需要确保不是已经判断过的情况）
	if gap1 == 1 || gap2 == 1 {
		return "两张连续"
	}

	// 弱顺子听牌（总间隔4或更少，但不满足上述条件）
	if totalGap <= 4 {
		return "弱顺子听牌"
	}

	// 特殊情况：A可以和小牌组成顺子（A-2-3-4-5）
	if ranks[0] == 14 && ranks[len(ranks)-1] <= 5 {
		// 检查是否有轮牌顺子结构
		smallCards := []int{}
		hasAce := false
		for _, r := range ranks {
			if r == 14 {
				hasAce = true
			} else if r <= 5 {
				smallCards = append(smallCards, r)
			}
		}
		if hasAce && len(smallCards) >= 2 {
			// 检查小牌之间的连续性
			if len(smallCards) >= 2 && smallCards[0]-smallCards[1] <= 2 {
				return "轮牌顺子听牌"
			}
		}
	}

	return "无顺子听牌"
}

func writeSimpleJSONLFile(data []SimpleTrainingData, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, item := range data {
		err := encoder.Encode(item)
		if err != nil {
			return err
		}
	}

	return nil
}

func splitSimpleEvalData(data []SimpleTrainingData, ratio float64) []SimpleTrainingData {
	evalSize := int(float64(len(data)) * ratio)
	if evalSize == 0 {
		return []SimpleTrainingData{}
	}

	// 简单的随机分割，实际可以使用更好的随机化方法
	return data[:evalSize]
}

// JSONL训练数据相关结构体
type BoardTexture struct {
	Type          string `json:"type"`
	Suitedness    string `json:"suitedness"`
	Connectedness string `json:"connectedness"`
	IsPaired      bool   `json:"is_paired"`
}

type InputData struct {
	GameStage                      string       `json:"game_stage"`
	Board                          string       `json:"board"`
	PlayerPosition                 string       `json:"player_position"`
	OpponentPosition               string       `json:"opponent_position"`
	PlayerIsOOP                    bool         `json:"player_is_oop"`
	CurrentNodeActionHistoryOnFlop string       `json:"current_node_action_history_on_flop"`
	SPRAtDecisionPoint             float64      `json:"spr_at_decision_point"`
	BoardTextureSummary            BoardTexture `json:"board_texture_summary"`
}

type ActionStrategy struct {
	ActionType   string  `json:"action_type"`
	SizePctPot   float64 `json:"size_pct_pot,omitempty"`
	FrequencyPct float64 `json:"frequency_pct"`
	AverageEVBB  float64 `json:"average_ev_bb"`
}

type ComboExample struct {
	Combo        string  `json:"combo"`
	Action       string  `json:"action"`
	FrequencyPct float64 `json:"frequency_pct"`
	Note         string  `json:"note"`
}

type OutputData struct {
	GTOStrategyDistribution     []ActionStrategy `json:"gto_strategy_distribution"`
	RepresentativeComboExamples []ComboExample   `json:"representative_combo_examples"`
	OverallNodeEVBB             float64          `json:"overall_node_ev_bb"`
}

type TrainingData struct {
	Instruction string     `json:"instruction"`
	Input       InputData  `json:"input"`
	Output      OutputData `json:"output"`
}

// 数据库记录结构体
type DBRecord struct {
	NodePrefix string  `json:"node_prefix"`
	BetLevel   int     `json:"bet_level"`
	BoardID    int     `json:"board_id"`
	ComboID    int     `json:"combo_id"`
	ComboStr   string  `json:"combo_str"`
	BoardStr   string  `json:"board_str"`
	IPOrOOP    string  `json:"ip_or_oop"`
	StackDepth float64 `json:"stack_depth"`
	BetPct     float64 `json:"bet_pct"`
	SPR        float64 `json:"spr"`
	Action1    string  `json:"action1"`
	Freq1      float64 `json:"freq1"`
	EV1        float64 `json:"ev1"`
	EQ1        float64 `json:"eq1"`
	Action2    string  `json:"action2"`
	Freq2      float64 `json:"freq2"`
	EV2        float64 `json:"ev2"`
	EQ2        float64 `json:"eq2"`
}

// 聚合键结构体
type AggregationKey struct {
	NodePrefix string
	BoardID    int
	IPOrOOP    string
	StackDepth float64
	BetPct     float64
}

// 动作聚合数据
type ActionAggregation struct {
	ActionType    string
	SizePctPot    float64
	TotalFreq     float64
	TotalEV       float64
	ComboCount    int
	ComboExamples []ComboExample
}

// 简化的训练数据结构，每个手牌一个样本
type SimpleTrainingData struct {
	Board               string       `json:"board"`
	HoleCards           string       `json:"hole_cards"`
	PlayerPosition      string       `json:"player_position"`
	OpponentPosition    string       `json:"opponent_position"`
	PlayerIsOOP         bool         `json:"player_is_oop"`
	SPR                 float64      `json:"spr"`
	BoardTextureSummary BoardTexture `json:"board_texture_summary"`
	ActionHistory       string       `json:"action_history"`
	GTOAction           string       `json:"gto_action"`
	FrequencyPct        float64      `json:"frequency_pct"`
	EV                  float64      `json:"ev"`

	// 新增字段，提高泛化能力
	HandFeatures HandFeatures `json:"hand_features"` // 手牌特征
	Equity       float64      `json:"equity"`        // 手牌胜率（原EQ字段）
	PotOdds      float64      `json:"pot_odds"`      // 底池赔率（面对下注时需要的赔率）
	StackDepth   float64      `json:"stack_depth"`   // 有效筹码深度
	BetLevel     int          `json:"bet_level"`     // 当前下注轮次
	BetPct       float64      `json:"bet_pct"`       // 最近下注占底池比例
}

// 手牌特征结构体
type HandFeatures struct {
	IsPair            bool   `json:"is_pair"`             // 是否口袋对
	IsSuited          bool   `json:"is_suited"`           // 是否同花
	IsConnected       bool   `json:"is_connected"`        // 是否顺连张（间隔0）
	IsSemiConnected   bool   `json:"is_semi_connected"`   // 是否半连张（间隔1-2）
	HighCardRank      int    `json:"high_card_rank"`      // 最大牌点数(2-14)
	LowCardRank       int    `json:"low_card_rank"`       // 最小牌点数(2-14)
	Gap               int    `json:"gap"`                 // 间隔数
	HandCategory      string `json:"hand_category"`       // 手牌分类：premium/strong/medium/weak
	HandStrengthScore int    `json:"hand_strength_score"` // 手牌强度数值：4=premium, 3=strong, 2=medium, 1=weak
	ConnectorType     string `json:"connector_type"`      // 连接类型：connected/one_gap/two_gap/none
	HasStraightDraw   bool   `json:"has_straight_draw"`   // 是否有顺子听牌
	HasFlushDraw      bool   `json:"has_flush_draw"`      // 是否有同花听牌
	MadeHandType      string `json:"made_hand_type"`      // 成牌类型：high_card/pair/two_pair/set/straight/flush等
}

// analyzeHandFeatures 分析手牌特征
func analyzeHandFeatures(handStr string, boardStr string) HandFeatures {
	features := HandFeatures{}

	// 解析手牌（格式如 "AhKs" 或 "Ah Ks"）
	handStr = strings.ReplaceAll(handStr, " ", "")
	if len(handStr) < 4 {
		return features
	}

	// 提取两张牌
	card1 := handStr[:2]
	card2 := handStr[2:4]

	rank1, suit1 := parseCard(card1)
	rank2, suit2 := parseCard(card2)

	// 获取牌面值
	rankValue1 := getRankValue(rank1)
	rankValue2 := getRankValue(rank2)

	// 设置高低牌
	if rankValue1 >= rankValue2 {
		features.HighCardRank = rankValue1
		features.LowCardRank = rankValue2
	} else {
		features.HighCardRank = rankValue2
		features.LowCardRank = rankValue1
	}

	// 判断是否口袋对
	features.IsPair = (rankValue1 == rankValue2)

	// 判断是否同花
	features.IsSuited = (suit1 == suit2)

	// 计算间隔
	features.Gap = features.HighCardRank - features.LowCardRank - 1
	if features.Gap < 0 {
		features.Gap = 0
	}

	// 判断连接类型
	if features.IsPair {
		features.ConnectorType = "pair"
		features.IsConnected = false
		features.IsSemiConnected = false
	} else if features.Gap == 0 {
		features.ConnectorType = "connected"
		features.IsConnected = true
		features.IsSemiConnected = false
	} else if features.Gap == 1 {
		features.ConnectorType = "one_gap"
		features.IsConnected = false
		features.IsSemiConnected = true
	} else if features.Gap == 2 {
		features.ConnectorType = "two_gap"
		features.IsConnected = false
		features.IsSemiConnected = true
	} else {
		features.ConnectorType = "none"
		features.IsConnected = false
		features.IsSemiConnected = false
	}

	// 手牌分类
	features.HandCategory = classifyHand(features.HighCardRank, features.LowCardRank, features.IsPair, features.IsSuited)

	// 设置数值评分
	switch features.HandCategory {
	case "premium":
		features.HandStrengthScore = 4
	case "strong":
		features.HandStrengthScore = 3
	case "medium":
		features.HandStrengthScore = 2
	case "weak":
		features.HandStrengthScore = 1
	default:
		features.HandStrengthScore = 1
	}

	// 分析在当前牌面的听牌和成牌情况
	if boardStr != "" {
		features.HasStraightDraw = checkStraightDraw(handStr, boardStr)
		features.HasFlushDraw = checkFlushDraw(handStr, boardStr)
		features.MadeHandType = evaluateMadeHand(handStr, boardStr)
	}

	return features
}

// classifyHand 对手牌进行分类
func classifyHand(highRank, lowRank int, isPair, isSuited bool) string {
	// AA, KK, QQ, AKs
	if isPair && highRank >= 12 { // QQ+
		return "premium"
	}
	if highRank == 14 && lowRank == 13 && isSuited { // AKs
		return "premium"
	}

	// JJ, TT, 99, AK, AQs, AJs, KQs
	if isPair && highRank >= 9 { // 99+
		return "strong"
	}
	if highRank == 14 && lowRank >= 12 { // AQ+
		return "strong"
	}
	if highRank == 14 && lowRank == 11 && isSuited { // AJs
		return "strong"
	}
	if highRank == 13 && lowRank == 12 && isSuited { // KQs
		return "strong"
	}

	// 中等牌力：中小对子、同花连张、Ax
	if isPair && highRank >= 6 { // 66+
		return "medium"
	}
	if highRank == 14 { // Any Ax
		return "medium"
	}
	if isSuited && (highRank-lowRank) <= 2 && highRank >= 9 { // 同花连张或间隔张
		return "medium"
	}

	// 其他都是弱牌
	return "weak"
}

// checkStraightDraw 检查是否有顺子听牌
func checkStraightDraw(handStr, boardStr string) bool {
	// 简化实现：检查是否有4张牌能组成顺子
	// 实际实现需要更复杂的逻辑
	allCards := handStr + strings.ReplaceAll(boardStr, " ", "")

	// 提取所有牌的点数
	ranks := make(map[int]bool)
	for i := 0; i < len(allCards); i += 2 {
		if i+1 < len(allCards) {
			rank, _ := parseCard(allCards[i : i+2])
			rankValue := getRankValue(rank)
			ranks[rankValue] = true
		}
	}

	// 检查是否有4张连续或接近连续的牌
	for start := 14; start >= 5; start-- {
		count := 0
		for i := 0; i < 5; i++ {
			if ranks[start-i] || (start-i == 1 && ranks[14]) { // A可以当1用
				count++
			}
		}
		if count >= 4 {
			return true
		}
	}

	return false
}

// checkFlushDraw 检查是否有同花听牌
func checkFlushDraw(handStr, boardStr string) bool {
	// 统计各花色数量
	suits := make(map[string]int)

	// 统计手牌花色
	handStr = strings.ReplaceAll(handStr, " ", "")
	for i := 0; i < len(handStr); i += 2 {
		if i+1 < len(handStr) {
			_, suit := parseCard(handStr[i : i+2])
			suits[suit]++
		}
	}

	// 统计公牌花色
	cards := strings.Fields(boardStr)
	for _, card := range cards {
		_, suit := parseCard(card)
		suits[suit]++
	}

	// 检查是否有4张同花
	for _, count := range suits {
		if count >= 4 {
			return true
		}
	}

	return false
}

// evaluateMadeHand 评估成牌类型
func evaluateMadeHand(handStr, boardStr string) string {
	// 简化实现，实际需要完整的牌力评估算法
	handStr = strings.ReplaceAll(handStr, " ", "")
	rank1, _ := parseCard(handStr[:2])
	rank2, _ := parseCard(handStr[2:4])

	// 检查是否成对
	boardRanks := make(map[string]int)
	cards := strings.Fields(boardStr)
	for _, card := range cards {
		rank, _ := parseCard(card)
		boardRanks[rank]++
	}

	// 检查三条
	if boardRanks[rank1] == 2 || boardRanks[rank2] == 2 {
		return "set"
	}

	// 检查两对
	pairCount := 0
	if rank1 == rank2 {
		pairCount++
	}
	if boardRanks[rank1] == 1 {
		pairCount++
	}
	if boardRanks[rank2] == 1 && rank1 != rank2 {
		pairCount++
	}

	if pairCount >= 2 {
		return "two_pair"
	}

	// 检查一对
	if pairCount == 1 || boardRanks[rank1] == 1 || boardRanks[rank2] == 1 {
		return "pair"
	}

	// TODO: 检查顺子、同花等

	return "high_card"
}

// extractLastActionSize 从节点路径中提取最后一个动作的大小（占底池百分比）
func extractLastActionSize(nodePrefix string) float64 {
	// 移除 "r:0:" 前缀
	if strings.HasPrefix(nodePrefix, "r:0:") {
		nodePrefix = strings.TrimPrefix(nodePrefix, "r:0:")
	}

	// 分割动作序列
	actions := strings.Split(nodePrefix, ":")
	if len(actions) == 0 {
		return 0
	}

	// 从后往前查找最后一个下注/加注动作
	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if strings.HasPrefix(action, "b") || strings.HasPrefix(action, "r") {
			// 提取数字
			sizeStr := strings.TrimPrefix(action, "b")
			sizeStr = strings.TrimPrefix(sizeStr, "r")
			if size, err := strconv.ParseFloat(sizeStr, 64); err == nil {
				// TODO: 这里需要计算实际的底池大小来得到准确的百分比
				// 目前简化处理，假设是标准下注大小
				if size <= 33 {
					return 33.0 // 33% pot
				} else if size <= 50 {
					return 50.0 // 50% pot
				} else if size <= 75 {
					return 75.0 // 75% pot
				} else if size <= 100 {
					return 100.0 // 100% pot
				} else {
					return 150.0 // 150% pot
				}
			}
		}
	}

	return 0
}
