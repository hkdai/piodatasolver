# Qwen3-14B 德州扑克GTO策略微调指南

## 目标
使用Unsloth框架在Windows环境下微调Qwen3-14B模型，使其能够提供专业的德州扑克GTO策略建议。

## 环境要求
- **操作系统**: Windows 10/11
- **GPU**: RTX 5090 (24GB显存)
- **Python**: 3.10 或 3.11
- **CUDA**: 12.1+
- **显卡驱动**: 545.xx 或更高

## 安装步骤

### 1. 环境准备
```powershell
# 在PowerShell中运行安装脚本
.\setup_windows.ps1
```

或手动安装：
```powershell
# 创建虚拟环境
python -m venv venv_poker_gto

# 激活虚拟环境
.\venv_poker_gto\Scripts\Activate.ps1

# 安装依赖
pip install -r requirements.txt
```

### 2. 数据准备
确保已经生成了JSONL格式的训练数据：
```powershell
# 从SQL数据生成JSONL
.\piodatasolver.exe jsonl
```

这会生成：
- `train.jsonl`: 训练数据集
- `eval.jsonl`: 评估数据集

### 3. 开始训练
```powershell
# 激活虚拟环境
.\venv_poker_gto\Scripts\Activate.ps1

# 运行训练脚本
python train_qwen3.py
```

## 训练参数说明

### 模型配置
- **基础模型**: Qwen/Qwen2.5-14B-Instruct
- **量化**: 4bit (节省显存)
- **LoRA配置**:
  - rank: 16
  - alpha: 16
  - dropout: 0.05

### 训练超参数
- **批次大小**: 1 (单卡)
- **梯度累积**: 8步
- **学习率**: 2e-4
- **训练轮数**: 3
- **序列长度**: 2048

### 显存优化
- 4bit量化加载
- 梯度检查点
- 8bit AdamW优化器
- FP16混合精度训练

## 训练过程监控

### 使用WandB
训练会自动记录到WandB，可以实时查看：
- 损失曲线
- 学习率变化
- 评估指标

### 本地日志
查看训练输出目录中的日志文件：
```
./qwen3_poker_gto_YYYYMMDD_HHMMSS/
├── checkpoint-*/     # 模型检查点
├── runs/             # TensorBoard日志
└── trainer_state.json # 训练状态
```

## 常见问题

### 1. 显存不足
如果遇到OOM错误：
- 减小批次大小: `per_device_train_batch_size=1`
- 增加梯度累积: `gradient_accumulation_steps=16`
- 减小序列长度: `max_seq_length=1024`

### 2. Windows多进程错误
确保设置：
```python
os.environ["TOKENIZERS_PARALLELISM"] = "false"
dataloader_num_workers=0
```

### 3. bitsandbytes安装失败
使用Windows专用版本：
```powershell
pip install bitsandbytes-windows
```

### 4. CUDA错误
检查CUDA版本匹配：
```python
import torch
print(torch.cuda.is_available())
print(torch.version.cuda)
```

## 模型测试

### 快速测试
```powershell
python inference_test.py
```

### 交互式测试
运行推理脚本后，可以输入自定义牌局进行测试。

### 示例输入
```
牌面: As Kd 7c
手牌: Ah Qh
位置: CO vs BB
SPR: 3.5
行动历史: OOP 过牌
```

## 模型部署

### 1. 保存格式
训练完成后会生成：
- `qwen3_poker_gto_final/`: HuggingFace格式
- `qwen3_poker_gto_gguf/`: GGUF格式（用于llama.cpp）

### 2. 模型量化
可以进一步量化以减小模型大小：
```python
# 在训练脚本中已包含
model.save_pretrained_gguf("model_q4", quantization_method="q4_k_m")
```

### 3. 推理优化
- 使用vLLM或TGI进行高效推理
- 部署为API服务
- 集成到扑克软件中

## 性能预期

### 训练时间
- 14B模型 + 5090显卡
- 10k样本: 约2-3小时
- 100k样本: 约20-30小时

### 推理速度
- 4bit量化: ~30-50 tokens/s
- FP16: ~15-25 tokens/s

### 质量指标
- 策略准确率: >85%
- 频率误差: <10%
- EV预测相关性: >0.8

## 进阶优化

### 1. 数据增强
- 增加更多牌局场景
- 平衡不同动作的样本
- 加入河牌圈数据

### 2. 模型优化
- 尝试不同的LoRA rank
- 调整学习率调度
- 使用更长的训练轮数

### 3. 推理优化
- 实现批量推理
- 缓存常见查询
- 优化提示模板

## 相关资源
- [Unsloth官方文档](https://github.com/unslothai/unsloth)
- [Qwen模型卡片](https://huggingface.co/Qwen)
- [德州扑克GTO理论](https://www.gtowizard.com/)

## 许可证
本项目仅供学习研究使用，请遵守相关模型的使用协议。 