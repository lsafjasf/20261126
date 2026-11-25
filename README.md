# 20261126

媒体锚定的事件映射（纯 Go，仅标准库）。

重剪影片后，章节、强制字幕和普通字幕不再引用旧的绝对时间：事件以「源媒体
区间」为锚点持久化，时间线位置由 EDL 重新推导；被切口分开的区间拆成独立
片段，无法自动映射时进入人工选择，结果保存后重开完全稳定。

## 快速上手

```bash
go run ./cmd/mediaanchor demo ./demo      # 生成示例工程并演示完整流程
go run ./cmd/mediaanchor map demo/demo-initial.json
go run ./cmd/mediaanchor unresolved demo/demo-initial.json
```

## 命令

- `validate <project.json>`：结构与区间校验。
- `map <project.json>`：输出全部片段映射、未决项、过期决策与工程校验和。
- `unresolved <project.json>`：列出需要人工处理的片段及候选目标。
- `resolve <project.json> --key <piece-key> (--segment <id> | --drop | --start <t> --end <t>)`：
  人工裁决一个歧义或被删除片段。
- `anchor <project.json> --event <id> --media <id> --start <t> --end <t>`：
  为无锚点的遗留事件人工指定源媒体区间。
- `unresolve <project.json> (--key <piece-key> | --anchor-event <id>)`：撤销人工决策。
- `checksum <project.json>`：规范化 JSON 的 SHA-256，用于重开稳定性核对。

时间默认为整数 ticks；也支持 `12s`、`250ms`、`1500us` 与 `HH:MM:SS.mmm`，
按工程的 `ticks_per_second` 精确换算（亚 tick 精度会报错而非四舍五入）。

设计细节见 `docs/design.md`。
