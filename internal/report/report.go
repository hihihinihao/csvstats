// Package report 提供 csvstats 的集中式输出层。
//
// 消费主程序已解析好的输出列（表头与每行均处于 -keep 顺序），按位置输出，
// 不做列名匹配——大小写不敏感匹配在读取阶段已完成。
package report

import (
	"fmt"
	"io"
	"strings"
)

// Format 输出格式。
type Format string

const (
	CSV      Format = "csv"
	Table    Format = "table"
	Tabs     Format = "tabs"
	Markdown Format = "markdown"
	JSON     Format = "json"
)

var supported = []Format{CSV, Table, Tabs, Markdown, JSON}

// ParseFormat 大小写不敏感地解析格式名；空值默认 csv；未知格式报错并列出支持列表。
func ParseFormat(s string) (Format, error) {
	f := Format(strings.ToLower(strings.TrimSpace(s)))
	if f == "" {
		return CSV, nil
	}
	for _, k := range supported {
		if k == f {
			return f, nil
		}
	}
	names := make([]string, len(supported))
	for i, k := range supported {
		names[i] = string(k)
	}
	return "", fmt.Errorf("未知输出格式 %q，支持的格式: %s", s, strings.Join(names, ", "))
}

// Write 把已按输出列顺序组织好的结果写到 w。
func Write(w io.Writer, format Format, headers []string, rows [][]string) error {
	switch format {
	case CSV:
		return writeCSV(w, headers, rows)
	case Table:
		return writeTable(w, headers, rows)
	case Tabs:
		return writeTabs(w, headers, rows)
	case Markdown:
		return writeMarkdown(w, headers, rows)
	case JSON:
		return writeJSON(w, headers, rows)
	default:
		return fmt.Errorf("未知输出格式 %q", format)
	}
}
