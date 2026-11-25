# 20261126

Repository initialized for 20261126.

## trackselect

纯 Go 标准库实现的字幕/音轨选择引擎，覆盖普通字幕、SDH、强制叙述轨与解说轨。

- `track.go` — 轨道模型、场景（常规/嘈杂/视障/语言学习）与场景化偏好。
- `selector.go` — 优先级：偏好语言顺序 > 场景类型权重 > 容器默认标记 > 回退链；音轨与字幕语言不一致时自动叠加同语言强制叙述轨。
- `validate.go` — 静态检测：同语言双默认强制轨冲突、默认轨回退链循环引用（三色 DFS）、悬空回退、偏好语种缺失。
- `player.go` — 播放器内核：切换轨道只更新选择不动时钟，播放位置天然连续，并记录切换历史。

```go
tracks := loadTracks()
prefs := trackselect.DefaultPreferences(trackselect.ScenarioNoisy)
for _, issue := range trackselect.Validate(tracks, prefs) {
    log.Println(issue.Kind, issue.Message)
}
p := trackselect.NewPlayer(trackselect.Select(tracks, prefs))
p.Advance(90 * time.Second)
p.Switch(tracks, trackselect.DefaultPreferences(trackselect.ScenarioImpaired)) // 位置不变
```

运行测试：`go test ./...`
