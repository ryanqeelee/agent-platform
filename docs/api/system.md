# 系统管理 API

[返回目录](./README.md)

平台基础设施配置由不隶属企业的 SystemAdmin 统一维护，管理接口使用登录 JWT，不接受企业身份或 API Key 代替平台权限。企业知识库、资源和会话仍属于各自企业；企业页面只读取使用功能所需的能力清单。

| 方法   | 路径                              | 描述                   |
| ------ | --------------------------------- | ---------------------- |
| GET    | `/system/capabilities`            | 获取部署能力清单       |
| GET    | `/system/info`                    | 企业可见版本能力摘要           |
| GET    | `/system/admin/info`              | 平台完整运行信息 |
| GET    | `/system/parser-engines`          | 企业可用解析能力清单（不含配置） |
| GET    | `/system/admin/parser-engines`    | 平台解析能力与连接状态 |
| GET/PUT | `/system/admin/parser-engine-config` | 读取/保存平台共享解析配置 |
| GET/PUT | `/system/admin/chat-history-config` | 读取/保存平台消息索引策略 |
| GET | `/system/admin/chat-history-stats` | 汇总企业私有消息索引统计 |
| GET/PUT | `/system/admin/memory-runtime-config` | 读取/保存平台共享记忆运行配置 |
| POST   | `/system/admin/parser-engines/check`    | 检查解析引擎可用性     |

## GET `/system/capabilities` - 获取部署能力清单

返回当前部署版本，以及各功能模块是否已在后端注册对应路由。`supported: false` 表示 SPA 应隐藏相关入口；字段缺失或接口不可用时不应据此清空整个菜单（fail-open），但 Lite 版会始终将 `organizations` 标记为不支持。

**权限**：Viewer+（租户成员）；任意有效 API Key 可读（`apiKeyAny`）。

**请求**:

```curl
curl --location 'http://localhost:8080/api/v1/system/capabilities' \
--header 'Authorization: Bearer <token>' \
--header 'Content-Type: application/json'
```

**响应**:

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "edition": "standard",
    "capabilities": {
      "organizations": { "supported": true },
      "agents": { "supported": true },
      "integrations.im": { "supported": true },
      "integrations.embed": { "supported": false, "reason": "route_not_registered" },
      "integrations.api": { "supported": true },
      "settings.mcp": { "supported": true },
      "settings.websearch": { "supported": true },
      "settings.vectorstore": { "supported": true },
      "settings.storage": { "supported": true },
      "settings.sandbox": { "supported": true }
    }
  }
}
```

Lite 版示例（共享空间不可用）:

```json
{
  "capabilities": {
    "organizations": {
      "supported": false,
      "reason": "not_supported_in_lite"
    }
  }
}
```

## 系统信息

企业成员读取 `/system/info` 的受限摘要；平台运营端使用 `/system/admin/info` 获取版本、运行组件和诊断信息。后者只接受 SystemAdmin 登录身份，企业账号及 API Key 无权读取。

共享存储和向量连接的配置、测试及默认项分别见 [存储配置](./storage-backend.md) 和 [向量连接](./vector-store.md)。

## 平台消息索引策略

`GET/PUT /system/admin/chat-history-config` 维护唯一的平台消息索引策略，请求体只有
`enabled` 和 `embedding_model_id`。关闭时允许模型为空；开启时必须选择一个处于 active 状态的
平台 Embedding 模型。此接口不接受企业 ID，企业管理员也不能覆盖该配置。

策略开启后，每个企业只在第一条符合条件的消息需要索引时创建自己的隐藏知识库。知识库、消息和
搜索结果仍按真实企业及用户身份隔离，不存在承载多企业数据的虚拟平台企业或全局知识库。关闭策略
只阻止新的索引任务；已经排队的任务可以完成。

一旦任一企业已经绑定消息索引知识库，Embedding 模型不能直接切换，因为可能仍有异步写入。平台
需先清理这些内部索引，再选择新模型。`GET /system/admin/chat-history-stats` 返回绑定企业数与已索引
消息总数，用于判断当前平台状态。

## GET `/system/parser-engines` - 获取企业可用解析能力

已认证的企业成员可读。响应仅含 `Name`、`Description`、`FileTypes`、`Available`，不返回服务地址、密钥或连接诊断。平台管理页使用 `/system/admin/parser-engines` 获取管理状态；配置本身由 `/system/admin/parser-engine-config` 维护，密钥返回掩码，保存掩码保留原值。

**请求**:

```curl
curl --location 'http://localhost:8080/api/v1/system/parser-engines' \
--header 'X-API-Key: sk-xxxxx' \
--header 'Content-Type: application/json'
```

**响应**:

```json
{
  "code": 0,
  "msg": "success",
  "data": [
    {"Name": "docreader", "Description": "文档解析", "FileTypes": ["pdf"], "Available": true}
  ]
}
```

## POST `/system/admin/parser-engines/check` - 检查解析引擎可用性

**请求**:

```curl
curl --location 'http://localhost:8080/api/v1/system/admin/parser-engines/check' \
--header 'Authorization: Bearer <system-admin-token>' \
--header 'Content-Type: application/json' \
--data '{
    "addr": "http://docreader:8000"
}'
```

**响应**:

```json
{
    "data": [
        {
            "name": "docreader",
            "label": "DocReader",
            "description": "高精度文档解析引擎",
            "available": true
        }
    ],
    "success": true
}
```

