# EDL Framebase

零第三方依赖的 Go 原生 EDL 时间基检测与修复工具。仓库初始状态没有实际样例，因此使用版本化 JSON 投影表达 EDL 的 `source list` 与事件表；核心类型和算法不依赖 CLI，可直接接入现有导入器。

## 检测范围

- 卷级冲突：同一 `reelId` 的 `reels.sourceListRate` 与 `reels.eventTableRate` 不一致。
- Source list 内部冲突：同一卷或同一 source 的帧率声明互相矛盾。
- 事件级冲突：事件行显式 `rate`，或从事件表继承的帧率，与该卷统一时间基不一致。
- 全量处理：报告和修复遍历所有事件，不会在第一条冲突后停止。
- 时间码校验：支持 `24`、`25`、`30`、`29.97DF`、`30000/1001DF` 等声明及 drop-frame 换算。
- 跨午夜：`crossesMidnight: true` 时给 out 增加一个时间码日；未显式声明但从 `23` 点段落到 `00` 点段落时，也会窄规则推断为午夜回绕。
- 非法负时长：普通 `out < in` 且没有午夜回绕依据时标记 `negative_duration`，不做自动转换。

## 统一方案

默认以每卷 source list 的帧率作为权威时间基（样例为 `25fps`），并：

1. 将卷的 `sourceListRate`、`eventTableRate` 和所有可安全处理事件的 `rate` 统一为 canonical rate。
2. 用精确有理数按物理时长换算：
   `canonicalFrames = round(sourceFrames * canonicalRate / declaredRate)`。
3. 分别换算起点和时长，再令 `end = start + duration`，避免起点、终点独立四舍五入后产生反向区间。
4. 输出每个事件的 declared/canonical `[start,end)`、包含端点、帧数和 `affectedFrames`。
5. 负时长等非法行保留原始时间码，并显式写回其旧帧率，避免卷默认值修改后改变该行含义；这些行进入 manual review。

也可用 `-prefer event-table` 或 `-target 30` 改变统一目标。

## 运行样例

```bash
go test ./...
go build -o bin/edlframe ./cmd/edlframe
./bin/edlframe check -input examples/sample.json
./bin/edlframe check -input examples/sample.json -json
./bin/edlframe fix -input examples/sample.json -out fixed.json -report report.json
```

样例故意保留一条负时长事件，因此编译后的 `check` 和 `fix` 会以状态码 `2` 结束，表示仍有 findings/manual review；运行时错误使用状态码 `1`。直接使用 `go run` 时，Go 工具可能把子进程的状态码 `2` 包装并输出为 `exit status 2`。

## 样例影响帧

帧率冲突为 `24fps -> 25fps`。帧编号是从 `00:00:00:00` 起的线性帧号，跨午夜的 out 含一个日偏移。

| 事件 | 声明区间 | 时长 | 统一后区间 | 影响的 canonical 帧 |
| --- | ---: | ---: | ---: | --- |
| `001` | `[240,288)` | 48 | `[250,300)` / 50 帧 | `250..299` |
| `002` 跨午夜 | `[2073552,2073600)` | 48 | `[2159950,2160000)` / 50 帧 | `2159950..2159999` |
| `003` 继承事件表 | `[86400,86424)` | 24 | `[90000,90025)` / 25 帧 | `90000..90024` |
| `004` 负时长 | `[480,456)` | -24 | 不自动修复 | 人工处理 |

三个可转换事件的 source 帧合计为 `50 + 50 + 25 = 125`。`recordIn/recordOut` 也会独立计算同样的映射。

## Go API

```go
doc := edlframe.Document{ /* load JSON or map your EDL parser into this model */ }
report, err := edlframe.Analyze(doc, edlframe.UnifyOptions{ExpandFrames: true})
fixed, report, err := edlframe.Unify(doc, edlframe.UnifyOptions{})
_ = fixed
_ = report
_ = err
```

输入字段见 `model.go`，可直接 `json.Unmarshal`。如果已有 CMX/AVE 等 EDL 解析器，只需把事件映射到 `Document.Events`，把两张表的帧率声明映射到 `Document.Reels`。
