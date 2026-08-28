package store

import (
	"testing"

	"orangerepo/internal/model"
)

func TestBookletDirectories(t *testing.T) {
	s := newTestStore(t)

	// 空列表
	dirs, err := s.ListBookletDirectories()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Fatalf("empty list = %d, want 0", len(dirs))
	}

	// 根目录 + 子目录 + 孙目录
	rootID, err := s.CreateBookletDirectory("数学", nil)
	if err != nil {
		t.Fatal(err)
	}
	subID, err := s.CreateBookletDirectory("几何", &rootID)
	if err != nil {
		t.Fatal(err)
	}
	childID, err := s.CreateBookletDirectory("圆", &subID)
	if err != nil {
		t.Fatal(err)
	}
	_ = childID
	otherID, err := s.CreateBookletDirectory("算法", nil)
	if err != nil {
		t.Fatal(err)
	}

	// 非法：空名称、父目录不存在
	if _, err := s.CreateBookletDirectory("  ", nil); err == nil {
		t.Error("空目录名应报错")
	}
	ghost := int64(9999)
	if _, err := s.CreateBookletDirectory("幽灵", &ghost); err == nil {
		t.Error("父目录不存在应报错")
	}

	if err := s.RenameBookletDirectory(rootID, "数学竞赛"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameBookletDirectory(rootID, ""); err == nil {
		t.Error("空重命名应报错")
	}
	if err := s.RenameBookletDirectory(ghost, "x"); err == nil {
		t.Error("重命名不存在的目录应报错")
	}

	dirs, err = s.ListBookletDirectories()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]model.BookletDirectory{}
	for _, d := range dirs {
		byID[d.ID] = d
	}
	if byID[rootID].Name != "数学竞赛" || byID[rootID].ParentID != nil {
		t.Errorf("root: %+v", byID[rootID])
	}
	if byID[subID].ParentID == nil || *byID[subID].ParentID != rootID {
		t.Errorf("sub parent: %+v", byID[subID])
	}

	// 训练/练习创建与移动
	trainID, err := s.CreateTraining("训练A", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pracID, err := s.CreatePractice("练习B", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	trainInDir, err := s.CreateTraining("训练C", "", nil, &subID)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetTrainingFolder(trainID, &rootID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPracticeFolder(pracID, &rootID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTrainingFolder(trainID, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPracticeFolder(pracID, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTrainingFolder(trainID, &ghost); err == nil {
		t.Error("移动到不存在的目录应报错")
	}
	if err := s.SetTrainingFolder(ghost, &rootID); err == nil {
		t.Error("训练不存在应报错")
	}

	trainings, err := s.ListTrainings()
	if err != nil {
		t.Fatal(err)
	}
	practices, err := s.ListPractices()
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range trainings {
		if tr.ID == trainID && tr.FolderID != nil {
			t.Errorf("训练A 应无目录: %v", tr.FolderID)
		}
		if tr.ID == trainInDir {
			if tr.FolderID == nil || *tr.FolderID != subID {
				t.Errorf("训练C 应在子目录: %v", tr.FolderID)
			}
		}
	}
	for _, p := range practices {
		if p.ID == pracID && p.FolderID != nil {
			t.Errorf("练习B 应无目录: %v", p.FolderID)
		}
	}

	// 把训练A 放入将被删除的根目录（验证删除时上移一层）
	if err := s.SetTrainingFolder(trainID, &rootID); err != nil {
		t.Fatal(err)
	}

	// 不删题册删除「数学竞赛」：子目录「几何」上移一层；训练A 移到顶层
	if err := s.DeleteBookletDirectory(rootID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBookletDirectory(ghost, false); err == nil {
		t.Error("删除不存在的目录应报错")
	}

	dirs, err = s.ListBookletDirectories()
	if err != nil {
		t.Fatal(err)
	}
	// 删除「数学竞赛」后剩余：几何（上移到根）、圆（仍在几何下）、算法（根）= 3
	if len(dirs) != 3 {
		t.Fatalf("删除后目录数 = %d, want 3", len(dirs))
	}
	byID = map[int64]model.BookletDirectory{}
	for _, d := range dirs {
		byID[d.ID] = d
	}
	if byID[subID].ParentID != nil {
		t.Errorf("几何应上移到根，实际 parent=%v", byID[subID].ParentID)
	}
	if byID[childID].ParentID == nil || *byID[childID].ParentID != subID {
		t.Errorf("圆应仍在几何下，实际 parent=%v", byID[childID].ParentID)
	}
	if byID[otherID].ParentID != nil {
		t.Errorf("算法应保持在根，实际 parent=%v", byID[otherID].ParentID)
	}
	trainings, err = s.ListTrainings()
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range trainings {
		if tr.ID == trainID && tr.FolderID != nil {
			t.Errorf("删除目录后训练 %d 应上移到根，实际 %v", tr.ID, tr.FolderID)
		}
		if tr.ID == trainInDir {
			if tr.FolderID == nil || *tr.FolderID != subID {
				t.Errorf("训练C 应保持在子目录，实际 %v", tr.FolderID)
			}
		}
	}
}

func TestSetBookletDirectoryLayout(t *testing.T) {
	s := newTestStore(t)
	a, err := s.CreateBookletDirectory("A", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBookletDirectory("B", nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateBookletDirectory("C", &a)
	if err != nil {
		t.Fatal(err)
	}
	ghost := int64(9999)

	dirList := func() []model.BookletDirectory {
		dirs, err := s.ListBookletDirectories()
		if err != nil {
			t.Fatal(err)
		}
		return dirs
	}
	assertParent := func(dirs []model.BookletDirectory, id int64, want *int64) {
		t.Helper()
		for _, d := range dirs {
			if d.ID == id {
				got := d.ParentID
				if (got == nil) != (want == nil) || (got != nil && *got != *want) {
					t.Errorf("dir %d parent = %v, want %v", id, got, want)
				}
				return
			}
		}
		t.Errorf("dir %d not found", id)
	}

	// 移动：C 改挂到 B 下；顺序 B(1) A(2)
	layout := []model.BookletDirectory{
		{ID: b, ParentID: nil, OrderNo: 1},
		{ID: c, ParentID: &b, OrderNo: 1},
		{ID: a, ParentID: nil, OrderNo: 2},
	}
	if err := s.SetBookletDirectoryLayout(layout); err != nil {
		t.Fatalf("layout: %v", err)
	}
	dirs := dirList()
	assertParent(dirs, c, &b)
	byID := map[int64]model.BookletDirectory{}
	for _, d := range dirs {
		byID[d.ID] = d
	}
	if byID[b].OrderNo != 1 || byID[a].OrderNo != 2 {
		t.Errorf("orderNo wrong: %+v", byID)
	}

	// 非法：漏目录
	if err := s.SetBookletDirectoryLayout([]model.BookletDirectory{
		{ID: b, ParentID: nil, OrderNo: 1},
		{ID: a, ParentID: nil, OrderNo: 2},
	}); err == nil {
		t.Error("漏目录应报错")
	}
	// 非法：外来 id
	if err := s.SetBookletDirectoryLayout([]model.BookletDirectory{
		{ID: b, ParentID: nil, OrderNo: 1},
		{ID: c, ParentID: nil, OrderNo: 2},
		{ID: 9999, ParentID: nil, OrderNo: 3},
	}); err == nil {
		t.Error("外来 id 应报错")
	}
	// 非法：自父
	if err := s.SetBookletDirectoryLayout([]model.BookletDirectory{
		{ID: b, ParentID: nil, OrderNo: 1},
		{ID: c, ParentID: nil, OrderNo: 2},
		{ID: a, ParentID: &a, OrderNo: 3},
	}); err == nil {
		t.Error("自父应报错")
	}
	// 非法：成环（A 挂到 C 下、C 挂到 A 下）
	if err := s.SetBookletDirectoryLayout([]model.BookletDirectory{
		{ID: a, ParentID: &c, OrderNo: 1},
		{ID: c, ParentID: &a, OrderNo: 1},
		{ID: b, ParentID: nil, OrderNo: 2},
	}); err == nil {
		t.Error("成环应报错")
	}
	// 非法：父不存在
	if err := s.SetBookletDirectoryLayout([]model.BookletDirectory{
		{ID: b, ParentID: nil, OrderNo: 1},
		{ID: c, ParentID: nil, OrderNo: 2},
		{ID: a, ParentID: &ghost, OrderNo: 3},
	}); err == nil {
		t.Error("父不存在应报错")
	}

	// 失败后原布局不变
	dirs = dirList()
	assertParent(dirs, c, &b)
}