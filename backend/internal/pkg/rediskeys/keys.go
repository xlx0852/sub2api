// Package rediskeys centralizes Redis key formats shared by repositories and services.
// Keep these builders pure: they must not import domain, repository, or service.
package rediskeys

import "strconv"

const (
	tempUnschedAccountPrefix    = "temp_unsched:account:"
	schedulerAccountPrefix      = "sched:acc:"
	schedulerAccountMetaPrefix  = "sched:meta:"
	schedulerBucketSetKey       = "sched:buckets"
	schedulerOutboxWatermarkKey = "sched:outbox:watermark"
)

func TempUnschedAccount(accountID int64) string {
	return tempUnschedAccountPrefix + strconv.FormatInt(accountID, 10)
}

func SchedulerAccount(accountID int64) string {
	return schedulerAccountString(strconv.FormatInt(accountID, 10))
}

func SchedulerAccountString(accountID string) string {
	return schedulerAccountString(accountID)
}

func schedulerAccountString(accountID string) string {
	return schedulerAccountPrefix + accountID
}

func SchedulerAccountMeta(accountID int64) string {
	return SchedulerAccountMetaString(strconv.FormatInt(accountID, 10))
}

func SchedulerAccountMetaString(accountID string) string {
	return schedulerAccountMetaPrefix + accountID
}

func SchedulerBucketSet() string {
	return schedulerBucketSetKey
}

func SchedulerOutboxWatermark() string {
	return schedulerOutboxWatermarkKey
}

func SchedulerBucket(prefix string, groupID int64, platform, mode string) string {
	return prefix + strconv.FormatInt(groupID, 10) + ":" + platform + ":" + mode
}

func SchedulerSnapshot(groupID int64, platform, mode, version string) string {
	return SchedulerBucket("sched:", groupID, platform, mode) + ":v" + version
}
