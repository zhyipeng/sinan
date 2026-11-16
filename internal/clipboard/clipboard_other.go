//go:build (!darwin && !windows && !linux) || (darwin && !cgo)

package clipboard

func readClipboard(previous uint64) (content, uint64, error) {
	return content{}, previous, clipboardSupportError("当前宿主构建不支持系统剪贴板")
}
func writeClipboard(content) error {
	return clipboardSupportError("当前宿主构建不支持系统剪贴板")
}
