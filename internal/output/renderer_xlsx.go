package output

import (
	"regexp"
	"strconv"
	"strings"
)

var markdownSeparatorCellPattern = regexp.MustCompile(`^:?-{3,}:?$`)

func renderXLSX(command RenderCommand) (RenderedFile, error) {
	table := parseMarkdownTable(command.BusinessResponse.Response)
	var sheets []xlsxSheet
	if table != nil {
		casesRows := make([][]string, 0, len(table.rows)+2)
		casesRows = append(casesRows, []string{command.BuilderConfig.Name})
		casesRows = append(casesRows, table.headers)
		casesRows = append(casesRows, table.rows...)
		sheets = append(sheets, xlsxSheet{Name: "cases", Rows: casesRows})

		if len(table.summaryLines) > 0 {
			summaryRows := make([][]string, 0, len(table.summaryLines)+1)
			summaryRows = append(summaryRows, []string{command.BuilderConfig.Name})
			for _, line := range table.summaryLines {
				summaryRows = append(summaryRows, []string{line})
			}
			sheets = append(sheets, xlsxSheet{Name: "summary", Rows: summaryRows})
		}
	} else {
		rows := [][]string{
			{command.BuilderConfig.Name},
			{"Line", "Content"},
		}
		lines := strings.Split(strings.ReplaceAll(command.BusinessResponse.Response, "\r\n", "\n"), "\n")
		for index, line := range lines {
			rows = append(rows, []string{strconv.Itoa(index + 1), line})
		}
		sheets = []xlsxSheet{{Name: "consult", Rows: rows}}
	}

	bytes, err := buildXLSXWorkbook(sheets)
	if err != nil {
		return RenderedFile{}, err
	}
	return RenderedFile{
		FileName:    buildFileName(command.BuilderConfig, "xlsx"),
		ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		FileBytes:   bytes,
	}, nil
}

type markdownTable struct {
	headers      []string
	rows         [][]string
	summaryLines []string
}

func parseMarkdownTable(response string) *markdownTable {
	if strings.TrimSpace(response) == "" {
		return nil
	}

	lines := strings.Split(strings.ReplaceAll(response, "\r\n", "\n"), "\n")
	for index := 0; index < len(lines)-1; index++ {
		if !looksLikeMarkdownRow(lines[index]) || !isSeparatorRow(lines[index+1]) {
			continue
		}

		headers := splitMarkdownRow(lines[index])
		if len(headers) == 0 {
			continue
		}

		rows := make([][]string, 0)
		endIndex := index + 2
		for endIndex < len(lines) && looksLikeMarkdownRow(lines[endIndex]) {
			values := splitMarkdownRow(lines[endIndex])
			if len(values) == len(headers) {
				rows = append(rows, values)
			}
			endIndex++
		}
		if len(rows) == 0 {
			return nil
		}

		summaryLines := make([]string, 0)
		for lineIndex, line := range lines {
			if lineIndex >= index && lineIndex < endIndex {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				summaryLines = append(summaryLines, trimmed)
			}
		}
		return &markdownTable{
			headers:      headers,
			rows:         rows,
			summaryLines: summaryLines,
		}
	}

	return nil
}

func looksLikeMarkdownRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && strings.Count(trimmed, "|") >= 2
}

func isSeparatorRow(line string) bool {
	cells := splitMarkdownRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !markdownSeparatorCellPattern.MatchString(cell) {
			return false
		}
	}
	return true
}

func splitMarkdownRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	if strings.TrimSpace(trimmed) == "" {
		return nil
	}

	tokens := strings.Split(trimmed, "|")
	cells := make([]string, 0, len(tokens))
	for _, token := range tokens {
		cells = append(cells, strings.TrimSpace(token))
	}
	return cells
}
