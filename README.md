# Opus 字幕样本对齐

零外部依赖的 Go 标准库实现，用来修正 Opus/Ogg 字幕同步中的样本位置错误。

Opus 的 granule/时间戳是固定 `48 kHz` 坐标系中的**末端位置**，并且开头包含
`OpusHead.pre_skip`。不能把时间戳直接当成包起点，也不能按“第 N 个包 × 固定时长”
一路累加：时间戳跳变可能对应丢包、DTX 或其他隐式 padding。

## 真实区间

对一个带原始末端锚点 `G`（Ogg granule 或等价 raw end）的包，TOC 推导出的时长为
`D`，pre-skip 为 `P`：

```text
playback_end   = G - P
playback_start = playback_end - D = G - P - D
raw_range      = [G-D, G)
playback_range = [G-P-D, G-P)
audible_range  = playback_range 与 [0, +∞) 的交集
```

相邻包之间：

- `next.start > prev.end`：正样本缺口，标记为 `missing_ranges` / `implicit_padding`。
- `next.start == prev.end`：连续。
- `next.start < prev.end`：重叠；若 ID、序列号或区间相同，额外标记 `duplicate`。
- 序列号跳号但没有可信样本锚点时，只标记 `sequence_gaps`，不伪造样本长度。
- TOC 非法、帧数非法、时长不是 `120..5760` 中 `120` 的整数倍，或 TOC 与显式
  `packet duration` 不一致，都进入 `invalid_durations`。
- EOS 末页 granule 小于完整 TOC 时长时按 Ogg end trimming 处理，不计为重复。

## 丢包原则

普通播放链路中 `received: false` 的包会保留真实区间并标记 `missing`，但其播放字幕
文本始终为空。输入中的 `generator_text` 只代表生成端原文，会在报告和播放字幕中被
抑制；不会回填到丢失包，也不会序列化进普通播放报告。

## Go API

```go
package main

import (
    "fmt"
    "opusalign"
)

func main() {
    toc := byte(0x00) // config 0, code 0 => SILK NB 10 ms = 480 samples
    report, err := opusalign.BuildTimeline(opusalign.TimelineInput{
        PreSkip: 120,
        Packets: []opusalign.Packet{
            {TOC: &toc, GranulePosition: opusalign.Int64(480)},
        },
    })
    if err != nil {
        panic(err)
    }
    fmt.Printf("%+v\n", report.Intervals[0].Audible) // {Start:0 End:360}
}
```

TOC 与 OpusHead 可单独解析：

```go
duration, toc, err := opusalign.PacketDurationSamples(rawOpusPacket)
preSkip, err := opusalign.PreSkipFromOpusHead(opusHeadBytes)
```

Ogg Opus 文件可直接分析，页内多个包会从页 granule 按 TOC 时长反向分配端点：

```go
report, head, err := opusalign.AnalyzeOgg(file)
```

## CLI

```sh
go run ./cmd/opusalign -in testdata/timeline.example.json
go run ./cmd/opusalign -in timeline.json -srt-out aligned.srt
go run ./cmd/opusalign -ogg recording.ogg -out report.json
```

JSON 输入字段见 `testdata/timeline.example.json`。样本字段均为 `48 kHz` 整数样本；
`granule_position` 按 Ogg 原始 granule 处理并自动减去 `pre_skip`。如果时间戳已经是
减去 pre-skip 的播放末端，可改用 `playback_end_sample`。

SRT 时间码按 `1 ms = 48 samples` 精确换算。空文本的丢失字幕不会写入 SRT，缺口和
抑制记录保留在 JSON 报告中。

## 验证

```sh
gofmt -w .
go vet ./...
go test ./...
```

测试覆盖 TOC 配置表、code 3 帧数字节、pre-skip 小端解析、隐式 padding、重复序列号/
区间、非法时长、TOC 与 packet duration 冲突、EOS 尾部裁剪、Ogg granule 反向分摊、
SRT 换算，以及“生成端原文不得补丢包”的序列化断言。
