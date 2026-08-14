---
name: data-analyst
description: 数据分析技能，支持 CSV/TSV/XLSX/JSON/JSONL 格式的数据处理、清洗、分析和统计
metadata: {"clawdbot":{"emoji":"📊","requires":{"anyBins":["python3","python"]},"os":["linux","darwin","win32"]}}
---

# Data Analyst Skill 📊

## 支持格式

CSV, TSV, XLSX, JSON, JSON Lines

## Quick Start

1. 标准化 xlsx: `df = normalize_xlsx('data.xlsx')`
2. 清洗数据: `df = clean_numbers(df)`
3. 分析: `df.groupby('col').agg(...)`

---

## xlsx 格式预处理

xlsx 文件常包含复杂格式，需要先标准化为干净的行列数据。

### 快速检查

```python
def inspect_xlsx(path):
    """检查 xlsx 文件结构"""
    import pandas as pd
    from openpyxl import load_workbook
    
    # 查看所有 sheet
    xl = pd.ExcelFile(path)
    print(f"工作表: {xl.sheet_names}")
    
    # 查看原始结构（前10行，不做任何处理）
    wb = load_workbook(path, data_only=True)
    ws = wb.active
    print(f"\n前10行原始内容:")
    for i, row in enumerate(ws.iter_rows(max_row=10, values_only=True), 1):
        print(f"  Row {i}: {row}")
    
    # 检查合并单元格
    if ws.merged_cells:
        print(f"\n合并单元格: {len(ws.merged_cells)} 个")
    
    return xl.sheet_names
```

### 常见格式问题

| 问题 | 描述 | 解决方案 |
|------|------|---------|
| **合并单元格** | 标题/分类行合并了多个单元格 | 取消合并，向下/向右填充 |
| **多级表头** | 表头占2-3行 | 合并为一行，用 `_` 连接 |
| **前缀行** | 前 N 行是标题/说明，非数据 | 跳过前 N 行，或自动检测数据起始行 |
| **空行/空列** | 中间有空行分隔 | 删除空行空列 |
| **多工作表** | 一个文件多个 sheet | 指定 sheet 或合并所有 |
| **格式化数字** | "¥1,234.56" 或 "12.34%" | 提取纯数值 |
| **日期格式混乱** | "2024年1月1日" / "01/01/24" 等 | 统一转 ISO 格式 |
| **公式单元格** | 显示值 vs 公式 | 读取计算后的值 |
| **隐藏行列** | 有隐藏的数据 | 可选择是否包含 |
| **数据区混合** | 数据区外有注释/小计 | 识别并清理 |

### 标准化处理

```python
import pandas as pd
from openpyxl import load_workbook
from openpyxl.utils import range_boundaries
import re

def normalize_xlsx(
    path,
    sheet_name=0,
    skip_rows=None,      # 跳过前N行，None=自动检测
    header_rows=1,       # 表头行数（多级表头时>1）
    fill_merged=True,    # 填充合并单元格
    drop_empty=True,     # 删除空行空列
    data_only=True       # 读取公式计算值
):
    """
    将 xlsx 标准化为干净的 DataFrame
    
    返回: pandas DataFrame
    """
    wb = load_workbook(path, data_only=data_only)
    
    # 获取工作表
    if isinstance(sheet_name, int):
        ws = wb.worksheets[sheet_name]
    else:
        ws = wb[sheet_name]
    
    # 1. 处理合并单元格
    if fill_merged and ws.merged_cells:
        ws = _fill_merged_cells(ws)
    
    # 2. 读取数据
    data = list(ws.iter_rows(values_only=True))
    
    # 3. 自动检测数据起始行
    if skip_rows is None:
        skip_rows = _detect_header_start(data)
    
    # 4. 处理多级表头
    if header_rows > 1:
        headers = _merge_multiheader(data[skip_rows:skip_rows+header_rows])
        rows = data[skip_rows+header_rows:]
    else:
        headers = data[skip_rows]
        rows = data[skip_rows+1:]
    
    # 5. 创建 DataFrame
    df = pd.DataFrame(rows, columns=headers)
    
    # 6. 清理空行空列
    if drop_empty:
        df = _drop_empty(df)
    
    # 7. 清理列名
    df.columns = [_clean_column_name(c) for c in df.columns]
    
    return df

def _fill_merged_cells(ws):
    """取消合并并填充值"""
    for merged_range in list(ws.merged_cells.ranges):
        min_col, min_row, max_col, max_row = range_boundaries(str(merged_range))
        value = ws.cell(row=min_row, column=min_col).value
        
        # 取消合并
        ws.unmerge_cells(str(merged_range))
        
        # 填充所有单元格
        for row in range(min_row, max_row + 1):
            for col in range(min_col, max_col + 1):
                ws.cell(row=row, column=col, value=value)
    
    return ws

def _detect_header_start(data, max_scan=20):
    """自动检测表头起始行"""
    for i, row in enumerate(data[:max_scan]):
        non_empty = sum(1 for v in row if v is not None)
        if non_empty >= len(row) * 0.5:
            if i + 1 < len(data):
                next_row = data[i + 1]
                next_non_empty = sum(1 for v in next_row if v is not None)
                if next_non_empty >= len(next_row) * 0.5:
                    return i
    return 0

def _merge_multiheader(rows):
    """合并多级表头"""
    if not rows:
        return []
    
    result = []
    for col_idx in range(len(rows[0])):
        parts = []
        for row in rows:
            val = row[col_idx]
            if val is not None and str(val).strip():
                parts.append(str(val).strip())
        result.append('_'.join(parts) if parts else f'col_{col_idx}')
    return result

def _drop_empty(df):
    """删除全空行和全空列"""
    df = df.dropna(how='all')
    df = df.dropna(axis=1, how='all')
    df = df.reset_index(drop=True)
    return df

def _clean_column_name(name):
    """清理列名"""
    if name is None:
        return 'unnamed'
    name = str(name).strip()
    name = re.sub(r'[\s\-]+', '_', name)
    name = re.sub(r'[^\w\u4e00-\u9fff]', '', name)
    return name.lower() if name else 'unnamed'
```

### 格式化数字清理

```python
def clean_formatted_numbers(df, columns=None):
    """
    清理格式化数字：
    - "¥1,234.56" → 1234.56
    - "12.34%" → 0.1234
    - "(1,234)" → -1234  (会计负数格式)
    """
    def parse_number(val):
        if pd.isna(val) or val == '':
            return None
        if isinstance(val, (int, float)):
            return val
        
        s = str(val).strip()
        
        # 百分比
        if s.endswith('%'):
            try:
                return float(s.rstrip('%')) / 100
            except:
                pass
        
        # 会计负数格式 (1234)
        if s.startswith('(') and s.endswith(')'):
            s = '-' + s[1:-1]
        
        # 移除货币符号、千分位
        s = re.sub(r'[¥$€£,，]', '', s)
        
        try:
            return float(s)
        except:
            return val
    
    cols = columns if columns else df.columns
    for col in cols:
        if col in df.columns:
            df[col] = df[col].apply(parse_number)
    
    return df
```

### 日期格式标准化

```python
def normalize_dates(df, columns=None, target_format='%Y-%m-%d'):
    """
    标准化各种日期格式：
    - "2024年1月1日" / "01/01/24" / "2024-01-01" → "2024-01-01"
    - Excel 日期序列号 → ISO 日期
    """
    from datetime import datetime, timedelta
    
    def parse_date(val):
        if pd.isna(val):
            return None
        
        if isinstance(val, datetime):
            return val.strftime(target_format)
        
        # Excel 日期序列号
        if isinstance(val, (int, float)) and val > 0 and val < 100000:
            try:
                base = datetime(1899, 12, 30)
                return (base + timedelta(days=int(val))).strftime(target_format)
            except:
                pass
        
        s = str(val).strip()
        
        formats = [
            '%Y-%m-%d', '%Y/%m/%d', '%Y.%m.%d',
            '%Y年%m月%d日',
            '%m/%d/%Y', '%m-%d-%Y',
            '%d/%m/%Y', '%d-%m-%Y',
            '%Y%m%d'
        ]
        
        for fmt in formats:
            try:
                return datetime.strptime(s, fmt).strftime(target_format)
            except:
                continue
        
        return val
    
    cols = columns if columns else df.columns
    for col in cols:
        if col in df.columns:
            sample = df[col].dropna().head(5)
            is_date_col = any(
                isinstance(v, datetime) or 
                re.search(r'[年月日/\-]', str(v)) or
                (isinstance(v, (int, float)) and 1 < v < 50000)
                for v in sample
            )
            if is_date_col:
                df[col] = df[col].apply(parse_date)
    
    return df
```

### 一键标准化

```python
def standardize_xlsx(path, output_path=None, **kwargs):
    """
    一键标准化 xlsx 文件
    
    参数:
        path: 输入 xlsx 路径
        output_path: 输出路径（None 则不保存）
        **kwargs: 传递给 normalize_xlsx 的参数
    
    返回:
        标准化后的 DataFrame
    """
    df = normalize_xlsx(path, **kwargs)
    df = clean_formatted_numbers(df)
    df = normalize_dates(df)
    
    if output_path:
        if output_path.endswith('.xlsx'):
            df.to_excel(output_path, index=False)
        else:
            df.to_csv(output_path, index=False)
        print(f"已保存: {output_path}")
    
    return df

# 使用示例
df = standardize_xlsx(
    'messy_data.xlsx',
    output_path='clean_data.csv',
    skip_rows=2,
    header_rows=2,
    fill_merged=True
)
```

### 多工作表处理

```python
def read_all_sheets(path, **kwargs):
    """读取所有工作表，返回字典 {sheet_name: DataFrame}"""
    xl = pd.ExcelFile(path)
    result = {}
    for sheet in xl.sheet_names:
        result[sheet] = normalize_xlsx(path, sheet_name=sheet, **kwargs)
    return result

def merge_sheets(path, add_source_col=True, **kwargs):
    """合并所有工作表到一个 DataFrame"""
    sheets = read_all_sheets(path, **kwargs)
    dfs = []
    for name, df in sheets.items():
        if add_source_col:
            df['_source_sheet'] = name
        dfs.append(df)
    return pd.concat(dfs, ignore_index=True)
```

---

## 数据操作

脚本位置: `scripts/data_ops.py`

### 命令行用法

```bash
# 查看文件信息
python scripts/data_ops.py read data.xlsx

# 过滤数据
python scripts/data_ops.py filter data.csv "amount > 1000" output.csv

# 聚合
python scripts/data_ops.py aggregate data.csv category amount sum summary.csv

# 连接两个文件
python scripts/data_ops.py join orders.csv customers.csv customer_id merged.csv
```

### Python 中调用

```python
from scripts.data_ops import read_data, save_data, filter_data, aggregate, join_files

# 读取
df = read_data('sales.xlsx')

# 保存
save_data(df, 'output.csv')

# 过滤
filtered = filter_data('data.csv', 'amount > 1000', 'filtered.csv')

# 聚合
summary = aggregate('data.csv', 'category', 'amount', 'sum', 'summary.csv')

# 连接
joined = join_files('orders.csv', 'customers.csv', 'customer_id', 'left', 'merged.csv')
```

---

## 大文件处理

对于内存无法一次性加载的大文件，使用流式处理：

```python
def stream_process(input_path, output_path, transform_fn, chunk_size=10000):
    """
    流式处理大型文件
    
    参数:
        input_path: 输入文件路径
        output_path: 输出文件路径
        transform_fn: 转换函数，接收 DataFrame 返回 DataFrame 或 None（跳过）
        chunk_size: 每块行数
    """
    import pandas as pd
    
    first_chunk = True
    for chunk in pd.read_csv(input_path, chunksize=chunk_size):
        result = transform_fn(chunk)
        if result is None:
            continue
        
        if first_chunk:
            result.to_csv(output_path, index=False)
            first_chunk = False
        else:
            result.to_csv(output_path, mode='a', header=False, index=False)

# 示例：过滤并计算新列
def process_row(chunk):
    chunk = chunk[chunk['amount'] > 100]
    chunk['amount_usd'] = chunk['amount'] * 1.0
    return chunk

stream_process('big_file.csv', 'output.csv', process_row, chunk_size=5000)
```

---

## 数据清洗

### 常见问题检查表

```markdown
# 数据质量检查

## 行级检查
- [ ] 总行数: ____
- [ ] 重复行数: ____
- [ ] 含空值行数: ____

## 列级检查
- [ ] 每列空值数量
- [ ] 每列唯一值数量
- [ ] 数值列的 min/max/mean
- [ ] 日期列的范围

## 格式检查
- [ ] 日期格式是否统一
- [ ] 数字是否含符号/千分位
- [ ] 文本是否有前后空格
```

### 快速检查代码

```python
def check_data_quality(df):
    """数据质量检查报告"""
    print(f"总行数: {len(df)}")
    print(f"总列数: {len(df.columns)}")
    print(f"\n重复行: {df.duplicated().sum()}")
    print(f"\n空值统计:")
    print(df.isnull().sum())
    print(f"\n数据类型:")
    print(df.dtypes)
    print(f"\n数值列统计:")
    print(df.describe())
```

### 数据验证

```python
def validate_rows(df, schema):
    """
    验证数据
    
    schema: dict, 如 {'amount': 'float', 'email': 'email', 'date': 'date'}
    返回: (valid_df, error_list)
    """
    import re
    
    errors = []
    valid_indices = []
    
    for i, row in df.iterrows():
        row_errors = []
        for col, dtype in schema.items():
            val = row.get(col)
            if pd.isna(val) or str(val).strip() == '':
                continue
            
            val_str = str(val).strip()
            
            if dtype == 'int':
                try:
                    int(val_str)
                except ValueError:
                    row_errors.append(f"{col}: '{val}' 不是整数")
            
            elif dtype == 'float':
                try:
                    float(val_str)
                except ValueError:
                    row_errors.append(f"{col}: '{val}' 不是数字")
            
            elif dtype == 'email':
                if not re.match(r'^[^@]+@[^@]+\.[^@]+$', val_str):
                    row_errors.append(f"{col}: '{val}' 不是有效邮箱")
            
            elif dtype == 'date':
                if not re.match(r'^\d{4}-\d{2}-\d{2}', val_str):
                    row_errors.append(f"{col}: '{val}' 不是 YYYY-MM-DD 格式")
        
        if row_errors:
            errors.append({'row': i, 'errors': row_errors, 'data': row.to_dict()})
        else:
            valid_indices.append(i)
    
    valid_df = df.loc[valid_indices]
    return valid_df, errors

# 使用示例
schema = {'amount': 'float', 'email': 'email', 'date': 'date'}
valid_df, errors = validate_rows(df, schema)
print(f"有效行: {len(valid_df)}, 错误: {len(errors)}")
```

---

## 统计分析

### Descriptive Statistics

| Statistic | Description |
|-----------|-------------|
| Mean | |
| Median | |
| Mode | |
| Std Dev | |
| Min/Max | |
| Percentiles | |

*(留白，支持后续新增)*

---

## 分析工作流

### Standard Analysis Process

1. **Define the Question**
   - What are we trying to answer?
   - What decisions will this inform?

2. **Understand the Data**
   - What data is available?
   - What's the structure and quality?

3. **Clean and Prepare**
   - Handle missing values
   - Fix data types
   - Remove duplicates

4. **Explore**
   - Descriptive statistics
   - Initial visualizations
   - Identify patterns

5. **Analyze**
   - Deep dive into findings
   - Statistical tests if needed
   - Validate hypotheses

6. **Communicate**
   - Clear visualizations
   - Actionable insights
   - Recommendations

### Analysis Request Template

```markdown
# Analysis Request

## Question
[What are we trying to answer?]

## Context
[Why does this matter? What decision will it inform?]

## Data Available
- [Dataset 1]: [Description]
- [Dataset 2]: [Description]

## Expected Output
- [Deliverable 1]
- [Deliverable 2]

## Timeline
[When is this needed?]

## Notes
[Any constraints or considerations]
```

---

## 报告生成

### 标准报告结构

```markdown
# [Report Name]

**Period:** [Date range]
**Generated:** [Date]

## Executive Summary
[2-3 sentences with key findings]

## Key Metrics
| Metric | Current | Previous | Change |
|--------|---------|----------|--------|
| [Metric] | [Value] | [Value] | [+/-X%] |

## Findings
1. **[Finding]**: [Evidence]
2. **[Finding]**: [Evidence]

## Recommendations
1. [Actionable recommendation]
2. [Actionable recommendation]

## Methodology
- Data source: [Source]
- Date range: [Range]
- Filters applied: [Filters]
```

---

## 最佳实践

1. **先明确问题** — 知道要回答什么
2. **验证数据质量** — 垃圾进，垃圾出
3. **记录所有步骤** — 查询、假设、决策
4. **展示工作过程** — 方法论很重要
5. **给出可操作结论** — "然后呢？" 比 "是什么" 重要

---

## 常见错误

1. ❌ **确认偏差** — 只找支持结论的数据
2. ❌ **相关 ≠ 因果** — 谨慎下结论
3. ❌ **忽略异常值** — 调查后再决定是否删除
4. ❌ **过度复杂化** — 简单分析往往更有效
5. ❌ **缺少上下文** — 没有对比的数字没有意义

---

## License

MIT — use freely, modify, distribute.
