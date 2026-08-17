package report

import (
	"fmt"
	"io"
	"strings"
)

// writeTable 对齐表格输出：自实现显示宽度对齐，CJK 全角字符按 2 个显示列计，
// 表头与数据同一规则（tabwriter 把全角当 1 格会错位，故不用）。
func writeTable(w io.Writer, headers []string, rows [][]string) error {
	ncols := len(headers)
	widths := make([]int, ncols)

	// 第一遍：扫描全部单元格（含表头）确定每列最大显示宽度
	all := make([][]string, 0, 1+len(rows))
	all = append(all, headers)
	all = append(all, rows...)
	for _, row := range all {
		for i := 0; i < ncols && i < len(row); i++ {
			if wd := displayWidth(row[i]); wd > widths[i] {
				widths[i] = wd
			}
		}
	}

	// 表头与数据之间的分隔行
	sep := make([]string, ncols)
	for i, wd := range widths {
		sep[i] = strings.Repeat("-", wd)
	}

	writeLine := func(row []string) error {
		cells := make([]string, ncols)
		for i := 0; i < ncols; i++ {
			var cell string
			if i < len(row) {
				cell = row[i]
			}
			cells[i] = padTo(cell, widths[i])
		}
		_, err := fmt.Fprintln(w, strings.Join(cells, "  "))
		return err
	}

	if err := writeLine(headers); err != nil {
		return err
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

// displayWidth 返回字符串的显示宽度：全角字符按 2 列计，其余按 1 列计。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeDisplayWidth(r)
	}
	return w
}

// padTo 按显示宽度右侧补空格到指定宽度（已超过则原样返回）。
func padTo(s string, width int) string {
	if d := displayWidth(s); d < width {
		return s + strings.Repeat(" ", width-d)
	}
	return s
}

// runeDisplayWidth 粗略按 East Asian Wide/Fullwidth 区间判定全角（占 2 列）。
// 覆盖常用中文字符、CJK 标点、全角符号（含 U+FF1B 全角分号）。
func runeDisplayWidth(r rune) int {
	switch {
	case r >= 0x1100 && (r <= 0x115F || // Hangul Jamo
		r == 0x2329 || r == 0x232A || // 角括号 〈 〉
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) || // CJK 部首、汉字、谚文音节、彝文等
		(r >= 0xAC00 && r <= 0xD7A3) || // 谚文音节
		(r >= 0xF900 && r <= 0xFAFF) || // CJK 兼容表意文字
		(r >= 0xFE10 && r <= 0xFE19) || // 竖排形式
		(r >= 0xFE30 && r <= 0xFE6F) || // CJK 兼容形式
		(r >= 0xFF00 && r <= 0xFF60) || // 全角形式（含全角标点）
		(r >= 0xFFE0 && r <= 0xFFE6)): // 全角符号
		return 2
	default:
		return 1
	}
}
