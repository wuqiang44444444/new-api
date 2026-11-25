//go:build !yuan_gateway_local_dev

package setting

func ArkCacheIsolatedLocalDatabase() bool { return false }
