# eac3switch — E-AC-3 解码前节目状态图

纯 Go（仅标准库）实现。针对 E-AC-3 的 independent substream、dependent/associated
substream，以及把两者合并呈现的 mixed program，在真正改动解码器和音频输出之前建立
节目状态图。

任何切换都不会被静默吞掉：每次 `Graph.Switch` 都产生一条带序号的图边，边上有：

- 明确的原因码和旧值 → 新值；
- 帧边界提交策略；
- 一组按顺序执行的受控重建事件；
- 可序列化为 JSON / Mermaid / Markdown 的审计报告。

## 运行

```sh
go test ./...
go run ./cmd/eac3switch -format markdown
go run ./cmd/eac3switch -format mermaid
go run ./cmd/eac3switch -format json -out artifacts/eac3-graph.json
```

内置场景依次覆盖：

1. 首次打开独立 5.1 节目；
2. 切到有关联 substream 的 7.1 节目；
3. 在同一节目的 independent + dependent mixed graph 中切换 DRC；
4. 跨节目切到 44.1 kHz 立体声；
5. 重复选择当前状态，输出 `NO_CHANGE`，不产生重建事件。

## 状态图模型

- `Graph.Programs`：解码前拓扑目录，按节目保存 independent/dependent substream 的
  `strmtyp`、`substreamid`、最近帧头和观测次数。
- `Graph.States`：可切换的不可变节目状态节点。节点包含 anchor、解码引用、渲染引用、
  采样率、声道结构、DRC 配置和帧边界规格。
- `Graph.Transitions`：切换边。每条边保存请求、原因、边界决策和重建事件。

E-AC-3 的 dependent substream 不能脱离匹配的 independent anchor 做有意义的呈现。
因此 `associated` 状态仍在解码图中保留 anchor，只把渲染路由切到关联内容；`mixed`
状态则同时解码/合并 anchor 与 dependent。

## 变化到重建事件

| 解码前变化 | 原因码 | 受控事件 | 爆音防护 |
| --- | --- | --- | --- |
| 首次准备 | `INITIAL_PREPARE` | `session.initialize` + 全套构建事件 | 从第一个有效 syncframe preroll |
| 节目身份变化 | `PROGRAM_CHANGED` | `program_graph.rebuild` | 重建节目/substream 拓扑 |
| independent/associated/mixed 模式变化 | `PROGRAM_MODE_CHANGED` | `program_graph.rebuild`、`mixer.rebuild` | 先在帧尾淡出，再换图 |
| substream 引用或渲染路由变化 | `SUBSTREAM_SELECTION_CHANGED` | `program_graph.rebuild`、`mixer.rebuild` | 校验 anchor，禁止裸切 dependent |
| 采样率变化 | `SAMPLE_RATE_CHANGED` | `decoder.rebuild`、`output.rebuild` | flush 残留 PCM，重建采样时钟 |
| 声道结构/chanmap 变化 | `CHANNEL_STRUCTURE_CHANGED` | `mixer.rebuild`、`output.rebuild` | 重建声道映射和输出格式 |
| DRC 稳定配置变化 | `DRC_CONFIG_CHANGED` | `drc.rebuild`（soft） | 替换曲线并重置增益平滑器 |
| PCM 帧节奏变化 | `FRAME_CADENCE_CHANGED` | `frame.resync`（hard） | 按新 block 数重建样本栅格 |
| `frmsiz` 字节边界变化 | `CODED_FRAME_BOUNDARY_CHANGED` | `frame.resync`（soft） | 用目标帧头重新播种下一帧位置 |
| gap/强制同步 | `RESYNC_REQUIRED` | `frame.resync`（hard） | 丢残留字节，重新寻找 `0x0B77` |

所有真实切换都先产生 `boundary.gate`：完成当前 syncframe，在下一个帧边界提交。
存在硬格式变化时，事务首尾包含 `output.ramp` 短淡出/淡入，避免阶跃爆音。

## DRC 的边界

这里的 `DRCState` 表示稳定 DRC 配置身份：`mode`、`profile`、`dialnorm`、目标参考电平、
显式 `config_id` 或 `metadata_epoch`。配置身份变化才生成 `drc.rebuild`。

逐 audio block 变化的 `dynrng`/增益字属于运行时系数更新；如果每个 block 都重建 DRC
链或音频输出，反而会破坏平滑器并制造爆音。播放器应把这些值送入现有增益平滑路径，
不进入节目状态图重建边。

## Go API

```go
g := eac3.NewGraph()

// 1. 从 0x0B77 syncframe 解析最小固定帧头。
header, err := eac3.ParseHeader(raw)
if err != nil {
    return err
}
if err := g.Observe(header.FrameInfo); err != nil {
    return err
}

// 2. 登记解析后的复合节目状态。复合声道不能只把 acmod 声道数相加：
// dependent 可能增加声道，也可能替换已有声道，因此应由 BSI/chanmap 或
// 节目描述给出最终 ChannelStructure。
anchor := eac3.SubstreamRef{Type: eac3.SubstreamIndependent, ID: 0}
dep := eac3.SubstreamRef{Type: eac3.SubstreamDependent, ID: 0}
_, err = g.Declare(eac3.State{
    ProgramID: "p1",
    Mode:      eac3.ModeMixed,
    Anchor:    &anchor,
    DecodeSubstreams: []eac3.SubstreamRef{anchor, dep},
    RenderSubstreams: []eac3.SubstreamRef{anchor, dep},
    Spec: eac3.ProgramSpec{
        SampleRateHz: 48000,
        Channel:      eac3.ChannelStructure{Channels: 8 /* ... */},
        DRC:          eac3.DRCState{ConfigID: "line-main"},
        Boundary:     eac3.BoundaryState{SamplesPerFrame: 768},
    },
})

// 3. 切换；sw.Reasons 给出原因，sw.Events 是有序受控重建事务。
sw, err := g.Switch(eac3.SwitchRequest{
    ProgramID: "p1",
    Mode:      eac3.ModeMixed,
    Reason:    "用户选择混合节目",
})
```

如果不需要保存图和审计边，可直接调用：

```go
events := eac3.PlanSpecChange(oldSpec, newSpec, "source switch")
initial := eac3.PlanInitialSpec(spec)
```

## 帧头解析范围

`ParseHeader` 解析解码前状态图需要的固定字段：

- `syncword = 0x0B77`
- `strmtyp`
- `substreamid`
- `frmsiz`
- `fscod` / `fscod2`
- `numblkscod` 与每帧样本数（`blocks × 256`）
- `acmod` / `lfeon`

`ScanHeaders` 使用 `frmsiz` 推进，并在坏帧处搜索下一个 syncword。条件 BSI、
`chanmap` 和 DRC 字段数量多且受语法标志控制；本包把它们建模为调用方可填回的
`ChannelStructure.ChannelMap` 与 `DRCState`，避免在没有完整位流语法验证时猜测。

## 推荐执行顺序

1. 收到切换请求后冻结目标选择，但不要立即修改解码器。
2. 在当前 syncframe 完成前继续渲染已排队 PCM。
3. 到达帧边界后执行 `boundary.gate`。
4. 按事件 `Order` 执行 resync、program graph、decoder、mixer、DRC、output。
5. 对硬格式变化 flush 旧 residual PCM，并做短淡入。
6. 首帧 preroll 成功后再恢复正常渲染。

## 验证

```sh
go vet ./...
go test ./...
```
