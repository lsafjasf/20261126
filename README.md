# CEA-708 Service Switch / Window Latch / Clear Rules

纯 Go 标准库实现（module `cea708`，**零外部依赖**），修复节目在 service 1 与
service 2 之间切换时“旧窗口短暂叠字”的问题，并提供：

- **service 选择与路由**：DTVCC service block 自动分发到对应 service；
- **窗口锁存**：每个 service 独立保留 8 个窗口与最后一帧，切走冻结、切回还原；
- **清屏规则**：完整区分 `CLW / DSW / HDW / TGW / DLW / RST` 与切换边界清屏；
- **独立屏幕状态**：`map[ServiceID]*Decoder`，互不共享窗口/画布；
- **断线恢复**：恢复点令牌（可含快照）+ 序列号去重 + 从干净 reset 边界重建。

## 叠字根因与修复

典型错误实现把所有 service 的命令写进**同一份窗口数组/同一块帧缓冲**，或在
新 service 到来时把新内容直接叠加在上一帧上。于是 service 1 的窗口字形会在
切到 service 2 后残留若干帧，直到 service 2 自己发 `CLW/DLW/RST` 或覆盖同格。

本实现从根上消除跨 service 状态共享：

1. 每个 `ServiceID` 拥有独立的 `Decoder`（8 个 `Window`、当前窗口、锁存帧）。
2. 每次合成一律在**全新清空的 `Canvas`** 上只绘制当前 service 的可见窗口，
   因此不存在“在上一帧上增量叠加”。
3. `Manager.SelectService` 在同一把锁内原子完成：
   记录 `ClearServiceSwitch` 清屏事件 → 切换选中 service → 在清空画布上合成
   目标 service 的锁存内容并追加一帧 `FrameSwitch`。不发布中间空白帧，
   所以既不叠字也不闪黑。
4. 已呈现的 `Frame` 是不可变值快照；每个非空格子带 `Owner ServiceID`，
   可机械检测一帧内是否混入了其他 service 的字形。

注意区分两类“清除”：切换边界清空的是**合成画布/显示输出**，绝不会清掉旧
service 的窗口缓冲——旧窗口正是靠这份保留状态完成“锁存”，切回时原样还原；
而 `CLW/DLW/RST` 清的是该 service 自己的窗口状态。

## 清屏规则

| 命令 | 操作码 | 掩码/参数 | 语义 |
| --- | --- | --- | --- |
| `CW0..CW7` | `0x80..0x87` | 0 | 选择当前窗口（允许指向尚未定义的窗口） |
| `CLW` | `0x88` | 1 字节窗口掩码 | 擦除窗口文字、笔位回原点；**定义/位置/可见性保留** |
| `DSW` | `0x89` | 掩码 | 显示窗口；被 `HDW` 隐藏的原文仍在 |
| `HDW` | `0x8A` | 掩码 | 隐藏窗口，**文字缓冲保留** |
| `TGW` | `0x8B` | 掩码 | 翻转可见性 |
| `DLW` | `0x8C` | 掩码 | 删除窗口：定义、几何、文字全部销毁 |
| `RST` | `0x8F` | 0 | 复位整个 service：删除全部窗口、笔状态归默认、清锁存 |
| `SPA/SPC/SPL` | `0x90/0x91/0x92` | 2/3/2 | 笔属性/颜色/位置（字节被正确消费以保持对齐） |
| `SWA` | `0x97` | 4 | 窗口属性（消费对齐） |
| `DF0..DF7` | `0x98..0x9F` | 6 | 定义窗口（结构化 `WindowConfig` 为权威模型） |

另外处理 `BS(0x08)`、`CR(0x0D)`、`HCR(0x0E)` 与 `FF(0x0C)`（按“走纸=清当前窗”
的净效果处理）。CEA-708 的正式清屏手段仍是 `CLW/DLW/RST`。

## 协议分层

```
cc_data 三元组 (0xFC..0xFF)
   └─ CCAssembler：cc_type=3 起始 / 2 续传，组装 DTVCC Packet
        └─ DecodePacket：sequence(2bit)+packet_size(6bit)
             └─ ServiceBlock：service_number(3bit)+block_size(5bit)，7 为扩展转义
                  └─ Decoder.ParseCommands：C0/G0/C1/G1 解释到 Window
                       └─ Canvas 合成 → Frame（带 Owner）
```

核心状态模型与协议字节解耦：既可用结构化 API
（`DefineWindow/WriteText/ClearWindows/...`），也可直接喂连线字节
（`IngestPacket` / `IngestCCData`）。解析器对截断命令和未知操作码保持
panic-free：保留已完成前缀，记录首个 `Diagnostic`。

## 网络中断与恢复点重建

每个输入单元是连续序列号的 `Item{Seq, PTS, Packet, Select, Reset}`：

- `RecoverySession.Ingest` 维护连续高水位 `Applied`：
  `Seq <= Applied` 判定为重复并跳过（重放重叠不会重复上字），
  跳号返回 `ErrSequenceGap`（必须重建而不是硬接）。
- `Reset:true` 标记干净的随机访问边界（实际系统中对应 IDR 对齐、
  重发 `RST + DFx + DSW` 的恢复点），记录为 `Boundary`。
- `Checkpoint(snapshot)` 生成可 JSON+base64URL 持久化的 `RecoveryPoint` 令牌，
  快照包含**所有 service** 的窗口/屏幕状态。
- `Rebuild(point, source, target)` 两种重建路径：
  - **快照 + 增量重放**：恢复快照，只重放 `Seq+1..target`；
  - **边界重放**：无快照时新建 `Manager`，从 `Boundary` 干净 reset 点重放。

重建得到的各 service 窗口/屏幕状态与不间断解码结果用 `SameState` 比较一致。

## 快速开始

```sh
go test ./...
go test -race ./...
go test -cover ./...

# 确定性场景：1<->2 切换时间线、清屏规则、断线重建
go run ./cmd/cea708demo
go run ./cmd/cea708demo -format json -out artifacts/demo.json
```

结构化用法：

```go
m := cea708.NewManager(cea708.WithSelectedService(1))
m.Update(1, func(d *cea708.Decoder) {
    d.DefineWindow(cea708.WindowConfig{ID: 0, RowCount: 2, ColCount: 24, Visible: true})
    d.WriteText("hello")
})
sw := m.SelectService(2)        // 原子清屏 + 只合成 service 2
_ = sw
screen, ok := m.LatchedScreen(1) // service 1 状态仍在，可随时切回
_ = screen; _ = ok
```

断线恢复：

```go
cp := session.Checkpoint(true)  // *cea708.RecoveryPoint
token, _ := cp.Token()           // 持久化到播放器/网络侧
rp, _ := cea708.ParseRecoveryPoint(token)
mgr, report, err := cea708.Rebuild(rp, source, 0)
_ = mgr; _ = report; _ = err
```

## 文件地图

- `canvas.go`：带 `Owner` 的字符画布与清空/合成原语。
- `window.go`：单 service 的 8 窗口模型、文字缓冲与窗口级清除/显隐/删除。
- `decoder.go`：每 service 独立的命令状态机与帧合成、锁存。
- `manager.go`：service 注册表、`SelectService`、呈现时间线与清屏审计。
- `switcher.go`：`Switcher = Manager` 领域别名。
- `opcode.go` / `parse.go`：CEA-708 操作码表、长度与字节解释器。
- `packet.go`：DTVCC packet / service block 组帧与 `IngestPacket`。
- `ccdata.go`：`cc_data` 三元组组装与往返编码。
- `snapshot.go`：全 service 状态快照、恢复与 `SameState`。
- `recovery.go`：序列号、日志、恢复点令牌、去重会话与 `Rebuild`。
- `cmd/cea708demo/`：文本/JSON 确定性演示。

## 范围说明

画布采用固定字符网格（默认 32×15，可通过 `WithCanvas` 调整）；`DFx` 的比例
锚点/样式位被归约为单元坐标与保留字段（`EncodeDefineWindow` 与
`defineWindowBytes` 自洽往返），不做像素渲染。这使“跨 service 叠字”和恢复
等价性可以被确定性测试精确验证；接入真实 A/53 ES 时，只需在
`CCAssembler.Push` 之前完成 TS/PES 提取。
