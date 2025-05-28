"""
Qwen3-14B 德州扑克GTO策略微调训练脚本
使用Unsloth框架在Windows环境下进行高效微调
"""

import os
import json
import torch
from datasets import Dataset, load_dataset
from transformers import TrainingArguments
from trl import SFTTrainer
from unsloth import FastLanguageModel
import wandb
from datetime import datetime

# 设置环境变量，避免Windows下的多进程问题
os.environ["TOKENIZERS_PARALLELISM"] = "false"

def load_jsonl_data(file_path):
    """加载JSONL格式的训练数据"""
    data = []
    with open(file_path, 'r', encoding='utf-8') as f:
        for line in f:
            data.append(json.loads(line.strip()))
    return data

def format_poker_instruction(example):
    """将训练数据格式化为指令微调格式"""
    # 构建输入提示
    input_text = f"""你是一名德州扑克GTO策略助手。请根据以下牌局信息提供最优的行动建议。

牌面：{example['board']}
手牌：{example['hole_cards']}
位置：{example['player_position']} vs {example['opponent_position']}
玩家位置：{'OOP' if example['player_is_oop'] else 'IP'}
SPR：{example['spr']:.2f}
行动历史：{example['action_history']}

牌面结构：
- 类型：{example['board_texture_summary']['type']}
- 花色：{example['board_texture_summary']['suitedness']}  
- 连接性：{example['board_texture_summary']['connectedness']}

手牌特征：
- 类型：{example['hand_features']['hand_category']}
- 强度评分：{example['hand_features']['hand_strength_score']}/4
- 连接类型：{example['hand_features']['connector_type']}
- 成牌/听牌：{example['hand_features'].get('made_hand_type', '高牌')}
- 胜率：{example['equity']:.2%}

游戏信息：
- 有效筹码：{example['stack_depth']:.0f}bb
- 下注轮次：{example['bet_level']}
- 最近下注占底池：{example['bet_pct']:.2%}
- 底池赔率：{example['pot_odds']:.2%}

请分析最佳GTO行动。"""

    # 构建输出响应
    output_text = f"""基于当前牌局，GTO最优行动是：**{example['gto_action'].upper()}** (频率：{example['frequency_pct']:.1f}%)

行动分析：
1. **行动选择**：{example['gto_action'].upper()}
2. **执行频率**：{example['frequency_pct']:.1f}%
3. **期望值(EV)**：{example['ev']:.3f}bb

策略解释：
- 手牌类型：{example['hand_features']['hand_category']}（{example['hole_cards']}）
- 在当前牌面（{example['board']}）上，手牌胜率为{example['equity']:.2%}
- 考虑到SPR={example['spr']:.2f}和位置（{'OOP' if example['player_is_oop'] else 'IP'}），该行动是最优选择
- 频率{example['frequency_pct']:.1f}%确保了策略的平衡性"""

    # 使用Alpaca格式
    return f"""### Instruction:
{input_text}

### Response:
{output_text}"""

def prepare_dataset(train_file, eval_file=None):
    """准备训练和评估数据集"""
    print("正在加载训练数据...")
    train_data = load_jsonl_data(train_file)
    
    # 格式化数据
    formatted_train = []
    for example in train_data:
        formatted_text = format_poker_instruction(example)
        formatted_train.append({"text": formatted_text})
    
    train_dataset = Dataset.from_list(formatted_train)
    print(f"训练集大小：{len(train_dataset)} 条")
    
    eval_dataset = None
    if eval_file and os.path.exists(eval_file):
        print("正在加载评估数据...")
        eval_data = load_jsonl_data(eval_file)
        formatted_eval = []
        for example in eval_data:
            formatted_text = format_poker_instruction(example)
            formatted_eval.append({"text": formatted_text})
        eval_dataset = Dataset.from_list(formatted_eval)
        print(f"评估集大小：{len(eval_dataset)} 条")
    
    return train_dataset, eval_dataset

def main():
    # 配置参数
    max_seq_length = 2048  # Qwen3支持的序列长度
    dtype = torch.float16  # 5090支持float16
    load_in_4bit = True   # 使用4bit量化节省显存
    
    # 模型名称
    model_name = "Qwen/Qwen2.5-14B-Instruct"  # 使用Qwen2.5作为基础（Qwen3尚未开源）
    
    print(f"正在加载模型：{model_name}")
    
    # 使用Unsloth加载模型
    model, tokenizer = FastLanguageModel.from_pretrained(
        model_name=model_name,
        max_seq_length=max_seq_length,
        dtype=dtype,
        load_in_4bit=load_in_4bit,
        # 针对5090显卡的优化
        device_map="auto",
        trust_remote_code=True,
    )
    
    # 准备LoRA进行参数高效微调
    model = FastLanguageModel.get_peft_model(
        model,
        r=16,  # LoRA rank
        target_modules=["q_proj", "k_proj", "v_proj", "o_proj",
                       "gate_proj", "up_proj", "down_proj"],
        lora_alpha=16,
        lora_dropout=0.05,
        bias="none",
        use_gradient_checkpointing="unsloth",  # 使用Unsloth的梯度检查点
        random_state=42,
        use_rslora=False,
        loftq_config=None,
    )
    
    # 准备数据集
    train_dataset, eval_dataset = prepare_dataset("train.jsonl", "eval.jsonl")
    
    # 训练参数配置
    training_args = TrainingArguments(
        output_dir=f"./qwen3_poker_gto_{datetime.now().strftime('%Y%m%d_%H%M%S')}",
        per_device_train_batch_size=1,  # 5090显卡建议使用小batch size
        gradient_accumulation_steps=8,  # 通过梯度累积实现更大的有效batch size
        warmup_steps=10,
        num_train_epochs=3,
        learning_rate=2e-4,
        fp16=True,  # 使用FP16加速
        logging_steps=10,
        save_steps=100,
        eval_steps=100 if eval_dataset else None,
        evaluation_strategy="steps" if eval_dataset else "no",
        save_strategy="steps",
        load_best_model_at_end=True if eval_dataset else False,
        report_to="wandb",  # 使用wandb进行实验跟踪
        run_name=f"qwen3_poker_gto_{datetime.now().strftime('%Y%m%d_%H%M%S')}",
        
        # Windows特定配置
        dataloader_num_workers=0,  # Windows下避免多进程问题
        remove_unused_columns=False,
        
        # 5090显卡优化
        optim="paged_adamw_8bit",  # 使用8bit优化器节省显存
        gradient_checkpointing=True,
        max_grad_norm=0.3,
        weight_decay=0.001,
        seed=42,
    )
    
    # 初始化训练器
    trainer = SFTTrainer(
        model=model,
        tokenizer=tokenizer,
        train_dataset=train_dataset,
        eval_dataset=eval_dataset,
        dataset_text_field="text",
        max_seq_length=max_seq_length,
        packing=False,  # 不使用packing以简化训练
        args=training_args,
    )
    
    # 开始训练
    print("开始训练...")
    trainer.train()
    
    # 保存模型
    print("正在保存模型...")
    model.save_pretrained("qwen3_poker_gto_final")
    tokenizer.save_pretrained("qwen3_poker_gto_final")
    
    # 保存为GGUF格式（可选，用于llama.cpp等推理）
    print("正在导出GGUF格式...")
    model.save_pretrained_gguf("qwen3_poker_gto_gguf", tokenizer, quantization_method="q4_k_m")
    
    print("训练完成！")

if __name__ == "__main__":
    # 初始化wandb（可选）
    wandb.init(project="poker-gto-qwen3", name=f"training_{datetime.now().strftime('%Y%m%d_%H%M%S')}")
    
    # 检查CUDA是否可用
    if torch.cuda.is_available():
        print(f"使用GPU: {torch.cuda.get_device_name(0)}")
        print(f"显存大小: {torch.cuda.get_device_properties(0).total_memory / 1024**3:.2f} GB")
    else:
        print("警告：未检测到GPU，将使用CPU训练（速度会很慢）")
    
    main() 