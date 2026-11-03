# 路线图与目标

本文档描述短期与中期目标。服务实现了什么以 `docs/` 为准。上游 Teamgram 的社区版/企业版划分不是本仓库的功能边界。

## 短期目标

- **文档与规范**：保持 specs（架构、协议、依赖、贡献、安全、发布、路线图）与代码一致。
- **CI**：保持 `.github/workflows/` 里的工作流为绿（Publish images、`go test`、客户端回归、UI acceptance）。
- **文档一致性**：README、`docker-compose-env.yaml` 和安装文档只描述仓库里真实存在的文件。

## 中期目标

- **测试覆盖**：增加核心路径的单元测试与集成测试，逐步建立覆盖率要求。
- **可观测与运维**：补充监控栈（Prometheus、Grafana、Jaeger、ELK）使用说明与 runbook。
- **安装与部署**：覆盖更多安装场景（如其他 Linux 发行版、Kubernetes 示例），与 docker-compose-env 文档保持一致。

## 范围

功能行为写在 `docs/`（通话、存储、推送、贴纸，以及 API 缺口列表）。不要把上游 Teamgram 的企业版目录当成这份代码缺了或有了什么。生产运维见 [docs/SECURITY-OPS.md](../docs/SECURITY-OPS.md)。
