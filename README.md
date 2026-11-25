# wavchannel

纯 Go 标准库实现的 5.1 WAV 声道掩码 / codec 声明 / 样本排列一致性诊断工具。不使用任何音频、解码或探测库。

## 它检测什么

WAV 的多声道顺序不能只看 `nChannels=6`：

- `WAVEFORMATEXTENSIBLE.dwChannelMask` 的置位位声明扬声器位置。
- 交错样本的线序按掩码位从低到高排列。
- 常见 5.1 掩码 `0x3F` 的声明顺序是 `FL, FR, FC, LFE, BL, BR`。
- 侧环绕 5.1 掩码 `0x60F` 的顺序是 `FL, FR, FC, LFE, SL, SR`。
- `fmt` / SubFormat GUID 决定样本应按 PCM integer、IEEE float 还是压缩 codec 解释。
- 样本特征只能在存在已知测试信号、特征规则或参考样本时，可靠地映射到具体扬声器角色。

工具会输出：

- codec 与容器声明；
- 掩码位到 WAV 线序的映射；
- 每条线的 RMS、峰值、过零率、主频、频谱质心和频带能量；
- 所有可行角色排列中的最低成本候选；
- `CENTER_SURROUND_SWAP` 等冲突、证据和置信度；
- 左右环绕不可区分、掩码缺失、codec 不可解码等不可判定项；
- 若映射唯一，给出把当前线序重排回掩码顺序的 source wires。

## 快速开始

```sh
go test ./...

# 生成一个“FC 与 BL 接反”的 5.1/16-bit PCM 样例
go run ./cmd/wavchannelgen -output /tmp/swapped.wav -scenario swapped

# auto profile 会识别内置校准音；普通音乐会保守地输出不可判定身份
go run ./cmd/wavchannelcheck -input /tmp/swapped.wav

# 显式使用内置校准特征
go run ./cmd/wavchannelcheck -input /tmp/swapped.wav -calibration -json

# 或提供一个已知正确的参考 WAV，用对齐样本模板匹配任意特征音
go run ./cmd/wavchannelcheck -input suspect.wav -reference correct.wav -json

# CI：发现中高置信冲突时退出码为 1
go run ./cmd/wavchannelcheck -input /tmp/swapped.wav -check

# 仅在映射唯一且可靠时，重排并写出修正 WAV
go run ./cmd/wavchannelcheck \
  -input testdata/5.1-swapped-center-surround.wav \
  -repair-output /tmp/fixed.wav

# 也支持从 stdin 读取
cat input.wav | go run ./cmd/wavchannelcheck -input - -json
```

仓库内也放了一个可直接检查的合成样例：

```sh
go run ./cmd/wavchannelcheck -input testdata/5.1-swapped-center-surround.wav
```

该样例的关键结果是：

```text
wire 2: declared=FC  observed=BL  status=mismatch
wire 4: declared=BL  observed=FC  status=mismatch
CENTER_SURROUND_SWAP [error/high]
source indices for mask order: [0 1 4 3 2 5]
```

`[0 1 4 3 2 5]` 表示输出掩码顺序中的 `FC` 应取当前 wire 4，`BL` 应取当前 wire 2。

## 自定义多声道特征

内置 `wavchannel-calibration-v1` 只匹配 `cmd/wavchannelgen` 生成的独立测试音：

| 角色 | 主频 |
| --- | ---: |
| FL | 320 Hz |
| FR | 400 Hz |
| FC | 1000 Hz |
| LFE | 70 Hz |
| BL | 520 Hz |
| BR | 640 Hz |

真实项目的特征样本可能使用语音、噪声、脉冲、DTMF 或参考文件。此时应用 JSON 提供角色规则，示例见 `testdata/surround-family-signatures.json`：

```sh
go run ./cmd/wavchannelgen -output /tmp/ambiguous.wav -scenario ambiguous
go run ./cmd/wavchannelcheck \
  -input /tmp/ambiguous.wav \
  -signatures testdata/surround-family-signatures.json \
  -json
```

支持的规则字段包括：

- `frequency_hz` / `frequency_tolerance_hz`：主频契约；
- `min_frequency_hz` / `max_frequency_hz`：频率范围；
- `min_rms`：最低有效电平；
- `min_low_band_ratio`：低频能量占比，用于 LFE；
- `max_zero_crossing_rate`：噪声/高频上限约束。
- `template` / `template_start`：可选的对齐样本片段，用 Pearson 相关性匹配任意特征样本。

如果有已知正确的参考 WAV，`-reference correct.wav` 会信任参考文件的掩码顺序，并自动为每个角色提取样本模板。

程序会对角色到线序做带代价的全排列匹配（当前对至多 8 个角色穷举），输出最佳候选、逐线证据和备选角色。

## 为什么必须保留不可判定项

普通 PCM 样本本身没有通用的“我是中置”标签。若所有声道都是静音、左右环绕使用完全相同的信号、或缺少已知测试规则，任何唯一角色名都是过度推断。

因此工具遵循以下边界：

- 掩码与 `nChannels` 不一致：高置信元数据冲突；
- 特征唯一且候选间隔明显：输出高置信映射和修复顺序；
- 能确定“中置/环绕族接反”，但无法区分 BL/SL 或 BR/SR：输出 `CENTER_SURROUND_SWAP`（medium）和 `CENTER_OR_SURROUND_SIDE_AMBIGUOUS`；
- 没有特征规则且不是内置校准音：保留主频等证据，但输出 `SPEAKER_IDENTITY_NOT_ENCODED`；
- 普通 `fmt` 没有掩码：6 声道时仅给低置信常规 `0x3F` 候选，并输出 `MASK_NOT_DECLARED`；
- G.711、AC-3 等内置读取器未解码的 codec：仍检查元数据，但输出 `CONTENT_DECODE_UNAVAILABLE`。

## 作为 Go 库使用

```go
report, err := channelmap.AnalyzeBytes(wavBytes, channelmap.Options{
    Profile: "auto", // 或传入 Signatures: sig
})
if err != nil {
    return err
}
out, _ := channelmap.RenderJSON(report)
fmt.Println(string(out))
```

主要导出类型为 `Report`、`Conflict`、`UndecidableItem`、`MappingEntry`、`InferredEntry`、`Candidate` 和 `Repair`。

## 验证

```sh
gofmt -w .
go vet ./...
go test ./...
go build ./...
```
