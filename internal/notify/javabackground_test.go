package notify

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type javaCall struct {
	method  string
	payload any
}

type fakeCaller struct {
	calls  []javaCall
	values map[string]string
	err    error
}

func (f *fakeCaller) call(method string, payload any) (string, error) {
	f.calls = append(f.calls, javaCall{method, payload})
	if f.err != nil {
		return "", f.err
	}
	return f.values[method], nil
}

func TestJavaBackgroundCallsJavaMethods(t *testing.T) {
	f := &fakeCaller{values: map[string]string{"notificationPermission": "denied"}}
	b := JavaBackground{Call: f.call}

	b.ScheduleSync(true)
	b.ScheduleSync(false)
	b.ForgetAccount("acct")
	b.RequestPermission()
	assert.Equal(t, PermissionDenied, b.Permission())
	require.NoError(t, b.OpenSettings())

	assert.Equal(t, []javaCall{
		{"scheduleSync", map[string]any{"enabled": true}},
		{"scheduleSync", map[string]any{"enabled": false}},
		{"forgetAccount", map[string]any{"accountId": "acct"}},
		{"requestNotificationPermission", map[string]any{}},
		{"notificationPermission", map[string]any{}},
		{"openNotificationSettings", map[string]any{}},
	}, f.calls)
}

func TestJavaBackgroundPermissionValues(t *testing.T) {
	for value, want := range map[string]Permission{
		"granted": PermissionGranted, "denied": PermissionDenied, "": PermissionUnsupported, "other": PermissionUnsupported,
	} {
		f := &fakeCaller{values: map[string]string{"notificationPermission": value}}
		assert.Equal(t, want, JavaBackground{Call: f.call}.Permission(), "value=%q", value)
	}
}

func TestJavaBackgroundFailures(t *testing.T) {
	f := &fakeCaller{err: errors.New("bridge unavailable")}
	b := JavaBackground{Call: f.call}
	// 失敗しても呼び出し側を止めない（ログだけ）。許可状態は「項目を出さない」へ。
	b.ScheduleSync(true)
	b.ForgetAccount("a")
	b.RequestPermission()
	assert.Equal(t, PermissionUnsupported, b.Permission())
	assert.Error(t, b.OpenSettings())
}
