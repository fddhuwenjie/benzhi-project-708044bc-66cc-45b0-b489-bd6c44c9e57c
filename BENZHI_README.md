# BENZHI_README

## 项目说明
- 项目：benzhi-project-708044bc-66cc-45b0-b489-bd6c44c9e57c
- 项目用途：种质资源萌发复壮批次管理 HTTP 服务，覆盖身份核验、萌发观察、异常复壮、审核和证据归档全流程。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：种质资源萌发复壮批次管理服务
- 项目介绍：面向植物种质资源保藏团队的单流程 HTTP 服务，将低活力种质从批次建档、身份核验、萌发试验、异常复壮一直推进到保藏释放与证据封存，确保每次状态变化均可并发校验、幂等重放和审计追溯。
- 项目概述：面向植物种质资源保藏团队的单流程 HTTP 服务，将低活力种质从批次建档、身份核验、萌发试验、异常复壮一直推进到保藏释放与证据封存，确保每次状态变化均可并发校验、幂等重放和审计追溯。
- 核心工作流：保藏员创建待评估批次并登记种质条目后，依次完成来源身份核验、萌发方案锁定、试验观测与判定；出现异常时进入复壮处置并以复测结果回到待审核状态，审核通过后生成保藏释放决定和不可变证据包，批次按 draft→identity_verified→trial_active→trial_review→remediation_active→trial_review→approved→archived 的唯一流程结束。
- 对外接口：提供版本化 HTTP JSON API，调用方通过批次资源、状态命令、观测记录、异常处置、审核与归档端点完成全部流程；服务支持 -addr=127.0.0.1:<port>，默认监听 127.0.0.1:19081，并在 PORT 为端口号时监听 127.0.0.1:<PORT>，绝不默认绑定 0.0.0.0。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/seedvault -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-708044bc-66cc-45b0-b489-bd6c44c9e57c-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-708044bc-66cc-45b0-b489-bd6c44c9e57c-arm64 linux/arm64

docker run -it benzhi-project-708044bc-66cc-45b0-b489-bd6c44c9e57c-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/seedvault -self-check -addr=127.0.0.1:19081`
