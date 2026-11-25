# 20261126

Repository initialized for 20261126.

## 有理数锚定时间轴

`rational-timeline` 是纯 Go 工具，用 `math/big.Rat` 保存秒数和帧坐标，不经过浮点中转。

### 生成内置混合帧率报告

```bash
go run ./cmd/rational-timeline \
  -json artifacts/anchor-report.json \
  -markdown artifacts/anchor-report.md
```

不传 `-input` 时使用内置工程：25 fps 序列、3600 帧、24/25/30 fps 媒体、149 个字幕/章节/片段标记。

### 自定义输入

```bash
go run ./cmd/rational-timeline -input timeline.json -json report.json -markdown report.md
```

输入字段：

- `global_time_seconds`：精确有理数秒，例如 `"1201/25"`。
- `anchor_kind`：`media`、`clip` 或 `global`。
- `media_offset_frames`：媒体锚点在其媒体速率下的起点帧；报告不输出该内部字段。
- `clip_offset_frames`：片段本地锚点在片段速率下的起点帧；报告不输出该内部字段。
- `nearest_even_tie`：半帧时是否选择偶数帧；普通媒体默认半帧向上，章节/片段默认偶数舍入。
- `role_policy`：`floor`、`nearest` 或 `ceil`，只决定角色优先候选，不覆盖权威锚点。

### 锚定语义

- `media`：权威值是源媒体帧，候选中的 `anchor_coordinate` 转回媒体本地坐标。
- `clip`：权威值是片段本地帧，移动片段会改变全局时间但不改变本地锚点。
- `global`：权威值是全局序列帧，不受片段移动或媒体滑动影响。
- 所有吸附候选都保留精确 `exact_frame`、帧误差、秒误差和反向 `floor/nearest/ceil`。
- `round_trip_twice` 从权威有理数锚点重新计算；两次跨栅格往返后仍等于原帧。

### 验证

```bash
go test ./...
```
