# CPU SenseVoice-Small sidecar. The int8 ONNX weights are fetched while the
# image is built (GitHub Actions can reach GitHub releases). The running
# container does not download from Hugging Face or ModelScope.
# sherpa-onnx is Apache-2.0. The weights follow FunASR MODEL_LICENSE v1.1.
# See NOTICE.
FROM python:3.11-slim-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg tar bzip2 \
    && rm -rf /var/lib/apt/lists/*

ARG MODEL_URL=https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2
RUN curl -fsSL -o /tmp/model.tar.bz2 "$MODEL_URL" \
    && mkdir -p /opt/asr/model \
    && tar -xjf /tmp/model.tar.bz2 -C /opt/asr/model --strip-components=1 \
    && rm -f /tmp/model.tar.bz2 \
    && rm -rf /opt/asr/model/test_wavs /opt/asr/model/export-onnx.py \
    && test -s /opt/asr/model/model.int8.onnx \
    && test -s /opt/asr/model/tokens.txt

RUN pip install --no-cache-dir 'sherpa-onnx==1.13.8' 'numpy>=1.26,<3'

COPY NOTICE server.py /opt/asr/
ENV ASR_THREADS=2 \
    OMP_NUM_THREADS=2 \
    MKL_NUM_THREADS=2 \
    OPENBLAS_NUM_THREADS=2 \
    ASR_MODEL=/opt/asr/model/model.int8.onnx \
    ASR_TOKENS=/opt/asr/model/tokens.txt \
    ASR_PORT=8080 \
    ASR_MAX_SECONDS=300 \
    ASR_CHUNK_SECONDS=30 \
    ASR_MAX_INFLIGHT=2 \
    ASR_MAX_QUEUE=8 \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1
EXPOSE 8080
WORKDIR /opt/asr
USER nobody
CMD ["python", "-u", "/opt/asr/server.py"]
