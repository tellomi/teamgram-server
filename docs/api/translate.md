# 聊天翻译（只英译简体中文）

`messages.translateText`、`messages.translateRichMessage`，以及带 `translate_to_lang` 的 `messages.composeMessageWithAI`，都只走自建翻译服务。不会调用百炼、Google、Telegram、Apple，失败也不改去调它们。

`to_lang` 只能是 `zh`、`zh-cn`、`zh-hans`。别的语言是 `TO_LANG_INVALID`。服务没就绪是 `TRANSLATE_REQ_FAILED`（compose 里的翻译失败仍是 `AICOMPOSE_TIMEOUT`）。

## 开关

`help.getAppConfig`：

| 键 | 何时是 `enabled` |
| --- | --- |
| `translations_manual_enabled` | `TELLOMI_TRANSLATE_URL` 的 `/health` 返回 200 |
| `translations_auto_enabled` | 上面成立，并且 `TELLOMI_TRANSLATE_AUTO=1` |
| `tellomi_translate_enabled` | 和手动开关同一条件，布尔值 |

现在的机器上没有模型权重，`/health` 不会是 200。这三个开关保持关闭。`TELLOMI_TRANSLATE=1` 和 `TELLOMI_AI_API_KEY` 不再打开翻译。

单聊「不翻译」仍是 `messages.togglePeerTranslations`。自动翻译默认关，避免模型一健康就把每个会话都翻掉。

## sidecar

`GET {TELLOMI_TRANSLATE_URL}/health`

`POST {TELLOMI_TRANSLATE_URL}/v1/translate`

```json
{ "texts": ["hello"], "from": "en", "to": "zh" }
```

```json
{ "texts": ["你好"] }
```

打算用的模型是 Helsinki-NLP `opus-mt-en-zh`（CC-BY-4.0，大约 300MB）。仓库里没有权重，`services/translate/server.py` 在没有加载好的运行时之前一直报不健康。这台环境大约 4 核、可用内存不到 5GB，没有做过隔离压测，所以不能把开关打开。
