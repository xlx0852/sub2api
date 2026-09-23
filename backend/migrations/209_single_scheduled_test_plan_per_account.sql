-- 209: 定时测试（诊断）每个账号仅允许一份计划。
--
-- 先收敛历史重复（保留 enabled 优先、其次最近更新的那一份），再加唯一索引兜底，
-- 让并发创建/旧客户端也无法产生第二份。
DELETE FROM scheduled_test_plans dup
WHERE dup.id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY account_id
                   ORDER BY enabled DESC, updated_at DESC NULLS LAST, id DESC
               ) AS rn
        FROM scheduled_test_plans
    ) ranked
    WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_scheduled_test_plans_account
    ON scheduled_test_plans (account_id);
