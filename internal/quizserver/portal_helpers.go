// 门户辅助：空间解析变体、刷题题目解析、随机数。
package quizserver

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
)

// resolveSpaceCtx 供无 :id 路径参数端点使用：校验 user 是 spaceID 成员或管理员并注入。
func (s *Server) resolveSpaceCtx(c *fiber.Ctx, spaceID int64) (int64, error) {
	user := currentUser(c)
	if isAdminRole(user.Role) {
		if user.Role == accounts.RoleDomainAdmin && user.DomainID != nil {
			ok, err := s.QS.Repo.SpaceOfDomain(spaceID, *user.DomainID)
			if err != nil {
				return 0, err
			}
			if !ok {
				return 0, respondError(c, fiber.StatusForbidden, "无权访问该空间")
			}
		}
		c.Locals(spaceLocals, spaceID)
		return spaceID, nil
	}
	ok, err := s.QS.Repo.SpaceMember(spaceID, user.ID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, respondError(c, fiber.StatusForbidden, "你不是该空间成员")
	}
	c.Locals(spaceLocals, spaceID)
	return spaceID, nil
}

// quizSpaceOf 刷题项目所属空间（不存在报错）。
func (s *Server) quizSpaceOf(qid int64) (int64, error) {
	var spaceID int64
	err := s.QS.Repo.DB.QueryRow(`SELECT space_id FROM space_quizzes WHERE id=?`, qid).Scan(&spaceID)
	return spaceID, err
}

// quizProblemIDs 解析刷题项目题集（tags 源=按标签筛域题库单选/判断；repo 源=模板条目客观题）。
// 域范围：项目空间所属域。
func (s *Server) quizProblemIDs(qid int64) ([]int64, error) {
	var (
		spaceID    int64
		sourceType string
		repoKind   string
		repoID     int64
		tagsJSON   string
	)
	err := s.QS.Repo.DB.QueryRow(`SELECT space_id,source_type,repo_kind,repo_id,tags_json
		FROM space_quizzes WHERE id=?`, qid).
		Scan(&spaceID, &sourceType, &repoKind, &repoID, &tagsJSON)
	if err != nil {
		return nil, err
	}
	domainID, err := s.QS.Repo.SpaceDomain(spaceID)
	if err != nil {
		return nil, err
	}
	if sourceType == "tags" {
		var tags []string
		_ = jsonUnmarshalTags(tagsJSON, &tags)
		rows, err := s.QS.Repo.DB.Query(`SELECT id FROM problems
			WHERE domain_id=? AND type IN ('single_choice','true_false') ORDER BY id`, domainID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			if len(tags) == 0 {
				ids = append(ids, id)
				continue
			}
			// tags_json 内包含任一选中标签（前缀粗匹配）
			matched := false
			for _, t := range tags {
				if strings.Contains(tagsJSON, "\""+t+"\"") {
					matched = true
					break
				}
			}
			if matched {
				ids = append(ids, id)
			}
		}
		return ids, nil
	}
	// repo 源：模板（仓库训练/练习）条目 → 客观题
	if repoKind == "training" {
		rows, err := s.QS.Repo.DB.Query(`SELECT i.problem_id FROM training_items i
			JOIN training_chapters c ON i.chapter_id=c.id
			JOIN problems p ON p.id=i.problem_id
			WHERE c.training_id=? AND p.type IN ('single_choice','true_false')
			ORDER BY i.id`, repoID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, nil
	}
	rows, err := s.QS.Repo.DB.Query(`SELECT i.problem_id FROM practice_items i
		JOIN problems p ON p.id=i.problem_id
		WHERE i.practice_id=? AND p.type IN ('single_choice','true_false')
		ORDER BY i.id`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func jsonUnmarshalTags(s string, out *[]string) error {
	if strings.TrimSpace(s) == "" {
		*out = []string{}
		return nil
	}
	return json.Unmarshal([]byte(s), out)
}

// rankDomainOf 排行榜域：member 默认其任一空间的域（取第一个）；管理员可带 domainId query。
func (s *Server) rankDomainOf(c *fiber.Ctx, user *accounts.User) (int64, error) {
	if raw := strings.TrimSpace(c.Query("domainId")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, fiber.NewError(fiber.StatusBadRequest, "invalid domainId")
		}
		return id, nil
	}
	if user.Role == accounts.RoleDomainAdmin && user.DomainID != nil {
		return *user.DomainID, nil
	}
	spaces, err := s.QS.Repo.UserDomainSpaceIDs(user.ID)
	if err != nil {
		return 0, err
	}
	if len(spaces) == 0 {
		return 0, fiber.NewError(fiber.StatusBadRequest, "当前用户未加入任何空间")
	}
	return spaces[0].DomainID, nil
}

// randIntn 密码学安全随机 [0,n)。
func randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	bi, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(bi.Int64())
}
