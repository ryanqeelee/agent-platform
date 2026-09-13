# Web Search API

[返回目录](./README.md)

网络搜索 Provider 定义、默认选择和凭据由平台统一管理。企业请求只携带显式
Provider ID，或使用平台唯一的默认 Provider；不存在的显式 ID 会返回错误，不会
回退到其他配置。原有空间级 `web-search-config` KV 接口已移除。

## 鉴权与接口

平台配置接口仅接受已登录的 `SystemAdmin`，不接受 API Key：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/system/admin/web-search-providers` | 获取完整全局 Provider 列表 |
| POST | `/system/admin/web-search-providers` | 创建全局 Provider |
| GET | `/system/admin/web-search-providers/types` | 获取 Provider 类型及参数元数据 |
| POST | `/system/admin/web-search-providers/test` | 用未保存配置测试连接，不落库 |
| GET | `/system/admin/web-search-providers/:id` | 获取全局 Provider 详情 |
| PUT | `/system/admin/web-search-providers/:id` | 更新非凭据字段，可设为全局默认 |
| DELETE | `/system/admin/web-search-providers/:id` | 删除非默认 Provider |
| POST | `/system/admin/web-search-providers/:id/test` | 测试已保存 Provider |
| PUT | `/system/admin/web-search-providers/:id/credentials` | 写入 API Key |
| DELETE | `/system/admin/web-search-providers/:id/credentials/api_key` | 删除 API Key |

`GET /web-search-providers` 是企业运行时的安全投影。普通 Bearer 用户只会看到
平台默认 Provider 是否已配置，不会看到 ID、地址、代理或凭据。已有的 Platform
API Key 能力策略仍适用于该运行时接口。

## 创建和更新

```json
{
  "name": "Keenable",
  "provider": "keenable",
  "description": "平台网络搜索",
  "parameters": {
    "base_url": "https://search.example.com",
    "extra_config": {
      "search_engine": "search_std"
    }
  },
  "is_default": true
}
```

`provider` 创建后不可修改。主资源响应不会返回 `api_key`，只在
`credentials.api_key.configured` 中返回是否已配置。写入凭据使用专用接口：

```http
PUT /api/v1/system/admin/web-search-providers/{id}/credentials
Content-Type: application/json

{"api_key":"new-secret"}
```

全局只能有一个 `is_default=true` 的有效 Provider。设置默认与删除操作在仓储事务
中串行化；默认 Provider 必须先切换后才能删除。升级迁移会保留所有 Provider ID，
并清除旧空间默认标记，等待 SystemAdmin 明确选择全局默认。

## 连接测试

保存前测试接收与创建接口相同的 `provider` 和 `parameters`：

```http
POST /api/v1/system/admin/web-search-providers/test
Content-Type: application/json

{"provider":"keenable","parameters":{"api_key":"test-secret"}}
```

保存后测试使用 `POST /system/admin/web-search-providers/:id/test`。两种测试都执行
真实样本搜索，但保存前测试不会写数据库。
