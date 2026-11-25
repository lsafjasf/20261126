# vttlayout — WebVTT 多 region 竖屏遮挡重排器（纯 Go）

竖屏切换后，WebVTT 里按横屏 authored 的多个 `REGION` 因宽度变窄、折行变多而
互相遮挡，但每条 cue 的文字仍在自己的 region 内换行、并未越界。
`vttlayout` 用 Go 完成解析、检查、重排、无解组合检测、修复候选验证与预览帧渲染，
不依赖 Python / ffmpeg / ImageMagick 等外部程序。

## 它如何对应需求

- **按 cue 时间**：扫描全部 cue 端点，切出“活动 cue 集合恒定”的时间阶段
  （`BuildStages`），只有时间区间相交的 cue 才允许互相碰撞；时间错开的 cue
  可以复用同一垂直位置。
- **按行数**：内置 CJK / 全角感知、西文比例字宽估算与贪心折行
  （`WrapLines`），用折出的视觉行数计算每个文本块的像素高度。
- **按区域优先级**：支持扩展设置 `priority:N`（REGION 或 cue 设置均可，
  cue 优先于 region）。优先级高的 cue 优先保留其锚点位置，低优先级 cue
  寻找最近的无碰撞垂直槽位。
- **按可用画布**：内置 `landscape`、`portrait`（1080×1920）、
  `portrait-720` 预设与 `WxH` 自定义画布，均带安全区（bezel / 播放器控件）。
  重排只在安全矩形内进行。
- **无解组合检测**：对每个阶段做 X 轴扫描，求“同一横坐标上覆盖的 cue
  高度和”峰值；因为这些块在水平方向必重叠，其高度和（含间距）超过安全高度
  即得到**可认证的无解组合**（`Certified=true`）。
- **延长 / 拆行候选**：每个无解组合给出并**重新用 Go 求解验证**：
  - `split_cue` 拆行：按句读 / 逗号 / 汉字边界把最低优先级 cue 拆成两条分时 cue；
  - `extend_time` 延长错峰：按阅读速度模型计算所需时长，把 cue 移到组合之后的
    空闲时段，给出平移量与结束时间延长毫秒数；
  - `shift_time` 前置错峰：组合之前的可行槽位（不可行时如实标记 `resolves:false`）；
  - `extend_canvas` 延长画布：所需增加的安全区 / 画布高度；
  - `reduce_font` 缩小字号：搜索能让组合放下的字号。
- **预览帧由 Go 完成**：
  - `frames` 子命令为每个关键阶段输出 before/after **SVG 帧**和 `index.html`
    对照页（CJK 用系统字体栈，浏览器直接打开）；
  - `preview` 子命令在终端输出 ANSI / 纯字符帧（汉字占两列）。

## 快速开始

需要 Go 1.22+（开发环境为 go1.27，无第三方依赖）。

```bash
go test ./...

# 一键生成示例产物：fixed.vtt、report.json、候选 VTT、SVG 帧、终端预览
go run ./cmd/vttlayout demo

# 检查（存在无解组合时退出码 1，用法/解析错误退出码 2）
go run ./cmd/vttlayout check examples/portrait_overlap.vtt
go run ./cmd/vttlayout check examples/infeasible_stack.vtt --json
# CI 严格模式：只要原始排布存在遮挡/越界就退出 1
go run ./cmd/vttlayout check examples/portrait_overlap.vtt --strict

# 输出重排后的 WebVTT（每块生成一个顶部左锚定的 L<n> region，几何可回读）
go run ./cmd/vttlayout plan examples/portrait_overlap.vtt -o fixed.vtt

# 应用报告中的第 N 个修复候选（拆行 / 错峰），输出新的 VTT
go run ./cmd/vttlayout plan examples/infeasible_stack.vtt --apply 1 -o split.vtt

# 生成 before/after SVG 预览帧 + index.html
go run ./cmd/vttlayout frames examples/portrait_overlap.vtt --dir frames --at 4.5

# 终端字符预览（--mode before|after|both，--no-color）
go run ./cmd/vttlayout preview examples/portrait_overlap.vtt --at 4.5 --mode both
```

参数可以放在文件名前后（CLI 会重排 flag）。`--canvas portrait-720`、
`--canvas landscape` 或 `--canvas 1080x1920` 可切换画布。

## 内置示例

- `examples/portrait_overlap.vtt`：3 个并发 region。横屏时各 1–2 行、锚点间距
  充足；竖屏 92px（高度相对字号，对应 WebVTT 的 `vh` 字号模型）下折成 3 行，
  `host` 与 `guest` 重叠约 121px，文字均未越界。求解器让高优先级 `guest`
  （P8）留在原锚点，把 `host`（P5）上移到最近无碰撞槽位，`ticker`（P3）
  收进安全区，最终 0 冲突。
- `examples/infeasible_stack.vtt`：4 条同位置长 cue 在 3–9s 全并发，
  高度需求 1644px > 安全高度 1560px（溢出 84px），得到 1 个认证无解组合。
  最低优先级 cue 的拆行候选与延长错峰候选均被 Go 复验为 `resolves:true`。

`demo` 会把上述两个示例的 `input.vtt`、`fixed.vtt`、`report.json`、
`candidate_*.vtt` 与 `frames/*.svg` 写到 `demo-output/<case>/`。

## 处理流水线

```
Parse (parse.go)
  WEBVTT / REGION / cue / 设置 / 内联标签与实体
   └─ BuildAuthoredBoxes (geometry.go, canvas.go)
        百分比锚点 → 像素矩形；WrapLines 计算行数与块高
         ├─ Before 检查：时间相交且矩形相交 → region_overlap
         │              超出整画布        → out_of_bounds
         ├─ BuildStages / BuildClusters (timeline.go)
         ├─ Layout (layout.go)
         │    按 (-priority, start, order) 排序；
         │    每条 cue 在 {期望 Y, 安全边, 每个障碍的上/下边界}
         │    候选中选位移最小的合法 Y；再做单调松弛修复近失配
         ├─ DetectCombos：阶段内 X 扫描求高度峰值 → 认证无解组合
         ├─ BuildCandidates (repair.go, repair_time.go)
         │    生成候选 → 改写 Document → 重跑 Layout+DetectCombos
         │    → 仅当冲突时间窗消失才标记 Resolves
         ├─ Report (report.go)：文本 / JSON
         ├─ Serialize / FixedDocument / ApplyCandidate (write.go)
         └─ RenderSVGFrame (render_svg.go, frames.go)
              RenderTerminalFrame (termframe.go)
```

## 几何约定

- 坐标为目标画布的编码像素，原点左上，Y 向下。
- REGION 的 `viewportanchor` / `regionanchor` 按百分比映射；cue 的
  `line`（百分比或行数，负数从底边计）、`position`、`size`、`align`
  会覆盖 region 锚点；无 region 的 cue 落在安全区底部居中。
- 默认字号为画布高度的 ~4.8%（竖屏 1080×1920 预设为 92px）。这是竖屏
  更容易拥挤的根因：字号随高度保持很大，而折行宽度随宽度变窄。
- 字宽模型：汉字 / 假名 / 谚文 / 全角标点 = 1em，大写与数字略宽，
  其余西文约 0.58em；折行支持空格、CJK 任意字符边界与超长西文词硬切。

## 无解的判定边界

求解器保持各 cue 的水平区间（只做安全区夹紧），只重排 Y。于是在某个
时刻，凡是 X 区间覆盖同一横坐标的块都必须在垂直方向互不相交。X 扫描的
峰值高度和是**必要条件**：它超过安全高度时，任何保持水平区间的排布都
无解，因此该组合是可认证的。未通过高度界但贪心未排下的情形会标记
`Certified:false`，同样给出候选，报告中不会把两者混为一谈。

## 作为库使用

```go
doc, _ := vttlayout.Parse(vttText)
cv := vttlayout.Portrait1080x1920()
plan := vttlayout.Analyze(doc, cv, vttlayout.Options{})

for _, c := range plan.Combos { /* 无解组合 */ }
for _, cand := range plan.Candidates {
    if cand.Resolves && cand.Kind == vttlayout.CandidateSplitCue {
        doc2, _ := vttlayout.ApplyCandidate(doc, cand)
        _ = vttlayout.Serialize(doc2)
    }
}
svg := vttlayout.RenderSVGFrame(plan, 4500, vttlayout.FrameAfter)
```

## 项目结构

- `vtt.go` `parse.go`：类型、时间码、WebVTT 解析与文本解码
- `canvas.go` `geometry.go`：画布 / 安全区 / 字宽 / 折行 / 锚点几何
- `timeline.go`：时间阶段与重叠簇
- `layout.go`：遮挡检查、优先级求解、无解组合扫描
- `repair.go` `repair_time.go`：拆行 / 延长 / 错峰 / 画布 / 字号候选与复验
- `report.go` `write.go`：文本 + JSON 报告、VTT 序列化与候选应用
- `render_svg.go` `frames.go` `termframe.go`：SVG 帧、HTML 对照页、终端帧
- `cmd/vttlayout/`：`check|plan|frames|preview|demo` CLI
- `examples/`：两个可复现样例；根包测试覆盖解析、折行、时间线、
  优先级、认证无解、候选复验、SVG XML 合法性与 VTT 回读

## 验证

```bash
gofmt -w .
go vet ./...
go test ./...
go run ./cmd/vttlayout demo --outdir demo-output
```

SVG 帧是标准 XML（测试用 `encoding/xml` 校验良构），用浏览器打开
`demo-output/*/frames/index.html` 即可看到每个时间点的 before/after 对照。

## 已知简化

- 不实现 WebVTT 的垂直书写、语音样式与完整 CSS；内联标签会被剥离为纯文本。
- 字宽是确定性估算而非 TrueType 实测；留出少量余量即可，需要像素级时可把
  `Meter` 换成基于 `golang.org/x/image` 字体度量的实现。
- 求解器保持水平位置、只做垂直重排；这与字幕 region 通常只做上下堆叠的
  实践一致，也让“高度和越界”成为可证明的无解下界。
