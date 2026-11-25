# timelinesnap

多人协作时间线的版本快照库（Go 单模块，无第三方依赖）。解决三个问题：

- 快照只记录**可达**媒体，丢弃资源表里的巨大无效条目；
- 差异比较区分**真实内容变化**与**无意义的表顺序抖动**；
- 快照可回滚，且通过内容寻址去重 + 保留策略防止存储无限膨胀。

## 核心模型

| 集合 | 顺序是否有意义 | 快照中的处理 |
| --- | --- | --- |
| 资源表 `resources` | 否（无序集合） | 仅保留被片段引用的资源，按 ID 排序 |
| 项目标记 `markers` | 否（无序集合） | 按 ID 排序 |
| 轨道 / 片段 / 效果 | 是（决定渲染与播放） | 完整保留顺序与轨道状态 |

快照 ID 是规范化 JSON 的 SHA-256：资源/标记表怎么打乱，内容哈希都不变；
片段时长、效果参数、轨道 `enabled/disabled/locked` 状态等任何真实改动都会产生新 ID。

## 差异语义

- `added` / `deleted` / `modified`：真实内容变化（含轨道状态、效果参数、meta 键级差异）。
- `reordered`：有序序列（轨道、片段、效果栈）重排——同样元素但顺序变了，属于真实变化。
- `order-only`：资源表/标记表纯抖动，值完全相同，**不计入** `HasContentChanges()`。
- 不可达资源的增删改完全不出现在差异里。

## 防膨胀与回滚

- Blob 按内容哈希存储，相同内容全局去重；相同内容重复提交是 no-op，不新增历史。
- `Revert` 不删除历史，而是追加一个指向旧快照 blob 的新提交。
- `GC(RetentionPolicy{MaxCommits: n})` 只保留最近 n 个提交，并清理无引用 blob。

## 库用法

```go
store, _ := timelinesnap.OpenFileStore(".timelinesnap")

// 构建并提交（自动裁剪不可达资源）
res, _ := timelinesnap.SnapshotProject(ctx, store, project, "v1", "alice")

// 比较两版
report, _ := timelinesnap.CompareRevisions(ctx, store, 1, 2)
report.HasContentChanges() // false 表示仅有 order-only 抖动

// 回滚到第 1 版（追加新提交，历史保留）
store.Revert(ctx, 1, "rollback", "bob")

// 恢复为可编辑项目
restored, _ := timelinesnap.RestoreCommit(ctx, store, 3)

// 限制历史规模
store.GC(ctx, timelinesnap.RetentionPolicy{MaxCommits: 20})
```

## CLI

```bash
go build ./cmd/timelinesnap
timelinesnap snapshot -project project.json -m "first cut" -a alice
timelinesnap list
timelinesnap diff -old 1 -new 2 --check   # 有真实变化时退出码 1
timelinesnap revert -rev 1
timelinesnap restore -rev 3 -out restored.json
timelinesnap gc -keep 20
```

## 测试

```bash
go test ./...      # 22 个用例，覆盖可达性、顺序无关哈希、差异分类、回滚与 GC
go test -race ./...
```
