# MCP Service API

[返回目录](./README.md)

MCP 连接定义、静态凭据和工具审批策略由平台统一管理。企业运行时仍携带真实
Tenant 与 Principal：非 OAuth 连接按 `service + tenant` 隔离，OAuth 连接按
`service + tenant + principal_type + principal_id` 隔离。OAuth 客户端注册和令牌
继续存储在企业/身份范围内。

## 平台配置接口

以下接口仅接受已登录的 `SystemAdmin`，不接受 API Key：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/system/admin/mcp-services` | 获取完整全局 MCP 服务列表 |
| POST | `/system/admin/mcp-services` | 创建全局服务定义 |
| GET | `/system/admin/mcp-services/:id` | 获取服务详情 |
| PUT | `/system/admin/mcp-services/:id` | 更新服务定义 |
| DELETE | `/system/admin/mcp-services/:id` | 删除服务定义 |
| POST | `/system/admin/mcp-services/:id/test` | 用临时客户端测试连接 |
| GET | `/system/admin/mcp-services/:id/tools` | 用临时客户端发现工具 |
| GET | `/system/admin/mcp-services/:id/resources` | 用临时客户端发现资源 |
| PUT | `/system/admin/mcp-services/:id/credentials` | 写入静态凭据 |
| DELETE | `/system/admin/mcp-services/:id/credentials/:field` | 删除 `api_key` 或 `token` |
| GET | `/system/admin/mcp-services/:id/tool-approvals` | 获取全局工具审批策略 |
| PUT | `/system/admin/mcp-services/:id/tool-approvals/:tool_name` | 设置全局工具审批策略 |

控制面测试不会构造虚假的 Tenant，也不会写入或复用企业运行时客户端缓存。更新
连接配置或凭据会关闭该服务在所有 Tenant/Principal 下的缓存连接，下一次调用按
新配置重连。

主资源响应不返回 `api_key`、`token`，只在 `credentials` 中报告各字段是否已
配置。`is_builtin` 仍表示内置、不可编辑的服务，不表示平台所有权。

## 企业运行时目录

`GET /mcp-services` 返回可供企业 Agent 选择的全局服务安全投影，仅包含 ID、名称、
描述和启用状态。普通 Bearer 用户可读取；已有 Platform API Key 能力策略仍适用。
不存在或已删除的显式 Service ID 会返回错误，不会回退到其他服务。

## OAuth

OAuth 操作是企业/身份范围的运行时接口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/mcp-services/:id/oauth/authorize-url` | 为当前 Tenant/Principal 发起授权 |
| GET | `/mcp-services/:id/oauth/status` | 查询当前 Tenant/Principal 授权状态 |
| DELETE | `/mcp-services/:id/oauth/token` | 撤销当前 Tenant/Principal 令牌 |
| GET | `/mcp-oauth/callback` | OAuth 服务端回调，使用单次 state 鉴权 |
| POST | `/agent/mcp-oauth-resolutions/:pending_id` | 完成对话内授权并恢复调用 |
| POST | `/agent/mcp-oauth-resolutions/:pending_id/cancel` | 跳过对话内授权 |

同一用户在两个 Tenant 中会得到两条不同的 OAuth Client 注册记录和 Token 记录。
回调 state 绑定 Tenant、Principal 与 Service，不能跨企业复用。

## 工具人工审批

平台策略通过 `/system/admin/mcp-services/:id/tool-approvals/*` 配置。命中
`require_approval=true` 后，真实企业会话中的用户通过
`POST /agent/tool-approvals/:pending_id` 提交：

```json
{
  "decision": "approve",
  "modified_args": {"path": "/tmp/target.txt"},
  "reason": "已确认"
}
```

`decision` 只能是 `approve` 或 `reject`；`modified_args` 必须是非 null JSON
对象。待审批记录同时校验 Tenant 与 Principal，只有会话所有者能处理。
