// Package version 集中保存发布标识，值由 build.sh 通过 -ldflags -X 注入。
// 单独成包是为了让入口、报告、托盘共用同一个来源，避免版本号散落各处不一致。
package version

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

// String 是可直接打印的完整构建信息（--version 与报告页脚使用）。
func String() string {
	return fmt.Sprintf("KeyPulse %s (build %s, %s/%s, %s)",
		Version, BuildTime, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// Label 是托盘提示用的短标识。
func Label() string {
	if Version == "dev" || Version == "" {
		return "KeyPulse dev"
	}
	return "KeyPulse v" + Version
}
