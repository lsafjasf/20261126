# timemap：复合片段父子时间映射

解决 Compound Clip 展开后「内部变速 × 外部变速」叠加导致的标记漂移问题。
全程只使用 Go 标准库（`math/big`、`encoding/json`、`encoding/csv` 等）。

## 漂移根因

速率是有理数，帧映射是向下取整，而取整不满足结合律：

```
floor(floor(t · 2/3) · 3/2)  ≠  floor(t · (2/3 · 3/2))
```

朴素做法把内外速率先连乘成一个数、再做一次 floor，边界帧会出现 off-by-N。
本项目禁止预先连乘，改为保留每一层 `Stage`，逐级 floor 重放父子链。

## 设计

- `timeline/speed.go`：`Speed` 有理数速率（约分、乘积、倒数），`Speed.Map` 做整数 floor 映射，无浮点。
- `timeline/model.go`：`Project / Compound / Item / Marker / Source / FrameRecord`。
- `timeline/resolver.go`：逐帧沿父子链解析，分别记录内部速率、外部速率（父层累积乘积）、合并速率与源帧。
- `timeline/flatten.go`：逐帧驱动展开，按连续同路径帧聚合为根坐标叶子；每层引用保留独立 `Stage`，展开后查询同样逐级 floor。
- `timeline/naive.go`：朴素连乘展开，仅用于复现/对照漂移（`CompareMarkers`）。
- `timeline/cycle.go`：循环引用单独检测，返回环上完整 ID 链（`A -> B -> A`），构建解析器前直接拒绝。
- 冻结段独立分支：`Freeze=true` 的叶子源帧恒定、内部速率记为 `1`，不参与任何速率乘积。
- `timeline/verify.go` / `markers.go`：逐帧比对展开前后画面索引、速率；标记做根帧集合包含校验。
- `timeline/export.go`：导出 JSON 与 CSV，CSV 含 `internalSpeed,externalSpeed,combinedSpeed,sourceFrame` 列。

## 运行

```bash
go test ./...
go run ./cmd/timemap                 # 打印映射、漂移对照、循环检测
go run ./cmd/timemap -out ./out      # 额外导出 out/frames.csv 与 out/frames.json
```

演示工程包含内层 `3/2` 变速、外层 `2/3` 变速、根级冻结段与多层标记；
`CyclicProject` 演示 `A -> B -> A` 循环被独立拦截。
