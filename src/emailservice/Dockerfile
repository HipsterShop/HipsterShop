FROM python:3.11-alpine AS base
FROM base AS builder
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
RUN apk update \
    && apk add --no-cache g++ linux-headers \
    && rm -rf /var/cache/apk/*
COPY requirements.txt .
RUN pip install -r requirements.txt
FROM base
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV ENABLE_PROFILER=1
RUN apk update \
    && apk add --no-cache libstdc++ \
    && rm -rf /var/cache/apk/*
WORKDIR /email_server
COPY --from=builder /usr/local/lib/python3.11/ /usr/local/lib/python3.11/
COPY --from=builder /usr/local/bin/ /usr/local/bin/
COPY . .
EXPOSE 8080
ENTRYPOINT [ "python", "email_server.py" ]
