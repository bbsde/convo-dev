-- __PLUGIN_NAME__ 插件 schema migration 1：从零建表（v1 基线）。
-- 「通用账号表标准列契约」：host 只认标准列，按 manifest.tables.accounts 的表名
-- 通用 CRUD；credentials/display 对 host 不透明（插件自管 JSON）。
--   - credentials：{"api_key":"...","base_url":"..."}（verify_key 组装回写）。
--   - external_id：账号唯一外部标识（同账号重复添加走 upsert 合并）。
--   - display：任意展示 JSON（卡片渲染用），verify_key 回写。

CREATE TABLE IF NOT EXISTS __PLUGIN_NAME___accounts (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id    TEXT,
    credentials    TEXT NOT NULL,
    display        TEXT NOT NULL DEFAULT '{}',
    enabled        INTEGER NOT NULL DEFAULT 1,
    expires_at     TEXT,
    last_used_at   TEXT,
    error_count    INTEGER NOT NULL DEFAULT 0,
    disabled_until TEXT,
    created_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx___PLUGIN_NAME___accounts_external_id
    ON __PLUGIN_NAME___accounts(external_id) WHERE external_id IS NOT NULL;
