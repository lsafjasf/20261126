# ADTS 分段解析器

这个仓库提供纯 Go 标准库实现的 ADTS 扫描与分段工具。它不把整段音频当成一个
流，而是在每个有效帧边界检查状态；MPEG 版本、profile、采样率或声道配置变化时
开始新分段。

## 行为

- 用 12 位 syncword 定位帧，并用 13 位 `frame_length` 跳过整个帧。
- 校验 layer、采样率索引、帧长是否覆盖 ADTS 头、没有越过输入末尾，也没有把后
  一个完整帧包进当前帧。
- 同一状态下的连续有效帧合并为一个 `segment`。
- 状态在有效帧边界变化时属于正常切换，只产生新分段，不产生坏帧。
- 无效 syncword、非法头字段或越界帧长会记录到 `badFrames`，随后向后搜索下一
  个完整有效帧。坏区会结束当前分段，避免把不连续的数据合并。
- 每帧样本数为 `1024 * (number_of_raw_data_blocks_in_frame + 1)`。
- `sampleCount` 是分段内样本数，`cumulativeSamples` 是截至该分段末尾的累计
  有效样本数，`totalSamples` 是整个报告的总数。

## 用法

```sh
go run ./cmd/adts-segment input.aac
go run ./cmd/adts-segment -o report.json input.aac
cat input.aac | go run ./cmd/adts-segment -
```

输出为 JSON。输入中的坏帧不会导致命令失败，它们会出现在 `badFrames` 数组里；
只有文件或 I/O 错误会返回非零退出码。

## 库接口

```go
report, err := adts.Read(r)       // io.Reader -> Report
report := adts.ParseBytes(data)   // []byte -> Report
err := adts.WriteJSON(w, report)  // Report -> io.Writer
```

`Report` 包含 `segments`、`badFrames`、有效帧数、坏字节数以及累计样本数。所有
输入输出均使用 `io.Reader`、`io.Writer`、`os.File` 和 `encoding/json`。
