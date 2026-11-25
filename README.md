# dsfmeta

DSF (DSD Stream File) 元数据工具：解析 DSD/fmt/data chunk，按真实 DSD
采样率（DSD64=2822400 Hz，DSD128=5644800 Hz）建立时间轴，把标签与章节
映射到正确时间，修复编辑工具按 PCM 采样率累计导致的时间轴翻倍问题。

纯 Go 标准库，无任何第三方依赖。

## 子命令

- `dsfmeta info <file.dsf>` — 打印 chunk 结构、块布局（含尾部不足块）、
  头部声明时长、按载荷推算的真实时长，以及 PCM 误读的拉伸倍数。
- `dsfmeta map [--segs N@rate,M@rate] [--chap 采样序号=标题]... <file.dsf>`
  — 打印分段时间轴与章节的真实起止时间。拼接文件用 `--segs` 描述每段
  的采样数与速率，例如 `98304@2822400,66336@5644800`。
- `dsfmeta tag [--segs ...] [--title/--artist/--album/--track/--genre/--date]`
  `[--chap 采样序号=标题]... <file.dsf>` — 写入 ID3v2.4 标签（含 CHAP/CTOC
  章节帧），章节时间由分段时间轴换算成毫秒。

## 载荷保护

`tag` 只改写两处：DSD chunk 头部的记账字段（文件总大小、元数据指针，
偏移 12..28）和文件尾部的 ID3 区域。fmt chunk、data chunk 头与全部 DSD
载荷逐字节复制，写入前后做 SHA-256 校验，不一致则拒绝替换原文件。
临时文件 + rename 原子提交，并保留原文件权限。

## 尾部不足块

载荷按 `块大小 × 声道数` 为一组切分；末尾不足一组时按声道均分计入
实际采样数（`ActualSampleCount`），不能被声道数整除的余字节作为
stray 单独报告，不计入时长。

## 测试

`go test ./...` 覆盖：chunk 解析、部分块布局、DSD64+DSD128 混合速率
时间轴往返映射、PCM 误读倍数、标签写入后载荷哈希不变与头部字段修正。
