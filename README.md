# 无障碍音频描述排程器

用 Go 编写的命令行工具。它把字幕、对白占用、音效保留区和已存在的音频描述合并为同一组时间约束，再为每条描述选择长版/短版及具体落点。发生冲突时，输出会说明改用了哪个版本、舍弃了哪些描述片段，以及冲突来自哪条对白、音效或已排描述。

## 运行

需要 Go 1.23 或更新版本。

```bash
go run ./cmd/audiodesc -input examples/plan.json -format text
go run ./cmd/audiodesc -input examples/plan.json -output schedule.json
go test ./...
```

参数：

- `-input`：输入 JSON 路径，默认 `-`（标准输入）。
- `-output`：输出路径，默认 `-`（标准输出）。
- `-format`：`json`（默认）或 `text`。

## 输入

所有时间均为毫秒，区间采用左闭右开语义：描述结束时刻等于对白开始时刻不算重叠。

```json
{
  "timeline": {
    "duration_ms": 30000,
    "dialogue": [
      {"id": "d1", "start_ms": 0, "end_ms": 3000, "speaker": "林", "text": "你听见了吗？"}
    ],
    "sound_effects": [
      {"id": "fx1", "start_ms": 6000, "end_ms": 8500, "text": "门轴吱呀声"}
    ],
    "subtitles": [
      {"id": "sub1", "start_ms": 13000, "end_ms": 14000, "text": "[远处雷声]"}
    ],
    "existing_audio_descriptions": [
      {"id": "ad-old", "start_ms": 11000, "end_ms": 12000, "text": "走廊亮起灯"}
    ]
  },
  "cues": [
    {
      "id": "ad-001",
      "anchor_ms": 7000,
      "priority": 10,
      "window": {"start_ms": 0, "end_ms": 12000},
      "long": {
        "segments": [
          {"id": "a", "text": "他握紧扶手", "duration_ms": 2500},
          {"id": "b", "text": "车窗外掠过路灯", "duration_ms": 2500}
        ]
      },
      "short": {
        "segments": [
          {"id": "a", "text": "他握紧扶手", "duration_ms": 2000}
        ]
      }
    }
  ],
  "options": {"min_clearance_ms": 250}
}
```

字段说明：

- `anchor_ms`：描述所指画面的时间点，用于在多个可行落点中选最近的。
- `window`：允许落点范围；省略时默认取 `anchor_ms` 前 3 秒到后 8 秒，并裁剪到 `duration_ms`。
- `priority`：数字越大越先排程，默认 0。
- `long` / `short`：至少提供一个版本。`duration_ms` 可省略，此时由 `segments[].duration_ms` 求和。
- `segments[].id`：长短版之间用 ID 对齐内容；短版中没有的长版片段会进入 `dropped_info`。
- `min_clearance_ms`：每个约束前后额外保留的最小空隙，默认 0。

## 排程规则

1. 将四类固定区间合并为一个按时间排序的约束集合，再加入本次已经排入的音频描述。
2. 按 `priority` 降序、`anchor_ms` 升序、`id` 升序处理描述，保证结果确定。
3. 对每条描述先在窗口内寻找能容纳长版的空闲段；没有时再尝试短版。
4. 同一版本有多个落点时，优先覆盖锚点，其次让落点中心靠近锚点；距离相同时优先锚点之后，再取更早的落点。
5. 排入的描述立即成为后续描述的 `audio_description` 约束，避免音频描述彼此重叠。
6. 长版放不下但短版可放下：使用短版，按片段 ID（缺省时按文本）差集列出舍弃内容。
7. 两个版本都放不下：整条舍弃，列出全部已知片段、可用的最大空隙和冲突约束。

这是一个按优先级进行的贪心排程器，而不是全局最优求解器。优先级高的描述会先占用时间；如果有全局最优需求，可先调整 `priority`。

## 输出

JSON 输出包含：

- `placements`：已排描述，含 `variant`、`start_ms`、`end_ms`、`dropped_info`、`conflicts` 和中文 `explanation`。
- `omitted`：无法排入的描述，含舍弃原因及全部被舍弃内容。
- `constraints`：最终合并的约束集合，包含字幕、对白、音效、已有及本次排入的音频描述。
- `summary`：长版、短版、舍弃数量及各类约束数量。

`explanation` 面向人工复核；程序化处理应读取结构化的 `dropped_info`、`conflicts`、`reason` 字段。
