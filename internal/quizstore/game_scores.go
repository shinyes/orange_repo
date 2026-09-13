// 小游戏成绩与榜单（休息时间）：每人每游戏一条记录（最高分 + 游玩次数）。
// 设计要点：
//   - game 维度通用：后端不内置游戏清单，新增小游戏只加前端注册表（榜单按 game 分隔）
//   - 只保留最高分：低分提交不覆盖最高分，但仍累计 plays（用于"玩了多少次"）
//   - 基础校验：分数区间 [0, maxScore] + 提交限频（进程内），不信任前端任意数值
//   - 域归属：提交时按 spaceId 解析空间所属域写入，客户端不能自选域
package quizstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// GameScore 某用户在某游戏的最好成绩（榜单行）。
type GameScore struct {
	Rank      int    `json:"rank"`
	UserID    int64  `json:"userId"`
	UserName  string `json:"userName"`
	DomainID  *int64 `json:"domainId,omitempty"`
	BestScore int    `json:"bestScore"`
	Plays     int    `json:"plays"`
	IsMe      bool   `json:"isMe,omitempty"`
}

// MaxGameScore 单局分数上限（服务端硬上限，防止随手改数字刷榜）。
const MaxGameScore = 100000

// gameScoreMinInterval 同一用户两次提交的最小间隔（限频）。
const gameScoreMinInterval = 3 * time.Second

// gameScoreLastSubmit 进程内限频表（重启即清空，属可接受的弱约束）。
// 必须加锁：check-then-act 在没有互斥时既能被并发请求绕过，又构成 map 数据竞争。
var (
	gameScoreRateMu     sync.Mutex
	gameScoreLastSubmit = map[int64]time.Time{}
)

// allowGameScoreSubmit 限频判定（原子）：允许时立即占位，避免并发请求同时通过。
func allowGameScoreSubmit(userID int64) bool {
	gameScoreRateMu.Lock()
	defer gameScoreRateMu.Unlock()
	if last, ok := gameScoreLastSubmit[userID]; ok && time.Since(last) < gameScoreMinInterval {
		return false
	}
	gameScoreLastSubmit[userID] = time.Now()
	return true
}

// ValidateGameID 校验 game 标识（与前端注册表约定一致）。
func ValidateGameID(game string) bool {
	if len(game) == 0 || len(game) > 32 {
		return false
	}
	for _, r := range game {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// SubmitGameScore 记录一次成绩：只保留最高分、累计次数、写回域归属。
// 返回（最高分, 是否刷新了最高分, 错误）。
// 低分（< 当前最高分）不算刷新，但仍计入 plays。
func (s *Store) SubmitGameScore(game string, userID, domainID int64, score int) (int, bool, error) {
	if !ValidateGameID(game) {
		return 0, false, fmt.Errorf("非法游戏标识")
	}
	if score < 0 || score > MaxGameScore {
		return 0, false, fmt.Errorf("分数超出允许范围")
	}
	// 限频：同一用户 3 秒内只接受一次（正常一局远长于此）。判定与占位在同一临界区内完成。
	if !allowGameScoreSubmit(userID) {
		return 0, false, ErrGameScoreTooFrequent
	}

	// 单条 UPSERT：并发首次提交同一 (game,user) 不会撞 UNIQUE 报错；
	// 只在更高分时覆盖 best_score，其余情况仅累计 plays。
	// 是否刷新最高分由事务内读到的旧值判定（更早的 50 → 再次提交 50 不算新纪录）。
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var prevBest int
	hasPrev := true
	if err := tx.QueryRow(`SELECT best_score FROM game_scores WHERE game=? AND user_id=?`, game, userID).Scan(&prevBest); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, false, err
		}
		hasPrev = false
	}
	if _, err := tx.Exec(`INSERT INTO game_scores(game,user_id,domain_id,best_score,plays)
		VALUES(?,?,?,?,1)
		ON CONFLICT(game,user_id) DO UPDATE SET
			best_score = MAX(game_scores.best_score, excluded.best_score),
			plays = game_scores.plays + 1,
			domain_id = COALESCE(excluded.domain_id, game_scores.domain_id),
			updated_at = CURRENT_TIMESTAMP`,
		game, userID, nullInt64Ptr(domainID), score); err != nil {
		return 0, false, err
	}
	var best int
	if err := tx.QueryRow(`SELECT best_score FROM game_scores WHERE game=? AND user_id=?`, game, userID).Scan(&best); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return best, !hasPrev || score > prevBest, nil
}

// ErrGameScoreTooFrequent 提交过于频繁。
var ErrGameScoreTooFrequent = errors.New("提交过于频繁，请稍后再试")

// ListGameRank 榜单：domainID>0 时只看该域（本域榜单），否则全部域（全域榜单）。
// 返回前 limit 名 + 指定用户自己的排名行（不在前 limit 时也返回，便于"我的排名"）。
func (s *Store) ListGameRank(game string, domainID, forUserID int64, limit int) ([]GameScore, *GameScore, error) {
	if !ValidateGameID(game) {
		return nil, nil, fmt.Errorf("非法游戏标识")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := `WHERE g.game=?`
	args := []any{game}
	if domainID > 0 {
		where += ` AND g.domain_id=?`
		args = append(args, domainID)
	}

	// 排名用窗口函数：**同分即并列**（rank 只按分数，不含时间），显示次序按 rank 升序、同 rank 内更早达到者靠前。
	// 列都取别名：该子查询会被外层再包一次（取"我的排名"）。
	base := `SELECT g.user_id AS user_id, COALESCE(u.username,'') AS username, g.domain_id AS domain_id,
			g.best_score AS best_score, g.plays AS plays,
			RANK() OVER (ORDER BY g.best_score DESC) AS rnk
		FROM game_scores g LEFT JOIN users u ON u.id=g.user_id ` + where

	rows, err := s.DB.Query(base+` ORDER BY rnk ASC, g.updated_at ASC LIMIT ?`, append(append([]any{}, args...), limit)...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []GameScore{}
	for rows.Next() {
		var it GameScore
		var dom sql.NullInt64
		if err := rows.Scan(&it.UserID, &it.UserName, &dom, &it.BestScore, &it.Plays, &it.Rank); err != nil {
			return nil, nil, err
		}
		if dom.Valid {
			d := dom.Int64
			it.DomainID = &d
		}
		it.IsMe = forUserID > 0 && it.UserID == forUserID
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// 我的排名（不在前 limit 时单独取）。
	// 注意：排行榜窗口必须**先在全量榜内算排名**、再筛出我这一行。
	// 若直接在窗口函数所在查询里加 user_id 过滤，窗口内只剩一行 → 排名恒为 1（错误）。
	var mine *GameScore
	if forUserID > 0 {
		inTop := false
		for i := range out {
			if out[i].UserID == forUserID {
				inTop = true
				break
			}
		}
		if !inTop {
			row := s.DB.QueryRow(`SELECT user_id, username, domain_id, best_score, plays, rnk FROM (`+base+`) t
				WHERE t.user_id=?`, append(append([]any{}, args...), forUserID)...)
			var it GameScore
			var dom sql.NullInt64
			if err := row.Scan(&it.UserID, &it.UserName, &dom, &it.BestScore, &it.Plays, &it.Rank); err == nil {
				if dom.Valid {
					d := dom.Int64
					it.DomainID = &d
				}
				it.IsMe = true
				mine = &it
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, nil, err
			}
		}
	}
	return out, mine, nil
}

// MyGameScore 我在某游戏的最好成绩（没有记录时返回 nil,0）。
func (s *Store) MyGameScore(game string, userID int64) (int, int, error) {
	var best, plays int
	err := s.DB.QueryRow(`SELECT best_score, plays FROM game_scores WHERE game=? AND user_id=?`, game, userID).Scan(&best, &plays)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return best, plays, nil
}

// nullInt64Ptr 0 视为 NULL（用户可能没有域归属）。
func nullInt64Ptr(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

// ResetGameScoreRateLimit 仅测试用：清空进程内限频表。
func ResetGameScoreRateLimit() {
	gameScoreLastSubmit = map[int64]time.Time{}
}

// trimGame 便于调用方规范化输入（去空白 + 小写）。
func trimGame(game string) string { return strings.ToLower(strings.TrimSpace(game)) }

// NormalizeGameID 外部输入规范化。
func NormalizeGameID(game string) string { return trimGame(game) }
