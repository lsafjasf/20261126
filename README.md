# 20261126

Repository initialized for 20261126.

## audio: 设备能力 ↔ 流参数协商状态机（纯 Go）

解决「设备从 44.1 kHz 切到 48 kHz，播放器保留旧声道布局产生爆音」的问题：

- `audio/types.go` — 采样率 / 声道布局 / 采样格式与设备能力模型
- `audio/negotiate.go` — 协商逻辑：精确匹配优先，部分支持时按维度回退并记录 `Fallback`
- `audio/engine.go` — 状态机：`running` / `degraded`（部分支持）/ `device_lost`（消失，位置冻结）/ 恢复后自动重协商
- `audio/position.go` — 样本位置：以源采样率为规范时间线，有理数累加器无漂移换算，重协商点记录 `Anchor`
- `audio/convert.go` — 渲染侧修复：按协商结果做声道重映射（5.1→stereo 等）与重采样，不再把旧布局推给设备
- `cmd/demo` — 端到端复现：44.1→48 kHz 切换、拔插恢复、锚点记录

```sh
go test ./...   # 含 -race 通过
go run ./cmd/demo
```
