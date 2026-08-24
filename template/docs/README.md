# __PLUGIN_NAME__

> TODO：一句话说清这个插件接入什么平台、怎么配置。

- 接入平台：
- 接入形态：API key 直配 / OAuth / 订阅 Cookie…
- 支持端点：chat / embed / image / speech / video
- 上游文档：

## 使用

1. Convo 控制台 → 平台管理 → 添加账号（见插件 UI）
2. 客户端以 OpenAI 兼容方式调用，`model` 填 `__PLUGIN_NAME__:<model-id>` 或裸 `<model-id>`

## 开发

- 构建：`./build.sh`（产物在 `dist/`）
- 契约文档（本目录）：[manifest.md](./manifest.md)（plugin.json 字段）· [pdk-api.md](./pdk-api.md)（SDK API）· [rules.md](./rules.md)（**硬性约定与坑**）· [testing.md](./testing.md)（调试/分发/自查）
- 逆向笔记（端点指纹/风控/模型清单事实源）：[NOTES.md](./NOTES.md)
