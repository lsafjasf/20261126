# 剪辑轨道状态快照

这个 Go 实现把 `mute`、`solo`、`lock`、`enabled` 当作四个独立布尔位保存，不再用一个字段承载它们。迁移和导入都按轨道处理，因此一条轨道的切换不会覆盖其他轨道。

## 状态快照

`SnapshotProject` 为每条轨道生成一个 `TrackSnapshot`。快照同时包含：

- `state`：完整、无损的四个布尔位；
- `projectedStatus`：给只能接收单值状态的旧消费端使用的投影；
- `unrepresentable`：投影无法携带的源标志；
- `revision`：该轨道自己的修订号。

示例：

```json
{
  "schemaVersion": 1,
  "projectId": "demo",
  "tracks": [
    {
      "trackId": "video",
      "revision": 4,
      "state": {
        "mute": true,
        "solo": true,
        "lock": true,
        "enabled": true
      },
      "projectedStatus": "locked",
      "unrepresentable": ["mute", "solo"]
    }
  ]
}
```

完整的 `state` 永远优先。只有必须读单值字段时才使用 `projectedStatus`，并且应同时读取 `unrepresentable`，不能悄悄丢弃组合状态。

## 冲突优先级

单值投影按以下顺序取第一个匹配项：

1. `disabled`：`enabled == false`，硬覆盖其他所有状态；
2. `locked`：`lock == true`，避免编辑保护状态在迁移中消失；
3. `muted`：`mute == true`，静音优先于 solo，避免误放出声音；
4. `solo`：`solo == true`；
5. `active`：无冲突的默认状态。

对应实现是 `State.Project()` 和 `ConflictPriority()`。

## 不可表达状态

“不可表达”只针对单值 `Status`。四布尔快照能表达全部 16 种组合；单值投影只有 5 种无损状态，即 `active`、`muted`、`solo`、`locked`、`disabled`。其余 11 种组合都是损失投影。

规则如下：

- 非 `disabled` 状态隐含 `enabled == true`；`disabled` 隐含 `enabled == false`。
- 除被选中的状态位外，任何为 `true` 的源标志都会写入 `unrepresentable`。
- `disabled` 优先于其他一切，因此 `disabled + mute/solo/lock` 会投影为 `disabled`，并报告所有额外的 `true` 位。

例如：

| 源状态 | `projectedStatus` | `unrepresentable` |
| --- | --- | --- |
| `mute=true` | `muted` | 无 |
| `solo=true, lock=true` | `locked` | `solo` |
| `mute=true, lock=true` | `locked` | `mute` |
| `mute=true, solo=true` | `muted` | `solo` |
| `mute=solo=lock=true` | `locked` | `mute`, `solo` |
| `enabled=false, mute=solo=lock=true` | `disabled` | `mute`, `solo`, `lock` |

## 导入保证

`ApplyPatch` 接收逐轨补丁，并且只在字段明确出现时写入：

```json
{
  "projectId": "demo",
  "tracks": [
    {
      "trackId": "music",
      "state": { "enabled": false }
    }
  ]
}
```

- 没列出的轨道完全不变，状态和 `revision` 都不变。
- 轨道中没出现的布尔字段保持原值；只有显式 `false` 才会关闭标志。
- 显式给出与当前相同的值不会增加修订号。
- 可选的 `expectedRevision` 用于拒绝过期写入；任一轨道校验失败时，整批导入不产生半更新。

这避免了把 JSON 缺失字段解码成 Go `bool` 零值 `false` 后误覆盖已有状态。

## 运行验证

```sh
go test ./...
go run ./cmd/state-demo
```

测试覆盖：

- 16 种组合投影与冲突优先级；
- 同一轨道连续切换 `mute -> solo -> lock -> enabled` 时四个位互不覆盖；
- 连续切换过程中其他轨道快照和修订号保持不变；
- 部分 JSON 导入只修改显式轨道和显式字段；
- 过期修订号导致整批导入原子失败。
