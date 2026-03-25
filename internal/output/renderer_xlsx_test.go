package output

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"com.citrus.internalaicopilot/internal/infra"
)

func TestRenderXLSXParsesMarkdownTableIntoWorkbook(t *testing.T) {
	file, err := renderXLSX(RenderCommand{
		BuilderConfig: infra.BuilderConfig{
			BuilderID:  2,
			Name:       "QA 冒煙測試",
			FilePrefix: "qa-smoke-doc",
		},
		BusinessResponse: infra.ConsultBusinessResponse{
			Status: true,
			Response: `冒煙測試摘要
- 覆蓋首頁入口與 Rewards 主流程

| 用例編號 | 用例名稱 |
| --- | --- |
| TC-001 | 點擊首頁浮動按鈕入口 |`,
		},
	})
	if err != nil {
		t.Fatalf("renderXLSX returned error: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(file.FileBytes), int64(len(file.FileBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader returned error: %v", err)
	}

	entries := map[string]string{}
	for _, zipFile := range reader.File {
		handle, openErr := zipFile.Open()
		if openErr != nil {
			t.Fatalf("Open returned error: %v", openErr)
		}
		content, readErr := io.ReadAll(handle)
		_ = handle.Close()
		if readErr != nil {
			t.Fatalf("ReadAll returned error: %v", readErr)
		}
		entries[zipFile.Name] = string(content)
	}

	if !strings.Contains(entries["xl/workbook.xml"], `sheet name="cases"`) {
		t.Fatalf("workbook.xml did not contain cases sheet: %s", entries["xl/workbook.xml"])
	}
	if !strings.Contains(entries["xl/workbook.xml"], `sheet name="summary"`) {
		t.Fatalf("workbook.xml did not contain summary sheet: %s", entries["xl/workbook.xml"])
	}
	if !strings.Contains(entries["xl/worksheets/sheet1.xml"], "TC-001") {
		t.Fatalf("sheet1.xml did not contain table row: %s", entries["xl/worksheets/sheet1.xml"])
	}
	if !strings.Contains(entries["xl/worksheets/sheet2.xml"], "冒煙測試摘要") {
		t.Fatalf("sheet2.xml did not contain summary line: %s", entries["xl/worksheets/sheet2.xml"])
	}
}
