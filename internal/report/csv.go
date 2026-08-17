package report

import (
	"encoding/csv"
	"fmt"
	"io"
)

// writeCSV 用 encoding/csv.Writer 输出，自动处理引号/换行。
// 输出分隔符固定逗号（不跟随输入 -d），默认输出表头（与 R1 一致）。
func writeCSV(w io.Writer, headers []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	cw.Comma = ','
	if err := cw.Write(headers); err != nil {
		return fmt.Errorf("写出表头: %w", err)
	}
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("写出行: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("写出 CSV: %w", err)
	}
	return nil
}
