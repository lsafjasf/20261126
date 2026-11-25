# wavfix — G.711/WAV fact 与容器长度修复

这个模块只用 Go 标准库解析 RIFF/WAVE 的 chunk 偏移表，核对 `fmt `、`fact`、
`data` 三者的真实关系，并生成**只改容器元数据、不修改音频字节**的新 WAV。

## 为什么会翻倍

设：

- `C = fmt.nChannels`
- `R = fmt.nSamplesPerSec`，单位是每声道帧/秒
- `B = fmt.nBlockAlign`，即一个交错采样帧的字节数
- `D = data.ckSize` / 实际 `data` 载荷字节数
- `N = fact.dwSampleLength`

对固定帧长的 G.711 A-law（format tag `6`）或 μ-law（tag `7`）：

```text
B = C × (wBitsPerSample / 8) = C
D = N × B
N = D / B
时长 T = N / R = D / (B × R)
```

`fact.dwSampleLength` 的单位是**每声道采样帧**，不是 `data` 字节数，也不是
`C × N` 个交错声道样本。把 `D` 直接当成 `N` 时：

- 单声道、8-bit G.711：`B=1`，错误恰好被掩盖；
- 立体声 G.711：`B=2`，得到的帧数和时长正好翻倍；
- 解码成 float 后，缓冲里有 `D` 个交错 float；正确时长仍是
  `len(floats) / C / R`，而不是 `len(floats) / R`。

G.711 每个在线字节解码成一个 float/线性样本；解码后的 16-bit 内部量程不改变
时间轴帧数，不能用 16-bit 宽度重新除一遍 `data`。

## 修复规则

- 解析并保留 RIFF chunk 顺序和偏移信息。
- `fact` 缺失时，在 `fmt ` 后插入标准 12 字节 `fact`。
- `fact` 计数或 chunk 长度错误时，将其规范化为 `D / nBlockAlign`。
- 重新计算外层 RIFF size。
- `data` 载荷逐字节复制，测试用 SHA-256 和 `bytes.Equal` 双重确认。
- `fmt `、`LIST`、`JUNK`、厂商自定义 chunk 等未知/非目标 chunk 原样保留。
- `data` 不是整帧或缺少 `fmt/data` 时返回明确错误，不静默猜测。
- chunk 声明长度大于实际文件时，默认按实际可用载荷修正长度并保留全部剩余字节；
  也可用 `Options.StrictTruncation` 或 CLI `--strict-truncation` 改为拒绝。

## CLI

```sh
go run ./cmd/wavfix inspect testdata/g711-stereo-bad-fact.wav
go run ./cmd/wavfix check   testdata/g711-stereo-bad-fact.wav
go run ./cmd/wavfix fix -o /tmp/fixed.wav testdata/g711-stereo-bad-fact.wav
go run ./cmd/wavfix inspect -json testdata/g711-mono-missing-fact.wav
```

`check` 在发现缺 `fact`、计数不一致、RIFF 长度不一致等问题时以非零状态退出；
`fix` 会写出新文件，默认接受 PCM、IEEE float、A-law 和 μ-law 的固定帧长关系。

## Go API

```go
raw, _ := os.ReadFile("input.wav")

file, err := wavfix.Parse(raw)
report := file.BuildReport()
fmt.Println(report.DataFrames, report.DurationFromData, report.Issues)

fixed, report, err := wavfix.Repair(raw)
_ = os.WriteFile("fixed.wav", fixed, 0o644)
```

文件级入口为 `wavfix.RepairFile(input, output, wavfix.Options{})`，输出使用同目录
临时文件加 rename，避免直接写坏目标路径。

## 样例与测试覆盖

`testdata/gen/main.go` 可重复生成以下小样例：

- `g711-stereo-bad-fact.wav`：立体声 μ-law，2000 data 字节 / 2 = 1000 帧；
  错误 `fact=2000`，错误时长是真实值的 2 倍；含 `xUNK` 和 `META` chunk。
- `g711-mono-missing-fact.wav`：单声道 μ-law，缺少 `fact`，含 `LIST` chunk。
- `g711-mono-alaw-correct.wav`：正确 A-law 文件，修复结果必须与原文件字节相同。
- `g711-stereo-bad-riff-size.wav`：RIFF size 错误但可按 EOF 解析，尾部未知 chunk
  不能丢失。
- `g711-stereo-overlong-data.wav`：`data` 声明长度比实际载荷大；修复时把 chunk 头
  改回实际可用的 1000 字节，不补造音频样本。

重新生成样例：

```sh
go run ./testdata/gen
```

生成器会同时刷新 `testdata/expected/*.fixed.wav`，这些 golden 文件由
`golden_test.go` 逐字节校验。

运行全部测试：

```sh
go fmt ./...
go vet ./...
go test ./...
```

测试还覆盖短 `fact` chunk、截断 `data`、非整帧立体声数据、缺少 `fmt`、文件原子
写入、过长 data 长度修正以及 G.711 解码后的 float 时长关系。G.711 解码辅助函数只用于分析和测试；
修复路径不调用解码器，因此不会改变任何音频样本。
