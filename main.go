// csvstats — CSV 数据分析命令行工具
//
// 参考开源项目 github.com/adamdecaf/csvq 重构 + 扩展（Apache-2.0）。
// R1：核心变换 —— 多文件输入、-keep 选列、-d 分隔符、输入健壮性。
// 约束：纯标准库，仅使用 encoding/csv。
package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// main 解析参数并逐个处理输入文件。
// 多文件语义：逐文件处理、按命令行顺序依次输出，每个文件各自打印一次表头（与 csvq 一致）。
// 任一文件出错：错误输出到 stderr，最终以非 0 退出码结束。
func main() {
	keep := flag.String("keep", "", "逗号分隔的列名列表，输出顺序按此指定")
	delimFlag := flag.String("d", ",", "字段分隔符（必须是恰好一个字符，支持多字节 UTF-8）")
	flag.Parse()

	delim, err := parseDelimiter(*delimFlag)
	if err != nil {
		fatalf("%v", err)
	}

	files := flag.Args()
	if len(files) == 0 {
		flag.Usage()
		fatalf("至少需要指定一个 CSV 文件")
	}

	exitCode := 0
	for _, path := range files {
		if err := processFile(path, *keep, delim); err != nil {
			fmt.Fprintf(os.Stderr, "csvstats: %v\n", err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

// parseDelimiter 校验分隔符必须是恰好一个字符。
// 注意：用 []rune(s)[0] 而非 rune(s[0]) —— rune(s[0]) 取的是第一个字节，
// 会把多字节 UTF-8 分隔符（如全角分号 ；）截坏，这是 csvq 的原 bug，这里不能重蹈覆辙。
func parseDelimiter(s string) (rune, error) {
	runes := []rune(s)
	if len(runes) != 1 {
		return 0, fmt.Errorf("分隔符必须是恰好一个字符，当前为 %d 个字符: %q", len(runes), s)
	}
	return runes[0], nil
}

func processFile(path, keep string, delim rune) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("文件不存在: %s", path)
		}
		return fmt.Errorf("无法打开 %s: %w", path, err)
	}
	defer f.Close()

	// 剥离开头 UTF-8 BOM（EF BB BF），避免污染第一列列名、影响表头匹配。
	br := bufio.NewReader(f)
	if bom, _ := br.Peek(3); len(bom) == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		br.Discard(3)
	}

	rdr := csv.NewReader(br)
	rdr.Comma = delim

	headers, err := rdr.Read()
	if err == io.EOF {
		fmt.Fprintf(os.Stderr, "csvstats: %s 为空文件，无数据\n", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 %s 表头失败: %w", path, err)
	}

	outHeaders, outIdx, err := resolveColumns(headers, splitKeep(keep))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	// 先读一行，用于检测"只有表头、无数据行"的情形（此时不输出空表头）。
	rec, err := rdr.Read()
	if err == io.EOF {
		fmt.Fprintf(os.Stderr, "csvstats: %s 只有表头，无数据行\n", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, wrapParseError(err))
	}

	w := csv.NewWriter(os.Stdout)
	w.Comma = delim // 输出沿用输入分隔符，保证可往返
	if err := w.Write(outHeaders); err != nil {
		return fmt.Errorf("%s: 写出表头失败: %w", path, err)
	}
	if err := writeRow(w, rec, outIdx); err != nil {
		return fmt.Errorf("%s: 写出行失败: %w", path, err)
	}

	for {
		rec, err := rdr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, wrapParseError(err))
		}
		if err := writeRow(w, rec, outIdx); err != nil {
			return fmt.Errorf("%s: 写出行失败: %w", path, err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("%s: 写出结果失败: %w", path, err)
	}
	return nil
}

// splitKeep 把 -keep 参数按逗号拆分成列名，忽略分隔出的空白片段。
func splitKeep(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// resolveColumns 决定输出列：顺序 = -keep 中出现顺序；表头匹配大小写不敏感且忽略首尾空格；
// 未指定 -keep 时保留全部列、顺序与文件一致。列不存在时报错并列出缺失列名。
func resolveColumns(headers, keep []string) ([]string, []int, error) {
	if len(keep) == 0 {
		out := make([]string, len(headers))
		idx := make([]int, len(headers))
		for i, h := range headers {
			out[i] = h
			idx[i] = i
		}
		return out, idx, nil
	}

	norm := make([]string, len(headers))
	for i, h := range headers {
		norm[i] = normHeader(h)
	}

	out := make([]string, 0, len(keep))
	idx := make([]int, 0, len(keep))
	var missing []string
	for _, k := range keep {
		nk := normHeader(k)
		found := false
		for i, nh := range norm {
			if nh == nk {
				out = append(out, k) // 输出表头用 -keep 里的写法
				idx = append(idx, i) // 文件中重复列名只取第一个匹配
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, nil, fmt.Errorf("列不存在: %s", strings.Join(missing, ", "))
	}
	return out, idx, nil
}

// normHeader 归一化表头用于匹配：小写 + 去首尾空格。
func normHeader(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// writeRow 按列索引映射输出一行，索引越界时补空串（防御性，正常不会发生）。
func writeRow(w *csv.Writer, rec []string, idx []int) error {
	out := make([]string, len(idx))
	for j, ci := range idx {
		if ci < len(rec) {
			out[j] = rec[ci]
		}
	}
	return w.Write(out)
}

// wrapParseError 把 csv 解析错误补上行号信息。
func wrapParseError(err error) error {
	var pe *csv.ParseError
	if errors.As(err, &pe) {
		return fmt.Errorf("第 %d 行解析失败: %w", pe.Line, err)
	}
	return err
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "csvstats: "+format+"\n", args...)
	os.Exit(2)
}
