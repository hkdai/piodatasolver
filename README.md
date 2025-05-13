# PioDataSolver

PioDataSolver是一个用于与PioSolver通信并解析数据的Go工具。

## 功能

- 利用UPI协议与PioSolver通信
- 加载和分析游戏树
- 获取节点信息、子节点和策略数据
- 支持手牌顺序索引查询

## 使用方法

1. 确保已安装PioSolver并获得许可
2. 修改配置指向您的PioSolver安装目录
3. 运行程序：`go run main.go`

## 项目结构

- `main.go` - 程序入口点
- `internal/upi` - UPI客户端实现
- `internal/cache` - 缓存和工具类

## 依赖

- Go 1.20或更高版本
- PioSolver 3.0或更高版本

## 许可

仅供个人学习使用 