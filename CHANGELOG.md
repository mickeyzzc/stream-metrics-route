# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.1.0] - 2026-05-13

### 核心功能

- **双重 Hashmod 调度** — 基于指定标签（如 `__name__`、`job`）进行一致性哈希，确保相同维度的指标始终路由到同一后端节点，避免数据分片丢失
- **Prometheus Remote Write 接收** — 原生兼容 Prometheus remote write 协议，无缝对接现有 Prometheus/Agent 采集链路
- **Kafka 异步分发** — 可选 Kafka 生产者，将指标数据异步写入指定 Topic，解耦采集与消费
- **Relabel 规则过滤** — 完整支持 Prometheus relabeling 规则语法，在路由前对标签进行增删改过滤
- **内置熔断器** — 对每个后端实例独立维护熔断状态（Closed → Open → Half-Open），自动摘除故障节点，防止级联失败
- **多后端负载均衡** — 支持同一路由规则配置多个 Remote Write 或 Kafka 后端，自动分摊写入压力

### API 接口

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/write` | POST | Prometheus remote write 入口 |
| `/metrics` | GET | 自身 Prometheus 指标暴露 |
| `/stats` | GET | 实时路由统计信息 |
| `/-/health` | GET | 健康检查 |
| `/-/ready` | GET | 就绪检查 |

### 部署支持

- Docker 多架构镜像（amd64/arm64）
- Kubernetes 部署清单
- Linux 二进制发布（amd64/arm64）
