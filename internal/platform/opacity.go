package platform

import "errors"

func validateWindowOpacity(percent int) error {
	if percent < 30 || percent > 100 {
		return errors.New("窗口不透明度必须为 30 至 100 的整数")
	}
	return nil
}
