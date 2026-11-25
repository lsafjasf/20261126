# cafmarker

纯 Go 标准库实现的 CAF（Core Audio Format）marker / packet table / audio
description 诊断工具。它把 marker 的 `mFramePosition` 放到由 `desc` 与 `pakt`
共同建立的 packet、sample-frame、byte 坐标中，定位 instrumentation marker
比真实播放位置晚一个 packet 的 off-by-one 来源。

## 误差来源

CAF marker 的 `mFramePosition` 是从第 0 个 audio frame 开始的零基帧号；packet
也是零基。packet `N` 的右边界同时是 packet `N+1` 的左边界：

```text
FrameStart[N+1] = FrameEnd[N] = FrameStart[N] + FrameCount[N]
```

instrumentation 常见错误是在写完 packet 后才记录 marker，却把“写后计数/下一
个游标”当成当前 packet：

```go
// 错误：writePacket 返回后，count 已经是 N+1。
n := writePacket(p)
writeCAFMarker(frameStartForPacket(n)) // 实际写到了 FrameStart[N+1]

// 正确：记录刚写完的零基 packet N，或在推进游标前记录起点。
writeCAFMarker(Packets[N].FrameStart)
```

因此现象会是：真实位置在 packet `N`，marker 却映射到 packet `N+1`；在变长
packet 下，这个差值不能用固定的 `packetIndex * FramesPerPacket` 或 packet
字节长度计算，必须累加 packet table 中每项的帧数。

同一个帧坐标既是 packet `N` 的排他末尾，也是 packet `N+1` 的起点。仅凭 CAF
文件无法判断“marker 数值错了”还是“读取方把边界归给了上一个 packet”；工具会
输出两种解释。若提供 marker 名称中的 packet 引用、数字 marker ID 规则或外部
真实位置，工具可以给出高置信度归因和修正帧号。

## 解析内容

- CAF 文件头与 chunk 头：big-endian FourCC + `int64` chunk size。
- `desc` / `CAFAudioFormat`：sample rate、format ID、format flags、固定
  bytes/frames-per-packet、声道数和位深。
- `pakt` / `CAFPacketTableHeader`：packet 数、valid frames、priming frames、
  remainder frames，以及 CAF base-128 VLQ packet size/frame-count 表。
- `mark` / `CAFMarkerChunk`：SMPTE type、marker count 和每个 28 字节
  `CAFMarker`。
- `strg`：把 marker ID 解析成 instrumentation 标签。
- `data`：跳过 4 字节 edit count，确定音频样本的绝对文件偏移；支持
  `mChunkSize = -1`（chunk 到 EOF）。

`pakt` 表项按 `desc` 决定：

| `BytesPerPacket` | `FramesPerPacket` | `pakt` 表项 |
|---:|---:|---|
| 0 | 非 0 | 每个 packet 一个 VLQ：byte size |
| 非 0 | 0 | 每个 packet 一个 VLQ：frame count |
| 0 | 0 | 每个 packet 两个 VLQ：`(byte size, frame count)` |
| 非 0 | 非 0 | 无逐项表，按定长布局算术映射 |

## 样本位置映射

对第 `i` 个 packet：

```text
FrameStart[i] = sum(FrameCount[0:i])
FrameEnd[i]   = FrameStart[i] + FrameCount[i]
ByteOffset[i] = sum(ByteSize[0:i])
FileOffset    = data.AudioOffset + ByteOffset[i] + ByteOffsetInPacket
```

- 对 PCM 等未压缩格式，`ByteOffsetInPacket = FrameOffset * BytesPerFrame`，
  可以继续映射到具体样本字节。
- 对压缩格式，packet 内部样本位置依赖编解码器，工具只保证 packet 边界和
  packet 内 frame offset，不虚构字节偏移。
- 定长 packet 使用除法/取模做紧凑算术映射；长 PCM 文件不会展开成数百万个
  `Packet` 结构。少量 packet 会同时放在 JSON 的 `packets` 数组中。

播放坐标还会应用 `pakt` 的前后填充：

```text
CodedFrameCount - PrimingFrames - RemainderFrames = NumberValidFrames
AudibleFrame = CodedFrame - PrimingFrames
```

落在 priming / remainder 区间的 marker 会被标记，但不会从编码时间轴中删除，
因为 CAF marker 的 `mFramePosition` 仍按文件帧号解析。

## 尾部不足

工具分别报告三类尾部问题：

- `short_byte_tail` / `packet_data_short`：packet table 声明的字节数超过
  `data` chunk 实际音频字节；最后一个 packet 给出
  `available_bytes` 和 `missing_bytes`。
- `truncated_chunk`：chunk header 声明长度超过文件实际长度。
- `packet_table_short`：`pakt.mNumberPackets` 大于可解码的 VLQ 表项数。
- `remainder_frame`：marker 落在最后一个 packet 的 remainder 填充中；这是
  可播放尾部被裁剪，不等于文件字节截断。

## CLI

```sh
go test ./...

# 解析并给出边界/尾部报告
go run ./cmd/cafmarker inspect testdata/instrumentation-shifted.caf

# 根据 marker 名称中的 packet=N 自动归因
go run ./cmd/cafmarker diagnose testdata/instrumentation-shifted.caf

# 对没有 packet 标签的 marker，提供已知真实 packet/frame
go run ./cmd/cafmarker diagnose path/to/file.caf \
  -marker-index 0 -actual-packet 1
go run ./cmd/cafmarker diagnose path/to/file.caf \
  -marker-index 0 -actual-frame 1024

# marker ID 本身就是零基 packet 序号时
go run ./cmd/cafmarker diagnose path/to/file.caf -id-is-packet

# JSON 输出；flag 可以放在路径前后
go run ./cmd/cafmarker diagnose path/to/file.caf -json
```

期望文件格式：

```json
{
  "expectations": [
    {"marker_index": 0, "actual_packet": 0},
    {"marker_id": 17, "actual_frame": 2048}
  ]
}
```

然后使用 `-expect expectations.json`。

## Go API

```go
f, err := cafmarker.ParseFile("input.caf")
m, err := f.BuildMap()
loc := m.LocateFrame(int64(marker.FramePosition), f)
report := cafmarker.Diagnose(f, cafmarker.Options{
    Expectations: []cafmarker.Expectation{{
        MarkerIndex:  intPtr(0),
        ActualPacket: intPtr(2),
    }},
})
_ = report.RootCause
```

也可以使用 `cafmarker.ParseBytes` / `cafmarker.DiagnoseBytes` 处理内存夹具。

## 测试夹具

`testdata/gen/main.go` 仅用标准库生成确定性 CAF：

- `instrumentation-shifted.caf`：名称引用 packet `0`/`2`，marker 却位于
  packet `1`/`3` 的起点，复现写后游标 off-by-one。
- `instrumentation-healthy.caf`：相同 packet table，marker 位于正确起点。
- `short-tail.caf`：最后一个变长 packet 声明 70 字节但 data 尾部只给 45
  字节。

重新生成：

```sh
go run ./testdata/gen
go test ./...
```

## 范围

工具不解码压缩音频，也不根据 PCM 波形寻找实际提示音；“真实播放位置”来自
格式声明、packet table、priming/remainder 信息以及可选的外部期望值。它负责
证明 marker 坐标与 CAF packet 映射之间的精确偏差和算术来源。

字段语义参考 Apple Core Audio Format Specification：
<https://developer.apple.com/library/archive/documentation/MusicAudio/Reference/CAFSpec/CAF_spec/CAF_spec.html>。
