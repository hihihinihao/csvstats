// csvstats — CSV 数据分析命令行工具
//
// 参考开源项目 github.com/adamdecaf/csvq 重构 + 扩展（Apache-2.0）。
// R1：核心变换（多文件输入、-keep 选列、-d 分隔符、输入健壮性）。
// R2：数值感知多列排序（-sort.asc / -sort.dsc）。
// R3：集中式输出层（-format csv/table/tabs/markdown/json，独立 internal/report 包）。
// 约束：纯标准库。
package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hihihinihao/csvstats/internal/report"
)

// SortKey 表示一个排序列及方向（按命令行出现顺序）。
type SortKey struct {
	Name string
	Desc bool
}

func main() {
	// Go 标准 flag 包不保留 -sort.asc / -sort.dsc 的相对顺序，且空值参数需要忽略，
	// 因此先从 os.Args 中剥离排序参数（保序、忽略空值），剩余参数再交给 flag 解析。
	sortKeys, cleanArgs := extractSortArgs(os.Args[1:])

	fs := flag.NewFlagSet("csvstats", flag.ExitOnError)
	keep := fs.String("keep", "", "逗号分隔的列名列表，输出顺序按此指定")
	delimFlag := fs.String("d", ",", "字段分隔符（必须是恰好一个字符，支持多字节 UTF-8）")
	formatFlag := fs.String("format", "", "输出格式: csv/table/tabs/markdown/json（默认 csv）")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: csvstats [选项] 文件1 [文件2 ...]\n")
		fmt.Fprintf(os.Stderr, "选项:\n")
		fmt.Fprintf(os.Stderr, "  -keep 列1,列2      选列，输出顺序按此指定\n")
		fmt.Fprintf(os.Stderr, "  -d 分隔符          字段分隔符（默认逗号，支持多字节 UTF-8）\n")
		fmt.Fprintf(os.Stderr, "  -sort.asc 列[,列]  按列升序排序（可多个，按出现顺序；支持 = 写法）\n")
		fmt.Fprintf(os.Stderr, "  -sort.dsc 列[,列]  按列降序排序（同上）\n")
		fmt.Fprintf(os.Stderr, "  -format 格式       输出格式: csv/table/tabs/markdown/json（默认 csv）\n")
	}
	fs.Parse(cleanArgs)

	delim, err := parseDelimiter(*delimFlag)
	if err != nil {
		fatalf("%v", err)
	}

	format, err := report.ParseFormat(*formatFlag)
	if err != nil {
		fatalf("%v", err)
	}

	files := fs.Args()
	if len(files) == 0 {
		fs.Usage()
		fatalf("至少需要指定一个 CSV 文件")
	}

	exitCode := 0
	for _, path := range files {
		if err := processFile(path, *keep, delim, sortKeys, format); err != nil {
			fmt.Fprintf(os.Stderr, "csvstats: %v\n", err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

// extractSortArgs 遍历 os.Args，剥离排序参数并保持出现顺序；空值参数直接忽略。
// 返回 (保序的排序列, 供 flag 解析的其余参数)。
func extractSortArgs(args []string) ([]SortKey, []string) {
	var keys []SortKey
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, hasInline := splitSortArg(arg)
		if name == "" {
			rest = append(rest, arg)
			continue
		}
		desc := name == "sort.dsc"
		if !hasInline {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				val = args[i+1]
				i++ // 消费值参数
			} else {
				continue // 空值参数：忽略
			}
		}
		for _, n := range splitKeep(val) {
			if n != "" {
				keys = append(keys, SortKey{Name: n, Desc: desc})
			}
		}
	}
	return keys, rest
}

// splitSortArg 解析单个参数是否为排序 flag，返回 (flag名, 内联值, 是否有内联值)。
func splitSortArg(arg string) (name, val string, hasInline bool) {
	n, v, has := strings.Cut(arg, "=")
	n = strings.TrimLeft(n, "-")
	if n != "sort.asc" && n != "sort.dsc" {
		return "", "", false
	}
	return n, v, has
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

func processFile(path, keep string, delim rune, sortKeys []SortKey, format report.Format) error {
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

	// 排序在 -keep 之后的列空间上进行：索引指向输出行的位置。
	var resolved []resolvedSortKey
	if len(sortKeys) > 0 {
		resolved, err = resolveSortKeys(sortKeys, outHeaders)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return emit(path, rdr, outHeaders, outIdx, resolved, format)
}

// resolvedSortKey 排序时输出行内的列位置。
type resolvedSortKey struct {
	idx  int
	desc bool
}

// emit 读入全部行、映射到 -keep 列空间、（可选）稳定排序，再交给 report 包按格式输出。
// 注：R3 起统一为内存结果模型以支撑多格式报表；大文件流式处理留待后续轮次优化。
func emit(path string, rdr *csv.Reader, outHeaders []string, outIdx []int, sortKeys []resolvedSortKey, format report.Format) error {
	var rows [][]string
	for {
		rec, err := rdr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, wrapParseError(err))
		}
		rows = append(rows, mapRow(rec, outIdx))
	}
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "csvstats: %s 只有表头，无数据行\n", path)
		return nil
	}

	if len(sortKeys) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			for _, k := range sortKeys {
				if cmp := compareCells(rows[i][k.idx], rows[j][k.idx], k.desc); cmp != 0 {
					return cmp < 0
				}
			}
			return false
		})
	}

	return report.Write(os.Stdout, format, outHeaders, rows)
}

// resolveSortKeys 把排序列名解析为输出行内的位置（-keep 之后的空间）。
// 引用不在输出中的列（含未知列）时，报错并列出该列名。
func resolveSortKeys(keys []SortKey, outHeaders []string) ([]resolvedSortKey, error) {
	out := make([]resolvedSortKey, 0, len(keys))
	for _, k := range keys {
		idx := -1
		for i, h := range outHeaders {
			if normHeader(h) == normHeader(k.Name) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("排序引用的列不在输出中（或未包含在 -keep 中）: %s", k.Name)
		}
		out = append(out, resolvedSortKey{idx: idx, desc: k.Desc})
	}
	return out, nil
}

// compareCells 比较两个格值。空值（TrimSpace 后为空）在任何方向上都排最后，
// 因此空值的判定不参与 desc 翻转——否则降序时空值会被排到最前。
func compareCells(a, b string, desc bool) int {
	as, bs := strings.TrimSpace(a), strings.TrimSpace(b)
	ae, be := as == "", bs == ""
	if ae || be {
		switch {
		case ae && be:
			return 0
		case ae:
			return 1 // a 空 → a 始终排后
		default:
			return -1
		}
	}

	cmp := compareNonEmpty(as, bs)
	if desc {
		return -cmp
	}
	return cmp
}

// compareNonEmpty 比较两个非空值：都解析为数字时按数值比较；
// 数值相等（含大整数超出 float64 精度、或 "1"/"01"/"1.0" 之类）回落字符串比较保证确定性。
// NaN/±Inf 这类 ParseFloat 能接受但不能正常比较的特殊值，识别后一律按字符串处理。
func compareNonEmpty(a, b string) int {
	an, aErr := strconv.ParseFloat(a, 64)
	bn, bErr := strconv.ParseFloat(b, 64)
	aNum := aErr == nil && !specialFloat(an)
	bNum := bErr == nil && !specialFloat(bn)
	if aNum && bNum {
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		}
	}
	return strings.Compare(a, b)
}

// specialFloat 判断是否为 NaN / ±Inf —— 它们与任何数值比较都会破坏排序，应按字符串处理。
func specialFloat(f float64) bool {
	return math.IsNaN(f) || math.IsInf(f, 0)
}

// splitKeep 把逗号分隔的列名列表拆成数组，忽略空白片段。
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

// mapRow 按列索引映射一行到输出（keep 空间），索引越界时补空串（防御性）。
func mapRow(rec []string, outIdx []int) []string {
	out := make([]string, len(outIdx))
	for j, ci := range outIdx {
		if ci < len(rec) {
			out[j] = rec[ci]
		}
	}
	return out
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
