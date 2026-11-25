# subselect · 并发字幕语言选择器

零第三方依赖的 Go 标准库实现，修复“两个浏览器同时切换字幕语言，一个显示新
语言但时间仍是旧的”所暴露的共享状态与时间基复算问题，并覆盖缺轨回退、同语
种多版本选择以及切换 / 暂停 / 恢复验收。

## 症状与根因

现象：浏览器 A、B 同时切换语言后，某个播放器字幕文字已经是新语言，字幕出现
的时间窗口却仍属于旧轨（错位、提前、延后或干脆不显示）。

根因有两个，且会叠加：

1. **选择器共享可变状态**。“当前轨 / 当前语言”被放在包级变量或两个浏览器
   共用的同一个 Selector/Player 上。A 的切换会改写 B 的当前轨，文字切片、
   时间基、选中 ID 实际来自不同会话或不同次切换。
2. **文字与时间基不是一次原子提交，且切换后没有按新轨复算**。每条轨有自己
   的 `TimeBase`（`timescale`，如 `1000 / 48000 / 90000`，外加 edit-list
   偏移）。旧实现只替换了 cue 文本，时间仍按旧轨坐标系解释；或者分两次加锁
   更新“文本表”和“时间表”，并发读恰好落在中间态。异步切轨时，旧请求的慢
   加载还可能在新请求之后回写，形成新语言 + 旧时间。

典型错误形态（本仓库不再包含这种可变共享选择器）：

```go
var currentTrack *Track   // 所有浏览器共享 —— 串会话
var currentCues  []Cue    // 先更新文本……
var timeBase     TimeBase // ……后更新时间基，中间可被读到；且常常忘记更新
```

## 修复设计

- `Catalog` 是一条影片的不可变轨集合，可被所有浏览器 / goroutine 共享；
  `Selector` 是无状态解析器，只返回 `Decision`，不保存“当前选择”。
- `Player` 是**每个浏览器一个**的会话状态（选择代次、播放状态、位置、已提交
  视图），会话之间完全隔离；`Hub` 管理 `sessionID -> *Player`。
- 切换时用所选轨自己的 `TimeBase` 调 `Rebase`，把 cue 统一映射到正片规范时
  间轴 `time.Duration`，生成不可变 `Schedule`。
- 轨身份（`Decision.Track`）、复算后的 `Schedule`、当前位置和活动 cue 组装
  成一个不可变 `View`，在同一把锁内整体替换。观察者要么看到完整旧视图，要么
  看到完整新视图，不可能读到“新文本 + 旧时间”。
- 每次 `Switch` 递增 generation；异步加载完成后只有仍是最新代次才允许提交，
  旧切换返回 `ErrStaleSwitch` 和当前视图，杜绝慢请求回写。
- 选择器给出有序候选链 `Decision.Candidates`：精确 `TrackID` → 语种（精确
  BCP-47 标签优先，主 subtag 兜底，如 `zh-CN`↔`zh`）→ 同语种版本排序 →
  配置的回退语种 → `Default` 轨 → ID 最前的可用轨 → 关闭。加载失败（如 404）
  沿候选链继续回退并记录 `LoadErrors`，全部失败才提交显式 off 视图。
- 同语种多版本用 `Track.Version`、`TrackID`、`Kind`、`Forced`、
  `HearingImpaired`（SDH）、`Default`、`Ranking` 区分；只给语种时按上述维
  度确定性排序并在 `Decision.Variants` 暴露全部版本，ID 作为最终平局项。

## 包结构

- `timebase.go`：有理数安全的时间基换算 / 反算。
- `track.go`、`catalog.go`：轨、cue、选择条件与不可变目录。
- `select.go`：语种匹配、多版本排序、回退链与 `Decision`。
- `schedule.go`：`Rebase` 与半开区间 `[Start,End)` 的活动 cue 查找。
- `player.go`：单浏览器会话、原子 `View`、generation 防过期、播放 / 暂停 /
  恢复 / Seek。
- `clock.go`：`Clock`、`SystemClock` 与确定性的 `FakeClock`。
- `hub.go`：多会话注册表。
- `httpapi/`：HTTP/JSON 接口，用于真正的双 HTTP 客户端并发验收。
- `cmd/subdemo/`：可运行演示，也可用 `-serve` 启动 API。

## Go API

```go
cat, _ := subselect.NewCatalog(tracks...)
sel := subselect.NewSelector(cat, subselect.SelectorConfig{
    FallbackLanguages: []string{"en"}, // 缺轨时的回退语种
})

browserA := subselect.NewPlayer("browser-A", sel,
    subselect.WithClock(subselect.NewFakeClock(time.Now())))

res, err := browserA.Switch(ctx, subselect.Criteria{
    Language: "zh",
    Version:  "director-cut", // 同语种多版本
})
_ = err
fmt.Println(res.View.Cue.Text, res.View.Cue.Start, res.View.Cue.End)

browserA.Pause()
browserA.Resume()
view := browserA.Snapshot() // 一致的时点快照（位置 + 活动 cue）
```

## HTTP 接口（两个浏览器验收）

```sh
go run ./cmd/subdemo -serve 127.0.0.1:8080

A=$(curl -s -X POST localhost:8080/api/sessions | jq -r .session_id)
B=$(curl -s -X POST localhost:8080/api/sessions | jq -r .session_id)

curl -s -X POST localhost:8080/api/sessions/$A/switch \
  -d '{"language":"zh","version":"director-cut"}'
curl -s -X POST localhost:8080/api/sessions/$B/switch \
  -d '{"language":"en","hearing_impaired":"yes"}'

curl -s -X POST localhost:8080/api/sessions/$A/pause
curl -s -X POST localhost:8080/api/sessions/$A/resume
curl -s localhost:8080/api/sessions/$A
```

接口：`POST /api/sessions`、`GET /api/sessions/{id}`、
`POST /api/sessions/{id}/switch|pause|resume|play|seek`。响应中的
`cue.start_ms/end_ms` 始终是用当前轨时间基复算后的正片时间。

## 验收与测试

```sh
go test -race -count=1 ./...
go test -race -run Acceptance ./...
go run ./cmd/subdemo
```

验收用例与题意的对应：

- `TestAcceptance_TwoBrowsers_ConcurrentLanguageSwitch`：两个独立
  `Player` 共享同一 `Catalog/Selector`，各 150 轮并发切换 + 并发读快照；
  断言会话不串、语种不串，且每条 cue 的窗口都能用“所选轨自己的 TimeBase +
  源 cue”独立复算一致（专门抓“新语言 + 旧时间”）。
- `httpapi.TestHTTPAcceptance_TwoBrowsersSwitchConcurrently`：两个真实
  HTTP 客户端 / `httptest.Server` 并发切换并轮询状态，做同样的独立复算。
- `TestAcceptance_SwitchWhilePaused_ShowsNewLanguageWithRebasedTiming`：
  暂停在 5s 时中→英，必须立即显示 90 kHz 复算后的 `4.5s..7.5s` 英文 cue；
  暂停一小时位置不漂移。
- `TestAcceptance_PauseAndResume_PreservesSelectionAndProgress`：
  播放→暂停→带偏移版本切换→长暂停→恢复→再暂停→切语种→再恢复，校验选择、
  时间基、位置累计（暂停时长不计入）全程正确。
- `TestAcceptance_SwitchWhilePlaying_KeepsCanonicalPosition`：播放中切换
  不重置时间轴，新 schedule 立即生效。
- `TestPlayer_StaleAsyncSwitchCannotOverwriteNewerSelection`：用门控
  Loader 确定性复现“旧请求晚回写”，断言 generation 保护丢弃旧结果。
- `TestPlayer_LoadFailureFallsBackAlongChain`、
  `TestPlayer_AllCandidatesFail_CommitsExplicitOff`、
  `TestSelect_MissingLanguageFallbackChain`：缺轨 / 加载失败回退。
- `TestSelect_SameLanguageMultipleVersions`、
  `TestSelect_HearingImpairedPreferencePicksSDH`：同语种多版本、SDH、
  确定性平局选择。

测试使用 `FakeClock` 和门控加载器，不依赖真实 sleep，可稳定重复运行；
上述并发用例已用 `-race -count=20` 重复验证。
