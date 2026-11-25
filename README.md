# edlrelink

离线剪辑师基于旧版工程带回一份变更清单（EDL），而在线工程已经演进：
片段 ID 可能被复用、文件可能改名或移动，其中两段素材已被删除。
`edlrelink` 按 **片段 ID + 媒体内容哈希（SHA-256）** 双键重新定位每条
离线修改，并把结果分成互不重叠的三类。文件名字永远不充当素材身份。

仅使用 Go 标准库（`crypto/sha256`、`encoding/json`、`flag`、`os` 等），
无第三方依赖。

## 身份规则

- 素材身份 = `(clip_id, media_hash)`；`media_hash` 是媒体字节的 SHA-256。
- 文件路径/文件名只作为证据展示，不参与自动匹配。
- 可选 `-media-root` 对在线清单中的每条素材重新落盘算哈希，清单与磁盘
  不一致时直接报错中止，杜绝按篡改后的清单做决策。

## 三类判定

- `applied`（成功修改）：内容哈希在在线库中唯一命中，且不存在冲突维度。
  - ID 与哈希同时命中（`id_and_hash_match`），或内容迁移到新 ID 但没有
    任何 ID/同名冲突（`hash_relocated_to_new_id`）。
- `pending`（待确认）：机器能给出建议目标，但必须人工拍板。
  - 旧 ID 已被不同内容复用（ID 冲突）；
  - 同名文件被无关内容占用（文件名碰撞，按文件名会选错）；
  - 同一内容哈希在在线库出现多次（`hash_present_on_multiple_assets`）；
  - 离线清单自身的 ID/哈希对与旧库不一致。
- `dead`（已失效）：离线修改引用的内容哈希在在线库彻底不存在。
  即使存在同文件名或同 ID 的“替身”，也只列为证据，绝不自动匹配。

`apply` 命令产出的在线变更清单只包含 `applied` 项；`pending` 与 `dead`
永远不会被自动落地。

## 目录

- `internal/edl/` — 类型、JSON 读取、落盘哈希校验、三键索引与分类解析、
  文本/JSON 报告渲染。
- `cmd/edlrelink/` — CLI（`check` / `apply`）。
- `testdata/gen/` — 确定性素材生成器，产出两个媒体库、离线变更清单和
  真实的占位媒体文件（哈希按文件实际字节计算）。
- `testdata/` — 生成的库 JSON、媒体树与输出样例。

## 素材场景矩阵（testdata）

| 离线 op | 场景 | 在线库情况 | 分类 |
| --- | --- | --- | --- |
| seq1 c01 | ID 与哈希都未变 | 原样存在 | applied |
| seq2 c02 | 内容迁移到 c12，旧 ID c02 被新内容占用 | ID 复用冲突 | pending |
| seq3 c03 | 内容改名迁移到 c13；c13b 占用旧文件名且内容不同 | 文件名碰撞 | pending |
| seq4 c04 | 内容迁移到 c14，无任何冲突 | 纯哈希重定位 | applied |
| seq5 c05 | 素材被删除，哈希消失 | 无替身 | dead |
| seq6 c06 | 同一内容在 c16a/c16b 各有一份 | 哈希重复 | pending |
| seq7 c07 | 素材被删除，但 c17 保留同名文件（内容不同） | 同名陷阱 | dead |
| seq8 c08 | ID 与哈希都未变 | 原样存在 | applied |
| seq9 | 离线记录自身 ID/哈希错配 | 旧库校验不过 | pending |

## 使用

```bash
# 生成确定性测试素材（幂等，可重复运行）
go run ./testdata/gen

# 分类并输出报告；-media-root 同时做落盘哈希校验
go run ./cmd/edlrelink check \
  -offline testdata/offline_library.json \
  -online  testdata/online_library.json \
  -changelist testdata/offline_changelist.json \
  -media-root testdata/online_media \
  -out-dir testdata/output

# 在分类基础上额外生成仅含 applied 项的在线变更清单
go run ./cmd/edlrelink apply \
  -offline testdata/offline_library.json \
  -online  testdata/online_library.json \
  -changelist testdata/offline_changelist.json \
  -media-root testdata/online_media \
  -out-dir testdata/output
```

输出：

- `relocation-report.json` — 机器可读的三类完整报告，含每条 op 的候选、
  命中维度（id/hash/name）、建议目标与告警。
- `relocation-report.txt` — 人类审阅版。
- `applied-changelist.json` — 仅 `apply` 生成，可导入在线工程的变更清单，
  编辑参数（入出点、轨道、备注）原样保留，只替换身份引用。

## 测试

```bash
go vet ./...
go test ./...
```
