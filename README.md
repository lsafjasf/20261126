# 20261126

# cea608

纯 Go 标准库实现的 CEA-608/EIA-608 字节对解析器。它把普通 caption 与
Extended Data Services（XDS）包送入互不覆盖的状态，并保留 15×32 caption
屏幕、pop-on/paint-on/roll-up 状态机、首个坏控制码以及截断恢复诊断。

```sh
go test ./...
go run ./cmd/cea608-dump -json input.608
```

## 核心用法

```go
package main

import (
    "fmt"
    "cea608"
)

func main() {
    data := []byte{
        0x94, 0x20, // RCL（奇校验在线字节）
        0x94, 0x40, // PAC：第 14 行
        0xC8, 'E', 'L', 'L', 'O', 0x80,
        0x94, 0x2F, // EOC
    }

    result := cea608.Parse(data)
    fmt.Println(result.DisplayText)
    fmt.Printf("xds=%d bad=%v truncated=%d\n",
        len(result.XDSPackets),
        result.FirstBadCode,
        result.TruncatedPairs)
}
```

流式输入可按任意字节边界调用；半个双字节对会缓存到下一次：

```go
d := cea608.NewParser()
for _, chunk := range chunks {
    d.Decode(chunk)
}
d.Finish()
result := d.Result()
```

如果一次中断已经永久丢失字节对的第二个字节，新的数据段应从双字节边界开始，
调用 `DecodeSegment` 会自动丢弃上一段残留的首字节；也可以显式调用
`Resync`/`Recover`。

## Caption 与 XDS 隔离

- `Decoder.Caption` 保存模式、累积文本、cue、行列和 roll-up 窗口。
- `Decoder.XDS` 保存当前 XDS class/type/content、已完成包和校验错误。
- XDS 帧按 `class/type + content + 0x0F/checksum` 收集；线字节 `0x8F`
  （带奇校验的 `0x0F`）也可识别。
- XDS 内容不会追加到 `Caption.Text`、`Lines()` 或 cue。
- 残缺 XDS 后收到 `0x10..0x1F` caption 控制前导码时，会中止 XDS 并回到
  caption 解析。

## 坏控制码与恢复

`FirstBadControl` 只记录第一次出现的坏控制对，包含归一化码值、原始位置、
pair 序号、原因和是否为校验错误；后续坏码计入 `BadControls`，不会覆盖首个
证据。奇校验可自动识别：首个字节对符合奇校验时按线格式严格校验，否则按已经
去掉校验位的 7-bit 数据处理。也可用 `NewWithOptions` 强制
`ParityWire` 或 `ParityOff`。

完整坏字节对会被丢弃并继续下一对；尾部单字节会计入 `TruncatedPairs`。
解析器不会因坏码、坏校验或截断而终止。

## 兼容 API

同时提供 `New` / `NewDecoder` / `NewParser`、`Parse` / `ParseChecked` /
`ParseBytes`、`Decoder.Decode` / `Feed` / `Parse`，以及旧测试常用的
`Lines()`、`Mode()`、`String()`、`BadPairs` 和 `Reset()`。
