# fontaudit — 字幕烧录字形覆盖审计（Go）

烧录（burn-in）后部分中文、希腊字母、特殊标点显示为方框，但字幕源文件本身不缺字符。
本工具用 Go 复现并定位这一类问题，输出**缺失字符、首次出现时间、替换候选与回退字体**，
并渲染豆腐块证据图。它只读文件、只给建议，**绝不静默替换字体，也不改写字幕**。

## 它检查什么

1. **字体字符覆盖（cmap）**：直接解析 TTF/OTF/TTC/OTC 的 `cmap` 表，字形索引为 0
   （`.notdef`）即判定为烧录时会画方框，而不是依赖 fontconfig 猜测。支持 TTC/OTC 多字面。
2. **字形回退（fallback）**：扫描指定字体目录，找出真正包含缺失字形的字体，按覆盖率
   排序作为**候选**，主字体自身被排除。候选不会被自动启用。
3. **渲染编码**：识别 UTF-8（含/不含 BOM）、UTF-16 LE/BE（含/不含 BOM）、UTF-32、
   GB18030/GBK，统计 `U+FFFD` 替换符、内嵌 NUL，并检测双重编码（mojibake）。

## 安装与构建

```bash
go build -o bin/fontaudit ./cmd/fontaudit
```

## 用法

```bash
# 基本审计：主字体 + 回退扫描目录 + 豆腐块证据图
bin/fontaudit \
  -font testdata/fonts/NimbusSans-Regular.otf \
  -fallback-dirs testdata/fonts/fallback \
  -proof-dir reports/proof \
  testdata/subtitles/lecture.srt

# TTC/OTC 多字面用 -face 选择
bin/fontaudit -font testdata/fonts/collection.ttc -face 1 testdata/subtitles/lecture.vtt

# 机器可读 JSON
bin/fontaudit -json -font primary.ttf -fallback-dirs /usr/share/fonts/truetype sub.ass
```

退出码：`0` 无缺失；`1` 存在豆腐块或编码损坏（可用于 CI 门禁）；`2` 用法/IO 错误。

## 报告字段

- 每个缺失字形：码位、显示字符、Unicode 区块/文字/类别、命中次数、**首次出现的 cue 与
  时间戳**、所在 cue 列表。
- **替换候选**（仅供人工评审）：
  - `punctuation-alias`：排版标点到 ASCII/CJK 基础标点的等价映射（如 `：`→`:`）。
  - `nfkc`：NFKC 兼容性折叠（全角/半角、带圈数字、合字等，如 `①`→`1`）。
  - 对 CJK 汉字、希腊字母等**不提供删除式替换**，只建议加装覆盖字体。
- **回退字体**：实际包含该字形的字体路径与家族名，标注“需显式批准”。
- 证据 PNG：用主字体、**不接任何回退链**真实绘制，缺字处画空心方框。

## 设计约束

- 不修改字幕字节、不替换主字体；所有修复动作都以候选/建议形式输出。
- 字幕中声明的字体（ASS `Style` 的 Fontname、行内 `\fn`、SRT/VTT `<font face>`）被
  原样记录，便于核对烧录管线实际请求的字体。

## 目录

- `cmd/fontaudit`：CLI 与文本/JSON 报告。
- `cmd/genfixtures`：从规范 UTF-8 字幕再生编码变体与合成 TTC 的可复现生成器。
- `internal/subtitle`：SRT / WebVTT / ASS(SSA) 解析与标记清理。
- `internal/encodingx`：编码探测与损坏证据。
- `internal/fontscan`：sfnt cmap 覆盖、TTC 解析、回退候选扫描。
- `internal/runeinfo`：Unicode 区块/类别、标点等价与 NFKC 折叠。
- `internal/render`：主字体豆腐块证据渲染。
- `internal/audit`：汇总引擎与报告模型。
- `testdata`：可复现字幕（含 UTF-16/GB18030/mojibake 变体）与字体夹具。

## 测试与夹具再生

```bash
go test ./...
go run ./cmd/genfixtures .   # 再生 testdata 编码变体与 testdata/fonts/collection.ttc
```

## 复现“方框”场景

夹具主字体 Nimbus Sans 含希腊字母与常见排版符号，但**不含任何 CJK 汉字、全角标点、
书名号与带圈数字**；回退目录里的 Droid Sans Fallback 覆盖 CJK 却不含希腊字母。因此
`lecture.srt` 稳定复现“中文/特殊标点变方框、希腊字母正常”的现象，证据图见 `reports/proof/`。
