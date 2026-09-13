# Skills API

沙箱连接、技能目录、安装与安装器智能体都是平台共享配置。这些接口不接受 `tenant_id`，只允许 tenantless SystemAdmin 的浏览器会话调用；平台 API Key 与企业成员都不能调用。

企业的脚本执行禁用策略仍属于企业，由当前登录身份通过 `GET/PUT /sandbox-policy` 读写。平台管理员可以为技能保存平台通用默认变量；用户私有变量仍按真实企业和 principal 隔离。

[返回目录](./README.md)

## 平台技能目录

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/system/admin/skills` | 列出目录技能及其沙箱安装情况 |
| POST | `/system/admin/skills` | 从 zip 或 `source` 注册目录技能 |
| POST | `/system/admin/skills/{id}/install` | 安装到一个或多个沙箱连接 |
| DELETE | `/system/admin/skills/{id}` | 删除未被安装引用的目录技能 |
| GET | `/system/admin/skills/{id}/files` | 列出目录存档文件 |
| GET | `/system/admin/skills/{id}/files/content?path=...` | 读取目录存档文件 |

上传 zip 或从公开来源注册：

```curl
curl --location 'http://localhost:8080/api/v1/system/admin/skills' \
--header 'Authorization: Bearer <system-admin-token>' \
--form 'file=@"skill.zip"'

curl --location 'http://localhost:8080/api/v1/system/admin/skills' \
--header 'Authorization: Bearer <system-admin-token>' \
--header 'Content-Type: application/json' \
--data '{"source":"@owner/slug"}'
```

`source` 可以是 ClawHub slug、ClawHub/SkillHub/skills.sh/GitHub/GitLab 页面，或直接的 zip / `SKILL.md` URL。来源必须可匿名读取；私有源请先导出 zip 再上传。

安装已注册技能：

```curl
curl --location 'http://localhost:8080/api/v1/system/admin/skills/{id}/install' \
--header 'Authorization: Bearer <system-admin-token>' \
--header 'Content-Type: application/json' \
--data '{"sandbox_config_ids":["cfg-1","cfg-2"]}'
```

响应中 `installs` 是 `sandbox_config_id -> skill_id` 映射；部分失败时 `errors` 保留各沙箱的失败原因。

## 技能安装器智能体

安装器是固定的平台内置智能体 `builtin-skill-installer`，沿用平台智能体接口：

```curl
curl --location 'http://localhost:8080/api/v1/system/admin/agents/builtin-skill-installer' \
--header 'Authorization: Bearer <system-admin-token>'

curl --location --request PUT \
'http://localhost:8080/api/v1/system/admin/agents/builtin-skill-installer' \
--header 'Authorization: Bearer <system-admin-token>' \
--header 'Content-Type: application/json' \
--data '{"config":{"model_id":"model-id"}}'
```

## 沙箱连接与默认值

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/system/admin/sandbox-configs` | 列出或创建平台沙箱连接 |
| GET/PUT/DELETE | `/system/admin/sandbox-configs/{id}` | 读取、更新或删除连接 |
| PUT | `/system/admin/sandbox-configs/default` | 设置新会话使用的默认连接 |
| POST | `/system/admin/sandbox-configs/check` | 检查未保存或已保存连接 |
| POST | `/system/admin/sandbox-configs/templates/query` | 查询或构建标准模板 |
| GET | `/system/admin/sandbox-configs/{id}/sandboxes` | 查看跨企业的当前占用 |

`GET` 返回的每条连接都含 `is_default`。切换默认值：

```curl
curl --location --request PUT \
'http://localhost:8080/api/v1/system/admin/sandbox-configs/default' \
--header 'Authorization: Bearer <system-admin-token>' \
--header 'Content-Type: application/json' \
--data '{"config_id":"cfg-2"}'
```

切换默认值不改写已有会话的沙箱绑定。

## 沙箱上的技能

以下接口都以 `/system/admin/sandbox-configs/{id}/skills` 为根：

| 方法 | 相对路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/` | 列出或安装技能 |
| GET/PATCH/DELETE | `/{skillId}` | 读取、启停、设置平台默认变量或卸载技能 |
| POST | `/{skillId}/reinstall` | 用目录存档重试安装 |
| POST | `/{skillId}/stop` | 停止进行中的安装 |
| GET | `/{skillId}/install-events` | 通过 SSE 跟随进度 |
| GET | `/{skillId}/transcript` | 通过 SSE 跟随安装器记录 |
| GET | `/{skillId}/transcript/history` | 读取持久化的安装记录 |
| GET | `/{skillId}/files` | 列出已安装文件 |
| GET | `/{skillId}/files/content?path=...` | 读取已安装文件 |

安装、重装与卸载是异步操作。使用 `install-events` 中的 `done` 字段判定完成。安装记录保存在技能安装行，不会创建企业业务会话。

## 企业脚本策略

```curl
curl --location 'http://localhost:8080/api/v1/sandbox-policy' \
--header 'Authorization: Bearer <enterprise-admin-token>'

curl --location --request PUT 'http://localhost:8080/api/v1/sandbox-policy' \
--header 'Authorization: Bearer <enterprise-admin-token>' \
--header 'Content-Type: application/json' \
--data '{"scripts_disabled":true}'
```

服务端从认证上下文取得企业，路径和请求体都不接受 `tenant_id`。禁用后，该企业不能准备或执行技能脚本。

## 平台默认变量与用户私有变量

技能安装会声明所需变量的名称、说明和是否必填。SystemAdmin 可通过 `PATCH /system/admin/sandbox-configs/{id}/skills/{skillId}` 的 `envs` 字段保存平台通用默认值；接口只返回是否已设置，不回读明文。用户没有私有值时才使用平台默认值。

用户私有值使用以下接口，并按当前认证上下文中的真实企业和 principal 保存。一个企业的私有值不会被另一个企业读取或执行，网页登录与 API Key 也是不同 principal。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/me/env-vars` | 列出当前 principal 可见的变量声明与设置状态 |
| PUT/DELETE | `/me/env-vars/skill` | 设置或删除当前 principal 的技能变量 |
| PUT/DELETE | `/me/env-vars/sandbox` | 设置或删除当前 principal 的沙箱变量 |

这些接口也不会回读已保存的明文值。
