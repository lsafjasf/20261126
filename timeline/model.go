package timeline

// ItemKind 区分叶子片段与复合片段引用。
type ItemKind string

const (
	// KindLeaf 是引用具体源媒体的叶子片段。
	KindLeaf ItemKind = "leaf"
	// KindRef 是在父时间轴上对某个 Compound 的引用（嵌套）。
	KindRef ItemKind = "ref"
)

// Item 是时间轴上的一个片段。
//
// 叶子片段：Kind=KindLeaf，SourceID 指向源媒体，SrcIn/SrcOut 为源帧区间。
// 复合引用：Kind=KindRef，CompoundID 指向另一个 Compound，此时必须与
// 被引用复合片段等长（SrcIn/SrcOut 留空），映射通过子时间轴逐帧完成。
//
// Freeze=true 表示冻结段：整个条目持续显示 SrcIn 这一源帧。
type Item struct {
	ID         string   `json:"id"`
	Kind       ItemKind `json:"kind"`
	SourceID   string   `json:"sourceID,omitempty"`
	CompoundID string   `json:"compoundID,omitempty"`

	Start int64 `json:"start"`
	Dur   int64 `json:"dur"`

	// 叶子片段的源帧区间（含 SrcIn，不含 SrcOut）。
	SrcIn  int64 `json:"srcIn,omitempty"`
	SrcOut int64 `json:"srcOut,omitempty"`

	// 条目自身（内层）速率。零值视为 1.0。
	Speed Speed `json:"speed"`

	Freeze bool `json:"freeze,omitempty"`

	Markers []Marker `json:"markers,omitempty"`
}

// Marker 是附着在所属时间轴局部坐标上的标记，Offset 为相对条目起点的帧偏移。
type Marker struct {
	ID     string `json:"id"`
	Offset int64  `json:"offset"`
	Note   string `json:"note,omitempty"`
}

// Compound 是一条可被复用的子时间轴。
type Compound struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Dur     int64    `json:"dur"`
	Items   []*Item  `json:"items"`
	Markers []Marker `json:"markers,omitempty"`
}

// Source 是一段源媒体，Frames 为其总帧数。
type Source struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Frames int64  `json:"frames"`
}

// Project 是完整工程：源媒体库、复合片段库与根时间轴。
type Project struct {
	Name      string               `json:"name,omitempty"`
	Sources   map[string]*Source   `json:"sources"`
	Compounds map[string]*Compound `json:"compounds"`
	Root      *Compound            `json:"root"`
}

// FrameKind 标记解析结果的类型。
type FrameKind string

const (
	// FrameMedia 是真实媒体帧。
	FrameMedia FrameKind = "media"
	// FrameFreeze 是冻结段，SourceFrame 固定为冻结源帧。
	FrameFreeze FrameKind = "freeze"
	// FrameGap 是时间轴空隙（无任何条目覆盖）。
	FrameGap FrameKind = "gap"
)

// FrameRecord 是某一层时间轴上一帧的完整父子映射记录。
//
// 展开前后对同一根帧分别解析，InternalSpeed/ExternalSpeed/SourceFrame
// 必须逐项一致，从而证明标记位置没有漂移。
type FrameRecord struct {
	RootFrame     int64     `json:"rootFrame"`
	Kind          FrameKind `json:"kind"`
	Path          []string  `json:"path"`
	SourceID      string    `json:"sourceID,omitempty"`
	SourceFrame   int64     `json:"sourceFrame,omitempty"`
	InternalSpeed Speed     `json:"internalSpeed"`
	ExternalSpeed Speed     `json:"externalSpeed"`
	CombinedSpeed Speed     `json:"combinedSpeed"`
	Markers       []string  `json:"markers,omitempty"`

	// product 是从根到当前层的外部速率累积，仅内部使用。
	product Speed
}
