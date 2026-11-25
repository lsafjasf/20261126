# CaptionHub — 节目段 Caption Session

纯 Go 标准库实现，用来解决直播切节目或遇到 discontinuity 后字幕时间轴重置、
观众端仍应用旧 cue 的问题。

核心原则是：**媒体时间不是完整坐标，`(session_id, epoch, media_time)` 才是**。

## 模型

- `Segment`：一个节目段，由 `program_id + segment_id` 标识。
- `Session`：节目段对应的字幕会话，ID 形如 `live-2`。
- `Epoch`：会话内的时间轴世代。
  - 新节目段：创建新的 `Session`，从 `epoch=1` 开始，并 seal 旧 Session。
  - `EXT-X-DISCONTINUITY`：同一 Session 内创建新 Epoch，ID 形如 `live-2#e2`。
- `Cue`：必须携带 `session_id + epoch`，时间是该世代内的本地毫秒时间。
- `Checkpoint`：观众持久化的恢复令牌，包含 session、epoch、模式和播放位置。
- `ConnectionState`：每个连接当前绑定的时间轴、状态、可见 cue 和恢复令牌。

Cue 区间采用 `[start_ms, end_ms)`。迟到观众只会拿到当前位置仍活跃的 cue，
已经结束的 cue 不会补播。

## 场景处理

- **切换节目**：`SwitchProgram` / `BeginSegment` seal 旧会话；live 连接立即离开旧
  时间轴并清空可见字幕。旧 cue 再写入会得到 `ErrStaleSession`。
- **Discontinuity**：`Discontinuity` 增加 epoch；发往旧 epoch 的 cue 返回
  `ErrStaleEpoch`。live 连接等待新时间轴位置，replay 连接仍可固定在旧 epoch。
- **迟到观众**：`ConnectAt(id, ms)` 绑定当前 session/epoch，只输出该位置的活跃 cue。
- **Seek / 回放**：`Seek` 可向后重新显示 cue，也可向前跳过过期 cue。
- **断线恢复**：`RecoverPosition(id, checkpoint)` 精确恢复仍保留的旧 session/epoch；
  未知令牌进入 `resync_required`，随后用 `ResyncToLive(id, currentTime)` 对齐新直播轴。
- **Live 与 Replay 分离**：live checkpoint 指向已切换节目时要求重新对齐直播；
  replay checkpoint 指向保留的旧节目段时可恢复旧字幕。
- **观众端防护**：`Tracker.Accept` 在 UI 侧再次校验 session/epoch，直接丢弃延迟到达的旧 cue。

## 运行

```sh
go test ./...
go test -race ./...

# 确定性场景的人类可读输出
go run ./cmd/captionhub -format table

# 完整 JSON 快照
go run ./cmd/captionhub -format json

# 输出到文件
go run ./cmd/captionhub -format json -out artifacts/caption-states.json

# 启动只读状态服务（内置同一场景）
go run ./cmd/captionhub -serve 127.0.0.1:8080
curl http://127.0.0.1:8080/connections
```

HTTP API：

- `GET /snapshot`：session 与全部连接快照。
- `GET /connections`：按 connection id 排序的可见字幕状态。
- `GET /connections/{id}`：单个连接状态。

## 示例状态

```text
CONNECTION      MODE    STATUS             TIMELINE   TIME    CUE
conn-late       live    awaiting_position  live-2#e2  -       -
conn-live       live    visible            live-2#e2  2000ms  signal-restored: 直播信号已恢复
conn-reconnect  replay  resync_required    -          -       -
conn-replay     replay  visible            live-1#e1  3000ms  news-welcome: 欢迎收看晚间新闻
```

`conn-replay` 仍显示旧节目的 cue 是正确的：它处于回放模式并持有旧 session/epoch
checkpoint。直播连接不会使用该旧 cue。

## Go API 片段

```go
hub := captionhub.NewHub()
show, _ := hub.SwitchProgram("NEWS", "news-1")
_, _ = hub.AddCue(captionhub.Cue{
    ID: "welcome", SessionID: show.SessionID, Epoch: show.Epoch,
    StartMS: 2000, EndMS: 8000, Text: "欢迎收看",
})

viewer, _ := hub.ConnectAt("conn-1", 3000)
// viewer.Status == captionhub.StatusVisible
// viewer.ActiveCue.ID == "welcome"

_, _ = hub.SwitchProgram("DRAMA", "ep-42")
states := hub.States() // 每个连接的权威可见字幕状态
_ = hub.WriteStatesJSON(os.Stdout)
```

## 状态值

- `no_session`：Hub 还没有节目段。
- `awaiting_position`：已绑定时间轴，但播放器尚未上报新位置。
- `idle`：时间轴有效，当前位置没有 cue。
- `visible`：`VisibleCues` 中有活跃 cue。
- `reset`：节目切换或 discontinuity 后已清空旧 cue，等待新位置。
- `resync_required`：checkpoint 无法解析，必须重新对齐直播或重新选择回放位置。

Hub 使用 `sync.RWMutex` 保护 session 和连接表；返回的 cue 切片为复制值，
JSON 快照可安全序列化。旧 session 默认保留以便回放恢复，接入生产时可在节目段
归档策略后调用自己的清理/持久化流程。
