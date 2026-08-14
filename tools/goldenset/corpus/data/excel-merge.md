---
name: excel-merge
description: |
  Merge multiple same-structure Excel files into one — validates header consistency then appends rows. Format inherits from the first file.
  合并多个同结构的 Excel 文件为一个——验证表头一致后按行追加。格式继承第一个文件。
  Trigger keywords: "merge" "combine" "concatenate" "consolidate" "merge multiple files" "append"
  触发词包括"合并""拼接""合在一起""汇总""多个文件合并""追加"。
---

> This skill follows [[excel-safe-workflow]]. Small files use pandas concat + openpyxl write-back, large files use XML row append.
> 本技能遵循 [[excel-safe-workflow]]。小文件用 pandas concat + openpyxl 写回，大文件用 XML 行追加。

# Excel Merge / Excel 合并

## 功能

```
文件1.xlsx (1000行)  ─┐
文件2.xlsx (800行)   ─┤
文件3.xlsx (1200行)  ─┼──→ 合并结果.xlsx (3000行)
...                  ─┘
```

## 第零步：需求解析

| 要素 | 用户说 | 默认值 |
|------|--------|--------|
| **文件列表** | "把这三个文件合并" / "合并这个文件夹里的所有xlsx" | 必须明确 |
| **输出文件** | "输出到 merged.xlsx" | `合并结果.xlsx` |
| **表头处理** | — | 第一行是表头，只保留一次 |

## 第一步：勘察——验证表头一致

```python
import pandas as pd, os

FILES = ['文件1.xlsx', '文件2.xlsx', ...]

# 读表头
headers = {}
for fp in FILES:
    df = pd.read_excel(fp, nrows=0)
    headers[fp] = list(df.columns)

# 对比
base = headers[FILES[0]]
print(f'基准表头 ({len(base)} 列): {FILES[0]}')
all_match = True
for fp in FILES[1:]:
    h = headers[fp]
    if h != base:
        print(f'  ❌ {fp}: 表头不匹配!')
        # 列出差异
        only_base = set(base) - set(h)
        only_this = set(h) - set(base)
        if only_base: print(f'    缺少列: {only_base}')
        if only_this: print(f'    多余列: {only_this}')
        all_match = False

if not all_match:
    print('请确认是否强制合并（缺失列填空）')
```

## 第二步：规划

- 确认所有文件表头一致（不一致时询问是否强制合并）
- 估算总行数
- 选引擎：总文件 <10MB 用 pandas，否则用 XML

## 第三步：执行

### 小文件 — pandas + openpyxl

```python
import pandas as pd
from openpyxl import load_workbook
import shutil, os

FILES = ['文件1.xlsx', ...]
OUTPUT = '合并结果.xlsx'

# 读取并拼接
dfs = []
total = 0
for fp in FILES:
    df = pd.read_excel(fp)
    dfs.append(df)
    total += len(df)
    print(f'  {os.path.basename(fp)}: {len(df)} 行')

merged = pd.concat(dfs, ignore_index=True)
print(f'合并: {total} 行')

# 用第一个文件做模板，写回数据
shutil.copy2(FILES[0], OUTPUT)
wb = load_workbook(OUTPUT)
ws = wb.active

# 清空数据行（保留表头）
for row in range(2, ws.max_row + 1):
    for col in range(1, ws.max_column + 1):
        ws.cell(row=row, column=col).value = None

# 写入合并数据（从第2行开始）
for r_idx, row_data in merged.iterrows():
    for c_idx, val in enumerate(row_data):
        ws.cell(row=r_idx + 2, column=c_idx + 1).value = val
    if r_idx % 10000 == 0:
        print(f'  进度: {r_idx}/{total}')

wb.save(OUTPUT)
print(f'输出: {OUTPUT} ({total} 行)')
```

### 大文件 — XML 行追加（含 sharedStrings inline 化）

> ⚠️ **关键**：不同文件各自有独立的 `sharedStrings.xml`，直接拼接 `<row>` 会导致引用断裂。
> 合并时**必须**将 `t="s"` 单元格转为内联字符串，最终输出空 sharedStrings。

```python
import zipfile, os, shutil, re
from lxml import etree

FILES = ['文件1.xlsx', ...]
OUTPUT = '合并结果.xlsx'
S_NS = 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'

# 以第一个文件为基础
shutil.copy2(FILES[0], OUTPUT)

# 解压基础文件
TMP = OUTPUT.replace('.xlsx', '_merge_tmp')
if os.path.exists(TMP): shutil.rmtree(TMP)
os.makedirs(TMP)
with zipfile.ZipFile(OUTPUT, 'r') as z:
    z.extractall(TMP)

ws_dir = os.path.join(TMP, 'xl', 'worksheets')
parser = etree.XMLParser(remove_blank_text=False, huge_tree=True)

# 找到主 sheet
sheet_path = None
for sf in sorted(os.listdir(ws_dir)):
    if sf.endswith('.xml') and sf.startswith('sheet'):
        sheet_path = os.path.join(ws_dir, sf)
        break

tree = etree.parse(sheet_path, parser)
root = tree.getroot()
ns = {'s': S_NS}

# 获取当前最大行号
existing_rows = [int(re.get('r')) for re in root.findall('.//s:row', ns)]
next_row = max(existing_rows) + 1 if existing_rows else 2

# ====== 处理第一个文件的 inline 化 ======
# 第一个文件作为基础也需要 inline 化（它的 sharedStrings 仍指向原文件）
# 先读第一个文件的 sharedStrings
src_ss_path = os.path.join(TMP, 'xl', 'sharedStrings.xml')
si_lookup_first = {}
if os.path.exists(src_ss_path):
    ss_tree = etree.parse(src_ss_path, parser)
    for idx, si in enumerate(ss_tree.findall('.//{'+S_NS+'}si')):
        t = si.find('{'+S_NS+'}t')
        si_lookup_first[idx] = t.text if t is not None else ''

# inline 化第一个文件的已有行
for row_elem in root.findall('.//s:row', ns):
    for cell in row_elem.findall('s:c', ns):
        if cell.get('t') == 's':
            v_elem = cell.find('s:v', ns)
            if v_elem is not None and v_elem.text:
                si = int(v_elem.text)
                val = si_lookup_first.get(si, '')
                cell.set('t', 'inlineStr')
                for child in list(cell):
                    tag = child.tag.split('}')[-1]
                    if tag in ('v', 'f', 'is'): cell.remove(child)
                is_new = etree.SubElement(cell, '{'+S_NS+'}is')
                t_new = etree.SubElement(is_new, '{'+S_NS+'}t')
                t_new.text = val

# ====== 逐个追加其他文件的数据行 ======
total_appended = 0
for fp in FILES[1:]:
    print(f'  追加: {os.path.basename(fp)}...')
    # 解压源文件
    src_tmp = fp.replace('.xlsx', '_src_tmp')
    if os.path.exists(src_tmp): shutil.rmtree(src_tmp)
    os.makedirs(src_tmp)
    with zipfile.ZipFile(fp, 'r') as z:
        z.extractall(src_tmp)

    # ====== 读源文件 sharedStrings ======
    si_lookup = {}
    src_ss_path = os.path.join(src_tmp, 'xl', 'sharedStrings.xml')
    if os.path.exists(src_ss_path):
        src_ss_tree = etree.parse(src_ss_path, parser)
        for idx, si_elem in enumerate(src_ss_tree.findall('.//{'+S_NS+'}si')):
            t_elem = si_elem.find('{'+S_NS+'}t')
            si_lookup[idx] = t_elem.text if t_elem is not None else ''

    src_sheet = os.path.join(src_tmp, 'xl', 'worksheets', 'sheet1.xml')
    src_tree = etree.parse(src_sheet, parser)
    src_root = src_tree.getroot()

    # 找到 <sheetData> 元素
    sheet_data = root.find('.//s:sheetData', ns)
    if sheet_data is None:
        sheet_data = etree.SubElement(root, '{'+S_NS+'}sheetData')

    appended = 0
    for row_elem in src_root.findall('.//s:row', ns):
        r = int(row_elem.get('r'))
        if r == 1:  # 跳过表头
            continue

        # ====== 关键：inline 化所有 t="s" 的 cell ======
        for cell in row_elem.findall('s:c', ns):
            if cell.get('t') == 's':
                v_elem = cell.find('s:v', ns)
                if v_elem is not None and v_elem.text:
                    si = int(v_elem.text)
                    val = si_lookup.get(si, '')
                    cell.set('t', 'inlineStr')
                    for child in list(cell):
                        tag = child.tag.split('}')[-1]
                        if tag in ('v', 'f', 'is'): cell.remove(child)
                    is_new = etree.SubElement(cell, '{'+S_NS+'}is')
                    t_new = etree.SubElement(is_new, '{'+S_NS+'}t')
                    t_new.text = val

            # 更新行号引用
            old_ref = cell.get('r', '')
            m = re.match(r'([A-Z]+)(\d+)', old_ref)
            if m:
                cell.set('r', f'{m.group(1)}{next_row}')
            # 更新公式中的行引用
            f_elem = cell.find('s:f', ns)
            if f_elem is not None and f_elem.text:
                offset = next_row - r
                def shift_ref(m):
                    return f'{m.group(1)}{int(m.group(2)) + offset}'
                f_elem.text = re.sub(r'([A-Z]+)(\d+)', shift_ref, f_elem.text)

        row_elem.set('r', str(next_row))
        sheet_data.append(row_elem)
        next_row += 1
        appended += 1

    shutil.rmtree(src_tmp)
    total_appended += appended
    print(f'    追加 {appended} 行 (累计 {total_appended})')

# ====== 写入空的 sharedStrings（openpyxl 需要它存在）======
empty_ss = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="0" uniqueCount="0"/>'
ss_path_out = os.path.join(TMP, 'xl', 'sharedStrings.xml')
with open(ss_path_out, 'wb') as f:
    f.write(empty_ss.encode('utf-8'))

# 更新 dimension
dim = root.find('.//s:dimension', ns)
if dim is not None:
    all_cols = set()
    for re_elem in root.findall('.//s:row', ns):
        for c in re_elem.findall('s:c', ns):
            m = re.match(r'([A-Z]+)', c.get('r', ''))
            if m: all_cols.add(m.group(1))
    if all_cols:
        max_col = max(all_cols, key=lambda x: (len(x), x))
        dim.set('ref', f'A1:{max_col}{next_row - 1}')

# 写回 + 打包
sheet_xml = etree.tostring(root, xml_declaration=True, encoding='UTF-8', standalone=True)
with open(sheet_path, 'wb') as f:
    f.write(sheet_xml)

with zipfile.ZipFile(OUTPUT, 'w', zipfile.ZIP_DEFLATED) as zout:
    for dirpath, _, filenames in os.walk(TMP):
        for fn in filenames:
            full = os.path.join(dirpath, fn)
            zout.write(full, os.path.relpath(full, TMP).replace('\\\\', '/'))
shutil.rmtree(TMP)

print(f'合并完成: {len(FILES)} 文件 → {OUTPUT} ({total_appended + existing_rows - 1} 行)')
```

## 第四步：验证

```python
import pandas as pd
df = pd.read_excel(OUTPUT).dropna(how='all')
# 验证总行数、表头正确
```

## 注意事项

1. **表头必须一致**：不一致时先统一再合并
2. **sharedStrings 自动 inline 化**：合并过程中所有 `t="s"` 单元格自动转为内联字符串，最终输出空 sharedStrings.xml，确保数据不会因索引断裂而错乱
3. **公式行号偏移**：XML 方案会自动调整追加行的公式引用
4. **大文件用 XML**：>10MB 自动走 XML 追加方案
5. **格式继承首个文件**：样式、列宽等取自第一个文件
6. **操作前必备份**：合并是高风险操作——务必在合并前备份所有源文件。输出文件出错时从备份恢复
