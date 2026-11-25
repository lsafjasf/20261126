# aafdiag — AAF 引用图诊断与最小工程修复

针对 AAF 工程中描述性元数据（DescriptiveMetadata）、源片段（SourceClip）
与组合片段（CompositionMob/OperationGroup）引用互相断开的问题：重建对象
引用图，找出缺失节点与不可表达属性，输出可重导入的最小工程，或在无法
安全修复时给出明确失败原因。仅使用 Go 标准库。

关键原则：缺失效果不等于无效果。OperationGroup 引用的效果定义
（OperationDefinition）、效果输入或参数定义缺失时是致命错误，绝不静默
丢弃（这正是现有转换器的缺陷）。

## 用法

```sh
go build -o aafdiag .
./aafdiag -in broken.json -out minimal.json -report report.json
```

- 退出码 0：已输出最小工程（允许警告）。
- 退出码 1：存在致命问题，未输出工程；失败原因见报告。
- 退出码 2：用法或 IO 错误。

## 输入格式

AAF 对象转储（JSON）：每个对象有 id、class、properties。属性值中
`{"ref": id}` 为强引用，`{"weakref": id, "targetClass": ...}` 为弱引用，
`{"opaque": base64, "type": ...}` 为最小模型不可表达的原始数据，其余为
标量或数组。

## 诊断与修复策略

| 代码 | 级别 | 处理 |
| --- | --- | --- |
| MISSING_EFFECT / MISSING_EFFECT_INPUT / MISSING_EFFECT_PARAM | fatal | 效果定义/输入/参数缺失，明确失败 |
| MISSING_STRUCTURAL / MISSING_DATA_DEF / MISSING_NODE | fatal | 结构引用断裂，无法重组 |
| INEXPRESSIBLE_CLASS | fatal | 可达对象不属于最小可表达模型 |
| MISSING_SOURCE | warning | 源片段指向缺失 SourceMob，替换为等长 Filler 保持时长 |
| MISSING_METADATA / MISSING_ESSENCE_DESC / DANGLING_WEAKREF | warning | 断开挂载/弱引用，安全丢弃 |
| INEXPRESSIBLE_PROP / INEXPRESSIBLE_VALUE | warning | 最小模型无对应属性/值，丢弃 |
| ORPHAN_PRUNED | info | 从 Header 不可达的对象被剪除 |

最小工程的可表达模式见 schema.go。发射前执行自检：输出重新加载后
引用图必须无悬空边（可重导入性证明）。

## 示例

```sh
./aafdiag -in testdata/broken_recoverable.json -out minimal.json
./aafdiag -in testdata/broken_fatal.json
go test ./...
```
