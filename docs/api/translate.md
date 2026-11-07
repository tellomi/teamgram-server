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

`TELLOMI_TRANSLATE=1` 和 `TELLOMI_AI_API_KEY` 不再打开翻译。`TELLOMI_TRANSLATE_URL` 空着，或者 `/health` 不是 200，三个开关都保持关闭。

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

## 模型

Helsinki-NLP `opus-mt-en-zh`，许可证 CC-BY-4.0。镜像构建时把权重转成 CTranslate2 float32（`model.bin` 约 297MB）。运行中的容器不再下载，也不调用别的翻译服务。许可证和出处在 `services/translate/NOTICE`。

`/health` 只有在模型完成一句预热（`Thank you.`）之后才是 200。目录不存在或加载失败是 503，`reason` 为 `model_missing` 或 `runtime_not_loaded`。

这个检查点对单独的 `hello` 会重复同一个词。PyTorch 原模型和这边一样。服务端遇到整句只有同一个 token 时只留一份，所以 `hello` 返回 `你好`。正常句子不动。`services/translate/accuracy_test.py` 对照的是这个模型自己的 beam 4 结果：`Thank you.` → `谢谢`，`Good morning.` → `早上好,你好吗?`，`I like this book.` → `我喜欢这本书`。

2026-11-07 在这台 4 核机器上，模型加载后这四句是 54–109ms，进程最大 RSS 约 530MB。没有对生产做压测。

## 部署

镜像名 `translate`，由 `.github/workflows/docker-acr.yml` 在 master 上构建。`docker-compose.yaml` 里的服务带 profile `translate`，`deploy-pull.sh` 不会拉起它。

```bash
# 确认宿主机还能空出大约 1GB 再启动
docker compose --profile translate up -d translate
# teamgram 环境，不进默认 compose
TELLOMI_TRANSLATE_URL=http://translate:8091
# 需要自动翻译时才加
TELLOMI_TRANSLATE_AUTO=1
```

容器限制 1GB 内存、1 个 CPU。镜像内 `TELLOMI_TRANSLATE_MODEL=/opt/translate/model`，`TELLOMI_TRANSLATE_THREADS=2`。这台开发机测下来 RSS 约 530MB，1GB 限制装得下。生产机器还剩多少内存没有在这里量过，所以默认不启动，由你决定要不要开。
