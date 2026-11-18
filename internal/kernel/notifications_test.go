package kernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	protocol "sinan/packages/plugin-protocol"
)

func TestNotificationAuthorityAndBudget(t *testing.T) {
	calls := 0
	send := func(_ context.Context, value protocol.NotificationParams) error {
		if value.Title != "运行完成" || value.Body != "第一行\n第二行" {
			t.Fatal("unexpected notification")
		}
		calls++
		return nil
	}
	request := json.RawMessage(`{"title":"运行完成","body":"第一行\n第二行"}`)
	for _, handler := range []func(context.Context, string, json.RawMessage) (any, error){
		notificationHandler(nil, send),
		notificationHandler([]string{"notifications.send"}, nil),
	} {
		if _, err := handler(t.Context(), "notifications.send", request); err == nil {
			t.Fatal("unavailable notification was accepted")
		}
	}
	handler := notificationHandler([]string{"notifications.send"}, send)
	large, _ := json.Marshal(protocol.NotificationParams{Title: "运行完成", Body: strings.Repeat("测", protocol.NotificationMaxBodyBytes)})
	for _, raw := range []json.RawMessage{
		[]byte(`{"title":"x","body":"x","pluginId":"other"}`),
		[]byte(`{"title":"x","body":"x\u0000"}`),
		[]byte(`{"title":" ","body":"x"}`),
		large,
	} {
		if _, err := handler(t.Context(), "notifications.send", raw); err == nil {
			t.Fatal("invalid notification was accepted")
		}
	}
	for i := 0; i < 24; i++ {
		if _, err := handler(t.Context(), "notifications.send", request); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := handler(t.Context(), "notifications.send", request); err == nil || calls != 24 {
		t.Fatal("notification rate budget was bypassed")
	}
}
