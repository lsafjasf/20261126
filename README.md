# audioedit

用声明式项目生成 AAC/Opus 剪辑的统一样本位置表。codec delay（AAC priming、
Opus pre-skip）会进入解码坐标，而不是直接从输出样本 0 开始截取，因此多段素材
连续拼接时不会在每段开头重复丢样本。

## 坐标与推导

位置不是逐段修补出来的，而是每次从项目根完整重算：

1. 源展示坐标 `p` 从编辑声明开始，`p=0` 表示编码延迟之后的第一帧节目内容。
2. 解码坐标 `d` 包含 codec delay：`d = codec_delay + p`。
3. 段首解码位置按编码帧对齐：`decode_start = floor((codec_delay + source_start) / frame) * frame`；源开头从 0 开始解码。
4. 预滚量：`preroll_samples = content_decode_start - decode_start`。
5. 段尾至少解码到完整帧：`frame_end = ceil(content_decode_end / frame) * frame`。
   若片段结束在容器编辑声明的尾部，则使用声明的编码样本尾 `encoded_samples`，
   把编码器尾部填充也计入裁剪。
6. 尾部裁剪：`tail_trim_samples = decode_end - content_decode_end`。
7. 输出长度由源采样率映射到项目采样率；时间轴按片段顺序首尾相接，可选统一间隔。

不变量：`timeline_end - timeline_start == output_samples`，且每一段
`content_decode_start/end`、`preroll_samples`、`tail_trim_samples` 都经过校验。

## 项目声明

```json
{
  "params": { "sample_rate": 48000, "gap_samples": 0 },
  "sources": [
    {
      "id": "intro-aac",
      "format": "aac",
      "sample_rate": 44100,
      "codec_delay": 2112,
      "encoded_samples": 108544,
      "edit": { "media_end": 105840 }
    },
    {
      "id": "music-opus",
      "format": "opus",
      "sample_rate": 48000,
      "encoded_samples": 12480,
      "edit": { "media_end": 11520 }
    }
  ],
  "clips": [
    { "id": "intro", "source_id": "intro-aac", "source_end": 44100 },
    { "id": "music", "source_id": "music-opus", "source_end": 9600 }
  ]
}
```

关键字段：

- `codec_delay`：解码流开头的 priming/pre-skip 样本数。省略时 AAC 默认 1024，
  Opus 默认 312；显式写 `0` 也有效。
- `frame_samples`：编码帧的样本数。省略时 AAC 默认 1024，Opus 默认 960。
- `encoded_samples`：源流可用解码样本总数，含开头 priming 和结尾编码填充。
  也可用 `packets`（乘以帧大小）声明。
- `edit.media_start/media_end`：容器编辑声明暴露的展示区间，终点为开区间。
  AAC/MP4 可由 `elst/stts` 得出，Opus 可由 OpusHead pre-skip 与末页 granule
  得出后填入。
- `clip.source_start/source_end`：展示坐标中的片段区间；`source_end: 0` 表示
  使用 `edit.media_end`。所有值为源采样率。
- `params.sample_rate`：输出时间轴采样率；`gap_samples` 只插在片段之间。

## 统一位置表

每个片段输出一行，主要列：

```text
segment_id source_id format
timeline_start timeline_end output_samples
source_start source_end source_sample_rate
codec_delay content_decode_start content_decode_end
decode_start decode_end preroll_samples tail_trim_samples
```

`timeline_*` 是项目采样率，`source_*` 是源采样率，`decode_*` 是含 codec delay
的源解码样本索引。CLI 输出还包含所有片段的预滚/尾裁总和。

## 用法

```bash
go run . -in examples/basic.json
go run . -in examples/basic.json -out positions.json -compact
go test ./...
```

也可作为库使用。`Editor` 的每次成功修改都会重新编译全部片段；失败修改是事务性的，
不会留下半更新状态：

```go
editor, err := audioedit.NewEditor(project)
if err != nil {
    return err
}

// 改一个参数，所有依赖它的片段及后续时间轴都自动重算。
if err := editor.SetCodecDelay("intro-aac", 1024); err != nil {
    return err
}
if err := editor.SetGapSamples(480); err != nil {
    return err
}

table := editor.Table()
fmt.Println(table.Revision, table.TotalOutputSamples)
```

常用的自动重算入口还有 `SetClipRange`、`SetSourceEdit`、
`SetProjectSampleRate`，或用 `Update(func(*Project) error)` 做批量声明修改。

实现是纯声明式的：不解析实际媒体文件；调用方应把容器中的 edit list、
OpusHead pre-skip、最终 granule 或编码尾填充填入上述字段。
