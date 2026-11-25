# ssastyle

旧 SSA（SubStation Alpha）样式经常同时存在**多层继承、同名样式/重复字段、
空列与显式 null**。朴素转换用 map “后者覆盖前者”，会让后出现的 `null`/空串
把继承链上游或先出现的有效值抹掉，最终样式静默变化。

`ssastyle` 是一个**只依赖 Go 标准库**的模块与命令行工具，负责：

1. 用**存在性感知的值**实现正确的空值语义；
2. 解析 `BasedOn/Parent` 继承链（含环、断链检测），求出每个样式的**最终生效值与来源**；
3. **逐字段比较**旧 SSA 生效样式与转换结果，给出 `equal/different/missing/extra/invalid`；
4. 把 `AlphaLevel`、未知扩展列等**无法映射或只能有损派生的属性单独报告**，绝不静默丢弃；
5. 解析/回写真实的 `[V4 Styles]`(SSA) 与 `[V4+ Styles]`(ASS)，也支持 JSON。

## 快速开始

```sh
go test ./...
go build ./...

# 跑一个自包含比对用例（退出码 0=一致，1=有差异，2=用法/IO 错误）
go run ./cmd/ssastyle compare -case testdata/case_ok.json

# 复现“后出现的空值覆盖有效值”，查看逐字段差异与不可映射属性
go run ./cmd/ssastyle compare -case testdata/case_buggy.json

# 解析旧 SSA 的继承链与最终生效样式
go run ./cmd/ssastyle resolve -in testdata/legacy.ssa

# 正确转换：JSON（含 unmapped_properties）或回写 ASS
go run ./cmd/ssastyle convert -in testdata/legacy.ssa
go run ./cmd/ssastyle convert -in testdata/legacy.ssa -format ass
```

也可以直接比较两个文件（按扩展名识别 `.ssa/.ass/.json`）：

```sh
ssastyle compare -legacy old.ssa -converted new.ass -json
ssastyle compare -legacy old.ssa -converted converted.json -styles Default,Title
```

## 空值语义

每个值用 `Presence` 区分三种状态，`0`、`-1`、`false` 这类**零值是真实值**，
绝不能当空值：

| 输入 | Presence | 默认合并行为 |
| --- | --- | --- |
| 键缺失 / SSA 缺列 | `Absent` | 无意见，继续取继承值 |
| `null` / SSA 空位置列 | `ExplicitNull` | “本层未提供”，**不覆盖**已有/继承值 |
| `""` 或纯空白 | `Set` + 空串 | 默认 `empty_is_unset=true`，与 null 同等处理 |
| `0` / `-1` / `false` | `Set` | **真实值，正常覆盖** |

可切换的策略（CLI flag / `MergeOptions`）：

- `-keep-empty`（`empty_is_unset=false`）：空白串是显式值，可覆盖；
- `-null-reset`（`explicit_null_resets=true`）：更近层的显式 `null` 表示
  **有意重置/切断继承**，解析结果留下 `reset` 标记，而不是继续向父层取值；
- `-dup-error`（`duplicate_conflict_is_error=true`）：同层重复字段取值冲突按错误处理。

只有当空值确实试图抹掉一个**已经生效的值**时，才产生一条
`empty_value_ignored` 诊断；普通的“该列留空”不制造噪声。

## 继承链与重复字段

- 样式可用 `Parent` / `BasedOn` / `Inherits`（JSON 键或 SSA `Format` 扩展列）声明父样式。
- 解析顺序为 root → leaf，链上每层按字段出现顺序做 null 安全合并，因此：
  - 父层有效值 → 子层 `null`：保留父值；
  - 先出现有效值 → 后出现 `null`：保留先值；
  - 后出现的另一个有效值：按“最后一个有效值生效”。
- 每个最终字段都带 `source_style/source`，可追溯是哪一层、哪一行提供的。
- 同名多条 `Style:`、JSON 数组中的重复样式、甚至 **JSON 对象里的重复键**
  都会被完整保留（标准 `encoding/json` 进 map 会丢前值，这里用流式 Decoder 保留）。
- 继承环报 `inheritance_cycle`(error) 并安全截断；未知父样式报 `unknown_parent` 并断链。

## 逐字段比较

`Compare` 分别把两侧文档解析成最终生效样式，再按映射表逐字段归一化后比较。
归一化消除无关的书写差异：

- 颜色：`&HAABBGGRR` / `&HBBGGRR` / `#RRGGBB[AA]` 统一为 RGBA（注意 ASS 的
  Alpha 字节 `00=不透明` 与常规约定相反，解析时会翻转）；
- 布尔：`-1/1/true/yes/on` 与 `0/false/no/off`；
- 数值：`20` 与 `20.0`、`010` 与 `10` 相等；
- 对齐：旧 SSA 码 `5/6/7 → 7/8/9`、`9/10/11 → 4/5/6` 换算为 ASS 小键盘码
  （按文档方言 `ssa`/`ass` 处理）；
- 键名：`Font_Name`、`fontName`、`font-name`、`MarginLeft`、美式/英式
  `Color/Colour` 都归到同一规范键。

每个字段的状态：`equal`、`different`、`missing_in_converted`（典型即被空值
覆盖或丢失）、`extra_in_converted`、`invalid_value`，并带两侧原始值、归一化值
和生效来源。

## 无法映射的属性

默认映射表是旧 SSA V4 → ASS V4+ / 规范模型：

- 直接映射：`Fontname/Fontsize/PrimaryColour/Bold/...`；
- 改名：`TertiaryColour → OutlineColour`；
- 值变换：`Alignment`（旧码换算）；
- 有损派生：`AlphaLevel`——ASS 无独立字段，只能折算进颜色 Alpha 字节，
  作为 `derived` 单独报告，不做静默转换；
- 完全无法映射：未知扩展列（如 `XStudioID`）作为 `unmapped` 单独报告。

这些属性进入报告顶层与样式内的 `unmapped_properties`，**不参与字段一致性
判定**，也不会在转换时被静默丢弃。加 `-strict-unmapped` 可让 CI 在存在
不可映射/有损属性时也返回非零。可通过 `CompareOptions.Mapping` 提供自定义映射表。

## 输入格式

- `.ssa`：读取 `[V4 Styles]`，缺省使用 SSA 18 列；按 `Format:` 动态映射。
- `.ass`：读取 `[V4+ Styles]`，缺省使用 ASS 23 列。
- `.json`：三种形态都支持，且保留 `null` 与重复键：

```json
{
  "dialect": "ssa",
  "styles": [
    {"name": "Global", "fields": {"fontname": "Arial", "marginl": 10}},
    {"name": "Title", "parent": "Global", "fields": [
      ["fontsize", 48], ["bold", -1], ["fontname", null]
    ]}
  ]
}
```

`fields` 可以是对象、`[["k", v], ...]`（保重复次序）或
`[{"key":"k","value":v}]`。也接受顶层 `{ "StyleName": { ...字段 } }` 的扁平映射。

`-case` 文件可把两侧写在一起，`legacy`/`converted` 既可以是内联文档，
也可以是相对于 case 文件的路径字符串，另支持 `styles`、`merge`、
`report_extra`、`strict_unmapped`、`mapping`。

## 退出码与 JSON

| 退出码 | 含义 |
| --- | --- |
| `0` | 所有可映射字段一致（普通模式下有 unmapped 仍为 0） |
| `1` | 存在字段差异/错误诊断；或 `-strict-unmapped` 命中 |
| `2` | 参数、读取或解析错误 |

`-json` 输出稳定结构的报告：`equal/ok/summary/styles[].fields[]/
unmapped_properties/diagnostics`，可直接接入 CI 或代码审查流水线。

## 作为库使用

```go
package main

import (
    "fmt"
    "ssastyle"
)

func main() {
    legacy, _, err := ssastyle.LoadDocumentFile("testdata/legacy.ssa")
    if err != nil {
        panic(err)
    }
    converted, _, err := ssastyle.LoadDocumentFile("testdata/converted_buggy.json")
    if err != nil {
        panic(err)
    }

    rep := ssastyle.Compare(legacy, converted, ssastyle.DefaultCompareOptions())
    fmt.Println("equal:", rep.Equal, "unmapped:", rep.Summary.Unmapped)

    resolved := ssastyle.NewCatalog(ssastyle.DefaultMergeOptions(), legacy).ResolveAll()
    if v, ok := resolved["Title"].Value("fontname"); ok {
        fmt.Println("Title fontname =", v)
    }
}
```

## 工程结构

- `value.go`：存在性感知值与空值合并选项；
- `model.go` / `jsonio.go` / `jsondup.go`：样式模型、JSON 与重复键保留解析；
- `ssa.go`：SSA/ASS `Format` 驱动解析；
- `fields.go` / `normalize.go`：字段登记、颜色/布尔/数值/对齐归一化；
- `resolve.go`：目录、继承链、来源追溯与诊断；
- `mapping.go` / `convert.go` / `asswrite.go`：映射表、正确转换与 ASS 回写；
- `compare.go` / `render.go`：逐字段比较、报告汇总与文本渲染；
- `cli.go` / `cli_compare.go` / `cli_ops.go` / `cmd/ssastyle`：命令行；
- `testdata/`：多层继承 + 重复 Style 行 + 扩展列的旧 SSA、有缺陷的转换结果和 case。

仅使用 Go 标准库；`go test -race ./...` 与 `go vet ./...` 均通过。

