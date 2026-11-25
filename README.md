# fourch

`fourch` 处理一个只有 `4ch` 容器标签、没有声道掩码或编码标识的四声道音频流。它不会仅凭声道数猜结论，而是把解释保留为可检验的候选，并检查候选到离散扬声器的线性映射是否可逆。

实现只使用 Go 标准库。

## 候选解释

默认提供两个候选，顺序不表示偏好：

| 维度 | quadraphonic | ACN/SN3D Ambisonics |
| --- | --- | --- |
| 声道角色 | `FL, FR, BL, BR` 四个离散扬声器馈送 | `ACN0/W, ACN1/Y, ACN2/Z, ACN3/X` |
| 坐标约定 | AmbiX：`+X` 前、`+Y` 左、`+Z` 上；FL/FR/BL/BR 为 `+45/-45/+135/-135` 度 | 同一 AmbiX 坐标；方位角从前向 `+Y` 逆时针为正 |
| 源方向模型 | 水平四角之间的二维 VBAP | `[W,Y,Z,X] = [1, sin(az)cos(el), sin(el), cos(az)cos(el)]`，SN3D |
| 高度信息 | 无 | 由 ACN2/Z 表示 |

裸 `4ch` 不包含声道掩码，因此 `DefaultCandidates()` 会同时返回两者。若有可信的容器掩码、编码器元数据或制作说明，应先用它们缩小候选集，再调用证据匹配。

## 旋转证据

`Observation` 表示一次“已知声源方向，读取四声道值”的结果。`Interpret` 将观测向量和候选预测向量各自归一化，避免录音增益影响判断；每个观测的欧氏残差都必须落在容差内，候选才算 `supported`。

- 没有观测或只有静音观测时，不产生方向证据：所有候选标为 `insufficient-evidence`，并保持歧义。
- 多个候选同时通过容差时，`Interpretation.Ambiguous` 为真，`Plausible` 保留全部候选，`Best` 为空。
- 多角度旋转通常能区分二者：Ambisonic 的 W 项不随方位变化，而一阶球谐项按方向连续变化；quad 的能量只在相邻两只扬声器之间转移。
- 坐标约定本身也可能残留歧义。例如两个候选只有 ACN2/Z 的符号不同，纯水平旋转无法区分，加入有高度的观测后才能判定；测试集显式覆盖了这种情况。
- 单一角度也可能足以排除一个候选，但这取决于实际角度和噪声。调用方可以调整 `InterpretOptions.ResidualTolerance`，也可以用自定义 `Candidate` 表达另一种声像算法或坐标符号约定。

判断依据是“已知源方向”本身可信。如果旋转角度或声源轨迹不可信，不应把残差最小者当作确定结论。

## 可逆性

`Mapping.Invertibility` 对映射矩阵做带主元消元，返回秩、零空间和（方阵满秩时的）逆矩阵。

- quad 直连到四只同名扬声器是一个 `4x4` 置换矩阵，满秩，因此可逆。`CanonicalQuadSpeakers()` 在标准顺序下得到单位矩阵。
- ACN/SN3D 到经典水平四角扬声器的压力投影解码，每行是 `[1,y,z,x]`。四只扬声器都在水平面时 `z=0`，所以矩阵秩为 3，零空间由 `ACN2/Z` 张成。解码后无法从扬声器馈送恢复高度分量，映射不可逆。
- 四只非共面扬声器（例如包含头顶扬声器）可能让该 `4x4` 解码满秩并可逆；这是扬声器几何的性质，不是 Ambisonics 的普遍保证。
- 多于四只扬声器时，满列秩只说明映射单射、存在左逆，不应报告为方阵可逆。

这些结论针对理想线性矩阵，不包含距离衰减、房间反射、HRTF、滤波器组、max-rE 权重或扬声器时延。

## 使用

```go
observations := []fourch.Observation{
	{AzimuthDeg: 0, Channels: [4]float64{1, 0, 0, 1}},
	{AzimuthDeg: 90, Channels: [4]float64{1, 1, 0, 0}},
}

result, err := fourch.Interpret(
	observations,
	fourch.DefaultCandidates(),
	fourch.InterpretOptions{},
)
if err != nil {
	panic(err)
}
for _, candidate := range result.Plausible {
	// 在证据不足时这里会保留多个候选。
	fmt.Println(candidate.Candidate.Label, candidate.Residual)
}

speakers := fourch.CanonicalQuadSpeakers()
mapping, err := fourch.AmbisonicToSpeakers(speakers)
if err != nil {
	panic(err)
}
report, err := mapping.Invertibility(0)
if err != nil {
	panic(err)
}
fmt.Println(report.Rank, report.Invertible, report.Nullspace)
```

## 测试

```sh
go test ./...
go vet ./...
```

测试角度、录音增益、矩阵规模和扬声器布局均在代码中循环生成；覆盖旋转证据判别、无证据歧义、quad 置换映射往返、水平 Ambisonics 解码降秩及零空间见证。
