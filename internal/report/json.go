package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// writeJSON 输出结构化对象数组 [ {字段名: 值}, ... ]，字段名 = 列名，保持列顺序。
// 用 SetEscapeHTML(false) 关闭 Go 默认的 < > & → \uXXXX 转义，保证日志 URL 里的 & 可读；
// 中文本身不被 Go 转义，直接 UTF-8 输出。缩进两层空格，可读性好。
func writeJSON(w io.Writer, headers []string, rows [][]string) error {
	var b bytes.Buffer
	b.WriteString("[\n")
	for ri, row := range rows {
		if ri > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("  {")
		for i, h := range headers {
			if i > 0 {
				b.WriteString(", ")
			}
			key, err := marshalNoEscape(h)
			if err != nil {
				return err
			}
			val, err := marshalNoEscape(cellAt(row, i))
			if err != nil {
				return err
			}
			b.Write(key)
			b.WriteString(": ")
			b.Write(val)
		}
		b.WriteString("}")
	}
	b.WriteString("\n]\n")
	if _, err := w.Write(b.Bytes()); err != nil {
		return fmt.Errorf("写出 JSON: %w", err)
	}
	return nil
}

func cellAt(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}

// marshalNoEscape 用 SetEscapeHTML(false) 编码单个值，并去掉 Encoder 追加的换行。
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("JSON 编码失败: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
