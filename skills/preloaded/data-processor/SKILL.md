---
name: 数据处理器
description: 数据处理与分析技能。当用户需要对知识库检索结果进行数据分析、统计计算、格式转换、数据提取或生成报告，或需要比较 governed_data_query 返回的精确结果文件时使用。支持 Python 脚本执行进行高级数据处理。
---

# Data Processor

企业级知识库数据处理与分析技能，用于处理 RAG 检索结果和执行数据分析任务。

## 核心能力

1. **数据分析**: 对检索到的文档数据进行统计分析
2. **格式转换**: JSON/CSV/Markdown 等格式相互转换
3. **数据提取**: 从非结构化文本中提取结构化信息
4. **报告生成**: 生成数据分析报告和摘要

## 使用场景

当用户请求涉及以下内容时，使用此技能：
- "分析这些数据"、"统计一下"、"计算总数/平均值"
- "转换为 JSON/CSV 格式"
- "提取关键信息"、"整理成表格"
- "生成报告"、"数据汇总"

## 可用脚本

### 1. analyze.py - 数据分析脚本

分析输入的 JSON 数据，生成统计报告。

**命令行用法** (仅供参考):
```bash
# 通过 stdin 传入 JSON 数据
echo '{"items": [1, 2, 3, 4, 5]}' | python scripts/analyze.py

# 或传入文件路径（需要文件实际存在）
python scripts/analyze.py --file data.json
```

**使用 execute_skill_script 工具时**:
- 如果你有内存中的数据（如 JSON 字符串），使用 `input` 参数传入，不要使用 `args`
- `--file` 参数仅用于读取技能目录中已存在的文件，不适用于传递内存数据

```json
// ✅ 正确：通过 input 传入数据
{
  "skill_name": "数据处理器",
  "script_path": "scripts/analyze.py",
  "input": "{\"items\": [1, 2, 3], \"query\": \"统计分析\"}"
}

// ❌ 错误：--file 需要文件路径，不能单独使用
{
  "skill_name": "数据处理器",
  "script_path": "scripts/analyze.py",
  "args": ["--file"],
  "input": "{...}"
}
```

**输入格式**:
```json
{
  "items": [数据项数组],
  "query": "可选的查询描述"
}
```

**输出**: JSON 格式的统计结果，包含计数、求和、平均值等。

### 2. format_converter.py - 格式转换脚本

在 JSON、CSV、Markdown 表格之间转换数据。

**用法**:
```bash
# JSON 转 CSV
echo '[{"name": "A", "value": 1}]' | python scripts/format_converter.py --to csv

# JSON 转 Markdown 表格
echo '[{"name": "A", "value": 1}]' | python scripts/format_converter.py --to markdown

# CSV 转 JSON
echo 'name,value\nA,1' | python scripts/format_converter.py --from csv --to json
```

### 3. extract_info.py - 信息提取脚本

从文本中提取结构化信息（数字、日期、关键词等）。

**用法**:
```bash
echo "2024年销售额为100万元，同比增长15%" | python scripts/extract_info.py
```

**输出**:
```json
{
  "numbers": ["100", "15"],
  "dates": ["2024年"],
  "percentages": ["15%"],
  "amounts": ["100万元"]
}
```

### 4. governed_compare.py - 受治理查询精确比较

仅当输入来自 `governed_data_query` 返回的精确 `input_file` 时使用。不要把工具预览、手工抄写的行、glob 选出的文件或知识库片段传给此脚本。普通 RAG 数据继续使用前述脚本。

周期数值比较：

```bash
python scripts/governed_compare.py period \
  --baseline /workspace/data/governed-query-BASE.json \
  --current /workspace/data/governed-query-CURRENT.json \
  --key store_id --key product_id \
  --value sales_amount --unit sales_amount=元 \
  --baseline-period 2025-08 --current-period 2026-08 \
  --output /workspace/output/period-comparison.json
```

库存/销售等两侧覆盖对账：

```bash
python scripts/governed_compare.py reconcile \
  --left /workspace/data/governed-query-INVENTORY.json \
  --right /workspace/data/governed-query-SALES.json \
  --key store_id --key product_id \
  --left-value inventory_qty --right-value sales_qty \
  --unit inventory_qty=公斤 --unit sales_qty=公斤 \
  --output /workspace/output/inventory-sales-reconciliation.json
```

只传每个查询文件中已声明的列，并为每个数值列明确传入来源单位；period 还必须明确两个查询实际代表的期间。键必须非空且在各文件内唯一；如脚本报告重复键，应按 Catalog 定义的事实粒度在来源 SQL 中预聚合，不要让脚本猜测聚合方式。周期比较对选定数值执行 Decimal 差额和 38 位有效数字的相对变化；零基期、NULL 和缺少一侧键保持未知，负基期附解释限制。reconcile 执行 full outer 键覆盖，保留左右存在性和 NULL，不计算库存减销售或擅自生成周转/覆盖天数。

输出的 `inputs` 列表保留文件路径、SHA-256、`query_id`、SQL、Catalog、列、`limits` 和 evidence；`coverage` 只统计两个查询返回行的键集合。即使查询未截断，也不能据此声称完整业务总体；`limits.coverage` 是来源对象观测，不是对查询总体的实测覆盖。使用 `--output` 时，stdout 只给摘要，完整产物写入指定路径。读回完整产物并核对 `inputs[].query_id`、期间、范围、单位和所用数值后再回答。用户指出的计算错误应形成经过评审的回归 fixture；不要把它自动写成模型记忆、语义规则或企业指标定义。

## 处理流程

### 分析 RAG 检索结果

当需要分析知识库检索结果时：

1. 收集检索到的文档片段
2. 提取关键数据点
3. 使用 `analyze.py` 进行统计
4. 整理并呈现分析结果

**示例**：
```
用户: "帮我统计知识库中提到的所有产品销售数据"

步骤:
1. 使用 knowledge_search 检索相关文档
2. 整理数据为 JSON 格式
3. 调用 execute_skill_script:
   - skill_name: "data-processor"
   - script_path: "scripts/analyze.py"
   - 通过 stdin 传入数据
4. 解析输出并生成报告
```

### 数据格式转换

当用户需要特定格式输出时：

1. 整理数据为标准 JSON 格式
2. 使用 `format_converter.py` 转换
3. 返回目标格式结果

## 最佳实践

1. **数据预处理**: 调用脚本前，确保数据格式正确
2. **错误处理**: 检查脚本执行结果，处理异常情况
3. **结果验证**: 验证输出结果的合理性
4. **渐进处理**: 大数据量时分批处理

## 输出格式

分析结果示例：
```markdown
## 数据分析报告

### 基本统计
- 数据条数: 50
- 数值总和: 1,234,567
- 平均值: 24,691.34
- 最大值: 99,999
- 最小值: 100

### 分布情况
| 区间 | 数量 | 占比 |
|------|------|------|
| 0-1000 | 10 | 20% |
| 1000-10000 | 25 | 50% |
| >10000 | 15 | 30% |

### 结论
根据数据分析，XXX...
```

## 注意事项

- 脚本在 Docker 沙箱中执行，确保安全隔离
- 执行超时默认为 60 秒
- 输入数据大小有限制，大文件请分批处理
- 脚本输出为 JSON 格式，便于后续处理
