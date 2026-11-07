# CPU English-to-simplified-Chinese sidecar.
# The int8 CTranslate2 weights are converted while the image is built.
# The running container does not download them and does not call another
# translator. See NOTICE for the CC-BY-4.0 attribution.
FROM python:3.11-slim-bookworm AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir 'ctranslate2==4.8.5' 'sentencepiece>=0.2,<0.3' 'transformers>=4.46,<5'
RUN pip install --no-cache-dir 'torch==2.6.0' --index-url https://download.pytorch.org/whl/cpu

ARG MODEL_ID=Helsinki-NLP/opus-mt-en-zh
RUN ct2-transformers-converter \
        --model "$MODEL_ID" \
        --output_dir /opt/translate/model \
        --quantization float32 \
        --copy_files source.spm target.spm \
        --low_cpu_mem_usage \
        --force \
    && test -s /opt/translate/model/model.bin \
    && test -s /opt/translate/model/source.spm \
    && test -s /opt/translate/model/target.spm

FROM python:3.11-slim-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir 'ctranslate2==4.8.5' 'sentencepiece>=0.2,<0.3'

COPY --from=build /opt/translate/model /opt/translate/model
COPY NOTICE server.py accuracy_test.py /opt/translate/

ENV TELLOMI_TRANSLATE_MODEL=/opt/translate/model \
    TELLOMI_TRANSLATE_THREADS=2 \
    OMP_NUM_THREADS=2 \
    PORT=8091 \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1
EXPOSE 8091
WORKDIR /opt/translate
USER nobody
CMD ["python", "-u", "/opt/translate/server.py"]
