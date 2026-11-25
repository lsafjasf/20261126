# assclip

审计 ASS/SSA 字幕的 vector `\clip` / `\iclip` 在目标分辨率下是否错位、裁字或压住主体。实现与测试全部使用 Go，无第三方依赖。

## 处理方式

1. 读取 `PlayResX/PlayResY`（也可用 `-play-res` 覆盖）。
2. 将脚本画布映射到目标画幅，支持三种策略：
   - `fit`：等比缩放，完整保留脚本画布，可能留黑边。
   - `fill`：等比缩放，铺满目标画幅，可能裁掉画布边缘。
   - `stretch`：横向和纵向独立缩放，会改变画布宽高比。
3. 同一个仿射变换同时作用于 vector clip 点、矩形 clip、边距和富文本位置。边距的横向长度按 `scale_x`、纵向长度按 `scale_y` 换算。
4. 安全区从“映射后的脚本内容区”计算，而不是错误地贴到输出帧边缘；因此 `fit` 留黑边时不会被算成额外安全空间。
5. 输出每个 cue 的脚本坐标 bbox、像素 bbox、裁到帧内的 bbox、边距安全区及冲突。

文本 bbox 是无字体文件时的保守估算，仅用于发现明显的裁字/越界；它不是最终光栅化结果。vector 路径也以路径坐标的包围盒参与检测。

选择策略时的取舍：`fit` 不会裁掉脚本上下边缘，但会留黑边；`fill` 保持背景铺满，但脚本边缘的 cue 会真实地落到画框外，报告会给出 `text_outside_frame`；`stretch` 能铺满且不裁内容，但会改变字体和图形的宽高比。若背景已经裁切、字幕需要重新排版，应优先重做字幕边距，而不是只换坐标。

## 用法

```sh
go run ./cmd/assclip -w 1080 -h 1920 -strategy fit testdata/demo.ass
go run ./cmd/assclip -w 3440 -h 1440 -strategy fill \
  -subject 1200,300,1000,700 -format json testdata/demo.ass
cat testdata/demo.ass | go run ./cmd/assclip \
  -w 1080 -h 1920 -strategy fit -i -
```

常用参数：

- `-w` / `-h`：目标宽高。
- `-strategy fit|fill|stretch`：画布缩放策略，默认 `fit`。
- `-play-res 1920x1080`：覆盖脚本分辨率，适合缺失或写错的 `PlayRes`。
- `-subject x,y,w,h`：主体保护区，输出像素坐标，可重复传入。
- `-format text|json`：人类可读表格或 JSON，默认 `text`。
- `-fail-on-conflict`：存在 error 级冲突时退出码为 1。

参数应写在输入文件前，或使用 `-i FILE`。`-h` 单独使用时显示帮助。

## 输出

文本模式逐 cue 输出：

```text
#2 0:00:02.00-0:00:04.00 style=Default an=2 text="This clip is too small..."
  clip: clip vector script=(0.00,0.00)-(800.00,400.00) pixels=(0.00,656.25)-(450.00,881.25) ...
  margins: L=56.25 R=56.25 V=28.12 px | safe=(56.25,656.25)-(1023.75,1235.62) (bottom)
  text~: (...)
  [error] text_cropped_by_clip: the clip bounds do not cover the estimated text box overflow{...}
```

JSON 输出的关键字段：

- `target`：脚本/目标分辨率、`scale_x`、`scale_y`、`offset_x`、`offset_y`。
- `cue.clip.bounds_script`：原始 ASS 坐标包围盒。
- `cue.clip.bounds_pixels`：换算后的输出像素包围盒。
- `cue.clip.bounds_clamped_to_frame`：裁到目标帧内的部分。
- `cue.clip.frame_overflow`：四边越出目标帧的像素量。
- `cue.safe_area`：边距安全区及上/中/下锚点。
- `cue.estimated_text_bounds`：估算文本 bbox。
- `cue.conflicts`：结构化冲突。

冲突码：

- `text_cropped_by_clip`：`\clip` 包围盒没有覆盖估算文本。
- `text_hidden_by_inverse_clip`：文本落入 `\iclip` 的排除区。
- `text_outside_safe_area`：文本越过边距安全区。
- `text_outside_frame`：文本越过目标帧。
- `clip_outside_frame`：clip 本身越出目标帧，部分路径不会被渲染。
- `text_visible_over_subject`：按 clip 可见性估算后，文本仍压到 `-subject` 保护区。

## 测试

```sh
go test ./...
go vet ./...
```

测试覆盖：格式列重排、矩形/矢量/反向 clip、`fit`/`fill`/`stretch` 数值、1080x1920 竖屏、3440x1440 超宽画幅、留黑边安全区、边距换算、裁字和主体冲突。
