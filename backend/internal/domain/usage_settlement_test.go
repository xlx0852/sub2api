package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageSettlementAttemptCanRebindStartedAccount(t *testing.T) {
	started := &UsageSettlementAttempt{
		BillingIdentity: "client:failover", RequestFingerprint: strings.Repeat("a", 64),
		APIKeyID: 84, UserID: 37, AccountID: 108, State: UsageSettlementStateStarted,
	}
	require.True(t, started.CanRebindStartedAccount(37, 73, started.RequestFingerprint))
	require.False(t, started.CanRebindStartedAccount(37, 108, started.RequestFingerprint))
	require.False(t, started.CanRebindStartedAccount(38, 73, started.RequestFingerprint))
	require.False(t, started.CanRebindStartedAccount(37, 73, strings.Repeat("b", 64)))

	started.PayloadHash = strings.Repeat("c", 64)
	require.False(t, started.CanRebindStartedAccount(37, 73, started.RequestFingerprint))

	ready := &UsageSettlementAttempt{
		BillingIdentity: "client:failover", RequestFingerprint: strings.Repeat("a", 64),
		APIKeyID: 84, UserID: 37, AccountID: 108, State: UsageSettlementStateReady,
	}
	require.False(t, ready.CanRebindStartedAccount(37, 73, ready.RequestFingerprint))
}

func TestUsageSettlementAttemptRejectsOversizedRequestFingerprint(t *testing.T) {
	attempt := &UsageSettlementAttempt{
		BillingIdentity: "local:test", RequestFingerprint: strings.Repeat("f", 65),
		APIKeyID: 1, UserID: 1, AccountID: 1, Provider: "openai",
		State: UsageSettlementStateStarted,
	}
	require.EqualError(t, attempt.ValidateStarted(), "usage settlement request fingerprint exceeds 64 characters")
}

func TestUsageSettlementTransitions(t *testing.T) {
	require.True(t, CanTransitionUsageSettlement(UsageSettlementStateStarted, UsageSettlementStateReady))
	require.True(t, CanTransitionUsageSettlement(UsageSettlementStateProcessing, UsageSettlementStateSettled))
	require.True(t, CanTransitionUsageSettlement(UsageSettlementStateReconciliationRequired, UsageSettlementStateReady))
	require.False(t, CanTransitionUsageSettlement(UsageSettlementStateSettled, UsageSettlementStateReady))
	require.ErrorIs(t, ValidateUsageSettlementTransition(UsageSettlementStateSettled, UsageSettlementStateReady), ErrUsageSettlementInvalidTransition)
}

func TestUsageSettlementPayloadRoundTripExcludesRelations(t *testing.T) {
	createdAt := time.Unix(1700000000, 0).UTC()
	pricingCurrency := string(PricingCurrencyCNY)
	preUpstreamMs, totalMs := 120, 980
	command := &UsageBillingCommand{RequestID: "local:turn-1", APIKeyID: 2, UserID: 3, AccountID: 4, AccountType: AccountTypeAPIKey, InputTokens: 10, BalanceCost: 0.1}
	command.Normalize()
	log := &UsageLog{
		UserID: 3, APIKeyID: 2, AccountID: 4, RequestID: "local:turn-1", Model: "gpt-test",
		InputTokens: 10, ActualCost: 0.1, PricingCurrency: &pricingCurrency, CreatedAt: createdAt,
		FirstOutputStages: &FirstOutputStages{GatewayPreUpstreamMs: &preUpstreamMs, GatewayFirstOutputTotalMs: &totalMs},
		User:              &User{Email: "must-not-be-serialized@example.com"},
		Account:           &Account{Credentials: map[string]any{"access_token": "secret"}},
	}
	payload := NewUsageSettlementPayloadV1(command, log, "openai")
	encoded, hash, err := payload.CanonicalJSON("local:turn-1", 2, 3, 4)
	require.NoError(t, err)
	require.Len(t, hash, 64)
	require.NotContains(t, string(encoded), "must-not-be-serialized")
	require.NotContains(t, string(encoded), "access_token")

	var decoded UsageSettlementPayloadV1
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, command.RequestFingerprint, decoded.Billing.RequestFingerprint)
	require.Equal(t, createdAt, decoded.UsageLog.CreatedAt)
	require.Equal(t, log.RequestID, decoded.UsageLog.UsageLog().RequestID)
	require.Equal(t, pricingCurrency, *decoded.UsageLog.UsageLog().PricingCurrency)
	require.Equal(t, preUpstreamMs, *decoded.UsageLog.UsageLog().FirstOutputStages.GatewayPreUpstreamMs)
	require.Equal(t, totalMs, *decoded.UsageLog.UsageLog().FirstOutputStages.GatewayFirstOutputTotalMs)
}

func TestUsageSettlementPayloadRejectsIdentityMismatch(t *testing.T) {
	command := &UsageBillingCommand{RequestID: "local:turn-1", APIKeyID: 2, UserID: 3, AccountID: 4}
	command.Normalize()
	payload := NewUsageSettlementPayloadV1(command, &UsageLog{RequestID: "local:turn-1", APIKeyID: 2, UserID: 3, AccountID: 4}, "")
	require.Error(t, payload.Validate("local:turn-2", 2, 3, 4))
}

func TestValidateUsageSettlementPayloadJSONRejectsPromptResponseAndCredentials(t *testing.T) {
	for name, encoded := range map[string][]byte{
		"prompt":      []byte(`{"usage_log":{"prompt":"secret"}}`),
		"messages":    []byte(`{"usage_log":{"messages":[]}}`),
		"response":    []byte(`{"usage_log":{"response_body":"secret"}}`),
		"credentials": []byte(`{"billing":{"credentials":{"access_token":"secret"}}}`),
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, ValidateUsageSettlementPayloadJSON(encoded), ErrUsageSettlementSensitivePayload)
		})
	}
	require.NoError(t, ValidateUsageSettlementPayloadJSON([]byte(`{"usage_log":{"input_tokens":10,"output_tokens":2,"user_agent":"codex/test"}}`)))
}

// 结算 payload 快照是唯一真正落库 usage_logs 的路径：UsageLogSnapshot 少一个
// 字段，该字段就会被静默丢弃（历史上 upstream_response_model 就踩过这个坑）。
// 这个守卫用反射填充所有字段做往返，确保快照与实体保持同步。
func TestUsageLogSnapshotRoundTripKeepsEveryField(t *testing.T) {
	source := UsageLog{}
	fillUsageLogForSnapshotTest(reflect.ValueOf(&source).Elem())

	roundTripped := NewUsageLogSnapshot(&source).UsageLog()

	// 有意不参与快照往返的字段：
	//   ID              —— 由数据库生成
	//   PricingPolicyID —— ChannelID 的历史别名，UsageLog() 用 ChannelID 回填
	//   AccountStatsCost—— legacy overlay，新写入恒为 nil
	//   其余关联实体（User/APIKey/Account/Group/Subscription）不是 usage_logs 列，
	//   只用于计费事务内的副作用，快照按设计不承载它们。
	omitted := map[string]bool{
		"ID": true, "PricingPolicyID": true, "AccountStatsCost": true,
		"User": true, "APIKey": true, "Account": true, "Group": true, "Subscription": true,
	}

	sourceValue := reflect.ValueOf(source)
	roundTrippedValue := reflect.ValueOf(*roundTripped)
	sourceType := sourceValue.Type()
	for i := 0; i < sourceType.NumField(); i++ {
		name := sourceType.Field(i).Name
		if omitted[name] {
			continue
		}
		if !reflect.DeepEqual(sourceValue.Field(i).Interface(), roundTrippedValue.Field(i).Interface()) {
			t.Errorf("UsageLogSnapshot 往返丢失字段 %s: %#v -> %#v",
				name, sourceValue.Field(i).Interface(), roundTrippedValue.Field(i).Interface())
		}
	}
}

func fillUsageLogForSnapshotTest(value reflect.Value) {
	valueType := value.Type()
	for i := 0; i < valueType.NumField(); i++ {
		field := value.Field(i)
		if !field.CanSet() {
			continue
		}
		switch field.Kind() {
		case reflect.String:
			field.SetString("field-" + valueType.Field(i).Name)
		case reflect.Bool:
			field.SetBool(true)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			field.SetInt(7)
		case reflect.Float32, reflect.Float64:
			field.SetFloat(1.5)
		case reflect.Map:
			populated := reflect.MakeMap(field.Type())
			if field.Type().Key().Kind() == reflect.String && field.Type().Elem().Kind() == reflect.Int {
				populated.SetMapIndex(reflect.ValueOf("size").Convert(field.Type().Key()), reflect.ValueOf(2).Convert(field.Type().Elem()))
			}
			field.Set(populated)
		case reflect.Struct:
			if field.Type() == reflect.TypeOf(time.Time{}) {
				field.Set(reflect.ValueOf(time.Unix(1700000000, 0).UTC()))
				continue
			}
			if field.CanAddr() {
				fillUsageLogForSnapshotTest(field)
			}
		case reflect.Ptr:
			field.Set(reflect.New(field.Type().Elem()))
			if field.Type().Elem().Kind() == reflect.Struct {
				fillUsageLogForSnapshotTest(field.Elem())
			}
		}
	}
}
