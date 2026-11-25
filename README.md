# clipfade — 片段级交叉淡化模型（Go 原生）

两条音频在切点做 200 ms 交叉淡化。旧实现把淡化曲线存成一组绝对时间线
自动化点，移动画面片段（连带移动切点）后没有重建，曲线仍停在旧坐标。

本仓库把淡化改为**片段级派生模型**：淡化只绑定两个片段 ID 与切点关系，
绝对自动化点只是当前几何版本下的派生缓存。任何切点/片段几何变更都会
递增全局版本号、使所有已发出的引用失效、清空旧自动化点并整体重算。

纯 Go 标准库实现，无 CGo、无第三方依赖。

## 运行

```bash
go test -race ./...
go run ./cmd/crossfade-demo
```

48 kHz 下 200 ms = 9600 样本；默认居中锚点，淡化区为
`[cut-4800, cut+4800)`，两侧各需要 4800 样本 handle。

## 片段级模型

- `Clip`：时间线片段。`Start` 是使用入点的时间线采样位置，`InOffset`
  是该位置在原生媒体 `Samples` 中的下标，`VisibleLen` 是使用长度，
  因此切点由几何派生：`cut = left.Start + left.VisibleLen = right.Start`。
  `Group` 相同的片段构成链接组（画面片段与音频一起移动）。
- `ClipFade`：片段级交叉淡化，只保存 `Left/Right ClipID`、时长、锚点、
  曲线律、极性策略等，不保存绝对自动化点。
- `AutomationPoint`：派生的增益断点（默认每轨 17 点），带 `Revision`。
  渲染始终使用采样级增益函数，自动化点只用于 UI / 序列化。
- `FadeReference`：对外持有的不可变引用令牌
  `{FadeID, Revision, CutSample, Fingerprint}`。
- `Model`：拥有片段、淡化和**当前版本**的点表/曲线缓存，`sync.RWMutex`
  保护，可并发使用。

## 失效与重算（核心修复）

`MoveClipBy` / `MoveClipTo`（移动画面片段会移动整个链接组）、
`SetCutPoint`（滚编辑点/改切点）、`SnapCutToZeroCrossing` 都会：

1. 修改片段几何；
2. `Revision++`；
3. **整张点表替换**：丢弃所有淡化的旧自动化点与旧曲线；
4. 对所有淡化重新分析、渲染、生成新版本点集。

之后：

- 旧 `FadeReference` 再 `Resolve` 必然 `errors.Is(err, ErrStaleReference)`，
  调用方必须丢弃旧句柄、重新 `Reference`；
- 存储中不存在任何旧 `Revision` 的自动化点（即使重算失败也先清空，
  绝不保留/继续服务旧曲线，此时 `Resolve` 返回具体的重算错误，
  例如 `ErrClipsNotAdjacent`、`ErrInsufficientHandle`）；
- `Fingerprint` 覆盖淡化参数、两侧片段几何与媒体内容哈希，版本号相同
  但输入被旁路改动时同样判定失效。

`SetCutPoint` 保持右片段时间线音源映射不变（`Start` 与 `InOffset`
同步补偿），相当于在已铺好的音频上滚动编辑边界，因此零交叉吸附看到的
交叉点在改切点前后是同一个音源边缘。

## 同相 / 反相 / 不相关

`AnalyzePhase` 在可配置滞后范围（默认 ±2 ms）内计算归一化互相关
（Pearson ρ），按阈值（默认 0.6）分类，`LawAuto` 据此选曲线：

| 关系 | 判定 | 自动曲线 | 声学依据 |
| --- | --- | --- | --- |
| 同相 `in_phase` | ρ ≥ +0.6 | 等增益线性 `1-x, x` | 相同同相信号相加恒等于原值，中点无 +3 dB 隆起 |
| 不相关 `uncorrelated` | \|ρ\| < 0.6 | 等功率 `cos/sin` | `gOut²+gIn²=1`，不相关信号 RMS 恒定 |
| 反相 `anti_phase` | ρ ≤ −0.6 | 等增益 + 警告 | 任何增益律都无法消除相干抵消；报告凹陷深度，可选 `PolarityCorrectAntiPhase` 翻转右声道后按同相处理 |

## 峰值与零交叉检查

每次重算产出 `FadeReport`：

- `Peak`：采样峰值、峰值位置、超天花板样本数、`Pass`（天花板默认 1.0）。
  例如 0.8 振幅同相素材被强制等功率曲线时，中点峰值约 1.131，检查失败。
- `ZeroCrossings`：混音缓冲内的零交叉边缘列表与数量。
- `EditZeroCrossing`：两侧片段在切点附近最近的零交叉距离、是否正好落在
  切点（`OnCut`）。
- `CancellationDepthDB`：中央窗口相对两端窗口的 RMS 凹陷，反相时可量化
  抵消缺口（精确反相约 −22 dB）。
- `SnapCutToZeroCrossing(fadeID, maxWindowSamples)`：搜索两侧共同的最近
  零交叉边缘并走统一的改切点失效/重算路径；找不到返回
  `ErrNoZeroCrossing`。

## 代码索引

- `clip.go`：`Clip`、坐标映射、媒体哈希、链接组。
- `fade.go`：`ClipFade`、锚点/极性策略、几何校验、`FadePlan`/`FadeReport`、渲染。
- `curve.go`：等增益/等功率增益函数与 `Mix`。
- `phase.go`：互相关、滞后搜索、相位分类。
- `checks.go`：峰值、零交叉、联合零交叉搜索与切点检查。
- `model.go`：`Model`、`Revision`、`FadeReference`、失效与整体重算。
- `cmd/crossfade-demo/main.go`：端到端演示。

## 测试覆盖

`go test -race ./...` 覆盖：200 ms/采样换算与三种锚点几何、三类相位
分类与滞后对齐、等增益重建与等功率平方和、峰值越限、零交叉搜索与吸附、
**移动画面片段后引用失效/点集整体平移且无旧点残留**、改切点使两个淡化
的全部引用失效、重算失败时旧点同样被清空，以及并发解析/移动。
