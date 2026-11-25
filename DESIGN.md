# ASS 卡拉 OK 逐字高亮与变速映射

纯 Go 实现，不依赖任何字幕/音频第三方库。

## 偏移的根因

常见错误是把字幕时间戳整体按 `newRate/oldRate` 线性缩放：

```
mediaAt(play) = mediaStart + play * rate   // 错：单一线性函数
```

它有两个问题：

1. 变速点**之前**已经发生的高亮也被平移，历史不再可信。
2. 每次切换都从原点重新缩放，误差随切换次数累积，逐字高亮逐渐“跑偏”。

正确模型是**分段线性（piecewise linear）**：每次变速都在“当前播放位置”插入一个
锚点 `Breakpoint{Play, Media, Rate}`，锚点两侧各自线性，斜率由该段速率决定。
锚点一旦写入就不再移动，因此历史区间恒定，无累积误差。

## 三个时间基

- 播放时间 `Play`：实际墙钟时间，由音频设备已消费的输出样本换算。
- 媒体时间 `Media`：ASS 事件/音节与源音频使用的时间轴。
- 输出样本 `Sample`：声卡实际播放的样本序号，`Play = Sample / SampleRate`。

速率语义：`Rate = 每秒播放时间消费的媒体秒数`（2.0 = 两倍快放，0.5 = 半速）。

分段内（锚点 `bp`）的映射：

```
Media = bp.Media + (Play - bp.Play) * Rate        // 正向：播放 -> 媒体
Play  = bp.Play  + (Media - bp.Media) / Rate       // 反向：媒体 -> 播放
Sample(media) = MediaToPlay(media) * SampleRate    // 媒体 -> 输出样本
Media(sample) = PlayToMedia(sample / SampleRate)   // 输出样本 -> 媒体
```

`Timeline` 同时提供浮点（亚样本精度）与整数样本（四舍五入）两套接口。

## ASS 解析

- `ParseASS` 读取 `[Script Info]`、`[V4+ Styles]`、`[Events]`，按 `Format` 行
  动态映射列，`Text` 作为最后一列保留内部逗号。
- 颜色解析 `&HAABBGGRR`（6 位 `BBGGRR` 默认不透明）。
- 时间 `H:MM:SS.cc` 精确到厘秒。
- 标签分词支持 `\k`、`\kf`、`\ko` 与 `\kt`：
  - `\kf` 音节在持续期内从左到右扫过，渲染端用 `Fill∈[0,1]` 控制裁剪宽度；
  - `\k`/`\ko` 在音节开始瞬间整体变色；
  - `\kt` 把高亮游标重置到行起点后的指定厘秒；
  - 无 `\k` 的引导文本生成零时长占位音节，不参与计时。

## 渲染入口

音频回调里只需要传当前输出样本序号：

```go
tl := asskaraoke.NewTimeline(1.0, 48000)
hl := asskaraoke.NewHighlighter(doc, tl)

// 用户变速：以设备当前样本换算出的播放时间为锚点
tl.SetSpeed(tl.OutputSamplesToPlay(float64(deviceSample)), 2.0)

for _, line := range hl.RenderAtSample(deviceSample) {
    for _, sr := range line.Syls {
        // sr.State: SylPending / SylActive / SylDone
        // sr.Fill: 仅 \kf 需要的扫过比例
    }
}
```

`SampleBoundaries` 给出每个音节边界对应的输出样本点，可改为事件驱动重绘而非轮询。

## 变速、跳转与音频处理

- 变速：`SetSpeed` 在当前播放位置锚定后改斜率，历史区间不变。
- 跳转：`Seek` 丢弃历史分段，从新媒体位置以给定速率重新建立单段映射。
- 变速音频（声码器/重采样）只改变每秒输出的媒体量，样本↔播放时间关系不变，
  因此同一套映射对“保音高变速”和“变调变速”都成立。

## 验证

`go test ./...` 覆盖：常量速率往返、多次变速无历史漂移、样本双向映射、
seek 重置、样式/颜色解析、`\kf`/`\ko`/`\kt` 音节时间、变速下逐字状态。
`go run ./cmd/karaoke-demo` 可观察 1x→2x→0.5x 切换中高亮始终对齐。
