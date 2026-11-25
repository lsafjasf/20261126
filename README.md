# wubi — WebVTT cue identity editor (pure Go)

Wubi 风格字幕编辑曾把相同的 **cue identifier** 当作唯一键去重，导致同名但
时间或文本不同的合法分段被误删。本库把 cue 的身份拆成三层，只有三层完全
一致才认定为真正的重复项。

## 三层身份

`vtt.Identity` 由以下三层组成（见 `vtt/types.go`）：

1. **cue ID**：标识符行，WebVTT 规范允许重复，也允许为空（匿名 cue）。
2. **时间区间**：`Start`/`End`，统一以整数毫秒保存，避免浮点误差。
3. **文本内容**：cue payload（允许多行）。

两个 cue 仅在 `ID + Start + End + Text` 全部相等时才是重复
（`vtt.IsDuplicate` / `Cue.Identity`）。定位与样式 `Settings` 不参与身份判定，
因为它不改变分段本身。重复 ID 但时间或文本不同的 cue 一律保留，并在
`DedupReport.RepeatedIDs` 中作为“合法重名”报告。

## API

```go
doc, _ := vtt.Parse(data)                 // 支持 BOM、CRLF/CR、STYLE/REGION/NOTE
report := doc.Deduplicate()               // 三层身份去重，保序、保非 cue 块
out, err := doc.Serialize(vtt.WriteOptions{}) // 导出前 Validate，保证规范合法
```

- `Document.Blocks` 保持原始文件顺序，STYLE/REGION/NOTE 块原样保留。
- `Serialize` 可选 `WithBOM`、`CRLF`，时间戳规范化为 `mm:ss.ttt` /
  `hh:mm:ss.ttt`。
- `Validate` 拒绝时间倒序、空文本、含 `-->` 的非法标识符，但从不拒绝重复 ID。

## CLI

```sh
go run ./cmd/wubi dedupe   -in in.vtt -out out.vtt   # 去重并重导出
go run ./cmd/wubi validate -in out.vtt
go run ./cmd/wubi count    -in in.vtt
```

`testdata/sample.vtt` 中三个 ID 为 `1` 的分段，只有时间与文本完全相同的那对
会被合并；另两个同名分段完整保留。

## 标准解析器兼容性

`vtt/write_test.go` 的 `TestExportAcceptedByFFmpeg` 在检测到 `ffmpeg` 时，
会把重新导出的文件交给 ffmpeg 解析，确认输出可被标准解析器接受（含重复 ID
场景）。无 ffmpeg 时该用例自动跳过，其余用例仍覆盖自解析往返。

纯标准库实现，无第三方依赖：`go test ./...`。
