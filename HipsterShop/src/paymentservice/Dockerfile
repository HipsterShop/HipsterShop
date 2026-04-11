FROM node:20.20.1-alpine@sha256:b88333c42c23fbd91596ebd7fd10de239cedab9617de04142dde7315e3bc0afa AS builder
RUN apk add --update --no-cache \
    python3 \
    make \
    g++
WORKDIR /usr/src/app
COPY package*.json ./
RUN npm install --only=production
FROM alpine:3.23.3@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659
RUN apk add --no-cache nodejs
WORKDIR /usr/src/app
COPY --from=builder /usr/src/app/node_modules ./node_modules
COPY . .
EXPOSE 50051
ENTRYPOINT [ "node", "index.js" ]
