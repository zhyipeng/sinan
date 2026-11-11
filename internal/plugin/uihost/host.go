// Package uihost runs a bridge-free native UI in a host-selected child process.
package uihost

import (
	"encoding/json"
	"errors"
	"sinan/internal/plugin/uiview"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
)

type Session interface {
	Active() bool
	Resource(string) (uiview.Resource, bool)
	Message([]byte) (uiview.Action, error)
	Revoke()
	OnRevoke(func()) func()
}

type Config struct {
	Theme           *protocol.ThemeState `json:"theme,omitempty"`
	BridgeVersion   string               `json:"bridgeVersion,omitempty"`
	Panel           bool                 `json:"panel"`
	Entry           string               `json:"entry"`
	CSP             string               `json:"csp"`
	Title           string               `json:"title"`
	Probe           bool                 `json:"probe"`
	DeadlineSeconds int                  `json:"deadlineSeconds"`
	Target          string               `json:"-"`
}

type Report struct {
	Detached               bool   `json:"detached"`
	TogglesHandled         int    `json:"togglesHandled"`
	FocusHandled           int    `json:"focusHandled"`
	EventLoopResponsive    bool   `json:"eventLoopResponsive"`
	Failure                string `json:"failure,omitempty"`
	HeartbeatCompleted     bool   `json:"heartbeatCompleted"`
	Ready                  bool   `json:"ready"`
	BindingsAbsent         bool   `json:"bindingsAbsent"`
	StorageIsolated        bool   `json:"storageIsolated"`
	OwnResourceLoaded      bool   `json:"ownResourceLoaded"`
	ForeignResourceBlocked bool   `json:"foreignResourceBlocked"`
	NavigationBlocked      bool   `json:"navigationBlocked"`
	NetworkBlocked         bool   `json:"networkBlocked"`
	InvalidMessageRejected bool   `json:"invalidMessageRejected"`
	CloseReceived          bool   `json:"closeReceived"`
	BudgetRevoked          bool   `json:"budgetRevoked"`
	LifecycleRevoked       bool   `json:"lifecycleRevoked"`
}

func Run(config Config, session Session, disable func() error) (Report, error) {
	if (config.Theme != nil && theme.ValidateState(*config.Theme) != nil) || (config.BridgeVersion != "" && !protocol.SupportedUIBridge(config.BridgeVersion)) || config.CSP != uiview.CSP || config.Entry == "" || config.DeadlineSeconds < 0 {
		return Report{}, errors.New("UI 宿主配置无效")
	}
	data, err := json.Marshal(config)
	if err != nil {
		return Report{}, errors.New("UI 宿主配置无效")
	}
	return runNative(data, config.Target, session, disable)
}
