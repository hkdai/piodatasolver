package main

import (
	"log"  // 保留这个
	"time" // 保留这个

	"piodatasolver/internal/cache"
	"piodatasolver/internal/upi" // 添加这个新包
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
	handOrder := &cache.HandOrder{}

	// 初始化HandOrder
	if err := handOrder.Init(client); err != nil {
		log.Fatalf("初始化HandOrder失败: %v", err)
	}

	// 加载树
	responses, err := client.LoadTree(`E:\zdsbddz\piosolver\piosolver3\saves\asth4d.cfr`)
	if err != nil {
		log.Fatalf("加载树失败: %v", err)
	}
	for _, resp := range responses {
		log.Printf("加载树响应: %s", resp)
	}

	//开始编写递归解析算法
	parseNode(client, "r:0")

	// //4.发一条指令并读取第一行回复
	// fmt.Println("\n--- 发送指令: is_ready ---")
	// fmt.Fprintln(stdin, "is_ready")

	// // 给程序时间响应
	time.Sleep(20 * time.Second)
	// // 关闭标准输入，让程序知道不会再有输入
	// stdin.Close()
	// //5. 退出
	// cmd.Process.Kill()
}

func parseNode(client *upi.Client, node string) {
	//show_node 获取当前节点信息，公牌，行动方（IP/OOP）

	//show_children 获取当前节点下的子节点，每一个子节点代表一个行动，与后续的show_strategy、每一行的结果对应

	//show_strategy 获取当前节点1326手牌各行动对应的策略频率，行动类别参考show_children的结果

	//calc_ev 计算当前节点下1326手牌各行动的期望值,返回结果两行，只取第一行的ev值

	//calc_eq_node 计算当前节点下1326手牌的胜率，只取第一行的eq值

	//遍历子节点，递归调用解析，但是当自己点的类型为SPLIT_NODE时，不再递归调用

	//展示当前节点下的子节点

}
