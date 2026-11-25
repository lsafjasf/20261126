# cuemap

`cuemap` 为中、阿、日混排字幕建立“逻辑字符 → 屏幕显示顺序”的可验证映射，
并处理组合字符、日文注音（ruby）附挂、横竖排间距和无法安全排版时的 cue 回指。

实现、CLI、测试和 `testdata/` 素材只使用 Go 标准库；`go.mod` 没有第三方依赖。

## 解决的问题

- 阿拉伯语不能按裸 `[]rune` 拆开后整体反转：组合记号必须跟随基础字符，
  混合的中文、英文、数字和空格要按双向级别分段重排。
- 日文注音不能作为普通正文插入：注音通过 rune 区间附挂到基准文字，竖排时
  使用独立注音轨道，并扩大列间距/字距避免覆盖。
- 排版结果保留每个屏幕字格到逻辑 rune/字簇的双向索引。
- 注音区间非法、跨行、重叠或几何尺寸超过约束时，不输出危险坐标，而是在
  `Result.Fallback` 中回指原始 `Cue` 和原始素材文本。

## 逻辑到屏幕映射

```go
mapping := cuemap.MapLine("中文 مرحبا", cuemap.DirectionAuto)

// ScreenOrder[screenSlot] = logical cluster index
// LogicalToScreen[logicalCluster] = screenSlot
// RuneToScreen[logicalRune] = screenSlot
// ScreenToRunes[screenSlot] = logical rune indexes
```

映射分两层：

- `RuneInfo`：每个 Unicode 码点的字节偏移、rune 序号、bidi 分类和所属字簇。
- `Cluster`：不可拆分的显示单元。组合记号、变体选择符、肤色修饰、常见
  ZWJ 序列和区域指示符配对会与基础字符合并。

双向解析使用字幕场景所需的级别模型：自动基础方向、L/R/AL、EN/AN、
分隔符/空格/中性字符解析、隐式级别，以及从最高级别向下的标准连续段重排。
`ScreenOrder` 一定是逻辑字簇的排列，`LogicalToScreen` 与其互为反查。

`Mapping.Visual` 是按屏幕顺序拼接的诊断文本。需要真正塑形时，应继续把
`Cue.Text` 这样的逻辑文本交给支持 UBA/Arabic shaping 的平台，并用本包的
索引做命中、标注和回指；不要把 `Visual` 再交给 bidi 渲染器二次处理。

## 注音与排版

可以直接构造 rune 区间：

```go
cue := cuemap.Cue{
    ID:   "cue-42",
    Text: "東京",
    Raw:  `<ruby>東<rt>とう</rt></ruby>京`,
    Ruby: []cuemap.RubySpan{{
        StartRune: 0,
        EndRune:   1,
        Text:      "とう",
    }},
}

result := cuemap.LayoutCue(cue, cuemap.Options{
    Mode: cuemap.ModeVertical,
    Metrics: cuemap.Metrics{
        BaseSize: 24, RubySize: 12, RubyGap: 4,
        CharSpacing: 0, LineSpacing: 10, Padding: 8,
    },
})
```

也可以解析常见注音标记：

```go
cue := cuemap.ParseCue("jp-1", `<ruby>東<rt>とう</rt></ruby>京`)
aozora := cuemap.ParseCue("jp-2", `｜東京《とうきょう》`)
```

坐标使用整数像素样单位，原点为左上角，X 向右、Y 向下：

- `ModeHorizontal`：注音位于基准文字上方，硬换行向下堆叠。
- `ModeVertical`：字簇沿 Y 轴自上而下，硬换行形成自右向左的列；注音位于
  基准列右侧的独立轨道。
- 长注音会扩大相邻基准字距；列间距至少包含 `BaseSize + RubyGap + RubySize`
  和 `LineSpacing`，防止注音侵入相邻中文竖排列。
- `FindCollision` 会检查所有最终 `Cell.Rect`，包括跨硬换行/竖排列的单元。

## 无法排版时回指 cue

```go
result := cuemap.LayoutCue(cue, options)
if !result.OK {
    original, _ := result.OriginalCue()
    log.Printf("fallback %s -> %s", result.Fallback.Reason, result.Fallback.CueRef)
    renderRaw(original.Raw)
}
```

回退原因包括：

- `invalid_ruby` / `empty_ruby` / `overlapping_ruby`
- `ruby_crosses_line`
- `layout_bounds_exceeded`
- `glyph_collision`
- `unsupported_layout`

`Fallback.Cue` 保留完整输入 cue，`CueRef` 为 `cue:<ID>`，`OriginalText`
优先使用 `Cue.Raw`。

## SRT 与素材生成

`ParseSRT` 可读取最小 SubRip 块，解析时间码和 ruby 标记，并在 `Raw` 中保留
原始 cue 块。

重新生成测试素材：

```sh
go generate ./...
# 或
go run ./cmd/genfixtures -out testdata
```

生成内容：

- `testdata/cues.json`：阿语组合字符、中阿日混排竖排、HTML/青空注音、
  重叠注音回退和尺寸超限回退。
- `testdata/trilingual.srt`：同一组素材的 SubRip 版本。

查看某个 cue 的完整映射和坐标：

```sh
go run ./cmd/cuemap -text '中文 <ruby>東<rt>とう</rt></ruby>' -mode vertical
go run ./cmd/cuemap -fixtures testdata/cues.json -name fallback-overlapping-ruby
```

## 验证

```sh
go fmt ./...
go vet ./...
go test -count=1 ./...
```

测试覆盖阿语 RTL 段反转、阿英混合中英文段不反转、rune/字簇双向反查、
组合记号同槽、HTML/青空注音解析、竖排注音轨道和列间距、长注音扩距、
碰撞检测、SRT 回放、生成素材不变量以及原始 cue 回指。
