package rediskeys

import "testing"

func TestKeyBuilders(t *testing.T) {
	if got := TempUnschedAccount(42); got != "temp_unsched:account:42" {
		t.Fatalf("unexpected temp unsched key: %s", got)
	}
	if got := SchedulerAccount(42); got != "sched:acc:42" {
		t.Fatalf("unexpected scheduler account key: %s", got)
	}
	if got := SchedulerAccountMeta(42); got != "sched:meta:42" {
		t.Fatalf("unexpected scheduler metadata key: %s", got)
	}
	if got := SchedulerBucket("sched:active:", 7, "anthropic", "normal"); got != "sched:active:7:anthropic:normal" {
		t.Fatalf("unexpected scheduler bucket key: %s", got)
	}
	if got := SchedulerSnapshot(7, "anthropic", "normal", "9"); got != "sched:7:anthropic:normal:v9" {
		t.Fatalf("unexpected scheduler snapshot key: %s", got)
	}
}
