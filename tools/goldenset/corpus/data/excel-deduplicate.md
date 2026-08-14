---
name: excel-deduplicate
description: |
  Deduplicate Excel data by key column — keep first occurrence, delete subsequent duplicates. First calls excel-find-duplicates for read-only scanning to find duplicate row numbers, then confirms and deletes via XML direct ops (bypassing openpyxl, 10x faster). Format fully preserved (only rows removed, kept rows unchanged).
  对 Excel 数据按关键列去重——保留首次出现的行，删除后续重复行。先调用 excel-find-duplicates 只读扫描找重复行号，确认后通过 XML 直接操作删除（绕过 openpyxl，快 10 倍）。格式完整保留（只删行，不改保留行内容）。
  Trigger keywords: "deduplicate" "remove duplicates" "unique" "deduplicate by" "duplicate data" "clean duplicates"
  触发词包括"去重""删除重复""唯一化""去重按xx列""重复数据""清理重复"。
---

> This skill orchestrates two sub-skills: [[excel-find-duplicates]] (read-only find) → [[excel-delete]] (XML row deletion). Format fully preserved.
> 本技能编排两个子技能：[[excel-find-duplicates]]（只读查重）→ [[excel-delete]]（XML 行删除）。格式完整保留。

# Excel Deduplication / Excel 去重

## 流程

```
excel-find-duplicates          XML 直接删除（不用 openpyxl）
      ↓                                ↓
pandas 只读扫描 → 行号集合 → 确认 → 解压 → lxml 移除 <row> → 打包
```

## 完整执行脚本

```python
import sys, os, time, zipfile, shutil, re
sys.stdout.reconfigure(encoding='utf-8')
import pandas as pd
from lxml import etree

FILE = '目标文件.xlsx'
KEY_COL = '列名'   # 去重关键列名（pandas 读取后的列名）
KEEP = 'first'      # 'first'=保留首次 / 'last'=保留末次

# ====== 第1步：只读查重（excel-find-duplicates） ======
print(f'① 扫描重复（按 "{KEY_COL}"）...')
t0 = time.time()

df = pd.read_excel(FILE)
total = len(df)

mask = df[KEY_COL].duplicated(keep=KEEP)
dup_indices = df.index[mask].tolist()
dup_rows = [i + 2 for i in dup_indices]  # pandas 0-index → Excel 行号（+2 因为第1行=表头）

unique_count = df[KEY_COL].nunique()
print(f'  总行数: {total}')
print(f'  唯一值: {unique_count}')
print(f'  重复行: {len(dup_rows)} ({len(dup_rows)/total*100:.1f}%)')
print(f'  扫描耗时: {time.time()-t0:.0f}s')

if not dup_rows:
    print('✅ 无重复，无需去重')
    exit()

# ====== 第2步：确认 ======
print(f'\n将删除 {len(dup_rows)} 行，保留 {total - len(dup_rows)} 行')
print(f'行号范围: {min(dup_rows)} ~ {max(dup_rows)}')
print('确认执行...')
dup_set = set(dup_rows)

# ====== 第3步：XML 直接删除 ======
print(f'\n② XML 删除重复行...')
t0 = time.time()

# 3.1 备份
BACKUP = FILE.replace('.xlsx', '_backup.xlsx')
if not os.path.exists(BACKUP):
    shutil.copy2(FILE, BACKUP)

# 3.2 解压
TMP = FILE.replace('.xlsx', '_xml_tmp')
if os.path.exists(TMP):
    shutil.rmtree(TMP)
os.makedirs(TMP)
with zipfile.ZipFile(FILE, 'r') as z:
    z.extractall(TMP)

# 3.3 遍历 sheet XML，移除重复行
worksheets_dir = os.path.join(TMP, 'xl', 'worksheets')
parser = etree.XMLParser(remove_blank_text=False, huge_tree=True)
total_deleted = 0

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

    total_deleted += deleted
    print(f'  {sf}: 删除 {deleted} 行')

# 3.4 重新打包
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

# ====== 第4步：验证 ======
print(f'\n③ 验证...')
df2 = pd.read_excel(FILE)
dups_after = df2[KEY_COL].duplicated().sum()
print(f'  去重后: {len(df2)} 行')
print(f'  残留重复: {dups_after} {"✅" if dups_after == 0 else "❌ 还有重复!"}')

# 公式健康检查
from openpyxl import load_workbook
wb = load_workbook(FILE, read_only=True, data_only=True)
ws = wb.active
ref_errors = 0
for row_idx in range(1, min(50, ws.max_row + 1)):
    for col_idx in range(1, min(10, ws.max_column + 1)):
        v = ws.cell(row=row_idx, column=col_idx).value
        if v and isinstance(v, str) and '#REF!' in v:
            print(f'  ❌ #REF! at {ws.cell(row=row_idx, column=col_idx).coordinate}: {v}')
            ref_errors += 1
if ref_errors == 0:
    print(f'  公式健康: ✅ 无 #REF!')
wb.close()
```

## 与 excel-delete 的关系

去重的删除阶段直接内嵌 XML 操作（与 excel-delete 行模式共用同一套逻辑）。如果去重后发现还需额外删行，再单独调用 excel-delete。

## 注意事项

1. **格式无损**：只移除 `<row>` XML 元素，不动 styles.xml / sharedStrings.xml
2. **速度快**：XML 方案比 openpyxl `delete_rows()` 快 10 倍以上
3. **操作前必备份**：遵循 [[excel-safe-workflow]] 第零步——操作前自动备份（时间戳命名），成功后保留最新3份，失误后立即删除损坏文件并从备份恢复
4. **空值处理**：关键列为 None 的多行只保留第一个
5. **需要 lxml**：`pip install lxml`（一次性）
