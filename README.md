# 20261126

Repository initialized for 20261126.

## edlrate — 容忍缺失声明的剪辑表帧率推断器

解析 CMX3600 风格剪辑表（EDL），按**卷**、**媒体**（`* FROM CLIP NAME` /
`* SOURCE FILE`）和**时间线**（记录侧）三个维度统计推断帧率，
支持 DF/NDF 与不同卷起始时码，仅依赖 Go 标准库。

```sh
go run . testdata/sample.edl   # 或 go build -o edlrate . && ./edlrate file.edl
go test ./...
```

推断证据：
- 帧号上限：观测到的最大帧号排除标称率过低的候选，并按均匀分布似然打分
- 丢帧证据：非整十分钟边界出现帧号 00/01 排除 DF；帧号从 02 起计支持 DF
- 时长一致性：源出-入与记录出-入帧长是否吻合（区分 DF/NDF 计数）
- 显式声明：`FCM:` 行与 `* FRAME RATE:` 注释优先，但与硬证据冲突时显式报告

置信度低于 70% 时只报告、不自动修正。
