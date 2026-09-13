// 休息时间小游戏：成绩提交与榜单（本域榜单 / 全域榜单）。
// 权限：登录用户即可提交自己的成绩、查看两种榜单（按产品决定：全域榜单对所有登录用户开放）。
// 域归属由服务端按 spaceId 解析，客户端不能自选域；榜单不受域「排行榜公开」开关约束。
package quizserver

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
	"orangeoj/internal/quizstore"
)

// handleGameScoreSubmit POST /api/portal/game/:game/score
// body: {score:int, spaceId:int?} → 记录成绩（只保留最高分），返回最高分与当前排名。
func (s *Server) handleGameScoreSubmit(c *fiber.Ctx) error {
	user := currentUser(c)
	game := quizstore.NormalizeGameID(c.Params("game"))
	if !quizstore.ValidateGameID(game) {
		return respondError(c, fiber.StatusBadRequest, "游戏标识不合法")
	}
	var req struct {
		Score   *int  `json:"score"`
		SpaceID int64 `json:"spaceId"`
	}
	if err := c.BodyParser(&req); err != nil || req.Score == nil {
		return respondError(c, fiber.StatusBadRequest, "缺少分数")
	}
	// 域归属：按 spaceId 解析空间所属域（拿不到则为空，不阻断提交）
	domainID := s.gameScoreDomain(user, req.SpaceID)

	best, isNewBest, err := s.QS.SubmitGameScore(game, user.ID, domainID, *req.Score)
	if err != nil {
		if errors.Is(err, quizstore.ErrGameScoreTooFrequent) {
			return respondError(c, fiber.StatusTooManyRequests, err.Error())
		}
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}

	// 回带两种榜单各自的排名（前端提交后即可刷新「我的排名」）

	return respondData(c, fiber.StatusOK, fiber.Map{
		"bestScore":  best,
		"isNewBest":  isNewBest,
		"score":      *req.Score,
		"domainId":   domainID,
		"rankDomain": s.gameRankOfUser(game, domainID, user.ID, 50),
		"rankAll":    s.gameRankOfUser(game, 0, user.ID, 50),
	})
}

// gameRankOfUser 取某用户在某榜（domainID=0 为全域）的名次；0 表示暂无记录。
// ListGameRank 的第二个返回值只在「我不在前 limit 内」时给出，在前 limit 内要回到 rows 里找自己，
// 否则提交响应里的名次会恒为 0。
func (s *Server) gameRankOfUser(game string, domainID, userID int64, limit int) int {
	rows, mine, err := s.QS.ListGameRank(game, domainID, userID, limit)
	if err != nil {
		return 0
	}
	if mine != nil {
		return mine.Rank
	}
	for _, r := range rows {
		if r.IsMe {
			return r.Rank
		}
	}
	return 0
}

// handleGameRank GET /api/portal/game/:game/rank?scope=domain|all&spaceId=&limit=
// 本域榜单（scope=domain，按空间所属域）/ 全域榜单（scope=all，所有域）。
func (s *Server) handleGameRank(c *fiber.Ctx) error {
	user := currentUser(c)
	game := quizstore.NormalizeGameID(c.Params("game"))
	if !quizstore.ValidateGameID(game) {
		return respondError(c, fiber.StatusBadRequest, "游戏标识不合法")
	}
	scope := strings.ToLower(strings.TrimSpace(c.Query("scope")))
	if scope == "" {
		scope = "domain"
	}
	if scope != "domain" && scope != "all" {
		return respondError(c, fiber.StatusBadRequest, "scope 仅支持 domain 或 all")
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	var spaceID int64
	if raw := strings.TrimSpace(c.Query("spaceId")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			spaceID = n
		}
	}
	// 本域榜单：域取自当前空间（无空间则取不到 → 该用户看不到本域榜内容，返回空榜）
	domainID := int64(0)
	domainName := ""
	noDomainScope := false
	if scope == "domain" {
		domainID = s.gameScoreDomain(user, spaceID)
		if domainID > 0 {
			_ = s.QS.Repo.DB.QueryRow(`SELECT name FROM domains WHERE id=?`, domainID).Scan(&domainName)
		} else {
			// 用户没有任何空间归属：本域榜单无从谈起。**不能**退化成 domainID=0（那是全域榜单的口径），
			// 否则「本域榜单」会静默显示全部域的数据。
			noDomainScope = true
		}
	}

	if noDomainScope {
		best, plays, err := s.QS.MyGameScore(game, user.ID)
		if err != nil {
			return respondError(c, fiber.StatusInternalServerError, err.Error())
		}
		return respondData(c, fiber.StatusOK, fiber.Map{
			"scope":      scope,
			"game":       game,
			"domainId":   0,
			"domainName": "",
			"noDomain":   true,
			"rows":       []quizstore.GameScore{},
			"myBest":     best,
			"myPlays":    plays,
			"myRank":     0,
			"scopeHint":  "你还没有加入任何空间，暂时看不到本域榜单；可查看全域榜单。",
		})
	}

	rows, mine, err := s.QS.ListGameRank(game, domainID, user.ID, limit)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	best, plays, err := s.QS.MyGameScore(game, user.ID)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, err.Error())
	}
	resp := fiber.Map{
		"scope":      scope,
		"game":       game,
		"domainId":   domainID,
		"domainName": domainName,
		"rows":       rows,
		"myBest":     best,
		"myPlays":    plays,
	}
	if mine != nil {
		resp["myRank"] = mine.Rank
		resp["myRow"] = mine
	} else if best > 0 {
		// 在前 50 内：从 rows 里找自己
		for _, r := range rows {
			if r.IsMe {
				resp["myRank"] = r.Rank
				resp["myRow"] = r
				break
			}
		}
	}
	return respondData(c, fiber.StatusOK, resp)
}

// gameScoreDomain 解析成绩归属域：优先请求里的 spaceId（须该用户可访问），否则取该用户任一空间的域。
// 域由服务端解析，客户端不能自选（避免刷别域榜单）。
func (s *Server) gameScoreDomain(user *accounts.User, spaceID int64) int64 {
	if spaceID > 0 && s.spaceAccessible(user, spaceID) {
		var d int64
		if err := s.QS.Repo.DB.QueryRow(`SELECT domain_id FROM spaces WHERE id=?`, spaceID).Scan(&d); err == nil {
			return d
		}
	}
	// 兜底：该用户加入的第一个空间的域（成员常见情形）
	var id int64
	_ = s.QS.Repo.DB.QueryRow(`SELECT s.domain_id FROM space_members m JOIN spaces s ON s.id=m.space_id
		WHERE m.user_id=? ORDER BY s.id LIMIT 1`, user.ID).Scan(&id)
	return id
}

// spaceAccessible 该用户能否访问该空间（管理员按域；成员按成员关系）。
func (s *Server) spaceAccessible(user *accounts.User, spaceID int64) bool {
	if isAdminRole(user.Role) {
		if user.Role == accounts.RoleDomainAdmin && user.DomainID != nil {
			ok, err := s.QS.Repo.SpaceOfDomain(spaceID, *user.DomainID)
			return err == nil && ok
		}
		return true
	}
	ok, err := s.QS.Repo.SpaceMember(spaceID, user.ID)
	return err == nil && ok
}
