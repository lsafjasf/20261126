# mp3timeline

纯 Go 标准库实现的 MP3 时间轴分析器。它同时读取 Xing/Info/VBRI 声明和逐帧扫描结果，处理 LAME/Lavf 的首帧 encoder delay 与尾部 padding，并给出播放器可反复 seek 而不漂移的统一时间映射。

## 两种时间源

- **声明时间源**：`Xing.FrameCount * SamplesPerFrame / SampleRate`。它可能把 metadata 帧算进帧数，也不能直接描述 gapless 内容。
- **扫描时间源**：从 `audio_start` 开始逐帧验证 MPEG 头并累计 `SamplesPerFrame`。Xing/Info 所在的 metadata 帧不计入音频帧。VBR 不影响每帧样本数，因此这是线性、无累计误差的样本时钟。

默认规则：

1. 帧扫描完整时，`stable_mapping.authority = "frame_scan"`。
2. 完整扫描下，若 `Xing.FrameCount == 扫描帧数 + 1`，判定声明把 metadata 帧算进去了，并报告 `counts_metadata_frame`；映射仍使用扫描帧数。
3. 文件尾部截断、扫描不完整时，时长回退到 Xing 声明值，`authority = "xing_declared"`；已有帧的 seek 偏移仍来自实际帧表。
4. 任何情况下都不把平均码率、Xing 时长和逐帧时间混用。

`-json` 输出中的 `comparison.frame_delta`、`comparison.raw_sample_delta`、`comparison.duration_delta_us` 和 `comparison.relation` 是两种时间源的直接差异。

## 首帧延迟与尾部 padding

若 Xing/Info 后存在 LAME、Lavc、Lavf 等扩展，会解析 12-bit encoder delay 和 12-bit tail padding：

```text
raw_samples          = audio_frame_count * samples_per_frame
presentation_samples = raw_samples - encoder_delay - tail_padding
```

`presentation_samples` 是唯一的展示时长。delay 和 padding 都只扣一次，不能在一次 seek 中扣、下一次又不扣。

## 稳定映射

时间单位统一为整数样本，显示时只在最后做一次整数换算：

```text
time_us(frame) = clamp(frame * samples_per_frame - encoder_delay,
                        0, presentation_samples) * 1_000_000 / sample_rate

frame(time_us) = clamp(floor((encoder_delay + round(time_us * sample_rate / 1_000_000))
                             / samples_per_frame),
                       0, last_frame)
```

反向换算按最近采样点取整，使实际有展示时间的帧满足
`FrameAt(TimeAtFrame(i)) == i`，避免纳秒/微秒截断导致的回退一帧。时间到达或超过 `duration_us` 时固定映射到最后一帧并标记 `at_end`。若 encoder delay 跨越多个帧，`first_frame` 是第一个包含展示样本的帧，前面的帧仅作为解码预热区。

VBR 的字节位置由扫描得到的 `frames[].offset` 决定，不能用 `总字节 / 总时长` 或 Xing TOC 反推平均位置。TOC 只作为提示保留，不作为规范时间轴。

播放器接入方式：

```go
tl, err := mp3timeline.AnalyzeFile("song.mp3")
if err != nil {
    return err
}

p, ok := tl.SeekPointAt(1_234_000 * time.Microsecond)
if ok {
    // p.Offset、p.Size 用于 seek；显示时间使用 p.FrameTimeUS 或同一套
    // TimeAtFrame/FrameAt，不要回到 Xing 声明时长。
}
```

## 命令行

```bash
go run ./cmd/mp3map song.mp3
go run ./cmd/mp3map -json song.mp3
go run ./cmd/mp3map -json -frames song.mp3
go run ./cmd/mp3map -at 1.25s song.mp3
```

- 默认输出两种时间源的对照、gapless 信息和映射公式。
- `-json` 输出完整机器可读报告。
- `-frames` 附带每一帧的 `offset/size/bitrate/presentation_start_us`，供 VBR seek 表使用。
- `-at` 额外打印一个时间点的帧号和字节偏移。

## 测试

```bash
go test ./...
go vet ./...
```

测试使用合成的 MPEG-1/2、Xing/Info、VBRI、LAME 帧，覆盖声明与扫描一致、Xing 多算 metadata 帧、声明/扫描不一致、尾部截断回退、无 Xing、seek 往返等场景。

## 边界

- 支持 MPEG-1、2、2.5 的 Layer I/II/III 时间计算；Xing/VBRI 主要按 Layer III 使用。
- 不支持 free-format bitrate。
- 假定单条音轨内 version/layer/sample rate 不变；格式变化时扫描停止并给出截断标记。
- 只处理 ID3v2 前置标签和 ID3v1 尾部标签，不解析 APE 尾部标签。
- 为修复 MP3 bit reservoir，解码器通常应从目标帧前 1~3 帧开始预热；这不应改变显示时间，显示必须继续使用同一映射。

依赖仅限 Go 标准库。
