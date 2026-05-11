package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// 全 msgCode に対応する `msg_<code>` define が message.html に存在し、
// Execute がエラーなく動作することを検証する。dispatcher パターンは未定義 name で
// 実行時エラーになるため、コード追加時の漏れをここで早期検出する。
func TestAllMsgCodesHaveTemplateDefine(t *testing.T) {
	for _, code := range allMsgCodes {
		name := fmt.Sprintf("msg_%s", code)
		if tplMessage.Lookup(name) == nil {
			t.Errorf("template define %q not found for msgCode %q", name, code)
		}
	}
}

func TestRenderMessageCodeExecutes(t *testing.T) {
	for _, code := range allMsgCodes {
		t.Run(string(code), func(t *testing.T) {
			data := struct {
				Code        msgCode
				DisplayName string
			}{Code: code, DisplayName: "sample-name"}
			var buf bytes.Buffer
			if err := tplMessage.Execute(&buf, data); err != nil {
				t.Fatalf("execute %q: %v", code, err)
			}
			body := buf.String()
			if !strings.Contains(body, "<title>") {
				t.Errorf("rendered body for %q missing <title>", code)
			}
		})
	}
}

// 全 tokenAction に対応する `tok_heading_<action>` define が token.html に存在することを検証する。
func TestAllTokenActionsHaveTemplateDefine(t *testing.T) {
	for _, action := range allTokenActions {
		name := fmt.Sprintf("tok_heading_%s", action)
		if tplToken.Lookup(name) == nil {
			t.Errorf("template define %q not found for tokenAction %q", name, action)
		}
	}
}

func TestRenderTokenExecutes(t *testing.T) {
	for _, action := range allTokenActions {
		t.Run(string(action), func(t *testing.T) {
			data := struct {
				Action      tokenAction
				DisplayName string
				JWT         string
			}{Action: action, DisplayName: "alice", JWT: "header.payload.sig"}
			var buf bytes.Buffer
			if err := tplToken.Execute(&buf, data); err != nil {
				t.Fatalf("execute %q: %v", action, err)
			}
		})
	}
}

func TestSharedFragmentsAvailableInPortalAndMessage(t *testing.T) {
	fragments := []string{"frag_name_taken_notice", "frag_name_banned_notice"}
	for _, name := range fragments {
		if tplMessage.Lookup(name) == nil {
			t.Errorf("tplMessage missing fragment %q", name)
		}
		if tplPortal.Lookup(name) == nil {
			t.Errorf("tplPortal missing fragment %q", name)
		}
	}
}

func TestMsgCodeStatusMapping(t *testing.T) {
	cases := map[msgCode]int{
		msgServerError:      http.StatusInternalServerError,
		msgBadRequest:       http.StatusBadRequest,
		msgSessionInvalid:   http.StatusBadRequest,
		msgSessionExpired:   http.StatusBadRequest,
		msgRateLimited:      http.StatusTooManyRequests,
		msgOAuthFailed:      http.StatusBadGateway,
		msgDiscordBanned:    http.StatusForbidden,
		msgNameBanned:       http.StatusForbidden,
		msgNameTaken:        http.StatusConflict,
		msgNotRegistered:    http.StatusNotFound,
		msgRegisterFailed:   http.StatusInternalServerError,
		msgUnregisterFailed: http.StatusInternalServerError,
		msgUnregistered:     http.StatusOK,
	}
	for code, want := range cases {
		if got := code.status(); got != want {
			t.Errorf("%q.status() = %d, want %d", code, got, want)
		}
	}
	// allMsgCodes と cases の網羅性チェック: 新規 code を追加したらこのテストも更新する。
	if len(cases) != len(allMsgCodes) {
		t.Errorf("status mapping covers %d codes, but allMsgCodes has %d", len(cases), len(allMsgCodes))
	}
}
