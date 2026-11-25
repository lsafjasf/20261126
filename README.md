# SMPTE-TT 分层预览器

用 Go 标准库解析 SMPTE-TT，并按资源类型拆成三个独立图层：

- `background_image`：`smpte:backgroundImage`、`tts:backgroundImage` 和 `<image>` 资源；
- `foreground_text`：普通字幕/文字块；
- `annotation`：`role`/`class` 标注、`metadata` 以及 XML 注释。

工具不只解析 XML：它会解码图片、解析样式与时间区间、计算每个时刻的可见性，再将三个图层实际光栅化为 PNG，最后合成逐时刻预览帧并输出检查清单。

## 快速开始

```bash
go run ./cmd/smpte-tt \
  -in testdata/sample.ttml \
  -out artifacts/sample-preview \
  -times 0s,1.5s,3.5s,5.5s
```

仓库中的 `artifacts/sample-preview/` 已按上述命令生成，可直接打开查看。

输出示例：

```text
artifacts/sample-preview/
├── manifest.json
├── frames/
│   └── 000-000000000000ns.png
└── layers/
    ├── background_image/
    ├── foreground_text/
    └── annotation/
```

每个采样时刻都会生成四个 PNG：三个独立图层和一个按
`background_image -> foreground_text -> annotation` 顺序合成的预览帧。

## 可见性检查

`manifest.json` 中的 `visibility_checks` 会分别汇总背景图、前景文字和注释：

- `ever_visible` / `never_visible`：在所选采样时刻内是否真正参与渲染；
- `intrinsically_hidden`：被样式或语义永久隐藏的资源数；
- `hidden_annotations.passed`：隐藏注释是否始终不可见；
- 每帧的 `layers.*.non_transparent_pixels` 和 PNG SHA-256 用于确认图层不是空壳；
- `checks.passed = false` 时，隐藏注释发生了可见泄漏。

隐藏规则包括：

- `tts:display="none"`；
- `tts:visibility="hidden"` 或 `collapse`；
- `tts:opacity="0"`；
- `metadata`、`desc` 和 XML 注释只进入注释层，不参与显示；
- 时间区间之外默认不可见，区间为左闭右开 `[begin, end)`。

使用 `-strict` 时，任何隐藏注释可见泄漏都会让命令以非零状态退出。

## 时间采样

默认按事件边界采样：每个资源的 `begin` 和 `end - 1ns`，因此能覆盖背景切换、
字幕进出和注释隐藏区间。也可以用 `-fps` 做均匀采样，或用 `-times` 精确指定：

```bash
go run ./cmd/smpte-tt -in captions.ttml -out preview -fps 2
go run ./cmd/smpte-tt -in captions.ttml -out preview -times 0s,500ms,00:00:02.250
```

支持的时间格式包括 `1.5s`、`250ms`、`10f`、`100t`、
`HH:MM:SS.mmm` 和 `HH:MM:SS:FF`。子元素的相对时间以父元素 `begin` 为基准。

## 主要参数

```text
-in, -input     输入 SMPTE-TT 文件
-out, -output   输出目录（默认 smpte-tt-preview）
-times          逗号分隔的精确采样时刻
-fps            均匀采样率；0 表示按事件边界采样
-max-frames     最大采样帧数（默认 200）
-width/-height  覆盖画布尺寸
-strict         隐藏注释泄漏时失败
```

## 实现范围

- 仅使用 Go 标准库，无第三方依赖，也不调用 ImageMagick/ffmpeg 等外部程序。
- 支持 PNG、JPEG、GIF 内嵌 data URI 和本地文件；不主动抓取网络图片。
- 支持常用 `tts` 样式：颜色、背景、字号、行高、对齐、padding、透明度、
  display/visibility、region origin/extent 和 writing mode。
- 内建可缩放 5×7 点阵字体，覆盖 ASCII；非 ASCII 文本会在清单中保留原文，
  预览中使用兜底方框字形。竖排文字会标记为隐藏/不支持。
- 未实现 TTML 动画、条件样式、字体文件下载和完整 SMPTE-TT 一致性校验。

## 验证

```bash
go test ./...
go test -race ./...
go vet ./...
```

测试会真实解码输出 PNG，比较不同时刻的背景/文字层哈希，并确认隐藏注释在
激活期间仍保持零像素。
