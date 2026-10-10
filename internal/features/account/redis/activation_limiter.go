// internal/features/account/redis/activation_limiter.go
package accountredis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const activationKeyPrefix = "sonnda:professional-activation:"

var recordFailureScript = redis.NewScript(`
local account_count = redis.call('INCR', KEYS[1])
if account_count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local origin_count = redis.call('INCR', KEYS[2])
if origin_count == 1 then
  redis.call('PEXPIRE', KEYS[2], ARGV[1])
end
return {account_count, origin_count}
`)

type ActivationLimiter struct {
	client redis.UniversalClient
}

func NewActivationLimiter(client redis.UniversalClient) *ActivationLimiter {
	return &ActivationLimiter{client: client}
}

func (l *ActivationLimiter) CanAttempt(ctx context.Context, accountID uuid.UUID, origin string, limit int, _ time.Duration) (bool, error) {
	if l == nil || l.client == nil {
		return false, fmt.Errorf("redis client is not configured")
	}
	values, err := l.client.MGet(ctx, activationKeys(accountID, origin)...).Result()
	if err != nil {
		return false, err
	}
	for _, value := range values {
		if value == nil {
			continue
		}
		count, err := strconv.Atoi(fmt.Sprint(value))
		if err != nil {
			return false, fmt.Errorf("parse activation attempt count: %w", err)
		}
		if count >= limit {
			return false, nil
		}
	}
	return true, nil
}

func (l *ActivationLimiter) RecordFailure(ctx context.Context, accountID uuid.UUID, origin string, window time.Duration) error {
	if l == nil || l.client == nil {
		return fmt.Errorf("redis client is not configured")
	}
	_, err := recordFailureScript.Run(ctx, l.client, activationKeys(accountID, origin), window.Milliseconds()).Result()
	return err
}

func activationKeys(accountID uuid.UUID, origin string) []string {
	digest := sha256.Sum256([]byte(origin))
	return []string{
		activationKeyPrefix + "account:" + accountID.String(),
		activationKeyPrefix + "origin:" + hex.EncodeToString(digest[:]),
	}
}

var _ account.ActivationLimiter = (*ActivationLimiter)(nil)
