# safereflow · 动态安全区字幕重排器

纯 Go 标准库实现。针对“安全区随镜头移动而变化、重排器却把整条视频当成同一个矩形”的问题：

1. 按**时间区间**读取动态安全区（JSON / CSV，支持归一化坐标与逐段置信度）；
2. 把每条 cue 的时间区间与安全区区间切分对齐，逐段计算 **cue 框 × 安全矩形的交集、溢出边与交集占比**；
3. 给出 **拆行 `rewrap`、缩短 `shorten`、迁移 `migrate`** 方案，以及处理镜头大幅漂移的 **按时间分段迁移 `time_split`**；
4. 置信度是硬门禁：低置信度 / 覆盖不全时，迁移方案只输出坐标、**绝不会被自动执行**。

## 根因与模型

旧重排器隐含一个假设：全片只有一个安全矩形。镜头运动后，安全区在 `[start,end)` 各段里有不同的 `Rect`，
用任意一个固定矩形去校验整条 cue，要么漏报（用过宽的旧矩形），要么误报（用过窄的新矩形）。

本工程把两个坐标维度都做成“区间”：

- 时间：所有时间为秒，区间统一是半开区间 `[Start, End)`；
- 空间：画布像素坐标，左上角原点，`Rect{X,Y,Width,Height}`。

对一条 cue `[cs,ce)`，用所有安全区边界把它切成原子时间片 `Slice`；重叠的多个安全区按**交集**合并（最保守），
没有任何区间覆盖的时间段显式标记为 `covered=false` 的覆盖空洞——“未知”不会被当成“安全”。

每个时间片计算：

- `intersection`：cue 框与有效安全矩形的几何交集；
- `overflow{left,right,top,bottom}`：四条边各自越界多少像素；
- `intersection_ratio`：交集面积 / cue 框面积；
- `inside`：是否完全位于安全区内；
- `confidence / trusted`：该段跟踪是否可信。

再对 cue 全时段求所有安全矩形的交集，得到“全程公共安全矩形” `common_safe_rect`：
固定位置的方案只需通过这一个矩形校验，就保证整条 cue 在任何时刻都安全。

## 输入格式

动态安全区 `safe_areas.json`（完整示例见 `testdata/safe_areas.json`）：

```json
{
  "canvas": { "width": 1920, "height": 1080 },
  "unit": "pixel",
  "confidence_threshold": 0.8,
  "safe_areas": [
    { "id": "wide", "start": 0, "end": 4,
      "rect": { "x": 120, "y": 120, "width": 1680, "height": 860 },
      "confidence": 0.95, "source": "camera-track/wide" },
    { "id": "closeup", "start": 4, "end": 8,
      "rect": { "x": 420, "y": 200, "width": 1080, "height": 700 },
      "confidence": 0.92 }
  ]
}
```

- `unit` 可为 `pixel`（默认）或 `normalized`（矩形取值 0..1，按 canvas 缩放）；
- `confidence` 可省略：省略即“未知”，不会通过置信度门禁；
- 也支持 CSV：表头至少含 `start,end,x,y,width,height`，可选 `id,confidence,source`，见 `testdata/safe_areas.csv`；
- cue 输入支持 JSON（可带显式 `box`、对齐与样式）和 SubRip `.srt`（用内置排版度量推导 box）。

## 判定状态

`Finding.Status` 取值：

- `safe`：全程覆盖且无几何越界；
- `violation`：存在**可信**时间片明确越界；
- `suspected`：仅在低/未知置信度时间片观察到越界；
- `indeterminate`：存在覆盖空洞，无法给出确定结论。

## 置信度铁律

每条 finding / plan 都带同一个 `Gate`：

```text
trusted          = 全程覆盖 && 置信度已知 && min(confidence) >= threshold
auto_allowed     = trusted        // 默认严格策略：任何自动修改都要求可信
migration_allowed = trusted       // 迁移硬门禁，任何自动路径都不可绕过
block_reasons    = coverage_gap | confidence_unknown | low_confidence
```

因此：

- 低置信度安全区上的迁移方案 `feasible=true`（坐标照算，供人工/NLE 使用），但 `auto_eligible=false`；
- `ApplyAuto`（等价于不带 `PlanIDs` 的 `Apply`）只应用“推荐且 `auto_eligible`”的方案，低置信 cue 原框不动，
  并在 `apply-report.json` 记录 `low_confidence_move_not_automatic`；
- 即使显式 `Apply({PlanIDs:["...-migrate"]})`，默认仍返回 `ErrLowConfidence` 且**原子地不改动文档**；
- 只有显式设置 `ApplyOptions.AcknowledgeLowConfidenceMove=true`（代表人工复核）才能执行，
  审计模式记为 `manual_low_confidence_ack`。CLI 不提供这个开关。

## 方案是怎么算的

- **拆行 `rewrap`**：把换行宽度收到公共安全区内容宽，用贪心换行（拉丁按词、CJK 逐字），保持对齐锚点不动；
  行数超过 `max_lines` 或文本块仍超高则判不可行。若还需一次最小平移，plan 会带 `move` 并按迁移门禁处理。
- **缩短 `shorten`**：二分求“能放下的最长保留长度”，按拉丁词边界回退并加 `…`。属于内容损失，
  即使高置信也**不会自动应用**，必须显式选择该 plan。
- **迁移 `migrate`**：排版尺寸不变，用 `MinimalTranslation` 求把 box 移入公共安全矩形的最小 `(dx,dy)`。
  box 比公共矩形还大时不可行，提示改用拆行/缩短。
- **分段迁移 `time_split`**：镜头漂移导致两个安全矩形没有能容纳 cue 的公共位置时，按时间边界拆分 cue，
  每段独立求目标 box，相邻且位移相同的段会合并。

推荐顺序固定为：无损拆行 → 不损失内容的迁移 → 缩短 → 分段迁移；只推荐 `feasible` 的方案。

文本度量由 `Measurer` 接口抽象，默认 `GridMeasurer` 用等宽估计（CJK/全角 1.0em、窄字符 0.56em、空格 0.3em），
无外部依赖；接入真实渲染器时可替换为 FreeType/HarfBuzz 实现。

## 命令行

```bash
go build ./cmd/safereflow

# 只分析，输出完整 JSON 报告（含每个时间片交集与全部方案坐标）
safereflow -safe testdata/safe_areas.json -cues testdata/cues.json -out report.json

# 应用自动方案（高置信拆行/迁移），低置信迁移保持不动
safereflow -safe testdata/safe_areas.json -cues testdata/cues.json \
  -apply -write-cues out.cues.json -write-srt out.srt \
  -apply-report apply.json -pixel-align

# 直接吃 SRT（SRT 无几何/画布，需要 -canvas 与默认样式推导 box）
safereflow -safe testdata/safe_areas.json -srt testdata/subtitles.srt -canvas 1920x1080

# CSV 安全区
safereflow -safe testdata/safe_areas.csv -cues testdata/cues.json

# 显式应用某个方案；低置信迁移会以退出码 3 拒绝
safereflow -safe ... -cues ... -plan c-lowconf-migrate
```

退出码：`0` 成功；`1` 输入/分析/写出错；`2` 参数错误；`3` 显式应用被置信度门禁阻止。

`examples/` 下有一份对 `testdata` 跑 `-apply -pixel-align` 的真实产物：`report.json`、
`apply-report.json`、`reflowed-cues.json`、`reflowed.srt`。

## Go 库用法

```go
track, _ := safereflow.LoadSafeFile("safe_areas.json") // 或 LoadSafeCSVFile
doc, _   := safereflow.LoadCueFile("cues.json")

eng := safereflow.NewEngine(doc, track)
rep, _ := eng.Analyze()

for _, f := range rep.Findings {
    fmt.Println(f.CueID, f.Status, f.Gate.Trusted)
    for _, seg := range f.Segments {           // 逐时间片交集
        _ = seg.Intersection
        _ = seg.Overflow
    }
    if f.RecommendedPlanID != "" { /* ... */ }
}

newDoc, applied, _ := eng.ApplyAuto(rep)        // 低置信迁移不会被应用
_ = safereflow.WriteJSON("out.cues.json", newDoc)
```

`Engine.Apply` 在文档深拷贝上修改，输入文档不会被改动；应用后的 box 会回写为显式矩形，
因此对结果再跑一次 `Analyze` 可以验证闭环（测试中的 “apply → re-analyze → safe” 即此性质）。

## 验证

```bash
gofmt -w .
go vet ./...
go test ./...
```

测试覆盖：矩形求交/溢出/最小平移、归一化与参数校验、时间切片与覆盖空洞、未知置信度、
高置信迁移闭环、低置信禁止自动移动（含显式选择被拒与人工 ack 审计）、覆盖空洞锁定、
拆行/缩短/分段迁移的应用与复检安全、SRT 往返、CSV 与 CLI 端到端。

## 目录

- `geom.go`：矩形交集、包含、溢出、最小平移、取整；
- `model.go` / `safearea.go` / `safe_csv.go`：领域模型与动态安全区读取、切片、公共矩形；
- `measure.go` / `cue.go`：文本度量、贪心换行、锚点与 cue box 解析；
- `analyze.go`：逐片求交、状态判定、`Gate` 置信度门禁；
- `plan.go` / `plan_rewrap.go` / `plan_shorten.go` / `plan_migrate.go`：方案生成与推荐；
- `apply.go`：深拷贝、自动/显式应用、硬门禁与分段 cue 拆分；
- `io.go` / `srt.go` / `cli.go`：JSON IO、SRT、命令行；
- `cmd/safereflow/main.go`：CLI 入口；
- `testdata/`：安全区 JSON/CSV、cue JSON、SRT 样例；
- `*_test.go`：单元与端到端测试。
