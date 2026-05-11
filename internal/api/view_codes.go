package api

import "net/http"

type msgCode string

const (
	msgServerError      msgCode = "server_error"
	msgBadRequest       msgCode = "bad_request"
	msgSessionInvalid   msgCode = "session_invalid"
	msgSessionExpired   msgCode = "session_expired"
	msgRateLimited      msgCode = "rate_limited"
	msgOAuthFailed      msgCode = "oauth_failed"
	msgDiscordBanned    msgCode = "discord_banned"
	msgNameBanned       msgCode = "name_banned"
	msgNameTaken        msgCode = "name_taken"
	msgNotRegistered    msgCode = "not_registered"
	msgRegisterFailed   msgCode = "register_failed"
	msgUnregisterFailed msgCode = "unregister_failed"
	msgUnregistered     msgCode = "unregistered"
)

func (c msgCode) status() int {
	switch c {
	case msgUnregistered:
		return http.StatusOK
	case msgBadRequest, msgSessionInvalid, msgSessionExpired:
		return http.StatusBadRequest
	case msgDiscordBanned, msgNameBanned:
		return http.StatusForbidden
	case msgNotRegistered:
		return http.StatusNotFound
	case msgNameTaken:
		return http.StatusConflict
	case msgRateLimited:
		return http.StatusTooManyRequests
	case msgOAuthFailed:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

var allMsgCodes = []msgCode{
	msgServerError,
	msgBadRequest,
	msgSessionInvalid,
	msgSessionExpired,
	msgRateLimited,
	msgOAuthFailed,
	msgDiscordBanned,
	msgNameBanned,
	msgNameTaken,
	msgNotRegistered,
	msgRegisterFailed,
	msgUnregisterFailed,
	msgUnregistered,
}

type tokenAction string

const (
	tokenActionRegister tokenAction = "register"
	tokenActionRenewal  tokenAction = "renewal"
	tokenActionRename   tokenAction = "rename"
)

var allTokenActions = []tokenAction{
	tokenActionRegister,
	tokenActionRenewal,
	tokenActionRename,
}
