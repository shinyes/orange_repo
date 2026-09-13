// 测评记录数据层测试：提交历史必须带回源码（前端详情直接用列表里的源码渲染），
// 以及管理端「全部成员」视图须带提交者用户名。
package quizstore_test

import (
	"testing"

	"orangeoj/internal/accounts"
	"orangeoj/internal/judge"
	"orangeoj/internal/quizstore"
)

// TestListSubmissionsIncludesSourceCode 本人历史必须包含 source_code。
// 回归：列表查询曾漏选该列，导致测评记录详情「代码」页恒显示“无代码”。
func TestListSubmissionsIncludesSourceCode(t *testing.T) {
	qs := newTestEnvironment(t)
	uid, err := qs.Accounts.CreateUser("histOwner", "pw", accounts.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	const problemID, trainingID = int64(9001), int64(77)
	const src = "print('hello history')\n# 源码回传回归"
	if _, err := qs.CreateProgrammingSubmission(uid, problemID, trainingID, 0, "programming",
		"python", src, "", judge.SubmitTypeSubmit); err != nil {
		t.Fatal(err)
	}

	list, err := qs.ListSubmissions(uid, problemID, trainingID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("历史条数 = %d, want 1", len(list))
	}
	if list[0].SourceCode != src {
		t.Fatalf("列表 sourceCode = %q, want %q（漏选 source_code 会让详情显示“无代码”）", list[0].SourceCode, src)
	}
	// 也要带输入数据（自定义输入运行时详情要用）
	if list[0].InputData != "" {
		t.Fatalf("inputData = %q, want 空", list[0].InputData)
	}
}

// TestListSubmissionsAllUsers 管理端「全部成员」：跨用户返回，且带 userId/userName；
// 上下文过滤（trainingId）同样生效。
func TestListSubmissionsAllUsers(t *testing.T) {
	qs := newTestEnvironment(t)
	alice, err := qs.Accounts.CreateUser("histAlice", "pw", accounts.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := qs.Accounts.CreateUser("histBob", "pw", accounts.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	const problemID = int64(9002)
	const trainingID = int64(88)
	mk := func(uid int64, code string, training int64) {
		t.Helper()
		if _, err := qs.CreateProgrammingSubmission(uid, problemID, training, 0, "programming",
			"cpp", code, "", judge.SubmitTypeSubmit); err != nil {
			t.Fatal(err)
		}
	}
	mk(alice, "// alice code", trainingID)
	mk(bob, "// bob code", trainingID)
	mk(bob, "// bob other-context code", 0) // 非本训练上下文：不应出现在 trainingId 过滤结果里

	list, err := qs.ListSubmissionsAllUsers(problemID, trainingID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("全部成员（训练内）条数 = %d, want 2（含他人、不含其它上下文）", len(list))
	}
	byUser := map[string]quizstore.Submission{}
	for _, s := range list {
		if s.UserName == "" {
			t.Fatalf("提交 #%d 缺 userName（管理端需显示提交者）", s.ID)
		}
		if s.UserID == 0 {
			t.Fatalf("提交 #%d 缺 userId", s.ID)
		}
		if s.SourceCode == "" {
			t.Fatalf("提交 #%d 缺 sourceCode", s.ID)
		}
		byUser[s.UserName] = s
	}
	if _, ok := byUser["histAlice"]; !ok {
		t.Fatalf("缺少 alice 的提交：%v", byUser)
	}
	if s, ok := byUser["histBob"]; !ok {
		t.Fatalf("缺少 bob 的提交：%v", byUser)
	} else if s.SourceCode != "// bob code" {
		t.Fatalf("bob sourceCode = %q, want // bob code（训练内那条）", s.SourceCode)
	}

	// 不带上下文：三条全部返回
	all, err := qs.ListSubmissionsAllUsers(problemID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("全部成员（无上下文过滤）条数 = %d, want 3", len(all))
	}

	// 对照：成员本人列表只有自己的，且不带 userName（不泄露他人）
	mine, err := qs.ListSubmissions(alice, problemID, trainingID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].SourceCode != "// alice code" {
		t.Fatalf("alice 本人历史 = %+v, want 仅自己那条", mine)
	}
	if mine[0].UserName != "" || mine[0].UserID != 0 {
		t.Fatalf("本人历史不应带 userId/userName：%+v", mine[0])
	}
}
