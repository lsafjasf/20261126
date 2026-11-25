# transitionhandle

纯 Go 标准库的转场 handle 审计与最小修复工程。

## 根因

源范围（source range）和序列范围（sequence range）是两个坐标空间。转场需要：

- 出点片段在切点之后的 `tail_frames`；
- 入点片段在切点之前的 `head_frames`。

错误的重导出步骤 `trim_handles_for_export` 把 handle 长度当成“向内修剪量”：

```go
// 错误：Source.End 左移、Source.Start 右移，引用不到预留素材。
left.Source.End -= transition.Tail
right.Source.Start += transition.Head
```

这会让源范围短于序列范围，转场窗口无法被源素材覆盖，后续导出只能退化为硬切。

最小修复是不要向内剪这两条边；在导出副本中按切点映射源坐标，并把源范围和序列范围同步扩展到 handle 区域：

```go
// 正确：两侧坐标空间移动相同帧数。
left.Source.End += tailFrames
left.Sequence.End += tailFrames
right.Source.Start -= headFrames
right.Sequence.Start -= headFrames
```

## 能力

- `Audit` / `AuditTrace`：按顺序遍历全部转场，比较源范围、序列范围、切点映射和媒体边界。
- `DiffSourceEdges` / `LocateDamage`：对比重导出前后快照，确认是哪一个命名步骤移动了 `source_start` 或 `source_end`。
- `ReverseEdits`：按修剪轨迹逆向恢复。
- `PrepareExport`：在副本上扩展 handle；输入工程不会被修改，重复执行结果稳定。
- 边界保证：实际帧数取请求值、媒体余量、序列余量三者最小值；核心映射失配时返回 `errMapping`，越界时包装 `ErrOutOfBounds`。余量为零时安全报告硬切，不生成越界范围。

## 运行

```sh
go test ./...

# 健康工程：审计并生成保留 handle 的导出
go run ./cmd/handlefix -in testdata/project.json -export /tmp/fixed.json

# 受损重导出工程：结合修剪轨迹定位具体步骤，再逆向恢复并导出
go run ./cmd/handlefix \
  -in testdata/damaged-reexport.json \
  -trace testdata/trim-edits.json \
  -export /tmp/restored.json
```

夹具包含两条转场；中间片段的入点 handle 被前一条转场修剪、出点 handle 被后一条转场修剪，轨迹仍能分别定位四个侧面。
