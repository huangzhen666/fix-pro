# PG-003 数据处理决策

- 决策日期：2026-08-07
- 决策：保留现有 MySQL 数据并迁移到 PostgreSQL
- 原因：源库已经包含人工创建的分类、SKU、媒体和发布版本，不能视为纯空库
- 写入策略：不双写；最终切换时停止 MySQL 写入后执行最终复制
- 旧库策略：切换后只读保留，不在本次任务中删除 MySQL 数据目录
- 回滚边界：PostgreSQL 产生新写入后，不直接恢复 MySQL 写入

## 已识别有效数据

- organization：1
- customer：1
- service_category：6
- service_sku：1
- media_asset：1
- service_sku_media：1
- service_sku_version：1
- 订单及购物车相关表：当前为 0

