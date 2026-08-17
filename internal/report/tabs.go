package report

import (
	"fmt"
	"io"
	"strings"
)

// writeTabs 制表符分隔输出。单元格含 tab/换行会破坏对齐，属已知限制（README 说明）。
func writeTabs(w io.Writer, headers []string, rows [][]string) error {
	writeLine := func(fields []string) error {
		_, err := fmt.Fprintln(w, strings.Join(fields, "\t"))
		return err
	}
	if err := writeLine(headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeLine(row); err != nil {
			return err
		}
	}
	return nil
}
