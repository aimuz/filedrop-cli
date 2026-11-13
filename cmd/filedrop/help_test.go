package main

import (
	"strings"
	"testing"
)

// 每个命令的 help 都必须说清楚它是干什么的。
// 之前 list / rm / quota 这些没有选项的命令只打一行「用法：…」，
// 想查用法的人反而什么也没查到。
func TestEveryCommandHasSummary(t *testing.T) {
	for name, spec := range commandTable() {
		if strings.TrimSpace(spec.summary) == "" {
			t.Errorf("%s 缺少 summary，help 里只会剩一行用法", name)
		}
		if spec.run == nil {
			t.Errorf("%s 没有 run 函数", name)
		}
		if strings.TrimSpace(spec.synopsis) == "" {
			t.Errorf("%s 缺少 synopsis", name)
		}
	}
}

// 别名字段指向的命令必须存在，否则 `filedrop ls` 会报未知命令。
func TestAliasesPointToRealCommands(t *testing.T) {
	table := commandTable()
	for alias, target := range aliases {
		if _, ok := table[target]; !ok {
			t.Errorf("别名 %q 指向不存在的命令 %q", alias, target)
		}
	}
}

// 新加命令时别忘了写进总览：命令表里有、usage 里没有，用户就找不到它。
func TestUsageListsEveryCommand(t *testing.T) {
	for name := range commandTable() {
		if !strings.Contains(usage, "filedrop "+name) {
			t.Errorf("总览里缺少 %s", name)
		}
	}
}
