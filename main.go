package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"piodatasolver/internal/cache"
	"piodatasolver/internal/upi"
	"piodatasolver/internal/util"
	"piodatasolver/model"
)

var handOrder *cache.HandOrder

// CFR文件路径 - 用于生成输出文件名
var cfrFilePath string

// 目标手牌，用于调试
var targetHand = "5h4d"

// 全局变量，用于统计过滤的动作数量
var (
	filteredActionCount int = 0
)

func main() {

	client := upi.NewClient("./PioSOLVER3-edge.exe", `E:\zdsbddz\piosolver\piosolver3`)

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

	// 初始化HandOrder
	if err := handOrder.Init(client); err != nil {
		log.Fatalf("初始化HandOrder失败: %v", err)
	}

	// 加载树并保存CFR文件路径
	cfrFilePath = `E:\zdsbddz\piosolver\piosolver3\saves\asth4d.cfr`
	_, err = client.LoadTree(cfrFilePath)
	if err != nil {
		log.Fatalf("加载树失败: %v", err)
	}

	// 创建输出目录
	err = os.MkdirAll("data", 0755)
	if err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	// 设置目标节点
	targetNode := "r:0"

	// 打印目标手牌的原始UPI数据
	// printTargetHandRawData(client, targetNode)

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

	for _, resp := range responses {
		log.Printf("执行指令响应: %s", resp)
	}

	actor := responses[1]
	board := responses[2]
	pot := responses[3]

	// 调试信息：当前节点的基本信息
	log.Printf("调试[%s]：节点=%s, 行动方=%s, 公牌=%s, 底池=%s", targetHand, node, actor, board, pot)

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

				// 记录添加了频率为0的动作
				if freq == 0 {
					log.Printf("添加频率为0的%s动作到手牌 %s", action.Label, hand)
				}

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

			// 获取目标手牌在手牌列表中的索引位置
			targetIdx := -1
			for j, hand := range handCards {
				if hand == targetHand {
					targetIdx = j
					break
				}
			}

			// 如果找到了目标手牌的位置，打印其原始EV值
			if targetIdx >= 0 && targetIdx < len(ev_split) {
				rawEV := ev_split[targetIdx]
				log.Printf("调试[%s]：动作=%s, 原始EV值=%s", targetHand, action.Label, rawEV)
			}

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

						// 调试目标手牌
						if hand == targetHand {
							log.Printf("调试[%s]：节点=%s, 动作=%s, 更新EV值=%.2f", targetHand, node, action.Label, ev)
						}
						break
					}
				}
			}
		}

		//calc_eq_node 计算当前节点下1326手牌的胜率，只取第一行的eq值
		cmd = fmt.Sprintf("calc_eq_node %s %s", actorCmd, node)
		log.Printf("调试[%s]：执行命令 %s", targetHand, cmd)
		eq_lines, err := client.ExecuteCommand(cmd, 10*time.Second)
		if err != nil {
			log.Printf("执行指令calc_eq_node失败: %v，跳过EQ处理", err)
		} else if len(eq_lines) == 0 || strings.Contains(eq_lines[0], "ERROR") {
			log.Printf("calc_eq_node返回错误或为空: %v，跳过EQ处理", eq_lines)
		} else {
			//只读取第一行的数据
			eq_line := eq_lines[0]
			eq_split := strings.Fields(eq_line)

			// 获取目标手牌在手牌列表中的索引位置
			targetIdx := -1
			for j, hand := range handCards {
				if hand == targetHand {
					targetIdx = j
					break
				}
			}

			// 如果找到了目标手牌的位置，打印其原始EQ值
			if targetIdx >= 0 && targetIdx < len(eq_split) {
				rawEQ := eq_split[targetIdx]
				log.Printf("调试[%s]：原始EQ值=%s", targetHand, rawEQ)
			}

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

					// 调试目标手牌
					if hand == targetHand {
						log.Printf("调试[%s]：节点=%s, 动作=%s, 更新EQ值=%.4f", targetHand, node, record.Actions[k].Label, eq)
					}
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

			// 只有当三个值都无效时才过滤
			if freqIsInvalid && evIsInvalid && eqIsInvalid {
				if hand == targetHand {
					log.Printf("过滤掉手牌 %s 的全无效动作: freq=%.6f, ev=%.2f, eq=%.6f",
						targetHand, action.Freq, action.Ev, action.Eq)
				}
				filteredActionCount++ // 增加过滤计数
				continue
			}

			validActions = append(validActions, action)
		}

		// 更新record的Actions
		record.Actions = validActions

		// 只有当有有效Action时，才添加到finalRecords
		if len(record.Actions) > 0 {
			finalRecords = append(finalRecords, record)
		}
	}

	// 打印JSON格式并写入到文件
	if len(finalRecords) > 0 {
		// 从CFR文件路径提取文件名
		_, cfrFileName := filepath.Split(cfrFilePath)
		cfrFileName = strings.TrimSuffix(cfrFileName, filepath.Ext(cfrFileName))

		// 构建输出文件路径
		outputPath := filepath.Join("data", cfrFileName+".json")
		log.Printf("准备写入数据到文件: %s, 记录数: %d", outputPath, len(finalRecords))

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

			// 创建或覆盖文件
			err = os.WriteFile(outputPath, jsonData, 0644)
			if err != nil {
				log.Printf("写入文件失败: %v", err)
				return
			}

			log.Printf("已将数据写入到文件: %s，大小: %d 字节", outputPath, len(jsonData))

			// 打印总结信息
			log.Printf("处理完成根节点 %s，数据已保存到文件中", node)
		} else {
			// 如果不是根节点，尝试读取现有文件
			var existingRecords []*model.Record

			fileData, err := os.ReadFile(outputPath)
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
			err = os.WriteFile(outputPath, jsonData, 0644)
			if err != nil {
				log.Printf("写入文件失败: %v", err)
				return
			}

			log.Printf("已更新文件数据: %s，大小: %d 字节", outputPath, len(jsonData))
		}
	}

	// 打印前20条记录作为示例
	recordLimit := 20
	if len(finalRecords) < recordLimit {
		recordLimit = len(finalRecords)
	}

	log.Printf("共有记录 %d 条，前 %d 条如下:", len(finalRecords), recordLimit)
	for i := 0; i < recordLimit; i++ {
		record := finalRecords[i]
		// 专门详细打印目标手牌的完整记录
		if record.Hand == targetHand {
			log.Printf("===== 目标手牌 %s 完整记录 =====", targetHand)
			log.Printf("节点: %s", record.Node)
			log.Printf("行动方: %s", record.Actor)
			log.Printf("公牌: %s", record.Board)
			log.Printf("动作数量: %d（已过滤全0动作）", len(record.Actions))

			for j, action := range record.Actions {
				log.Printf("  动作 #%d:", j+1)
				log.Printf("    标签: %s", action.Label)
				log.Printf("    频率: %.4f", action.Freq)
				log.Printf("    期望值: %.2f", action.Ev)
				log.Printf("    胜率: %.4f", action.Eq)
				log.Printf("    子节点ID: %s", action.ChildNodeID)
			}
			log.Println("==============================")
		}

		log.Printf("=== 记录 #%d ===", i+1)
		log.Printf("节点: %s", record.Node)
		log.Printf("行动方: %s", record.Actor)
		log.Printf("公牌: %s", record.Board)
		log.Printf("手牌: %s", record.Hand)
		log.Printf("动作数量: %d", len(record.Actions))

		for j, action := range record.Actions {
			log.Printf("  动作 #%d:", j+1)
			log.Printf("    标签: %s", action.Label)
			log.Printf("    频率: %.4f", action.Freq)
			log.Printf("    期望值: %.2f", action.Ev)
			log.Printf("    胜率: %.4f", action.Eq)
		}
		log.Println()
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

// 直接查询目标手牌的EV值
func directQueryTargetHand(client *upi.Client, node string) {
	log.Printf("======= 开始直接查询手牌 %s 的EV值 =======", targetHand)

	// 获取节点信息
	cmd := fmt.Sprintf("show_node %s", node)
	responses, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil || len(responses) < 4 {
		log.Printf("获取节点信息失败: %v", err)
		return
	}

	actor := responses[1]
	var actorCmd string
	if actor == "IP_DEC" {
		actorCmd = "IP"
	} else if actor == "OOP_DEC" {
		actorCmd = "OOP"
	} else {
		log.Printf("未知的行动方: %s", actor)
		return
	}

	// 获取子节点
	cmd = fmt.Sprintf("show_children %s", node)
	responses, err = client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Printf("获取子节点失败: %v", err)
		return
	}

	// 存储子节点ID
	var childNodeIDs []string
	for i := 0; i < len(responses); i += 7 {
		if i+1 < len(responses) {
			childNodeIDs = append(childNodeIDs, responses[i+1])
		}
	}

	// 找出目标手牌在手牌列表中的索引
	targetIdx := -1
	handCards := handOrder.Order()
	for i, card := range handCards {
		if card == targetHand {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		log.Printf("未找到手牌 %s 在HandOrder中的位置", targetHand)
		return
	}

	// 使用get_range_ev命令，该命令返回一个手牌在特定节点的期望值
	cmd = fmt.Sprintf("get_range_ev %s %s %s", actorCmd, node, targetHand)
	log.Printf("执行命令: %s", cmd)
	responses, err = client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil || len(responses) == 0 {
		log.Printf("直接查询EV失败: %v", err)
		return
	}

	log.Printf("直接查询手牌 %s 的EV值结果: %v", targetHand, responses)

	// 使用calc_individual_ev命令查询AhAd手牌的具体EV值
	cmd = fmt.Sprintf("calc_individual_ev %s", targetHand)
	log.Printf("执行命令: %s", cmd)
	responses, err = client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Printf("使用calc_individual_ev查询失败: %v", err)
	} else {
		log.Printf("calc_individual_ev结果: %v", responses)
	}

	// 对每个子节点尝试计算手牌的EV值
	for _, childNodeID := range childNodeIDs {
		// 先获取子节点对应的动作类型(call/fold)
		label := "未知"
		if strings.HasSuffix(childNodeID, ":c") {
			label = "call"
		} else if strings.HasSuffix(childNodeID, ":f") {
			label = "fold"
		}

		// 使用calc_ev命令查询特定子节点的EV值
		cmd = fmt.Sprintf("calc_ev %s %s", actorCmd, childNodeID)
		log.Printf("对子节点 %s (动作: %s) 执行命令: %s", childNodeID, label, cmd)
		responses, err = client.ExecuteCommand(cmd, 10*time.Second)
		if err != nil {
			log.Printf("查询子节点EV失败: %v", err)
			continue
		}

		// 从EVs中提取目标手牌的值
		if len(responses) > 0 {
			evs := strings.Fields(responses[0])
			if targetIdx < len(evs) {
				ev := evs[targetIdx]
				log.Printf("手牌 %s 在子节点 %s (动作: %s) 的EV值: %s", targetHand, childNodeID, label, ev)

				// 尝试使用calc_individual_ev_at_node命令（如果PioSOLVER支持）
				cmd = fmt.Sprintf("calc_individual_ev_at_node %s %s %s", actorCmd, childNodeID, targetHand)
				log.Printf("尝试命令: %s", cmd)
				indivResponses, err := client.ExecuteCommand(cmd, 10*time.Second)
				if err != nil {
					log.Printf("尝试calc_individual_ev_at_node失败: %v", err)
				} else {
					log.Printf("手牌 %s 在子节点 %s 使用calc_individual_ev_at_node的结果: %v",
						targetHand, childNodeID, indivResponses)
				}
			}
		}
	}

	// 尝试直接使用show_strategy_line查询特定手牌的策略
	cmd = fmt.Sprintf("show_strategy_line %s %s", node, targetHand)
	log.Printf("执行命令: %s", cmd)
	responses, err = client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Printf("使用show_strategy_line查询失败: %v", err)
	} else {
		log.Printf("show_strategy_line结果: %v", responses)
	}

	// 尝试手动计算EV
	log.Printf("开始手动计算手牌 %s 的EV值...", targetHand)
	var weightedEv float64
	var totalFreq float64

	// 获取策略频率
	cmd = fmt.Sprintf("show_strategy %s", node)
	strategyResp, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err == nil && len(strategyResp) >= len(childNodeIDs) {
		for i, childID := range childNodeIDs {
			if i < len(strategyResp) {
				strats := strings.Fields(strategyResp[i])
				if targetIdx < len(strats) {
					freq, err := strconv.ParseFloat(strats[targetIdx], 64)
					if err == nil {
						// 获取该动作下的EV
						cmd = fmt.Sprintf("calc_ev %s %s", actorCmd, childID)
						evResp, err := client.ExecuteCommand(cmd, 10*time.Second)
						if err == nil && len(evResp) > 0 {
							evs := strings.Fields(evResp[0])
							if targetIdx < len(evs) {
								ev, err := strconv.ParseFloat(evs[targetIdx], 64)
								if err == nil {
									log.Printf("动作 %s: 频率 = %.4f, EV = %.2f", childID, freq, ev)
									weightedEv += freq * ev
									totalFreq += freq
								}
							}
						}
					}
				}
			}
		}

		if totalFreq > 0 {
			finalEv := weightedEv / totalFreq
			log.Printf("手牌 %s 的手动计算EV值 = %.2f (总频率 = %.4f)", targetHand, finalEv, totalFreq)
		}
	}

	log.Printf("======= 完成直接查询 =======")
}

// 打印目标手牌的原始UPI数据
func printTargetHandRawData(client *upi.Client, node string) {
	fmt.Printf("\n======== 打印手牌 %s 的原始UPI数据 ========\n\n", targetHand)

	// 1. 获取节点信息
	cmd := fmt.Sprintf("show_node %s", node)
	responses, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil || len(responses) < 4 {
		log.Fatalf("获取节点信息失败: %v", err)
	}

	fmt.Println("【节点信息】")
	for _, resp := range responses {
		fmt.Println(resp)
	}
	fmt.Println()

	// 确定行动方
	actor := responses[1]
	var actorCmd string
	if actor == "IP_DEC" {
		actorCmd = "IP"
	} else if actor == "OOP_DEC" {
		actorCmd = "OOP"
	} else {
		log.Fatalf("未知的行动方: %s", actor)
	}

	// 2. 获取子节点信息
	cmd = fmt.Sprintf("show_children %s", node)
	childResponses, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Fatalf("获取子节点失败: %v", err)
	}

	fmt.Println("【子节点信息】")
	for _, resp := range childResponses {
		fmt.Println(resp)
	}
	fmt.Println()

	// 解析出子节点ID
	var childNodeIDs []string
	var childLabels []string
	for i := 0; i < len(childResponses); i += 7 {
		if i+1 < len(childResponses) {
			nodeID := childResponses[i+1]
			// 提取动作标签
			var label string
			if strings.HasSuffix(nodeID, ":c") {
				label = "call"
			} else if strings.HasSuffix(nodeID, ":f") {
				label = "fold"
			} else if strings.HasSuffix(nodeID, ":k") {
				label = "check"
			} else if strings.Contains(nodeID, ":b") {
				label = "bet/raise"
			} else {
				label = "未知"
			}

			childNodeIDs = append(childNodeIDs, nodeID)
			childLabels = append(childLabels, label)
		}
	}

	// 3. 获取策略频率
	cmd = fmt.Sprintf("show_strategy %s", node)
	strategyResp, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Fatalf("获取策略频率失败: %v", err)
	}

	// 找出目标手牌在手牌列表中的索引
	targetIdx := -1
	handCards := handOrder.Order()
	for i, card := range handCards {
		if card == targetHand {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		log.Fatalf("未找到手牌 %s 在HandOrder中的位置", targetHand)
	}

	fmt.Println("【策略频率原始数据】")
	for i, resp := range strategyResp {
		if i < len(childLabels) {
			fmt.Printf("动作 %s (%s):\n%s\n", childNodeIDs[i], childLabels[i], resp)
		}
	}
	fmt.Println()

	// 提取目标手牌的频率数据
	fmt.Println("【目标手牌策略频率】")
	for i, resp := range strategyResp {
		if i < len(childLabels) && i < len(childNodeIDs) {
			fields := strings.Fields(resp)
			if targetIdx < len(fields) {
				freq, err := strconv.ParseFloat(fields[targetIdx], 64)
				if err != nil {
					fmt.Printf("动作 %s (%s): 无法解析频率\n", childNodeIDs[i], childLabels[i])
				} else {
					fmt.Printf("动作 %s (%s): 频率 = %.6f\n", childNodeIDs[i], childLabels[i], freq)
				}
			} else {
				fmt.Printf("动作 %s (%s): 索引超出范围\n", childNodeIDs[i], childLabels[i])
			}
		}
	}
	fmt.Println()

	// 4. 获取当前节点的EQ值
	cmd = fmt.Sprintf("calc_eq_node %s %s", actorCmd, node)
	eqResp, err := client.ExecuteCommand(cmd, 10*time.Second)
	if err != nil {
		log.Fatalf("获取EQ值失败: %v", err)
	}

	fmt.Println("【EQ原始数据】")
	for _, resp := range eqResp {
		fmt.Println(resp)
	}
	fmt.Println()

	// 提取目标手牌的EQ值
	if len(eqResp) > 0 {
		fields := strings.Fields(eqResp[0])
		if targetIdx < len(fields) {
			eq, err := strconv.ParseFloat(fields[targetIdx], 64)
			if err != nil {
				fmt.Printf("手牌 %s: EQ = 无法解析\n", targetHand)
			} else {
				fmt.Printf("手牌 %s: EQ = %.6f\n", targetHand, eq)
			}
		} else {
			fmt.Printf("手牌 %s: EQ索引超出范围\n", targetHand)
		}
	}
	fmt.Println()

	// 5. 获取各个子节点的EV值
	fmt.Println("【各动作EV原始数据】")
	for i, childID := range childNodeIDs {
		if i < len(childLabels) {
			cmd = fmt.Sprintf("calc_ev %s %s", actorCmd, childID)
			evResp, err := client.ExecuteCommand(cmd, 10*time.Second)
			if err != nil {
				fmt.Printf("获取动作 %s (%s) 的EV值失败: %v\n", childID, childLabels[i], err)
				continue
			}

			fmt.Printf("动作 %s (%s):\n", childID, childLabels[i])
			for _, resp := range evResp {
				fmt.Println(resp)
			}
			fmt.Println()

			// 提取目标手牌的EV值
			if len(evResp) > 0 {
				fields := strings.Fields(evResp[0])
				if targetIdx < len(fields) {
					ev, err := strconv.ParseFloat(fields[targetIdx], 64)
					if err != nil {
						fmt.Printf("动作 %s (%s): 手牌 %s 的EV = 无法解析\n", childID, childLabels[i], targetHand)
					} else {
						fmt.Printf("动作 %s (%s): 手牌 %s 的EV = %.6f\n", childID, childLabels[i], targetHand, ev)
					}
				} else {
					fmt.Printf("动作 %s (%s): 手牌 %s 的EV索引超出范围\n", childID, childLabels[i], targetHand)
				}
			}
			fmt.Println()
		}
	}

	// 6. 总结
	fmt.Println("【总结】")
	fmt.Printf("手牌: %s\n", targetHand)
	fmt.Printf("节点: %s\n", node)
	fmt.Printf("行动方: %s\n", actor)
	for i, childID := range childNodeIDs {
		if i < len(childLabels) && i < len(strategyResp) {
			freqFields := strings.Fields(strategyResp[i])

			// 获取频率
			var freqStr = "无法解析"
			var freq float64 = 0
			if targetIdx < len(freqFields) {
				var err error
				freq, err = strconv.ParseFloat(freqFields[targetIdx], 64)
				if err == nil {
					freqStr = fmt.Sprintf("%.6f", freq)
				}
			}

			// 获取EV
			cmd = fmt.Sprintf("calc_ev %s %s", actorCmd, childID)
			evResp, _ := client.ExecuteCommand(cmd, 10*time.Second)
			var evStr = "无法解析"
			var ev float64 = 0
			if len(evResp) > 0 {
				evFields := strings.Fields(evResp[0])
				if targetIdx < len(evFields) {
					var err error
					ev, err = strconv.ParseFloat(evFields[targetIdx], 64)
					if err == nil {
						evStr = fmt.Sprintf("%.6f", ev)
					} else {
						evStr = evFields[targetIdx] // 保留原始值，如NaN或Inf
					}
				}
			}

			// 获取EQ (如果存在)
			var eqStr = "未查询"
			var eq float64 = 0
			if len(eqResp) > 0 {
				eqFields := strings.Fields(eqResp[0])
				if targetIdx < len(eqFields) {
					var err error
					eq, err = strconv.ParseFloat(eqFields[targetIdx], 64)
					if err == nil {
						eqStr = fmt.Sprintf("%.6f", eq)
					} else {
						eqStr = eqFields[targetIdx] // 保留原始值，如NaN或Inf
					}
				}
			}

			// 判断是否会被过滤
			var filterReason string
			// 检查是否所有三个值都是无效值(0、NaN或Inf)
			freqIsInvalid := freq == 0
			evIsInvalid := ev == 0 || math.IsInf(ev, 0) || strings.Contains(strings.ToLower(evStr), "nan") || strings.Contains(strings.ToLower(evStr), "inf")
			eqIsInvalid := eq == 0 || math.IsInf(eq, 0) || strings.Contains(strings.ToLower(eqStr), "nan") || strings.Contains(strings.ToLower(eqStr), "inf")

			// 只有当三个值都无效时才标记为将被过滤
			if freqIsInvalid && evIsInvalid && eqIsInvalid {
				filterReason = "[将被过滤: 所有值都无效]"
			}

			if filterReason != "" {
				fmt.Printf("动作 %s (%s): 频率 = %s, EV = %s, EQ = %s %s\n",
					childID, childLabels[i], freqStr, evStr, eqStr, filterReason)
			} else {
				fmt.Printf("动作 %s (%s): 频率 = %s, EV = %s, EQ = %s\n",
					childID, childLabels[i], freqStr, evStr, eqStr)
			}
		}
	}

	fmt.Println("\n======== 打印完成 ========\n")
}
