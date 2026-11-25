# 20261126

Repository initialized for 20261126.

## 时间线约束求解

`timelineconstraint` 用 `math/big.Rat` 联合求解四个输入量：

- `source range`：源时间码的入点、出点，帧号由 `timecode rate` 解释。
- `record range`：时间线入点、出点，帧号由时间线录制率解释。
- `speed`：源媒体时长相对时间线时长的倍数；`2x` 表示 10 秒源媒体占用 5 秒时间线。
- `timecode rate`：源时间码标签速率，例如 `24000/1001` 或 `30000/1001 DF`。

记：

- `Sf` 为 source range 的帧数。
- `Rf` 为 record range 的帧数。
- `Rr` 为 record range 的实际帧率。
- `v` 为 `speed`。
- `Rt` 为 source timecode rate。

则：

```text
record duration   Tl = Rf / Rr
source media time Tm = v * Tl
actual source rate Ra = Sf / Tm
                   Ra = Sf * Rr / (Rf * v)
```

如果软件同时要求 `Ra = Rt`，就必须满足：

```text
Sf * Rr = Rf * Rt * v
```

该等式不成立时，源时间码速率和实际录制帧率无法同时满足。`Solve` 返回诊断结果；`SolveStrict` 直接返回 `ConflictError`。

音频定位使用实际媒体时间，而不是时间码标签时间：

```text
timecode time = source in frame / Rt
media time    = source in frame / Ra
audio offset  = media time - timecode time
```

因此只按时间码标签放置音频会产生整段起点偏移；片段时长也会随帧号持续漂移。

## 支持的时间码

- NDF：按标称帧率线性编号。
- DF：仅允许 30/60 fps 标称族；跳帧标签会被拒绝。
- 29.97 DF 使用 `30000/1001`，跳帧规则仍按 30 fps 标称帧号处理。

所有计算使用有理数，23.976、29.97 DF 和变速比例不会产生浮点累计误差。

## 用法

```go
rate := timelineconstraint.NewRate(24000, 1001, false)
result, err := timelineconstraint.Solve(timelineconstraint.Constraint{
    TimecodeRate: rate,
    Source: timelineconstraint.Range{
        In:  timelineconstraint.Timecode{Hour: 0, Second: 0, Frame: 0},
        Out: timelineconstraint.Timecode{Hour: 0, Second: 10, Frame: 0},
    },
    Record: timelineconstraint.RatedRange{
        Rate: rate,
        In:   timelineconstraint.Timecode{Hour: 1, Frame: 0},
        Out:  timelineconstraint.Timecode{Hour: 1, Second: 10, Frame: 0},
    },
    Speed: big.NewRat(1, 1),
})
```

## 测试

```sh
go test ./...
go vet ./...
```

覆盖：

- 23.976 NDF。
- 29.97 DF，包括跳帧标签拒绝。
- `0.5x`、`2x` 变速素材。
- timecode rate 与 actual recording rate 不可同时满足的冲突点和严格模式。
