package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const providerOAuthSessionKeyPrefix = "oauth:provider:session:"

var acquireProviderOAuthPollLeaseScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return {-1, ''} end
local value = cjson.decode(raw)
local now = tonumber(ARGV[1])
local next_poll = tonumber(value.next_poll_at_unix_milli or 0)
local lease_until = tonumber(value.poll_lease_until_unix_milli or 0)
if value.status ~= 'pending' or next_poll > now or lease_until > now then
  return {0, raw}
end
value.version = tonumber(value.version or 0) + 1
value.poll_lease_id = ARGV[2]
value.poll_lease_until_unix_milli = tonumber(ARGV[3])
local encoded = cjson.encode(value)
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 then
  redis.call('SET', KEYS[1], encoded, 'PX', ttl)
else
  redis.call('SET', KEYS[1], encoded)
end
return {1, encoded}
`)

var commitProviderOAuthPollScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
local current = cjson.decode(raw)
if current.status ~= 'pending' then return 0 end
if tostring(current.poll_lease_id or '') ~= ARGV[1] then return 0 end
if tonumber(current.version or 0) ~= tonumber(ARGV[2]) then return 0 end
local updated = cjson.decode(ARGV[3])
updated.id = current.id
updated.provider = current.provider
updated.flow = current.flow
updated.version = tonumber(current.version or 0) + 1
updated.poll_lease_id = nil
updated.poll_lease_until_unix_milli = nil
local encoded = cjson.encode(updated)
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 then
  redis.call('SET', KEYS[1], encoded, 'PX', ttl)
else
  redis.call('SET', KEYS[1], encoded)
end
return 1
`)

var cancelProviderOAuthSessionScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
local value = cjson.decode(raw)
if value.status == 'cancelled' then return 1 end
value.status = 'cancelled'
value.version = tonumber(value.version or 0) + 1
value.poll_lease_id = nil
value.poll_lease_until_unix_milli = nil
value.next_poll_at_unix_milli = nil
value.payload = nil
value.error = nil
local encoded = cjson.encode(value)
redis.call('SET', KEYS[1], encoded, 'PX', tonumber(ARGV[1]))
return 1
`)

var consumeAuthorizedProviderOAuthSessionScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return {0, ''} end
local value = cjson.decode(raw)
if value.status ~= 'authorized' then return {0, raw} end
redis.call('DEL', KEYS[1])
return {1, raw}
`)

type providerOAuthSessionStore struct {
	rdb *redis.Client
}

func NewProviderOAuthSessionStore(rdb *redis.Client) service.ProviderOAuthSessionStore {
	return &providerOAuthSessionStore{rdb: rdb}
}

func (s *providerOAuthSessionStore) Create(ctx context.Context, session *service.ProviderOAuthSession, ttl time.Duration) error {
	if err := s.validate(session, ttl); err != nil {
		return err
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	created, err := s.rdb.SetNX(ctx, providerOAuthSessionKey(session.ID), payload, ttl).Result()
	if err != nil {
		return err
	}
	if !created {
		return errors.New("provider OAuth session already exists")
	}
	return nil
}

func (s *providerOAuthSessionStore) Get(ctx context.Context, sessionID string) (*service.ProviderOAuthSession, error) {
	if s == nil || s.rdb == nil {
		return nil, errors.New("provider OAuth session store is not configured")
	}
	payload, err := s.rdb.Get(ctx, providerOAuthSessionKey(sessionID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeProviderOAuthSession(payload)
}

func (s *providerOAuthSessionStore) AcquirePollLease(ctx context.Context, sessionID string, now time.Time, leaseTTL time.Duration) (*service.ProviderOAuthSessionLease, error) {
	if s == nil || s.rdb == nil {
		return nil, errors.New("provider OAuth session store is not configured")
	}
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Second
	}
	leaseID, err := randomProviderOAuthLeaseID()
	if err != nil {
		return nil, err
	}
	result, err := acquireProviderOAuthPollLeaseScript.Run(ctx, s.rdb, []string{providerOAuthSessionKey(sessionID)}, now.UnixMilli(), leaseID, now.Add(leaseTTL).UnixMilli()).Result()
	if err != nil {
		return nil, err
	}
	items, ok := result.([]any)
	if !ok || len(items) != 2 {
		return nil, fmt.Errorf("invalid provider OAuth lease result %T", result)
	}
	code, err := redisScriptInt64(items[0])
	if err != nil {
		return nil, err
	}
	if code < 0 {
		return &service.ProviderOAuthSessionLease{}, nil
	}
	raw := fmt.Sprint(items[1])
	session, err := decodeProviderOAuthSession([]byte(raw))
	if err != nil {
		return nil, err
	}
	return &service.ProviderOAuthSessionLease{Session: session, ID: leaseID, Held: code == 1}, nil
}

func (s *providerOAuthSessionStore) CommitPoll(ctx context.Context, lease *service.ProviderOAuthSessionLease, updated *service.ProviderOAuthSession) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("provider OAuth session store is not configured")
	}
	if lease == nil || !lease.Held || lease.Session == nil || lease.ID == "" || updated == nil {
		return false, errors.New("provider OAuth poll lease is invalid")
	}
	payload, err := json.Marshal(updated)
	if err != nil {
		return false, err
	}
	result, err := commitProviderOAuthPollScript.Run(ctx, s.rdb, []string{providerOAuthSessionKey(lease.Session.ID)}, lease.ID, lease.Session.Version, payload).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (s *providerOAuthSessionStore) Cancel(ctx context.Context, sessionID string, tombstoneTTL time.Duration) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("provider OAuth session store is not configured")
	}
	if tombstoneTTL <= 0 {
		tombstoneTTL = 5 * time.Minute
	}
	result, err := cancelProviderOAuthSessionScript.Run(ctx, s.rdb, []string{providerOAuthSessionKey(sessionID)}, tombstoneTTL.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (s *providerOAuthSessionStore) ConsumeAuthorized(ctx context.Context, sessionID string) (*service.ProviderOAuthSession, error) {
	if s == nil || s.rdb == nil {
		return nil, errors.New("provider OAuth session store is not configured")
	}
	result, err := consumeAuthorizedProviderOAuthSessionScript.Run(ctx, s.rdb, []string{providerOAuthSessionKey(sessionID)}).Result()
	if err != nil {
		return nil, err
	}
	items, ok := result.([]any)
	if !ok || len(items) != 2 {
		return nil, fmt.Errorf("invalid provider OAuth consume result %T", result)
	}
	code, err := redisScriptInt64(items[0])
	if err != nil {
		return nil, err
	}
	if code != 1 {
		return nil, nil
	}
	return decodeProviderOAuthSession([]byte(fmt.Sprint(items[1])))
}

func (s *providerOAuthSessionStore) Delete(ctx context.Context, sessionID string) error {
	if s == nil || s.rdb == nil {
		return errors.New("provider OAuth session store is not configured")
	}
	return s.rdb.Del(ctx, providerOAuthSessionKey(sessionID)).Err()
}

func (s *providerOAuthSessionStore) validate(session *service.ProviderOAuthSession, ttl time.Duration) error {
	if s == nil || s.rdb == nil {
		return errors.New("provider OAuth session store is not configured")
	}
	if session == nil || session.ID == "" || session.Provider == "" || session.Flow == "" {
		return errors.New("provider OAuth session is invalid")
	}
	if ttl <= 0 {
		return errors.New("provider OAuth session has expired")
	}
	return nil
}

func providerOAuthSessionKey(sessionID string) string {
	return providerOAuthSessionKeyPrefix + sessionID
}

func decodeProviderOAuthSession(payload []byte) (*service.ProviderOAuthSession, error) {
	var session service.ProviderOAuthSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func randomProviderOAuthLeaseID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func redisScriptInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	default:
		return 0, fmt.Errorf("invalid Redis script integer %T", value)
	}
}
