# Vector Store API

[返回目录](./README.md)

向量连接由平台统一管理。完整连接参数仅对 SystemAdmin 开放；企业成员只能读取知识库绑定所需的安全投影（`id`、`name`、`engine_type`）。

未显式设置 `vector_store_id` 时，检索使用唯一的平台默认连接。默认未设置、显式 ID 不存在或引擎不可用时请求失败，不会回退到 `RETRIEVE_DRIVER` 虚拟连接。向量库中的 tenant、knowledge、source 与 chunk 过滤保持不变。

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/vector-stores` | Viewer+ | 安全能力投影，供知识库选择 |
| GET | `/system/admin/vector-stores` | SystemAdmin | 列出完整配置，凭据掩码 |
| POST | `/system/admin/vector-stores` | SystemAdmin | 创建并验证连接 |
| GET | `/system/admin/vector-stores/:id` | SystemAdmin | 获取连接详情，凭据掩码 |
| PUT | `/system/admin/vector-stores/:id` | SystemAdmin | 更新名称 |
| DELETE | `/system/admin/vector-stores/:id` | SystemAdmin | 软删除连接 |
| PUT | `/system/admin/vector-stores/:id/default` | SystemAdmin | 设为平台默认 |
| POST | `/system/admin/vector-stores/test` | SystemAdmin | 测试未保存配置 |
| POST | `/system/admin/vector-stores/:id/test` | SystemAdmin | 测试已保存配置 |
| GET | `/system/admin/vector-stores/types` | SystemAdmin | 获取引擎字段元数据 |

支持 PostgreSQL、SQLite、Elasticsearch、OpenSearch、Qdrant、Milvus、Weaviate、Doris 和 Tencent VectorDB。PostgreSQL 与 SQLite 仅支持复用应用数据库：

```json
{
  "name": "platform-postgres",
  "engine_type": "postgres",
  "connection_config": {
    "use_default_connection": true
  },
  "index_config": {}
}
```

自定义 PostgreSQL/SQLite 连接不会被接受。其他引擎继续使用类型元数据声明的 `connection_config` 与 `index_config`。响应中的 `password`、`api_key` 等敏感字段使用 `***` 掩码。

创建会校验参数、SSRF 策略并测试连接。连接不会按名称、地址或索引配置去重；活动显示名保持唯一。更新仅允许修改名称。

平台默认连接或被任意企业知识库引用的连接不能删除。删除检查覆盖所有企业，并与默认切换共享事务锁边界；成功删除后复用现有 registry 生命周期注销该连接。

创建后对返回的 ID 调用 `PUT /system/admin/vector-stores/:id/default`。平台不会从环境变量自动生成或选取默认连接。
