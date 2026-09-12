// 空间项目可见性回归测试（新语义：**已开放 AND 已分配**）。
//
// 语义（internal/quizstore/repo_space.go）：
//   - 成员（userID>0）可见某训练/练习/刷题项目，须**同时**满足
//     ① 项目 is_public=1（界面文案「开放」）；② 该成员在该项目的可见名单表中。
//     两个条件缺一即不可见（仅开放未分配、仅分配未开放都不可见）；
//   - 管理员（userID<=0，viewerID 把域/系统管理员归零）恒可见，与开放/分配无关。
//
// 本文件用真实临时 sqlite 库（与门户同一条路径：ListSpace*Brief / GetSpace*Brief /
// *VisibleForUser），三类项目各跑一遍同一套矩阵断言。
package quizstore_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"orangeoj/internal/model"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/store"
)

// svCombos 五组合：覆盖「两个条件各自的半边」与「同时满足」「分配给别人」。
type svCombos struct {
	plain        int64 // 未开放 + 未分配
	assignedOnly int64 // 未开放 + 已分配（本成员）——仅分配不够
	openOnly     int64 // 已开放 + 未分配 ——仅开放不够
	both         int64 // 已开放 + 已分配（本成员）
	otherOnly    int64 // 已开放 + 分配给他人（名单不含本成员）
}

// svEnv 测试环境：一个域/空间 + 两名成员 + 三类项目。
type svEnv struct {
	qs      *quizstore.Store
	main    *store.Store // 保持打开：测试中翻转 is_public / 改写可见名单
	spaceID int64
	member  int64 // 被判定可见性的成员
	other   int64 // 另一名成员（只出现在 otherOnly 的名单里）
	problem int64
	items   map[string]svCombos // kind → 五组合 id
}

// svKind 三类项目的统一访问面（表驱动：同一套断言跑训练/练习/刷题）。
type svKind struct {
	name   string
	visTbl string // 可见名单表
	create func(e *svEnv, title string, isPublic bool) (int64, error)
	// list 门户列表（member 视角过滤生效）→ 项目 id 列表。
	list func(r *quizstore.RepoReader, spaceID, userID int64) ([]int64, error)
	// get 单项目读取：不可见须返回 ErrNotFound（管理员视角不过滤）。
	get func(r *quizstore.RepoReader, itemID, userID int64) error
	// isPublic 列表项回传的开放位（供「开放位未被过滤改写」断言）。
	isPublic func(r *quizstore.RepoReader, spaceID, userID, itemID int64) (bool, bool, error)
	// visibleForUser 单项目可见性判定（与作答流校验同源）。
	visibleForUser func(r *quizstore.RepoReader, itemID, userID int64) (bool, error)
}

// svKinds 三类项目的统一访问面。
func svKinds() []svKind {
	return []svKind{
		{
			name:   "training",
			visTbl: "space_training_visible",
			create: func(e *svEnv, title string, isPublic bool) (int64, error) {
				id, err := e.main.CreateSpaceTraining(e.spaceID, title, "", nil, 3, isPublic)
				if err != nil {
					return 0, err
				}
				ch, err := e.main.CreateSpaceChapter(id, "章")
				if err != nil {
					return 0, err
				}
				if _, err := e.main.AddSpaceChapterItems(ch, []int64{e.problem}); err != nil {
					return 0, err
				}
				return id, nil
			},
			list: func(r *quizstore.RepoReader, spaceID, userID int64) ([]int64, error) {
				briefs, err := r.ListSpaceTrainingsBrief(spaceID, userID)
				if err != nil {
					return nil, err
				}
				ids := make([]int64, 0, len(briefs))
				for _, b := range briefs {
					ids = append(ids, b.ID)
				}
				return ids, nil
			},
			get: func(r *quizstore.RepoReader, itemID, userID int64) error {
				_, _, err := r.GetSpaceTrainingBrief(itemID, userID)
				return err
			},
			isPublic: func(r *quizstore.RepoReader, spaceID, userID, itemID int64) (bool, bool, error) {
				briefs, err := r.ListSpaceTrainingsBrief(spaceID, userID)
				if err != nil {
					return false, false, err
				}
				for _, b := range briefs {
					if b.ID == itemID {
						return b.IsPublic, true, nil
					}
				}
				return false, false, nil
			},
			// 训练/练习的作答入口用 Brief 查询做可见性判定（内部带 visibleClause），
			// 故这里直接以「能否取到 Brief」作为单项目可见性，与生产路径一致。
			visibleForUser: func(r *quizstore.RepoReader, itemID, userID int64) (bool, error) {
				_, _, err := r.GetSpaceTrainingBrief(itemID, userID)
				if errors.Is(err, quizstore.ErrNotFound) {
					return false, nil
				}
				if err != nil {
					return false, err
				}
				return true, nil
			},
		},
		{
			name:   "practice",
			visTbl: "space_practice_visible",
			create: func(e *svEnv, title string, isPublic bool) (int64, error) {
				id, err := e.main.CreateSpacePractice(e.spaceID, title, "", nil, isPublic)
				if err != nil {
					return 0, err
				}
				if err := e.main.AddSpacePracticeItems(id, []int64{e.problem}); err != nil {
					return 0, err
				}
				return id, nil
			},
			list: func(r *quizstore.RepoReader, spaceID, userID int64) ([]int64, error) {
				briefs, err := r.ListSpacePracticesBrief(spaceID, userID)
				if err != nil {
					return nil, err
				}
				ids := make([]int64, 0, len(briefs))
				for _, b := range briefs {
					ids = append(ids, b.ID)
				}
				return ids, nil
			},
			get: func(r *quizstore.RepoReader, itemID, userID int64) error {
				_, _, err := r.GetSpacePracticeBrief(itemID, userID)
				return err
			},
			isPublic: func(r *quizstore.RepoReader, spaceID, userID, itemID int64) (bool, bool, error) {
				briefs, err := r.ListSpacePracticesBrief(spaceID, userID)
				if err != nil {
					return false, false, err
				}
				for _, b := range briefs {
					if b.ID == itemID {
						return b.IsPublic, true, nil
					}
				}
				return false, false, nil
			},
			// 同训练：练习以 Brief 查询（visibleClause）作为生产可见性判定。
			visibleForUser: func(r *quizstore.RepoReader, itemID, userID int64) (bool, error) {
				_, _, err := r.GetSpacePracticeBrief(itemID, userID)
				if errors.Is(err, quizstore.ErrNotFound) {
					return false, nil
				}
				if err != nil {
					return false, err
				}
				return true, nil
			},
		},
		{
			name:   "quiz",
			visTbl: "space_quiz_visible",
			create: func(e *svEnv, title string, isPublic bool) (int64, error) {
				// tags 源 + 空标签 = 空间所在域全部客观题（题集内容与可见性判定无关）
				return e.main.CreateSpaceQuiz(e.spaceID, title, nil, "tags", "", 0, 0, isPublic)
			},
			list: func(r *quizstore.RepoReader, spaceID, userID int64) ([]int64, error) {
				briefs, err := r.ListSpaceQuizzesBrief(spaceID, userID)
				if err != nil {
					return nil, err
				}
				ids := make([]int64, 0, len(briefs))
				for _, b := range briefs {
					ids = append(ids, b.ID)
				}
				return ids, nil
			},
			// 刷题项目没有「单项目 gated 读取」函数：门户用 quizSpaceOf（存在性）
			// + QuizVisibleForUser（可见性）拼出 404，这里等价复现该组合。
			get: func(r *quizstore.RepoReader, itemID, userID int64) error {
				ok, err := r.QuizVisibleForUser(itemID, userID)
				if err != nil {
					return err
				}
				if !ok {
					return quizstore.ErrNotFound
				}
				return nil
			},
			isPublic: func(r *quizstore.RepoReader, spaceID, userID, itemID int64) (bool, bool, error) {
				briefs, err := r.ListSpaceQuizzesBrief(spaceID, userID)
				if err != nil {
					return false, false, err
				}
				for _, b := range briefs {
					if b.ID == itemID {
						return b.IsPublic, true, nil
					}
				}
				return false, false, nil
			},
			visibleForUser: func(r *quizstore.RepoReader, itemID, userID int64) (bool, error) {
				return r.QuizVisibleForUser(itemID, userID)
			},
		},
	}
}

// newSVEnv 建环境：域/空间/一道域内题 + 三类项目 × 五组合 + 两名成员（均入空间）。
func newSVEnv(t *testing.T) *svEnv {
	t.Helper()
	dir := t.TempDir()
	main, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	t.Cleanup(func() { _ = main.Close() })

	domainID, err := main.CreateDomain("可见性域")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := main.CreateSpace(domainID, "可见性班")
	if err != nil {
		t.Fatal(err)
	}
	pid, err := main.CreateProblem(model.Problem{
		Type: model.TypeSingleChoice, Title: "可见性题", Tags: []string{"可见性"},
		StatementMD: "1+1=?", BodyJSON: json.RawMessage(`{"options":["1","2"]}`),
		AnswerJSON: json.RawMessage(`{"answerIndex":1}`), Solutions: json.RawMessage(`[]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, pid); err != nil {
		t.Fatal(err)
	}

	env := &svEnv{main: main, spaceID: spaceID, problem: pid, items: map[string]svCombos{}}
	for _, k := range svKinds() {
		var c svCombos
		// 组合顺序固定，便于阅读；标题带 kind+组合名，失败信息可直接定位。
		for _, spec := range []struct {
			dst      *int64
			name     string
			isPublic bool
		}{
			{&c.plain, "未开放未分配", false},
			{&c.assignedOnly, "仅分配", false},
			{&c.openOnly, "仅开放", true},
			{&c.both, "开放且分配", true},
			{&c.otherOnly, "开放分配他人", true},
		} {
			id, cerr := k.create(env, fmt.Sprintf("%s-%s", k.name, spec.name), spec.isPublic)
			if cerr != nil {
				t.Fatalf("create %s %s: %v", k.name, spec.name, cerr)
			}
			*spec.dst = id
		}
		env.items[k.name] = c
	}
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}

	qs, err := quizstore.Open(dir)
	if err != nil {
		t.Fatalf("open quiz store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })
	env.qs = qs
	env.member, err = qs.Accounts.CreateUser("svMember", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}
	env.other, err = qs.Accounts.CreateUser("svOther", "pw", "member")
	if err != nil {
		t.Fatal(err)
	}

	// 回主库：两人入空间；按组合写可见名单（plain/openOnly 不分配，otherOnly 只分配 other）。
	main2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = main2.Close() })
	env.main = main2
	if err := main2.SetSpaceMembers(spaceID, []int64{env.member, env.other}); err != nil {
		t.Fatal(err)
	}
	for _, k := range svKinds() {
		c := env.items[k.name]
		if err := main2.SetVisibleUsers(k.visTbl, c.assignedOnly, []int64{env.member}); err != nil {
			t.Fatal(err)
		}
		if err := main2.SetVisibleUsers(k.visTbl, c.both, []int64{env.member}); err != nil {
			t.Fatal(err)
		}
		if err := main2.SetVisibleUsers(k.visTbl, c.otherOnly, []int64{env.other}); err != nil {
			t.Fatal(err)
		}
	}
	return env
}

// setPublic 直接翻转某项目的开放位（模拟门户「开放/关闭」按钮）。
func (e *svEnv) setPublic(t *testing.T, k svKind, itemID int64, pub bool) {
	t.Helper()
	tbl := map[string]string{
		"training": "space_trainings", "practice": "space_practices", "quiz": "space_quizzes",
	}[k.name]
	if tbl == "" {
		t.Fatalf("未知项目类型 %q", k.name)
	}
	if _, err := e.main.DB.Exec(`UPDATE `+tbl+` SET is_public=? WHERE id=?`, b2iVis(pub), itemID); err != nil {
		t.Fatal(err)
	}
}

func b2iVis(b bool) int {
	if b {
		return 1
	}
	return 0
}

// setVisible 覆盖式改写某项目的可见名单。
func (e *svEnv) setVisible(t *testing.T, k svKind, itemID int64, userIDs []int64) {
	t.Helper()
	if err := e.main.SetVisibleUsers(k.visTbl, itemID, userIDs); err != nil {
		t.Fatal(err)
	}
}

// assertList 断言列表 id 集合完全等于 want（顺序按 id 升序，比较元素与长度）。
func assertList(t *testing.T, label string, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 列表长度 = %d %v，want %d %v", label, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s 列表 = %v，want %v（第 %d 项应为 %d）", label, got, want, i, want[i])
		}
	}
}

// TestSpaceVisibilityMatrix 三类项目 × 五组合（新语义 AND 的核心矩阵）。
func TestSpaceVisibilityMatrix(t *testing.T) {
	env := newSVEnv(t)
	for _, k := range svKinds() {
		k := k
		t.Run(k.name, func(t *testing.T) {
			c := env.items[k.name]
			r, uid := env.qs.Repo, env.member

			// ① 未开放 + 未分配 → 列表不出现 / 单项目 404 / VisibleForUser=false
			// ② 未开放 + 已分配 → 同上（**仅分配不够**）
			// ③ 已开放 + 未分配 → 同上（**仅开放不够**）
			// ⑤ 已开放 + 分配给他人（名单不含本成员）→ 同上
			for _, bad := range []struct {
				name string
				id   int64
			}{
				{"未开放未分配", c.plain},
				{"仅分配未开放", c.assignedOnly},
				{"仅开放未分配", c.openOnly},
				{"开放但分配给他人", c.otherOnly},
			} {
				vis, err := k.visibleForUser(r, bad.id, uid)
				if err != nil {
					t.Fatalf("%s VisibleForUser: %v", bad.name, err)
				}
				if vis {
					t.Fatalf("%s（id=%d）VisibleForUser = true，want false", bad.name, bad.id)
				}
				if err := k.get(r, bad.id, uid); !errors.Is(err, quizstore.ErrNotFound) {
					t.Fatalf("%s（id=%d）单项目读取 err = %v，want ErrNotFound", bad.name, bad.id, err)
				}
			}
			ids, err := k.list(r, env.spaceID, uid)
			if err != nil {
				t.Fatal(err)
			}
			assertList(t, "成员（四组合均不可见时）", ids, []int64{c.both})

			// ④ 已开放 + 已分配 → 列表出现 / 单项目可取 / VisibleForUser=true
			vis, err := k.visibleForUser(r, c.both, uid)
			if err != nil {
				t.Fatal(err)
			}
			if !vis {
				t.Fatalf("开放且分配（id=%d）VisibleForUser = false，want true", c.both)
			}
			if err := k.get(r, c.both, uid); err != nil {
				t.Fatalf("开放且分配（id=%d）单项目读取 err = %v，want nil", c.both, err)
			}
			pub, found, err := k.isPublic(r, env.spaceID, uid, c.both)
			if err != nil {
				t.Fatal(err)
			}
			if !found || !pub {
				t.Fatalf("列表项 id=%d found=%v isPublic=%v，want found=true isPublic=true", c.both, found, pub)
			}

			// 动态翻转：仅分配的项被「开放」后立即可见（AND 的第二个条件先满足、第一个条件后满足）。
			env.setPublic(t, k, c.assignedOnly, true)
			vis, err = k.visibleForUser(r, c.assignedOnly, uid)
			if err != nil {
				t.Fatal(err)
			}
			if !vis {
				t.Fatalf("「仅分配」项开放后（id=%d）VisibleForUser = false，want true", c.assignedOnly)
			}
			ids, err = k.list(r, env.spaceID, uid)
			if err != nil {
				t.Fatal(err)
			}
			assertList(t, "成员（仅分配项已开放后）", ids, []int64{c.assignedOnly, c.both})

			// 反向翻转：把「开放且分配」关掉开放 → 立刻不可见（分配仍在，仍不可见）。
			env.setPublic(t, k, c.both, false)
			vis, err = k.visibleForUser(r, c.both, uid)
			if err != nil {
				t.Fatal(err)
			}
			if vis {
				t.Fatalf("「开放且分配」项关闭开放后（id=%d）VisibleForUser = true，want false（分配仍在也不可见）", c.both)
			}
			ids, err = k.list(r, env.spaceID, uid)
			if err != nil {
				t.Fatal(err)
			}
			assertList(t, "成员（开放且分配项已关闭开放后）", ids, []int64{c.assignedOnly})
		})
	}
}

// TestSpaceVisibilityAdminAlwaysVisible 管理员（userID=0 / 负数）恒可见：
// 列表含全部五组合，VisibleForUser 全 true，且与开放/分配无关。
func TestSpaceVisibilityAdminAlwaysVisible(t *testing.T) {
	env := newSVEnv(t)
	for _, k := range svKinds() {
		k := k
		t.Run(k.name, func(t *testing.T) {
			c := env.items[k.name]
			all := []int64{c.plain, c.assignedOnly, c.openOnly, c.both, c.otherOnly}
			for _, adminID := range []int64{0, -1} {
				for _, id := range all {
					vis, err := k.visibleForUser(env.qs.Repo, id, adminID)
					if err != nil {
						t.Fatalf("管理员(%d) VisibleForUser(%d): %v", adminID, id, err)
					}
					if !vis {
						t.Fatalf("管理员(%d) VisibleForUser(%d) = false，want true（管理员恒可见）", adminID, id)
					}
					if err := k.get(env.qs.Repo, id, adminID); err != nil {
						t.Fatalf("管理员(%d) 单项目读取(%d) err = %v，want nil", adminID, id, err)
					}
				}
				ids, err := k.list(env.qs.Repo, env.spaceID, adminID)
				if err != nil {
					t.Fatal(err)
				}
				assertList(t, fmt.Sprintf("管理员(%d)", adminID), ids, all)
			}

			// 成员与管理员对照：同一次快照下成员列表严格更小（证明过滤真的按 userID 生效）。
			memberIDs, err := k.list(env.qs.Repo, env.spaceID, env.member)
			if err != nil {
				t.Fatal(err)
			}
			wantMember := []int64{c.both}
			assertList(t, "成员对照", memberIDs, wantMember)
		})
	}
}

// TestSpaceVisibilityMissingItemID 项目 id 不存在：成员视角 false 且不报错、单项目 ErrNotFound。
func TestSpaceVisibilityMissingItemID(t *testing.T) {
	env := newSVEnv(t)
	const missing = int64(999999)
	for _, k := range svKinds() {
		k := k
		t.Run(k.name, func(t *testing.T) {
			vis, err := k.visibleForUser(env.qs.Repo, missing, env.member)
			if err != nil {
				t.Fatalf("不存在 id VisibleForUser 报错: %v", err)
			}
			if vis {
				t.Fatal("不存在 id VisibleForUser = true，want false")
			}
			if err := k.get(env.qs.Repo, missing, env.member); !errors.Is(err, quizstore.ErrNotFound) {
				t.Fatalf("不存在 id 单项目读取 err = %v，want ErrNotFound", err)
			}
			// 存在性判定先于管理员短路：不存在的 id 对任何角色都是 false
			// （与各入口对不存在项目返回 404 的口径一致）。
			vis, err = k.visibleForUser(env.qs.Repo, missing, 0)
			if err != nil {
				t.Fatalf("管理员 不存在 id VisibleForUser 报错: %v", err)
			}
			if vis {
				t.Fatal("管理员 不存在 id VisibleForUser = true，want false（存在性先于管理员短路）")
			}
		})
	}
}

// TestSpaceVisibilityNoSpaceLevelMerge 防「把两个条件合并成空间级判断」：
// 同空间同类型的两个项目——一个只开放、一个只分配——成员两个都看不到；
// 之后逐个补齐条件时，只有被补齐的那个出现（可见性按项目逐个判定，不是空间级开关）。
func TestSpaceVisibilityNoSpaceLevelMerge(t *testing.T) {
	env := newSVEnv(t)
	for _, k := range svKinds() {
		k := k
		t.Run(k.name, func(t *testing.T) {
			c := env.items[k.name]
			r, uid := env.qs.Repo, env.member

			// 只开放（openOnly）与只分配（assignedOnly）同时存在 → 成员列表必须为空。
			ids, err := k.list(r, env.spaceID, uid)
			if err != nil {
				t.Fatal(err)
			}
			assertList(t, "只开放+只分配并存时", ids, []int64{c.both})
			// c.both 是环境里唯一双条件满足项；先把它的开放关掉，使列表真正为空，
			// 再验证两个半边项目都不出现（避免把 both 误当成「合并判定」的产物）。
			env.setPublic(t, k, c.both, false)
			ids, err = k.list(r, env.spaceID, uid)
			if err != nil {
				t.Fatal(err)
			}
			assertList(t, "空间中仅存两个半边项目时", ids, []int64{})
			for _, bad := range []int64{c.openOnly, c.assignedOnly} {
				if err := k.get(r, bad, uid); !errors.Is(err, quizstore.ErrNotFound) {
					t.Fatalf("半边项目（id=%d）单项目读取 err = %v，want ErrNotFound", bad, err)
				}
			}

			// 补齐「只分配」的开放位 → 只出现它，「只开放」仍不可见。
			env.setPublic(t, k, c.assignedOnly, true)
			ids, err = k.list(r, env.spaceID, uid)
			if err != nil {
				t.Fatal(err)
			}
			assertList(t, "只分配项补齐开放后", ids, []int64{c.assignedOnly})

			// 再补齐「只开放」的名单 → 两个都出现（顺序按 id）。
			env.setVisible(t, k, c.openOnly, []int64{uid})
			ids, err = k.list(r, env.spaceID, uid)
			if err != nil {
				t.Fatal(err)
			}
			want := []int64{c.openOnly, c.assignedOnly}
			if c.assignedOnly < c.openOnly {
				want = []int64{c.assignedOnly, c.openOnly}
			}
			assertList(t, "两个半边都补齐后", ids, want)
		})
	}
}
