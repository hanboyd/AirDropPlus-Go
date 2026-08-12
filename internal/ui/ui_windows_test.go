//go:build windows

package ui

import "testing"

func TestLongTextDetection(t *testing.T) {
	if isLongText("short text") {
		t.Fatal("short text was treated as long")
	}
	if !isLongText("first line\nsecond line") {
		t.Fatal("multiline text should expand")
	}
	if !isLongText("这是一个用于验证完整展开功能的长文本。它超过六十个字符以后，应当显示展开入口，而不是只留下无法继续阅读的省略号。这部分文字还会继续增加，确保长度超过界面预览阈值。") {
		t.Fatal("long CJK text should expand")
	}
}
