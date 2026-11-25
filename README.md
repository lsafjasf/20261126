# clipmerge — 片段属性的确定性合并服务

对同一片段（clip）的并发编辑做**属性级三方合并**：只有当合并后的属性
组合仍满足全部媒体边界约束时才自动合并，否则输出可复现的冲突报告。
服务以 Go 实现，不依赖任何第三方包。

## 问题

两个客户端基于同一基线（revision 7）并发编辑同一片段：一个改入点、
一个改出点、另一个改变速。逐属性看每个编辑都合法，但三者共同决定
片段的时间线时长 `timelineDuration = (outPoint - inPoint) / speed`，
朴素合并会让时间线时长偏离任一客户端的预期，下游片段随之错位。
因此合并的正确性单位不是单个属性，而是**属性组合是否仍满足媒体边界**。

## 属性级冲突定义

片段的受管属性（规范顺序）：`inPoint`、`outPoint`（源内毫秒，int64）、
`speed`（播放速率，既约有理数 `numerator/denominator`）。

每条编辑（edit）是单属性操作 `{id, clientId, baseRevision, property,
value}`，**不携带时间戳**。相对共同基线做三方合并，冲突分两类：

- **同属性分歧 `same_property_divergence`**：多个客户端把同一属性改成
  不同的值。候选值 ≥ 2 时该属性不可裁决（`unresolved`），必须人工选择。
  **禁止按最后写入时间取胜**：编辑不含时间戳，到达顺序不影响结果，
  报告对编辑按 `id` 排序、对候选按值排序，输出与顺序无关。
- **依赖违例 `dependency_violation`**：各属性分别都成功裁决（每个属性
  至多一个候选值），但组合后的值违反媒体边界检查。冲突记录标注该检查
  依赖的属性集合、相关编辑 ID 与组合后的实际值。

另有过期编辑处理：`baseRevision` 与当前基线不一致的编辑不参与合并，
记入 `ignoredEdits`（`stale_ignored`），不产生冲突。

## 依赖关系

```
inPoint ─┐
         ├─► sourceSpan = outPoint - inPoint ─┐
outPoint ┘                                    ├─► timelineDuration = sourceSpan / speed
speed ────────────────────────────────────────┘
```

- `inPoint`、`outPoint` 共同定义源区间：`0 <= inPoint < outPoint <= duration`。
- `speed` 把源区间映射为时间线占用：`timelineDuration = sourceSpan / speed`。
- 三者任一变化都会改变 `timelineDuration`，即改变下游片段的时间线位置——
  这就是"画面错位"的根源。因此 `timeline_duration_min` 检查同时依赖三个
  属性，任一单属性编辑都可能让组合失效。

边界检查（固定顺序，全部通过才允许自动合并）：

| 检查 | 依赖属性 | 规则 |
|---|---|---|
| `source_in_bounds` | inPoint | `0 <= inPoint <= duration` |
| `source_out_bounds` | outPoint | `0 <= outPoint <= duration` |
| `source_interval_order` | inPoint, outPoint | `inPoint < outPoint` |
| `speed_positive` | speed | `speed > 0` |
| `timeline_duration_min` | 三者 | `timelineDuration >= minTimelineDuration` |

## 自动合并门控

1. 每个属性独立三方合并：候选去重后 0 个 → 取基线值；1 个 → 取该值；
   ≥ 2 个 → 不可裁决。
2. 存在不可裁决属性 → `manual_resolution_required`，边界检查全部跳过
   （`skipped`），绝不猜测取值。
3. 全部属性裁决成功 → 对组合结果运行全部边界检查：
   - 全部通过 → `auto_merged`，报告附 `mergedClip`（revision + 1）。
   - 任一失败 → `media_boundary_violation`，每个失败的检查生成一条
     `dependency_violation` 冲突，不产出合并结果。

## 可复现的冲突报告

报告是请求的纯函数：无时钟、无随机、无到达顺序。同一请求永远得到
字节级一致的报告（`clip-merge/v1`）。

- 所有集合确定性排序：编辑按 `id`，候选按值，`editIds`/`clientIds`
  字典序，属性与检查按规范顺序。
- 时间用 int64 毫秒、速率用既约有理数，`timelineDuration` 精确计算，
  无浮点误差。
- `reportId` / `conflictId` 是内容哈希：对**省略该 ID 字段**的规范
  JSON（紧凑、按键序、不做 HTML 转义）取 SHA-256，前缀 `sha256:`。
  ID 只随内容变化，可独立校验。
- 决策码：`auto_merged` / `manual_resolution_required` /
  `media_boundary_violation`。

`artifacts/` 与 `testdata/` 收录三个场景的报告：三属性干净合并、
三属性组合违例（错位场景）、同属性分歧。

## 使用

```sh
# 一次性评估
go run ./cmd/clipmerge run -request testdata/auto-merge-request.json

# HTTP 服务
go run ./cmd/clipmerge serve -addr 127.0.0.1:8080
curl -X POST localhost:8080/v1/merges --data @testdata/auto-merge-request.json

# 测试
go test ./...
```

## 布局

- `types.go` / `value.go` — 领域模型与属性值（有理数、确定性排序）
- `merge.go` / `resolve.go` — 属性级三方合并引擎
- `checks.go` — 五项媒体边界检查
- `report.go` — 报告模型、规范 JSON 与内容哈希
- `server.go` — HTTP API（`POST /v1/merges`、`GET /healthz`）
- `cmd/clipmerge` — CLI（`run` / `serve`）
- `testdata/` — 场景请求与 golden 报告
