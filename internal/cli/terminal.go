package cli

import "strings"

// renderTerminal 用显式黑白颜色渲染模块；每行恢复颜色，奇数行的下半格补白。
// modules 已包含标准留白；full 每模块占两列，half 每字符格容纳上下两个模块。
func renderTerminal(modules [][]bool, mode string) string {
	var result strings.Builder
	if mode == "full" {
		for _, row := range modules {
			for _, black := range row {
				if black {
					result.WriteString("\x1b[40m  ")
				} else {
					result.WriteString("\x1b[47m  ")
				}
			}
			result.WriteString("\x1b[0m\n")
		}
		return result.String()
	}
	for y := 0; y < len(modules); y += 2 {
		result.WriteString("\x1b[30;47m")
		for x, upper := range modules[y] {
			lower := y+1 < len(modules) && modules[y+1][x]
			switch {
			case upper && lower:
				result.WriteRune('█')
			case upper:
				result.WriteRune('▀')
			case lower:
				result.WriteRune('▄')
			default:
				result.WriteByte(' ')
			}
		}
		result.WriteString("\x1b[0m\n")
	}
	return result.String()
}
