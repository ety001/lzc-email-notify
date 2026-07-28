package notify

import (
	"net/url"
	"os"
)

// defaultAppPackageID 是应用在懒猫微服上的包 ID，用于构造 deeplink。
// 可用环境变量 APP_PACKAGE_ID 覆盖（例如 dev 包名）。
const defaultAppPackageID = "ink.akawa.ety001.emailnotify"

// AppPackageID 返回当前应用的包 ID。
func AppPackageID() string {
	if id := os.Getenv("APP_PACKAGE_ID"); id != "" {
		return id
	}
	return defaultAppPackageID
}

// OpenAppDeeplink 构造点击系统通知后打开本应用指定路由的 deeplink。
// path 为应用内路径（本应用是 HashRouter，如 "/#/open/<accountID>"），
// 已做 URL 编码，可直接拼进 deeplink 的 query。
// 参考：https://developer.lazycat.cloud/advanced-frontend-app-dev.html 系统通知章节
func OpenAppDeeplink(path string) string {
	return "lzc://client/app/open?appId=" + AppPackageID() + "&path=" + url.QueryEscape(path)
}
