//go:build !darwin && !windows && !linux

package startup

import "errors"

func newRegistration(string) (Registration, error) {
	return nil, errors.New("当前平台不支持开机自启")
}
