# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.1.2] - 2026-05-13

### Added

- ✨ 完整的中英文文档系统
  - `docs/README.md` - 英文详细文档
  - `docs/README_zh.md` - 中文详细文档
  - `docs/architecture.md` - 英文架构设计文档
  - `docs/architecture_zh.md` - 中文架构设计文档
- 📊 GitHub CI/CD 自动发布系统
  - 支持 `v*` 标签自动触发发布
  - 自动构建多架构 Docker 镜像（amd64/arm64）
  - 自动编译多平台二进制文件
  - 自动从 CHANGELOG.md 提取发布说明
  - 发布到 GitHub Container Registry (ghcr.io)
- 📝 CHANGELOG.md 版本变更记录
- 🎯 项目结构说明和 API 接口文档
- 📦 Kubernetes 部署清单

### Changed

- 📝 更新主 README.md，添加多语言文档导航
- 📝 添加核心特性表格说明
- 📝 添加完整的 API 接口文档
- 📝 更新配置示例格式

### Documentation

- 📖 添加双重 Hashmod 算法详解
- 📖 添加架构流程图和数据流图
- 📖 添加熔断器状态机说明
- 📖 添加开发和部署指南

## [v0.1.1] - 2024-01-15

### Added

- 🔌 支持 Prometheus Remote Write 协议
- 📡 Kafka 生产者集成
- 🛡️ 内置熔断器模式
- 🔄 Prometheus Relabel 规则支持

## [v0.1.0] - 2023-12-01

### Added

- 🎉 项目初始化
- 🚀 双重 Hashmod 调度算法
- 🌐 HTTP 接收层
- 📦 基础路由功能
