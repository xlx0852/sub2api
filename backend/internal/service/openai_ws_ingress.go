package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

// WS ingress 三层（project.md）：
//   Session    — 客户端 WS 生命周期（mode、session hash、turn state、idle timeout）
//   Turn       — 一轮 response.create → 上游完成
//   Connection — 上游 WS lease 的 acquire / pin / release
//
// 三层共享一份可变状态，方法按层拆开，避免再混成同一个 1600 行函数。

type openAIWSClientPayload struct {
	payloadRaw         []byte
	rawForHash         []byte
	promptCacheKey     string
	previousResponseID string
	originalModel      string
	imageBillingModel  string
	imageSizeTier      string
	imageInputSize     string
	payloadBytes       int
}

type openAIWSIngressSession struct {
	svc        *OpenAIGatewayService
	ctx        context.Context
	c          *gin.Context
	clientConn *coderws.Conn
	account    *Account
	token      string
	hooks      *OpenAIWSIngressHooks

	wsDecision      OpenAIWSProtocolDecision
	ingressMode     string
	forceHTTPBridge bool
	dedicatedMode   bool
	isCodexCLI      bool
	debugEnabled    bool
	wsURL           string
	wsHost          string
	wsPath          string

	ingressSessionOriginalModel string
	fingerprintIDs              *codexFingerprintIDs
	fingerprintTurn             int

	turnState             string
	stateStore            OpenAIWSStateStore
	groupID               int64
	storeDisabledConnMode string
	sessionHash           string
	preferredConnID       string
	storeDisabled         bool

	pool                   *openAIWSConnPool
	baseAcquireReq         openAIWSAcquireRequest
	acquireTimeout         time.Duration
	agentTaskRecoveryTried bool
	forceNewConnOnRetry    bool
	sessionLease           *openAIWSConnLease
	sessionConnID          string
	pinnedSessionConnID    string
	lastTurnClean          bool

	turn                               int
	turnRetry                          int
	turnPrevRecoveryTried              bool
	turnIndexedCompatibilityRetryTried bool
	lastTurnFinishedAt                 time.Time
	lastTurnResponseID                 string
	lastTurnPayload                    []byte
	lastTurnStrictState                *openAIWSIngressPreviousTurnStrictState
	lastTurnReplayInput                []json.RawMessage
	lastTurnReplayInputExists          bool
	currentTurnReplayInput             []json.RawMessage
	currentTurnReplayInputExists       bool
	skipBeforeTurn                     bool
	currentPayload                     []byte
	currentOriginalModel               string
	currentImageBillingModel           string
	currentImageSizeTier               string
	currentImageInputSize              string
	currentPayloadBytes                int
}

func (sess *openAIWSIngressSession) refreshRouteState(payload openAIWSClientPayload) {
	s := sess.svc
	sess.sessionHash = s.GenerateSessionHash(sess.c, payload.rawForHash)
	if sess.turnState == "" && sess.stateStore != nil && sess.sessionHash != "" {
		if savedTurnState, ok := sess.stateStore.GetSessionTurnState(sess.groupID, sess.sessionHash); ok {
			sess.turnState = savedTurnState
		}
	}

	sess.preferredConnID = ""
	if sess.stateStore != nil && payload.previousResponseID != "" {
		if connID, ok := sess.stateStore.GetResponseConn(payload.previousResponseID); ok {
			sess.preferredConnID = connID
		}
	}

	sess.storeDisabled = s.isOpenAIWSStoreDisabledInRequestRaw(payload.payloadRaw, sess.account)
	if sess.stateStore != nil && sess.storeDisabled && payload.previousResponseID == "" && sess.sessionHash != "" {
		if connID, ok := sess.stateStore.GetSessionConn(sess.groupID, sess.sessionHash); ok {
			sess.preferredConnID = connID
		}
	}
}

func (sess *openAIWSIngressSession) isStrictAffinityTurn(payload []byte) bool {
	if !sess.storeDisabled {
		return false
	}
	return strings.TrimSpace(openAIWSPayloadStringFromRaw(payload, "previous_response_id")) != ""
}
