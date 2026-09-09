// Package model 定义 OrangeOJ 的全部数据类型与 JSON 形状。
// JSON 字段一律 camelCase，与上游 OrangeOJ 保持一致。
package model

import (
	"encoding/json"
	"time"
)

// ProblemType 题目类型，取值与上游一致。
type ProblemType string

const (
	TypeProgramming  ProblemType = "programming"
	TypeSingleChoice ProblemType = "single_choice"
	TypeTrueFalse    ProblemType = "true_false"
)

// ValidProblemType 判断题型是否合法。
func ValidProblemType(t ProblemType) bool {
	switch t {
	case TypeProgramming, TypeSingleChoice, TypeTrueFalse:
		return true
	}
	return false
}

// Solution 单条题解：语言、代码、Markdown 解读（与上游 problemSolution 一致）。
type Solution struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Markdown string `json:"markdown"`
}

// Problem 题目完整实体。BodyJSON/AnswerJSON/Solutions 以原始 JSON 存储，
// 结构约束由 zipio 的归一化逻辑负责。UUID 为跨库稳定标识（UUIDv7，导入缺则生成）。
// DomainID 为题目归属域（nil=未归域，仅迁移期存量；新建一律须带）。
// StarterCpp/StarterPy 为编程题起始代码模板（按语言；空=前端用通用模板）。
type Problem struct {
	ID             int64           `json:"id"`
	UUID           string          `json:"uuid,omitempty"`
	DomainID       *int64          `json:"domainId,omitempty"`
	Type           ProblemType     `json:"type"`
	Title          string          `json:"title"`
	Tags           []string        `json:"tags"`
	StatementMD    string          `json:"statementMd"`
	BodyJSON       json.RawMessage `json:"bodyJson"`
	AnswerJSON     json.RawMessage `json:"answerJson"`
	Solutions      json.RawMessage `json:"solutions"`
	StarterCpp     string          `json:"starterCpp,omitempty"`
	StarterPy      string          `json:"starterPy,omitempty"`
	TimeLimitMS    int             `json:"timeLimitMs"`
	MemoryLimitMiB int             `json:"memoryLimitMiB"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// ProblemSummary 列表视图，不含大字段。
type ProblemSummary struct {
	ID             int64       `json:"id"`
	UUID           string      `json:"uuid,omitempty"`
	DomainID       *int64      `json:"domainId,omitempty"`
	Type           ProblemType `json:"type"`
	Title          string      `json:"title"`
	Tags           []string    `json:"tags"`
	TimeLimitMS    int         `json:"timeLimitMs"`
	MemoryLimitMiB int         `json:"memoryLimitMiB"`
	CreatedAt      time.Time   `json:"createdAt"`
}

// BookletDirectory 题册目录：可嵌套的组织结构（训练/练习归属其中，nullable 表示根目录）。
type BookletDirectory struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID *int64 `json:"parentId"`
	OrderNo  int    `json:"orderNo"`
}

// Training 训练计划：章节化的题目编组。
type Training struct {
	ID           int64     `json:"id"`
	UUID         string    `json:"uuid,omitempty"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Tags         []string  `json:"tags"`
	FolderID     *int64    `json:"folderId,omitempty"`
	ProblemCount int       `json:"problemCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Chapter 训练章节。
type Chapter struct {
	ID         int64  `json:"id"`
	TrainingID int64  `json:"trainingId"`
	Title      string `json:"title"`
	OrderNo    int    `json:"orderNo"`
	Items      []Item `json:"items"`
}

// Item 训练章节内的题目条目。
type Item struct {
	ID           int64  `json:"id"`
	ChapterID    int64  `json:"chapterId"`
	ProblemID    int64  `json:"problemId"`
	OrderNo      int    `json:"orderNo"`
	ProblemTitle string `json:"problemTitle,omitempty"`
	ProblemType  string `json:"problemType,omitempty"`
}

// Practice 练习：平铺题目编组（含分值）。
type Practice struct {
	ID           int64     `json:"id"`
	UUID         string    `json:"uuid,omitempty"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Tags         []string  `json:"tags"`
	FolderID     *int64    `json:"folderId,omitempty"`
	ProblemCount int       `json:"problemCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// PracticeItem 练习条目。
type PracticeItem struct {
	ID           int64  `json:"id"`
	PracticeID   int64  `json:"practiceId"`
	ProblemID    int64  `json:"problemId"`
	OrderNo      int    `json:"orderNo"`
	ProblemTitle string `json:"problemTitle,omitempty"`
	ProblemType  string `json:"problemType,omitempty"`
}

// Domain 域：题库数据的逻辑隔离单位（题目/标签域级共享）。
type Domain struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	// LeaderboardPublic 排行榜是否公开（1=普通成员可查看本域排行；0=仅管理员）。
	LeaderboardPublic bool `json:"leaderboardPublic"`
}

// Space 空间：域内的做题组织单位（训练/练习/作答按空间隔离）。
type Space struct {
	ID        int64     `json:"id"`
	DomainID  int64     `json:"domainId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// SpaceMemberView 空间成员视图（user_id + 用户名，用户名由调用方注入或留空）。
type SpaceMemberView struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}
