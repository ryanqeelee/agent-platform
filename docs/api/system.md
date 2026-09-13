# 系统管理 API

[返回目录](./README.md)

平台基础设施配置由不隶属企业的 SystemAdmin 统一维护，管理接口使用登录 JWT，不接受企业身份或 API Key 代替平台权限。企业知识库、资源和会话仍属于各自企业；企业页面只读取使用功能所需的能力清单。

| 方法   | 路径                              | 描述                   |
| ------ | --------------------------------- | ---------------------- |
| GET    | `/system/capabilities`            | 获取部署能力清单       |
| GET    | `/system/info`                    | 获取系统信息           |
| GET    | `/system/parser-engines`          | 企业可用解析能力清单（不含配置） |
| GET    | `/system/admin/parser-engines`    | 平台解析能力与连接状态 |
| GET/PUT | `/system/admin/parser-engine-config` | 读取/保存平台共享解析配置 |
| POST   | `/system/admin/parser-engines/check`    | 检查解析引擎可用性     |
| GET    | `/system/storage-engine-status`   | 获取存储引擎状态       |
| POST   | `/system/storage-engine-check`    | 检查存储引擎连通性     |

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

## GET `/system/info` - 获取系统信息

**请求**:

```curl
curl --location 'http://localhost:8080/api/v1/system/info' \
--header 'X-API-Key: sk-xxxxx' \
--header 'Content-Type: application/json'
```

**响应**:

```json
{
    "data": {
        "version": "1.2.0",
        "edition": "community",
        "commit_id": "a1b2c3d",
        "build_time": "2025-08-12T08:00:00Z",
        "go_version": "go1.21.5",
        "keyword_index_engine": "bleve",
        "vector_store_engine": "milvus",
        "graph_database_engine": "neo4j",
        "minio_enabled": true,
        "db_version": "20250810_001"
    },
    "success": true
}
```

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

## GET `/system/storage-engine-status` - 获取存储引擎状态

**请求**:

```curl
curl --location 'http://localhost:8080/api/v1/system/storage-engine-status' \
--header 'Authorization: Bearer <system-admin-token>' \
--header 'Content-Type: application/json'
```

**响应**:

```json
{
    "data": {
        "engines": [
            {
                "name": "minio",
                "available": true,
                "description": "MinIO 对象存储"
            },
            {
                "name": "cos",
                "available": false,
                "description": "腾讯云 COS 对象存储"
            },
            {
                "name": "s3",
                "available": false,
                "description": "AWS S3 对象存储"
            },
            {
                "name": "oss",
                "available": false,
                "description": "阿里云 OSS 对象存储"
            }
        ],
        "minio_env_available": true
    },
    "success": true
}
```

## POST `/system/storage-engine-check` - 检查存储引擎连通性

**请求**:

```curl
curl --location 'http://localhost:8080/api/v1/system/storage-engine-check' \
--header 'Authorization: Bearer <system-admin-token>' \
--header 'Content-Type: application/json' \
--data '{
    "provider": "minio",
    "minio": {
        "endpoint": "localhost:9000",
        "access_key": "minioadmin",
        "secret_key": "minioadmin",
        "bucket": "weknora",
        "use_ssl": false
    }
}'
```

**响应**:

```json
{
    "data": {
        "ok": true,
        "message": "连接成功",
        "bucket_created": false
    },
    "success": true
}
```


