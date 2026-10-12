-- Rename only the original seeded organization; preserve custom tenant names.
UPDATE organization
SET name = '懂修匠本地演示'
WHERE id = 1 AND name = 'FixPro Local';
