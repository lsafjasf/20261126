# aiffmeta

纯 Go 标准库实现的 AIFF/AIFC 审计、保真重写与元数据丢失对比工具。

## 根因

AIFF/AIFC 是 IFF 容器，`COMM`、`SSND` 和文本 chunk 都是 FORM 下的
平级 chunk。标准没有要求 `COMM` 必须出现在 `SSND` 前面。只按固定顺序
读取或只重写 `COMM`/`SSND` 的转存器会得到两个结果：

- 音频帧仍能照常播放，因此看起来“音频没变”；
- `NAME`、`AUTH`、`(c) `、`ANNO`、`COMT`、`MARK` 等 chunk 被静默丢弃。

本工具遍历完整的 chunk 位移表，不假设 chunk 顺序；解析 `COMM`、`SSND`
和文本元数据；重写时按原顺序逐字节复制每个 chunk 载荷，因此 SSND 的
`offset`、`blockSize` 和音频字节都不经过解码或重新编码。

## 运行

```sh
# 查看 FORM、COMM、SSND、文本元数据和 chunk 位移表
go run ./cmd/aiffmeta inspect input.aifc

# 机器可读输出
go run ./cmd/aiffmeta inspect -json input.aifc

# 保真重写；默认保留所有 chunk
go run ./cmd/aiffmeta repack -out preserved.aifc input.aifc

# 显式丢弃文本 chunk，并输出具体丢掉的注释和标记
go run ./cmd/aiffmeta repack -out no-text.aifc \
  -drop ANNO,COMT,MARK input.aifc

# 对比原始文件和转存文件：音频哈希 + 缺失 chunk + 丢失元数据
go run ./cmd/aiffmeta compare original.aifc resaved.aifc

# 只提取 SSND 中的音频字节（不含 offset/blockSize）
go run ./cmd/aiffmeta extract -out audio.raw input.aifc

go test ./...
```

`repack` 拒绝丢弃 `COMM`、`SSND` 以及 AIFC 必需的 `FVER`。输出文件
通过同目录临时文件加 `rename` 提交；解析失败或拒绝丢弃时不会覆盖已有
文件。`-out -` 将二进制写到标准输出，此时报告改写到标准错误，避免污染
音频流。

## Chunk 位移表

`inspect` 的表格字段均以文件绝对字节位置表示：

| 字段 | 含义 |
| --- | --- |
| `header` | chunk ID 起点，即 chunk 头开始位置 |
| `data` | payload 起点，`header + 8` |
| `size` | chunk 声明的 payload 大小，不含头和偶数字节填充 |
| `stored` | 文件中实际可读的 payload 大小 |
| `data-end` | `data + size` |
| `padded-end` | 含 IFF 偶数填充后的结束位置 |
| `next` | 下一个可读取 chunk 的位置 |

`COMM` 会解析声道数、采样帧数、位深、80 位 IEEE 扩展采样率；AIFC 还会
解析压缩类型和压缩名。`SSND` 会解析其自身的 `offset`、`blockSize`，
并计算真实音频载荷的绝对偏移与长度。文本元数据覆盖：

- `NAME`、`AUTH`、`(c) `、`ANNO`；
- `COMT`（时间戳、关联 marker ID、正文）；
- `MARK`（ID、采样帧位置、Pascal 字符串名称）。

## 丢失信息

`compare` 比较的是 SSND 中真正的音频字节，不比较 SSND offset 参数或
chunk 填充，因此能明确回答“音频是否真的没变”。同时按多重集合比较
文本值，输出：

- `COMM` 的声道、帧数、位深、采样率或压缩类型变化；
- 源文件有、结果文件没有的 chunk；
- 丢失的 name/author/copyright/annotation；
- 丢失的 comment（含时间戳和 marker ID）；
- 丢失的 marker（含 ID、position 和名称）。

`-drop` 重写报告也使用同一套 `LostItem` 结构，便于脚本消费。

## Go 接口

```go
file, err := aiffmeta.ReadFile("input.aifc")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

_, err = file.RepackToFile("output.aifc", aiffmeta.RepackOptions{})
if err != nil {
    log.Fatal(err)
}

resaved, err := aiffmeta.ReadFile("output.aifc")
if err != nil {
    log.Fatal(err)
}
defer resaved.Close()
diff, err := aiffmeta.Compare(file, resaved)
```

主要导出接口：

- `Parse(io.ReaderAt, size)` / `ParseBytes` / `ReadFile`；
- `File.CopyAudio` / `File.AudioDigest`；
- `File.Repack` / `File.RepackToFile`；
- `CompareMetadata` / `Compare`；
- `WriteInspection` / `WriteComparison` / `WriteJSON`。
