# Storage Backend API

[返回目录](./README.md)

存储连接由平台统一管理。完整配置与凭据仅对 SystemAdmin 开放；企业成员只能读取知识库绑定所需的安全投影（`id`、`name`、`provider`），不能创建、修改、测试或删除连接。

未显式设置 `storage_backend_id` 时，运行时使用唯一的平台默认连接。平台默认未设置时请求失败；显式 ID 不存在或不可用时也直接失败，不会回退到企业配置或环境变量。文件对象和资源目录仍按真实 `tenant_id` 与知识库命名空间隔离。

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/storage-backends` | Viewer+ | 安全能力投影，供知识库选择 |
| GET | `/system/admin/storage-backends` | SystemAdmin | 列出完整配置，凭据掩码 |
| POST | `/system/admin/storage-backends` | SystemAdmin | 创建并验证连接 |
| GET | `/system/admin/storage-backends/:id` | SystemAdmin | 获取连接详情，凭据掩码 |
| PUT | `/system/admin/storage-backends/:id` | SystemAdmin | 更新名称、凭据或状态 |
| DELETE | `/system/admin/storage-backends/:id` | SystemAdmin | 软删除连接 |
| PUT | `/system/admin/storage-backends/:id/default` | SystemAdmin | 设为平台默认 |
| POST | `/system/admin/storage-backends/test` | SystemAdmin | 测试未保存配置 |
| POST | `/system/admin/storage-backends/:id/test` | SystemAdmin | 测试已保存配置 |
| GET | `/system/admin/storage-backends/types` | SystemAdmin | 列出允许的 provider |

支持 `local`、`minio`、`cos`、`tos`、`s3`、`oss`、`ks3`、`obs`。配置继续使用 `config` 归一化对象，包括 `mode`、`endpoint`、`region`、`access_key_id`、`secret_access_key`、`bucket_name`、`path_prefix`、`app_id`、`use_ssl`、`force_path_style` 和临时 bucket 字段。响应中的密钥使用 `***` 掩码；更新提交掩码时保留原值。

创建会校验配置、SSRF 策略并测试连通性。同名活动连接不允许重复；不会根据名称、地址或凭据合并已有连接。物理位置字段创建后不可变，凭据可以轮换。

禁用或删除会拒绝以下连接：

- 当前平台默认连接；
- 被任意企业知识库或活动资源引用；
- 被平台技能 catalog 或 install 的 `storage://<backend-id>/...` bundle 引用。

检查覆盖所有企业。切换默认、禁用和删除使用同一事务锁边界。业务文件保存仍传入真实企业 ID，因此多个企业使用同一连接时仍落在各自的数据命名空间。

首次配置示例：

```json
{
  "name": "platform-local",
  "provider": "local",
  "config": { "path_prefix": "weknora" },
  "status": "active"
}
```

创建后对返回的 ID 调用 `PUT /system/admin/storage-backends/:id/default`。平台不会自动选取默认连接。
