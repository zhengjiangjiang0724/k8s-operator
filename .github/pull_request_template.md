## Summary

<!-- 1-3 句话说明改了什么、为什么。 -->

## Test plan

- [ ] `make test` 全绿
- [ ] `make lint` 0 issues
- [ ] 涉及 controller/webhook 改动：手动 `make test-e2e` 验证一次
- [ ] 涉及 CRD schema 改动：v1alpha1 ↔ v1beta1 conversion 仍然 round-trip
- [ ] 涉及 metrics/SLI 改动：dashboard / runbook 已同步更新

## Type

- [ ] feat — 新功能
- [ ] fix — bug 修复
- [ ] refactor — 重构（行为不变）
- [ ] docs — 仅文档
- [ ] chore — 工具链 / CI / 依赖
- [ ] test — 仅测试

## Breaking change?

- [ ] **是** —— 影响范围：<!-- 哪些 API / 字段 / CR / metric / chart value -->
- [ ] 否

## Linked issues

Closes #
