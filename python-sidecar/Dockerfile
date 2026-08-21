# Pin TARGETPLATFORM so buildx on an arm64 Mac cannot tag an arm64 python
# image as linux/amd64 (same class of bug as jobshout-web / jobshout-server).
FROM --platform=$TARGETPLATFORM python:3.12-slim
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "amd64" ]; then uname -m | grep -qx x86_64; fi

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/ ./app/

EXPOSE 8001

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]
