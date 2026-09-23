package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 上游改票的形态时，本地过滤会连续拒票。必须在很短时间内变成 ERROR，
// 而不是像 2026-09-23 那次一样在 INFO 里沉寂 19 小时。
func TestProbeWatchAlertsOnRepeatedShapeRejection(t *testing.T) {
	resetCodexTurnStateProbeWatch()
	// 阈值是 4：一次 seed 作业就探测 6 个模型，所以第一轮内必然触发。
	for i := 1; i <= codexTurnStateRejectedShapeStreak; i++ {
		fire, streak, lengths := observeCodexTurnStateProbeOutcome("rejected_shape", 780)
		require.Equal(t, i, streak)
		if i < codexTurnStateRejectedShapeStreak {
			require.False(t, fire, "第 %d 次还不该告警", i)
			continue
		}
		require.True(t, fire, "达到阈值必须告警")
		require.Equal(t, []int{780}, lengths, "告警里要带上观察到的票长")
	}
	// 同一段异常只报一次，避免刷屏。
	fire, _, _ := observeCodexTurnStateProbeOutcome("rejected_shape", 780)
	require.False(t, fire, "同一段连续异常不得重复告警")
}

func TestProbeWatchResetsOnSuccess(t *testing.T) {
	resetCodexTurnStateProbeWatch()
	for i := 0; i < codexTurnStateRejectedShapeStreak-1; i++ {
		observeCodexTurnStateProbeOutcome("rejected_shape", 780)
	}
	// 成功一次 → 清零
	observeCodexTurnStateProbeOutcome("", 0)
	fire, streak, _ := observeCodexTurnStateProbeOutcome("rejected_shape", 780)
	require.False(t, fire)
	require.Equal(t, 1, streak, "成功后必须从头计数")
}

func TestProbeWatchResetsWhenAnomalyKindChanges(t *testing.T) {
	resetCodexTurnStateProbeWatch()
	for i := 0; i < codexTurnStateRejectedShapeStreak-1; i++ {
		observeCodexTurnStateProbeOutcome("rejected_shape", 780)
	}
	// 换成另一类异常 → 重新计数，不叠加
	fire, streak, _ := observeCodexTurnStateProbeOutcome("incomplete_stream", 0)
	require.False(t, fire)
	require.Equal(t, 1, streak)
}

// 「上游不发」与「流没跑完」很可能是上游短时行为，阈值取大以免噪声；
// 但持续两轮以上仍说明采票停摆，必须报出来。
func TestProbeWatchUsesHigherThresholdForUpstreamSideAnomalies(t *testing.T) {
	resetCodexTurnStateProbeWatch()
	for i := 1; i < codexTurnStateOtherAnomalyStreak; i++ {
		fire, _, _ := observeCodexTurnStateProbeOutcome("upstream_sent_none", 0)
		require.False(t, fire, "第 %d 次不该告警", i)
	}
	fire, streak, _ := observeCodexTurnStateProbeOutcome("upstream_sent_none", 0)
	require.True(t, fire)
	require.Equal(t, codexTurnStateOtherAnomalyStreak, streak)
}

// 告警里的长度列表要去重、排序，并且有上限（上游乱发长度时不能让它无限增长）。
func TestProbeWatchLengthsAreBounded(t *testing.T) {
	resetCodexTurnStateProbeWatch()
	// 第 codexTurnStateRejectedShapeStreak 次就会触发，长度取自这一段异常里出现过的值。
	var fire bool
	var lengths []int
	for i := 0; i < codexTurnStateRejectedShapeStreak; i++ {
		fire, _, lengths = observeCodexTurnStateProbeOutcome("rejected_shape", 800-i)
	}
	require.True(t, fire)
	require.Equal(t, []int{797, 798, 799, 800}, lengths, "去重且有序")
	require.LessOrEqual(t, len(lengths), codexTurnStateWatchMaxLengths)
	for i := 1; i < len(lengths); i++ {
		require.LessOrEqual(t, lengths[i-1], lengths[i], "必须有序")
	}
}
