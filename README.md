# vorbiscomment

面向 Vorbis comment 迁移的宽松读取、可定位诊断与保守重写工具。整个仓库只有一个 Go module，无第三方依赖。

## 行为保证

- 解析阶段不会因为单条 comment 的重复键、非法 UTF-8 或超长值而拒绝整首曲目。
- 所有条目按原始顺序保留，不建立 `map[key]value`，因此重复键和未知键都不会丢失。
- 每条诊断都带字节偏移、长度、条目序号和键名。嵌入 Ogg 时偏移会映射到文件位置。
- 重写直接使用解析时的原始字节。非法 UTF-8、超长值、未知键、键名大小写和重复项均不被猜测性修改。
- 默认 `safe` 模式只做无损结构修复：删除零填充、补回 packet framing bit。`conservative` 模式保留这些原始结构字节。
- vendor、条目数或 comment 长度发生截断时 `Complete=false`：仍可检查定位，但拒绝伪造一次完整重写。

## 诊断分级

| 情况 | 分级 | 处理 |
| --- | --- | --- |
| 重复键 | `repairable` | 保留全部条目；值不同时额外报告冲突 |
| 重复键且值不同 | `manual_confirmation` | 不自动选择保留哪个值 |
| 非法 UTF-8 | `manual_confirmation` | 报告首个非法字节，原值完全保留 |
| 值超过 `MaxValueBytes` | `manual_confirmation` | 报告完整范围，绝不截断 |
| 空键、非法键名、缺少 `=` | `manual_confirmation` | 原条目保留，需人工决定 |
| 零填充 | `repairable` | `safe` 模式可删除 |
| packet 缺少 framing bit | `repairable` | `safe` 模式可补回 |
| 未知键 | 无诊断 | 原样保留 |

“必须人工确认”不会阻止文件写出，因为工具会继续保留有问题的原始字节；它表示工具不会擅自更改该值。

## 库用法

```go
package main

import (
	"fmt"

	vorbiscomment "github.com/lsafjasf/20261126"
)

func inspect(packet []byte) error {
	comment, err := vorbiscomment.Parse(packet, vorbiscomment.Options{
		BaseOffset:    1024,
		MaxValueBytes: 64 * 1024,
	})
	if err != nil {
		return err
	}

	fmt.Println(comment.VendorString)
	for _, entry := range comment.Entries {
		fmt.Printf("%s=%q @%d\n", entry.Key, entry.Value, entry.Offset)
	}
	for _, issue := range comment.ManualIssues() {
		fmt.Println(issue)
	}

	_, err = comment.Marshal(vorbiscomment.RewriteOptions{
		Mode: vorbiscomment.RewriteSafe,
	})
	return err
}
```

`Parse` 同时接受裸 comment body 和带 `0x03vorbis` 头的完整 packet；`ParseBody` 可强制按裸 body 解析。

## 命令行

```sh
go run ./cmd/vorbis-migrate -json song.ogg
go run ./cmd/vorbis-migrate -write song.ogg
go run ./cmd/vorbis-migrate -raw -o fixed.bin comment.bin
```

常用参数：

- `-write`：通过同目录临时文件原子替换输入文件。
- `-o PATH`：写出到指定路径。
- `-mode safe|conservative`：默认 `safe`。
- `-max-value-bytes N`：值长度策略上限，默认 64 KiB。
- `-raw`：输入是裸 comment body/packet，而不是 Ogg 文件。
- `-json`：输出机器可读诊断。
- `-fail-on-manual`：仍有 `manual_confirmation` 时以状态码 2 退出。

默认只检查并输出报告，不修改文件。

## Ogg 安全性

Ogg 路径会重建 ident/comment/setup 头页并重算页 CRC，只更新受影响 logical stream 的页序号；音频 packet 字节保持不变。若某个音频 packet 与 header 共用页，无法无歧义迁移时工具会报 `unsupported page layout`，而不是写出可能损坏的文件。

## 验证

```sh
go test ./...
go vet ./...
```
