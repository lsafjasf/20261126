# sdhlayers — SDH 字幕语义拆层与重叠处理（Go）

SDH（听障辅助字幕）在同一条字幕里混合了**说话人标签**、**环境声/音效描述**和
**普通对白**，还常夹带**音乐歌词**与**画外音（V.O./O.S./旁白）**。现有编辑器把
这些文字全部塞进同一行/同一轨，导致无法单独开关 SDH 元素，时间重叠时也会互相遮挡。

本仓库用纯 Go 标准库实现：

- 把每条字幕文本按**语义角色**切分成 `Segment`，再构建独立**层（Layer/Track）**；
- 对时间重叠的条目求重叠组、分配不碰撞的**视觉车道（Lane）**，并输出带分层样式与
  垂直边距的 **ASS**；
- 提供“关闭 SDH”的普通字幕投影，并给出可校验的**完整性报告**：所有对白/画外音/歌词
  必须原样保留、时间码不变。

模块名 `sdh`，根包 `package sdh`，无第三方依赖，`go 1.22+`。

## 语义角色

| 角色 `Role` | 含义 | 典型写法 | 关闭 SDH 后 |
| --- | --- | --- | --- |
| `dialogue` | 普通对白 | `你听到了吗？` / `Hello.` | 保留 |
| `voiceover` | 画外音/旁白/场外音 | `JOHN (V.O.): …`、`（画外音）…`、`旁白：…`、`(O.S.)` | 保留 |
| `lyric` | 音乐歌词 | `♪ … ♪`、`♫ … ♫` | 默认保留（可配置丢弃） |
| `speaker` | 说话人标签 | `JOHN:`、`张三：`、`[John]`、`（张三）` | 移除（可配置保留） |
| `sound` | 环境声/动作描述 | `[雷声轰鸣]`、`(door creaks)`、`【敲门声】`、`*laughs*` | 移除 |
| `music` | 音乐（描述，非歌词） | `[upbeat jazz music]`、`（背景音乐）` | 移除 |

分类器是确定性的规则实现（中英文约定、括号族 `[] () （） 【】`、冒号标签、音符、
V.O./O.S. 修饰语、旁白专有说话人等），并会剥离 `<i>`/`<font>`/ASS override 等样式标记。

## 快速开始

```sh
go test ./...

# 生成普通字幕、分层 ASS、逐层 SRT 与 JSON 报告
go run ./cmd/sdhtool testdata/sdh_mixed.srt -out /tmp/sdh

# 位置参数与 flag 可混用；也支持 stdin
go run ./cmd/sdhtool testdata/sdh_mixed.srt -print-plain
cat testdata/sdh_mixed.srt | go run ./cmd/sdhtool -in - -print-report
```

常用 flag：`-out`（产物目录，传空串关闭文件输出）、`-base`、`-print-plain`、
`-print-ass`、`-print-report`、`-keep-labels`、`-exclude-lyrics`、`-strip-notes`、
`-max-lanes`。完整性校验失败时 CLI 返回码 `3`。

`examples/` 内有一份已生成的完整产物，可直接查看：

- `examples/sdh_mixed.plain.srt`：关闭 SDH 后的普通字幕（10 条源字幕 → 7 条，
  三条纯描述字幕正确消失，9 个发声单元全部保留）；
- `examples/sdh_mixed.sdh.ass`：六层样式 + 重叠车道；
- `examples/sdh_mixed.<role>.srt`：每个语义层单独的 SRT；
- `examples/sdh_mixed.layers.json`：分层计数、重叠组、车道数与完整性报告。

## 库用法

```go
package main

import (
    "fmt"
    "os"
    "sdh"
)

func main() {
    data, _ := os.ReadFile("in.srt")

    // 一站式：解析 -> 拆层 -> 重叠布局 -> SDH-off -> 完整性报告
    res, err := sdh.ProcessSRT(data, sdh.DefaultOptions())
    if err != nil {
        panic(err)
    }

    _ = res.Project      // *sdh.Project：Layers / Entries / Layout
    _ = res.Plain        // []sdh.Cue：关闭 SDH 后的普通字幕
    _ = res.Verification // *sdh.CompletenessReport

    fmt.Println(string(sdh.RenderSRT(res.Plain)))
    fmt.Println(string(sdh.RenderASS(res.Project, sdh.DefaultASSOptions())))
}
```

也可以分步调用：

```go
cues, _ := sdh.ParseSRTBytes(data)      // 或 sdh.ParseSRT(reader)
project := sdh.Split(cues)              // 分类 + 建层 + ResolveOverlaps
plain := project.Plain(sdh.DefaultPlainOptions())
report := sdh.VerifyCompleteness(cues, plain)
```

## 重叠处理

- `ResolveOverlaps` 用扫描线求时间区间的连通重叠组（`Layout.Groups`）；端点相接
  （`end == start`）不算重叠，可复用车道。
- 车道按锚点带分别贪心分配：对白/画外音/歌词/说话人在底部向上叠，环境声/音乐在顶部
  向下叠。同一 cue 内同角色同说话人的多行换行文本会合并为一个多行块，不会被误叠；
  不同说话人（如 `- MARY:` / `- JOHN:`）保持独立并分到不同车道。
- ASS 渲染器为每个 `(角色, 车道)` 生成带 `Alignment` 与递增 `MarginV` 的样式，按估算
  换行行数累计高度，保证同时显示的条目在垂直方向不碰撞。`-max-lanes` 只产生
  `Layout.Overflow` 告警，不会丢弃任何内容。

## “关闭 SDH 后仍完整”的保证

`PlainWithOptions` 逐行移除说话人标签与括号描述，保留 `dialogue`/`voiceover`/`lyric`
（歌词默认保留，可用 `ExcludeLyrics` 关闭），时间码与 ID 不变，并重新顺序编号。

`VerifyCompleteness` 会重新分类源字幕，对每个发声片段做归一化文本覆盖校验（忽略大小写、
标点、空白、音符与换行，CJK 按字符保留），优先按 cue ID 匹配、缺失时退化为时间窗口匹配，
并核对时间码。任何丢失都会出现在 `CompletenessReport.Missing` 中。测试覆盖音乐歌词、
画外音、双人对话、行内音效与时间重叠等场景。

## 测试

```sh
go test ./...
go test -race ./...
go vet ./...
```

覆盖：时间码/SRT（BOM、CRLF、缺序号、WebVTT 宽容解析）、分类规则表、说话人/画外音属性、
重叠组与车道（相接、三重重叠、复用、确定性、车道上限）、多行合并、SDH-off 完整性与
“编辑器丢字幕/改文本”检测、ASS 样式与车道边距、CLI 端到端。
