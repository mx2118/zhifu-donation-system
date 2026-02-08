package utils

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// KillProcessUsingPort 检测并杀死占用指定端口的进程
func KillProcessUsingPort(port int) error {
	// 构建lsof命令来查找占用端口的进程
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	// 执行命令
	err := cmd.Run()
	if err != nil {
		// 命令执行失败，可能是没有找到占用端口的进程
		return nil
	}

	// 解析输出
	output := out.String()
	lines := strings.Split(output, "\n")

	// 遍历输出行，查找进程ID
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 分割行，获取进程ID
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		// 尝试将第二个字段转换为进程ID
		pidStr := parts[1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// 杀死进程
		killCmd := exec.Command("kill", strconv.Itoa(pid))
		var killOut bytes.Buffer
		killCmd.Stdout = &killOut
		killCmd.Stderr = &killOut

		err = killCmd.Run()
		if err != nil {
			return fmt.Errorf("failed to kill process %d: %v, output: %s", pid, err, killOut.String())
		}

		return nil
	}

	return nil
}
