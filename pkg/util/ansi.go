package util

import (
	"io"
	"regexp"
)

// ansiRegex 匹配 ANSI 转义序列
// 覆盖了大多数常用的序列，包括颜色 (SGR)、光标移动 (CSI)、OSC 等
// ansiRegex 匹配更广泛的 ANSI 和 终端控制序列
// ansiRegex 匹配更广泛的 ANSI 和 终端控制序列 (CSI, OSC, APC, ST 等)
// ansiRegex 匹配全系列的 ANSI 转义序列和终端控制序列 (CSI, OSC, APC, ST 等)
var ansiRegex = regexp.MustCompile(`(?s)[\x1b\x9b](?:[[()#;?]*[0-9:;<=>?]*[!"#$%&'()*+,\-./]*[A-PRZcf-ntqry=><~]|\x5d.*?(?:\x07|\x1b\x5c)|\x5f.*?(?:\x1b\x5c)|\x5e.*?(?:\x1b\x5c)|[\x41-\x5a\x5c\x5e\x5f])`)

// StripAnsi 移除字符串中的 ANSI 转义序列
func StripAnsi(s string) string {
	if s == "" {
		return ""
	}
	return ansiRegex.ReplaceAllString(s, "")
}

// AnsiStripper 一个包装 Writer，在写入前移除 ANSI 序列
type AnsiStripper struct {
	Writer io.Writer
}

// Write 实现 io.Writer 接口
func (as *AnsiStripper) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	// 移除后写入。
	stripped := ansiRegex.ReplaceAll(p, []byte(""))
	_, err = as.Writer.Write(stripped)
	// 返回原始 p 的长度，以符合 io.Writer 接口约定，防止上层错误
	return len(p), err
}

// CrLfFixer 一个包装 Writer，将 \n 替换为 \r\n
type CrLfFixer struct {
	Writer io.Writer
}

// Write 实现 io.Writer 接口
func (w *CrLfFixer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	var buf []byte
	for i := 0; i < len(p); i++ {
		if p[i] == '\n' {
			// 如果 \n 前面没有 \r，则补充一个 \r
			if i == 0 || p[i-1] != '\r' {
				buf = append(buf, '\r')
			}
		}
		buf = append(buf, p[i])
	}

	_, err = w.Writer.Write(buf)
	// 返回原始 p 的长度
	return len(p), err
}
