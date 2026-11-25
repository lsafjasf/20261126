# ac3select — AC-3 节目/注释轨选择诊断

纯 Go 标准库实现的单二进制工具，用于检查原始 AC-3 elementary stream
（syncword `0x0B77`）中的：

- `bsmod` / bitstream mode：主节目、评论声、视障/听障辅助、对白、紧急广播等；
- `acmod`：`1+1`、`1/0`、`2/0`、`3/0`、`2/1`、`3/1`、`2/2`、`3/2`；
- `lfeon`：低频效果声道；
- block 0 coupling：`cplstre`、`cplinu`、`chincpl`、`phsflginu`、
  `cplbegf`、`cplendf` 和 coupling band structure；
- 连续节目段、段内实际声道顺序、coupling 统计和主/评论轨选择；
- 坏帧/失同步区间旁路，并从下一个合法 syncword 或可信声明帧边界继续。

不调用 `ffmpeg`、`ffprobe` 或任何第三方 Go 包。

## 构建与运行

```sh
go test ./...
go build -trimpath -ldflags='-s -w' -o ac3select ./cmd/ac3select

./ac3select input.ac3
./ac3select -json input.ac3 > report.json
./ac3select -json -frames input.ac3
cat input.ac3 | ./ac3select -json
```

参数：

- `-json`：输出完整 JSON 报告；
- `-frames`：JSON 中保留每帧详情（默认只输出节目段和坏帧）；
- `-strict-crc`：CRC-16/ANSI residual 非零时将该帧作为坏帧旁路；
- `-strict`：存在坏帧时以状态码 `1` 退出，便于 CI 卡控。

默认情况下，即使帧的 CRC 无效，只要位段元数据可解析，仍会保留该帧并在
`header.crc` 中给出校验状态；结构错误才会进入 `badFrames`。

## 选择规则

工具不按文件中的轨道顺序或声道数猜测评论轨，而是优先使用 `bsmod`：

| bsmod | 角色 | 选择含义 |
| --- | --- | --- |
| `0` | Complete Main | 首选主节目 |
| `1` | Music and Effects | 主节目候选 |
| `2` | Visually Impaired | 辅助/注释轨 |
| `3` | Hearing Impaired | 辅助/注释轨 |
| `4` | Dialogue | 对白/注释轨 |
| `5` | Commentary | 明确选择为评论声 |
| `6` | Emergency | 紧急广播 |
| `7` | Voice-over / Karaoke | 单声道按画外音，多声道按 Karaoke 展示 |

多个相同角色的非连续节目段不会合并；同角色候选按帧数更长、索引更靠前做
稳定选择。没有精确 `bsmod=5` 时不会把 VI/HI/D/VO 静默改标成 commentary，
而是列入 `selection.annotationTracks` 并在 `notes` 中说明。

## 报告字段

每个节目段包含偏移、帧数、毫秒时间范围、`bsmodName`、`acmodName`、
`layoutShort`（如 `5.1`、`2.0`）、按 AC-3 `chincpl` 位顺序排列的
`channels`、coupling 汇总和 `track.chosen` 选择结果。

```json
{
  "frameCount": 120,
  "badFrameCount": 1,
  "segments": [
    {
      "index": 0,
      "bsmod": 0,
      "acmod": 7,
      "layoutShort": "5.1",
      "lfeon": true,
      "channelCount": 6,
      "channels": [
        {"label": "L", "inCoupling": true},
        {"label": "C", "inCoupling": true},
        {"label": "R", "inCoupling": true},
        {"label": "Ls", "inCoupling": true},
        {"label": "Rs", "inCoupling": true},
        {"label": "LFE", "kind": "lfe", "inCoupling": false}
      ],
      "track": {"category": "main", "isMain": true, "chosen": true}
    }
  ],
  "badFrames": [{"offset": 1950, "length": 100, "kind": "sample_rate_code"}],
  "selection": {"main": {"segmentIndex": 0}, "commentary": {"segmentIndex": 1}}
}
```

## 错误恢复

扫描器从当前位置寻找 `0x0B77`：

1. 固定头合法且声明帧完整时，解析 BSI 和 block 0 coupling；
2. coupling/CRC 等帧后错误若能信任 `frmsizecod` 得到的帧长，按声明帧边界旁路；
3. 固定头非法或长度不可信时，继续搜索之后的 syncword；
4. 两个合法帧之间的垃圾/失败候选合并为一条 `badFrames` 区间，不中断后续解析。

## 范围

- 面向经典 AC-3（`bsid <= 10`，通常为 `8`）。检测到 E-AC-3（`bsid > 10`）时
  标记为 `unsupported_eac3` 并继续重新同步；E-AC-3 substream 状态图不属于本工具。
- coupling 摘要取自每个 AC-3 帧必须存在的 audio block 0 新策略；这足以识别
  哪些实际全频声道进入 coupling 以及耦合频带。工具不解码 mantissa/PCM。
- 输入应为 AC-3 elementary stream；TS/MP4/WAV 解封装不在这一个二进制的范围内。
