# mediarelink

工程迁移后，12 个媒体资产中有 10 个按记录路径找不到：2 个其实还在、只是扩展名变了；3 个被"同名不同内容"的文件占位；其余或改名、或移动、或真的丢失。`mediarelink` 只凭 **时长、编码、帧哈希、音频指纹** 四个内容信号重关联，文件名永不参与身份判定；置信度不足一律阻断，绝不自动落地。全程仅使用 Go 标准库。

## 身份信号与权重

| 维度 | 权重 | 说明 |
| --- | --- | --- |
| frame_hash | 0.40 | 帧载荷的 SHA-256，内容身份，精确匹配得 1 |
| audio_fingerprint | 0.25 | 音频 64 位 SimHash 的汉明相似度 |
| duration | 0.20 | 精确 / ±40ms / 指数衰减 |
| codec | 0.15 | 精确匹配得 1 |

置信度 = 各维度加权平均。探测靠嗅探文件魔数（MRL1 容器），与扩展名无关，所以"扩展名变化"的文件天然可识别。

## 判定规则

- `relinked`：最佳候选置信度 ≥ 0.85，且领先次优 ≥ 0.10。
- `blocked`：以下任一情况，必须人工处理，绝不自动落地：
  - `no_candidate`：没有任何候选达到 0.45；
  - `low_confidence`：最佳候选在 0.45–0.85 之间（如重编码：帧哈希不同但音频/时长/编码一致，得 0.60）；
  - `ambiguous_candidates`：前两名差距不足 0.10；
  - `path_claimed_by_multiple_assets`：一个文件被多个资产同时认领。
- 同名诱饵（decoy）：帧哈希不符且音频相似度 < 0.90 的候选被标记 `REJECTED` 并留在报告中作为证据，绝不因同名而胜出。

## 阻断语义

- `check` 只出报告，退出码恒 0。
- `apply` 默认原子：只要还有一个资产 blocked，就什么都不写，退出码 2。
- `apply -allow-partial` 只落地高置信度的重关联，blocked 资产保留原记录路径，退出码 0。

## 可追溯性

- 效果（effect）只按 `media_id` 引用媒体；重关联只改写资产的 `path` 字段，`media_id` 与效果引用逐字保留。
- `apply` 落盘后自动复核：原媒体 ID 全集仍在、每条效果都能解析到媒体、每个重关联目标重新探测的帧哈希与工程记录一致；任一失败即报错。
- `relink-report.json` / `.txt` 记录每个资产的旧路径、新路径、置信度、逐维度证据、被拒诱饵与其引用的效果 ID。

## 目录

- `internal/media/` — MRL1 容器探测、帧哈希、音频 SimHash、目录扫描。
- `internal/match/` — 四维证据打分、诱饵识别、阻断判定。
- `internal/project/` — 工程模型、缺失检测、落盘与可追溯复核。
- `internal/report/` — JSON / 文本报告。
- `cmd/mediarelink/` — CLI（`check` / `apply`）。
- `testdata/gen/` — 确定性素材生成器（xorshift PRNG，幂等）。
- `testdata/` — 生成的工程、媒体树与输出样例。

## 素材场景矩阵（testdata）

| 资产 | 场景 | 判定 |
| --- | --- | --- |
| m01 intro / m02 logo | 扩展名变化（.mov→.mp4/.mkv），内容不变 | relinked (1.00) |
| m03 interview_a / m04 broll_city | 同名诱饵占位，真内容已迁移 | relinked，诱饵 REJECTED |
| m05 music_bed | 只剩同名诱饵，真内容已丢失 | blocked (no_candidate) |
| m06 product_shot | 移到子目录，内容不变 | relinked |
| m07 ambient_park / m10 extra_scene | 改名，内容不变 | relinked |
| m08 ending_logo | 重编码：帧哈希不同，音频/时长/编码一致 | blocked (low_confidence, 0.60) |
| m09 backup_take | 彻底丢失 | blocked (no_candidate) |
| m11 / m12 | 未受影响 | present，不参与匹配 |

## 使用

```bash
# 重新生成确定性素材（幂等）
go run ./testdata/gen

# 干跑：只出报告
go run ./cmd/mediarelink check \
  -project testdata/project.json \
  -media-root testdata/media \
  -out-dir testdata/output

# 落地：有 blocked 资产时整体阻断（退出码 2，什么都不写）
go run ./cmd/mediarelink apply \
  -project testdata/project.json \
  -media-root testdata/media \
  -out-dir testdata/output

# 显式允许部分落地
go run ./cmd/mediarelink apply \
  -project testdata/project.json \
  -media-root testdata/media \
  -out-dir testdata/output \
  -allow-partial
```

## 测试

```bash
go vet ./...
go test ./...
```
