# ttmlevents

纯 Go 标准库实现的 TTML 嵌套时间轴切片器。它不再只读取最外层 `<span>`，而是：

- 保留 XML 混合文本顺序，递归处理所有嵌套 `span`、`p`、`div` 和 `<br>`；
- 按 TTML/SMIL 时间容器语义归算祖先 `begin/end/dur`，并用父区间裁剪后代；
- 解析 `style`、`xml:id` 样式表、内联 `tts:*`、样式继承与 IDREF；
- 展开内部 `<animate>` / `<set>`，生成关键帧、线性/离散值和 `fill="freeze"` 区间；
- 汇总所有文本、元素和动画边界，形成相邻、无重叠的事件切片；
- 对同一时间戳使用固定因果全序，避免 map 迭代或 XML 顺序造成不确定输出；
- 输出机器可读 JSON、事件 CSV 和可直接评审的 Markdown 事件表。

## 快速运行

```bash
go test ./...

go run ./cmd/ttml-events \
  -i testdata/nested_animation.ttml \
  -format markdown

# 无参数运行（stdin 为终端或 /dev/null）会输出内置样例事件表
go run ./cmd/ttml-events -demo

go run ./cmd/ttml-events -i input.ttml -format json -o events.json
go run ./cmd/ttml-events -i input.ttml -format csv  -o events.csv
```

也可以从 stdin 读取：

```bash
cat input.ttml | go run ./cmd/ttml-events -format markdown
```

如果输入文件把每个嵌套节点的 `begin/end` 都写成文档媒体轴上的绝对时间，可使用兼容模式：

```bash
go run ./cmd/ttml-events -i input.ttml --absolute-times
```

## Go API

```go
package main

import (
    "fmt"
    "ttmlevents"
)

func main() {
    table, err := ttmlevents.AnalyzeString(`<tt><body>
        <p begin="0s" end="4s">
            <span begin="0s" end="3s" tts:color="red">外层
                <span begin="1s" end="2s" tts:color="blue">内层</span>
            </span>
        </p>
    </body></tt>`)
    if err != nil {
        panic(err)
    }

    for _, slice := range table.Slices {
        fmt.Printf("[%.1f, %v) %s\n",
            slice.BeginSeconds, slice.EndSeconds, slice.Text)
    }

    fmt.Print(ttmlevents.RenderMarkdown(table))
}
```

需要绝对时间兼容模式时：

```go
table, err := ttmlevents.AnalyzeStringWithOptions(src, ttmlevents.TimingOptions{
    AbsoluteTimes: true,
})
```

## 输出模型

`ttmlevents.EventTable` 包含：

- `Events`：扁平、全局有序的因果事件行；
- `Groups`：按相同时间戳分组的事件，组内顺序固定；
- `Slices`：所有边界之间的半开区间 `[begin,end)`，含当时可见文本和生效样式；
- `Runs`：每个源文本节点或 `<br>` 的独立时间区间；
- `StyleIntervals`：按 `RunID + CSS/TTML 属性` 展开的最终样式区间；
- `Animations`：解析后的 `<animate>` / `<set>` 轨道和关键帧分段；
- `Timeline`：每个 XML 元素的 nominal/effective 时间区间；
- `Warnings`：坏时间值、缺失样式、被祖先裁剪等可恢复问题。

## 同刻顺序

活动区间采用半开区间 `[begin,end)`。同一时间戳按以下全序排列：

1. 时间升序；
2. 阶段顺序：`end/摘除` → `begin/进入` → `change/关键帧`；
3. `begin`：XML 前序遍历，祖先先于后代，兄弟保持源码顺序；
4. `end`：XML 后序遍历，后代先于祖先，兄弟保持源码顺序；
5. 关键帧按动画节点文档顺序和 keyframe 序号；
6. 最后以稳定 tie-breaker 兜底，并写入 `orderKey`。

每个事件还有 `sequence` 和 `causeSequence`。例如同刻换班时，会明确得到：

```text
end #inner → end #outer → begin #replacement
```

而不是依赖 Go map 或并发调度。

## 时间语义

默认遵循 TTML 的并行时间容器：子节点的 `begin` 和 `end` 是相对最近时间容器起点的偏移。父节点活动区间会裁剪所有后代的有效区间。`timeContainer="seq"` 也有基础支持：兄弟元素按前一个有限结束点依次排列，`begin` 作为额外偏移。

支持的常见格式：

- `1.5s`、`250ms`、`2m`、`1h`；
- `00:01:02.500`、`01:02:03`；
- `00:00:01:15` 帧时间码，帧率读取 `ttp:frameRate`，缺省 30fps；
- `f` / `t` 单位，分别使用 `frameRate` 和 `tickRate`。

若输入实际使用“所有属性都是文档绝对时间”的方言，请显式打开 `AbsoluteTimes`；这是一个有意的兼容选项，而不是静默猜测。

## 样式与动画

样式级联顺序为：

1. 可继承的祖先计算样式；
2. 当前节点 `style="id1 id2"` 引用的样式规则，IDREF 按顺序应用；
3. 规则自身的父样式引用；
4. 当前元素内联 `tts:*` 属性；
5. 当前活动动画 / `<set>` 覆盖。

祖先动画不会越过后代节点上更靠近文本的显式声明；实现通过 `DeclarationDepth` 保留这一级联边界。

线性动画会尝试插值：

- `#rgb` / `#rrggbb` / `rgb(r,g,b)` 和常见颜色名；
- 带相同单位的数值，例如 `0` → `1`、`12px` → `24px`。

无法插值的字符串保留 `from/to` 和插值模式，不伪造数值。

## 验证

```bash
gofmt -w .
go test ./...
go vet ./...
```

测试覆盖嵌套文本、祖先相对时间、父区间裁剪、同刻开始/结束顺序、确定性输出、动画关键帧、freeze/回退、隐藏文本和多种时间格式。

## 目录

- `parse.go`：标准库 XML 混合内容解析；
- `timecode.go`：TTML 时间表达式；
- `timing.go`：祖先/par/seq 时间归算；
- `content.go`：文本 run 与 `<br>` 展平；
- `style.go`：样式表、继承和 IDREF；
- `animation.go`、`effective.go`：动画轨道、优先级和插值；
- `builder.go`、`style_intervals.go`：边界、切片和样式 lane；
- `events.go`：因果事件与全序排序；
- `render.go`：Markdown/JSON/CSV 输出；
- `cmd/ttml-events`：命令行入口；
- `testdata/nested_animation.ttml`：嵌套 span + 动画样例。
