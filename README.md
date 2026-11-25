# FCPXML 效果参数命名空间

修复 FCPXML 导入时“不同效果或不同参数源中的同名参数互相覆盖”的问题。
项目是纯 Go 实现，只使用标准库，不依赖外部 XML、编辑器或媒体工具。

## 根因与修复

旧导入器把参数拍平成一个以 `name` 为键的映射。下面参数的 `name` 都是
`Amount`，后写入的值会静默覆盖先写入的值：

- `filter-video` 引用 `com.example.blur`，参数值为标量；
- 同一个 `filter-video` 中的结构化 `Amount`；
- `com.example.glow` 中来自旧式 `<parameter>` 的 `Amount`。

本实现为每个参数建立显式命名空间：

```text
effect kind + effect type + parameter source + name + value shape
            + parameter key + legacy parameter ID
```

- **effect kind**：参数所属的 XML 容器，例如 `filter-video`、
  `filter-audio`、`effect`、`adjust-volume`、`title`。
- **effect type**：优先从 `ref` 解析 resources 中的 effect
  `effectID`，其次使用当前节点的 `effectID`、`uid`、`name`。
- **parameter source**：从效果容器到参数元素的路径，例如
  `param[Amount]`、`param[Amount]/param[Center]`、`parameter[Amount]`。
- **value shape**：`scalar`、`structured`、`empty`、`raw`。因此同名同源
  的标量参数与结构化参数也不会碰撞。
- **key / parameter ID**：FCPXML 中用于标识具体参数的附加信息。

命名空间字符串对每段先写字节长度再写内容，因此名称中出现冒号、斜杠或
空字符时也不会产生拼接歧义。`Parameter.NamespaceID` 和 JSON 的
`namespace` 字段就是这个完整值。

Document 内部保存参数列表而不是“名称 -> 单值”映射。即使两个参数最终
具有相同命名空间，也会全部保留；读取方不会因后写值而丢失先写值。

## 映射与有损保存

支持的参数结构会规范化为 `ParameterValue`：

- `<param name="..." value="..."/>`；
- 带纯文本 `<value>` 的旧式 `<parameter>`；
- 嵌套 `<param>` 的结构化值；
- 旧式 `<value><horiz>...</horiz><vert>...</vert></value>`。

以下内容无法无损规范化为当前参数模型：

- `<data key="effectData">` 等二进制效果数据；
- `keyframe` 动画参数；
- 混合文本与子元素；
- 未知的子元素或 `value` 节点的未知属性；
- 缺少 `name`、`key` 和 `parameterID` 的参数。

这些参数仍会保留在列表中，并满足以下约束：

- `Mapped == false`；
- `Raw` 保存导入时的完整 XML 子树，原值不变；
- `Snapshot().Issues` 包含稳定的错误码和命名空间；
- `Validate()`、`Save()`、`SaveFile()` 返回 `ErrLossySave`。

`SaveFile` 在创建临时文件前先校验，因此有损文档不会截断或覆盖目标文件。
无有损问题时，保存写回导入的原始 XML 字节，不改变滤镜顺序等语义。

## 确定性

参数最终按命名空间、规范化值、规范化 XML 排序，不依赖 Go map
迭代顺序、输入节点顺序或 XML 属性顺序。`Snapshot()` 不包含仅反映格式
差异的 `Raw`，因此把效果、resource 和属性随机重排后导入，得到的
snapshot、JSON 和 SHA-256 digest 均一致。精确原始字节仍只在
`Document.Parameters()` / `Parameter.Raw` 中提供。

## 运行

```sh
go test ./...

go run ./cmd/fcpxml-params inspect testdata/collision.fcpxml
go run ./cmd/fcpxml-params save testdata/collision.fcpxml out.fcpxml

go run ./cmd/fcpxml-params inspect testdata/unmapped.fcpxml
# 输出导致阻断的 namespace/issue，退出码 1
go run ./cmd/fcpxml-params save testdata/unmapped.fcpxml out.fcpxml
# 拒绝保存，退出码 1，目标文件保持不变
```

也可使用 `save -out OUTPUT INPUT`。退出码约定：

- `0`：成功且无损；
- `1`：已识别到无法映射的参数，保存被阻断；
- `2`：参数错误、XML 解析错误或 I/O 错误。

`inspect` 在退出码为 `1` 时仍会先输出完整 JSON，便于定位具体参数。

## Go 接口

```go
doc, err := fcpxml.ParseFile("input.fcpxml")
if err != nil {
    // XML 或 I/O 错误
}

snapshot := doc.Snapshot()
for _, p := range snapshot.Parameters {
    // p.NamespaceID、p.EffectType、p.Source、p.Shape、p.Value
}

if err := doc.Validate(); errors.Is(err, fcpxml.ErrLossySave) {
    // p.Raw 已保留所有无法映射的原值
}

if err := doc.SaveFile("output.fcpxml"); err != nil {
    // 有损保存会被拒绝：
    // errors.Is(err, fcpxml.ErrLossySave)
}
```

`testdata/collision.fcpxml` 同时包含标量/结构化同名参数、不同效果类型的
同名参数和旧式 `<parameter>`。`testdata/unmapped.fcpxml` 演示二进制数据
被保留、报告并阻断保存。

## 测试覆盖

- 不同效果类型的同名参数使用不同命名空间；
- 标量与结构化同名参数使用不同命名空间；
- 旧式 `parameter` 与 `param` 参数源互不覆盖；
- 相同命名空间的两个不同值也全部保留；
- 随机重排 resource、filter、属性顺序后 snapshot 与 JSON 完全一致；
- 二进制数据、动画参数、未知结构化属性保留 `Raw` 并阻断保存；
- `SaveFile` 失败时原目标文件内容不变；
- CLI 的 inspect/save 与退出码行为。
