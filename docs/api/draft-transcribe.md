# 未发送录音的中文听写

给 iOS 和安卓。已登录才能调用。录音还在输入栏里，没有发出去。

```
messages.transcribeDraft#7e11da47 audio:bytes = messages.TranscribedAudio;
```

`audio` 是本机录好的 OGG/Opus、WAV 或 MP4，最大 8MB。服务端把它交给已经在跑的 SenseVoice（`TELLOMI_ASR_URL`，默认 `http://asr:8080/v1/transcribe`），只把文字放进 `messages.transcribedAudio`。

- 不调用 `messages.sendMessage` 或 `messages.sendMedia`。
- 不给对方发更新。也不给自己的其他设备发 `updateTranscribedAudio`。
- 音频只活在这一次请求里。识别完就丢掉，不进聊天库，不进对象存储。
- 未登录是 `USER_ID_INVALID`。空录音是 `MEDIA_EMPTY`。过长是 `AUDIO_TOO_LONG`。听写服务不可用是 `TRANSCRIPTION_UNAVAILABLE`。没有识别出字是 `TRANSCRIPTION_FAILED`。
- `pending` 固定是 false。正文在 `text`。

已经发出去的语音和圆形视频仍走 `messages.transcribeAudio#269e9a49`。那条会读已存储的文件，并给自己的其他设备推 `updateTranscribedAudio`。草稿不要用那条。
