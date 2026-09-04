// Package zipio 是 OrangeOJ ZIP 交换格式的唯一权威实现。
// 格式基线：上游 export_handlers.go / objective_answers.go / queue.go（见 docs/aegis/specs）。
package zipio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// ImageResolver 按文件名取图片内容；返回错误则跳过该图片（与上游一致）。
type ImageResolver func(filename string) ([]byte, error)

const (
	ProblemsJSONName = "problems.json"
	PlanJSONName     = "trainingPlan.json"
)

// imageRefPattern 匹配 /api/uploads/<file> 图片引用（与上游正则一致，
// 文件名可为 hex、UUID 或旧版序号）。
var imageRefPattern = regexp.MustCompile(`/api/uploads/([a-zA-Z0-9_-]+\.(?:png|jpe?g|gif|webp|svg))`)

// imagesPathPattern 匹配导入包内相对图片引用 (images/<file>)。
var imagesPathPattern = regexp.MustCompile(`\(images/`)

// ExportProblem 与上游 problemExportEntry 字段完全一致。
type ExportProblem struct {
	Type           string          `json:"type"`
	Title          string          `json:"title"`
	Tags           []string        `json:"tags"`
	StatementMD    string          `json:"statementMd"`
	BodyJSON       json.RawMessage `json:"bodyJson"`
	AnswerJSON     json.RawMessage `json:"answerJson"`
	Solutions      json.RawMessage `json:"solutions,omitempty"`
	TimeLimitMS    int             `json:"timeLimitMs,omitempty"`
	MemoryLimitMiB int             `json:"memoryLimitMiB,omitempty"`
}

// PlanChapter 与上游 trainingPlanChapterJSON 一致；ProblemIDs 为 problems.json 数组下标。
type PlanChapter struct {
	Title      string `json:"title"`
	OrderNo    int    `json:"orderNo"`
	ProblemIDs []int  `json:"problemIds"`
}

// PlanMeta 与上游 importedTrainingPlanMeta 一致。
type PlanMeta struct {
	Title       string        `json:"title,omitempty"`
	Description string        `json:"description,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	Chapters    []PlanChapter `json:"chapters,omitempty"`
}

// ProblemPayload 服务端题目请求体（导出条目 + API 创建/更新共用）。
type ProblemPayload struct {
	Type           string          `json:"type"`
	Title          string          `json:"title"`
	Tags           []string        `json:"tags"`
	StatementMD    string          `json:"statementMd"`
	BodyJSON       json.RawMessage `json:"bodyJson"`
	AnswerJSON     json.RawMessage `json:"answerJson"`
	Solutions      json.RawMessage `json:"solutions"`
	TimeLimitMS    int             `json:"timeLimitMs"`
	MemoryLimitMiB int             `json:"memoryLimitMiB"`
}

// ---------- 图片引用 ----------

// CollectImageRefs 收集各文本字段中的图片文件名，去重保序。
func CollectImageRefs(fields ...string) []string {
	seen := map[string]bool{}
	var refs []string
	for _, field := range fields {
		for _, m := range imageRefPattern.FindAllStringSubmatch(field, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				refs = append(refs, m[1])
			}
		}
	}
	return refs
}

// RewriteRefsForImport 将包内相对引用 (images/<file>) 重写为 (/api/uploads/<file>)。
func RewriteRefsForImport(s string) string {
	return imagesPathPattern.ReplaceAllString(s, "(/api/uploads/")
}

// ApplyImportRewrite 对题目全部文本字段执行导入重写。
func ApplyImportRewrite(p *ExportProblem) {
	p.StatementMD = RewriteRefsForImport(p.StatementMD)
	if len(p.BodyJSON) > 0 {
		p.BodyJSON = json.RawMessage(RewriteRefsForImport(string(p.BodyJSON)))
	}
	if len(p.AnswerJSON) > 0 {
		p.AnswerJSON = json.RawMessage(RewriteRefsForImport(string(p.AnswerJSON)))
	}
	if len(p.Solutions) > 0 {
		p.Solutions = json.RawMessage(RewriteRefsForImport(string(p.Solutions)))
	}
}

// ---------- ZIP 构建 ----------

// marshalNoEscape 与上游一致：2 空格缩进、不转义 HTML。
func marshalNoEscape(v any) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildZip 构建 OrangeOJ 兼容 ZIP：
//   - problems.json 必有
//   - trainingPlan.json 仅当 meta 非空且（有章节 或 有标题 或 有标签）时写出
//   - images/ 收集四个文本字段中引用到的图片
func BuildZip(problems []ExportProblem, meta *PlanMeta, resolve ImageResolver) ([]byte, error) {
	return buildZip(problems, meta, resolve, nil)
}

// BuildZipWithFiles 在 BuildZip 基础上附加任意扩展文件（如全库备份 orangerepo-backup.json）。
// OrangeOJ 及本仓库旧逻辑不识别附加文件时会自然忽略，保持兼容。
func BuildZipWithFiles(problems []ExportProblem, meta *PlanMeta, resolve ImageResolver, extraFiles map[string][]byte) ([]byte, error) {
	return buildZip(problems, meta, resolve, extraFiles)
}

func buildZip(problems []ExportProblem, meta *PlanMeta, resolve ImageResolver, extraFiles map[string][]byte) ([]byte, error) {
	if problems == nil {
		problems = []ExportProblem{}
	}
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	problemsJSON, err := marshalNoEscape(problems)
	if err != nil {
		return nil, err
	}
	pf, err := w.Create(ProblemsJSONName)
	if err != nil {
		return nil, err
	}
	if _, err := pf.Write(problemsJSON); err != nil {
		return nil, err
	}

	if meta != nil && (len(meta.Chapters) > 0 || meta.Title != "" || len(meta.Tags) > 0) {
		planData := map[string]any{"chapters": planChaptersOrEmpty(meta.Chapters)}
		if meta.Title != "" {
			planData["title"] = meta.Title
		}
		if meta.Description != "" {
			planData["description"] = meta.Description
		}
		if len(meta.Tags) > 0 {
			planData["tags"] = meta.Tags
		}
		planJSON, err := marshalNoEscape(planData)
		if err != nil {
			return nil, err
		}
		tf, err := w.Create(PlanJSONName)
		if err != nil {
			return nil, err
		}
		if _, err := tf.Write(planJSON); err != nil {
			return nil, err
		}
	}

	for _, p := range problems {
		refs := CollectImageRefs(p.StatementMD, string(p.BodyJSON), string(p.AnswerJSON), string(p.Solutions))
		for _, filename := range refs {
			if resolve == nil {
				continue
			}
			data, err := resolve(filename)
			if err != nil {
				continue
			}
			ff, err := w.Create("images/" + filename)
			if err != nil {
				return nil, err
			}
			if _, err := ff.Write(data); err != nil {
				return nil, err
			}
		}
	}

	// 扩展文件写至包根（文件名须唯一，避免与 problems.json/trainingPlan.json 冲突）
	written := map[string]bool{ProblemsJSONName: true, PlanJSONName: true}
	for name, data := range extraFiles {
		if name == "" || written[name] {
			continue
		}
		written[name] = true
		ef, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := ef.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func planChaptersOrEmpty(chapters []PlanChapter) []PlanChapter {
	if chapters == nil {
		return []PlanChapter{}
	}
	return chapters
}

// ---------- ZIP 解析 ----------

// findZipFileByNames 在 ZIP 内任意目录层级查找指定文件名（与上游兼容旧版行为一致）。
func findZipFileByNames(r *zip.Reader, name string) ([]byte, bool) {
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if path.Base(strings.ReplaceAll(f.Name, "\\", "/")) == name {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(rc); err != nil {
				rc.Close()
				continue
			}
			rc.Close()
			return buf.Bytes(), true
		}
	}
	return nil, false
}

// ParseZip 解析 OrangeOJ ZIP：题目数组、可选训练计划元数据、images/ 下图片。
func ParseZip(data []byte) (problems []ExportProblem, meta *PlanMeta, images map[string][]byte, err error) {
	problems, meta, images, _, err = ParseZipWithExtra(data)
	return problems, meta, images, err
}

// ParseZipWithExtra 在 ParseZip 基础上额外返回包根的扩展文件（文件名→内容），
// 如全库备份 orangerepo-backup.json；无扩展文件时 extra 为空 map。
func ParseZipWithExtra(data []byte) (problems []ExportProblem, meta *PlanMeta, images map[string][]byte, extra map[string][]byte, err error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("invalid zip file: %w", err)
	}
	raw, ok := findZipFileByNames(r, ProblemsJSONName)
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("missing %s", ProblemsJSONName)
	}
	if err := json.Unmarshal(raw, &problems); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parse %s: %w", ProblemsJSONName, err)
	}
	if planRaw, ok := findZipFileByNames(r, PlanJSONName); ok {
		meta = &PlanMeta{}
		if err := json.Unmarshal(planRaw, meta); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("parse %s: %w", PlanJSONName, err)
		}
	}
	images = map[string][]byte{}
	extra = map[string][]byte{}
	seen := map[string]bool{ProblemsJSONName: true, PlanJSONName: true}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		clean := strings.ReplaceAll(f.Name, "\\", "/")
		base := path.Base(clean)
		dir := path.Dir(clean)
		rc, err := f.Open()
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			rc.Close()
			continue
		}
		rc.Close()
		if dir == "images" || strings.HasSuffix(dir, "/images") {
			if imageRefPattern.MatchString("/api/uploads/" + base) {
				images[base] = buf.Bytes()
			}
			continue
		}
		// 包根的其余文件视为扩展文件（problems.json/trainingPlan.json 已在前面处理）
		if dir == "." && !seen[base] {
			seen[base] = true
			extra[base] = buf.Bytes()
		}
	}
	return problems, meta, images, extra, nil
}

// ---------- 题目载荷归一化（与上游 normalizeProblemPayload 等价） ----------

// NormalizeProblemPayload 归一化并校验题目载荷。
func NormalizeProblemPayload(p *ProblemPayload) error {
	p.Title = strings.TrimSpace(p.Title)
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	if p.Title == "" || p.Type == "" {
		return fmt.Errorf("type and title are required")
	}
	if p.Type != "programming" && p.Type != "single_choice" && p.Type != "true_false" {
		return fmt.Errorf("invalid problem type")
	}
	if p.Type == "programming" {
		if p.TimeLimitMS <= 0 {
			p.TimeLimitMS = 1000
		}
		if p.MemoryLimitMiB <= 0 {
			p.MemoryLimitMiB = 256
		}
	}
	if len(p.BodyJSON) == 0 {
		p.BodyJSON = json.RawMessage(`{}`)
	}
	if len(p.AnswerJSON) == 0 {
		p.AnswerJSON = json.RawMessage(`{}`)
	}
	if len(p.Solutions) == 0 {
		p.Solutions = json.RawMessage(`[]`)
	}
	normalized, err := NormalizeSolutions(p.Solutions)
	if err != nil {
		return err
	}
	p.Solutions = normalized
	if err := normalizeObjectiveAnswer(p); err != nil {
		return err
	}
	return nil
}

// ToExportProblem 转为导出条目（solutions 恒为数组）。
func (p *ProblemPayload) ToExportProblem() ExportProblem {
	return ExportProblem{
		Type:           p.Type,
		Title:          p.Title,
		Tags:           p.Tags,
		StatementMD:    p.StatementMD,
		BodyJSON:       p.BodyJSON,
		AnswerJSON:     p.AnswerJSON,
		Solutions:      p.Solutions,
		TimeLimitMS:    p.TimeLimitMS,
		MemoryLimitMiB: p.MemoryLimitMiB,
	}
}

// NormalizeSolutions 校验并归一化题解数组：非法输入归为 []，
// language 归一化后为空的项丢弃（与上游一致）。
func NormalizeSolutions(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return json.RawMessage(`[]`), nil
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, fmt.Errorf("solutions must be a JSON array")
	}
	type solution struct {
		Language string `json:"language"`
		Code     string `json:"code"`
		Markdown string `json:"markdown"`
	}
	normalized := make([]solution, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		lang := NormalizeSolutionLanguage(toString(item["language"]))
		if lang == "" {
			continue
		}
		normalized = append(normalized, solution{
			Language: lang,
			Code:     toString(item["code"]),
			Markdown: toString(item["markdown"]),
		})
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// NormalizeSolutionLanguage 归一化语言别名（与上游一致）。
func NormalizeSolutionLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "c++", "cpp", "c":
		return "cpp"
	case "python", "python3", "py", "python 3":
		return "python"
	case "go", "golang":
		return "go"
	case "turtle", "python turtle", "pythonturtle":
		return "turtle"
	default:
		return strings.ToLower(strings.TrimSpace(language))
	}
}

// normalizeObjectiveAnswer 归一化客观题答案（与上游 objective_answers.go 等价）。
func normalizeObjectiveAnswer(p *ProblemPayload) error {
	switch p.Type {
	case "single_choice":
		body, err := parseJSONMap(p.BodyJSON)
		if err != nil {
			return err
		}
		answer, err := parseJSONMap(p.AnswerJSON)
		if err != nil {
			return err
		}
		options := optionStrings(body["options"])
		if len(options) == 0 {
			return nil
		}
		if idx, ok := toInt(answer["answerIndex"]); ok && idx >= 0 && idx < len(options) {
			next, err := json.Marshal(map[string]any{"answerIndex": idx})
			if err != nil {
				return err
			}
			p.AnswerJSON = next
		} else if text := toString(answer["answer"]); text != "" {
			for i, opt := range options {
				if strings.EqualFold(strings.TrimSpace(text), strings.TrimSpace(opt)) {
					next, err := json.Marshal(map[string]any{"answerIndex": i})
					if err != nil {
						return err
					}
					p.AnswerJSON = next
					break
				}
			}
		}
	case "true_false":
		answer, err := parseJSONMap(p.AnswerJSON)
		if err != nil {
			return err
		}
		for _, key := range []string{"answer", "correct", "correctAnswer", "value"} {
			v, ok := answer[key]
			if !ok {
				continue
			}
			b, ok := toBool(v)
			if !ok {
				continue
			}
			answer["answer"] = b
			next, err := json.Marshal(answer)
			if err != nil {
				return err
			}
			p.AnswerJSON = next
			break
		}
	}
	return nil
}

func parseJSONMap(raw json.RawMessage) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		return nil, fmt.Errorf("bodyJson/answerJson must be a JSON object")
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func optionStrings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range list {
		out = append(out, toString(item))
	}
	return out
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		i := int(t)
		return i, float64(i) == t
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		return i, err == nil
	case bool:
		return 0, false
	default:
		return 0, false
	}
}

func toBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case float64:
		return t != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "对", "正确":
			return true, true
		case "false", "0", "no", "错", "错误":
			return false, true
		}
		return false, false
	default:
		return false, false
	}
}
