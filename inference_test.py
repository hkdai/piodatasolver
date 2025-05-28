"""
微调后模型的推理测试脚本
"""

import torch
from transformers import AutoTokenizer, AutoModelForCausalLM
from unsloth import FastLanguageModel
import json

def load_model(model_path="qwen3_poker_gto_final"):
    """加载微调后的模型"""
    print(f"正在加载模型：{model_path}")
    
    # 使用Unsloth加载
    model, tokenizer = FastLanguageModel.from_pretrained(
        model_name=model_path,
        max_seq_length=2048,
        dtype=torch.float16,
        load_in_4bit=True,
    )
    
    # 启用推理模式
    FastLanguageModel.for_inference(model)
    
    return model, tokenizer

def generate_poker_advice(model, tokenizer, board, hole_cards, player_pos, opp_pos, 
                         is_oop, spr, action_history, board_texture, hand_features,
                         equity, pot_odds, stack_depth, bet_level, bet_pct):
    """生成扑克策略建议"""
    
    # 构建输入提示
    prompt = f"""### Instruction:
你是一名德州扑克GTO策略助手。请根据以下牌局信息提供最优的行动建议。

牌面：{board}
手牌：{hole_cards}
位置：{player_pos} vs {opp_pos}
玩家位置：{'OOP' if is_oop else 'IP'}
SPR：{spr:.2f}
行动历史：{action_history}

牌面结构：
- 类型：{board_texture['type']}
- 花色：{board_texture['suitedness']}  
- 连接性：{board_texture['connectedness']}

手牌特征：
- 类型：{hand_features['hand_category']}
- 强度评分：{hand_features['hand_strength_score']}/4
- 连接类型：{hand_features['connector_type']}
- 成牌/听牌：{hand_features.get('made_hand_type', '高牌')}
- 胜率：{equity:.2%}

游戏信息：
- 有效筹码：{stack_depth:.0f}bb
- 下注轮次：{bet_level}
- 最近下注占底池：{bet_pct:.2%}
- 底池赔率：{pot_odds:.2%}

请分析最佳GTO行动。

### Response:"""

    # 生成回复
    inputs = tokenizer(prompt, return_tensors="pt").to("cuda")
    
    with torch.no_grad():
        outputs = model.generate(
            **inputs,
            max_new_tokens=512,
            temperature=0.7,
            top_p=0.9,
            do_sample=True,
            pad_token_id=tokenizer.eos_token_id,
        )
    
    response = tokenizer.decode(outputs[0], skip_special_tokens=True)
    
    # 提取生成的回复部分
    response_start = response.find("### Response:") + len("### Response:")
    generated_response = response[response_start:].strip()
    
    return generated_response

def test_scenarios():
    """测试不同的牌局场景"""
    test_cases = [
        {
            "name": "高张牌面的持续下注",
            "board": "As Kd 7c",
            "hole_cards": "Ah Qh",
            "player_pos": "CO",
            "opp_pos": "BB",
            "is_oop": False,
            "spr": 3.5,
            "action_history": "OOP 过牌",
            "board_texture": {
                "type": "高张",
                "suitedness": "彩虹",
                "connectedness": "无顺子听牌"
            },
            "hand_features": {
                "hand_category": "strong",
                "hand_strength_score": 3,
                "connector_type": "none",
                "made_hand_type": "pair"
            },
            "equity": 0.82,
            "pot_odds": 0.0,
            "stack_depth": 350,
            "bet_level": 0,
            "bet_pct": 0.0
        },
        {
            "name": "低张牌面的诈唬机会",
            "board": "7s 6d 2c",
            "hole_cards": "Kc Qd",
            "player_pos": "BTN",
            "opp_pos": "SB",
            "is_oop": False,
            "spr": 2.8,
            "action_history": "OOP 过牌",
            "board_texture": {
                "type": "低张",
                "suitedness": "彩虹",
                "connectedness": "两张连续"
            },
            "hand_features": {
                "hand_category": "medium",
                "hand_strength_score": 2,
                "connector_type": "connected",
                "made_hand_type": "high_card"
            },
            "equity": 0.35,
            "pot_odds": 0.0,
            "stack_depth": 280,
            "bet_level": 0,
            "bet_pct": 0.0
        },
        {
            "name": "面对下注的决策",
            "board": "Js Th 5h",
            "hole_cards": "Ac Jc",
            "player_pos": "BB",
            "opp_pos": "CO",
            "is_oop": True,
            "spr": 2.5,
            "action_history": "OOP 过牌，IP 下注 33 个筹码",
            "board_texture": {
                "type": "高张",
                "suitedness": "两张同花",
                "connectedness": "两张连续"
            },
            "hand_features": {
                "hand_category": "strong",
                "hand_strength_score": 3,
                "connector_type": "none",
                "made_hand_type": "pair"
            },
            "equity": 0.68,
            "pot_odds": 0.248,
            "stack_depth": 250,
            "bet_level": 1,
            "bet_pct": 0.33
        }
    ]
    
    return test_cases

def main():
    # 加载模型
    model, tokenizer = load_model()
    
    # 获取测试场景
    test_cases = test_scenarios()
    
    print("\n" + "="*80)
    print("开始测试微调后的扑克GTO模型")
    print("="*80 + "\n")
    
    # 测试每个场景
    for i, test_case in enumerate(test_cases):
        print(f"\n场景 {i+1}: {test_case['name']}")
        print("-" * 60)
        print(f"牌面: {test_case['board']}")
        print(f"手牌: {test_case['hole_cards']}")
        print(f"位置: {test_case['player_pos']} vs {test_case['opp_pos']}")
        print(f"行动历史: {test_case['action_history']}")
        print(f"SPR: {test_case['spr']}")
        print("-" * 60)
        
        # 生成建议
        advice = generate_poker_advice(
            model, tokenizer,
            test_case['board'],
            test_case['hole_cards'],
            test_case['player_pos'],
            test_case['opp_pos'],
            test_case['is_oop'],
            test_case['spr'],
            test_case['action_history'],
            test_case['board_texture'],
            test_case['hand_features'],
            test_case['equity'],
            test_case['pot_odds'],
            test_case['stack_depth'],
            test_case['bet_level'],
            test_case['bet_pct']
        )
        
        print("\n模型建议:")
        print(advice)
        print("\n" + "="*80)
    
    # 交互式测试
    print("\n进入交互式测试模式（输入 'quit' 退出）")
    while True:
        print("\n请输入牌局信息：")
        try:
            board = input("牌面（如 As Kd 7c）: ").strip()
            if board.lower() == 'quit':
                break
                
            hole_cards = input("手牌（如 Ah Qh）: ").strip()
            player_pos = input("玩家位置（如 CO）: ").strip().upper()
            opp_pos = input("对手位置（如 BB）: ").strip().upper()
            is_oop = input("是否OOP（y/n）: ").strip().lower() == 'y'
            spr = float(input("SPR: ").strip())
            action_history = input("行动历史: ").strip()
            
            # 简化输入，使用默认值
            board_texture = {
                "type": "高张",
                "suitedness": "彩虹",
                "connectedness": "无顺子听牌"
            }
            hand_features = {
                "hand_category": "medium",
                "hand_strength_score": 2,
                "connector_type": "none",
                "made_hand_type": "high_card"
            }
            
            advice = generate_poker_advice(
                model, tokenizer,
                board, hole_cards, player_pos, opp_pos, is_oop, spr, action_history,
                board_texture, hand_features,
                0.5, 0.0, 100, 0, 0.0  # 默认值
            )
            
            print("\n" + "-" * 60)
            print("模型建议:")
            print(advice)
            print("-" * 60)
            
        except Exception as e:
            print(f"错误: {e}")
            continue

if __name__ == "__main__":
    main() 