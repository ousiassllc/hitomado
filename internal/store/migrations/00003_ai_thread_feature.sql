-- +goose Up
-- 「機能ごとに使う AI」に「要約・質問」（ai.features.threadAi。#376）を足す。以前は下書きと同じ設定を使っていたので、
-- 00002 を適用済みの DB（threadAi が無く draft だけがある）には、draft の選択を写す。すでに threadAi があれば変えない。

UPDATE app_settings
SET value = json_set(value, '$.threadAi', json(json_extract(value, '$.draft')))
WHERE key = 'ai.features'
  AND json_type(value) = 'object'
  AND json_type(value, '$.draft') = 'object'
  AND json_type(value, '$.threadAi') IS NULL;

-- +goose Down
-- threadAi は 00002 の形に無いので捨てる。
UPDATE app_settings
SET value = json_remove(value, '$.threadAi')
WHERE key = 'ai.features'
  AND json_type(value) = 'object';
