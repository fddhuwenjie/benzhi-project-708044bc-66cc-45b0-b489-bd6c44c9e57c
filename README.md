# 种质资源萌发复壮批次管理服务

面向种质库团队的单流程 HTTP JSON 服务，覆盖批次建档、身份核验、萌发试验、异常复壮、科学审核和保藏证据归档。服务默认监听 `127.0.0.1:19081`，可通过 `-addr` 指定完整地址；未传 `-addr` 时也可用 `PORT` 指定回环端口。

## 运行与自检

```bash
go test ./...
go run ./cmd/seedvault -self-check -addr=127.0.0.1:19081
go run ./cmd/seedvault -addr=127.0.0.1:19081
```

## 主流程 API

- `POST /api/v1/batches` 创建草稿。条目使用 `accession_id`、`source_id`、`taxon_name`、`collection_site`、`collection_year`、`storage_generation`、`seed_count` 和 `baseline_viability`。响应中的 `readiness` 按条目列出阻断项和提醒；详情查询会重新计算同一摘要，但不增加修订或事件。
- `POST /api/v1/batches/{id}/identity` 提交 `evidence`、`confirmed_by` 和可选 `expected_revision`。每个条目的证据包含 `collection_record`、`taxonomic_identification`、`storage_history`，每份证据包含 `evidence_id`、`claimed_value` 和 `source_ref`。冲突响应通过 `details.conflict_matrix` 定位字段。
- `POST /api/v1/batches/{id}/protocol` 在 `entries` 中逐条目提交样本量、`treatment_groups` 配额、温湿度边界、观察日和阈值。锁定后保存稳定排序的 `protocol_id`、`version` 和 `digest`。
- `POST /api/v1/batches/{id}/observations` 使用 `observations` 数组原子提交同一 `day_index` 的多条记录。`GET` 返回有效记录、累计活力、完成度和下一观察日。
- `POST /api/v1/batches/{id}/observations/{observation_id}/correction` 使用 `correction`、`reason`、`operator_id` 更正试验未结束时的记录；`accession_id` 和 `day_index` 不可改变。
- `POST /api/v1/batches/{id}/remediation` 建立复壮案例；`POST /api/v1/batches/{id}/retest` 提交 `case_id`、四类原始计数、`sample_size`、`resolution` 和可选 `request_id`。服务确定性计算前后指标和效果分级。
- `POST /api/v1/batches/{id}/review` 提交五类 `checks`，类别为 `identity_chain`、`protocol_deviation`、`observation_completeness`、`threshold_determination`、`remediation_closure`；`decision` 为 `approve` 或 `return`。
- `POST /api/v1/batches/{id}/archive` 生成归档包。`GET /evidence-package?segment=identity|protocol|observation|remediation|release_decision` 可分段读取，`GET /integrity` 返回逐段摘要、事件连续性、链头和首个异常修订诊断。

所有业务写请求都支持 `X-Expected-Revision`；新结构也可在请求体中传 `expected_revision`。归档批次保持只读，详情、事件、证据包和完整性诊断仍可查询。
