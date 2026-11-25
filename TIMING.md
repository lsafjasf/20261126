# Matroska 音频延迟核对

## 字段语义

- `CodecDelay` 是 Matroska Ticks，单位固定为纳秒；写入时它表示编码器 priming。解封装器用它把非负块时间戳还原为实际帧时间，渲染端再丢弃对应 priming 样本；它不是字幕/音效的编辑延迟。
- `SeekPreRoll` 也是纳秒；它只定义不连续或 seek 后需要提前解码并丢弃的暖机区间。普通播放不扣它，seek 后只把抓取点之前的解码输出标记为 preroll 并丢弃。
- 首包时间戳存“加回 `CodecDelay` 后的原始块时间戳”，因此 Matroska 的 `Cluster/Timestamp + Block relative timestamp` 仍可保持非负；减完 `CodecDelay` 后的实际帧时间可能为负，负区间对应要裁掉的 priming 样本。
- 轨道真实起点、剪辑错位或与字幕/音效的对齐关系必须写入块时间轴，按每条轨道、每个样本位置独立计算，不能做统一毫秒平移。
- AAC 解码库若不自动做 gapless trimming，需要由解封装/渲染层依据 `CodecDelay` 建裁剪窗；Opus 的 pre-skip 通常由解码器内部消费，但容器仍必须保留 `CodecDelay` 来还原时间轴。

## 本仓库的两条轨道

- Opus：48 kHz，960 样本/帧，首个可听样本 2400，`CodecDelay=312` 样本，`SeekPreRoll=3840` 样本。
- AAC：44.1 kHz，1024 样本/帧，首个可听样本 705，`CodecDelay=2112` 样本，`SeekPreRoll=1024` 样本。
- AAC 的 44.1 kHz 时间戳天然不按毫秒对齐，测试保留纳秒取整并同时提供 `math/big.Rat` 的精确样本时间，防止把容器纳秒取整误当样本真值。

## 运行

```sh
go test ./...
go run ./cmd/timingdemo
```

测试会构造两条不同起点的 Opus/AAC 轨道、最小 Matroska/EBML 文档、80 个 `SimpleBlock`，并对重复 seek 的抓取包、priming/preroll 丢弃和首个可见样本逐样本断言。
