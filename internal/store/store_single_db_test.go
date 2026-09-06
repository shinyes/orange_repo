package store

import (
	"testing"

	"orangeoj/internal/model"
)

// TestDeleteProblemClearsSpaceItems 删题清理空间条目引用（训练/练习条目无 FK，防悬挂）。
func TestDeleteProblemClearsSpaceItems(t *testing.T) {
	s := newTestStore(t)
	domainID, err := s.CreateDomain("域A")
	if err != nil {
		t.Fatal(err)
	}
	spaceID, err := s.CreateSpace(domainID, "空间A")
	if err != nil {
		t.Fatal(err)
	}
	// 造题并归域
	pid, err := s.CreateProblem(model.Problem{Type: model.TypeSingleChoice, Title: "题", BodyJSON: []byte(`{"options":["a"]}`), AnswerJSON: []byte(`{"answerIndex":0}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, pid)
	// 空间训练（章内条目）+ 空间练习（条目）
	trID, _ := s.CreateSpaceTraining(spaceID, "训练", "", nil, 3)
	chID, _ := s.CreateSpaceChapter(trID, "章")
	_, _ = s.AddSpaceChapterItems(chID, []int64{pid})
	prID, _ := s.CreateSpacePractice(spaceID, "练习", "", nil)
	_ = s.AddSpacePracticeItems(prID, []int64{pid})

	// 删题
	if err := s.DeleteProblem(pid); err != nil {
		t.Fatal(err)
	}
	// 空间条目应被清理
	for _, q := range []string{`SELECT COUNT(1) FROM space_training_items WHERE problem_id=?`, `SELECT COUNT(1) FROM space_practice_items WHERE problem_id=?`} {
		var n int
		if err := s.DB.QueryRow(q, pid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("删除题目后空间条目残留: %q count=%d", q, n)
		}
	}
}

// TestDeleteDomainProblemsClearsSpaceItems 删域（级联删题）同时清空间条目。
func TestDeleteDomainProblemsClearsSpaceItems(t *testing.T) {
	s := newTestStore(t)
	domainID, _ := s.CreateDomain("域B")
	spaceID, _ := s.CreateSpace(domainID, "空间B")
	pid, _ := s.CreateProblem(model.Problem{Type: model.TypeSingleChoice, Title: "题", BodyJSON: []byte(`{"options":["a"]}`), AnswerJSON: []byte(`{"answerIndex":0}`)})
	_, _ = s.DB.Exec(`UPDATE problems SET domain_id=? WHERE id=?`, domainID, pid)
	trID, _ := s.CreateSpaceTraining(spaceID, "训练", "", nil, 3)
	chID, _ := s.CreateSpaceChapter(trID, "章")
	_, _ = s.AddSpaceChapterItems(chID, []int64{pid})
	if err := s.DeleteDomainProblems(domainID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM space_training_items WHERE problem_id=?`, pid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("删域后空间条目残留 count=%d", n)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE id=?`, pid).Scan(&n); err != nil || n != 0 {
		t.Fatalf("删域后题目残留 n=%d err=%v", n, err)
	}
}

// TestConcurrentMigrateSameFile 双连接并发对同一空库执行全量迁移（模拟两进程同时首启），
// 迁移必须幂等且不因竞态崩溃（-race 下运行）。
func TestConcurrentMigrateSameFile(t *testing.T) {
	dir := t.TempDir()
	// 两个连接同一文件并发迁移
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			st, err := Open(dir)
			if err != nil {
				done <- err
				return
			}
			done <- st.Close()
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("并发迁移失败: %v", err)
		}
	}
	// 迁移后表齐全可读写
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	domainID, err := st.CreateDomain("并发域")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSpace(domainID, "并发空间"); err != nil {
		t.Fatal(err)
	}
}
