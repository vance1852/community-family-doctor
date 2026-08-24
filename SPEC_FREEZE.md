# 社区医疗家庭医生基础项目规格冻结表

| 维度 | 冻结决策 |
|---|---|
| 业务边界 | 面向社区卫生服务中心的家庭医生签约、居民健康档案、随访采样、检验复核、慢病风险事件、转诊处置、服务许可与遥测告警；不做诊断建议、处方生成或急救决策。 |
| 角色与身份 | `protection_supervisor` 映射家庭医生团队负责人，`field_operator` 映射全科医生/护士，`lab_analyst` 映射检验人员；登录、服务端会话、主动退出撤销、过期和认证代次全部持久化。 |
| 持久化 | SQLite（modernc 纯 Go 驱动）作为真实关系数据库，开启外键、WAL、busy timeout；所有写操作经过 repository transaction。 |
| Migration | `internal/repository/sqlite/migrations/001_initial.sql` 由嵌入式 FS 加载，`schema_migrations` 记录版本并在启动时加锁、幂等执行。 |
| 关联表 | organizations、users、sessions、water_sources（医疗服务网络）、protection_zones（社区片区）、monitoring_stations（基层门诊点）、sampling_plans（随访计划）、samples/custody_events、lab_results、incidents、remediation_plans/actions、permits/discharge_events、telemetry_readings、alert_jobs、audit_events、outbox_events、idempotency_records。 |
| 事务与状态机 | 预约/随访采集、样本交接、检验复核、风险事件处置、许可和幂等上报均跨实体事务；状态流通过领域 `CanTransition` 和数据库约束校验。 |
| 并发控制 | 版本列乐观锁、序列行锁、SQLite 事务锁、事件指挥租约（token/generation/expiry）和 worker lease 防止重复处理。 |
| Context | HTTP request context 贯穿认证、服务、repository、迁移和 worker；取消后停止重试、释放租约并等待优雅关闭。 |
| Worker 与恢复 | alert/outbox worker 使用租约、有限重试、退避和永久失败记录；重启后从数据库恢复待处理任务。 |
| 错误传播 | 领域验证错误、冲突、未授权、未找到和存储错误分层包装，HTTP 层稳定映射为 JSON 状态码并保留 request id。 |
| HTTP | `/healthz`、`/readyz`、`/v1/auth/*` 以及受角色保护的 source/sampling/laboratory/incident/permit/remediation/telemetry API；输入校验、幂等键和审计在服务层完成。 |
| Docker | 多阶段 `golang:1.22.5-bookworm` 构建 `./cmd/server`，distroless nonroot 运行，数据卷位于 `/data`，入口和环境变量均来自真实目录。 |
| 测试 | 单元、HTTP、集成、迁移、认证、状态机、事务回滚、幂等、worker、重启恢复测试；执行普通、race、vet、build 和容器健康/就绪检查。 |
| 规模门禁 | `compact_10`：目标 10 题；生产 Go ≥2000 行、≥20 文件、≥10 package；测试 Go ≥1500 行。当前基础版本不得包含具体 Bug、题面、私测或任务分支。 |
| 禁止题材 | 不涉及支付、博彩、武器、恶意攻击、真实个人敏感数据或自动医疗诊断；仅使用合成居民与机构数据。 |
| 后续出题容量 | 预留至少 10 个彼此独立运行时边界：认证代次、会话过期、预约状态、样本序列、交接状态、检验复核、风险事件租约、许可窗口、幂等上报、worker 重试/恢复；本阶段不植入缺陷。 |

冻结日期：2026-08-24（Asia/Shanghai）  
基础项目档位：`compact_10`  
项目形态：`backend`  
