// RepoReader 空间内容只读访问（门户用）：读取主库中的空间/域结构、空间训练/练习/刷题
// 定义与题目域归属校验。只 SELECT，不写入主库。
package quizstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// nonNilSlice 空结果返回空切片而非 nil——nil 切片 JSON 序列化为 null，
// 前端对数组字段做 flatMap/map/filter 时会崩溃（如训练无条目时 chapters[].items=null）。
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// SpaceBrief 门户空间视图。
type SpaceBrief struct {
	ID         int64  `json:"id"`
	DomainID   int64  `json:"domainId"`
	DomainName string `json:"domainName,omitempty"`
	Name       string `json:"name"`
	// DefaultLang 空间默认编程语言：''=未设置（做题页沿用 python）；仅 'python' / 'cpp'。
	// 用户在某题上手动选过语言（本地记忆）时优先于该默认值。
	DefaultLang string `json:"defaultLang"`
	// CanViewLeaderboard 当前用户能否查看该空间所属域的排行榜
	// （管理员恒 true；成员取决于域的排行榜公开设置）——前端据此隐藏入口
	CanViewLeaderboard bool `json:"canViewLeaderboard"`
}

// SpaceTrainingBrief 空间训练列表项。
type SpaceTrainingBrief struct {
	ID           int64    `json:"id"`
	UUID         string   `json:"uuid,omitempty"`
	SpaceID      int64    `json:"spaceId"`
	IsPublic     bool     `json:"isPublic"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	MaxAttempts  int      `json:"maxAttempts"`
	ProblemCount int      `json:"problemCount"`
}

// SpaceTrainingChapter 空间训练章节（含条目题目简讯）。
type SpaceTrainingChapter struct {
	ID      int64               `json:"id"`
	Title   string              `json:"title"`
	OrderNo int                 `json:"orderNo"`
	Items   []SpaceTrainingItem `json:"items"`
}

// SpaceTrainingItem 空间训练条目（题目 id + 类型，作答判定用）。
type SpaceTrainingItem struct {
	ID           int64  `json:"id"`
	ProblemID    int64  `json:"problemId"`
	OrderNo      int    `json:"orderNo"`
	ProblemTitle string `json:"problemTitle,omitempty"`
	ProblemType  string `json:"problemType,omitempty"`
	ProblemUUID  string `json:"problemUuid,omitempty"`
}

// SpacePracticeBrief 空间练习列表项。
type SpacePracticeBrief struct {
	ID           int64    `json:"id"`
	UUID         string   `json:"uuid,omitempty"`
	SpaceID      int64    `json:"spaceId"`
	IsPublic     bool     `json:"isPublic"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	ProblemCount int      `json:"problemCount"`
}

// SpacePracticeItem 空间练习条目。
type SpacePracticeItem struct {
	ID           int64  `json:"id"`
	ProblemID    int64  `json:"problemId"`
	OrderNo      int    `json:"orderNo"`
	ProblemTitle string `json:"problemTitle,omitempty"`
	ProblemType  string `json:"problemType,omitempty"`
	ProblemUUID  string `json:"problemUuid,omitempty"`
}

// SpaceQuizBrief 空间刷题项目。
type SpaceQuizBrief struct {
	ID           int64    `json:"id"`
	UUID         string   `json:"uuid,omitempty"`
	SpaceID      int64    `json:"spaceId"`
	IsPublic     bool     `json:"isPublic"`
	Title        string   `json:"title"`
	Tags         []string `json:"tags"`
	SourceType   string   `json:"sourceType"`
	RepoKind     string   `json:"repoKind,omitempty"`
	RepoID       int64    `json:"repoId,omitempty"`
	RoundSize    int      `json:"roundSize"`
	ProblemCount int      `json:"problemCount"`
}

// SpaceOfDomain 空间属于域？
func (r *RepoReader) SpaceOfDomain(spaceID, domainID int64) (bool, error) {
	var n int
	err := r.DB.QueryRow(`SELECT COUNT(1) FROM spaces WHERE id=? AND domain_id=?`, spaceID, domainID).Scan(&n)
	return n > 0, err
}

// SpaceDomain 空间所属域（作答/排行榜域校验）。
func (r *RepoReader) SpaceDomain(spaceID int64) (int64, error) {
	var d int64
	err := r.DB.QueryRow(`SELECT domain_id FROM spaces WHERE id=?`, spaceID).Scan(&d)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return d, err
}

// UserDomainSpaceIDs 用户加入的全部空间（门户切换；space_members 在主库）。
func (r *RepoReader) UserDomainSpaceIDs(userID int64) ([]SpaceBrief, error) {
	rows, err := r.DB.Query(`SELECT sp.id,sp.domain_id,d.name,sp.name,sp.default_lang FROM space_members m
		JOIN spaces sp ON sp.id=m.space_id
		LEFT JOIN domains d ON d.id=sp.domain_id
		WHERE m.user_id=? ORDER BY sp.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceBrief
	for rows.Next() {
		var b SpaceBrief
		var dName sql.NullString
		if err := rows.Scan(&b.ID, &b.DomainID, &dName, &b.Name, &b.DefaultLang); err != nil {
			return nil, err
		}
		if dName.Valid {
			b.DomainName = dName.String
		}
		out = append(out, b)
	}
	return nonNilSlice(out), rows.Err()
}

// SpaceMember 是否空间成员（成员访问校验）。
func (r *RepoReader) SpaceMember(spaceID, userID int64) (bool, error) {
	var n int
	err := r.DB.QueryRow(`SELECT COUNT(1) FROM space_members WHERE space_id=? AND user_id=?`, spaceID, userID).Scan(&n)
	return n > 0, err
}

// DomainSpaceIDs 域内全部空间 id（排行榜域范围/域管理员内容管理）。
func (r *RepoReader) DomainSpaceIDs(domainID int64) ([]int64, error) {
	rows, err := r.DB.Query(`SELECT id FROM spaces WHERE domain_id=? ORDER BY id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return nonNilSlice(out), rows.Err()
}

// ---------- 空间训练/练习/刷题结构（门户只读） ----------

// visibleClause 可见性过滤 SQL 片段（userID<=0 = 管理员不过滤）。
// member（userID>0）：is_public=1（空间全体成员可见，公开项）直接放行；
// 否则须命中可见名单 v。调用方已先行校验空间成员身份（resolveSpaceCtx），
// 故此处无需重复成员判定。
func visibleClause(table, kind, alias string, userID int64) string {
	if userID <= 0 {
		return ""
	}
	return fmt.Sprintf(" AND (is_public=1 OR EXISTS(SELECT 1 FROM %s v WHERE v.%s_id=%s.id AND v.user_id=%d))", table, kind, alias, userID)
}

// ListSpaceTrainingsBrief 空间训练列表（含题量；member：公开项+已分配可见的）。
func (r *RepoReader) ListSpaceTrainingsBrief(spaceID, userID int64) ([]SpaceTrainingBrief, error) {
	rows, err := r.DB.Query(`SELECT t.id,t.uuid,t.space_id,t.title,t.description,t.tags_json,t.max_attempts,t.is_public,
		(SELECT COUNT(*) FROM space_training_items i JOIN space_training_chapters c ON i.chapter_id=c.id WHERE c.training_id=t.id)
		FROM space_trainings t WHERE t.space_id=?`+visibleClause("space_training_visible", "training", "t", userID)+` ORDER BY t.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceTrainingBrief
	for rows.Next() {
		var b SpaceTrainingBrief
		var tags string
		var pub int
		if err := rows.Scan(&b.ID, &b.UUID, &b.SpaceID, &b.Title, &b.Description, &tags, &b.MaxAttempts, &pub, &b.ProblemCount); err != nil {
			return nil, err
		}
		b.IsPublic = pub != 0
		b.Tags = decodeRepoTags(tags)
		out = append(out, b)
	}
	return nonNilSlice(out), rows.Err()
}

// GetSpaceTrainingBrief 单训练（含章节+条目题目类型/uuid——作答限次判定）。
// userID>0 时校验可见性（member：公开项或已分配；否则 ErrNotFound）。
func (r *RepoReader) GetSpaceTrainingBrief(trainingID, userID int64) (*SpaceTrainingBrief, []SpaceTrainingChapter, error) {
	var b SpaceTrainingBrief
	var tags string
	var pub int
	err := r.DB.QueryRow(`SELECT t.id,t.uuid,t.space_id,t.title,t.description,t.tags_json,t.max_attempts,t.is_public,
		(SELECT COUNT(*) FROM space_training_items i JOIN space_training_chapters c ON i.chapter_id=c.id WHERE c.training_id=t.id)
		FROM space_trainings t WHERE t.id=?`+visibleClause("space_training_visible", "training", "t", userID)+``, trainingID).
		Scan(&b.ID, &b.UUID, &b.SpaceID, &b.Title, &b.Description, &tags, &b.MaxAttempts, &pub, &b.ProblemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	b.IsPublic = pub != 0
	b.Tags = decodeRepoTags(tags)
	rows, err := r.DB.Query(`SELECT id,title,order_no FROM space_training_chapters WHERE training_id=? ORDER BY order_no,id`, trainingID)
	if err != nil {
		return nil, nil, err
	}
	var chapters []SpaceTrainingChapter
	for rows.Next() {
		var ch SpaceTrainingChapter
		if err := rows.Scan(&ch.ID, &ch.Title, &ch.OrderNo); err != nil {
			rows.Close()
			return nil, nil, err
		}
		chapters = append(chapters, ch)
	}
	rows.Close()
	for i := range chapters {
		items, err := r.spaceTrainingItems(chapters[i].ID)
		if err != nil {
			return nil, nil, err
		}
		chapters[i].Items = items
	}
	return &b, nonNilSlice(chapters), nil
}

func (r *RepoReader) spaceTrainingItems(chapterID int64) ([]SpaceTrainingItem, error) {
	rows, err := r.DB.Query(`SELECT i.id,i.problem_id,i.order_no,p.title,p.type,p.uuid
		FROM space_training_items i LEFT JOIN problems p ON p.id=i.problem_id
		WHERE i.chapter_id=? ORDER BY i.order_no,i.id`, chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceTrainingItem
	for rows.Next() {
		var it SpaceTrainingItem
		var titleN, tN, uN sql.NullString
		if err := rows.Scan(&it.ID, &it.ProblemID, &it.OrderNo, &titleN, &tN, &uN); err != nil {
			return nil, err
		}
		if titleN.Valid {
			it.ProblemTitle = titleN.String
		}
		if tN.Valid {
			it.ProblemType = tN.String
		}
		if uN.Valid {
			it.ProblemUUID = uN.String
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 空结果返回空数组而非 nil（nil 经 JSON 序列化为 null，前端 flatMap/filter 会崩）
	if out == nil {
		out = []SpaceTrainingItem{}
	}
	return out, nil
}

// ListSpacePracticesBrief 空间练习列表（member：公开项+已分配的）。
func (r *RepoReader) ListSpacePracticesBrief(spaceID, userID int64) ([]SpacePracticeBrief, error) {
	rows, err := r.DB.Query(`SELECT p.id,p.uuid,p.space_id,p.title,p.description,p.tags_json,p.is_public,
		(SELECT COUNT(*) FROM space_practice_items i WHERE i.practice_id=p.id)
		FROM space_practices p WHERE p.space_id=?`+visibleClause("space_practice_visible", "practice", "p", userID)+` ORDER BY p.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpacePracticeBrief
	for rows.Next() {
		var b SpacePracticeBrief
		var tags string
		var pub int
		if err := rows.Scan(&b.ID, &b.UUID, &b.SpaceID, &b.Title, &b.Description, &tags, &pub, &b.ProblemCount); err != nil {
			return nil, err
		}
		b.IsPublic = pub != 0
		b.Tags = decodeRepoTags(tags)
		out = append(out, b)
	}
	return nonNilSlice(out), rows.Err()
}

// GetSpacePracticeBrief 单练习（含条目；userID>0 校验可见性：公开项或已分配）。
func (r *RepoReader) GetSpacePracticeBrief(practiceID, userID int64) (*SpacePracticeBrief, []SpacePracticeItem, error) {
	var b SpacePracticeBrief
	var tags string
	var pub int
	err := r.DB.QueryRow(`SELECT p.id,p.uuid,p.space_id,p.title,p.description,p.tags_json,p.is_public,
		(SELECT COUNT(*) FROM space_practice_items i WHERE i.practice_id=p.id)
		FROM space_practices p WHERE p.id=?`+visibleClause("space_practice_visible", "practice", "p", userID)+``, practiceID).
		Scan(&b.ID, &b.UUID, &b.SpaceID, &b.Title, &b.Description, &tags, &pub, &b.ProblemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	b.IsPublic = pub != 0
	b.Tags = decodeRepoTags(tags)
	rows, err := r.DB.Query(`SELECT i.id,i.problem_id,i.order_no,p.title,p.type,p.uuid
		FROM space_practice_items i LEFT JOIN problems p ON p.id=i.problem_id
		WHERE i.practice_id=? ORDER BY i.order_no,i.id`, practiceID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var items []SpacePracticeItem
	for rows.Next() {
		var it SpacePracticeItem
		var titleN, tN, uN sql.NullString
		if err := rows.Scan(&it.ID, &it.ProblemID, &it.OrderNo, &titleN, &tN, &uN); err != nil {
			return nil, nil, err
		}
		if titleN.Valid {
			it.ProblemTitle = titleN.String
		}
		if tN.Valid {
			it.ProblemType = tN.String
		}
		if uN.Valid {
			it.ProblemUUID = uN.String
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	// 空结果返回空数组而非 nil（JSON null 会让前端 flatMap/filter 崩溃）
	if items == nil {
		items = []SpacePracticeItem{}
	}
	return &b, items, nil
}

// ListSpaceQuizzesBrief 空间刷题项目列表（member：公开项+已分配的）。
func (r *RepoReader) ListSpaceQuizzesBrief(spaceID, userID int64) ([]SpaceQuizBrief, error) {
	rows, err := r.DB.Query(`SELECT id,uuid,space_id,title,tags_json,source_type,repo_kind,repo_id,round_size,is_public
		FROM space_quizzes WHERE space_id=?`+visibleClause("space_quiz_visible", "quiz", "space_quizzes", userID)+` ORDER BY id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceQuizBrief
	for rows.Next() {
		var b SpaceQuizBrief
		var tags string
		var pub int
		if err := rows.Scan(&b.ID, &b.UUID, &b.SpaceID, &b.Title, &tags, &b.SourceType, &b.RepoKind, &b.RepoID, &b.RoundSize, &pub); err != nil {
			return nil, err
		}
		b.IsPublic = pub != 0
		b.Tags = decodeRepoTags(tags)
		out = append(out, b)
	}
	return nonNilSlice(out), rows.Err()
}

// QuizVisibleForUser 该用户是否可见某刷题项目（作答流校验；userID<=0 管理员恒可见）。
// is_public=1 → 空间全体成员可见（空间成员身份由调用方 resolveSpaceCtx 校验）；
// 否则查可见名单。
func (r *RepoReader) QuizVisibleForUser(quizID, userID int64) (bool, error) {
	if userID <= 0 {
		return true, nil
	}
	var isPublic int
	err := r.DB.QueryRow(`SELECT is_public FROM space_quizzes WHERE id=?`, quizID).Scan(&isPublic)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if isPublic == 1 {
		return true, nil
	}
	var n int
	err = r.DB.QueryRow(`SELECT COUNT(1) FROM space_quiz_visible v WHERE v.quiz_id=? AND v.user_id=?`, quizID, userID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// TrainingVisibleForUser / PracticeVisibleForUser 作答流校验（管理员恒可见；
// is_public=1 → 空间全体成员可见，否则查可见名单）。
func (r *RepoReader) TrainingVisibleForUser(trainingID, userID int64) (bool, error) {
	if userID <= 0 {
		return true, nil
	}
	var isPublic int
	err := r.DB.QueryRow(`SELECT is_public FROM space_trainings WHERE id=?`, trainingID).Scan(&isPublic)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if isPublic == 1 {
		return true, nil
	}
	var n int
	err = r.DB.QueryRow(`SELECT COUNT(1) FROM space_training_visible v WHERE v.training_id=? AND v.user_id=?`, trainingID, userID).Scan(&n)
	return n > 0, err
}

func (r *RepoReader) PracticeVisibleForUser(practiceID, userID int64) (bool, error) {
	if userID <= 0 {
		return true, nil
	}
	var isPublic int
	err := r.DB.QueryRow(`SELECT is_public FROM space_practices WHERE id=?`, practiceID).Scan(&isPublic)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if isPublic == 1 {
		return true, nil
	}
	var n int
	err = r.DB.QueryRow(`SELECT COUNT(1) FROM space_practice_visible v WHERE v.practice_id=? AND v.user_id=?`, practiceID, userID).Scan(&n)
	return n > 0, err
}

// decodeRepoTags 解析主库 tags_json（与 store.decodeTags 等价，避免跨包私有依赖）。
func decodeRepoTags(s string) []string {
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil || tags == nil {
		return []string{}
	}
	return tags
}
