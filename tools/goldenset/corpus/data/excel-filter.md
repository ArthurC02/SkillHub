---
name: excel-filter
description: |
  Filter Excel data rows by condition — keep or delete rows matching criteria. First uses pandas read-only scan to identify target row numbers, then uses XML direct operations to remove unwanted rows (format-preserving). Supports equals, contains, range, date comparison, and other common filters.
  按条件筛选 Excel 数据行——保留或删除符合条件的行。先用 pandas 只读扫描确定目标行号，再用 XML 直接操作移除不需要的行（格式无损）。支持等值、包含、范围、日期比较等常见筛选。
  Trigger keywords: "filter" "keep only" "extract" "keep rows matching" "delete rows matching" "by condition"
  触发词包括"筛选""过滤""只要""提取""保留满足条件的""删除满足条件的""按条件"。
---

> This skill follows [[excel-safe-workflow]] four-step method. Filtering logic uses pandas (fast), deletion uses XML direct ops (fast + format-preserving).
> 本技能遵循 [[excel-safe-workflow]] 四步法。筛选逻辑用 pandas（快），删除用 XML 直接操作（快+格式无损）。

# Excel Filter / Excel 筛选

## Two Modes / 两种模式

| 模式 | 含义 | 用户说 |
|------|------|--------|
| **keep**（保留） | 保留符合条件的行，删除其余 | "只要2020年后的""保留已授权的" |
| **remove**（删除） | 删除符合条件的行，保留其余 | "删掉空白的""去掉无效数据" |

默认是 **keep** 模式。

## 第零步：需求解析

### 条件类型识别

| 用户说 | 条件类型 | pandas 表达式 |
|--------|----------|---------------|
| "申请日大于2020年" | 大于 | `df[col] > '2020-01-01'` |
| "申请日=2020年" | 等于 | `df[col] == '2020'` |
| "标题包含石墨烯" | 包含 | `df[col].str.contains('石墨烯', na=False)` |
| "申请人包含 华为 或 腾讯" | 包含(或) | `df[col].str.contains('华为|腾讯', na=False)` |
| "申请日在2020到2023之间" | 范围 | `(df[col] >= '2020-01-01') & (df[col] <= '2023-12-31')` |
| "申请人等于华为 且 已授权" | 多条件与 | `(df[a]=='华为') & (df[b]=='已授权')` |
| "关键列为空" | 空值 | `df[col].isna()` |
| "关键列不为空" | 非空 | `df[col].notna()` |

### 解析示例

| 用户说 | 提取 |
|--------|------|
| "只要2020年后的专利申请" | keep模式, 申请日 ≥ 2020 |
| "删掉申请人是空白的数据" | remove模式, 申请人 is null |
| "提取已授权且申请日>2022的" | keep模式, 当前法律状态=授权 AND 申请日>2022 |

## 第一步：勘察

```python
import os, sys
sys.stdout.reconfigure(encoding='utf-8')
from openpyxl import load_workbook

FILE = '目标文件.xlsx'
size_mb = os.path.getsize(FILE) / 1024 / 1024
print(f'文件大小: {size_mb:.1f} MB')

wb = load_workbook(FILE, read_only=True)
ws = wb.active
print(f'工作表: {ws.title}, 行: {ws.max_row}, 列: {ws.max_column}')

# 表头
print('\n=== 表头 ===')
for col_idx in range(1, min(ws.max_column + 1, 30)):
    h = ws.cell(row=1, column=col_idx).value
    if h:
        col_letter = chr(64 + col_idx) if col_idx <= 26 else f'col{col_idx}'
        print(f'  列{col_idx} [{col_letter}]: {h}')

# 数据样本
print('\n=== 数据样本(前5行) ===')
for row_idx in range(2, min(7, ws.max_row + 1)):
    vals = []
    for col_idx in range(1, min(6, ws.max_column + 1)):
        v = str(ws.cell(row=row_idx, column=col_idx).value or '')[:40]
        vals.append(v)
    print(f'  行{row_idx}: {" | ".join(vals)}')

wb.close()
```

## 第二步：规划

- 确认筛选列、条件、模式（keep/remove）
- 确认表头行数（第 1 行是表头还是有多行表头）
- keep 模式 → 标记不匹配的行为删除目标
- remove 模式 → 标记匹配的行为删除目标

## 第三步：执行

```python
import pandas as pd
import zipfile, os, shutil, re, time
from lxml import etree

FILE = '目标文件.xlsx'
HEADER_ROW = 0      # pandas 表头行索引（通常第1行=0），如果有多行表头需调整
MODE = 'keep'       # 'keep'=保留匹配行 / 'remove'=删除匹配行

# ====== 3.1 用 pandas 确定目标行 ======
print(f'① pandas 扫描 ({MODE} 模式)...')
t0 = time.time()

df = pd.read_excel(FILE, header=HEADER_ROW)
total = len(df)
print(f'  总数据行: {total}')

# ══════════════════════════════════════
# 条件配置区 —— 根据需求修改
# ══════════════════════════════════════
COL = '列名'         # 筛选列名
OPERATOR = 'contains'  # eq / ne / gt / gte / lt / lte / contains / isna / between
VALUE = '筛选值'      # 比较值（between 时用 (min, max)；isna 时忽略）

if OPERATOR == 'eq':
    mask = df[COL] == VALUE
elif OPERATOR == 'ne':
    mask = df[COL] != VALUE
elif OPERATOR == 'gt':
    mask = df[COL] > VALUE
elif OPERATOR == 'gte':
    mask = df[COL] >= VALUE
elif OPERATOR == 'lt':
    mask = df[COL] < VALUE
elif OPERATOR == 'lte':
    mask = df[COL] <= VALUE
elif OPERATOR == 'contains':
    mask = df[COL].astype(str).str.contains(VALUE, na=False)
elif OPERATOR == 'isna':
    mask = df[COL].isna()
elif OPERATOR == 'notna':
    mask = df[COL].notna()
elif OPERATOR == 'between':
    mask = (df[COL] >= VALUE[0]) & (df[COL] <= VALUE[1])
# 多条件示例（按需组合）:
# mask = (df['申请人'].str.contains('华为', na=False)) & (df['当前法律状态'] == '授权')
# ══════════════════════════════════════

match_count = mask.sum()
print(f'  匹配行数: {match_count} ({match_count/total*100:.1f}%)')

if MODE == 'keep':
    delete_count = total - match_count
    delete_indices = df.index[~mask].tolist()
else:  # remove
    delete_count = match_count
    delete_indices = df.index[mask].tolist()

# pandas索引 → Excel行号（+2 = +1表头 +1 pandas 0-index）
delete_rows = [i + 2 + HEADER_ROW for i in delete_indices]
print(f'  将删除: {delete_count} 行')
print(f'  将保留: {total - delete_count} 行')
print(f'  扫描耗时: {time.time()-t0:.1f}s')

if delete_count == 0:
    print('无需要删除的行，结束。')
    exit()

# ====== 3.2 XML 直接删除 ======
print(f'\n② XML 删除...')
t0 = time.time()
dup_set = set(delete_rows)

# 备份
BACKUP = FILE.replace('.xlsx', '_backup.xlsx')
if not os.path.exists(BACKUP):
    shutil.copy2(FILE, BACKUP)

# 解压
TMP = FILE.replace('.xlsx', '_xml_tmp')
if os.path.exists(TMP):
    shutil.rmtree(TMP)
os.makedirs(TMP)
with zipfile.ZipFile(FILE, 'r') as z:
    z.extractall(TMP)

# 遍历 sheet XML
worksheets_dir = os.path.join(TMP, 'xl', 'worksheets')
parser = etree.XMLParser(remove_blank_text=False, huge_tree=True)

for sf in sorted(os.listdir(worksheets_dir)):
    if not sf.endswith('.xml'):
        continue
    sp = os.path.join(worksheets_dir, sf)
    tree = etree.parse(sp, parser)
    root = tree.getroot()
    ns = {'s': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}

    deleted = 0
    for row_elem in root.findall('.//s:row', ns):
        if int(row_elem.get('r')) in dup_set:
            row_elem.getparent().remove(row_elem)
            deleted += 1

    if deleted == 0:
        continue

    # 清理合并单元格
    for mc in root.findall('.//s:mergeCells/s:mergeCell', ns):
        m = re.match(r'[A-Z]+(\d+):[A-Z]+(\d+)', mc.get('ref', ''))
        if m:
            r1, r2 = int(m.group(1)), int(m.group(2))
            if all(r in dup_set for r in range(r1, r2 + 1)):
                mc.getparent().remove(mc)

    # 更新 dimension
    dim = root.find('.//s:dimension', ns)
    if dim is not None:
        remaining = sorted([int(re.get('r')) for re in root.findall('.//s:row', ns)])
        all_cols = []
        for re_elem in root.findall('.//s:row', ns):
            for c in re_elem.findall('s:c', ns):
                m = re.match(r'([A-Z]+)', c.get('r', ''))
                if m: all_cols.append(m.group(1))
        if remaining and all_cols:
            max_col = max(all_cols, key=lambda x: (len(x), x))
            dim.set('ref', f'A1:{max_col}{max(remaining)}')

    sheet_xml = etree.tostring(root, xml_declaration=True, encoding='UTF-8', standalone=True)
    with open(sp, 'wb') as f:
        f.write(sheet_xml)
    print(f'  {sf}: 删除 {deleted} 行')

# 重新打包
with zipfile.ZipFile(FILE, 'w', zipfile.ZIP_DEFLATED) as zout:
    for dirpath, _, filenames in os.walk(TMP):
        for fn in filenames:
            full = os.path.join(dirpath, fn)
            zout.write(full, os.path.relpath(full, TMP).replace('\\', '/'))

shutil.rmtree(TMP)

elapsed = time.time() - t0
old_sz = os.path.getsize(BACKUP) / 1024 / 1024
new_sz = os.path.getsize(FILE) / 1024 / 1024
print(f'  删除耗时: {elapsed:.0f}s, {old_sz:.1f}MB → {new_sz:.1f}MB')
```

### ⚠️ 删除后必须询问：是否压实空白行 / Must Ask After Deletion: Compact Blank Rows?

XML 删除行后行号不连续，Excel 打开会显示空白行。**筛选/删除完成后必须询问用户**：

> "筛选完成，共删除 X 行。XML 删除后行号不连续，Excel 打开会看到空白行。是否压实行号让数据连续？"

用户确认后，执行 [[excel-delete]] 中的压实步骤（解压 → 行号重新连续编号 → 公式引用同步更新 → 打包）。

## 第四步：验证

```python
print(f'\n③ 验证...')

# 用 openpyxl 验证
from openpyxl import load_workbook
wb = load_workbook(FILE, read_only=True, data_only=True)
ws = wb.active
print(f'  当前: {ws.max_row}行 × {ws.max_column}列')

# 公式健康检查
print(f'  公式健康检查...')
ref_errors = 0
for row_idx in range(1, min(50, ws.max_row + 1)):
    for col_idx in range(1, min(10, ws.max_column + 1)):
        v = ws.cell(row=row_idx, column=col_idx).value
        if v and isinstance(v, str) and '#REF!' in v:
            print(f'  ❌ {ws.cell(row=row_idx, column=col_idx).coordinate}: {v}')
            ref_errors += 1
if ref_errors == 0:
    print(f'  ✅ 无 #REF!')

wb.close()

# 用 pandas 验证筛选结果
df2 = pd.read_excel(FILE)
print(f'  结果行数: {len(df2)}')
if MODE == 'keep':
    # 检查留下的都满足条件
    if OPERATOR == 'contains':
        not_match = df2[~df2[COL].astype(str).str.contains(VALUE, na=False)]
    elif OPERATOR == 'eq':
        not_match = df2[df2[COL] != VALUE]
    print(f'  不符合条件残留: {len(not_match)} {"✅" if len(not_match)==0 else "❌"}')
```

## 常用条件速查

```python
# 单条件
df['申请人'] == '华为技术有限公司'                            # 等于
df['申请人'].str.contains('华为', na=False)                   # 包含
df['申请日'] >= '2020-01-01'                                  # 大于等于
df['申请日'].between('2020-01-01', '2023-12-31')              # 范围
df['申请人'].isna()                                           # 为空

# 多条件
(df['申请人'].str.contains('华为', na=False)) & (df['法律状态'] == '授权')   # 与
(df['申请人'].str.contains('华为', na=False)) | (df['申请人'].str.contains('腾讯', na=False))  # 或
~(df['申请人'].str.contains('华为', na=False))                               # 非
```

## 注意事项

1. **条件用 pandas**：pandas 的字符串/日期比较比 openpyxl 逐格判断快几个数量级
2. **删除用 XML**：格式无损，比 openpyxl 快 10 倍
3. **操作前必备份**：遵循 [[excel-safe-workflow]] 第零步——操作前自动备份（时间戳命名），成功后保留最新3份，失误后立即删除损坏文件并从备份恢复
4. **日期列**：如果是 Excel 日期（datetime 类型），用 `pd.Timestamp('2020-01-01')` 比较
5. **空值**：`isna()` 匹配 None/NaN，不会匹配空字符串，如需匹配空字符串用 `df[col] == ''`
