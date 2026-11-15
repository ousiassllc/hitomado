-- +goose Up
-- AI 設定を「つなぎ先」と「機能ごとに使う AI」に分ける（#376）。
-- 旧 ai.config（既定のプロバイダ・モデル・Base URL・エフォート）は、「AI で下書き」の選択（ai.features.draft）と、
-- OpenAI 互換の Base URL（ai.connection）へ移し、旧キーは消す。API キー・ChatGPT のトークン・同意（ai.consent）は変えない。

INSERT INTO app_settings (key, value, updated_at)
SELECT 'ai.features', json_object('draft', json_remove(value, '$.baseUrl')), updated_at
FROM app_settings
WHERE key = 'ai.config'
  AND json_type(value) = 'object';

INSERT INTO app_settings (key, value, updated_at)
SELECT 'ai.connection', json_object('openaiBaseUrl', json_extract(value, '$.baseUrl')), updated_at
FROM app_settings
WHERE key = 'ai.config'
  AND json_type(value) = 'object'
  AND json_extract(value, '$.provider') = 'openai'
  AND coalesce(json_extract(value, '$.baseUrl'), '') <> '';

DELETE FROM app_settings WHERE key = 'ai.config';

-- +goose Down
INSERT INTO app_settings (key, value, updated_at)
SELECT 'ai.config',
       CASE
           WHEN json_extract(f.value, '$.draft.provider') = 'openai'
                AND coalesce(json_extract(c.value, '$.openaiBaseUrl'), '') <> ''
           THEN json_set(json_extract(f.value, '$.draft'), '$.baseUrl', json_extract(c.value, '$.openaiBaseUrl'))
           ELSE json_extract(f.value, '$.draft')
       END,
       f.updated_at
FROM app_settings AS f
LEFT JOIN app_settings AS c ON c.key = 'ai.connection'
WHERE f.key = 'ai.features'
  AND json_type(f.value, '$.draft') = 'object';

DELETE FROM app_settings WHERE key IN ('ai.features', 'ai.connection');
