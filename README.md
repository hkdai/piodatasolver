# PioSolver数据解析工具

一个用Go语言编写的PioSolver CFR文件批量解析工具，支持将CFR文件解析为JSON格式和SQL格式，并提供CSV转换功能用于高效数据库导入。

## 🚀 功能特性

- ✅ **批量解析CFR文件**：自动处理指定目录下的所有CFR文件
- ✅ **智能跳过机制**：自动跳过已解析的文件，支持断点续传
- ✅ **多格式输出**：支持JSON和SQL两种输出格式
- ✅ **数据完整性**：自动验证JSON和SQL记录数量一致性
- ✅ **动态表名生成**：根据CFR文件名自动生成对应的数据库表名
- ✅ **真实筹码计算**：自动获取每个CFR文件的真实有效筹码
- ✅ **策略分析**：包含SPR、筹码深度、下注比例等关键指标
- ✅ **位置判断**：自动识别IP/OOP位置信息
- ✅ **SQL文件合并**：将多个SQL文件合并为单一文件
- ✅ **CSV转换**：支持SQL转CSV格式，优化数据库导入性能

## 📋 系统要求

- Windows 10/11
- PioSolver软件已安装
- PowerShell 5.0+
- MySQL 5.7+ (用于数据导入)

## 🛠️ 安装说明

1. 下载编译好的可执行文件 `piodatasolver.exe`
2. 将可执行文件放置在包含CFR文件的目录中
3. 确保PioSolver软件已正确安装并可以通过命令行调用

## 📖 使用方法

### 1. 解析CFR文件 (parse命令)

将CFR文件解析为JSON和SQL格式：

```powershell
# 解析当前目录下的所有CFR文件
.\piodatasolver.exe parse

# 解析指定目录下的CFR文件
.\piodatasolver.exe parse -dir "C:\path\to\cfr\files"
```

**输出结果**：
- `data/` 目录：包含所有JSON文件
- `data/` 目录：包含所有SQL文件
- `data/hand_mapping.json`：手牌映射文件

### 2. 计算模式 (calc命令)

仅计算数据而不解析CFR文件：

```powershell
.\piodatasolver.exe calc
```

### 3. 合并SQL文件 (merge命令)

将data目录下的所有SQL文件合并为单一文件：

```powershell
.\piodatasolver.exe merge
```

**输出结果**：
- `data/data.sql`：包含所有SQL语句的合并文件
- 显示处理统计信息（文件数、语句数、文件大小）

### 4. 转换为CSV格式 (mergecsv命令)

将SQL文件转换为CSV格式，用于高效数据库导入：

```powershell
.\piodatasolver.exe mergecsv
```

**输出结果**：
- `csv/` 目录：包含所有CSV文件（每个公牌一个文件）
- `csv/load_data.sql`：MySQL导入脚本，包含所有LOAD DATA语句

## 📊 数据结构说明

### JSON输出格式

```json
[
  {
    "spr": "6.1667",
    "bet_pct": "0.0000", 
    "node": "r:0",
    "actor": "OOP_DEC",
    "board": "2c 2d 2h ",
    "board_id": 546,
    "hand": "3d3c",
    "combo_id": 14,
    "stack_depth": "370.000",
    "ip_or_oop": "OOP",
    "action1": "check",
    "freq1": "1.000",
    "ev1": "47.216", 
    "eq1": "0.615",
    "action2": "",
    "freq2": "0.000",
    "ev2": "0.000",
    "eq2": "0.000"
  }
]
```

### SQL表结构

```sql
CREATE TABLE flop_40bb_co_bb (
  id INT AUTO_INCREMENT PRIMARY KEY,
  node_prefix VARCHAR(255),
  bet_level INT,
  board_id INT,
  combo_id INT,
  stack_depth DECIMAL(10,3),
  bet_pct DECIMAL(8,4),
  spr DECIMAL(8,4),
  board_str VARCHAR(20),
  combo_str VARCHAR(10),
  ip_or_oop VARCHAR(10),
  action1 VARCHAR(50),
  freq1 DECIMAL(8,4),
  ev1 DECIMAL(8,4),
  eq1 DECIMAL(8,4),
  action2 VARCHAR(50),
  freq2 DECIMAL(8,4),
  ev2 DECIMAL(8,4),
  eq2 DECIMAL(8,4)
);
```

### 字段说明

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `node_prefix` | 字符串 | 节点路径，如"r:0:c:20" |
| `bet_level` | 整数 | 下注级别 |
| `board_id` | 整数 | 公牌ID |
| `combo_id` | 整数 | 手牌组合ID |
| `stack_depth` | 小数 | 筹码深度（后手筹码） |
| `bet_pct` | 小数 | 下注占底池比例 |
| `spr` | 小数 | 栈底比（Stack-to-Pot Ratio） |
| `board_str` | 字符串 | 公牌文字，如"2c 2d 2h" |
| `combo_str` | 字符串 | 手牌文字，如"3d3c" |
| `ip_or_oop` | 字符串 | 位置信息："IP"或"OOP" |
| `action1/2` | 字符串 | 行动选项，如"check"、"bet" |
| `freq1/2` | 小数 | 行动频率（0-1） |
| `ev1/2` | 小数 | 期望值 |
| `eq1/2` | 小数 | 胜率 |

## 🗂️ 文件命名规则

### CFR文件命名格式
```
{筹码深度}_{位置}_{公牌}.cfr
例如：40bb_COvsBB_2c2d2h.cfr
```

### 生成的表名格式
```
flop_{筹码深度}_{位置}
例如：flop_40bb_co_bb
```

### CSV文件命名格式
```
flop_{筹码深度}_{位置}_{公牌}.csv
例如：flop_40bb_co_bb_2c2d2h.csv
```

## 🚀 数据库导入

### 使用LOAD DATA INFILE导入

1. 运行mergecsv命令生成CSV文件
2. 将csv目录复制到MySQL服务器
3. 执行导入脚本：

```sql
-- 在MySQL中执行
source /path/to/csv/load_data.sql;
```

### 导入脚本示例

```sql
-- 导入表: flop_40bb_co_bb
LOAD DATA LOCAL INFILE 'E:/zdsbddz/piodatasolver/csv/flop_40bb_co_bb_2c2d2h.csv'
INTO TABLE flop_40bb_co_bb
FIELDS TERMINATED BY ',' 
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS;
```

## 📈 性能优化

- **批量处理**：支持大量CFR文件的批量解析
- **断点续传**：自动跳过已处理的文件
- **内存优化**：流式处理大文件，避免内存溢出
- **CSV导入**：使用LOAD DATA INFILE比INSERT语句快10-100倍
- **IGNORE机制**：自动跳过重复数据，避免导入错误

## 🔧 故障排除

### 常见问题

1. **PioSolver无法启动**
   - 检查PioSolver是否正确安装
   - 确认环境变量PATH中包含PioSolver路径

2. **解析失败**
   - 检查CFR文件是否损坏
   - 确认文件命名格式是否正确

3. **数据库导入失败**
   - 检查MySQL服务是否运行
   - 确认文件路径使用正斜杠（/）
   - 检查表是否已存在

4. **记录数不匹配**
   - CSV文件包含表头行，比JSON多206行是正常现象
   - 使用IGNORE导入时，重复记录会被跳过但ID仍会递增

## 📊 统计信息示例

```
=================================
【解析完成】统计信息
=================================
✅ 成功解析文件数: 206
✅ JSON总记录数: 1,284,545
✅ SQL总记录数: 1,284,545
✅ 数据一致性: 通过
✅ 总处理时间: 45分钟
=================================
```

## 🤝 贡献

欢迎提交Issue和Pull Request来改进这个工具。

## 📄 许可证

MIT License

---

**注意**：使用前请确保已备份重要的CFR文件，避免数据丢失。 