# aache

`aache` 为 AAC-HE / HE-AAC v2 剪辑时间线建立可验证的样本映射。它只使用 Go
标准库，不调用 FFmpeg、GStreamer 或任何媒体框架。

输入是已经由 demux 层拆出的 access unit 元数据：每帧携带的 SBR/PS 头、
可选的解码输出样本观测，以及流的核心采样率。项目不解析或解码 AAC 音频载荷。

问题不是“每帧时长是多少”，而是三个时间层被混在了一起：

| 层 | 含义 | 本项目的字段 |
| --- | --- | --- |
| 核心帧 | AAC 核心帧的固定时长，默认 1024 个核心样本 | `CoreStart`、`CoreSamples`、`CoreRate`、`CoreDuration` |
| 扩展配置 | SBR/PS 头在哪一帧生效，以及之后如何延续 | `Config`、`ConfigSource`、`ConfigEffectiveAt`、`ConfigChanged` |
| 实际输出 | 解码器实际输出的样本数与采样率 | `OutputSamples`、`OutputRate`、`OutputStart`、`PresentationDuration`、`Start`、`End` |

SBR/PS 头只在部分帧出现。`nil` 配置表示“本帧没有更新”，不是“关闭扩展”。
配置会一直生效，直到后续帧显式更新或关闭。HE-AAC 典型情况下：

```text
核心帧时长 = 1024 / 24000 = 42.666... ms
SBR 输出样本 = 1024 * 48000 / 24000 = 2048
输出帧时长 = 2048 / 48000 = 42.666... ms
```

错误做法是拿 `1024 / 输出采样率` 逐帧累加。丢掉 SBR 状态的帧会得到
`1024 / 48000 = 21.333... ms`，于是转场后每遇到一个无头帧就短一半。

## 快速使用

```go
stream := aache.Stream{
	CoreRate: 24000,
	Initial: &aache.ExtensionUpdate{
		SBR: &aache.SBRConfig{Enabled: true, OutputRate: 48000},
		PS:  &aache.PSConfig{Enabled: true},
	},
	Frames: []aache.Frame{
		{Config: &aache.ExtensionUpdate{
			SBR: &aache.SBRConfig{Enabled: true, OutputRate: 48000},
			PS:  &aache.PSConfig{Enabled: true},
		}},
		{}, // 没有 SBR/PS 头，但配置继续生效
		{},
	},
}

timeline, err := aache.BuildTimeline(stream)
```

`BuildTimeline` 使用持久化规则。每一帧的 `OutputSamples` 先由有效扩展状态推导；
如果调用方提供了 `Frame.OutputSamples`，则把它当作解码后的实际样本数。
时间累积使用有理数时钟，不会在 44.1 kHz 等非整数纳秒帧长上逐帧漂移。
`PresentationDuration` 是每帧时长的一次性纳秒取整；`Start`/`End` 是累计位置，
两者在不可整除的采样率下可能相差 1 ns，`Duration()` 返回前者。

## 找出配置丢失的第一处

`Compare` 同时构建两条时间线：

- `Persisted`：正确规则，无头帧沿用最后一次 SBR/PS 配置；
- `FrameLocal`：复现旧剪辑器，无头帧把扩展状态清空。

```go
comparison, err := aache.CompareStream(stream)
if err != nil {
	panic(err)
}
fmt.Println(comparison.FirstLoss.Frame)
fmt.Println(comparison.FirstLoss.PersistedSamples)
fmt.Println(comparison.FirstLoss.FrameLocalSamples)
```

诊断字段有三个层次：

- `Timeline.FirstConfigGap`：第一个“逐帧查找为空、但旧配置仍应生效”的帧；
- `Timeline.FirstOutputMismatch`：第一个解码观测与持久化配置矛盾的帧；
- `Comparison.FirstLoss`：正确时间线与旧逐帧时间线第一次分歧的帧。

显式关闭必须使用非 `nil` 的配置：

```go
Frame{Config: &aache.ExtensionUpdate{
	SBR: &aache.SBRConfig{Enabled: false},
}}
```

关闭 SBR 会同时关闭 PS，因为 PS 不能脱离 SBR 单独存在。

## 多段素材与转场

- 同一个编码流被拆分后再拼接：后一段设置 `Inherit: true`，延续前一段的
  SBR/PS 状态。
- 独立素材拼接：保持 `Inherit: false`，从 `Initial` 提供该素材
  AudioSpecificConfig 中的扩展配置。独立素材的首帧没有带内 SBR 头时，不能
  假定它继承上一段素材的扩展状态。

```go
timeline, err := aache.BuildStreams(firstCut, secondCut)
```

每个 `StreamTiming` 记录源段在全局时间线上的起止帧、起止时间和实际输出样本区间。

## 查询时间线

```go
frame, ok := timeline.FrameAtOutputSample(2048)
at, ok := timeline.TimeAtOutputSample(2048)
frame, ok = timeline.FrameAtTime(at)
```

样本位置按半开区间 `[OutputStart, OutputStart+OutputSamples)` 查找；输出采样率
中途变化时也保持正确。

## CLI 演示

```sh
go run ./cmd/aacheck
go run ./cmd/aacheck -input testdata/transition.json
go run ./cmd/aacheck -input testdata/transition.json -json
```

内置场景包含两个转场素材，SBR/PS 头只出现在部分帧。文本模式会列出每帧：

```text
persisted: 8 frames, 8192 core samples, 16384 output samples, 341.333333ms
first config gap: frame 1 (stream 0 local 1), last header frame 0, carried sbr+ps@48000

frame stream   local   core(ms)    out(ms)       cfg@    start(ms)      end(ms) out/rate
    0      0       0     42.667     42.667 sbr+ps@48000        0.000       42.667     2048/48000
    1      0       1     42.667     42.667 sbr+ps@48000       42.667       85.333     2048/48000
```

对照结果会指出旧逐帧策略首次丢掉配置的帧，以及累计时长误差：

```text
frame-local first loss: frame 1 (stream 0 local 1)
  samples: persisted 2048/48000 Hz, frame-local 1024/48000 Hz
  total delta: 106.666666ms over 7 divergent frames
```

## 边界与错误

- `SBRConfig.OutputRate == 0` 时默认取核心采样率的 2 倍。
- SBR 输出率必须大于核心采样率；核心帧样本数必须为正。
- PS 单独启用会返回 `ErrPSWithoutSBR`。
- 输出率与核心率不能整除核心帧样本数时返回 `ErrUnevenRatio`，不会静默取整。
- `OutputSamples > 0` 是实际输出样本的权威值，同时会与持久化配置推导值比较并
  记录 `OutputMismatch`。

## 测试

```sh
go test ./...
go vet ./...
```

测试覆盖跨无头帧持久化、显式关闭、仅 PS 更新、跨流继承、观测到的配置丢失、
按样本/时间反查，以及 44.1 kHz 长序列无累计漂移。
