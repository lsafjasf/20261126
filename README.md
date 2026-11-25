# 20261126

Repository initialized for 20261126.

# 片段组事务化编辑（Go）

针对“片段组整体移动后撤销，组内单项的锁定状态与效果参数没有恢复”的修复实现。
核心思路：**组操作 = 一个不可部分提交的事务**，事务保存「组、全部成员片段、
效果依赖资源」提交前后的完整版本；所有 Do/Undo/Redo 写入带 CRC 的分帧日志，
日志损坏时恢复到最近完整点；交错的 Undo/Redo 序列可序列化并重放。

## 运行

```bash
go test -race ./...     # 全部单元测试（含竞态检测）
go run ./cmd/demo       # 端到端演示
```

仅依赖 Go 标准库（`go.mod` 声明 `go 1.21`）。

## 设计要点

- `model.go`：`Group` / `Clip`（`Locked`、`Effects.Params`）/ `Resource` 模型，
  以及组聚合的完整深拷贝快照 `Snapshot`（组 + `MemberIDs` 中全部片段 +
  效果引用的全部依赖资源）。
- `editor.go`：`Editor.Transact` 在隔离的 `Aggregate` 副本上执行修改：
  - 变更函数失败 / 聚合不完整 → 直接中止，文档、版本号、Undo 栈、日志均不变；
  - 成功后先把事务帧 `fsync` 落盘，再切换内存状态，杜绝部分提交。
  - `Undo()` / `Redo()` 在事务的 `Before` / `After` 两个完整快照点之间整体切换，
    所以锁定状态、效果参数、依赖资源版本与坐标一定一起恢复。
- `tx.go`：`TxRecord`（Before/After 版本快照）与 `VersionStore`，
  保存组、成员、依赖资源出现过的每个版本及其来源事务。
- `journal.go`：追加式分帧日志，帧格式
  `"CGTX" | uint32 长度 | JSON Entry | CRC32`。
  `ReadJournal` 顺序扫描，遇到截断 / 坏 magic / CRC 失配即停止，
  此前连续有效的帧前缀就是“最近完整点”；`RecoveryReport.Repair`
  把坏尾截断后可继续追加。
- `ops.go`：可 JSON 序列化的 `Op` / `Script`，`Replay` / `ReplayAt`
  支持把交错 Do/Undo/Redo 脚本在内存或新日志上确定性重放。

日志中的每个 `Entry` 为 `commit` / `undo` / `redo` 之一，因此重放日志本身
即可重建任意时刻的文档状态与 Undo/Redo 栈；`OpenEditor` 会自动恢复并修复。

## 关键测试

- `regression_test.go`：移动组后 Undo，逐项断言 `Locked`、`Effects.Params`、
  坐标完整恢复；组合事务（位置+锁定+效果+资源）一次 Undo 全部还原；
  并反证“只恢复位置”的部分撤销确实会丢失锁定与效果参数。
- `transaction_test.go`：变更中途报错不产生任何部分提交；日志写入失败时
  内存状态不前进；事务快照包含组/成员/依赖资源的版本。
- `recovery_test.go`：尾部截断、日志中段字节损坏（CRC 失配）、追加垃圾帧
  三类损坏都恢复到正确的最近完整点；修复后可继续提交与撤销。
- `replay_test.go`：三段交错 Undo/Redo 脚本（含三轮 undo/redo 交替与分叉
  新提交），分别经内存、JSON 脚本往返、日志恢复三条路径重放，
  文档摘要与双栈完全一致。
- `reopen_undo_test.go`：移动落盘 → 关闭 → 重开恢复 → Undo，
  锁定与效果参数仍然完整恢复。
