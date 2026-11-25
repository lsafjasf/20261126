package timeline

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strconv"
)

// ExportJSON 把逐帧映射记录导出为缩进 JSON。
func ExportJSON(records []FrameRecord) ([]byte, error) {
	return json.MarshalIndent(records, "", "  ")
}

// ExportCSV 把逐帧映射记录导出为 CSV，速率以约分后的分数字符串输出，
// 保证“内部速度、外部速度、源帧”三列可直接对账。
func ExportCSV(records []FrameRecord) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	header := []string{
		"rootFrame", "kind", "path", "sourceID", "sourceFrame",
		"internalSpeed", "externalSpeed", "combinedSpeed", "markers",
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, rec := range records {
		row := []string{
			strconv.FormatInt(rec.RootFrame, 10),
			string(rec.Kind),
			joinIDs(rec.Path),
			rec.SourceID,
			strconv.FormatInt(rec.SourceFrame, 10),
			rec.InternalSpeed.String(),
			rec.ExternalSpeed.String(),
			rec.CombinedSpeed.String(),
			joinIDs(rec.Markers),
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
