# medref

媒体路径引用管理工具，只用 Go 标准库。它解决三类问题：

1. **规范化引用**：同一条引用可能以盘符路径（`D:\...`）、UNC 路径
   （`\\server\share\...`）、POSIX 绝对路径、相对路径出现，还可能带
   Unicode 名称（含 NFC/NFD 两种拼写、韩文 Jamo）和不同大小写。
2. **保留原始字符串**：清单里逐字保存每个原始拼写（`aliases`），规范化
   结果只用于生成稳定的 `canonical` 键和合并判断，永不回写用户输入。
3. **整体迁移**：工程移动后批量重定位相对引用；外部卷通过映射重定位，
   缺失项与未映射卷分别列出，绝不猜测路径。

## 构建与测试

```bash
go build ./...
go test ./...
bash scripts/smoke.sh   # 端到端冒烟（在 mktemp 目录中运行）
```

## 命令

```bash
medref init     -root DIR [-platform linux|windows|darwin] [-m medref.json]
medref add      [-m medref.json] [-from refs.txt | PATH ...]
medref map      -volume 'D:' -target /mnt/d [-m medref.json]
medref status   [-m medref.json] [-json]
medref recheck  [-m medref.json] [-json]
medref relocate -root NEWDIR [-m medref.json] [-json]
```

## 规范化规则

- 相对路径按词法做 `path.Clean`（解析 `.`/`..`），但保留相对形态，键前缀
  `rel://`，例如 `./media/../a.png` 与 `media/a.png` 同键。
- 盘符路径同时接受 `/` 和 `\` 分隔符，盘符统一大写，卷内按
  Windows 语义折叠 ASCII 大小写，键前缀 `drive://D/`。
- UNC 路径的 server/share 折叠大小写，键前缀 `unc://server/share/`。
- POSIX 绝对路径保留原样，键前缀 `posix://`。

## 哪些路径会合并，哪些不会

合并只在以下两种情况下发生：

- **词法同键**：依据平台大小写策略。Windows 卷（盘符/UNC）始终大小写不
  敏感；相对路径在 `windows`/`darwin` 平台折叠大小写，在 `linux` 上不
  折叠；POSIX 绝对路径仅在 `darwin` 上折叠。
- **磁盘同一文件**：磁盘探测把引用解析到同一个已存在的绝对路径（含符号
  链接解析）。找不到字面拼写时，逐级目录回退到松散匹配：ASCII 大小写、
  NFC/NFD 重音（组合记号按 NFC 组合，未知记号保留）、韩文音节/Jamo。

因此在 Linux 上两个真实存在、仅大小写不同的文件（`a.mov` 与 `a.MOV`）
会保持为两条独立引用；Unicode 的不同规范化拼写在词法层也保持独立键，只
有在磁盘上确实指向同一文件时才合并。非 ASCII 字符不做大小写折叠，避免
把合法但不同的路径错误合并。

## 迁移语义

- `relocate -root NEWDIR` 把工程根改到新目录，相对引用基于新根重新解析；
  原始相对字符串保持不变。
- 盘符/UNC 引用需要先用 `map` 登记卷在本机的落点；未登记的卷进入报告的
  `unmapped`，不会被猜测改写。
- 新位置找不到的文件进入报告的 `missing`，并在清单中标 `"missing": true`，
  可用 `recheck` 随时重新探测。
