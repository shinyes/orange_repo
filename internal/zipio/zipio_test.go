package zipio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// zipWriterT 测试辅助：向内存缓冲写 ZIP 条目。
type zipWriterT struct{ z *zip.Writer }

func newZipWriter(buf *bytes.Buffer) *zipWriterT { return &zipWriterT{z: zip.NewWriter(buf)} }

func (w *zipWriterT) create(name, content string) error {
	f, err := w.z.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(content))
	return err
}

func (w *zipWriterT) Close() error { return w.z.Close() }

func sampleProblems() ([]ExportProblem, PlanMeta) {
	problems := []ExportProblem{
		{
			Type: "programming", Title: "A+B 问题", Tags: []string{"入门", "模拟"},
			StatementMD: "# A+B\n求 $a+b$。\n![图](/api/uploads/abcd1234.png)",
			BodyJSON:    json.RawMessage(`{"inputFormat":"一行两个整数","outputFormat":"一个整数","samples":[{"input":"1 2","output":"3"}],"testCases":[{"input":"1 2","output":"3"}]}`),
			AnswerJSON:  json.RawMessage(`{}`),
			Solutions:   json.RawMessage(`[{"language":"cpp","code":"int main(){}","markdown":"思路见题面"}]`),
			TimeLimitMS: 1000, MemoryLimitMiB: 256,
		},
		{
			Type: "single_choice", Title: "选择题样例", Tags: []string{"语法"},
			StatementMD: "下列哪个不是合法标识符？",
			BodyJSON:    json.RawMessage(`{"options":["int","2var","float"]}`),
			AnswerJSON:  json.RawMessage(`{"answerIndex":1}`),
			Solutions:   json.RawMessage(`[]`),
		},
		{
			Type: "true_false", Title: "判断题样例", Tags: nil,
			StatementMD: "C++ 是编译型语言。",
			BodyJSON:    json.RawMessage(`{}`),
			AnswerJSON:  json.RawMessage(`{"answer":true}`),
		},
	}
	meta := PlanMeta{
		Title: "第一章训练", Description: "示例计划", Tags: []string{"训练"},
		Chapters: []PlanChapter{
			{Title: "热身", OrderNo: 1, ProblemIDs: []int{0}},
			{Title: "进阶", OrderNo: 2, ProblemIDs: []int{1, 2}},
		},
	}
	return problems, meta
}

func jsonCompact(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("compact: %v", err)
	}
	return buf.String()
}

func TestBuildParseRoundTrip(t *testing.T) {
	problems, meta := sampleProblems()
	resolve := func(name string) ([]byte, error) {
		if name == "abcd1234.png" {
			return []byte("PNGDATA"), nil
		}
		return nil, fmtErrNotFound()
	}
	zipData, err := BuildZip(problems, &meta, resolve)
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}

	got, gotMeta, images, err := ParseZip(zipData)
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	if len(got) != len(problems) {
		t.Fatalf("problem count = %d, want %d", len(got), len(problems))
	}
	for i := range problems {
		if got[i].Type != problems[i].Type || got[i].Title != problems[i].Title ||
			got[i].StatementMD != problems[i].StatementMD ||
			got[i].TimeLimitMS != problems[i].TimeLimitMS ||
			got[i].MemoryLimitMiB != problems[i].MemoryLimitMiB {
			t.Errorf("problem %d scalar mismatch:\n got %+v\nwant %+v", i, got[i], problems[i])
		}
		if !reflect.DeepEqual([]string(got[i].Tags), []string(problems[i].Tags)) &&
			!(len(got[i].Tags) == 0 && len(problems[i].Tags) == 0) {
			t.Errorf("problem %d tags = %v, want %v", i, got[i].Tags, problems[i].Tags)
		}
		for _, field := range []struct {
			name      string
			got, want json.RawMessage
		}{
			{"bodyJson", got[i].BodyJSON, problems[i].BodyJSON},
			{"answerJson", got[i].AnswerJSON, problems[i].AnswerJSON},
			{"solutions", got[i].Solutions, problems[i].Solutions},
		} {
			if field.want == nil && len(field.got) == 0 {
				continue
			}
			if jsonCompact(t, field.got) != jsonCompact(t, field.want) {
				t.Errorf("problem %d %s mismatch:\n got %s\nwant %s", i, field.name,
					jsonCompact(t, field.got), jsonCompact(t, field.want))
			}
		}
	}
	if gotMeta == nil || gotMeta.Title != meta.Title || gotMeta.Description != meta.Description {
		t.Fatalf("meta mismatch: %+v", gotMeta)
	}
	if !reflect.DeepEqual(gotMeta.Chapters, meta.Chapters) {
		t.Errorf("chapters mismatch:\n got %+v\nwant %+v", gotMeta.Chapters, meta.Chapters)
	}
	if img, ok := images["abcd1234.png"]; !ok || string(img) != "PNGDATA" {
		t.Errorf("images[abcd1234.png] missing or wrong: %q %v", img, ok)
	}
}

func fmtErrNotFound() error { return errNotFound{} }

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }

func TestBareExportHasNoPlanFile(t *testing.T) {
	problems, _ := sampleProblems()
	zipData, err := BuildZip(problems, nil, nil)
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}
	if bytes.Contains(zipData, []byte(PlanJSONName)) {
		t.Errorf("bare export should not contain trainingPlan.json")
	}
	got, meta, _, err := ParseZip(zipData)
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	if meta != nil {
		t.Errorf("meta should be nil, got %+v", meta)
	}
	if len(got) != 3 {
		t.Errorf("problems = %d, want 3", len(got))
	}
}

func TestParseAnyDepth(t *testing.T) {
	// 兼容旧版 ZIP：文件可位于任意目录层级。
	var buf bytes.Buffer
	zw := newZipWriter(&buf)
	mustCreate(zw, "sub/dir/problems.json", `[]`)
	mustCreate(zw, "deep/nested/trainingPlan.json", `{"chapters":[],"title":"旧版"}`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_, meta, _, err := ParseZip(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	if meta == nil || meta.Title != "旧版" {
		t.Fatalf("meta = %+v, want title 旧版", meta)
	}
}

func TestImportRewrite(t *testing.T) {
	p := ExportProblem{
		StatementMD: "看图 (images/x-9_0.png) 与 (images/y.jpeg)",
		Solutions:   json.RawMessage(`[{"language":"python","code":"","markdown":"(images/z.webp)"}]`),
	}
	ApplyImportRewrite(&p)
	if p.StatementMD != "看图 (/api/uploads/x-9_0.png) 与 (/api/uploads/y.jpeg)" {
		t.Errorf("statementMd rewrite wrong: %q", p.StatementMD)
	}
	if !strings.Contains(string(p.Solutions), "(/api/uploads/z.webp)") {
		t.Errorf("solutions rewrite wrong: %s", p.Solutions)
	}
}

func TestNormalizePayload(t *testing.T) {
	// 编程题默认限制
	p := ProblemPayload{Type: "Programming", Title: "  T1 "}
	if err := NormalizeProblemPayload(&p); err != nil {
		t.Fatalf("normalize programming: %v", err)
	}
	if p.Type != "programming" || p.TimeLimitMS != 1000 || p.MemoryLimitMiB != 256 {
		t.Errorf("defaults wrong: %+v", p)
	}
	if string(p.BodyJSON) != `{}` || string(p.Solutions) != `[]` {
		t.Errorf("empty defaults wrong: %s %s", p.BodyJSON, p.Solutions)
	}

	// 单选：答案文本匹配选项 → answerIndex；非法下标保留原值
	p = ProblemPayload{
		Type: "single_choice", Title: "Q",
		BodyJSON:   json.RawMessage(`{"options":["甲","乙"]}`),
		AnswerJSON: json.RawMessage(`{"answer":"乙"}`),
	}
	if err := NormalizeProblemPayload(&p); err != nil {
		t.Fatalf("normalize single_choice: %v", err)
	}
	if string(p.AnswerJSON) != `{"answerIndex":1}` {
		t.Errorf("answerJson = %s, want {\"answerIndex\":1}", p.AnswerJSON)
	}

	// 判断：value 键强转布尔
	p = ProblemPayload{Type: "true_false", Title: "T", AnswerJSON: json.RawMessage(`{"value":"false","note":"x"}`)}
	if err := NormalizeProblemPayload(&p); err != nil {
		t.Fatalf("normalize true_false: %v", err)
	}
	if !strings.Contains(string(p.AnswerJSON), `"answer":false`) {
		t.Errorf("answerJson = %s, want answer:false kept note", p.AnswerJSON)
	}

	// 题解语言别名 + 空语言丢弃
	p = ProblemPayload{
		Type: "programming", Title: "S",
		Solutions: json.RawMessage(`[{"language":"C++","code":"a"},{"language":"golang","code":"b"},{"language":"","code":"c"}]`),
	}
	if err := NormalizeProblemPayload(&p); err != nil {
		t.Fatalf("normalize solutions: %v", err)
	}
	want := `[{"language":"cpp","code":"a","markdown":""},{"language":"go","code":"b","markdown":""}]`
	if jsonCompact(t, p.Solutions) != want {
		t.Errorf("solutions = %s, want %s", jsonCompact(t, p.Solutions), want)
	}

	// 非法题型 / 缺标题
	p = ProblemPayload{Type: "essay", Title: "X"}
	if err := NormalizeProblemPayload(&p); err == nil {
		t.Errorf("invalid type should fail")
	}
	p = ProblemPayload{Type: "programming"}
	if err := NormalizeProblemPayload(&p); err == nil {
		t.Errorf("missing title should fail")
	}
}

func TestNormalizeSolutionsRejectsNonArray(t *testing.T) {
	_, err := NormalizeSolutions(json.RawMessage(`{"language":"cpp"}`))
	if err == nil || !strings.Contains(err.Error(), "JSON array") {
		t.Errorf("want JSON array error, got %v", err)
	}
}

func mustCreate(w *zipWriterT, name, content string) {
	if err := w.create(name, content); err != nil {
		panic(err)
	}
}
