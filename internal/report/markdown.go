package report

import (
	"fmt"
	"io"
	"strings"
)

// writeMarkdown 输出 Markdown 表格。
// 分隔行 --- 的列数与表头一致；单元格内含 | 时转义为 \|，换行替换为 <br>，避免破坏表格。
func writeMarkdown(w io.Writer, headers []string, rows [][]string) error {
	writeLine := func(fields []string) error {
		esc := make([]string, len(fields))
		for i, f := range fields {
			esc[i] = escapeMarkdownCell(f)
		}
		_, err := fmt.Fprintln(w, "| "+strings.Join(esc, " | ")+" |")
		return err
	}

	if err := writeLine(headers); err != nil {
		return err
	}
	// 分隔行：列数与表头一致
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = "---"
	}
	if err := writeLine(sep); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeLine(row); err != nil {
			return err
		}
	}
	return nil
}

// escapeMarkdownCell 转义会破坏表格的字符。顺序重要：先反斜杠再竖线，
// 否则已写入的 \| 会被二次转义成 \\|。
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}
