# 音频轨道迁移与整单校验

这个 Go 模块把剪辑表里的普通音频轨恢复为影片需要的语义轨道：
对白、音乐、音效和多语配音轨。迁移结果统一保存语言、类型、默认轨和
可听/静音状态，并在落库前校验同一语言的默认音轨是否唯一。

## 运行

```sh
go run ./cmd/audio-migrate -out canonical.json testdata/film.json
go run ./cmd/audio-migrate -check testdata/film.json
go test ./...
```

不传 `-out` 时，规范化 JSON 输出到标准输出。输入非法时命令退出码为
`1`，不会创建或覆盖目标文件；参数或 I/O 错误退出码为 `2`。

## 输入映射

剪辑表导出器常把所有轨道写成 `role: "audio"`。迁移器不会把这个容器
类型直接当轨道类型，而是按以下顺序恢复：

1. 使用 `role` / `trackRole` 中的明确语义；
2. 使用 `type` / `mediaType` / `kind` 中的明确语义；
3. 两者都是普通音频时，从轨道名或片段名识别 `dialogue`、`music`、
   `effects`、`dub`（支持 `DX`、`MX`、`SFX`、`BGM`、`对白`、`配乐`、
   `音效`、`配音` 等常见写法）；
4. 仍无法唯一识别时整单拒绝，绝不猜成普通音频后静默保存。

迁移器同时接受 camelCase、snake_case 和 kebab-case，例如
`projectId` / `project_id`、`languageCode` / `language`、
`isDefault` / `default`、`isMuted` / `muted` / `is_muted`。

| 规范字段 | 含义 |
| --- | --- |
| `language` | 规范化语言标签，如 `zh-CN`、`en-US`、`ja-JP` |
| `type` | `dialogue`、`music`、`effects` 或 `dub` |
| `default` | 是否为该语言的默认音轨 |
| `audible` | `true` 可听，`false` 静音；`Track.Muted()` 返回其反值 |

对白和配音必须使用具体语言；音乐和音效缺省语言时映射为 `und`。旧输入
中的 `muted` 会转换为 `audible = !muted`。如果同时给出 `audible` 和
`muted`，两者必须一致，否则整单拒绝。

## 校验规则

- 轨道 ID 非空且项目内唯一；
- 语言必须可解析并规范化为统一形式；
- 类型必须是四种语义类型之一；
- 同一语言最多一个 `default: true`，比较发生在语言规范化之后，
  因此 `en_US` 与 `EN-us` 视为同一语言；
- 默认轨不能同时静音；
- 对白、配音不能使用 `und` / `zxx`；
- 所有问题汇总到 `ValidationError`，但不保存任何一条轨道。

`SaveService` 只有在整份项目通过校验后才调用一次仓储 `Save`。
`MemoryRepository` 在锁内整体替换项目，`SaveProjectJSONAtomic` 通过
同目录临时文件加 `rename` 提交，因此失败时旧项目或旧文件保持不变。

## 代码接口

```go
project, err := audiotracks.MigrateJSON(input)
if err != nil {
    // errors.As(err, &validation)，可读取全部 Problem。
}

service := audiotracks.NewSaveService(repo)
if err := service.Save(ctx, project); err != nil {
    // 校验失败时 repo.Save 不会被调用。
}
```

`testdata/invalid-duplicate-default.json` 演示了 `en_US` 与 `EN-us`
各有一条默认轨的非法组合；`cmd/audio-migrate` 会返回非零退出码且不
改写输出文件。
