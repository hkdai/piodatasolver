package upi

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Client 表示一个PioSolver UPI客户端
type Client struct {
	// PioSolver可执行文件路径
	exePath string
	// PioSolver工作目录
	workingDir string
	// 进程对象
	cmd *exec.Cmd
	// 标准输入写入器
	stdin io.WriteCloser
	// 标准输出读取器
	stdout io.ReadCloser
	// 互斥锁，确保同一时间只有一个命令在执行
	mu sync.Mutex
	// 结束标记字符串
	endString string
	// 是否已启动
	started bool
}

// NewClient 创建一个新的PioSolver UPI客户端
func NewClient(exePath, workingDir string) *Client {
	return &Client{
		exePath:    exePath,
		workingDir: workingDir,
		endString:  "PIO_END", // 默认结束标记
	}
}

// Start 启动PioSolver进程
func (c *Client) Start() error {
	if c.started {
		return fmt.Errorf("客户端已启动")
	}

	// 创建命令
	c.cmd = exec.Command(c.exePath)
	c.cmd.Dir = c.workingDir

	// 获取标准输入和输出管道
	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取标准输入管道失败: %v", err)
	}

	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取标准输出管道失败: %v", err)
	}

	// 启动进程
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败: %v", err)
	}

	// 等待初始化（可以根据需要调整等待时间）
	time.Sleep(2 * time.Second)

	// 设置结束标记字符串
	if _, err := c.ExecuteCommand(fmt.Sprintf("set_end_string %s", c.endString), 5*time.Second); err != nil {
		log.Printf("设置结束标记失败: %v", err)
		// 即使设置失败也继续运行
	}

	c.started = true
	return nil
}

// ExecuteCommand 执行一个命令并等待其完成
func (c *Client) ExecuteCommand(command string, timeout time.Duration) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started && command != fmt.Sprintf("set_end_string %s", c.endString) {
		return nil, fmt.Errorf("客户端未启动")
	}

	// 发送命令
	fmt.Printf("\n--- 发送指令: %s ---\n", command)
	if _, err := fmt.Fprintln(c.stdin, command); err != nil {
		return nil, fmt.Errorf("发送命令失败: %v", err)
	}

	// 读取响应直到遇到结束标记或超时
	responses, err := c.readResponseUntilEnd(timeout)
	if err != nil {
		return nil, err
	}

	return responses, nil
}

// readResponseUntilEnd 读取响应直到遇到结束标记或超时
func (c *Client) readResponseUntilEnd(timeout time.Duration) ([]string, error) {
	var responses []string
	reader := bufio.NewReader(c.stdout)
	doneChan := make(chan bool)
	errChan := make(chan error)

	// 在goroutine中读取响应
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					doneChan <- true
					return
				}
				errChan <- err
				return
			}

			// 去除行尾的换行符
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")

			// 打印响应行
			fmt.Printf("收到响应: %s\n", line)

			// 检查是否是结束标记
			if line == c.endString {
				doneChan <- true
				return
			}

			// 将非空行添加到响应列表
			if line != "" && !strings.HasPrefix(line, "SOLVER:") {
				responses = append(responses, line)
			}
		}
	}()

	// 等待读取完成或超时
	select {
	case <-doneChan:
		return responses, nil
	case err := <-errChan:
		return nil, fmt.Errorf("读取响应时出错: %v", err)
	case <-time.After(timeout):
		return nil, fmt.Errorf("读取响应超时")
	}
}

// IsReady 检查PioSolver是否准备好
func (c *Client) IsReady() (bool, error) {
	responses, err := c.ExecuteCommand("is_ready", 5*time.Second)
	if err != nil {
		return false, err
	}

	for _, resp := range responses {
		if resp == "is_ready ok!" {
			return true, nil
		}
	}

	return false, nil
}

// LoadTree 加载树
func (c *Client) LoadTree(filePath string) ([]string, error) {
	return c.ExecuteCommand(fmt.Sprintf("load_tree %s", filePath), 30*time.Second)
}

// ShowChildren 显示节点的子节点
func (c *Client) ShowChildren(node string) ([]string, error) {
	return c.ExecuteCommand(fmt.Sprintf("show_children %s", node), 10*time.Second)
}

// ShowNode 显示节点信息
func (c *Client) ShowNode(node string) ([]string, error) {
	return c.ExecuteCommand(fmt.Sprintf("show_node %s", node), 10*time.Second)
}

// ShowStrategy 获取节点的策略
func (c *Client) ShowStrategy(node string) ([]string, error) {
	return c.ExecuteCommand(fmt.Sprintf("show_strategy %s", node), 20*time.Second)
}

// CalcEV 计算节点的期望值
func (c *Client) CalcEV(node string) ([]string, error) {
	return c.ExecuteCommand(fmt.Sprintf("calc_ev %s", node), 20*time.Second)
}

// CalcEqNode 计算节点的均衡值
func (c *Client) CalcEqNode(node string) ([]string, error) {
	return c.ExecuteCommand(fmt.Sprintf("calc_eq_node %s", node), 20*time.Second)
}

// Close 关闭客户端并结束PioSolver进程
func (c *Client) Close() error {
	if !c.started {
		return nil
	}

	// 尝试发送exit命令
	_, _ = fmt.Fprintln(c.stdin, "exit")

	// 关闭标准输入，让程序知道不会再有输入
	if err := c.stdin.Close(); err != nil {
		return err
	}

	// 给一点时间让进程自己退出
	time.Sleep(1 * time.Second)

	// 终止进程
	if err := c.cmd.Process.Kill(); err != nil {
		return err
	}

	c.started = false
	return nil
}
